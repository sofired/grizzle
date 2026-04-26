# Querying

All query builders are in the `query` and `expr` packages. Every builder is **immutable** — each method returns a new copy, so you can safely share and extend base queries.

## Basic SELECT

```go
import (
    "github.com/sofired/grizzle/query"
    "github.com/sofired/grizzle/dialect"
    "myapp/db"
)

// SELECT *
sql, args := query.Select().From(db.UsersT).Build(dialect.Postgres)

// SELECT specific columns
sql, args := query.Select(db.UsersT.ID, db.UsersT.Username, db.UsersT.Email).
    From(db.UsersT).
    Build(dialect.Postgres)
```

## WHERE

```go
import "github.com/sofired/grizzle/expr"

// Single condition
query.Select().From(db.UsersT).
    Where(db.UsersT.Email.EQ("alice@example.com"))

// AND — nil conditions are silently dropped (safe for dynamic filters)
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
| `.Between(lo, hi)` | `col BETWEEN $lo AND $hi` |
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
query.Select().From(db.UsersT).
    InnerJoin(db.RealmsT, db.RealmsT.ID.EQCol(db.UsersT.RealmID))

// RIGHT JOIN / FULL JOIN also available
```

When you've pre-defined relations, use `JoinRel` / `InnerJoinRel` instead — see [Relations](/guide/relations).

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

## Prepared queries

For static queries that run repeatedly on the same shape of data, use
`PreparedSelect` and `PreparedExec`. The SQL is validated against the live
database at startup — wrong column names, type mismatches, and syntax errors
are surfaced before any traffic reaches the handler.

```go
import pgxdb "github.com/sofired/grizzle/driver/pgx"

// Declare at package level or inside an init function.
var activeUsers *pgxdb.PreparedSelect[db.UserSelect]

func initQueries(ctx context.Context, d *pgxdb.DB) error {
    var err error
    activeUsers, err = pgxdb.PrepareSelect[db.UserSelect](ctx, d, "active_users",
        query.Select(db.UsersT.ID, db.UsersT.Username, db.UsersT.Email).
            From(db.UsersT).
            Where(expr.And(db.UsersT.Enabled.IsTrue(), db.UsersT.DeletedAt.IsNull())).
            OrderBy(db.UsersT.CreatedAt.Desc()),
    )
    return err
}

// At query time — no SQL construction overhead.
users, err := activeUsers.QueryAll(ctx, db)
```

For multiple statements, use a `Registry` to validate them all in one shot:

```go
reg := pgxdb.NewRegistry(d)
activeUsers := pgxdb.RegisterSelect[db.UserSelect](reg, "active_users", activeUsersQuery)
softDelete  := pgxdb.RegisterExec(reg, "soft_delete_user", softDeleteQuery)

if err := reg.PrepareAll(ctx); err != nil {
    log.Fatal("query validation failed:", err)
}
```

The `name` you supply is a human-readable label for logging and diagnostics.
At execution time, Grizzle passes the SQL string to pgx so that pgx v5's
per-connection statement cache (`QueryExecModeCacheStatement`) handles
preparation on every pool connection automatically — no cross-connection
named-statement issues.

Use `PreparedExec` for mutations:

```go
var softDelete *pgxdb.PreparedExec

func initMutations(ctx context.Context, d *pgxdb.DB) error {
    var err error
    softDelete, err = pgxdb.PrepareExec(ctx, d, "soft_delete_user",
        query.Update(db.UsersT).
            Set("deleted_at", time.Now()).
            Where(db.UsersT.ID.EQ(uuid.Nil)), // placeholder arg
    )
    return err
}

// Inside a transaction:
_, err = softDelete.ExecTx(ctx, tx)
```

## Raw SQL escape hatch

For expressions not covered by the builder, use `expr.Raw`:

```go
// Use sparingly — no escaping is applied
query.Select().From(db.UsersT).
    Where(expr.Raw("lower(username) = lower($1)"))
```

::: warning
Never pass user-controlled input to `expr.Raw`. Use parameterized expressions whenever possible.
:::
