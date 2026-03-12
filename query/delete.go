package query

import (
	"fmt"
	"strings"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// DeleteBuilder constructs a DELETE query.
type DeleteBuilder struct {
	table     TableSource
	using     []TableSource
	where     expr.Expression
	returning []expr.SelectableColumn
	limit     int // 0 = no limit; MySQL/SQLite only
}

// DeleteFrom starts a DELETE FROM <table> query.
func DeleteFrom(t TableSource) *DeleteBuilder {
	return &DeleteBuilder{table: t}
}

// Using adds one or more tables to the USING clause of the DELETE statement
// (PostgreSQL DELETE … USING syntax). This allows WHERE conditions to reference
// columns from the additional tables.
//
// On dialects that do not support DELETE … USING (MySQL, SQLite), the USING
// tables are silently ignored.
//
//	query.DeleteFrom(SessionsT).
//	    Using(UsersT).
//	    Where(expr.And(
//	        SessionsT.UserID.EQCol(UsersT.ID),
//	        UsersT.DeletedAt.IsNotNull(),
//	    ))
func (b *DeleteBuilder) Using(tables ...TableSource) *DeleteBuilder {
	cp := *b
	cp.using = append(append([]TableSource(nil), cp.using...), tables...)
	return &cp
}

// Where sets the WHERE predicate.
func (b *DeleteBuilder) Where(e expr.Expression) *DeleteBuilder {
	cp := *b
	cp.where = e
	return &cp
}

// And appends an additional WHERE condition with AND semantics.
func (b *DeleteBuilder) And(e expr.Expression) *DeleteBuilder {
	return b.Where(expr.And(b.where, e))
}

// Returning specifies columns to return after delete (PostgreSQL RETURNING clause).
func (b *DeleteBuilder) Returning(cols ...expr.SelectableColumn) *DeleteBuilder {
	cp := *b
	cp.returning = cols
	return &cp
}

// Limit sets a row limit on the DELETE (MySQL / SQLite only).
// PostgreSQL does not support LIMIT on DELETE; this is silently ignored for
// dialects that do not support it.
func (b *DeleteBuilder) Limit(n int) *DeleteBuilder {
	cp := *b
	cp.limit = n
	return &cp
}

// Build renders the DELETE statement.
func (b *DeleteBuilder) Build(d dialect.Dialect) (string, []any) {
	ctx := expr.NewBuildContext(d)
	var sb strings.Builder

	sb.WriteString("DELETE FROM ")
	sb.WriteString(ctx.Quote(b.table.GrizTableName()))

	if len(b.using) > 0 && d.Name() == "postgres" {
		sb.WriteString(" USING ")
		for i, t := range b.using {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(ctx.Quote(t.GrizTableName()))
		}
	}

	sb.WriteString(buildWhere(ctx, b.where))

	if b.limit > 0 && d.Name() != "postgres" {
		fmt.Fprintf(&sb, " LIMIT %d", b.limit)
	}

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
