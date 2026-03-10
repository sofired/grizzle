package expr

import "strings"

// CaseExpr builds a searched CASE expression:
//
//	CASE WHEN cond1 THEN val1 WHEN cond2 THEN val2 [ELSE default] END
//
// CaseExpr implements both Expression (usable in WHERE / HAVING) and
// SelectableColumn (usable in SELECT).
//
// The THEN and ELSE values accept any Expression. Use expr.Lit(v) to wrap a
// Go literal, or pass a column reference or another expression directly.
//
// Example usage:
//
//	expr.Case().
//	    When(UsersT.Score.GTE(90), expr.Lit("A")).
//	    When(UsersT.Score.GTE(75), expr.Lit("B")).
//	    Else(expr.Lit("C")).
//	    As("grade")
type CaseExpr struct {
	whens []caseWhen
	else_ Expression // nil = no ELSE clause
	alias string
}

type caseWhen struct {
	cond Expression
	then Expression
}

// Case returns a new, empty CaseExpr. Chain .When() calls to add branches.
func Case() *CaseExpr { return &CaseExpr{} }

// When adds a WHEN cond THEN then branch.
// Both cond and then must be non-nil Expressions; use expr.Lit(v) for literal values.
func (c *CaseExpr) When(cond Expression, then Expression) *CaseExpr {
	cp := *c
	cp.whens = append(append([]caseWhen(nil), c.whens...), caseWhen{cond: cond, then: then})
	return &cp
}

// Else sets the ELSE clause. Use expr.Lit(v) for a literal fallback value.
func (c *CaseExpr) Else(expr Expression) *CaseExpr {
	cp := *c
	cp.else_ = expr
	return &cp
}

// As returns a copy with the given output alias (rendered as AS alias in SELECT).
func (c *CaseExpr) As(alias string) *CaseExpr {
	cp := *c
	cp.alias = alias
	return &cp
}

// ToSQL renders the CASE expression.
func (c *CaseExpr) ToSQL(ctx *BuildContext) string {
	var sb strings.Builder
	sb.WriteString("CASE")
	for _, w := range c.whens {
		sb.WriteString(" WHEN ")
		sb.WriteString(w.cond.ToSQL(ctx))
		sb.WriteString(" THEN ")
		sb.WriteString(w.then.ToSQL(ctx))
	}
	if c.else_ != nil {
		sb.WriteString(" ELSE ")
		sb.WriteString(c.else_.ToSQL(ctx))
	}
	sb.WriteString(" END")
	if c.alias != "" {
		sb.WriteString(" AS ")
		sb.WriteString(ctx.Quote(c.alias))
	}
	return sb.String()
}

// colRef implements colRefer so CaseExpr can appear in OrderExpr and binary expressions.
func (c *CaseExpr) colRef(ctx *BuildContext) string { return c.ToSQL(ctx) }

// ColumnName implements SelectableColumn. Returns the alias if set, otherwise "case".
func (c *CaseExpr) ColumnName() string {
	if c.alias != "" {
		return c.alias
	}
	return "case"
}

// TableName implements SelectableColumn. CASE expressions have no table prefix.
func (c *CaseExpr) TableName() string { return "" }

// Asc returns an ascending ORDER BY expression on this CASE result.
func (c *CaseExpr) Asc() OrderExpr { return OrderExpr{ref: c, dir: "ASC"} }

// Desc returns a descending ORDER BY expression on this CASE result.
func (c *CaseExpr) Desc() OrderExpr { return OrderExpr{ref: c, dir: "DESC"} }

// -------------------------------------------------------------------
// SimpleCaseExpr — CASE col WHEN val THEN result … END
// -------------------------------------------------------------------

// SimpleCaseExpr builds a simple CASE expression:
//
//	CASE col WHEN val1 THEN result1 WHEN val2 THEN result2 [ELSE default] END
//
// The subject column is compared to each WHEN value with =.
// Use expr.Lit(v) for THEN/ELSE literal values.
//
// Example:
//
//	expr.SimpleCase(UsersT.Status).
//	    WhenVal("active", expr.Lit("Active User")).
//	    WhenVal("banned", expr.Lit("Banned")).
//	    Else(expr.Lit("Unknown")).
//	    As("status_label")
type SimpleCaseExpr struct {
	subject SelectableColumn
	whens   []simpleWhen
	else_   Expression
	alias   string
}

type simpleWhen struct {
	val  any
	then Expression
}

// SimpleCase returns a new simple CASE expression with the given subject column.
func SimpleCase(subject SelectableColumn) *SimpleCaseExpr {
	return &SimpleCaseExpr{subject: subject}
}

// WhenVal adds a WHEN val THEN then branch.
func (c *SimpleCaseExpr) WhenVal(val any, then Expression) *SimpleCaseExpr {
	cp := *c
	cp.whens = append(append([]simpleWhen(nil), c.whens...), simpleWhen{val: val, then: then})
	return &cp
}

// Else sets the ELSE clause.
func (c *SimpleCaseExpr) Else(expr Expression) *SimpleCaseExpr {
	cp := *c
	cp.else_ = expr
	return &cp
}

// As returns a copy with the given output alias.
func (c *SimpleCaseExpr) As(alias string) *SimpleCaseExpr {
	cp := *c
	cp.alias = alias
	return &cp
}

// ToSQL renders the simple CASE expression.
func (c *SimpleCaseExpr) ToSQL(ctx *BuildContext) string {
	var sb strings.Builder
	sb.WriteString("CASE ")
	sb.WriteString(c.subject.colRef(ctx))
	for _, w := range c.whens {
		sb.WriteString(" WHEN ")
		sb.WriteString(ctx.Add(w.val))
		sb.WriteString(" THEN ")
		sb.WriteString(w.then.ToSQL(ctx))
	}
	if c.else_ != nil {
		sb.WriteString(" ELSE ")
		sb.WriteString(c.else_.ToSQL(ctx))
	}
	sb.WriteString(" END")
	if c.alias != "" {
		sb.WriteString(" AS ")
		sb.WriteString(ctx.Quote(c.alias))
	}
	return sb.String()
}

func (c *SimpleCaseExpr) colRef(ctx *BuildContext) string { return c.ToSQL(ctx) }
func (c *SimpleCaseExpr) ColumnName() string {
	if c.alias != "" {
		return c.alias
	}
	return "case"
}
func (c *SimpleCaseExpr) TableName() string          { return "" }
func (c *SimpleCaseExpr) Asc() OrderExpr             { return OrderExpr{ref: c, dir: "ASC"} }
func (c *SimpleCaseExpr) Desc() OrderExpr            { return OrderExpr{ref: c, dir: "DESC"} }

// -------------------------------------------------------------------
// Lit — wrap a Go literal as an Expression (for use in THEN / ELSE)
// -------------------------------------------------------------------

// Lit wraps a Go value as a bound-parameter Expression.
// Use this in THEN and ELSE clauses of a CASE expression, or anywhere a
// literal value needs to participate as an Expression rather than as an
// argument to a typed column method.
//
//	expr.Case().When(UsersT.Active.IsTrue(), expr.Lit("yes")).Else(expr.Lit("no"))
func Lit(v any) Expression { return litExpr{v: v} }

type litExpr struct{ v any }

func (e litExpr) ToSQL(ctx *BuildContext) string { return ctx.Add(e.v) }
