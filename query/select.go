package query

import (
	"fmt"
	"strings"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// SelectBuilder constructs a SELECT query.
// Each method returns a modified copy, so builders can be shared and
// extended without mutating the original.
type SelectBuilder struct {
	ctes    []cteClause             // optional WITH clauses (prepended as CTEs)
	cols    []expr.SelectableColumn // nil = SELECT *
	from    TableSource
	joins   []joinClause
	where   expr.Expression
	orderBy []expr.OrderExpr
	groupBy []expr.SelectableColumn
	having  expr.Expression
	limit   int // 0 = no limit
	offset  int // 0 = no offset
}

// cteClause holds a single WITH name AS (SELECT ...) entry.
type cteClause struct {
	name string
	sub  *SelectBuilder
}

// Select starts a SELECT query specifying the columns to return.
// Pass no columns to SELECT *.
//
//	query.Select(UsersT.ID, UsersT.Name)
//	query.Select() // SELECT *
func Select(cols ...expr.SelectableColumn) *SelectBuilder {
	return &SelectBuilder{cols: cols}
}

// With adds a Common Table Expression (CTE) to the query.
// The CTE is rendered as WITH name AS (sub) before the SELECT.
// Multiple CTEs are accumulated in order and rendered as WITH a AS (...), b AS (...).
//
// Example:
//
//	recent := query.Select(PostsT.ID, PostsT.AuthorID).
//	    From(PostsT).
//	    Where(PostsT.CreatedAt.GTE(cutoff))
//
//	query.Select(expr.Raw("recent.id")).
//	    With("recent", recent).
//	    From(query.CTERef("recent"))
func (b *SelectBuilder) With(name string, sub *SelectBuilder) *SelectBuilder {
	cp := *b
	cp.ctes = append(append([]cteClause(nil), cp.ctes...), cteClause{name: name, sub: sub})
	return &cp
}

// CTERef returns a TableSource that references a named CTE defined with .With().
// Use it in From() or Join() to reference the CTE by name.
func CTERef(name string) TableSource { return cteTableSource{name: name} }

type cteTableSource struct{ name string }

func (c cteTableSource) GRizTableName() string  { return c.name }
func (c cteTableSource) GRizTableAlias() string { return c.name }

// From sets the primary table.
func (b *SelectBuilder) From(t TableSource) *SelectBuilder {
	cp := *b
	cp.from = t
	return &cp
}

// Where sets the WHERE predicate. Call And/Or from the expr package to
// combine multiple conditions.
func (b *SelectBuilder) Where(e expr.Expression) *SelectBuilder {
	cp := *b
	cp.where = e
	return &cp
}

// And appends an additional condition with AND semantics.
// Equivalent to .Where(expr.And(existing, e)).
func (b *SelectBuilder) And(e expr.Expression) *SelectBuilder {
	return b.Where(expr.And(b.where, e))
}

// LeftJoin adds a LEFT JOIN clause.
func (b *SelectBuilder) LeftJoin(t TableSource, on expr.Expression) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinLeft, table: t, on: on})
	return &cp
}

// InnerJoin adds an INNER JOIN clause.
func (b *SelectBuilder) InnerJoin(t TableSource, on expr.Expression) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinInner, table: t, on: on})
	return &cp
}

// RightJoin adds a RIGHT JOIN clause.
func (b *SelectBuilder) RightJoin(t TableSource, on expr.Expression) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinRight, table: t, on: on})
	return &cp
}

// FullJoin adds a FULL JOIN clause.
func (b *SelectBuilder) FullJoin(t TableSource, on expr.Expression) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinFull, table: t, on: on})
	return &cp
}

// JoinRel adds a LEFT JOIN using a RelationDef. This is the idiomatic way to
// join tables when the ON condition is already encoded in the relation definition.
//
//	query.Select(UsersT.ID, RealmsT.Name).
//	    From(UsersT).
//	    JoinRel(UserRealm)
func (b *SelectBuilder) JoinRel(rel RelationDef) *SelectBuilder {
	return b.LeftJoin(rel.Table, rel.On)
}

// InnerJoinRel adds an INNER JOIN using a RelationDef.
//
//	query.Select(UsersT.ID, RealmsT.Name).
//	    From(UsersT).
//	    InnerJoinRel(UserRealm)
func (b *SelectBuilder) InnerJoinRel(rel RelationDef) *SelectBuilder {
	return b.InnerJoin(rel.Table, rel.On)
}

// OrderBy sets the ORDER BY clause.
func (b *SelectBuilder) OrderBy(exprs ...expr.OrderExpr) *SelectBuilder {
	cp := *b
	cp.orderBy = exprs
	return &cp
}

// GroupBy sets the GROUP BY clause.
func (b *SelectBuilder) GroupBy(cols ...expr.SelectableColumn) *SelectBuilder {
	cp := *b
	cp.groupBy = cols
	return &cp
}

// Having sets the HAVING clause (requires GroupBy).
func (b *SelectBuilder) Having(e expr.Expression) *SelectBuilder {
	cp := *b
	cp.having = e
	return &cp
}

// Limit sets the maximum number of rows to return. 0 means no limit.
func (b *SelectBuilder) Limit(n int) *SelectBuilder {
	cp := *b
	cp.limit = n
	return &cp
}

// Offset sets the number of rows to skip. 0 means no offset.
func (b *SelectBuilder) Offset(n int) *SelectBuilder {
	cp := *b
	cp.offset = n
	return &cp
}

// Build renders the query to a SQL string and bound arg slice.
func (b *SelectBuilder) Build(d dialect.Dialect) (string, []any) {
	ctx := expr.NewBuildContext(d)
	return b.buildWith(ctx), ctx.Args()
}

// buildWith renders the SELECT statement into an existing BuildContext.
// This is called by Build and by subquery expressions to share parameter numbering.
func (b *SelectBuilder) buildWith(ctx *expr.BuildContext) string {
	var sb strings.Builder

	// WITH (CTEs)
	if len(b.ctes) > 0 {
		sb.WriteString("WITH ")
		for i, cte := range b.ctes {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(ctx.Quote(cte.name))
			sb.WriteString(" AS (")
			sb.WriteString(cte.sub.buildWith(ctx))
			sb.WriteString(")")
		}
		sb.WriteString(" ")
	}

	// SELECT
	sb.WriteString("SELECT ")
	if len(b.cols) == 0 {
		sb.WriteString("*")
	} else {
		for i, c := range b.cols {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(selectColSQL(ctx, c))
		}
	}

	// FROM
	if b.from != nil {
		sb.WriteString(" FROM ")
		if sq, ok := b.from.(*SubquerySource); ok {
			// Subquery: (SELECT ...) AS alias — render into the same context.
			sb.WriteString("(")
			sb.WriteString(sq.sub.buildWith(ctx))
			sb.WriteString(") AS ")
			sb.WriteString(ctx.Quote(sq.alias))
		} else {
			sb.WriteString(ctx.Quote(b.from.GRizTableName()))
			if b.from.GRizTableAlias() != b.from.GRizTableName() {
				sb.WriteString(" AS ")
				sb.WriteString(ctx.Quote(b.from.GRizTableAlias()))
			}
		}
	}

	// JOINs
	for _, j := range b.joins {
		sb.WriteString(" ")
		sb.WriteString(string(j.kind))
		sb.WriteString(" ")
		sb.WriteString(ctx.Quote(j.table.GRizTableName()))
		if j.table.GRizTableAlias() != j.table.GRizTableName() {
			sb.WriteString(" AS ")
			sb.WriteString(ctx.Quote(j.table.GRizTableAlias()))
		}
		if j.on != nil {
			sb.WriteString(" ON ")
			sb.WriteString(j.on.ToSQL(ctx))
		}
	}

	// WHERE
	sb.WriteString(buildWhere(ctx, b.where))

	// GROUP BY
	if len(b.groupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		for i, c := range b.groupBy {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(selectColSQL(ctx, c))
		}
	}

	// HAVING
	if b.having != nil {
		sb.WriteString(" HAVING ")
		sb.WriteString(b.having.ToSQL(ctx))
	}

	// ORDER BY
	sb.WriteString(buildOrderBy(ctx, b.orderBy))

	// LIMIT
	if b.limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", b.limit))
	}

	// OFFSET
	if b.offset > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", b.offset))
	}

	return sb.String()
}

// selectColSQL produces the SQL fragment for a selectable column.
// For aggregate expressions (COUNT, SUM, …) that implement expr.Expression,
// ToSQL is called directly so the aggregate function syntax is preserved.
// For plain columns the standard quoted "table"."col" form is returned.
func selectColSQL(ctx *expr.BuildContext, c expr.SelectableColumn) string {
	if e, ok := c.(expr.Expression); ok {
		return e.ToSQL(ctx)
	}
	return ctx.ColRef(c.TableName(), c.ColumnName())
}
