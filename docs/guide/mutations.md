# Mutations

::: warning Target API status
This page describes the target mutation API for the RC.1 parity work. Some examples use APIs that are specified but not fully implemented yet, including `query.Assign[T]`, `SetStruct`, `DoUpdateSetStruct`, and optional shared conflict helpers. Check [spec/README.md](../spec/README.md) before treating an example as current branch behavior.
:::

## INSERT

Use a db-tagged struct to supply values. Optional non-null/defaulted fields use pointers with `omitempty`. Nullable assignable fields use `query.Assign[T]` so callers can distinguish omitted fields from explicit SQL `NULL`:

When examples use string column names in `Set` or optional string-name assignment helpers, those strings must be compile-time literals or generated constants. The builder must quote and validate them as single identifiers; never pass user input as a column name. Conflict examples use generated column handles through `query.ConflictColumn` and `query.SetValue`.

```go
type UserInsert struct {
    ID        *uuid.UUID          `db:"id,omitempty"`      // optional — DEFAULT gen_random_uuid()
    RealmID   uuid.UUID           `db:"realm_id"`           // required
    Username  string              `db:"username"`            // required
    Email     query.Assign[string] `db:"email"`              // nullable — unset/value/null
    Enabled   *bool               `db:"enabled,omitempty"`   // optional — DEFAULT true
}

row := UserInsert{
    RealmID:  realmID,
    Username: "alice",
    Email:    query.Unset[string](),
}

sql, args, err := query.InsertInto(db.UsersT).
    Values(row).
    Returning(db.UsersT.ID, db.UsersT.CreatedAt).
    Build(dialect.Postgres)
// INSERT INTO "users" (...) VALUES (... dialect defaults for omitted fields ...)
// RETURNING "users"."id", "users"."created_at"
```

### Multiple rows

```go
// Pass multiple structs in one call
qMulti := query.InsertInto(db.UsersT).
    Values(row1, row2)

// Or pass a slice
qSlice := query.InsertInto(db.UsersT).ValueSlice(rows)
```

### Executing an INSERT

```go
// With RETURNING — scan the returned rows
type InsertedUserID struct {
    ID uuid.UUID `db:"id"`
}

rows, err := d.Query(ctx,
    query.InsertInto(db.UsersT).Values(row).Returning(db.UsersT.ID),
)
result, err := pgxdb.ScanOne[InsertedUserID](rows, err)

// Without RETURNING — just get the row count
n, err := d.Exec(ctx, query.InsertInto(db.UsersT).Values(row))
```

## UPSERT

### PostgreSQL / SQLite — ON CONFLICT

```go
// Conflict on column list → DO UPDATE SET excluded columns
query.InsertInto(db.UsersT).
    Values(row).
    OnConflict(query.ConflictColumn(db.UsersT.RealmID), query.ConflictColumn(db.UsersT.Username)).
    DoUpdateSetExcluded(db.UsersT.Email, db.UsersT.Enabled)
// ON CONFLICT ("realm_id", "username")
// DO UPDATE SET "email" = EXCLUDED."email", "enabled" = EXCLUDED."enabled"

// Explicit SET values on conflict
query.InsertInto(db.UsersT).
    Values(row).
    OnConflict(query.ConflictColumn(db.UsersT.Email)).
    DoUpdateSet(
        query.SetValue(db.UsersT.Enabled, true),
        query.SetValue(db.UsersT.UpdatedAt, time.Now()),
    )

// Struct-based SET on conflict
enabled := true
query.InsertInto(db.UsersT).
    Values(row).
    OnConflict(query.ConflictColumn(db.UsersT.Email)).
    DoUpdateSetStruct(UserUpdate{Enabled: &enabled})
```

### Grizzle-only / future constraint targets

Drizzle RC.1 PostgreSQL conflict targets are column-based; SQLite also accepts trusted SQL conflict-target expressions. A named-constraint conflict helper such as `OnConflictConstraint("users_realm_username_idx")` is not RC.1 parity and must stay out of the initial parity path unless it is separately implemented and labeled as a Grizzle-only extension.

### Dialect-specific ignore helpers

```go
// Skip conflicting rows without returning an error. On MySQL, INSERT IGNORE
// can also downgrade broader constraint/data problems to warnings.
// MySQL: INSERT IGNORE INTO ... (assumes a generated MySQL schema package)
query.MySQLInsertInto(mysqlschema.UsersT).Ignore().Values(row)
```

::: info Dialect differences
`ON CONFLICT` is PostgreSQL / SQLite syntax and must not be presented as MySQL parity. MySQL upsert uses a separate `ON DUPLICATE KEY UPDATE` builder with no conflict target:

```go
query.MySQLInsertInto(mysqlschema.UsersT).
    Values(row).
    OnDuplicateKeyUpdateSet(query.MySQLSetColValue(mysqlschema.UsersT.Email, "alice@example.com"))

// MySQL no-op conflict handling:
query.MySQLInsertInto(mysqlschema.UsersT).
    Values(row).
    OnDuplicateKeyUpdateSet(query.MySQLSetColSelf(mysqlschema.UsersT.ID))
```

`IgnoreConflicts()` is an optional shared wrapper. If retained, it must render `ON CONFLICT DO NOTHING` for PostgreSQL and SQLite, `INSERT IGNORE` for MySQL, and a build error for unsupported/custom dialects.
:::

::: warning
Ignore helpers can hide data-quality or integrity problems. Use them only when skipped rows are expected and observable through row counts, diagnostics, or application-level reconciliation.
:::

## UPDATE

### Explicit SET

```go
sql, args, err := query.Update(db.UsersT).
    Set("email", "new@example.com").
    Set("updated_at", time.Now()).
    Where(db.UsersT.ID.EQ(userID)).
    Build(dialect.Postgres)
// UPDATE "users" SET "email" = $1, "updated_at" = $2 WHERE "users"."id" = $3
```

### Struct-based SET

Use a struct where non-null assignable fields are pointers and nullable assignable fields use `query.Assign[T]`. Nil pointers and unset assignments are skipped for UPDATE/UPSERT SET structs; `omitempty` is not required for update omission:

```go
type UserUpdate struct {
    Email     query.Assign[string]    `db:"email"`
    Enabled   *bool                   `db:"enabled"`
    DeletedAt query.Assign[time.Time] `db:"deleted_at"`
    UpdatedAt *time.Time              `db:"updated_at"`
}

now := time.Now()
sql, args, err := query.Update(db.UsersT).
    SetStruct(UserUpdate{
        DeletedAt: query.Value(now),
        UpdatedAt: &now,
    }).
    Where(db.UsersT.ID.EQ(userID)).
    Returning(db.UsersT.UpdatedAt).
    Build(dialect.Postgres)
// UPDATE "users" SET "deleted_at" = $1, "updated_at" = $2
// WHERE "users"."id" = $3 RETURNING "users"."updated_at"
```

### Executing an UPDATE

```go
// With RETURNING
type UpdatedTimestamp struct {
    UpdatedAt time.Time `db:"updated_at"`
}

rows, err := d.Query(ctx,
    query.Update(db.UsersT).
        SetStruct(update).
        Where(db.UsersT.ID.EQ(userID)).
        Returning(db.UsersT.UpdatedAt),
)
result, err := pgxdb.ScanOne[UpdatedTimestamp](rows, err)

// Without RETURNING
n, err := d.Exec(ctx,
    query.Update(db.UsersT).Set("enabled", false).Where(db.UsersT.ID.EQ(userID)),
)
```

## DELETE

```go
sql, args, err := query.DeleteFrom(db.UsersT).
    Where(db.UsersT.ID.EQ(userID)).
    Build(dialect.Postgres)
// DELETE FROM "users" WHERE "users"."id" = $1
```

### With RETURNING

```go
rows, err := d.Query(ctx,
    query.DeleteFrom(db.UsersT).
        Where(db.UsersT.DeletedAt.IsNotNull()).
        Returning(db.UsersT.ID, db.UsersT.Username),
)
type DeletedUser struct {
    ID       uuid.UUID `db:"id"`
    Username string    `db:"username"`
}

deleted, err := pgxdb.ScanAll[DeletedUser](rows, err)
```

## RETURNING clause

`RETURNING` is supported on INSERT, UPDATE, and DELETE for PostgreSQL and SQLite. MySQL does not have normal `RETURNING` parity; Grizzle must reject or omit that API for MySQL rather than silently dropping it.

```go
// Any selectable column can be returned
.Returning(db.UsersT.ID, db.UsersT.CreatedAt, db.UsersT.UpdatedAt)
```
