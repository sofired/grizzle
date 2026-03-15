package query

import (
	"fmt"
	"strings"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// LockStrength is the row-level locking mode for SELECT … FOR … statements.
type LockStrength string

const (
	LockForUpdate      LockStrength = "FOR UPDATE"
	LockForNoKeyUpdate LockStrength = "FOR NO KEY UPDATE"
	LockForShare       LockStrength = "FOR SHARE"
	LockForKeyShare    LockStrength = "FOR KEY SHARE"
)

// LockOption is a modifier for the row-level locking clause.
type LockOption string

const (
	// NoWait causes the query to fail immediately if any selected row cannot
	// be locked. Supported by PostgreSQL and MySQL 8.0+; silently dropped for SQLite.
	NoWait LockOption = "NOWAIT"
	// SkipLocked causes the query to skip rows that are already locked, returning
	// only the rows that could be locked. Supported by PostgreSQL and MySQL 8.0+;
	// silently dropped for SQLite.
	SkipLocked LockOption = "SKIP LOCKED"
)

// SelectBuilder constructs a SELECT query.
// Each method returns a modified copy, so builders can be shared and
// extended without mutating the original.
type SelectBuilder struct {
	ctes         []cteClause             // optional WITH clauses (prepended as CTEs)
	distinct     bool                    // SELECT DISTINCT
	distinctOn   []expr.SelectableColumn // DISTINCT ON cols (PostgreSQL only)
	cols         []expr.SelectableColumn // nil = SELECT *
	from         TableSource
	joins        []joinClause
	where        expr.Expression
	orderBy      []expr.OrderExpr
	groupBy      []expr.SelectableColumn
	having       expr.Expression
	limit        int           // 0 = no limit
	offset       int           // 0 = no offset
	lockStrength LockStrength  // row-level lock mode (empty = no lock)
	lockOpts     []LockOption  // NOWAIT / SKIP LOCKED modifiers
	lockOf       []TableSource // OF table list for row-level locking (PostgreSQL/MySQL)
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

// For sets the row-level locking mode for the query. strength must be one of
// LockForUpdate, LockForNoKeyUpdate, LockForShare, or LockForKeyShare.
// Zero or more LockOption modifiers (NoWait, SkipLocked) may be appended.
//
// Dialect behaviour:
//   - PostgreSQL: all four modes are emitted.
//   - MySQL: only LockForUpdate (FOR UPDATE) and LockForShare (LOCK IN SHARE MODE)
//     are emitted; LockForNoKeyUpdate and LockForKeyShare are silently dropped.
//   - SQLite: the entire clause is silently dropped (SQLite has no row-level locking).
//   - NoWait / SkipLocked are supported by PostgreSQL and MySQL 8.0+; silently
//     dropped for SQLite.
//
// Example:
//
//	query.Select().From(db.UsersT).For(query.LockForUpdate)
//	query.Select().From(db.UsersT).For(query.LockForUpdate, query.SkipLocked)
//	query.Select().From(db.UsersT).For(query.LockForShare, query.NoWait)
func (b *SelectBuilder) For(strength LockStrength, opts ...LockOption) *SelectBuilder {
	cp := *b
	cp.lockStrength = strength
	cp.lockOpts = append([]LockOption(nil), opts...)
	return &cp
}

// DistinctOn adds a PostgreSQL-specific SELECT DISTINCT ON (cols) clause,
// returning only the first row of each group defined by the given columns.
// The SELECT list must include at least the DISTINCT ON columns, and ORDER BY
// must start with those columns to make the "first row" deterministic.
//
//	query.Select(UsersT.RealmID, UsersT.Username).
//	    From(UsersT).
//	    DistinctOn(UsersT.RealmID).
//	    OrderBy(UsersT.RealmID.Asc(), UsersT.CreatedAt.Desc())
//	// SELECT DISTINCT ON ("users"."realm_id") "users"."realm_id", ...
//
// Dialect behaviour:
//   - PostgreSQL: rendered as DISTINCT ON (cols).
//   - MySQL / SQLite: SupportsDistinctOn() is false; the DISTINCT ON columns
//     are silently dropped and the query degrades to SELECT DISTINCT.
func (b *SelectBuilder) DistinctOn(cols ...expr.SelectableColumn) *SelectBuilder {
	cp := *b
	cp.distinct = true
	cp.distinctOn = append(append([]expr.SelectableColumn(nil), cp.distinctOn...), cols...)
	return &cp
}

// ForUpdate appends FOR UPDATE to the query, locking selected rows against
// concurrent updates. Supported by PostgreSQL and MySQL; silently dropped for
// SQLite, which uses file-level locking only.
//
// ForUpdate is a convenience wrapper around For(LockForUpdate).
func (b *SelectBuilder) ForUpdate() *SelectBuilder {
	return b.For(LockForUpdate)
}

// ForShare appends FOR SHARE (PostgreSQL) / LOCK IN SHARE MODE (MySQL) to
// the query, locking rows for read while allowing other readers.
// PostgreSQL and MySQL only — SQLite silently drops the clause.
//
// ForShare is a convenience wrapper around For(LockForShare).
func (b *SelectBuilder) ForShare() *SelectBuilder {
	return b.For(LockForShare)
}

// ForNoKeyUpdate appends FOR NO KEY UPDATE to the query. This PostgreSQL-specific
// lock mode is weaker than FOR UPDATE: it does not block INSERT of child rows that
// reference this row via a foreign key. Silently dropped for dialects that do not
// support this locking mode (e.g. MySQL, SQLite).
//
// ForNoKeyUpdate is a convenience wrapper around For(LockForNoKeyUpdate).
func (b *SelectBuilder) ForNoKeyUpdate() *SelectBuilder {
	return b.For(LockForNoKeyUpdate)
}

// ForKeyShare appends FOR KEY SHARE to the query. This PostgreSQL-specific
// lock mode is the weakest row lock: it only blocks DELETE and FOR UPDATE
// operations that would delete or change key values. Silently dropped for
// dialects that do not support this locking mode (e.g. MySQL, SQLite).
//
// ForKeyShare is a convenience wrapper around For(LockForKeyShare).
func (b *SelectBuilder) ForKeyShare() *SelectBuilder {
	return b.For(LockForKeyShare)
}

// Of restricts the locking clause to specific tables, rendering
// OF "alias1", "alias2" after the lock mode keyword.
//
// Each table is rendered using its alias (the value returned by
// GrizTableAlias()), not the underlying table name. PostgreSQL requires the
// alias when the table is aliased; using the base name in that case produces
// an error.
//
// Generated table handles always return the base table name from
// GrizTableAlias() — they do not carry a runtime alias. To use a custom alias,
// construct a value that implements TableSource with the desired alias:
//
//	type ordersAlias struct{}
//	func (ordersAlias) GrizTableName() string  { return "orders" }
//	func (ordersAlias) GrizTableAlias() string { return "o" }
//
//	o := ordersAlias{}
//	query.Select().
//	    From(o).
//	    ForUpdate().
//	    Of(o)
//	// SELECT * FROM "orders" AS "o" FOR UPDATE OF "o"
//
// Of works with all four lock modes (LockForUpdate, LockForNoKeyUpdate,
// LockForShare, LockForKeyShare). Dialect-specific behaviour:
//   - PostgreSQL: all specified tables are emitted for all four lock modes.
//   - MySQL: all specified tables are emitted for FOR UPDATE (MySQL 8.0+).
//     For LOCK IN SHARE MODE (LockForShare on MySQL), OF is not supported and is
//     silently dropped. LockForNoKeyUpdate and LockForKeyShare are dropped entirely
//     on MySQL, so OF has no effect for those modes.
//   - SQLite: OF is silently ignored (SQLite has no row-level locking).
//
// The call order relative to For/ForUpdate/ForShare does not matter.
func (b *SelectBuilder) Of(tables ...TableSource) *SelectBuilder {
	cp := *b
	cp.lockOf = append(append([]TableSource(nil), cp.lockOf...), tables...)
	return &cp
}

// With adds a Common Table Expression (CTE) to the query.
// The CTE is rendered as WITH name AS (sub) before the SELECT.
// Multiple CTEs are accumulated in order and rendered as WITH a AS (...), b AS (...).
//
// CTE support requires SupportsCTE() on the dialect. All built-in dialects
// (PostgreSQL, MySQL 8.0+, SQLite 3.8.3+) return true. When building against a
// dialect where SupportsCTE() is false, the WITH clause is silently dropped.
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
// FULL JOIN requires SupportsFullJoin() on the dialect. When building against a
// dialect where SupportsFullJoin() is false (MySQL, SQLite), the join is
// silently dropped from the output SQL.
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

	// WITH [RECURSIVE] (CTEs) — only emitted for dialects that support CTEs.
	if len(b.ctes) > 0 && ctx.Dialect().SupportsCTE() {
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

	// SELECT [DISTINCT [ON (cols)]]
	sb.WriteString("SELECT ")
	if b.distinct {
		if len(b.distinctOn) > 0 && ctx.Dialect().SupportsDistinctOn() {
			// PostgreSQL DISTINCT ON: SELECT DISTINCT ON (col1, col2) ...
			sb.WriteString("DISTINCT ON (")
			for i, c := range b.distinctOn {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(selectColSQL(ctx, c))
			}
			sb.WriteString(") ")
		} else {
			sb.WriteString("DISTINCT ")
		}
	}
	if len(b.cols) == 0 {
		sb.WriteString("*")
	} else {
		// Window functions are silently dropped for dialects that do not support them.
		written := 0
		for _, c := range b.cols {
			if _, isWin := c.(expr.WindowExpr); isWin && !ctx.Dialect().SupportsWindowFunctions() {
				continue
			}
			if written > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(selectColSQL(ctx, c))
			written++
		}
		if written == 0 {
			// All selected columns were window functions dropped by the dialect — fall back to *.
			sb.WriteString("*")
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

	// JOINs — FULL JOIN is silently dropped for dialects that do not support it.
	for _, j := range b.joins {
		if j.kind == joinFull && !ctx.Dialect().SupportsFullJoin() {
			continue
		}
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

	// Locking clauses — only emitted for dialects that support row-level locking.
	if b.lockStrength != "" && ctx.Dialect().SupportsForUpdate() {
		switch b.lockStrength {
		case LockForNoKeyUpdate, LockForKeyShare:
			// PostgreSQL-only modes: gate on SupportsForNoKeyUpdate.
			if !ctx.Dialect().SupportsForNoKeyUpdate() {
				break
			}
			sb.WriteString(" " + string(b.lockStrength))
			// OF table list: PostgreSQL supports it for all modes.
			if len(b.lockOf) > 0 {
				sb.WriteString(" OF ")
				for i, t := range b.lockOf {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(ctx.Quote(t.GrizTableAlias()))
				}
			}
			// NOWAIT / SKIP LOCKED modifiers.
			for _, opt := range b.lockOpts {
				sb.WriteString(" " + string(opt))
			}
		case LockForShare:
			sb.WriteString(" " + ctx.Dialect().ForShareClause())
			// OF table list: only emitted when the dialect declares support for it
			// (e.g. PostgreSQL FOR SHARE). MySQL's LOCK IN SHARE MODE does not
			// accept an OF clause, so SupportsForShareOf() returns false there.
			if len(b.lockOf) > 0 && ctx.Dialect().SupportsForShareOf() {
				sb.WriteString(" OF ")
				for i, t := range b.lockOf {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(ctx.Quote(t.GrizTableAlias()))
				}
			}
			// NOWAIT / SKIP LOCKED modifiers.
			for _, opt := range b.lockOpts {
				sb.WriteString(" " + string(opt))
			}
		default: // LockForUpdate (and any future modes)
			sb.WriteString(" " + string(b.lockStrength))
			// OF table list: supported by PostgreSQL and MySQL 8.0+ FOR UPDATE.
			if len(b.lockOf) > 0 {
				sb.WriteString(" OF ")
				for i, t := range b.lockOf {
					if i > 0 {
						sb.WriteString(", ")
					}
					sb.WriteString(ctx.Quote(t.GrizTableAlias()))
				}
			}
			// NOWAIT / SKIP LOCKED modifiers.
			for _, opt := range b.lockOpts {
				sb.WriteString(" " + string(opt))
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
