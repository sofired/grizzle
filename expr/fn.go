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

func (e colAsExpr) ToSQL(ctx *BuildContext) string {
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

func (l litRefer) colRef(ctx *BuildContext) string { return ctx.Add(l.v) }

// colSelAsRef wraps a SelectableColumn as a colRefer — used internally so
// column types can pass themselves to ArithExpr/CastExpr without needing to
// expose the private colRefer interface.
type colSelAsRef struct{ col SelectableColumn }

func (c colSelAsRef) colRef(ctx *BuildContext) string {
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
func (f FuncExpr) renderCore(ctx *BuildContext) string {
	parts := make([]string, len(f.args))
	for i, a := range f.args {
		parts[i] = a.ToSQL(ctx)
	}
	return f.fn + "(" + strings.Join(parts, ", ") + ")"
}

// ToSQL implements Expression. Includes the AS alias when set (for SELECT).
func (f FuncExpr) ToSQL(ctx *BuildContext) string {
	s := f.renderCore(ctx)
	if f.alias != "" {
		s += " AS " + ctx.Quote(f.alias)
	}
	return s
}

// colRef implements colRefer (no alias — for use inside other expressions).
func (f FuncExpr) colRef(ctx *BuildContext) string { return f.renderCore(ctx) }

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
func (a ArithExpr) renderCore(ctx *BuildContext) string {
	return "(" + a.left.colRef(ctx) + " " + a.op + " " + a.right.colRef(ctx) + ")"
}

// ToSQL implements Expression. Includes AS alias when set (for SELECT).
func (a ArithExpr) ToSQL(ctx *BuildContext) string {
	s := a.renderCore(ctx)
	if a.alias != "" {
		s += " AS " + ctx.Quote(a.alias)
	}
	return s
}

// colRef implements colRefer (no alias — for use inside other expressions).
func (a ArithExpr) colRef(ctx *BuildContext) string { return a.renderCore(ctx) }

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
// CastExpr — CAST(col AS sqlType)
// -------------------------------------------------------------------

// CastExpr represents a SQL CAST expression: CAST(col AS type).
//
//	expr.Cast(UsersT.Score, "numeric(10,2)").As("score_exact")
//	expr.Cast(UsersT.ID, "text").EQ("123")
type CastExpr struct {
	arg     colRefer
	sqlType string
	alias   string
}

// Cast returns CAST(col AS sqlType).
// sqlType must be a raw SQL type string, e.g. "text", "integer", "numeric(10,2)".
//
//	expr.Cast(UsersT.Price, "integer")   // CAST("users"."price" AS integer)
//	expr.Cast(UsersT.ID, "text")         // CAST("users"."id" AS text)
func Cast(col SelectableColumn, sqlType string) CastExpr {
	return CastExpr{arg: colSelAsRef{col}, sqlType: sqlType}
}

// CastExpr on an ArithExpr — allows CAST(col + 1 AS integer).
func CastArith(a ArithExpr, sqlType string) CastExpr {
	return CastExpr{arg: a, sqlType: sqlType}
}

func (e CastExpr) renderCore(ctx *BuildContext) string {
	return "CAST(" + e.arg.colRef(ctx) + " AS " + e.sqlType + ")"
}

func (e CastExpr) ToSQL(ctx *BuildContext) string {
	s := e.renderCore(ctx)
	if e.alias != "" {
		s += " AS " + ctx.Quote(e.alias)
	}
	return s
}

func (e CastExpr) colRef(ctx *BuildContext) string { return e.renderCore(ctx) }
func (e CastExpr) ColumnName() string {
	if e.alias != "" {
		return e.alias
	}
	return "cast"
}
func (e CastExpr) TableName() string        { return "" }
func (e CastExpr) As(alias string) CastExpr { e.alias = alias; return e }
func (e CastExpr) Asc() OrderExpr           { return OrderExpr{ref: e, dir: "ASC"} }
func (e CastExpr) Desc() OrderExpr          { return OrderExpr{ref: e, dir: "DESC"} }
func (e CastExpr) EQ(val any) Expression    { return binaryExpr{ref: e, op: "=", val: val} }
func (e CastExpr) NEQ(val any) Expression   { return binaryExpr{ref: e, op: "<>", val: val} }
func (e CastExpr) GT(val any) Expression    { return binaryExpr{ref: e, op: ">", val: val} }
func (e CastExpr) GTE(val any) Expression   { return binaryExpr{ref: e, op: ">=", val: val} }
func (e CastExpr) LT(val any) Expression    { return binaryExpr{ref: e, op: "<", val: val} }
func (e CastExpr) LTE(val any) Expression   { return binaryExpr{ref: e, op: "<=", val: val} }

// -------------------------------------------------------------------
// COALESCE / NULLIF
// -------------------------------------------------------------------

// Coalesce returns COALESCE(arg1, arg2, ...) — the first non-NULL value.
//
// Use Col() to pass column references; Lit() to pass literal values:
//
//	expr.Coalesce(expr.Col(UsersT.Nickname), expr.Col(UsersT.Name))
//	expr.Coalesce(expr.Col(UsersT.Price), expr.Lit(0.0))
func Coalesce(args ...Expression) FuncExpr {
	return FuncExpr{fn: "COALESCE", args: args}
}

// NullIf returns NULLIF(a, b) — NULL when a equals b, otherwise a.
//
//	expr.NullIf(expr.Col(UsersT.Score), expr.Lit(0))
func NullIf(a, b Expression) FuncExpr {
	return FuncExpr{fn: "NULLIF", args: []Expression{a, b}}
}

// -------------------------------------------------------------------
// String functions
// -------------------------------------------------------------------

// Upper returns UPPER(col).
//
//	expr.Upper(UsersT.Name).Like("ALICE%")
func Upper(col SelectableColumn) FuncExpr {
	return FuncExpr{fn: "UPPER", args: []Expression{Col(col)}}
}

// Lower returns LOWER(col).
//
//	expr.Lower(UsersT.Email).EQ("alice@example.com")
func Lower(col SelectableColumn) FuncExpr {
	return FuncExpr{fn: "LOWER", args: []Expression{Col(col)}}
}

// Length returns LENGTH(col) — number of characters (or bytes, dialect-dependent).
//
//	expr.Length(UsersT.Name).GT(3)
func Length(col SelectableColumn) FuncExpr {
	return FuncExpr{fn: "LENGTH", args: []Expression{Col(col)}}
}

// Trim returns TRIM(col) — strips leading and trailing whitespace.
//
//	expr.Trim(UsersT.Code).EQ("ABC")
func Trim(col SelectableColumn) FuncExpr {
	return FuncExpr{fn: "TRIM", args: []Expression{Col(col)}}
}

// Concat returns CONCAT(col1, col2, ...) — string concatenation.
// Most SQL dialects support CONCAT; PostgreSQL also accepts || but CONCAT is
// portable across MySQL, PostgreSQL, and SQLite (via printf).
//
//	expr.Concat(UsersT.FirstName, expr.Lit(" "), UsersT.LastName).As("full_name")
//
// Note: for mixed column + literal arguments, use expr.Lit() for literals:
//
//	expr.Concat(UsersT.FirstName, expr.Lit(" "), UsersT.LastName)
func Concat(args ...Expression) FuncExpr {
	return FuncExpr{fn: "CONCAT", args: args}
}

// ConcatCols returns CONCAT(col1, col2, ...) for column-only concatenation.
//
//	expr.ConcatCols(UsersT.FirstName, UsersT.LastName)
func ConcatCols(cols ...SelectableColumn) FuncExpr {
	args := make([]Expression, len(cols))
	for i, c := range cols {
		args[i] = Col(c)
	}
	return FuncExpr{fn: "CONCAT", args: args}
}

// -------------------------------------------------------------------
// Numeric / general functions
// -------------------------------------------------------------------

// Abs returns ABS(col).
func Abs(col SelectableColumn) FuncExpr {
	return FuncExpr{fn: "ABS", args: []Expression{Col(col)}}
}

// Ceil returns CEIL(col) (ceiling — round up to nearest integer).
func Ceil(col SelectableColumn) FuncExpr {
	return FuncExpr{fn: "CEIL", args: []Expression{Col(col)}}
}

// Floor returns FLOOR(col) (round down to nearest integer).
func Floor(col SelectableColumn) FuncExpr {
	return FuncExpr{fn: "FLOOR", args: []Expression{Col(col)}}
}

// Round returns ROUND(col) or ROUND(col, decimals).
//
//	expr.Round(OrdersT.Total)         // ROUND("orders"."total")
//	expr.Round(OrdersT.Total, 2)      // ROUND("orders"."total", 2)  (2 decimal places)
func Round(col SelectableColumn, decimals ...int) FuncExpr {
	if len(decimals) > 0 {
		return FuncExpr{fn: "ROUND", args: []Expression{Col(col), Lit(decimals[0])}}
	}
	return FuncExpr{fn: "ROUND", args: []Expression{Col(col)}}
}
