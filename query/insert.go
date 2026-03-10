package query

import (
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
	ignoreConflict bool // emit INSERT IGNORE / INSERT OR IGNORE
}

// upsertClause holds the ON CONFLICT … DO … specification.
type upsertClause struct {
	// conflict target — exactly one of these is set
	conflictCols       []string // ON CONFLICT (col1, col2)
	conflictConstraint string   // ON CONFLICT ON CONSTRAINT name

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
	cols, vals := structToColVals(row)
	cp := *b
	if len(cp.colNames) == 0 {
		cp.colNames = cols
	}
	cp.rows = append(append([][]any(nil), cp.rows...), vals)
	return &cp
}

// ValueSlice accepts a slice of structs and adds a row for each element.
func (b *InsertBuilder) ValueSlice(rows any) *InsertBuilder {
	rv := reflect.ValueOf(rows)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	cp := *b
	for i := 0; i < rv.Len(); i++ {
		cols, vals := structToColVals(rv.Index(i).Interface())
		if len(cp.colNames) == 0 {
			cp.colNames = cols
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
func (b *InsertBuilder) DoUpdateSetStruct(row any) *InsertBuilder {
	cols, vals := structSetsForUpdate(row)
	cp := *b
	u := b.upsertCopy()
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
//   - SQLite: emits INSERT OR IGNORE INTO …
//   - PostgreSQL: no direct equivalent; this flag is silently ignored.
//     Use OnConflict(cols).DoNothing() for PostgreSQL instead.
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
func (b *InsertBuilder) Build(d dialect.Dialect) (string, []any) {
	ctx := expr.NewBuildContext(d)
	var sb strings.Builder

	// Choose INSERT keyword based on ignore flag and dialect support.
	if b.ignoreConflict {
		if clause := d.InsertIgnoreClause(); clause != "" {
			sb.WriteString(clause)
			sb.WriteString(" INTO ")
		} else {
			sb.WriteString("INSERT INTO ")
		}
	} else {
		sb.WriteString("INSERT INTO ")
	}
	sb.WriteString(ctx.Quote(b.table.GRizTableName()))

	// Column list
	sb.WriteString(" (")
	for i, c := range b.colNames {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(ctx.Quote(c))
	}
	sb.WriteString(")")

	// VALUES
	sb.WriteString(" VALUES ")
	for ri, row := range b.rows {
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

	// Upsert clause — dialect-specific
	if b.upsert != nil {
		switch d.UpsertStyle() {
		case dialect.UpsertOnConflict:
			buildOnConflict(&sb, ctx, b.upsert)
		case dialect.UpsertDuplicateKey:
			buildOnDuplicateKey(&sb, ctx, b.upsert)
		// UpsertNone: silently drop the clause
		}
	}

	// RETURNING — only for dialects that support it
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
// Upsert rendering helpers
// -------------------------------------------------------------------

// buildOnConflict emits PostgreSQL / SQLite style:
//   ON CONFLICT (cols) DO NOTHING | DO UPDATE SET …
func buildOnConflict(sb *strings.Builder, ctx *expr.BuildContext, u *upsertClause) {
	sb.WriteString(" ON CONFLICT")

	switch {
	case len(u.conflictCols) > 0:
		sb.WriteString(" (")
		for i, c := range u.conflictCols {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(ctx.Quote(c))
		}
		sb.WriteString(")")
	case u.conflictConstraint != "":
		sb.WriteString(" ON CONSTRAINT ")
		sb.WriteString(ctx.Quote(u.conflictConstraint))
	}

	if u.doNothing {
		sb.WriteString(" DO NOTHING")
	} else {
		sb.WriteString(" DO UPDATE SET ")
		first := true
		for _, s := range u.sets {
			if !first {
				sb.WriteString(", ")
			}
			sb.WriteString(ctx.Quote(s.col))
			sb.WriteString(" = ")
			sb.WriteString(ctx.Add(s.val))
			first = false
		}
		for _, col := range u.excluded {
			if !first {
				sb.WriteString(", ")
			}
			sb.WriteString(ctx.Quote(col))
			sb.WriteString(" = EXCLUDED.")
			sb.WriteString(ctx.Quote(col))
			first = false
		}
	}
}

// buildOnDuplicateKey emits MySQL style:
//   ON DUPLICATE KEY UPDATE col = VALUES(col), col = val
// Note: MySQL ignores the conflict-target columns — the conflict is determined
// by the table's PRIMARY KEY and UNIQUE indexes automatically.
func buildOnDuplicateKey(sb *strings.Builder, ctx *expr.BuildContext, u *upsertClause) {
	if u.doNothing {
		// MySQL has no DO NOTHING equivalent in ON DUPLICATE KEY UPDATE syntax.
		// Callers should use IgnoreConflicts() to get INSERT IGNORE INTO instead.
		// Emit nothing so the statement remains valid (just a regular INSERT).
		return
	}
	sb.WriteString(" ON DUPLICATE KEY UPDATE ")
	first := true
	for _, s := range u.sets {
		if !first {
			sb.WriteString(", ")
		}
		sb.WriteString(ctx.Quote(s.col))
		sb.WriteString(" = ")
		sb.WriteString(ctx.Add(s.val))
		first = false
	}
	for _, col := range u.excluded {
		if !first {
			sb.WriteString(", ")
		}
		sb.WriteString(ctx.Quote(col))
		sb.WriteString(" = VALUES(")
		sb.WriteString(ctx.Quote(col))
		sb.WriteString(")")
		first = false
	}
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
func structToColVals(row any) (cols []string, vals []any) {
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

		parts := strings.SplitN(tag, ",", 2)
		colName := parts[0]
		omitempty := len(parts) > 1 && strings.Contains(parts[1], "omitempty")

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
	return
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
