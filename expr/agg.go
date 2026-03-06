package expr

import "strings"

// AggExpr represents a SQL aggregate function call such as COUNT(*), SUM(col),
// AVG(col), MAX(col), or MIN(col). It implements both Expression (usable in
// HAVING / WHERE) and SelectableColumn (usable in SELECT and ORDER BY).
//
// Example usage:
//
//	Select(expr.Count().As("cnt"), UsersT.RealmID).
//	    From(UsersT).
//	    GroupBy(UsersT.RealmID).
//	    Having(expr.Count().GT(5)).
//	    OrderBy(expr.Count().Desc())
type AggExpr struct {
	fn       string   // "COUNT", "SUM", "AVG", "MAX", "MIN"
	col      colRefer // nil means COUNT(*)
	distinct bool
	alias    string // optional AS alias (for SELECT only)
}

// ToSQL renders the aggregate function call, including AS alias when set.
// This is the form used in SELECT lists. For HAVING/ORDER BY, create the
// aggregate without As() so no alias is emitted.
func (a AggExpr) ToSQL(ctx *BuildContext) string {
	var arg string
	if a.col == nil {
		arg = "*"
	} else {
		arg = a.col.colRef(ctx)
	}
	if a.distinct {
		arg = "DISTINCT " + arg
	}
	result := a.fn + "(" + arg + ")"
	if a.alias != "" {
		result += " AS " + ctx.Quote(a.alias)
	}
	return result
}

// colRef implements colRefer so AggExpr can be embedded in OrderExpr.
func (a AggExpr) colRef(ctx *BuildContext) string { return a.ToSQL(ctx) }

// ColumnName implements SelectableColumn. Returns the alias if set, otherwise
// the lower-case function name.
func (a AggExpr) ColumnName() string {
	if a.alias != "" {
		return a.alias
	}
	return strings.ToLower(a.fn)
}

// TableName implements SelectableColumn. Aggregates have no table prefix.
func (a AggExpr) TableName() string { return "" }

// As returns a copy of the aggregate with the given output alias.
func (a AggExpr) As(alias string) AggExpr {
	a.alias = alias
	return a
}

// Asc returns an ascending ORDER BY expression.
func (a AggExpr) Asc() OrderExpr { return OrderExpr{ref: a, dir: "ASC"} }

// Desc returns a descending ORDER BY expression.
func (a AggExpr) Desc() OrderExpr { return OrderExpr{ref: a, dir: "DESC"} }

// -------------------------------------------------------------------
// Comparison helpers — produce Expressions suitable for HAVING clauses.
// -------------------------------------------------------------------

func (a AggExpr) GT(val any) Expression  { return binaryExpr{ref: a, op: ">", val: val} }
func (a AggExpr) GTE(val any) Expression { return binaryExpr{ref: a, op: ">=", val: val} }
func (a AggExpr) LT(val any) Expression  { return binaryExpr{ref: a, op: "<", val: val} }
func (a AggExpr) LTE(val any) Expression { return binaryExpr{ref: a, op: "<=", val: val} }
func (a AggExpr) EQ(val any) Expression  { return binaryExpr{ref: a, op: "=", val: val} }
func (a AggExpr) NEQ(val any) Expression { return binaryExpr{ref: a, op: "<>", val: val} }

// -------------------------------------------------------------------
// Factory functions
// -------------------------------------------------------------------

// Count returns COUNT(*).
func Count() AggExpr { return AggExpr{fn: "COUNT"} }

// CountCol returns COUNT(col).
func CountCol(col SelectableColumn) AggExpr { return AggExpr{fn: "COUNT", col: col} }

// CountDistinct returns COUNT(DISTINCT col).
func CountDistinct(col SelectableColumn) AggExpr {
	return AggExpr{fn: "COUNT", col: col, distinct: true}
}

// Sum returns SUM(col).
func Sum(col SelectableColumn) AggExpr { return AggExpr{fn: "SUM", col: col} }

// Avg returns AVG(col).
func Avg(col SelectableColumn) AggExpr { return AggExpr{fn: "AVG", col: col} }

// Max returns MAX(col).
func Max(col SelectableColumn) AggExpr { return AggExpr{fn: "MAX", col: col} }

// Min returns MIN(col).
func Min(col SelectableColumn) AggExpr { return AggExpr{fn: "MIN", col: col} }
