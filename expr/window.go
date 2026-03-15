package expr

import (
	"fmt"
	"strings"
)

// WindowExpr represents a SQL window function call:
//
//	fn(col) OVER (PARTITION BY ... ORDER BY ...)
//
// WindowExpr implements both Expression (usable in WHERE / HAVING / sub-expressions)
// and SelectableColumn (usable in SELECT and ORDER BY).
//
// Example usage:
//
//	query.Select(
//	    UsersT.ID,
//	    expr.RowNumber().PartitionBy(UsersT.RealmID).OrderBy(UsersT.Name.Asc()).As("rn"),
//	    expr.Rank().PartitionBy(UsersT.RealmID).OrderBy(UsersT.Score.Desc()).As("score_rank"),
//	).From(UsersT)
type WindowExpr struct {
	fn          string             // e.g. "ROW_NUMBER", "RANK", "SUM"
	col         SelectableColumn   // nil for no-argument functions (ROW_NUMBER, RANK, etc.)
	offset      *int               // optional numeric argument (NTH_VALUE n, LAG/LEAD offset)
	defaultVal  any                // optional default value for LAG/LEAD
	hasDefault  bool               // true when defaultVal was explicitly set
	partitionBy []SelectableColumn // PARTITION BY columns
	orderBy     []OrderExpr        // ORDER BY inside the window
	alias       string             // optional AS alias
}

// ToSQL renders the window function including the OVER clause and optional alias.
func (w WindowExpr) ToSQL(ctx *BuildContext) string {
	var sb strings.Builder
	sb.WriteString(w.fn)
	sb.WriteString("(")
	if w.col != nil {
		sb.WriteString(w.col.colRef(ctx))
		// Render optional numeric offset (NTH_VALUE n, LAG/LEAD offset).
		if w.offset != nil {
			sb.WriteString(", ")
			sb.WriteString(ctx.Add(*w.offset))
		}
		// Render optional default value for LAG/LEAD (Fix #93 — must use ctx.Add).
		if w.hasDefault {
			sb.WriteString(", ")
			sb.WriteString(ctx.Add(w.defaultVal))
		}
	}
	sb.WriteString(") OVER (")

	var parts []string
	if len(w.partitionBy) > 0 {
		cols := make([]string, len(w.partitionBy))
		for i, c := range w.partitionBy {
			cols[i] = c.colRef(ctx)
		}
		parts = append(parts, "PARTITION BY "+strings.Join(cols, ", "))
	}
	if len(w.orderBy) > 0 {
		orders := make([]string, len(w.orderBy))
		for i, o := range w.orderBy {
			orders[i] = o.ToSQL(ctx)
		}
		parts = append(parts, "ORDER BY "+strings.Join(orders, ", "))
	}
	sb.WriteString(strings.Join(parts, " "))
	sb.WriteString(")")

	if w.alias != "" {
		sb.WriteString(" AS ")
		sb.WriteString(ctx.Quote(w.alias))
	}
	return sb.String()
}

// colRef implements colRefer so WindowExpr can appear in OrderExpr and binary expressions.
func (w WindowExpr) colRef(ctx *BuildContext) string { return w.ToSQL(ctx) }

// ColumnName implements SelectableColumn. Returns the alias if set, otherwise
// the lower-case function name.
func (w WindowExpr) ColumnName() string {
	if w.alias != "" {
		return w.alias
	}
	return strings.ToLower(w.fn)
}

// TableName implements SelectableColumn. Window functions have no table prefix.
func (w WindowExpr) TableName() string { return "" }

// As returns a copy of the window expression with the given output alias.
func (w WindowExpr) As(alias string) WindowExpr {
	w.alias = alias
	return w
}

// PartitionBy returns a copy with the PARTITION BY columns replaced.
func (w WindowExpr) PartitionBy(cols ...SelectableColumn) WindowExpr {
	w.partitionBy = cols
	return w
}

// OrderBy returns a copy with the ORDER BY expressions (inside the OVER clause) replaced.
func (w WindowExpr) OrderBy(exprs ...OrderExpr) WindowExpr {
	w.orderBy = exprs
	return w
}

// Asc returns an ascending ORDER BY expression referencing this window function result.
func (w WindowExpr) Asc() OrderExpr { return OrderExpr{ref: w, dir: "ASC"} }

// Desc returns a descending ORDER BY expression referencing this window function result.
func (w WindowExpr) Desc() OrderExpr { return OrderExpr{ref: w, dir: "DESC"} }

// -------------------------------------------------------------------
// Factory functions
// -------------------------------------------------------------------

// RowNumber returns a ROW_NUMBER() window expression.
func RowNumber() WindowExpr { return WindowExpr{fn: "ROW_NUMBER"} }

// Rank returns a RANK() window expression.
func Rank() WindowExpr { return WindowExpr{fn: "RANK"} }

// DenseRank returns a DENSE_RANK() window expression.
func DenseRank() WindowExpr { return WindowExpr{fn: "DENSE_RANK"} }

// Lead returns a LEAD(col) window expression.
func Lead(col SelectableColumn) WindowExpr { return WindowExpr{fn: "LEAD", col: col} }

// LeadWithDefault returns a LEAD(col, offset, default) window expression.
// The default value is bound as a parameter (Fix #93 — not interpolated directly).
func LeadWithDefault(col SelectableColumn, offset int, defaultVal any) WindowExpr {
	return WindowExpr{fn: "LEAD", col: col, offset: &offset, defaultVal: defaultVal, hasDefault: true}
}

// Lag returns a LAG(col) window expression.
func Lag(col SelectableColumn) WindowExpr { return WindowExpr{fn: "LAG", col: col} }

// LagWithDefault returns a LAG(col, offset, default) window expression.
// The default value is bound as a parameter (Fix #93 — not interpolated directly).
func LagWithDefault(col SelectableColumn, offset int, defaultVal any) WindowExpr {
	return WindowExpr{fn: "LAG", col: col, offset: &offset, defaultVal: defaultVal, hasDefault: true}
}

// FirstValue returns a FIRST_VALUE(col) window expression.
func FirstValue(col SelectableColumn) WindowExpr { return WindowExpr{fn: "FIRST_VALUE", col: col} }

// LastValue returns a LAST_VALUE(col) window expression.
func LastValue(col SelectableColumn) WindowExpr { return WindowExpr{fn: "LAST_VALUE", col: col} }

// NthValue returns an NTH_VALUE(col, n) window expression.
// n must be >= 1; panics with a clear message if n < 1 (Fix #99).
func NthValue(col SelectableColumn, n int) WindowExpr {
	if n < 1 {
		panic(fmt.Sprintf("expr.NthValue: n must be >= 1, got %d", n))
	}
	return WindowExpr{fn: "NTH_VALUE", col: col, offset: &n}
}

// WinSum returns a SUM(col) window expression (aggregate used as a window function).
func WinSum(col SelectableColumn) WindowExpr { return WindowExpr{fn: "SUM", col: col} }

// WinAvg returns an AVG(col) window expression (aggregate used as a window function).
func WinAvg(col SelectableColumn) WindowExpr { return WindowExpr{fn: "AVG", col: col} }

// WinCount returns a COUNT(*) window expression.
func WinCount() WindowExpr { return WindowExpr{fn: "COUNT"} }

// -------------------------------------------------------------------
// Window frame sentinels (Fix #104 — immutable, cannot be mutated)
// -------------------------------------------------------------------

// WindowFrameBound represents a window frame boundary sentinel.
// Values are immutable zero-value structs; they cannot be assigned to or mutated.
type WindowFrameBound struct {
	sql string
}

// SQL returns the raw SQL fragment for this frame bound.
func (w WindowFrameBound) SQL() string { return w.sql }

// unboundedPrecedingBound, currentRowBound, unboundedFollowingBound are the
// unexported singleton values backing the exported accessor functions.
var (
	unboundedPrecedingBound  = WindowFrameBound{sql: "UNBOUNDED PRECEDING"}
	currentRowBound          = WindowFrameBound{sql: "CURRENT ROW"}
	unboundedFollowingBound  = WindowFrameBound{sql: "UNBOUNDED FOLLOWING"}
)

// UnboundedPreceding returns the UNBOUNDED PRECEDING window frame bound.
func UnboundedPreceding() WindowFrameBound { return unboundedPrecedingBound }

// CurrentRow returns the CURRENT ROW window frame bound.
func CurrentRow() WindowFrameBound { return currentRowBound }

// UnboundedFollowing returns the UNBOUNDED FOLLOWING window frame bound.
func UnboundedFollowing() WindowFrameBound { return unboundedFollowingBound }
