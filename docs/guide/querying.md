# Querying

::: warning Target query API
This guide describes the RC.1-parity target query API. The current branch still has implementation gaps: `Build(dialect)` may return only `(sql, args)`, and any silent degradation of unsupported dialect features is non-conforming implementation debt until the error-returning build contract and fail-fast dialect gates are implemented.
:::

All query builders are in the `query` and `expr` packages. The target Go API may keep immutable/value-copy builders for aliasing safety, but receiver mutability is a Go implementation choice; the parity requirement is the rendered SQL behavior.

## Basic SELECT

```go
import (
    "github.com/sofired/grizzle/query"
    "github.com/sofired/grizzle/dialect"
    "myapp/db"
)

// Conceptual shorthand for selecting all generated user columns.
sql, args, err := query.Select().From(db.UsersT).Build(dialect.Postgres)

// SELECT specific columns
sql, args, err := query.Select(db.UsersT.ID, db.UsersT.Username, db.UsersT.Email).
    From(db.UsersT).
    Build(dialect.Postgres)
```

## WHERE

```go
import "github.com/sofired/grizzle/expr"

// Single condition
query.Select().From(db.UsersT).
    Where(db.UsersT.Email.EQ("alice@example.com"))

// AND — nil conditions are intentionally omitted (safe for dynamic filters)
query.Select().From(db.UsersT).
    Where(expr.And(
        db.UsersT.DeletedAt.IsNull(),
        db.UsersT.Enabled.IsTrue(),
    ))

// OR
query.Select().From(db.UsersT).
    Where(expr.Or(
        db.UsersT.Email.EQ("alice@example.com"),
        db.UsersT.Email.EQ("bob@example.com"),
    ))

// NOT
query.Select().From(db.UsersT).
    Where(expr.Not(db.UsersT.Enabled.IsTrue()))
```

### Chaining conditions with .And()

As a shortcut, `.And(e)` appends another condition to an existing WHERE:

```go
q := query.Select().From(db.UsersT).Where(db.UsersT.DeletedAt.IsNull())
if req.RealmID != uuid.Nil {
    q = q.And(db.UsersT.RealmID.EQ(req.RealmID))
}
```

### Dynamic filters

Because `expr.And` drops nil entries, you can write conditional filters cleanly:

```go
func userFilter(req ListUsersRequest) expr.Expression {
    return expr.And(
        db.UsersT.DeletedAt.IsNull(),
        whenPtr(req.RealmID, func(id uuid.UUID) expr.Expression {
            return db.UsersT.RealmID.EQ(id)
        }),
        whenPtr(req.Email, func(e string) expr.Expression {
            return db.UsersT.Email.ILike("%" + e + "%")
        }),
    )
}

// Helper (not in grizzle — define in your app)
func whenPtr[T any](ptr *T, fn func(T) expr.Expression) expr.Expression {
    if ptr == nil {
        return nil
    }
    return fn(*ptr)
}
```

## Column operators

### String columns

| Method | SQL |
|---|---|
| `.EQ(s)` | `col = $n` |
| `.NEQ(s)` | `col <> $n` |
| `.Like(pattern)` | `col LIKE $n` |
| `.ILike(pattern)` | `col ILIKE $n` (PostgreSQL) |
| `.In(s1, s2, …)` | `col IN ($1, $2, …)` |
| `.NotIn(s1, s2, …)` | `col NOT IN ($1, $2, …)` |
| `.IsNull()` | `col IS NULL` |
| `.IsNotNull()` | `col IS NOT NULL` |
| `.EQCol(other)` | `col = other` (column–column) |

### Integer / Float columns

| Method | SQL |
|---|---|
| `.EQ(n)` | `col = $n` |
| `.NEQ(n)` | `col <> $n` |
| `.GT(n)` | `col > $n` |
| `.GTE(n)` | `col >= $n` |
| `.LT(n)` | `col < $n` |
| `.LTE(n)` | `col <= $n` |
| `.Between(lo, hi)` | `col BETWEEN $1 AND $2` or equivalent positional placeholders |
| `.In(…)` / `.NotIn(…)` | `col IN (…)` |

### Timestamp columns

Same comparison operators as integers, but typed to `time.Time`: `.EQ`, `.GT`, `.GTE`, `.LT`, `.LTE`, `.Between`. Also `.GTCol(other)`, `.GTECol(other)` for column–column comparisons.

### Boolean columns

| Method | SQL |
|---|---|
| `.EQ(b)` | `col = $n` |
| `.IsTrue()` | `col = true` |
| `.IsFalse()` | `col = false` |

## ORDER BY

```go
query.Select().From(db.UsersT).
    OrderBy(
        db.UsersT.CreatedAt.Desc(),
        db.UsersT.Username.Asc(),
    )
```

## Pagination

```go
query.Select().From(db.UsersT).
    OrderBy(db.UsersT.CreatedAt.Desc()).
    Limit(20).
    Offset(40)
```

## JOINs

```go
// LEFT JOIN (manual ON condition)
query.Select(db.UsersT.ID, db.RealmsT.Name).
    From(db.UsersT).
    LeftJoin(db.RealmsT, db.RealmsT.ID.EQCol(db.UsersT.RealmID))

// INNER JOIN
query.Select(db.UsersT.ID, db.UsersT.Username, db.RealmsT.Name).
    From(db.UsersT).
    InnerJoin(db.RealmsT, db.RealmsT.ID.EQCol(db.UsersT.RealmID))

// RIGHT JOIN / FULL JOIN are dialect-gated
query.Select(db.UsersT.ID, db.RealmsT.Name).
    From(db.UsersT).
    FullJoin(db.RealmsT, db.RealmsT.ID.EQCol(db.UsersT.RealmID))
```

When you've pre-defined relations, use `JoinRel` / `InnerJoinRel` instead — see [Relations](/guide/relations).

::: warning Dialect compatibility for RIGHT/FULL JOIN
Drizzle RC.1 exposes `rightJoin` for PostgreSQL, MySQL, and SQLite, and `fullJoin` for PostgreSQL and SQLite, not MySQL. Grizzle must match that surface: PostgreSQL may build both, MySQL may build `RIGHT JOIN` only, and SQLite may build either only when the selected engine is known to support RIGHT/FULL JOIN (SQLite 3.39+). Avoid RIGHT/FULL JOIN in queries that must run across all dialects, or check `d.SupportsRightJoin()` / `d.SupportsFullJoin()` before building.
:::

## Executing queries

The `driver/pgx` package provides execution helpers:

```go
import pgxdb "github.com/sofired/grizzle/driver/pgx"

// All rows
users, err := pgxdb.FromSelect[db.UserSelect](ctx, d,
    query.Select().From(db.UsersT).Where(db.UsersT.DeletedAt.IsNull()),
)

// Exactly one row — error if 0 or >1
user, err := pgxdb.FromSelectOne[db.UserSelect](ctx, d,
    query.Select().From(db.UsersT).Where(db.UsersT.ID.EQ(id)),
)

// Zero or one row — returns nil if not found
user, err := pgxdb.FromSelectOpt[db.UserSelect](ctx, d,
    query.Select().From(db.UsersT).Where(db.UsersT.Username.EQ("alice")),
)
```

Or use the lower-level two-step:

```go
rows, err := d.Query(ctx, query.Select().From(db.UsersT))
users, err := pgxdb.ScanAll[db.UserSelect](rows, err)
```

`ScanAll`, `ScanOne`, and `ScanOneOpt` own and close non-nil, non-typed-nil row sets. They return build/query errors before scanning, preserve context cancellation/deadline sentinels, and return redacted stable cardinality errors for zero-or-many rows where the helper requires exactly one row.

## Prepared queries

::: warning Target prepared-query API
This section describes the target Drizzle RC.1-parity prepared-query API. The current branch exposes static, no-parameter pgx helpers; the `BuildPrepared`, param-capable operator, and per-execution `query.Params` examples below are target APIs.
:::

For static queries that run repeatedly on the same shape of data, use
`PreparedSelect` and `PreparedExec`. The SQL is validated against the live
database at startup — wrong column names, type mismatches, and syntax errors
are surfaced before any traffic reaches the handler.

```go
import pgxdb "github.com/sofired/grizzle/driver/pgx"

type ActiveUser struct {
    ID       uuid.UUID `db:"id"`
    Username string    `db:"username"`
    Email    *string   `db:"email"`
}

// Declare at package level or inside an init function.
var activeUsers *pgxdb.PreparedSelect[ActiveUser]

func initQueries(ctx context.Context, d *pgxdb.DB) error {
    var err error
    activeUsers, err = pgxdb.PrepareSelect[ActiveUser](ctx, d, "active_users",
        query.Select(db.UsersT.ID, db.UsersT.Username, db.UsersT.Email).
            From(db.UsersT).
            Where(expr.And(db.UsersT.Enabled.IsTrue(), db.UsersT.DeletedAt.IsNull())).
            OrderBy(db.UsersT.CreatedAt.Desc()),
    )
    return err
}

func listPreparedUsers(ctx context.Context) ([]ActiveUser, error) {
    // At query time — no SQL construction overhead.
    return activeUsers.QueryAll(ctx, nil)
}
```

For multiple statements, use a `Registry` to validate them all in one shot:

```go
var (
    activeUsers *pgxdb.PreparedSelect[ActiveUser]
    softDelete  *pgxdb.PreparedExec
)

// activeUsersQuery and softDeleteQuery are builders declared with the query API.
func initPrepared(ctx context.Context, d *pgxdb.DB) error {
    reg, err := pgxdb.NewRegistry(d)
    if err != nil {
        return err
    }

    activeUsers, err = pgxdb.RegisterSelect[ActiveUser](reg, "active_users", activeUsersQuery)
    if err != nil {
        return err
    }
    softDelete, err = pgxdb.RegisterExec(reg, "soft_delete_user", softDeleteQuery)
    if err != nil {
        return err
    }

    return reg.PrepareAll(ctx)
}
```

The `name` you supply is a trusted static human-readable label for logging and diagnostics. Do not put secrets or personal data in statement names; controls/newlines are rejected and implementations must enforce a finite max length.
At execution time, Grizzle passes the SQL string to pgx so that pgx v5's
per-connection statement cache (`QueryExecModeCacheStatement`) handles
preparation on every pool connection automatically — no cross-connection
named-statement issues.

Use `PreparedExec` for mutations:

::: warning Target named-parameter API
The current pgx prepared helpers cover static, no-parameter statement shapes. The named parameter API below is the target Drizzle-parity design and remains a documented implementation gap until `BuildPrepared`, param-capable operators, and per-execution `query.Params` land.
:::

```go
var softDelete *pgxdb.PreparedExec

func initMutations(ctx context.Context, d *pgxdb.DB) error {
    var err error
    softDelete, err = pgxdb.PrepareExec(ctx, d, "soft_delete_user",
        query.Update(db.UsersT).
            SetParam(db.UsersT.DeletedAt, "deleted_at").
            Where(db.UsersT.ID.EQParam("id")),
    )
    return err
}

func softDeleteUser(ctx context.Context, tx *pgxdb.Tx, userID uuid.UUID) error {
    _, err := softDelete.ExecTx(ctx, tx, query.Params{
        "deleted_at": time.Now(),
        "id":         userID,
    })
    return err
}
```

Prefer generated column handles in mutation parameter helpers because they carry the column type and encoder metadata needed for prepared execution. If a future string-column helper exists, its names must be compile-time literals or generated constants; never pass user input as a column name.

## Raw SQL escape hatch

For expressions not covered by the builder, prefer `expr.RawArgs` when values are involved:

```go
// Use sparingly. `$?` placeholders are bound as driver parameters.
query.Select().From(db.UsersT).
    Where(expr.RawArgs("lower(username) = lower($?)", username))
```

::: warning
Never pass user-controlled strings to `expr.Raw`, the SQL template argument of `expr.RawArgs`, `query.RawSelectSQL`, `query.RawCountSQL`, `db.QueryRaw`, `db.ExecRaw`, `tx.QueryRaw`, `tx.ExecRaw`, or custom external expression/selectable implementations. Only dynamic values belong in bound arguments, and custom expressions must bind values through the build context instead of string interpolation.
:::

## Pessimistic locking

Use `ForUpdate()` or the lower-level `For(LockForUpdate)` to lock rows for the duration of a transaction.

```go
// Lock selected rows against concurrent updates (PostgreSQL, MySQL).
// Convenience form:
users, err := pgxdb.ScanAll[db.UserSelect](
    d.Query(ctx,
        query.Select().
            From(db.UsersT).
            Where(db.UsersT.Status.EQ("active")).
            ForUpdate(),
    ),
)

// Equivalent explicit form:
query.Select().From(db.UsersT).For(query.LockForUpdate)
```

### SKIP LOCKED / NOWAIT

Pass `query.SkipLocked()` or `query.NoWait()` as a second argument to `For()` to control behaviour when locked rows are encountered:

```go
// Skip rows that are already locked (e.g. queue-style job dispatch).
query.Select().From(db.JobsT).For(query.LockForUpdate, query.SkipLocked())

// Fail immediately if any row cannot be locked.
query.Select().From(db.UsersT).For(query.LockForShare, query.NoWait())
```

Both modifiers are supported by PostgreSQL and MySQL 8.0+. SQLite must fail fast or omit this API rather than silently dropping lock behavior.

### Restricting PostgreSQL locks with OF

Pass `Of(table)` to lock only specific tables in a multi-table join. RC.1 exposes lock `of` configuration only for PostgreSQL-style builders. Grizzle must render `OF` only for PostgreSQL-compatible dialects; MySQL and SQLite builders must omit the API or fail fast.

```go
// Generated PostgreSQL table aliases preserve the PGLockTableSource marker
// required by Of(...). Custom marker implementations are trusted extensions;
// generated views, CTEs, subqueries, raw sources, and non-PG handles do not
// carry the marker.
o := db.OrdersT.As("o")
i := db.ItemsT.As("i")

// Only lock orders rows, not the joined items rows.
sql, args, err := query.Select(o.ID).
    From(o).
    LeftJoin(i, expr.ColBase{TableAlias: "i", ColName: "order_id"}.
        EQCol(expr.ColBase{TableAlias: "o", ColName: "id"})).
    ForUpdate().
    Of(o).
    Build(dialect.Postgres)
// SELECT "o"."id" FROM "orders" AS "o" LEFT JOIN "items" AS "i" ON ... FOR UPDATE OF "o"

// Of() also works with the other PostgreSQL lock modes.
query.Select().From(o).For(query.LockForNoKeyUpdate).Of(o)
// SELECT <order columns> FROM "orders" AS "o" FOR NO KEY UPDATE OF "o"
```

Each table passed to `Of()` is identified by its **alias** (`TableRef.Alias`, falling back to `TableRef.Name`), not the underlying table name. Grizzle validates that every `Of(...)` table is an active `PGLockTableSource` handle in the current query and fails with `build_validation` before SQL execution when it is not.

`Of()` and `For()`/`ForUpdate()`/`ForShare()` can be called in any order. Calling `Of()` without a lock mode must fail at `Build`, or be impossible in a dialect-specific typed builder chain.

### Dialect behaviour

| | `FOR UPDATE` | `FOR SHARE` | `FOR NO KEY UPDATE` | `FOR KEY SHARE` | `OF` | `NOWAIT` / `SKIP LOCKED` |
|---|---|---|---|---|---|---|
| **PostgreSQL** | `FOR UPDATE` | `FOR SHARE` | `FOR NO KEY UPDATE` | `FOR KEY SHARE` | Accepts active `PGLockTableSource` handles and renders active alias/name | Supported |
| **MySQL** | `FOR UPDATE` | `FOR SHARE` | Unsupported: omit or fail fast | Unsupported: omit or fail fast | Unsupported: omit or fail fast | Supported (8.0+) |
| **SQLite** | Unsupported: omit or fail fast | Unsupported: omit or fail fast | Unsupported: omit or fail fast | Unsupported: omit or fail fast | Unsupported: omit or fail fast | Unsupported: omit or fail fast |

See [Dialect comparison](../reference/dialects.md) for the full feature table.
