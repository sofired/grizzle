package expr

import "strings"

// -------------------------------------------------------------------
// Col — wraps a SelectableColumn as an Expression
// -------------------------------------------------------------------

// Col wraps a SelectableColumn so it can be used anywhere an Expression is
// expected: CASE THEN/ELSE clauses, COALESCE arguments, etc.
//
//	expr.Coalesce(expr.Col(UsersT.Nickname), expr.Lit("anonymous"))
func Col(col SelectableColumn) Expression { return colAsExpr{col: col} }

type colAsExpr struct{ col SelectableColumn }

func (e colAsExpr) RenderSQL(ctx *BuildContext) (string, error) {
	if isNilInterface(e.col) {
		return "", NewError(CodeBuildValidation, "render_column", "column is nil")
	}
	// Use internal colRef when available (preserves complex expressions like
	// window functions, aggregates, arithmetic) — fall back to ColRef otherwise.
	if cr, ok := e.col.(colRefer); ok {
		return cr.colRef(ctx)
	}
	return ctx.ColRef(e.col.TableName(), e.col.ColumnName())
}

// -------------------------------------------------------------------
// litRefer — wraps a literal value so it can be used as a colRefer
// -------------------------------------------------------------------

// litRefer is the internal bridge that lets literal values appear as function
// arguments (arithmetic right-hand sides, etc.).
type litRefer struct{ v any }

func (l litRefer) colRef(ctx *BuildContext) (string, error) { return ctx.Add(l.v), nil }

// colSelAsRef wraps a SelectableColumn as a colRefer — used internally so
// column types can pass themselves to ArithExpr without needing to
// expose the private colRefer interface.
type colSelAsRef struct{ col SelectableColumn }

func (c colSelAsRef) colRef(ctx *BuildContext) (string, error) {
	if isNilInterface(c.col) {
		return "", NewError(CodeBuildValidation, "render_column", "column is nil")
	}
	if cr, ok := c.col.(colRefer); ok {
		return cr.colRef(ctx)
	}
	return ctx.ColRef(c.col.TableName(), c.col.ColumnName())
}

// -------------------------------------------------------------------
// FuncExpr — generic SQL scalar / aggregate function call
// -------------------------------------------------------------------

// FuncExpr represents a SQL scalar function call such as UPPER(col),
// COALESCE(a, b), LENGTH(col), CAST(col AS type), etc.
//
// It implements Expression, SelectableColumn, and colRefer so it can be
// used in SELECT lists, WHERE/HAVING predicates, and as arguments to other
// functions or arithmetic expressions.
type FuncExpr struct {
	fn    string       // e.g. "UPPER", "COALESCE", "LENGTH"
	args  []Expression // function arguments
	alias string       // optional SELECT alias
}

// renderCore renders the function call without the alias.
func (f FuncExpr) renderCore(ctx *BuildContext) (string, error) {
	return renderAtomically(ctx, func() (string, error) {
		if f.fn == "" {
			return "", NewError(CodeBuildValidation, "render_function", "function name is empty")
		}
		parts := make([]string, len(f.args))
		for i, a := range f.args {
			if isNilExpression(a) {
				return "", NewError(CodeBuildValidation, "render_function", "function argument is nil")
			}
			part, err := a.RenderSQL(ctx)
			if err != nil {
				return "", err
			}
			parts[i] = part
		}
		return f.fn + "(" + strings.Join(parts, ", ") + ")", nil
	})
}

// RenderSQL implements Expression. Includes the AS alias when set (for SELECT).
func (f FuncExpr) RenderSQL(ctx *BuildContext) (string, error) {
	return renderAtomically(ctx, func() (string, error) {
		s, err := f.renderCore(ctx)
		if err != nil {
			return "", err
		}
		if f.alias != "" {
			alias, err := ctx.Quote(f.alias)
			if err != nil {
				return "", err
			}
			s += " AS " + alias
		}
		return s, nil
	})
}

// colRef implements colRefer (no alias — for use inside other expressions).
func (f FuncExpr) colRef(ctx *BuildContext) (string, error) { return f.renderCore(ctx) }

// ColumnName implements SelectableColumn.
func (f FuncExpr) ColumnName() string {
	if f.alias != "" {
		return f.alias
	}
	return strings.ToLower(f.fn)
}

// TableName implements SelectableColumn. Functions have no table prefix.
func (f FuncExpr) TableName() string { return "" }

// As returns a copy with the given SELECT alias.
func (f FuncExpr) As(alias string) FuncExpr { f.alias = alias; return f }

// Asc returns an ascending ORDER BY expression on this function.
func (f FuncExpr) Asc() OrderExpr { return OrderExpr{ref: f, dir: "ASC"} }

// Desc returns a descending ORDER BY expression on this function.
func (f FuncExpr) Desc() OrderExpr { return OrderExpr{ref: f, dir: "DESC"} }

// Comparison operators — produce Expressions for use in WHERE / HAVING.
func (f FuncExpr) EQ(val any) Expression  { return binaryExpr{ref: f, op: "=", val: val} }
func (f FuncExpr) NEQ(val any) Expression { return binaryExpr{ref: f, op: "<>", val: val} }
func (f FuncExpr) GT(val any) Expression  { return binaryExpr{ref: f, op: ">", val: val} }
func (f FuncExpr) GTE(val any) Expression { return binaryExpr{ref: f, op: ">=", val: val} }
func (f FuncExpr) LT(val any) Expression  { return binaryExpr{ref: f, op: "<", val: val} }
func (f FuncExpr) LTE(val any) Expression { return binaryExpr{ref: f, op: "<=", val: val} }

// Like adds a LIKE predicate on the function result (useful after LOWER/UPPER).
func (f FuncExpr) Like(pattern string) Expression {
	return likeExpr{ref: f, op: "LIKE", pattern: pattern}
}

// ILike adds an ILIKE predicate (PostgreSQL case-insensitive LIKE).
func (f FuncExpr) ILike(pattern string) Expression {
	return likeExpr{ref: f, op: "ILIKE", pattern: pattern}
}

// -------------------------------------------------------------------
// ArithExpr — arithmetic between columns / columns and literals
// -------------------------------------------------------------------

// ArithExpr represents a SQL arithmetic expression such as col + 1,
// col * rate, or col1 - col2.
//
// It implements Expression, SelectableColumn, and colRefer, so it can
// appear in SELECT lists, WHERE predicates, and as the argument to other
// functions or arithmetic.
//
// Example:
//
//	query.Select(OrdersT.Quantity.Mul(OrdersT.UnitPrice).As("total")).From(OrdersT)
//	query.Select().From(OrdersT).Where(OrdersT.Stock.Sub(5).GTE(0))
type ArithExpr struct {
	left  colRefer
	op    string // "+", "-", "*", "/"
	right colRefer
	alias string
}

// renderCore renders the arithmetic expression without the alias.
func (a ArithExpr) renderCore(ctx *BuildContext) (string, error) {
	return renderAtomically(ctx, func() (string, error) {
		if isNilInterface(a.left) || isNilInterface(a.right) {
			return "", NewError(CodeBuildValidation, "render_arithmetic", "arithmetic operand is nil")
		}
		left, err := a.left.colRef(ctx)
		if err != nil {
			return "", err
		}
		right, err := a.right.colRef(ctx)
		if err != nil {
			return "", err
		}
		return "(" + left + " " + a.op + " " + right + ")", nil
	})
}

// RenderSQL implements Expression. Includes AS alias when set (for SELECT).
func (a ArithExpr) RenderSQL(ctx *BuildContext) (string, error) {
	return renderAtomically(ctx, func() (string, error) {
		s, err := a.renderCore(ctx)
		if err != nil {
			return "", err
		}
		if a.alias != "" {
			alias, err := ctx.Quote(a.alias)
			if err != nil {
				return "", err
			}
			s += " AS " + alias
		}
		return s, nil
	})
}

// colRef implements colRefer (no alias — for use inside other expressions).
func (a ArithExpr) colRef(ctx *BuildContext) (string, error) { return a.renderCore(ctx) }

// ColumnName implements SelectableColumn.
func (a ArithExpr) ColumnName() string { return a.alias }

// TableName implements SelectableColumn. Arithmetic expressions have no table.
func (a ArithExpr) TableName() string { return "" }

// As returns a copy with the given SELECT alias.
func (a ArithExpr) As(alias string) ArithExpr { a.alias = alias; return a }

// Asc returns an ascending ORDER BY on this arithmetic expression.
func (a ArithExpr) Asc() OrderExpr { return OrderExpr{ref: a, dir: "ASC"} }

// Desc returns a descending ORDER BY on this arithmetic expression.
func (a ArithExpr) Desc() OrderExpr { return OrderExpr{ref: a, dir: "DESC"} }

// Chained arithmetic — build compound expressions: (a + b) * c, etc.
func (a ArithExpr) Add(val any) ArithExpr { return ArithExpr{left: a, op: "+", right: litRefer{val}} }
func (a ArithExpr) Sub(val any) ArithExpr { return ArithExpr{left: a, op: "-", right: litRefer{val}} }
func (a ArithExpr) Mul(val any) ArithExpr { return ArithExpr{left: a, op: "*", right: litRefer{val}} }
func (a ArithExpr) Div(val any) ArithExpr { return ArithExpr{left: a, op: "/", right: litRefer{val}} }

// Comparison operators — produce Expressions for use in WHERE / HAVING.
func (a ArithExpr) EQ(val any) Expression  { return binaryExpr{ref: a, op: "=", val: val} }
func (a ArithExpr) NEQ(val any) Expression { return binaryExpr{ref: a, op: "<>", val: val} }
func (a ArithExpr) GT(val any) Expression  { return binaryExpr{ref: a, op: ">", val: val} }
func (a ArithExpr) GTE(val any) Expression { return binaryExpr{ref: a, op: ">=", val: val} }
func (a ArithExpr) LT(val any) Expression  { return binaryExpr{ref: a, op: "<", val: val} }
func (a ArithExpr) LTE(val any) Expression { return binaryExpr{ref: a, op: "<=", val: val} }

// -------------------------------------------------------------------
// PostgreSQL full-text search helper functions
// -------------------------------------------------------------------

// ToTsvector returns to_tsvector(col) or to_tsvector($config, col) —
// converts a column value to a tsvector for use in FTS expressions or SELECT lists.
// config is optional; pass it to specify a text search configuration (e.g. "english").
//
//	expr.ToTsvector(ArticlesT.Body)                    // to_tsvector("articles"."body")
//	expr.ToTsvector(ArticlesT.Body, "english")         // to_tsvector($1, "articles"."body")
func ToTsvector(col SelectableColumn, config ...string) TsvectorExpr {
	if len(config) > 0 {
		return TsvectorExpr{config: config[0], ref: colSelAsRef{col}, hasConfig: true}
	}
	return TsvectorExpr{ref: colSelAsRef{col}}
}

// ToTsquery returns to_tsquery($1) as a standalone tsquery Expression.
// Useful when you need a tsquery value in SELECT lists or as an argument to TsRank.
//
//	expr.ToTsquery("grizzle & orm") // to_tsquery($1)
func ToTsquery(query string) Expression {
	return tsQueryFnExpr{fn: "to_tsquery", query: query}
}

// ToTsqueryWithConfig returns to_tsquery($1, $2) using an explicit text search configuration.
// config is bound as $1 and query as $2, matching the PostgreSQL call signature.
//
//	expr.ToTsqueryWithConfig("english", "grizzle & orm") // to_tsquery($1, $2)
func ToTsqueryWithConfig(config, query string) Expression {
	return tsQueryFnExpr{fn: "to_tsquery", config: config, query: query, hasConfig: true}
}

// PlainToTsquery returns plainto_tsquery($1).
// Converts plain-text input to a tsquery by treating each word as a term connected with AND.
//
//	expr.PlainToTsquery("grizzle orm") // plainto_tsquery($1)
func PlainToTsquery(query string) Expression {
	return tsQueryFnExpr{fn: "plainto_tsquery", query: query}
}

// PlainToTsqueryWithConfig returns plainto_tsquery($1, $2) using an explicit text search configuration.
// config is bound as $1 and query as $2, matching the PostgreSQL call signature.
//
//	expr.PlainToTsqueryWithConfig("english", "grizzle orm") // plainto_tsquery($1, $2)
func PlainToTsqueryWithConfig(config, query string) Expression {
	return tsQueryFnExpr{fn: "plainto_tsquery", config: config, query: query, hasConfig: true}
}

// PhraseToTsquery returns phraseto_tsquery($1).
// Matches an exact phrase — words must appear adjacent and in order.
//
//	expr.PhraseToTsquery("fast full text search") // phraseto_tsquery($1)
func PhraseToTsquery(query string) Expression {
	return tsQueryFnExpr{fn: "phraseto_tsquery", query: query}
}

// PhraseToTsqueryWithConfig returns phraseto_tsquery($1, $2) using an explicit text search configuration.
// config is bound as $1 and query as $2, matching the PostgreSQL call signature.
//
//	expr.PhraseToTsqueryWithConfig("english", "fast full text search") // phraseto_tsquery($1, $2)
func PhraseToTsqueryWithConfig(config, query string) Expression {
	return tsQueryFnExpr{fn: "phraseto_tsquery", config: config, query: query, hasConfig: true}
}

// WebsearchToTsquery returns websearch_to_tsquery($1).
// Parses web-search-style input: quoted phrases, minus for exclusion, OR for alternatives.
//
//	expr.WebsearchToTsquery("grizzle -orm") // websearch_to_tsquery($1)
func WebsearchToTsquery(query string) Expression {
	return tsQueryFnExpr{fn: "websearch_to_tsquery", query: query}
}

// WebsearchToTsqueryWithConfig returns websearch_to_tsquery($1, $2) using an explicit text search
// configuration. config is bound as $1 and query as $2, matching the PostgreSQL call signature.
//
//	expr.WebsearchToTsqueryWithConfig("english", "grizzle -orm") // websearch_to_tsquery($1, $2)
func WebsearchToTsqueryWithConfig(config, query string) Expression {
	return tsQueryFnExpr{fn: "websearch_to_tsquery", config: config, query: query, hasConfig: true}
}

// TsRank returns TS_RANK(col, tsquery_expr) — a relevance ranking function for FTS results.
// col is the tsvector column; tsq is the tsquery expression (use ToTsquery, PlainToTsquery, etc.).
//
//	expr.TsRank(ArticlesT.SearchVector, expr.PlainToTsquery("grizzle orm")).Desc()
//	// → TS_RANK("articles"."search_vector", plainto_tsquery($1)) DESC
func TsRank(col SelectableColumn, tsq Expression) FuncExpr {
	return FuncExpr{fn: "TS_RANK", args: []Expression{Col(col), tsq}}
}

// TsRankCd returns TS_RANK_CD(col, tsquery_expr) — like TsRank but uses cover density ranking.
//
//	expr.TsRankCd(ArticlesT.SearchVector, expr.PlainToTsquery("grizzle orm")).Desc()
//	// → TS_RANK_CD("articles"."search_vector", plainto_tsquery($1)) DESC
func TsRankCd(col SelectableColumn, tsq Expression) FuncExpr {
	return FuncExpr{fn: "TS_RANK_CD", args: []Expression{Col(col), tsq}}
}

// -------------------------------------------------------------------
// AliasedCol — column with a SELECT-list alias (Fix #131)
// -------------------------------------------------------------------

// AliasedCol wraps a SelectableColumn and adds a SELECT-list alias.
// The AS clause is only emitted when the column appears in a SELECT list
// (via RenderSQL); colRef — used internally for ORDER BY and GROUP BY — emits
// only the underlying column reference without the alias.
//
// Usage:
//
//	expr.ColAs(UsersT.Email, "user_email")
type AliasedCol struct {
	col   SelectableColumn
	alias string
}

// ColAs returns col aliased to alias for use in SELECT lists.
// In ORDER BY and GROUP BY contexts only the underlying column reference
// is emitted (no AS clause), which is required by SQL.
func ColAs(col SelectableColumn, alias string) AliasedCol {
	return AliasedCol{col: col, alias: alias}
}

// RenderSQL emits "col AS alias" — for SELECT list position.
func (a AliasedCol) RenderSQL(ctx *BuildContext) (string, error) {
	return renderAtomically(ctx, func() (string, error) {
		ref, err := a.colRef(ctx)
		if err != nil {
			return "", err
		}
		if a.alias != "" {
			alias, err := ctx.Quote(a.alias)
			if err != nil {
				return "", err
			}
			return ref + " AS " + alias, nil
		}
		return ref, nil
	})
}

// colRef emits only the underlying column reference — no alias.
// Used in ORDER BY, GROUP BY, and any other non-SELECT position.
func (a AliasedCol) colRef(ctx *BuildContext) (string, error) {
	if isNilInterface(a.col) {
		return "", NewError(CodeBuildValidation, "render_column", "aliased column is nil")
	}
	if cr, ok := a.col.(colRefer); ok {
		return cr.colRef(ctx)
	}
	return ctx.ColRef(a.col.TableName(), a.col.ColumnName())
}

// ColumnName returns the alias (used as the result column name in scans).
func (a AliasedCol) ColumnName() string {
	if a.alias != "" {
		return a.alias
	}
	return a.col.ColumnName()
}

// TableName returns the underlying column's table name.
func (a AliasedCol) TableName() string { return a.col.TableName() }

// Unwrap returns the underlying SelectableColumn that this AliasedCol wraps.
// This allows callers to inspect the inner column type (e.g. to check whether
// it is a WindowExpr) without depending on the unexported field.
func (a AliasedCol) Unwrap() SelectableColumn { return a.col }

// Asc returns an ascending ORDER BY on the underlying column (no alias).
func (a AliasedCol) Asc() OrderExpr { return OrderExpr{ref: a, dir: "ASC"} }

// Desc returns a descending ORDER BY on the underlying column (no alias).
func (a AliasedCol) Desc() OrderExpr { return OrderExpr{ref: a, dir: "DESC"} }
