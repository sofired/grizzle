package query

import (
	"strings"

	"github.com/grizzle-orm/grizzle/dialect"
	"github.com/grizzle-orm/grizzle/expr"
)

// DeleteBuilder constructs a DELETE query.
type DeleteBuilder struct {
	table     TableSource
	where     expr.Expression
	returning []expr.SelectableColumn
}

// DeleteFrom starts a DELETE FROM <table> query.
func DeleteFrom(t TableSource) *DeleteBuilder {
	return &DeleteBuilder{table: t}
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

// Build renders the DELETE statement.
func (b *DeleteBuilder) Build(d dialect.Dialect) (string, []any) {
	ctx := expr.NewBuildContext(d)
	var sb strings.Builder

	sb.WriteString("DELETE FROM ")
	sb.WriteString(ctx.Quote(b.table.GRizTableName()))

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
