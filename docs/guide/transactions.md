# Transactions

::: warning Target transaction API
The callback commit/rollback shape is implemented for PostgreSQL. Row-vs-exec validation, stable redacted transaction errors, nil/typed-nil checks, and row ownership rules below describe the target contract until the driver package fully implements them.
:::

## Transaction callback

Pass a function to `db.Transaction`. If it returns a non-nil error, the transaction is automatically rolled back. On success, it is committed.

String column names in mutation helpers must be compile-time literals or generated constants. Do not pass user input as a column name.

```go
err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
    cost := int64(10)
    credits := int64(100)

    // All builders in here share the same transaction
    _, err := tx.Exec(ctx,
        query.InsertInto(db.OrdersT).Values(newOrder),
    )
    if err != nil {
        return err // triggers rollback
    }

    _, err = tx.Exec(ctx,
        query.Update(db.UsersT).
            Set("credits", credits-cost).
            Where(db.UsersT.ID.EQ(userID)),
    )
    return err // commit if nil, rollback if error
})
```

## Tx methods

`*pgxdb.Tx` exposes the same methods as `*pgxdb.DB`:

| Method | Description |
|---|---|
| `tx.Query(ctx, builder)` | Execute row-returning builders, including SELECT and mutations with `RETURNING`, returns `pgx.Rows` |
| `tx.Exec(ctx, builder)` | Execute exec-only mutations without `RETURNING`, returns rows affected |
| `tx.QueryRaw(ctx, sql, args...)` | Execute raw SQL, returns `pgx.Rows` |
| `tx.ExecRaw(ctx, sql, args...)` | Execute raw SQL, returns rows affected |

Raw transaction SQL follows the same rules as non-transaction raw SQL: never concatenate user input into SQL text, pass values as bound arguments, and keep diagnostics redacted.

## Scanning within a transaction

Use the package-level scan helpers with rows returned by `tx.Query`:

```go
type UserSummary struct {
    ID       uuid.UUID `db:"id"`
    Username string    `db:"username"`
}

err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
    rows, err := tx.Query(ctx,
        query.Select(db.UsersT.ID, db.UsersT.Username).
            From(db.UsersT).
            Where(db.UsersT.RealmID.EQ(realmID)).
            Limit(100),
    )
    users, err := pgxdb.ScanAll[UserSummary](rows, err)
    if err != nil {
        return err
    }

    for _, u := range users {
        _, err = tx.Exec(ctx,
            query.Update(db.UsersT).
                Set("enabled", false).
                Where(db.UsersT.ID.EQ(u.ID)),
        )
        if err != nil {
            return err
        }
    }
    return nil
})
```

## Error handling

Any error returned from the callback triggers a rollback. If rollback also fails, the callback error remains the primary error and rollback failure is exposed only as a redacted secondary diagnostic. If the callback succeeds but commit fails, `db.Transaction` returns a stable commit error and the transaction must not be treated as successful:

```go
err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
    return myOperation(tx)
})
if err != nil {
    // Keep raw database errors out of logs; expose a stable code/classifier instead.
    log.Printf("transaction failed")
}
```
