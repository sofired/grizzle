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

## Grizzle Callback Shape — PARITY

The callback transaction shape below is implemented parity for PostgreSQL: begin a transaction, run the callback, commit on nil, and roll back on error. The stricter DB/Tx row-vs-exec validation, nil/typed-nil checks, redacted stable transaction errors, row ownership rules, isolation options, savepoints, and MySQL/SQLite wrappers are target implementation gaps until the driver packages satisfy the contracts below.

```go
err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
    _, err := tx.Exec(ctx, query.InsertInto(db.UsersT).Values(row))
    if err != nil {
        return err // triggers rollback
    }
    _, err = tx.Exec(ctx, query.Update(db.AccountsT).
        SetCol(db.AccountsT.Balance, newBalance).
        Where(db.AccountsT.UserID.EQ(id)),
    )
    return err // nil → commit; non-nil → rollback
})
```

`*pgxdb.Tx` exposes the same methods as `*pgxdb.DB`:

| Method | Description |
|---|---|
| `tx.Query(ctx, builder)` | Execute any row-returning builder, including SELECT and mutations with `RETURNING`, returns `pgx.Rows` |
| `tx.Exec(ctx, builder)` | Execute exec-only mutation builders without `RETURNING`, returns rows affected |
| `tx.QueryRaw(ctx, sql, args...)` | Execute raw SQL, returns `pgx.Rows` |
| `tx.ExecRaw(ctx, sql, args...)` | Execute raw SQL, returns rows affected |

Raw transaction methods follow the same trusted-input, parameterization, and redaction rules as [query-builder.md](./query-builder.md#raw-sql). Values must be passed through arguments, not interpolated into SQL text.

Target driver method shape:

```go
func (db *pgxdb.DB) Query(ctx context.Context, b query.RowBuilder) (pgx.Rows, error)
func (db *pgxdb.DB) Exec(ctx context.Context, b query.ExecBuilder) (int64, error)
func (db *pgxdb.DB) QueryRaw(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
func (db *pgxdb.DB) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error)

func (tx *pgxdb.Tx) Query(ctx context.Context, b query.RowBuilder) (pgx.Rows, error)
func (tx *pgxdb.Tx) Exec(ctx context.Context, b query.ExecBuilder) (int64, error)
func (tx *pgxdb.Tx) QueryRaw(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
func (tx *pgxdb.Tx) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error)
```

Execution rules:

- `Query` accepts row-returning builders only, including SELECT and mutation builders with `RETURNING`; non-row builders fail before SQL execution
- `Exec` accepts exec-only builders only; row-returning builders fail before SQL execution
- nil and typed-nil builders, DBs, or transactions fail before method calls with stable redacted errors
- driver helpers must call `Build(dialect)` and return build errors before sending SQL
- successful `Query` transfers `pgx.Rows` ownership to the caller, who must close rows or pass them to a `Scan*` helper that closes them
- raw SQL entrypoints treat the SQL string as trusted input only; values must be supplied through `args`, and errors/diagnostics must redact SQL text, bind values, credentials, and raw driver errors
- direct `Build(dialect)` is contextless; helpers that build and execute should check context before and after build, then preserve context cancellation/deadline sentinels from query, exec, row iteration, commit, rollback, or cleanup under the shared error contract

Error precedence rules:

- if the callback returns an error, `Transaction` must attempt rollback and return the callback error as the primary error
- if rollback also fails, the returned error must preserve the callback error for `errors.Is` / `errors.As` when it is safe to expose, and include the rollback failure only as a redacted secondary diagnostic
- if the callback returns nil and commit fails, `Transaction` returns a stable redacted commit error and must not report the transaction as successful
- if beginning the transaction fails, the callback must not run
- context cancellation/deadline from begin, callback operations, commit, rollback, or cleanup must preserve `errors.Is(err, context.Canceled)` / `errors.Is(err, context.DeadlineExceeded)` while exposing a stable transaction error code
- raw driver errors, SQL text, bind values, and credentials must not be recoverable through transaction error strings, logs, verbose diagnostics, `Unwrap()`, or `%+v`

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
err := db.Transaction(ctx, fn, pgxdb.TransactionOptions{
    IsolationLevel: pgx.RepeatableRead,
    AccessMode:     pgx.ReadOnly,
})
```

Not yet implemented. The current transaction wrapper uses the database's default isolation level.

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
err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
    return tx.Transaction(ctx, func(nested *pgxdb.Tx) error {
        // rollback here → ROLLBACK TO SAVEPOINT; outer tx continues
        return doSomething(nested)
    })
})
```

Not yet implemented. Tracked as **#143**.

## MySQL and SQLite

MySQL and SQLite transactions follow the same callback shape through their driver packages. The PostgreSQL implementation is exposed as `(*pgxdb.DB).Transaction` and uses `pgxpool` internally. Equivalent wrappers for MySQL and SQLite are **DEVIATION:GAP (designed)** — the API should be identical; only the underlying driver call differs.
