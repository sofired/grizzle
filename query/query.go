// Package query provides the G-rizzle fluent query builder.
// All builders are safe for concurrent use after construction — they are
// immutable value types; each method returns a new copy.
//
// The query builder produces parameterized SQL strings and arg slices.
// Execution is handled by the driver/pgx package (or any compatible executor).
//
// Typical usage:
//
//	sql, args := query.Select(UsersT.ID, UsersT.Name).
//	    From(UsersT).
//	    Where(expr.And(
//	        UsersT.RealmID.EQ(realmID),
//	        UsersT.DeletedAt.IsNull(),
//	    )).
//	    OrderBy(UsersT.Name.Asc()).
//	    Limit(50).
//	    Build(dialect.Postgres)
package query

import (
	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// TableSource is implemented by generated table types and can appear in
// FROM and JOIN clauses.
type TableSource interface {
	// GRizTableName returns the SQL table name (without schema qualification).
	GRizTableName() string
	// GRizTableAlias returns the alias to use for this table in a query.
	// Usually the same as GRizTableName unless the table has been aliased.
	GRizTableAlias() string
}

// -------------------------------------------------------------------
// joinClause — internal representation of a JOIN
// -------------------------------------------------------------------

type joinType string

const (
	joinInner joinType = "INNER JOIN"
	joinLeft  joinType = "LEFT JOIN"
	joinRight joinType = "RIGHT JOIN"
	joinFull  joinType = "FULL JOIN"
)

type joinClause struct {
	kind  joinType
	table TableSource
	on    expr.Expression
}

// -------------------------------------------------------------------
// Shared build helper
// -------------------------------------------------------------------

func buildWhere(ctx *expr.BuildContext, where expr.Expression) string {
	if where == nil {
		return ""
	}
	return " WHERE " + where.ToSQL(ctx)
}

func buildOrderBy(ctx *expr.BuildContext, exprs []expr.OrderExpr) string {
	if len(exprs) == 0 {
		return ""
	}
	parts := make([]string, len(exprs))
	for i, o := range exprs {
		parts[i] = o.ToSQL(ctx)
	}
	s := " ORDER BY "
	for i, p := range parts {
		if i > 0 {
			s += ", "
		}
		s += p
	}
	return s
}

// Build is a convenience wrapper to produce SQL + args from a dialect in one call.
type Builder interface {
	Build(d dialect.Dialect) (string, []any)
}
