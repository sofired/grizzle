// Package query provides the Grizzle fluent query builder.
// All builders are safe for concurrent use after construction — they are
// immutable value types; each method returns a new copy.
//
// The query builder produces parameterized SQL strings and arg slices.
// Execution is handled by the driver/pgx package (or any compatible executor).
//
// Typical usage:
//
//	sql, args, err := query.Select(UsersT.ID, UsersT.Name).
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
	"errors"
	"reflect"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// TableSource is implemented by generated table types and can appear in
// FROM and JOIN clauses.
type TableSource interface {
	// GrizTableName returns the SQL table name (without schema qualification).
	GrizTableName() string
	// GrizTableAlias returns the alias to use for this table in a query.
	// Usually the same as GrizTableName unless the table has been aliased.
	GrizTableAlias() string
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
	joinCross joinType = "CROSS JOIN"
)

type joinClause struct {
	kind  joinType
	table TableSource
	on    expr.Expression
}

// -------------------------------------------------------------------
// Shared build helper
// -------------------------------------------------------------------

func buildWhere(ctx *expr.BuildContext, where expr.Expression) (string, error) {
	if where == nil {
		return "", nil
	}
	if isNilValue(where) {
		return "", NewError(CodeBuildValidation, "build_where", "where predicate is nil")
	}
	sql, err := where.RenderSQL(ctx)
	if err != nil {
		return "", err
	}
	return " WHERE " + sql, nil
}

func buildOrderBy(ctx *expr.BuildContext, exprs []expr.OrderExpr) (string, error) {
	if len(exprs) == 0 {
		return "", nil
	}
	parts := make([]string, len(exprs))
	for i, o := range exprs {
		part, err := o.RenderSQL(ctx)
		if err != nil {
			return "", err
		}
		parts[i] = part
	}
	s := " ORDER BY "
	for i, p := range parts {
		if i > 0 {
			s += ", "
		}
		s += p
	}
	return s, nil
}

// Build is a convenience wrapper to produce SQL + args from a dialect in one call.
type Builder interface {
	Build(d dialect.Dialect) (string, []any, error)
}

func newBuildContext(d dialect.Dialect) (*expr.BuildContext, error) {
	if isNilValue(d) {
		return nil, NewError(CodeUnsupportedDialect, "build_query", "dialect is nil")
	}
	return expr.NewBuildContext(d), nil
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func normalizeBuildError(op string, err error) error {
	if err == nil {
		return nil
	}
	var buildErr *Error
	if errors.As(err, &buildErr) {
		return buildErr
	}
	return NewError(CodeBuildValidation, op, "query rendering failed")
}

func buildFailure(op string, err error) (string, []any, error) {
	return "", nil, normalizeBuildError(op, err)
}

func quoteTableSource(ctx *expr.BuildContext, table TableSource) (string, error) {
	if isNilValue(table) {
		return "", NewError(CodeBuildValidation, "render_table_source", "table source is nil")
	}
	return ctx.Quote(table.GrizTableName())
}
