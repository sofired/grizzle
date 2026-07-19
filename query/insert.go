package query

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// InsertBuilder constructs an INSERT query.
type InsertBuilder struct {
	table          TableSource
	colNames       []string
	rows           [][]any
	returning      []expr.SelectableColumn
	upsert         *upsertClause
	ignoreConflict bool // emit dialect-specific no-op conflict handling
	buildErr       error
}

// upsertClause holds the ON CONFLICT … DO … specification.
type upsertClause struct {
	// conflict target — exactly one of these is set
	conflictCols       []string // ON CONFLICT (col1, col2)
	conflictConstraint string   // ON CONFLICT ON CONSTRAINT name
	conflictTargetSet  bool

	// conflict action — exactly one is set
	doNothing bool        // DO NOTHING
	sets      []setClause // DO UPDATE SET col = val  (explicit)
	excluded  []string    // DO UPDATE SET col = EXCLUDED.col
}

// InsertInto starts an INSERT INTO <table> query.
func InsertInto(t TableSource) *InsertBuilder {
	return &InsertBuilder{table: t}
}

// Values accepts a struct (or pointer to struct) and extracts column names
// and values from fields tagged with `db:"col_name"`.
// Fields with a zero value AND tagged `db:"...,omitempty"` are skipped.
// Fields tagged `db:"-"` are always skipped.
//
// For inserting multiple rows, call Values repeatedly or use ValueSlice.
func (b *InsertBuilder) Values(row any) *InsertBuilder {
	cols, vals, err := structToColVals(row)
	cp := *b
	if err != nil {
		if cp.buildErr == nil {
			cp.buildErr = err
		}
		return &cp
	}
	if len(cp.colNames) == 0 {
		cp.colNames = cols
	} else if !equalStrings(cp.colNames, cols) {
		if cp.buildErr == nil {
			cp.buildErr = fmt.Errorf("insert rows have inconsistent columns")
		}
		return &cp
	}
	cp.rows = append(append([][]any(nil), cp.rows...), vals)
	return &cp
}

// ValueSlice accepts a slice of structs and adds a row for each element.
func (b *InsertBuilder) ValueSlice(rows any) *InsertBuilder {
	cp := *b
	if rows == nil {
		cp.buildErr = fmt.Errorf("insert row slice is nil")
		return &cp
	}
	rv := reflect.ValueOf(rows)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			cp.buildErr = fmt.Errorf("insert row slice is nil")
			return &cp
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		cp.buildErr = fmt.Errorf("insert rows must be a slice or array")
		return &cp
	}
	for i := 0; i < rv.Len(); i++ {
		cols, vals, err := structToColVals(rv.Index(i).Interface())
		if err != nil {
			if cp.buildErr == nil {
				cp.buildErr = err
			}
			return &cp
		}
		if len(cp.colNames) == 0 {
			cp.colNames = cols
		} else if !equalStrings(cp.colNames, cols) {
			cp.buildErr = fmt.Errorf("insert rows have inconsistent columns")
			return &cp
		}
		cp.rows = append(cp.rows, vals)
	}
	return &cp
}

// OnConflict sets the conflict target to one or more column names.
// Must be followed by DoNothing(), DoUpdateSet(), DoUpdateSetExcluded(),
// or DoUpdateSetStruct() to complete the upsert clause.
//
//	query.InsertInto(UsersT).Values(row).
//	    OnConflict("realm_id", "username").DoUpdateSetExcluded("email", "enabled")
func (b *InsertBuilder) OnConflict(cols ...string) *InsertBuilder {
	cp := *b
	u := b.upsertCopy()
	u.conflictCols = cols
	u.conflictConstraint = ""
	u.conflictTargetSet = true
	cp.upsert = u
	return &cp
}

// OnConflictConstraint sets the conflict target to a named constraint.
//
//	query.InsertInto(UsersT).Values(row).
//	    OnConflictConstraint("users_realm_username_idx").DoNothing()
func (b *InsertBuilder) OnConflictConstraint(name string) *InsertBuilder {
	cp := *b
	u := b.upsertCopy()
	u.conflictConstraint = name
	u.conflictCols = nil
	u.conflictTargetSet = true
	cp.upsert = u
	return &cp
}

// DoNothing sets the conflict action to DO NOTHING.
func (b *InsertBuilder) DoNothing() *InsertBuilder {
	cp := *b
	u := b.upsertCopy()
	u.doNothing = true
	u.sets = nil
	u.excluded = nil
	cp.upsert = u
	return &cp
}

// DoUpdateSet adds an explicit col = val assignment to the DO UPDATE SET clause.
// Call multiple times to set multiple columns.
//
//	.OnConflict("email").DoUpdateSet("enabled", true).DoUpdateSet("username", "alice")
func (b *InsertBuilder) DoUpdateSet(col string, val any) *InsertBuilder {
	cp := *b
	u := b.upsertCopy()
	u.doNothing = false
	u.sets = append(append([]setClause(nil), u.sets...), setClause{col: col, val: val})
	cp.upsert = u
	return &cp
}

// DoUpdateSetExcluded adds SET col = EXCLUDED.col for each named column.
// This is the most common upsert pattern — overwrite with the values that
// were proposed for insertion.
//
//	.OnConflict("realm_id", "username").DoUpdateSetExcluded("email", "enabled")
func (b *InsertBuilder) DoUpdateSetExcluded(cols ...string) *InsertBuilder {
	cp := *b
	u := b.upsertCopy()
	u.doNothing = false
	u.excluded = append(append([]string(nil), u.excluded...), cols...)
	cp.upsert = u
	return &cp
}

// DoUpdateSetStruct extracts non-nil db-tagged fields and adds them to the
// DO UPDATE SET clause as explicit col = val assignments. Nil pointer fields
// are skipped (same semantics as UpdateBuilder.SetStruct).
// Invalid inputs are retained as build-validation errors and returned by Build.
func (b *InsertBuilder) DoUpdateSetStruct(row any) *InsertBuilder {
	cols, vals, err := structSetsForUpdate(row)
	cp := *b
	u := b.upsertCopy()
	if err != nil {
		if cp.buildErr == nil {
			cp.buildErr = err
		}
		return &cp
	}
	u.doNothing = false
	for i, c := range cols {
		u.sets = append(u.sets, setClause{col: c, val: vals[i]})
	}
	cp.upsert = u
	return &cp
}

// upsertCopy returns a shallow copy of the upsert clause, allocating a new one if nil.
func (b *InsertBuilder) upsertCopy() *upsertClause {
	if b.upsert == nil {
		return &upsertClause{}
	}
	cp := *b.upsert
	return &cp
}

// IgnoreConflicts marks the insert to silently skip rows that violate a
// unique or primary key constraint.
//
// Dialect behaviour:
//   - MySQL:  emits INSERT IGNORE INTO …
//   - PostgreSQL / SQLite: emits ON CONFLICT DO NOTHING
func (b *InsertBuilder) IgnoreConflicts() *InsertBuilder {
	cp := *b
	cp.ignoreConflict = true
	return &cp
}

// Returning specifies columns to return after insert (PostgreSQL RETURNING clause).
func (b *InsertBuilder) Returning(cols ...expr.SelectableColumn) *InsertBuilder {
	cp := *b
	cp.returning = cols
	return &cp
}

// Build renders the INSERT statement.
func (b *InsertBuilder) Build(d dialect.Dialect) (string, []any, error) {
	ctx, err := newBuildContext(d)
	if err != nil {
		return buildFailure("build_insert", err)
	}
	if b == nil {
		return buildFailure("build_insert", NewError(CodeBuildValidation, "build_insert", "insert builder is nil"))
	}
	if b.buildErr != nil {
		return buildFailure("build_insert", b.buildErr)
	}
	if len(b.colNames) == 0 || len(b.rows) == 0 {
		return buildFailure("build_insert", NewError(CodeBuildValidation, "build_insert", "insert contains no values"))
	}
	if b.ignoreConflict && b.upsert != nil {
		return buildFailure("build_insert", NewError(CodeBuildValidation, "build_insert", "ignore conflicts cannot be combined with an upsert clause"))
	}
	var sb strings.Builder

	// Choose INSERT keyword based on ignore flag and dialect support.
	if b.ignoreConflict {
		if !d.SupportsIgnoreConflicts() {
			return buildFailure("build_insert", NewError(CodeUnsupportedFeature, "build_insert", "ignore conflicts is not supported by this dialect"))
		}
		if d.UpsertStyle() == dialect.UpsertOnConflict {
			sb.WriteString("INSERT INTO ")
		} else if clause := d.InsertIgnoreClause(); clause != "" {
			sb.WriteString(clause)
			sb.WriteString(" INTO ")
		} else {
			return buildFailure("build_insert", NewError(CodeUnsupportedFeature, "build_insert", "ignore conflicts is not supported by this dialect"))
		}
	} else {
		sb.WriteString("INSERT INTO ")
	}
	table, err := quoteTableSource(ctx, b.table)
	if err != nil {
		return buildFailure("build_insert", err)
	}
	sb.WriteString(table)

	// Column list
	sb.WriteString(" (")
	for i, c := range b.colNames {
		if i > 0 {
			sb.WriteString(", ")
		}
		column, err := ctx.Quote(c)
		if err != nil {
			return buildFailure("build_insert", err)
		}
		sb.WriteString(column)
	}
	sb.WriteString(")")

	// VALUES
	sb.WriteString(" VALUES ")
	for ri, row := range b.rows {
		if len(row) != len(b.colNames) {
			return buildFailure("build_insert", NewError(CodeBuildValidation, "build_insert", "insert row does not match column count"))
		}
		if ri > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("(")
		for vi, val := range row {
			if vi > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(ctx.Add(val))
		}
		sb.WriteString(")")
	}

	if b.ignoreConflict && d.UpsertStyle() == dialect.UpsertOnConflict {
		if err := buildOnConflict(&sb, ctx, &upsertClause{doNothing: true}); err != nil {
			return buildFailure("build_insert", err)
		}
	}

	// Upsert clause — dialect-specific
	if b.upsert != nil {
		switch d.UpsertStyle() {
		case dialect.UpsertOnConflict:
			if err := buildOnConflict(&sb, ctx, b.upsert); err != nil {
				return buildFailure("build_insert", err)
			}
		case dialect.UpsertDuplicateKey:
			if err := buildOnDuplicateKey(&sb, ctx, b.upsert); err != nil {
				return buildFailure("build_insert", err)
			}
		default:
			return buildFailure("build_insert", NewError(CodeUnsupportedFeature, "build_insert", "upsert is not supported by this dialect"))
		}
	}

	// RETURNING — only for dialects that support it
	if len(b.returning) > 0 {
		if !d.SupportsReturning() {
			return buildFailure("build_insert", NewError(CodeUnsupportedFeature, "build_insert", "returning is not supported by this dialect"))
		}
		sb.WriteString(" RETURNING ")
		for i, c := range b.returning {
			if i > 0 {
				sb.WriteString(", ")
			}
			column, err := selectColSQL(ctx, c)
			if err != nil {
				return buildFailure("build_insert", err)
			}
			sb.WriteString(column)
		}
	}

	return sb.String(), ctx.Args(), nil
}

// -------------------------------------------------------------------
// Upsert rendering helpers
// -------------------------------------------------------------------

// buildOnConflict emits PostgreSQL / SQLite style:
//
//	ON CONFLICT (cols) DO NOTHING | DO UPDATE SET …
func buildOnConflict(sb *strings.Builder, ctx *expr.BuildContext, u *upsertClause) error {
	if u.conflictTargetSet && len(u.conflictCols) == 0 && u.conflictConstraint == "" {
		return NewError(CodeBuildValidation, "build_insert", "conflict target is empty")
	}
	if !u.doNothing && !u.conflictTargetSet {
		return NewError(CodeBuildValidation, "build_insert", "upsert update requires a conflict target")
	}
	sb.WriteString(" ON CONFLICT")

	switch {
	case len(u.conflictCols) > 0:
		sb.WriteString(" (")
		for i, c := range u.conflictCols {
			if i > 0 {
				sb.WriteString(", ")
			}
			column, err := ctx.Quote(c)
			if err != nil {
				return err
			}
			sb.WriteString(column)
		}
		sb.WriteString(")")
	case u.conflictConstraint != "":
		sb.WriteString(" ON CONSTRAINT ")
		constraint, err := ctx.Quote(u.conflictConstraint)
		if err != nil {
			return err
		}
		sb.WriteString(constraint)
	}

	if u.doNothing {
		sb.WriteString(" DO NOTHING")
	} else if len(u.sets) == 0 && len(u.excluded) == 0 {
		return NewError(CodeBuildValidation, "build_insert", "upsert update contains no assignments")
	} else {
		sb.WriteString(" DO UPDATE SET ")
		first := true
		for _, s := range u.sets {
			if !first {
				sb.WriteString(", ")
			}
			column, err := ctx.Quote(s.col)
			if err != nil {
				return err
			}
			sb.WriteString(column)
			sb.WriteString(" = ")
			sb.WriteString(ctx.Add(s.val))
			first = false
		}
		for _, col := range u.excluded {
			if !first {
				sb.WriteString(", ")
			}
			column, err := ctx.Quote(col)
			if err != nil {
				return err
			}
			sb.WriteString(column)
			sb.WriteString(" = EXCLUDED.")
			sb.WriteString(column)
			first = false
		}
	}
	return nil
}

// buildOnDuplicateKey emits MySQL style:
//
//	ON DUPLICATE KEY UPDATE col = VALUES(col), col = val
//
// Note: MySQL ignores the conflict-target columns — the conflict is determined
// by the table's PRIMARY KEY and UNIQUE indexes automatically.
func buildOnDuplicateKey(sb *strings.Builder, ctx *expr.BuildContext, u *upsertClause) error {
	if u.conflictTargetSet {
		return NewError(CodeUnsupportedFeature, "build_insert", "conflict targets are not supported by this dialect")
	}
	if u.doNothing {
		return NewError(CodeUnsupportedFeature, "build_insert", "do nothing upserts are not supported by this dialect")
	}
	if len(u.sets) == 0 && len(u.excluded) == 0 {
		return NewError(CodeBuildValidation, "build_insert", "upsert update contains no assignments")
	}
	sb.WriteString(" ON DUPLICATE KEY UPDATE ")
	first := true
	for _, s := range u.sets {
		if !first {
			sb.WriteString(", ")
		}
		column, err := ctx.Quote(s.col)
		if err != nil {
			return err
		}
		sb.WriteString(column)
		sb.WriteString(" = ")
		sb.WriteString(ctx.Add(s.val))
		first = false
	}
	for _, col := range u.excluded {
		if !first {
			sb.WriteString(", ")
		}
		column, err := ctx.Quote(col)
		if err != nil {
			return err
		}
		sb.WriteString(column)
		sb.WriteString(" = VALUES(")
		sb.WriteString(column)
		sb.WriteString(")")
		first = false
	}
	return nil
}

// -------------------------------------------------------------------
// Struct → (columns, values) reflection helper
// -------------------------------------------------------------------

// structToColVals extracts db-tagged field names and their values from a struct.
//
// Omitempty rules (mirrors encoding/json behaviour):
//   - Pointer fields: skip if nil
//   - Map/slice fields: skip if nil or len == 0
//   - Other fields: always included
func structToColVals(row any) (cols []string, vals []any, err error) {
	if row == nil {
		return nil, nil, fmt.Errorf("insert row is nil")
	}
	rv := reflect.ValueOf(row)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil, nil, fmt.Errorf("insert row is nil")
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, nil, fmt.Errorf("insert row must be a struct")
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
			return nil, nil, fmt.Errorf("insert row contains a tagged unexported field")
		}

		parts := strings.Split(tag, ",")
		colName := parts[0]
		if colName == "" {
			return nil, nil, fmt.Errorf("insert row contains an empty db tag")
		}
		if _, ok := seen[colName]; ok {
			return nil, nil, fmt.Errorf("insert row contains duplicate db tags")
		}
		seen[colName] = struct{}{}
		omitempty := false
		for _, option := range parts[1:] {
			switch option {
			case "", "omitempty":
				omitempty = omitempty || option == "omitempty"
			default:
				return nil, nil, fmt.Errorf("insert row contains an unsupported db tag option")
			}
		}

		if omitempty && isEmptyValue(fv) {
			continue
		}

		// Nil pointer without omitempty → send explicit NULL.
		if fv.Kind() == reflect.Ptr && fv.IsNil() {
			cols = append(cols, colName)
			vals = append(vals, nil)
			continue
		}

		// Dereference pointer.
		if fv.Kind() == reflect.Ptr {
			fv = fv.Elem()
		}

		cols = append(cols, colName)
		vals = append(vals, fv.Interface())
	}
	return cols, vals, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// isEmptyValue returns true for values that omitempty should treat as absent:
// nil pointers, nil/empty maps, nil/empty slices, and zero-length arrays.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Map, reflect.Slice:
		return v.IsNil() || v.Len() == 0
	case reflect.Array:
		return v.Len() == 0
	default:
		return false
	}
}
