package expr

import (
	"fmt"
	"strings"
)

// -------------------------------------------------------------------
// FrameBound — describes one end of a window frame
// -------------------------------------------------------------------

// FrameBound describes one boundary of a window frame (start or end).
// Use the package-level constants and constructors to create values:
//
//	expr.UnboundedPreceding
//	expr.CurrentRow
//	expr.UnboundedFollowing
//	expr.Preceding(n)
//	expr.Following(n)
type FrameBound struct {
	kind   frameBoundKind
	offset int // only used for Preceding(n) / Following(n)
}

type frameBoundKind int

const (
	fbUnboundedPreceding frameBoundKind = iota
	fbPreceding
	fbCurrentRow
	fbFollowing
	fbUnboundedFollowing
)

// UnboundedPreceding is the UNBOUNDED PRECEDING frame boundary.
var UnboundedPreceding = FrameBound{kind: fbUnboundedPreceding}

// CurrentRow is the CURRENT ROW frame boundary.
var CurrentRow = FrameBound{kind: fbCurrentRow}

// UnboundedFollowing is the UNBOUNDED FOLLOWING frame boundary.
var UnboundedFollowing = FrameBound{kind: fbUnboundedFollowing}

// Preceding returns an "n PRECEDING" frame boundary.
func Preceding(n int) FrameBound { return FrameBound{kind: fbPreceding, offset: n} }

// Following returns an "n FOLLOWING" frame boundary.
func Following(n int) FrameBound { return FrameBound{kind: fbFollowing, offset: n} }

func (b FrameBound) sql() string {
	switch b.kind {
	case fbUnboundedPreceding:
		return "UNBOUNDED PRECEDING"
	case fbPreceding:
		return fmt.Sprintf("%d PRECEDING", b.offset)
	case fbCurrentRow:
		return "CURRENT ROW"
	case fbFollowing:
		return fmt.Sprintf("%d FOLLOWING", b.offset)
	case fbUnboundedFollowing:
		return "UNBOUNDED FOLLOWING"
	default:
		return ""
	}
}

// -------------------------------------------------------------------
// frameClause — holds the optional ROWS/RANGE/GROUPS BETWEEN clause
// -------------------------------------------------------------------

type frameMode int

const (
	frameModeRows frameMode = iota
	frameModeRange
	frameModeGroups
)

type frameClause struct {
	mode  frameMode
	start FrameBound
	end   FrameBound
}

func (f *frameClause) sql() string {
	if f == nil {
		return ""
	}
	var keyword string
	switch f.mode {
	case frameModeRows:
		keyword = "ROWS"
	case frameModeRange:
		keyword = "RANGE"
	case frameModeGroups:
		keyword = "GROUPS"
	}
	return keyword + " BETWEEN " + f.start.sql() + " AND " + f.end.sql()
}

// -------------------------------------------------------------------
// WindowExpr
// -------------------------------------------------------------------

// WindowExpr represents a SQL window function call:
//
//	fn(col) OVER (PARTITION BY ... ORDER BY ... ROWS BETWEEN ... AND ...)
//
// WindowExpr implements both Expression (usable in WHERE / HAVING / sub-expressions)
// and SelectableColumn (usable in SELECT and ORDER BY).
//
// Example usage:
//
//	query.Select(
//	    UsersT.ID,
//	    expr.RowNumber().PartitionBy(UsersT.RealmID).OrderBy(UsersT.Name.Asc()).As("rn"),
//	    expr.WinSum(OrdersT.Amount).
//	        PartitionBy(OrdersT.CustomerID).
//	        OrderBy(OrdersT.CreatedAt.Asc()).
//	        Rows(expr.UnboundedPreceding, expr.CurrentRow).
//	        As("running_total"),
//	).From(UsersT)
type WindowExpr struct {
	fn          string             // e.g. "ROW_NUMBER", "RANK", "SUM"
	col         SelectableColumn   // nil for no-argument functions (ROW_NUMBER, RANK, etc.)
	extraArgs   []string           // additional raw SQL arguments (e.g. NTH_VALUE offset)
	partitionBy []SelectableColumn // PARTITION BY columns
	orderBy     []OrderExpr        // ORDER BY inside the window
	frame       *frameClause       // optional ROWS/RANGE/GROUPS BETWEEN clause
	alias       string             // optional AS alias
}

// ToSQL renders the window function including the OVER clause and optional alias.
func (w WindowExpr) ToSQL(ctx *BuildContext) string {
	var sb strings.Builder
	sb.WriteString(w.fn)
	sb.WriteString("(")
	if w.col != nil {
		sb.WriteString(w.col.colRef(ctx))
		for _, a := range w.extraArgs {
			sb.WriteString(", ")
			sb.WriteString(a)
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
	if w.frame != nil {
		parts = append(parts, w.frame.sql())
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

// Rows returns a copy with a ROWS BETWEEN start AND end frame clause.
//
//	expr.WinSum(col).OrderBy(col.Asc()).Rows(expr.UnboundedPreceding, expr.CurrentRow)
//	// → SUM(...) OVER (ORDER BY ... ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)
func (w WindowExpr) Rows(start, end FrameBound) WindowExpr {
	w.frame = &frameClause{mode: frameModeRows, start: start, end: end}
	return w
}

// Range returns a copy with a RANGE BETWEEN start AND end frame clause.
//
//	expr.WinSum(col).OrderBy(col.Asc()).Range(expr.UnboundedPreceding, expr.CurrentRow)
//	// → SUM(...) OVER (ORDER BY ... RANGE BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)
func (w WindowExpr) Range(start, end FrameBound) WindowExpr {
	w.frame = &frameClause{mode: frameModeRange, start: start, end: end}
	return w
}

// Groups returns a copy with a GROUPS BETWEEN start AND end frame clause.
// GROUPS mode is supported in PostgreSQL 11+ and SQLite 3.28+.
//
//	expr.WinSum(col).OrderBy(col.Asc()).Groups(expr.UnboundedPreceding, expr.CurrentRow)
//	// → SUM(...) OVER (ORDER BY ... GROUPS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)
func (w WindowExpr) Groups(start, end FrameBound) WindowExpr {
	w.frame = &frameClause{mode: frameModeGroups, start: start, end: end}
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

// LeadWithOffset returns a LEAD(col, offset) window expression.
//
//	expr.LeadWithOffset(UsersT.Score, 2)
//	// → LEAD("users"."score", 2) OVER (...)
func LeadWithOffset(col SelectableColumn, offset int) WindowExpr {
	return WindowExpr{fn: "LEAD", col: col, extraArgs: []string{fmt.Sprintf("%d", offset)}}
}

// LeadWithDefault returns a LEAD(col, offset, default) window expression.
// The defaultVal must be a literal SQL value (integer, float, boolean, or quoted string).
// For safety, pass only constant values — not user-controlled input.
//
//	expr.LeadWithDefault(UsersT.Score, 1, 0)
//	// → LEAD("users"."score", 1, 0) OVER (...)
func LeadWithDefault(col SelectableColumn, offset int, defaultVal any) WindowExpr {
	return WindowExpr{fn: "LEAD", col: col, extraArgs: []string{
		fmt.Sprintf("%d", offset),
		fmt.Sprintf("%v", defaultVal),
	}}
}

// Lag returns a LAG(col) window expression.
func Lag(col SelectableColumn) WindowExpr { return WindowExpr{fn: "LAG", col: col} }

// LagWithOffset returns a LAG(col, offset) window expression.
//
//	expr.LagWithOffset(UsersT.Score, 2)
//	// → LAG("users"."score", 2) OVER (...)
func LagWithOffset(col SelectableColumn, offset int) WindowExpr {
	return WindowExpr{fn: "LAG", col: col, extraArgs: []string{fmt.Sprintf("%d", offset)}}
}

// LagWithDefault returns a LAG(col, offset, default) window expression.
// The defaultVal must be a literal SQL value (integer, float, boolean, or quoted string).
// For safety, pass only constant values — not user-controlled input.
//
//	expr.LagWithDefault(UsersT.Score, 1, 0)
//	// → LAG("users"."score", 1, 0) OVER (...)
func LagWithDefault(col SelectableColumn, offset int, defaultVal any) WindowExpr {
	return WindowExpr{fn: "LAG", col: col, extraArgs: []string{
		fmt.Sprintf("%d", offset),
		fmt.Sprintf("%v", defaultVal),
	}}
}

// FirstValue returns a FIRST_VALUE(col) window expression.
func FirstValue(col SelectableColumn) WindowExpr { return WindowExpr{fn: "FIRST_VALUE", col: col} }

// LastValue returns a LAST_VALUE(col) window expression.
func LastValue(col SelectableColumn) WindowExpr { return WindowExpr{fn: "LAST_VALUE", col: col} }

// NthValue returns an NTH_VALUE(col, n) window expression.
// The n parameter specifies the row position (1-based) within the window frame.
//
//	expr.NthValue(UsersT.Score, 3)
//	// → NTH_VALUE("users"."score", 3) OVER (...)
func NthValue(col SelectableColumn, n int) WindowExpr {
	return WindowExpr{fn: "NTH_VALUE", col: col, extraArgs: []string{fmt.Sprintf("%d", n)}}
}

// WinSum returns a SUM(col) window expression (aggregate used as a window function).
func WinSum(col SelectableColumn) WindowExpr { return WindowExpr{fn: "SUM", col: col} }

// WinAvg returns an AVG(col) window expression (aggregate used as a window function).
func WinAvg(col SelectableColumn) WindowExpr { return WindowExpr{fn: "AVG", col: col} }

// WinCount returns a COUNT(*) window expression.
func WinCount() WindowExpr { return WindowExpr{fn: "COUNT"} }
