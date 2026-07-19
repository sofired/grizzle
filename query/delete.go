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
	where     expr.Expression
	returning []expr.SelectableColumn
	limit     int // 0 = no limit; MySQL/SQLite only
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

// Limit sets a row limit on the DELETE (MySQL / SQLite only).
// Build returns ErrUnsupportedFeature when the selected dialect does not
// support the clause.
func (b *DeleteBuilder) Limit(n int) *DeleteBuilder {
	cp := *b
	cp.limit = n
	return &cp
}

// Build renders the DELETE statement.
func (b *DeleteBuilder) Build(d dialect.Dialect) (string, []any, error) {
	ctx, err := newBuildContext(d)
	if err != nil {
		return buildFailure("build_delete", err)
	}
	if b == nil {
		return buildFailure("build_delete", NewError(CodeBuildValidation, "build_delete", "delete builder is nil"))
	}
	if b.limit < 0 {
		return buildFailure("build_delete", NewError(CodeBuildValidation, "build_delete", "delete limit must not be negative"))
	}
	var sb strings.Builder

	sb.WriteString("DELETE FROM ")
	table, err := quoteTableSource(ctx, b.table)
	if err != nil {
		return buildFailure("build_delete", err)
	}
	sb.WriteString(table)

	where, err := buildWhere(ctx, b.where)
	if err != nil {
		return buildFailure("build_delete", err)
	}
	sb.WriteString(where)

	if b.limit > 0 {
		if !d.SupportsLimitOnMutate() {
			return buildFailure("build_delete", NewError(CodeUnsupportedFeature, "build_delete", "delete limit is not supported by this dialect"))
		}
		_, _ = fmt.Fprintf(&sb, " LIMIT %d", b.limit)
	}

	if len(b.returning) > 0 {
		if !d.SupportsReturning() {
			return buildFailure("build_delete", NewError(CodeUnsupportedFeature, "build_delete", "returning is not supported by this dialect"))
		}
		sb.WriteString(" RETURNING ")
		for i, c := range b.returning {
			if i > 0 {
				sb.WriteString(", ")
			}
			column, err := selectColSQL(ctx, c)
			if err != nil {
				return buildFailure("build_delete", err)
			}
			sb.WriteString(column)
		}
	}

	return sb.String(), ctx.Args(), nil
}
