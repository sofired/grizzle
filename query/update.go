package query

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// UpdateBuilder constructs an UPDATE query.
type UpdateBuilder struct {
	table     TableSource
	sets      []setClause // explicit col = val pairs
	where     expr.Expression
	returning []expr.SelectableColumn
	limit     int // 0 = no limit; MySQL/SQLite only
	buildErr  error
}

type setClause struct {
	col string
	val any
}

// Update starts an UPDATE <table> query.
func Update(t TableSource) *UpdateBuilder {
	return &UpdateBuilder{table: t}
}

// Set adds a col = val assignment. Call multiple times to set multiple columns.
//
//	query.Update(UsersT).Set("name", "Alice").Set("enabled", true)
func (b *UpdateBuilder) Set(col string, val any) *UpdateBuilder {
	cp := *b
	cp.sets = append(append([]setClause(nil), cp.sets...), setClause{col: col, val: val})
	return &cp
}

// SetStruct extracts column assignments from a struct's db-tagged fields,
// equivalent to Drizzle's .set({ ... }). Pointer fields with nil values
// are skipped; non-nil pointer fields are dereferenced.
//
//	type UserUpdate struct {
//	    Name    *string `db:"name"`
//	    Enabled *bool   `db:"enabled"`
//	}
//	query.Update(UsersT).SetStruct(UserUpdate{Name: ptr("Alice")})
func (b *UpdateBuilder) SetStruct(row any) *UpdateBuilder {
	cp := *b
	cols, vals, err := structSetsForUpdate(row)
	if err != nil {
		if cp.buildErr == nil {
			cp.buildErr = err
		}
		return &cp
	}
	cp.sets = append([]setClause(nil), b.sets...)
	for i, col := range cols {
		cp.sets = append(cp.sets, setClause{col: col, val: vals[i]})
	}
	return &cp
}

// Where sets the WHERE predicate.
func (b *UpdateBuilder) Where(e expr.Expression) *UpdateBuilder {
	cp := *b
	cp.where = e
	return &cp
}

// And appends an additional WHERE condition with AND semantics.
func (b *UpdateBuilder) And(e expr.Expression) *UpdateBuilder {
	return b.Where(expr.And(b.where, e))
}

// Returning specifies columns to return after update (PostgreSQL RETURNING clause).
func (b *UpdateBuilder) Returning(cols ...expr.SelectableColumn) *UpdateBuilder {
	cp := *b
	cp.returning = cols
	return &cp
}

// Limit sets a row limit on the UPDATE (MySQL / SQLite only).
func (b *UpdateBuilder) Limit(n int) *UpdateBuilder {
	cp := *b
	cp.limit = n
	return &cp
}

// Build renders the UPDATE statement.
func (b *UpdateBuilder) Build(d dialect.Dialect) (string, []any, error) {
	ctx, err := newBuildContext(d)
	if err != nil {
		return buildFailure("build_update", err)
	}
	if b == nil {
		return buildFailure("build_update", NewError(CodeBuildValidation, "build_update", "update builder is nil"))
	}
	if b.buildErr != nil {
		return buildFailure("build_update", b.buildErr)
	}
	if b.limit < 0 {
		return buildFailure("build_update", NewError(CodeBuildValidation, "build_update", "update limit must not be negative"))
	}
	var sb strings.Builder

	sb.WriteString("UPDATE ")
	table, err := quoteTableSource(ctx, b.table)
	if err != nil {
		return buildFailure("build_update", err)
	}
	sb.WriteString(table)
	sb.WriteString(" SET ")

	// Collect all SET clauses.
	allSets := append([]setClause(nil), b.sets...)

	if len(allSets) == 0 {
		return buildFailure("build_update", NewError(CodeBuildValidation, "build_update", "update contains no assignments"))
	}

	for i, s := range allSets {
		if i > 0 {
			sb.WriteString(", ")
		}
		column, err := ctx.Quote(s.col)
		if err != nil {
			return buildFailure("build_update", err)
		}
		sb.WriteString(column)
		sb.WriteString(" = ")
		sb.WriteString(ctx.Add(s.val))
	}

	where, err := buildWhere(ctx, b.where)
	if err != nil {
		return buildFailure("build_update", err)
	}
	sb.WriteString(where)

	if b.limit > 0 {
		if !d.SupportsLimitOnMutate() {
			return buildFailure("build_update", NewError(CodeUnsupportedFeature, "build_update", "update limits are not supported by this dialect"))
		}
		_, _ = fmt.Fprintf(&sb, " LIMIT %d", b.limit)
	}

	if len(b.returning) > 0 {
		if !d.SupportsReturning() {
			return buildFailure("build_update", NewError(CodeUnsupportedFeature, "build_update", "returning is not supported by this dialect"))
		}
		sb.WriteString(" RETURNING ")
		for i, c := range b.returning {
			if i > 0 {
				sb.WriteString(", ")
			}
			column, err := selectColSQL(ctx, c)
			if err != nil {
				return buildFailure("build_update", err)
			}
			sb.WriteString(column)
		}
	}

	return sb.String(), ctx.Args(), nil
}

// -------------------------------------------------------------------
// Struct reflection helper for building typed update payloads
// -------------------------------------------------------------------

// structSetsForUpdate extracts db-tagged fields for SET clauses.
// ALL nil pointer fields are skipped regardless of omitempty — in an update
// struct, nil always means "leave this column unchanged".
// Returns an error if row is nil, an invalid reflect value, or not a struct.
func structSetsForUpdate(row any) (cols []string, vals []any, err error) {
	if row == nil {
		return nil, nil, fmt.Errorf("structSetsForUpdate: nil input")
	}
	rv := reflect.ValueOf(row)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return nil, nil, fmt.Errorf("structSetsForUpdate: invalid (nil pointer) input")
	}
	if rv.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("structSetsForUpdate: expected struct, got %s", rv.Kind())
	}
	rt := rv.Type()
	seen := make(map[string]struct{})
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rv.Field(i)
		tag := field.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		if field.PkgPath != "" || !fv.CanInterface() {
			return nil, nil, fmt.Errorf("structSetsForUpdate: tagged field is not exported")
		}
		parts := strings.Split(tag, ",")
		colName := parts[0]
		if colName == "" {
			return nil, nil, fmt.Errorf("structSetsForUpdate: empty db tag")
		}
		for _, option := range parts[1:] {
			if option != "" && option != "omitempty" {
				return nil, nil, fmt.Errorf("structSetsForUpdate: unsupported db tag option")
			}
		}
		if _, ok := seen[colName]; ok {
			return nil, nil, fmt.Errorf("structSetsForUpdate: duplicate db tag")
		}
		seen[colName] = struct{}{}
		if fv.Kind() == reflect.Ptr && fv.IsNil() {
			continue
		}
		if fv.Kind() == reflect.Ptr {
			fv = fv.Elem()
		}
		cols = append(cols, colName)
		vals = append(vals, fv.Interface())
	}
	return
}

// StructSets extracts non-nil db-tagged fields from a struct as col=val pairs.
// Useful when you want to inspect the assignments before building a query.
func StructSets(row any) map[string]any {
	rv := reflect.ValueOf(row)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	rt := rv.Type()
	result := make(map[string]any)

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rv.Field(i)
		tag := field.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		parts := strings.SplitN(tag, ",", 2)
		colName := parts[0]
		if fv.Kind() == reflect.Ptr && fv.IsNil() {
			continue
		}
		if fv.Kind() == reflect.Ptr {
			fv = fv.Elem()
		}
		result[colName] = fv.Interface()
	}
	return result
}
