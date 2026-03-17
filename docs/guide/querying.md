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

Pass `query.SkipLocked` or `query.NoWait` as a second argument to `For()` to control behaviour when locked rows are encountered:

```go
// Skip rows that are already locked (e.g. queue-style job dispatch).
query.Select().From(db.JobsT).For(query.LockForUpdate, query.SkipLocked)

// Fail immediately if any row cannot be locked.
query.Select().From(db.UsersT).For(query.LockForShare, query.NoWait)
```

Both modifiers are supported by PostgreSQL and MySQL 8.0+. They are silently dropped for SQLite.

### Restricting locks with OF

Pass `Of(table)` to lock only specific tables in a multi-table join. `Of()` works with all four lock modes.

```go
// Generated table handles always return the base table name from
// GrizTableAlias(). To use a custom alias in the FROM/JOIN and OF clauses,
// implement TableSource with the desired alias:
type ordersAlias struct{}
func (ordersAlias) GrizTableName() string  { return "orders" }
func (ordersAlias) GrizTableAlias() string { return "o" }

type itemsAlias struct{}
func (itemsAlias) GrizTableName() string  { return "items" }
func (itemsAlias) GrizTableAlias() string { return "i" }

o, i := ordersAlias{}, itemsAlias{}

// Only lock orders rows, not the joined items rows.
sql, args := query.Select().
    From(o).
    LeftJoin(i, db.ItemsT.OrderID.EQCol(db.OrdersT.ID)).
    ForUpdate().
    Of(o).
    Build(dialect.Postgres)
// SELECT * FROM "orders" AS "o" LEFT JOIN "items" AS "i" ON ... FOR UPDATE OF "o"

// Of() also works with the other lock modes (all four on PostgreSQL).
query.Select().From(o).For(query.LockForNoKeyUpdate).Of(o)
// SELECT * FROM "orders" AS "o" FOR NO KEY UPDATE OF "o"
```

Each table passed to `Of()` is identified by its **alias** (the value returned by `GrizTableAlias()`), not the underlying table name. Using the base table name when an alias is in scope causes a PostgreSQL error.

`Of()` and `For()`/`ForUpdate()`/`ForShare()` can be called in any order.

### Dialect behaviour

| | `FOR UPDATE` | `FOR SHARE` | `FOR NO KEY UPDATE` | `FOR KEY SHARE` | `OF` | `NOWAIT` / `SKIP LOCKED` |
|---|---|---|---|---|---|---|
| **PostgreSQL** | `FOR UPDATE` | `FOR SHARE` | `FOR NO KEY UPDATE` | `FOR KEY SHARE` | Emits all tables (all modes) | Supported |
| **MySQL** | `FOR UPDATE` | `LOCK IN SHARE MODE` | Silently dropped | Silently dropped | Emitted for `FOR UPDATE`; dropped for `LOCK IN SHARE MODE` | Supported (8.0+) |
| **SQLite** | Silently dropped | Silently dropped | Silently dropped | Silently dropped | Silently dropped | Silently dropped |

See [Dialect comparison](../reference/dialects.md) for the full feature table.
