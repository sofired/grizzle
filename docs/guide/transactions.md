# Transactions

## Transaction callback

Pass a function to `db.Transaction`. If it returns a non-nil error, the transaction is automatically rolled back. On success, it is committed.

```go
err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
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
| `tx.Query(ctx, builder)` | Execute a SELECT builder, returns `pgx.Rows` |
| `tx.Exec(ctx, builder)` | Execute an INSERT/UPDATE/DELETE builder, returns rows affected |
| `tx.QueryRaw(ctx, sql, args...)` | Execute raw SQL, returns `pgx.Rows` |
| `tx.ExecRaw(ctx, sql, args...)` | Execute raw SQL, returns rows affected |

## Scanning within a transaction

Use the package-level scan helpers with rows returned by `tx.Query`:

```go
err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
    rows, err := tx.Query(ctx,
        query.Select(db.UsersT.ID, db.UsersT.Username).
            From(db.UsersT).
            Where(db.UsersT.RealmID.EQ(realmID)).
            Limit(100),
    )
    users, err := pgxdb.ScanAll[db.UserSelect](rows, err)
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

Any error returned from the callback triggers a rollback. The transaction error (if any) is wrapped and returned from `db.Transaction`:

```go
err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
    return myOperation(tx)
})
if err != nil {
    // Could be a DB error or whatever myOperation returned
    log.Printf("transaction failed: %v", err)
}
```
