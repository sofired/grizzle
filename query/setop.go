package query

import (
	"fmt"
	"strings"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// SetOpBuilder combines two or more SELECT queries using set operations:
// UNION, UNION ALL, INTERSECT, or EXCEPT.
//
// Example — all active and admin emails combined, deduplicated:
//
//	active := query.Select(UsersT.Email).From(UsersT).Where(UsersT.Active.IsTrue())
//	admin  := query.Select(AdminsT.Email).From(AdminsT)
//
//	sql, args := active.Union(admin).
//	    OrderBy(UsersT.Email.Asc()).
//	    Build(dialect.Postgres)
//	// SELECT "users"."email" FROM "users" WHERE "users"."active" = $1
//	// UNION
//	// (SELECT "admins"."email" FROM "admins")
//	// ORDER BY "users"."email" ASC
type SetOpBuilder struct {
	parts   []setPart
	orderBy []expr.OrderExpr
	limit   int
	offset  int
}

type setPart struct {
	op  string // "", "UNION", "UNION ALL", "INTERSECT", "EXCEPT"
	sel *SelectBuilder
}

// -------------------------------------------------------------------
// Factory methods on SelectBuilder — start a set operation chain
// -------------------------------------------------------------------

// Union combines this SELECT with another using UNION (duplicates removed).
func (b *SelectBuilder) Union(other *SelectBuilder) *SetOpBuilder {
	return newSetOp(b, "UNION", other)
}

// UnionAll combines this SELECT with another using UNION ALL (duplicates kept).
func (b *SelectBuilder) UnionAll(other *SelectBuilder) *SetOpBuilder {
	return newSetOp(b, "UNION ALL", other)
}

// Intersect returns only rows that appear in both SELECT results.
func (b *SelectBuilder) Intersect(other *SelectBuilder) *SetOpBuilder {
	return newSetOp(b, "INTERSECT", other)
}

// Except returns rows in this SELECT that are not in the other.
func (b *SelectBuilder) Except(other *SelectBuilder) *SetOpBuilder {
	return newSetOp(b, "EXCEPT", other)
}

func newSetOp(left *SelectBuilder, op string, right *SelectBuilder) *SetOpBuilder {
	return &SetOpBuilder{
		parts: []setPart{
			{op: "", sel: left},
			{op: op, sel: right},
		},
	}
}

// -------------------------------------------------------------------
// Chaining — add more SELECT queries to an existing set operation
// -------------------------------------------------------------------

// Union adds another SELECT with UNION.
func (b *SetOpBuilder) Union(other *SelectBuilder) *SetOpBuilder {
	return b.addPart("UNION", other)
}

// UnionAll adds another SELECT with UNION ALL.
func (b *SetOpBuilder) UnionAll(other *SelectBuilder) *SetOpBuilder {
	return b.addPart("UNION ALL", other)
}

// Intersect adds another SELECT with INTERSECT.
func (b *SetOpBuilder) Intersect(other *SelectBuilder) *SetOpBuilder {
	return b.addPart("INTERSECT", other)
}

// Except adds another SELECT with EXCEPT.
func (b *SetOpBuilder) Except(other *SelectBuilder) *SetOpBuilder {
	return b.addPart("EXCEPT", other)
}

func (b *SetOpBuilder) addPart(op string, sel *SelectBuilder) *SetOpBuilder {
	cp := *b
	cp.parts = append(append([]setPart(nil), cp.parts...), setPart{op: op, sel: sel})
	return &cp
}

// -------------------------------------------------------------------
// Result shaping
// -------------------------------------------------------------------

// OrderBy sets the ORDER BY for the overall combined result.
// Column references used here must match the column names in the first SELECT.
func (b *SetOpBuilder) OrderBy(exprs ...expr.OrderExpr) *SetOpBuilder {
	cp := *b
	cp.orderBy = exprs
	return &cp
}

// Limit sets the maximum number of rows in the combined result.
func (b *SetOpBuilder) Limit(n int) *SetOpBuilder {
	cp := *b
	cp.limit = n
	return &cp
}

// Offset sets the number of rows to skip in the combined result.
func (b *SetOpBuilder) Offset(n int) *SetOpBuilder {
	cp := *b
	cp.offset = n
	return &cp
}

// -------------------------------------------------------------------
// Build
// -------------------------------------------------------------------

// Build renders the set operation query to a SQL string and bound arg slice.
func (b *SetOpBuilder) Build(d dialect.Dialect) (string, []any) {
	ctx := expr.NewBuildContext(d)
	var sb strings.Builder

	for i, part := range b.parts {
		if i > 0 {
			sb.WriteString(" ")
			sb.WriteString(part.op)
			sb.WriteString(" ")
		}
		// Wrap each component SELECT in parentheses. This is required when
		// individual SELECTs carry their own ORDER BY or LIMIT, and is always
		// syntactically correct for the overall statement.
		sb.WriteString("(")
		sb.WriteString(part.sel.buildWith(ctx))
		sb.WriteString(")")
	}

	// Overall ORDER BY, LIMIT, OFFSET (applied to the combined result set).
	sb.WriteString(buildOrderBy(ctx, b.orderBy))
	if b.limit > 0 {
		fmt.Fprintf(&sb, " LIMIT %d", b.limit)
	}
	if b.offset > 0 {
		fmt.Fprintf(&sb, " OFFSET %d", b.offset)
	}

	return sb.String(), ctx.Args()
}
