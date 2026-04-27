# Transactions Specification

## Drizzle equivalent

**Drizzle:**
```typescript
await db.transaction(async (tx) => {
  await tx.insert(users).values({ ... })
  await tx.update(accounts).set({ balance: newBalance }).where(eq(accounts.userId, id))
  // throws → automatic rollback
  // returns → automatic commit
})
```

Drizzle wraps the callback in a transaction and automatically commits on success or rolls back on any thrown error. The `tx` object exposes the same query API as `db`.

## Grizzle — PARITY

```go
err := pgxdb.Transaction(ctx, db, func(tx *pgxdb.Tx) error {
    _, err := tx.Exec(ctx, query.InsertInto(db.UsersT).Values(row))
    if err != nil {
        return err // triggers rollback
    }
    _, err = tx.Exec(ctx, query.Update(db.AccountsT).
        Set("balance", newBalance).
        Where(db.AccountsT.UserID.EQ(id)),
    )
    return err // nil → commit; non-nil → rollback
})
```

`*pgxdb.Tx` exposes the same methods as `*pgxdb.DB`:

| Method | Description |
|---|---|
| `tx.Query(ctx, builder)` | Execute a SELECT builder, returns `pgx.Rows` |
| `tx.Exec(ctx, builder)` | Execute INSERT/UPDATE/DELETE, returns rows affected |
| `tx.QueryRaw(ctx, sql, args...)` | Execute raw SQL, returns `pgx.Rows` |
| `tx.ExecRaw(ctx, sql, args...)` | Execute raw SQL, returns rows affected |

## Isolation levels — DEVIATION:GAP (designed)

**Drizzle:**
```typescript
await db.transaction(async (tx) => { ... }, {
  isolationLevel: 'read committed',  // or 'repeatable read', 'serializable'
  accessMode: 'read only',
  deferrable: true,
})
```

**Grizzle target:**
```go
err := pgxdb.Transaction(ctx, db, fn, pgxdb.TransactionOptions{
    IsolationLevel: pgx.RepeatableRead,
    AccessMode:     pgx.ReadOnly,
})
```

Not yet implemented. The `pgxdb.Transaction` wrapper currently uses the database's default isolation level.

## Nested transactions (savepoints) — DEVIATION:GAP (designed) — #143

**Drizzle:**
```typescript
await db.transaction(async (tx) => {
  await tx.transaction(async (nested) => {
    // internally uses a savepoint
    // throw in here → rollback to savepoint only
  })
})
```

**Grizzle target:** `Tx.Transaction(ctx, fn)` — a nested call that internally uses `SAVEPOINT` / `ROLLBACK TO SAVEPOINT` / `RELEASE SAVEPOINT`.

```go
err := pgxdb.Transaction(ctx, db, func(tx *pgxdb.Tx) error {
    return tx.Transaction(ctx, func(nested *pgxdb.Tx) error {
        // rollback here → ROLLBACK TO SAVEPOINT; outer tx continues
        return doSomething(nested)
    })
})
```

Not yet implemented. Tracked as **#143**.

## MySQL and SQLite

MySQL and SQLite transactions follow the same API via `database/sql`. The `pgxdb.Transaction` wrapper is PostgreSQL-specific (uses `pgxpool`). Equivalent wrappers for MySQL and SQLite are **DEVIATION:GAP (designed)** — the API should be identical; only the underlying driver call differs.
