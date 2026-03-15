package query

import (
	"fmt"
	"strings"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// lockMode represents the type of row-level locking clause.
type lockMode int

const (
	lockNone           lockMode = iota
	lockForUpdate               // FOR UPDATE
	lockForShare                // FOR SHARE / LOCK IN SHARE MODE (MySQL)
	lockForNoKeyUpdate          // FOR NO KEY UPDATE (PostgreSQL only)
	lockForKeyShare             // FOR KEY SHARE (PostgreSQL only)
)

// SelectBuilder constructs a SELECT query.
// Each method returns a modified copy, so builders can be shared and
// extended without mutating the original.
type SelectBuilder struct {
	ctes           []cteClause             // optional WITH clauses (prepended as CTEs)
	distinct       bool                    // SELECT DISTINCT
	cols           []expr.SelectableColumn // nil = SELECT *
	from           TableSource
	joins          []joinClause
	where          expr.Expression
	orderBy        []expr.OrderExpr
	groupBy        []expr.SelectableColumn
	having         expr.Expression
	limit          int           // 0 = no limit
	offset         int           // 0 = no offset
	lock           lockMode      // row-level locking mode
	lockOf         []TableSource // OF table list (PostgreSQL/MySQL)
	lockNoWait     bool          // NOWAIT modifier
	lockSkipLocked bool          // SKIP LOCKED modifier
}

// cteClause holds a single WITH name AS (...) entry.
// For regular CTEs, sub is set. For recursive CTEs, anchor and recursive are
// set instead and sub is nil; the body renders as "anchor UNION ALL recursive".
type cteClause struct {
	name      string
	sub       *SelectBuilder // regular CTE body
	anchor    *SelectBuilder // recursive CTE: base case
	recursive *SelectBuilder // recursive CTE: recursive term
}

// Select starts a SELECT query specifying the columns to return.
// Pass no columns to SELECT *.
//
//	query.Select(UsersT.ID, UsersT.Name)
//	query.Select() // SELECT *
func Select(cols ...expr.SelectableColumn) *SelectBuilder {
	return &SelectBuilder{cols: cols}
}

// Distinct adds the DISTINCT keyword to the SELECT clause, eliminating
// duplicate rows from the result set.
//
//	query.Select(UsersT.RealmID).From(UsersT).Distinct()
//	// SELECT DISTINCT "users"."realm_id" FROM "users"
func (b *SelectBuilder) Distinct() *SelectBuilder {
	cp := *b
	cp.distinct = true
	return &cp
}

// ForUpdate appends FOR UPDATE to the query, locking selected rows against
// concurrent updates. PostgreSQL and MySQL only — not supported by SQLite.
//
//	query.Select().From(UsersT).Where(UsersT.ID.EQ(id)).ForUpdate()
//	query.Select().From(JobsT).OrderBy(JobsT.CreatedAt.Asc()).Limit(1).ForUpdate().SkipLocked()
func (b *SelectBuilder) ForUpdate() *SelectBuilder {
	cp := *b
	cp.lock = lockForUpdate
	return &cp
}

// ForShare appends FOR SHARE (PostgreSQL) / LOCK IN SHARE MODE (MySQL) to
// the query, locking rows for read while allowing other readers.
func (b *SelectBuilder) ForShare() *SelectBuilder {
	cp := *b
	cp.lock = lockForShare
	return &cp
}

// ForNoKeyUpdate appends FOR NO KEY UPDATE to the query (PostgreSQL only).
// Like FOR UPDATE but does not block SELECT FOR KEY SHARE, making it
// suitable when you need to update non-key columns while allowing
// foreign-key checks to proceed concurrently.
func (b *SelectBuilder) ForNoKeyUpdate() *SelectBuilder {
	cp := *b
	cp.lock = lockForNoKeyUpdate
	return &cp
}

// ForKeyShare appends FOR KEY SHARE to the query (PostgreSQL only).
// The weakest locking mode — blocks FOR UPDATE and FOR NO KEY UPDATE
// while allowing concurrent reads and FOR SHARE locks. Typically used
// by referential-integrity checks.
func (b *SelectBuilder) ForKeyShare() *SelectBuilder {
	cp := *b
	cp.lock = lockForKeyShare
	return &cp
}

// SkipLocked adds the SKIP LOCKED modifier to the locking clause, causing
// already-locked rows to be skipped rather than waited on.
// Supported by PostgreSQL and, in this API, by MySQL when used with FOR UPDATE.
// Commonly used for queue/job-processing patterns.
//
//	query.Select().From(JobsT).Limit(1).ForUpdate().SkipLocked()
//	// → ... FOR UPDATE SKIP LOCKED
func (b *SelectBuilder) SkipLocked() *SelectBuilder {
	cp := *b
	cp.lockSkipLocked = true
	cp.lockNoWait = false
	return &cp
}

// NoWait adds the NOWAIT modifier to the locking clause, causing the query
// to fail immediately with an error if any selected row is already locked,
// rather than waiting for the lock to be released.
// Supported by PostgreSQL and MySQL.
//
//	query.Select().From(AccountsT).Where(AccountsT.ID.EQ(id)).ForUpdate().NoWait()
//	// → ... FOR UPDATE NOWAIT
func (b *SelectBuilder) NoWait() *SelectBuilder {
	cp := *b
	cp.lockNoWait = true
	cp.lockSkipLocked = false
	return &cp
}

// Of restricts the locking clause to the specified tables, adding
// OF "table1", "table2" after the lock mode keyword.
// Supported by PostgreSQL (any number of tables) and MySQL FOR UPDATE (single
// table only — additional tables are silently dropped at render time).
// Has no effect when used with SQLite (locking is silently dropped).
// The OF clause uses each table's alias as it appears in the FROM/JOIN clause.
//
//	query.Select().From(OrdersT).LeftJoin(UsersT, ...).ForUpdate().Of(OrdersT)
//	// → ... FOR UPDATE OF "orders"
func (b *SelectBuilder) Of(tables ...TableSource) *SelectBuilder {
	cp := *b
	cp.lockOf = append(append([]TableSource(nil), cp.lockOf...), tables...)
	return &cp
}

// With adds a Common Table Expression (CTE) to the query.
// The CTE is rendered as WITH name AS (sub) before the SELECT.
// Multiple CTEs are accumulated in order and rendered as WITH a AS (...), b AS (...).
//
// Example:
//
//	recent := query.Select(PostsT.ID, PostsT.AuthorID).
//	    From(PostsT).
//	    Where(PostsT.CreatedAt.GTE(cutoff))
//
//	query.Select(expr.Raw("recent.id")).
//	    With("recent", recent).
//	    From(query.CTERef("recent"))
func (b *SelectBuilder) With(name string, sub *SelectBuilder) *SelectBuilder {
	cp := *b
	cp.ctes = append(append([]cteClause(nil), cp.ctes...), cteClause{name: name, sub: sub})
	return &cp
}

// WithRecursive adds a recursive Common Table Expression (CTE).
// The CTE body is rendered as "anchor UNION ALL recursive", which is the
// standard SQL form for a recursive CTE that iterates until no new rows
// are produced.
//
// Example — traverse an org-chart by manager_id:
//
//	anchor := query.Select(EmployeesT.ID, EmployeesT.ManagerID).
//	    From(EmployeesT).
//	    Where(EmployeesT.ID.EQ(rootID))
//
//	rec := query.Select(EmployeesT.ID, EmployeesT.ManagerID).
//	    From(EmployeesT).
//	    InnerJoin(query.CTERef("org"), EmployeesT.ManagerID.EQCol(ManagerIDCol))
//
//	query.Select().
//	    WithRecursive("org", anchor, rec).
//	    From(query.CTERef("org"))
func (b *SelectBuilder) WithRecursive(name string, anchor, recursive *SelectBuilder) *SelectBuilder {
	cp := *b
	cp.ctes = append(append([]cteClause(nil), cp.ctes...), cteClause{
		name:      name,
		anchor:    anchor,
		recursive: recursive,
	})
	return &cp
}

// CTERef returns a TableSource that references a named CTE defined with .With().
// Use it in From() or Join() to reference the CTE by name.
func CTERef(name string) TableSource { return cteTableSource{name: name} }

type cteTableSource struct{ name string }

func (c cteTableSource) GrizTableName() string  { return c.name }
func (c cteTableSource) GrizTableAlias() string { return c.name }

// From sets the primary table.
func (b *SelectBuilder) From(t TableSource) *SelectBuilder {
	cp := *b
	cp.from = t
	return &cp
}

// Where sets the WHERE predicate. Call And/Or from the expr package to
// combine multiple conditions.
func (b *SelectBuilder) Where(e expr.Expression) *SelectBuilder {
	cp := *b
	cp.where = e
	return &cp
}

// And appends an additional condition with AND semantics.
// Equivalent to .Where(expr.And(existing, e)).
func (b *SelectBuilder) And(e expr.Expression) *SelectBuilder {
	return b.Where(expr.And(b.where, e))
}

// LeftJoin adds a LEFT JOIN clause.
func (b *SelectBuilder) LeftJoin(t TableSource, on expr.Expression) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinLeft, table: t, on: on})
	return &cp
}

// InnerJoin adds an INNER JOIN clause.
func (b *SelectBuilder) InnerJoin(t TableSource, on expr.Expression) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinInner, table: t, on: on})
	return &cp
}

// RightJoin adds a RIGHT JOIN clause.
func (b *SelectBuilder) RightJoin(t TableSource, on expr.Expression) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinRight, table: t, on: on})
	return &cp
}

// FullJoin adds a FULL JOIN clause.
func (b *SelectBuilder) FullJoin(t TableSource, on expr.Expression) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinFull, table: t, on: on})
	return &cp
}

// JoinRel adds a LEFT JOIN using a RelationDef. This is the idiomatic way to
// join tables when the ON condition is already encoded in the relation definition.
//
//	query.Select(UsersT.ID, RealmsT.Name).
//	    From(UsersT).
//	    JoinRel(UserRealm)
func (b *SelectBuilder) JoinRel(rel RelationDef) *SelectBuilder {
	return b.LeftJoin(rel.Table, rel.On)
}

// InnerJoinRel adds an INNER JOIN using a RelationDef.
//
//	query.Select(UsersT.ID, RealmsT.Name).
//	    From(UsersT).
//	    InnerJoinRel(UserRealm)
func (b *SelectBuilder) InnerJoinRel(rel RelationDef) *SelectBuilder {
	return b.InnerJoin(rel.Table, rel.On)
}

// OrderBy sets the ORDER BY clause.
func (b *SelectBuilder) OrderBy(exprs ...expr.OrderExpr) *SelectBuilder {
	cp := *b
	cp.orderBy = exprs
	return &cp
}

// GroupBy sets the GROUP BY clause.
func (b *SelectBuilder) GroupBy(cols ...expr.SelectableColumn) *SelectBuilder {
	cp := *b
	cp.groupBy = cols
	return &cp
}

// Having sets the HAVING clause (requires GroupBy).
func (b *SelectBuilder) Having(e expr.Expression) *SelectBuilder {
	cp := *b
	cp.having = e
	return &cp
}

// Limit sets the maximum number of rows to return. 0 means no limit.
func (b *SelectBuilder) Limit(n int) *SelectBuilder {
	cp := *b
	cp.limit = n
	return &cp
}

// Offset sets the number of rows to skip. 0 means no offset.
func (b *SelectBuilder) Offset(n int) *SelectBuilder {
	cp := *b
	cp.offset = n
	return &cp
}

// Build renders the query to a SQL string and bound arg slice.
func (b *SelectBuilder) Build(d dialect.Dialect) (string, []any) {
	ctx := expr.NewBuildContext(d)
	return b.buildWith(ctx), ctx.Args()
}

// buildWith renders the SELECT statement into an existing BuildContext.
// This is called by Build and by subquery expressions to share parameter numbering.
func (b *SelectBuilder) buildWith(ctx *expr.BuildContext) string {
	var sb strings.Builder

	// WITH [RECURSIVE] (CTEs)
	if len(b.ctes) > 0 {
		hasRecursive := false
		for _, cte := range b.ctes {
			if cte.anchor != nil {
				hasRecursive = true
				break
			}
		}
		if hasRecursive {
			sb.WriteString("WITH RECURSIVE ")
		} else {
			sb.WriteString("WITH ")
		}
		for i, cte := range b.ctes {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(ctx.Quote(cte.name))
			sb.WriteString(" AS (")
			if cte.anchor != nil {
				// Recursive CTE: anchor UNION ALL recursive
				sb.WriteString(cte.anchor.buildWith(ctx))
				sb.WriteString(" UNION ALL ")
				sb.WriteString(cte.recursive.buildWith(ctx))
			} else {
				sb.WriteString(cte.sub.buildWith(ctx))
			}
			sb.WriteString(")")
		}
		sb.WriteString(" ")
	}

	// SELECT [DISTINCT]
	sb.WriteString("SELECT ")
	if b.distinct {
		sb.WriteString("DISTINCT ")
	}
	if len(b.cols) == 0 {
		sb.WriteString("*")
	} else {
		for i, c := range b.cols {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(selectColSQL(ctx, c))
		}
	}

	// FROM
	if b.from != nil {
		sb.WriteString(" FROM ")
		if sq, ok := b.from.(*SubquerySource); ok {
			// Subquery: (SELECT ...) AS alias — render into the same context.
			sb.WriteString("(")
			sb.WriteString(sq.sub.buildWith(ctx))
			sb.WriteString(") AS ")
			sb.WriteString(ctx.Quote(sq.alias))
		} else {
			sb.WriteString(ctx.Quote(b.from.GrizTableName()))
			if b.from.GrizTableAlias() != b.from.GrizTableName() {
				sb.WriteString(" AS ")
				sb.WriteString(ctx.Quote(b.from.GrizTableAlias()))
			}
		}
	}

	// JOINs
	for _, j := range b.joins {
		sb.WriteString(" ")
		sb.WriteString(string(j.kind))
		sb.WriteString(" ")
		sb.WriteString(ctx.Quote(j.table.GrizTableName()))
		if j.table.GrizTableAlias() != j.table.GrizTableName() {
			sb.WriteString(" AS ")
			sb.WriteString(ctx.Quote(j.table.GrizTableAlias()))
		}
		if j.on != nil {
			sb.WriteString(" ON ")
			sb.WriteString(j.on.ToSQL(ctx))
		}
	}

	// WHERE
	sb.WriteString(buildWhere(ctx, b.where))

	// GROUP BY
	if len(b.groupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		for i, c := range b.groupBy {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(selectColSQL(ctx, c))
		}
	}

	// HAVING
	if b.having != nil {
		sb.WriteString(" HAVING ")
		sb.WriteString(b.having.ToSQL(ctx))
	}

	// ORDER BY
	sb.WriteString(buildOrderBy(ctx, b.orderBy))

	// LIMIT
	if b.limit > 0 {
		fmt.Fprintf(&sb, " LIMIT %d", b.limit)
	}

	// OFFSET
	if b.offset > 0 {
		fmt.Fprintf(&sb, " OFFSET %d", b.offset)
	}

	// Locking clauses — dialect-aware.
	// SQLite does not support row-level locking; clauses are silently dropped.
	// FOR NO KEY UPDATE and FOR KEY SHARE are PostgreSQL-only; they are silently
	// dropped on other dialects.
	if b.lock != lockNone && ctx.Dialect().Name() != "sqlite" {
		d := ctx.Dialect().Name()
		switch b.lock {
		case lockForUpdate:
			sb.WriteString(" FOR UPDATE")
		case lockForShare:
			if d == "mysql" {
				sb.WriteString(" LOCK IN SHARE MODE")
			} else {
				sb.WriteString(" FOR SHARE")
			}
		case lockForNoKeyUpdate:
			if d == "postgres" {
				sb.WriteString(" FOR NO KEY UPDATE")
			}
		case lockForKeyShare:
			if d == "postgres" {
				sb.WriteString(" FOR KEY SHARE")
			}
		}
		// OF table list (not applicable to MySQL LOCK IN SHARE MODE syntax, and
		// not applicable to non-postgres dialects when using postgres-only lock modes).
		// On MySQL only a single table may be specified; additional tables are dropped.
		lockModeEmitted := b.lock == lockForUpdate ||
			b.lock == lockForShare ||
			(d == "postgres" && (b.lock == lockForNoKeyUpdate || b.lock == lockForKeyShare))
		if len(b.lockOf) > 0 && lockModeEmitted && (b.lock != lockForShare || d != "mysql") {
			sb.WriteString(" OF ")
			tables := b.lockOf
			if d == "mysql" && len(tables) > 1 {
				tables = tables[:1]
			}
			for i, t := range tables {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(ctx.Quote(t.GrizTableAlias()))
			}
		}
		// Behaviour modifiers — NOWAIT and SKIP LOCKED.
		// MySQL renders FOR SHARE as LOCK IN SHARE MODE, which does not support
		// NOWAIT or SKIP LOCKED; modifiers are only applied for FOR UPDATE on MySQL.
		applyModifier := d == "postgres" ||
			(d == "mysql" && b.lock == lockForUpdate)
		if applyModifier {
			if b.lockSkipLocked {
				sb.WriteString(" SKIP LOCKED")
			} else if b.lockNoWait {
				sb.WriteString(" NOWAIT")
			}
		}
	}

	return sb.String()
}

// selectColSQL produces the SQL fragment for a selectable column.
// For aggregate expressions (COUNT, SUM, …) that implement expr.Expression,
// ToSQL is called directly so the aggregate function syntax is preserved.
// For plain columns the standard quoted "table"."col" form is returned.
func selectColSQL(ctx *expr.BuildContext, c expr.SelectableColumn) string {
	if e, ok := c.(expr.Expression); ok {
		return e.ToSQL(ctx)
	}
	return ctx.ColRef(c.TableName(), c.ColumnName())
}
