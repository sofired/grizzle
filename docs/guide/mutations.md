# Mutations

## INSERT

Use a db-tagged struct to supply values. Fields tagged `omitempty` are omitted when nil or zero:

```go
type UserInsert struct {
    ID        *uuid.UUID `db:"id,omitempty"`       // optional — DEFAULT gen_random_uuid()
    RealmID   uuid.UUID  `db:"realm_id"`            // required
    Username  string     `db:"username"`             // required
    Email     *string    `db:"email,omitempty"`      // optional
    Enabled   *bool      `db:"enabled,omitempty"`    // optional — DEFAULT true
}

row := UserInsert{
    RealmID:  realmID,
    Username: "alice",
}

sql, args := query.InsertInto(db.UsersT).
    Values(row).
    Returning(db.UsersT.ID, db.UsersT.CreatedAt).
    Build(dialect.Postgres)
// INSERT INTO "users" ("realm_id", "username") VALUES ($1, $2)
// RETURNING "users"."id", "users"."created_at"
```

### Multiple rows

```go
// Pass structs one at a time
q := query.InsertInto(db.UsersT).
    Values(row1).
    Values(row2)

// Or pass a slice
q := query.InsertInto(db.UsersT).ValueSlice(rows)
```

### Executing an INSERT

```go
// With RETURNING — scan the returned rows
rows, err := d.Query(ctx,
    query.InsertInto(db.UsersT).Values(row).Returning(db.UsersT.ID),
)
result, err := pgxdb.ScanOne[db.UserSelect](rows, err)

// Without RETURNING — just get the row count
n, err := d.Exec(ctx, query.InsertInto(db.UsersT).Values(row))
```

## UPSERT

### PostgreSQL — ON CONFLICT

```go
// Conflict on column list → DO UPDATE SET excluded columns
query.InsertInto(db.UsersT).
    Values(row).
    OnConflict("realm_id", "username").
    DoUpdateSetExcluded("email", "enabled")
// ON CONFLICT ("realm_id", "username")
// DO UPDATE SET "email" = EXCLUDED."email", "enabled" = EXCLUDED."enabled"

// Conflict on named constraint
query.InsertInto(db.UsersT).
    Values(row).
    OnConflictConstraint("users_realm_username_idx").
    DoNothing()

// Explicit SET values on conflict
query.InsertInto(db.UsersT).
    Values(row).
    OnConflict("email").
    DoUpdateSet("enabled", true).
    DoUpdateSet("updated_at", time.Now())

// Struct-based SET on conflict
query.InsertInto(db.UsersT).
    Values(row).
    OnConflict("email").
    DoUpdateSetStruct(UserUpdate{Enabled: ptr(true)})
```

### MySQL / SQLite — INSERT IGNORE

```go
// Silently skip rows that violate unique/PK constraints
// MySQL:  INSERT IGNORE INTO ...
// SQLite: INSERT OR IGNORE INTO ...
query.InsertInto(db.UsersT).Values(row).IgnoreConflicts()
```

::: info Dialect differences
`ON CONFLICT` is PostgreSQL / SQLite syntax. MySQL uses `ON DUPLICATE KEY UPDATE`, which is emitted automatically when the MySQL dialect is active. `IgnoreConflicts()` emits `INSERT IGNORE` for MySQL and `INSERT OR IGNORE` for SQLite.
:::

## UPDATE

### Explicit SET

```go
sql, args := query.Update(db.UsersT).
    Set("email", "new@example.com").
    Set("updated_at", time.Now()).
    Where(db.UsersT.ID.EQ(userID)).
    Build(dialect.Postgres)
// UPDATE "users" SET "email" = $1, "updated_at" = $2 WHERE "users"."id" = $3
```

### Struct-based SET

Use a struct where all fields are pointers. Nil fields are skipped — only non-nil fields are included in the SET clause:

```go
type UserUpdate struct {
    Email     *string    `db:"email"`
    Enabled   *bool      `db:"enabled"`
    DeletedAt *time.Time `db:"deleted_at"`
    UpdatedAt *time.Time `db:"updated_at"`
}

now := time.Now()
sql, args := query.Update(db.UsersT).
    SetStruct(UserUpdate{DeletedAt: &now, UpdatedAt: &now}).
    Where(db.UsersT.ID.EQ(userID)).
    Returning(db.UsersT.UpdatedAt).
    Build(dialect.Postgres)
// UPDATE "users" SET "deleted_at" = $1, "updated_at" = $2
// WHERE "users"."id" = $3 RETURNING "users"."updated_at"
```

### Executing an UPDATE

```go
// With RETURNING
rows, err := d.Query(ctx,
    query.Update(db.UsersT).
        SetStruct(update).
        Where(db.UsersT.ID.EQ(userID)).
        Returning(db.UsersT.UpdatedAt),
)
result, err := pgxdb.ScanOne[db.UserSelect](rows, err)

// Without RETURNING
n, err := d.Exec(ctx,
    query.Update(db.UsersT).Set("enabled", false).Where(db.UsersT.ID.EQ(userID)),
)
```

## DELETE

```go
sql, args := query.DeleteFrom(db.UsersT).
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
deleted, err := pgxdb.ScanAll[db.UserSelect](rows, err)
```

## RETURNING clause

`RETURNING` is supported on INSERT, UPDATE, and DELETE for PostgreSQL and SQLite. It is silently dropped for MySQL.

```go
// Any selectable column can be returned
.Returning(db.UsersT.ID, db.UsersT.CreatedAt, db.UsersT.UpdatedAt)
```
