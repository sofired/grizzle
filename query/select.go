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
	// be locked. Unsupported dialects cause Build to return ErrUnsupportedFeature.
	NoWait LockOption = "NOWAIT"
	// SkipLocked causes the query to skip rows that are already locked, returning
	// only the rows that could be locked. Unsupported dialects cause Build to
	// return ErrUnsupportedFeature.
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
	lockOfSet    bool
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
//   - MySQL: LockForUpdate and LockForShare are emitted; PostgreSQL-only modes
//     return ErrUnsupportedFeature.
//   - SQLite: all row-locking modes return ErrUnsupportedFeature.
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
// The DISTINCT ON expressions do not need to appear in the SELECT list, but
// ORDER BY must start with those columns to make the "first row" deterministic.
//
//	query.Select(UsersT.RealmID, UsersT.Username).
//	    From(UsersT).
//	    DistinctOn(UsersT.RealmID).
//	    OrderBy(UsersT.RealmID.Asc(), UsersT.CreatedAt.Desc())
//	// SELECT DISTINCT ON ("users"."realm_id") "users"."realm_id", ...
//
// Dialect behaviour:
//   - PostgreSQL: rendered as DISTINCT ON (cols).
//   - MySQL / SQLite: Build returns ErrUnsupportedFeature.
func (b *SelectBuilder) DistinctOn(cols ...expr.SelectableColumn) *SelectBuilder {
	cp := *b
	cp.distinct = true
	cp.distinctOn = append(append([]expr.SelectableColumn(nil), cp.distinctOn...), cols...)
	return &cp
}

// ForUpdate appends FOR UPDATE to the query, locking selected rows against
// concurrent updates. Unsupported dialects cause Build to return
// ErrUnsupportedFeature.
//
// ForUpdate is a convenience wrapper around For(LockForUpdate).
func (b *SelectBuilder) ForUpdate() *SelectBuilder {
	return b.For(LockForUpdate)
}

// ForShare appends FOR SHARE (PostgreSQL) / LOCK IN SHARE MODE (MySQL) to
// the query, locking rows for read while allowing other readers.
// PostgreSQL and MySQL only; unsupported dialects cause Build to return
// ErrUnsupportedFeature.
//
// ForShare is a convenience wrapper around For(LockForShare).
func (b *SelectBuilder) ForShare() *SelectBuilder {
	return b.For(LockForShare)
}

// ForNoKeyUpdate appends FOR NO KEY UPDATE to the query. This PostgreSQL-specific
// lock mode is weaker than FOR UPDATE: it does not block INSERT of child rows that
// reference this row via a foreign key. Unsupported dialects cause Build to
// return ErrUnsupportedFeature.
//
// ForNoKeyUpdate is a convenience wrapper around For(LockForNoKeyUpdate).
func (b *SelectBuilder) ForNoKeyUpdate() *SelectBuilder {
	return b.For(LockForNoKeyUpdate)
}

// ForKeyShare appends FOR KEY SHARE to the query. This PostgreSQL-specific
// lock mode is the weakest row lock: it only blocks DELETE and FOR UPDATE
// operations that would delete or change key values. Unsupported dialects cause
// Build to return ErrUnsupportedFeature.
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
//   - MySQL: lock table lists return ErrUnsupportedFeature.
//   - SQLite: Build returns ErrUnsupportedFeature.
//
// The call order relative to For/ForUpdate/ForShare does not matter.
func (b *SelectBuilder) Of(tables ...TableSource) *SelectBuilder {
	cp := *b
	cp.lockOf = append(append([]TableSource(nil), cp.lockOf...), tables...)
	cp.lockOfSet = true
	return &cp
}

// With adds a Common Table Expression (CTE) to the query.
// The CTE is rendered as WITH name AS (sub) before the SELECT.
// Multiple CTEs are accumulated in order and rendered as WITH a AS (...), b AS (...).
//
// CTE support requires SupportsCTE() on the dialect. All built-in dialects
// (PostgreSQL, MySQL 8.0+, SQLite 3.8.3+) return true. When building against a
// dialect where SupportsCTE() is false, Build returns ErrUnsupportedFeature.
//
// Example:
//
//	recent := query.Select(PostsT.ID, PostsT.AuthorID).
//	    From(PostsT).
//	    Where(PostsT.CreatedAt.GTE(cutoff))
//
//	query.Select(expr.ColBase{TableAlias: "recent", ColName: "id"}).
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
// CTE support requires SupportsCTE() on the dialect. All built-in dialects
// (PostgreSQL, MySQL 8.0+, SQLite 3.8.3+) return true. When building against a
// dialect where SupportsCTE() is false, Build returns ErrUnsupportedFeature.
//
// Example — traverse an org-chart by manager_id:
//
//	anchor := query.Select(EmployeesT.ID, EmployeesT.ManagerID).
//	    From(EmployeesT).
//	    Where(EmployeesT.ID.EQ(rootID))
//
//	// orgID is a typed reference to the "id" column of the "org" CTE:
//	orgID := expr.UUIDColumn{ColBase: expr.ColBase{TableAlias: "org", ColName: "id"}}
//	rec := query.Select(EmployeesT.ID, EmployeesT.ManagerID).
//	    From(EmployeesT).
//	    InnerJoin(query.CTERef("org"), EmployeesT.ManagerID.EQCol(orgID))
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

// RightJoin adds a RIGHT JOIN clause. Build returns ErrUnsupportedFeature when
// the selected dialect cannot guarantee RIGHT JOIN support.
func (b *SelectBuilder) RightJoin(t TableSource, on expr.Expression) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinRight, table: t, on: on})
	return &cp
}

// FullJoin adds a FULL JOIN clause.
// FULL JOIN requires SupportsFullJoin() on the dialect. When building against a
// dialect where SupportsFullJoin() is false (MySQL, SQLite), Build returns
// ErrUnsupportedFeature.
func (b *SelectBuilder) FullJoin(t TableSource, on expr.Expression) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinFull, table: t, on: on})
	return &cp
}

// CrossJoin adds a CROSS JOIN clause. A CROSS JOIN produces the Cartesian product
// of the two tables and has no ON condition.
//
//	query.Select().From(UsersT).CrossJoin(RealmsT)
//	// SELECT * FROM "users" CROSS JOIN "realms"
func (b *SelectBuilder) CrossJoin(t TableSource) *SelectBuilder {
	cp := *b
	cp.joins = append(append([]joinClause(nil), cp.joins...), joinClause{kind: joinCross, table: t})
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

// Build renders the query to a SQL string and bound arg slice. Validation or
// dialect-capability failures return empty SQL and no arguments.
func (b *SelectBuilder) Build(d dialect.Dialect) (string, []any, error) {
	ctx, err := newBuildContext(d)
	if err != nil {
		return buildFailure("build_select", err)
	}
	sql, err := b.buildWith(ctx)
	if err != nil {
		return buildFailure("build_select", err)
	}
	return sql, ctx.Args(), nil
}

// buildWith renders the SELECT statement into an existing BuildContext.
// This is called by Build and by subquery expressions to share parameter numbering.
func (b *SelectBuilder) buildWith(ctx *expr.BuildContext) (string, error) {
	if b == nil {
		return "", NewError(CodeBuildValidation, "build_select", "select builder is nil")
	}
	if b.limit < 0 || b.offset < 0 {
		return "", NewError(CodeBuildValidation, "build_select", "select limit and offset must not be negative")
	}
	var sb strings.Builder

	// WITH [RECURSIVE] (CTEs).
	if len(b.ctes) > 0 {
		if !ctx.Dialect().SupportsCTE() {
			return "", NewError(CodeUnsupportedFeature, "build_select", "common table expressions are not supported by this dialect")
		}
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
			name, err := ctx.Quote(cte.name)
			if err != nil {
				return "", err
			}
			sb.WriteString(name)
			sb.WriteString(" AS (")
			if cte.anchor != nil {
				if cte.recursive == nil {
					return "", NewError(CodeBuildValidation, "build_select", "recursive common table expression contains a nil term")
				}
				// Recursive CTE: anchor UNION ALL recursive
				anchor, err := cte.anchor.buildWith(ctx)
				if err != nil {
					return "", err
				}
				sb.WriteString(anchor)
				sb.WriteString(" UNION ALL ")
				recursive, err := cte.recursive.buildWith(ctx)
				if err != nil {
					return "", err
				}
				sb.WriteString(recursive)
			} else {
				if cte.sub == nil {
					return "", NewError(CodeBuildValidation, "build_select", "common table expression contains a nil select")
				}
				sub, err := cte.sub.buildWith(ctx)
				if err != nil {
					return "", err
				}
				sb.WriteString(sub)
			}
			sb.WriteString(")")
		}
		sb.WriteString(" ")
	}

	// SELECT [DISTINCT [ON (cols)]]
	sb.WriteString("SELECT ")
	if b.distinct {
		if len(b.distinctOn) > 0 {
			if !ctx.Dialect().SupportsDistinctOn() {
				return "", NewError(CodeUnsupportedFeature, "build_select", "distinct on is not supported by this dialect")
			}
			// PostgreSQL DISTINCT ON: SELECT DISTINCT ON (col1, col2) ...
			// Use distinctColSQL (not selectColSQL) to avoid emitting "AS alias"
			// when the caller passes an AliasedCol; DISTINCT ON does not accept aliases.
			sb.WriteString("DISTINCT ON (")
			for i, c := range b.distinctOn {
				if i > 0 {
					sb.WriteString(", ")
				}
				col, err := distinctColSQL(ctx, c)
				if err != nil {
					return "", err
				}
				sb.WriteString(col)
			}
			sb.WriteString(") ")
		} else {
			sb.WriteString("DISTINCT ")
		}
	}
	if len(b.cols) == 0 {
		sb.WriteString("*")
	} else {
		// AliasedCol is unwrapped one level (via Unwrap()) so that window
		// capability checks also apply to aliased window expressions.
		for i, c := range b.cols {
			if isNilValue(c) {
				return "", NewError(CodeBuildValidation, "build_select", "select list contains a nil column")
			}
			if !ctx.Dialect().SupportsWindowFunctions() && isWindowFunction(c) {
				return "", NewError(CodeUnsupportedFeature, "build_select", "window functions are not supported by this dialect")
			}
			if i > 0 {
				sb.WriteString(", ")
			}
			col, err := selectColSQL(ctx, c)
			if err != nil {
				return "", err
			}
			sb.WriteString(col)
		}
	}

	// FROM
	if b.from != nil {
		if isNilValue(b.from) {
			return "", NewError(CodeBuildValidation, "build_select", "from source is nil")
		}
		sb.WriteString(" FROM ")
		if sq, ok := b.from.(*SubquerySource); ok {
			if sq == nil || sq.sub == nil {
				return "", NewError(CodeBuildValidation, "build_select", "from subquery is nil")
			}
			// Subquery: (SELECT ...) AS alias — render into the same context.
			sb.WriteString("(")
			sub, err := sq.sub.buildWith(ctx)
			if err != nil {
				return "", err
			}
			sb.WriteString(sub)
			sb.WriteString(") AS ")
			alias, err := ctx.Quote(sq.alias)
			if err != nil {
				return "", err
			}
			sb.WriteString(alias)
		} else {
			table, err := quoteTableSource(ctx, b.from)
			if err != nil {
				return "", err
			}
			sb.WriteString(table)
			if b.from.GrizTableAlias() != b.from.GrizTableName() {
				sb.WriteString(" AS ")
				alias, err := ctx.Quote(b.from.GrizTableAlias())
				if err != nil {
					return "", err
				}
				sb.WriteString(alias)
			}
		}
	}

	// JOINs.
	for _, j := range b.joins {
		if j.kind == joinRight && !ctx.Dialect().SupportsRightJoin() {
			return "", NewError(CodeUnsupportedFeature, "build_select", "right join is not supported by this dialect")
		}
		if j.kind == joinFull && !ctx.Dialect().SupportsFullJoin() {
			return "", NewError(CodeUnsupportedFeature, "build_select", "full join is not supported by this dialect")
		}
		if j.kind != joinCross && isNilValue(j.on) {
			return "", NewError(CodeBuildValidation, "build_select", "join predicate is nil")
		}
		sb.WriteString(" ")
		sb.WriteString(string(j.kind))
		sb.WriteString(" ")
		table, err := quoteTableSource(ctx, j.table)
		if err != nil {
			return "", err
		}
		sb.WriteString(table)
		if j.table.GrizTableAlias() != j.table.GrizTableName() {
			sb.WriteString(" AS ")
			alias, err := ctx.Quote(j.table.GrizTableAlias())
			if err != nil {
				return "", err
			}
			sb.WriteString(alias)
		}
		if j.kind != joinCross {
			sb.WriteString(" ON ")
			on, err := j.on.RenderSQL(ctx)
			if err != nil {
				return "", err
			}
			sb.WriteString(on)
		}
	}

	// WHERE
	where, err := buildWhere(ctx, b.where)
	if err != nil {
		return "", err
	}
	sb.WriteString(where)

	// GROUP BY
	// Use distinctColSQL (not selectColSQL) to avoid emitting "AS alias" when
	// the caller passes an AliasedCol — GROUP BY does not accept aliases (#131).
	if len(b.groupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		for i, c := range b.groupBy {
			if i > 0 {
				sb.WriteString(", ")
			}
			col, err := distinctColSQL(ctx, c)
			if err != nil {
				return "", err
			}
			sb.WriteString(col)
		}
	}

	// HAVING
	if b.having != nil {
		if isNilValue(b.having) {
			return "", NewError(CodeBuildValidation, "build_select", "having expression is nil")
		}
		sb.WriteString(" HAVING ")
		having, err := b.having.RenderSQL(ctx)
		if err != nil {
			return "", err
		}
		sb.WriteString(having)
	}

	// ORDER BY
	orderBy, err := buildOrderBy(ctx, b.orderBy)
	if err != nil {
		return "", err
	}
	sb.WriteString(orderBy)

	// LIMIT
	if b.limit > 0 {
		_, _ = fmt.Fprintf(&sb, " LIMIT %d", b.limit)
	}

	// OFFSET
	if b.offset > 0 {
		_, _ = fmt.Fprintf(&sb, " OFFSET %d", b.offset)
	}

	// Locking clauses.
	if b.lockStrength != "" {
		switch b.lockStrength {
		case LockForUpdate, LockForNoKeyUpdate, LockForShare, LockForKeyShare:
		default:
			return "", NewError(CodeBuildValidation, "build_select", "row-lock strength is invalid")
		}
		seenOpts := make(map[LockOption]struct{}, len(b.lockOpts))
		for _, opt := range b.lockOpts {
			if opt != NoWait && opt != SkipLocked {
				return "", NewError(CodeBuildValidation, "build_select", "row-lock option is invalid")
			}
			if _, ok := seenOpts[opt]; ok {
				return "", NewError(CodeBuildValidation, "build_select", "row-lock option is duplicated")
			}
			seenOpts[opt] = struct{}{}
		}
		if b.lockOfSet && len(b.lockOf) == 0 {
			return "", NewError(CodeBuildValidation, "build_select", "row-lock table list is empty")
		}
		if !ctx.Dialect().SupportsForUpdate() {
			return "", NewError(CodeUnsupportedFeature, "build_select", "row locking is not supported by this dialect")
		}
		switch b.lockStrength {
		case LockForNoKeyUpdate, LockForKeyShare:
			// PostgreSQL-only modes: gate on SupportsForNoKeyUpdate.
			if !ctx.Dialect().SupportsForNoKeyUpdate() {
				return "", NewError(CodeUnsupportedFeature, "build_select", "requested row-lock strength is not supported by this dialect")
			}
			sb.WriteString(" " + string(b.lockStrength))
			// OF table list: PostgreSQL supports it for all modes.
			if len(b.lockOf) > 0 {
				sb.WriteString(" OF ")
				for i, t := range b.lockOf {
					if err := b.validateLockSource(t); err != nil {
						return "", err
					}
					if i > 0 {
						sb.WriteString(", ")
					}
					alias, err := ctx.Quote(t.GrizTableAlias())
					if err != nil {
						return "", err
					}
					sb.WriteString(alias)
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
			if len(b.lockOf) > 0 && !ctx.Dialect().SupportsForShareOf() {
				return "", NewError(CodeUnsupportedFeature, "build_select", "row-lock table lists are not supported by this dialect")
			}
			if len(b.lockOf) > 0 {
				sb.WriteString(" OF ")
				for i, t := range b.lockOf {
					if err := b.validateLockSource(t); err != nil {
						return "", err
					}
					if i > 0 {
						sb.WriteString(", ")
					}
					alias, err := ctx.Quote(t.GrizTableAlias())
					if err != nil {
						return "", err
					}
					sb.WriteString(alias)
				}
			}
			// NOWAIT / SKIP LOCKED modifiers.
			for _, opt := range b.lockOpts {
				sb.WriteString(" " + string(opt))
			}
		default: // LockForUpdate (and any future modes)
			sb.WriteString(" " + string(b.lockStrength))
			if len(b.lockOf) > 0 && !ctx.Dialect().SupportsForShareOf() {
				return "", NewError(CodeUnsupportedFeature, "build_select", "row-lock table lists are not supported by this dialect")
			}
			// OF table lists are PostgreSQL-compatible.
			if len(b.lockOf) > 0 {
				sb.WriteString(" OF ")
				for i, t := range b.lockOf {
					if err := b.validateLockSource(t); err != nil {
						return "", err
					}
					if i > 0 {
						sb.WriteString(", ")
					}
					alias, err := ctx.Quote(t.GrizTableAlias())
					if err != nil {
						return "", err
					}
					sb.WriteString(alias)
				}
			}
			// NOWAIT / SKIP LOCKED modifiers.
			for _, opt := range b.lockOpts {
				sb.WriteString(" " + string(opt))
			}
		}
	} else if b.lockOfSet {
		return "", NewError(CodeBuildValidation, "build_select", "row-lock table list requires a lock mode")
	}

	return sb.String(), nil
}

func (b *SelectBuilder) validateLockSource(target TableSource) error {
	if isNilValue(target) {
		return NewError(CodeBuildValidation, "build_select", "row-lock table source is nil")
	}
	wantAlias := target.GrizTableAlias()
	if b.from != nil && !isNilValue(b.from) && b.from.GrizTableAlias() == wantAlias {
		return nil
	}
	for _, join := range b.joins {
		if !isNilValue(join.table) && join.table.GrizTableAlias() == wantAlias {
			return nil
		}
	}
	return NewError(CodeBuildValidation, "build_select", "row-lock table source is not active in the query")
}

// isWindowFunction reports whether c is a window function expression.
// It handles both expr.WindowExpr (value) and *expr.WindowExpr (pointer) so
// that callers who pass a pointer satisfying SelectableColumn are also detected.
// It also unwraps one level of AliasedCol so that expr.ColAs(expr.RowNumber(), "rn")
// is correctly identified alongside a bare WindowExpr.
func isWindowFunction(c expr.SelectableColumn) bool {
	if isWindowExprType(c) {
		return true
	}
	type unwrapper interface{ Unwrap() expr.SelectableColumn }
	if u, ok := c.(unwrapper); ok {
		return isWindowExprType(u.Unwrap())
	}
	return false
}

// isWindowExprType reports whether c is an expr.WindowExpr or *expr.WindowExpr.
func isWindowExprType(c expr.SelectableColumn) bool {
	if _, ok := c.(expr.WindowExpr); ok {
		return true
	}
	if _, ok := c.(*expr.WindowExpr); ok {
		return true
	}
	return false
}

// selectColSQL produces the SQL fragment for a selectable column.
// For aggregate expressions (COUNT, SUM, …) that implement expr.Expression,
// RenderSQL is called directly so the aggregate function syntax is preserved.
// For plain columns the standard quoted "table"."col" form is returned.
func selectColSQL(ctx *expr.BuildContext, c expr.SelectableColumn) (string, error) {
	if isNilValue(c) {
		return "", NewError(CodeBuildValidation, "render_select_column", "selectable column is nil")
	}
	if e, ok := c.(expr.Expression); ok {
		return e.RenderSQL(ctx)
	}
	return ctx.ColRef(c.TableName(), c.ColumnName())
}

// distinctColSQL produces the SQL fragment for a column in non-SELECT positions
// such as GROUP BY and DISTINCT ON where an AS alias clause is invalid.
// AliasedCol values are unwrapped one level so only the bare column reference
// is emitted; all other expression types are rendered via RenderSQL as usual.
func distinctColSQL(ctx *expr.BuildContext, c expr.SelectableColumn) (string, error) {
	if isNilValue(c) {
		return "", NewError(CodeBuildValidation, "render_select_column", "selectable column is nil")
	}
	// Unwrap one level of AliasedCol so we render the inner column, not the alias.
	type unwrapper interface{ Unwrap() expr.SelectableColumn }
	if u, ok := c.(unwrapper); ok {
		c = u.Unwrap()
	}
	return selectColSQL(ctx, c)
}
