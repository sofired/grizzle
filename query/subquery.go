package query

import "github.com/sofired/grizzle/expr"

// -------------------------------------------------------------------
// Subquery expressions — usable in WHERE / HAVING clauses
// -------------------------------------------------------------------

// Exists returns an EXISTS (SELECT ...) expression.
//
//	Where(query.Exists(query.Select(expr.Raw("1")).From(PostsT).Where(...)))
func Exists(sub *SelectBuilder) expr.Expression {
	return existsExpr{sub: sub}
}

// NotExists returns a NOT EXISTS (SELECT ...) expression.
func NotExists(sub *SelectBuilder) expr.Expression {
	return notExistsExpr{sub: sub}
}

// SubqueryIn returns a "col IN (SELECT ...)" expression.
//
//	Where(query.SubqueryIn(UsersT.ID, query.Select(PostsT.AuthorID).From(PostsT)))
func SubqueryIn(col expr.SelectableColumn, sub *SelectBuilder) expr.Expression {
	return subqueryInExpr{col: col, sub: sub}
}

// SubqueryNotIn returns a "col NOT IN (SELECT ...)" expression.
func SubqueryNotIn(col expr.SelectableColumn, sub *SelectBuilder) expr.Expression {
	return subqueryNotInExpr{col: col, sub: sub}
}

// -------------------------------------------------------------------
// SubquerySource — use a SELECT as a FROM clause
// -------------------------------------------------------------------

// SubquerySource wraps a SelectBuilder so it can be used as a FROM clause.
// The alias is required for SQL validity.
//
//	sub := query.FromSubquery(
//	    query.Select(UsersT.RealmID, expr.Count().As("cnt")).
//	        From(UsersT).GroupBy(UsersT.RealmID),
//	    "counts",
//	)
//	query.Select(...).From(sub)
type SubquerySource struct {
	sub   *SelectBuilder
	alias string
}

// FromSubquery wraps sub as a named subquery table source: (SELECT …) AS alias.
func FromSubquery(sub *SelectBuilder, alias string) *SubquerySource {
	return &SubquerySource{sub: sub, alias: alias}
}

// GRizTableName satisfies TableSource. Returns the alias (used as the table
// reference in column expressions against this subquery).
func (s *SubquerySource) GRizTableName() string { return s.alias }

// GRizTableAlias satisfies TableSource.
func (s *SubquerySource) GRizTableAlias() string { return s.alias }

// -------------------------------------------------------------------
// internal expression types
// -------------------------------------------------------------------

type existsExpr struct{ sub *SelectBuilder }

func (e existsExpr) ToSQL(ctx *expr.BuildContext) string {
	return "EXISTS (" + e.sub.buildWith(ctx) + ")"
}

type notExistsExpr struct{ sub *SelectBuilder }

func (e notExistsExpr) ToSQL(ctx *expr.BuildContext) string {
	return "NOT EXISTS (" + e.sub.buildWith(ctx) + ")"
}

type subqueryInExpr struct {
	col expr.SelectableColumn
	sub *SelectBuilder
}

func (e subqueryInExpr) ToSQL(ctx *expr.BuildContext) string {
	return selectColSQL(ctx, e.col) + " IN (" + e.sub.buildWith(ctx) + ")"
}

type subqueryNotInExpr struct {
	col expr.SelectableColumn
	sub *SelectBuilder
}

func (e subqueryNotInExpr) ToSQL(ctx *expr.BuildContext) string {
	return selectColSQL(ctx, e.col) + " NOT IN (" + e.sub.buildWith(ctx) + ")"
}
