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

// GrizTableName satisfies TableSource. Returns the alias (used as the table
// reference in column expressions against this subquery).
func (s *SubquerySource) GrizTableName() string { return s.alias }

// GrizTableAlias satisfies TableSource.
func (s *SubquerySource) GrizTableAlias() string { return s.alias }

// -------------------------------------------------------------------
// internal expression types
// -------------------------------------------------------------------

type existsExpr struct{ sub *SelectBuilder }

func (e existsExpr) RenderSQL(ctx *expr.BuildContext) (string, error) {
	if e.sub == nil {
		return "", NewError(CodeBuildValidation, "render_subquery", "exists subquery is nil")
	}
	sub, err := e.sub.buildWith(ctx)
	if err != nil {
		return "", err
	}
	return "EXISTS (" + sub + ")", nil
}

type notExistsExpr struct{ sub *SelectBuilder }

func (e notExistsExpr) RenderSQL(ctx *expr.BuildContext) (string, error) {
	if e.sub == nil {
		return "", NewError(CodeBuildValidation, "render_subquery", "not-exists subquery is nil")
	}
	sub, err := e.sub.buildWith(ctx)
	if err != nil {
		return "", err
	}
	return "NOT EXISTS (" + sub + ")", nil
}

type subqueryInExpr struct {
	col expr.SelectableColumn
	sub *SelectBuilder
}

func (e subqueryInExpr) RenderSQL(ctx *expr.BuildContext) (string, error) {
	// Use distinctColSQL to strip any AS alias from an AliasedCol; the IN
	// left-hand side is a column reference, not a SELECT-list position (#131).
	column, err := distinctColSQL(ctx, e.col)
	if err != nil {
		return "", err
	}
	if e.sub == nil {
		return "", NewError(CodeBuildValidation, "render_subquery", "in subquery is nil")
	}
	sub, err := e.sub.buildWith(ctx)
	if err != nil {
		return "", err
	}
	return column + " IN (" + sub + ")", nil
}

type subqueryNotInExpr struct {
	col expr.SelectableColumn
	sub *SelectBuilder
}

func (e subqueryNotInExpr) RenderSQL(ctx *expr.BuildContext) (string, error) {
	// Use distinctColSQL to strip any AS alias from an AliasedCol; the NOT IN
	// left-hand side is a column reference, not a SELECT-list position (#131).
	column, err := distinctColSQL(ctx, e.col)
	if err != nil {
		return "", err
	}
	if e.sub == nil {
		return "", NewError(CodeBuildValidation, "render_subquery", "not-in subquery is nil")
	}
	sub, err := e.sub.buildWith(ctx)
	if err != nil {
		return "", err
	}
	return column + " NOT IN (" + sub + ")", nil
}
