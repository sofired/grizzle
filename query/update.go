package query

import (
	"reflect"
	"strings"

	"github.com/grizzle-orm/grizzle/dialect"
	"github.com/grizzle-orm/grizzle/expr"
)

// UpdateBuilder constructs an UPDATE query.
type UpdateBuilder struct {
	table     TableSource
	sets      []setClause // explicit col = val pairs
	setStruct any         // alternative: set via struct reflection
	where     expr.Expression
	returning []expr.SelectableColumn
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
	cp.setStruct = row
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

// Build renders the UPDATE statement.
func (b *UpdateBuilder) Build(d dialect.Dialect) (string, []any) {
	ctx := expr.NewBuildContext(d)
	var sb strings.Builder

	sb.WriteString("UPDATE ")
	sb.WriteString(ctx.Quote(b.table.GRizTableName()))
	sb.WriteString(" SET ")

	// Collect all SET clauses: explicit sets + struct sets
	allSets := append([]setClause(nil), b.sets...)
	if b.setStruct != nil {
		cols, vals := structSetsForUpdate(b.setStruct)
		for i, c := range cols {
			allSets = append(allSets, setClause{col: c, val: vals[i]})
		}
	}

	for i, s := range allSets {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(ctx.Quote(s.col))
		sb.WriteString(" = ")
		sb.WriteString(ctx.Add(s.val))
	}

	sb.WriteString(buildWhere(ctx, b.where))

	if len(b.returning) > 0 && d.SupportsReturning() {
		sb.WriteString(" RETURNING ")
		for i, c := range b.returning {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(selectColSQL(ctx, c))
		}
	}

	return sb.String(), ctx.Args()
}

// -------------------------------------------------------------------
// Struct reflection helper for building typed update payloads
// -------------------------------------------------------------------

// structSetsForUpdate extracts db-tagged fields for SET clauses.
// ALL nil pointer fields are skipped regardless of omitempty — in an update
// struct, nil always means "leave this column unchanged".
func structSetsForUpdate(row any) (cols []string, vals []any) {
	rv := reflect.ValueOf(row)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rv.Field(i)
		tag := field.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		colName := strings.SplitN(tag, ",", 2)[0]
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
