# Code Generation Specification

## `grizzle gen` — typed Go structs from schema definitions

`grizzle gen` reads Go schema definition files (containing `pg.Table(...)`, `mysql.Table(...)`, etc.) and emits `*_gen.go` files with typed Go structs and column handles.

This is **not** equivalent to Drizzle's `pull` command. Drizzle does not have a `gen` equivalent because TypeScript infers column types from the schema definition at compile time — no generation step is needed. `grizzle gen` exists because Go's type system cannot perform the same inference. This is **DEVIATION:LANGUAGE**.

```sh
grizzle gen --schema ./schema --out ./schema --package schema
```

### What it generates

For each table definition, `grizzle gen` produces four things:

**1. `<Table>Table` struct** — typed column handles for use in queries

```go
type UsersTable struct {
    ID        expr.UUIDColumn
    RealmID   expr.UUIDColumn
    Username  expr.StringColumn
    Email     expr.StringColumn
    Enabled   expr.BoolColumn
    CreatedAt expr.TimestampColumn
    DeletedAt expr.TimestampColumn
}

var UsersT = UsersTable{
    ID:        expr.NewUUIDColumn("users", "id"),
    RealmID:   expr.NewUUIDColumn("users", "realm_id"),
    Username:  expr.NewStringColumn("users", "username"),
    Email:     expr.NewStringColumn("users", "email"),
    Enabled:   expr.NewBoolColumn("users", "enabled"),
    CreatedAt: expr.NewTimestampColumn("users", "created_at"),
    DeletedAt: expr.NewTimestampColumn("users", "deleted_at"),
}
```

**2. `<Table>Select` struct** — for scanning query results; nullable columns are pointer types

```go
type UserSelect struct {
    ID        uuid.UUID  `db:"id"`
    RealmID   uuid.UUID  `db:"realm_id"`
    Username  string     `db:"username"`
    Email     *string    `db:"email"`       // nullable → pointer
    Enabled   bool       `db:"enabled"`
    CreatedAt time.Time  `db:"created_at"`
    DeletedAt *time.Time `db:"deleted_at"`  // nullable → pointer
}
```

**3. `<Table>Insert` struct** — for INSERT statements; required fields are plain types, optional fields are pointers with `omitempty`

```go
type UserInsert struct {
    ID        *uuid.UUID `db:"id,omitempty"`        // has DefaultRandom → optional
    RealmID   uuid.UUID  `db:"realm_id"`             // required
    Username  string     `db:"username"`              // required
    Email     *string    `db:"email,omitempty"`       // nullable → optional
    Enabled   *bool      `db:"enabled,omitempty"`     // has Default(true) → optional
    CreatedAt *time.Time `db:"created_at,omitempty"`  // has DefaultNow → optional
}
```

**4. `<Table>Update` struct** — for UPDATE statements; all fields are pointers so nil fields are omitted from the SET clause

```go
type UserUpdate struct {
    Username  *string    `db:"username"`
    Email     *string    `db:"email"`
    Enabled   *bool      `db:"enabled"`
    DeletedAt *time.Time `db:"deleted_at"`
}
```

Note: the primary key is excluded from `<Table>Update` because it is passed separately in the WHERE clause.

### Status

Implemented for PostgreSQL, MySQL, and SQLite.

### Known limitations

**`json:` struct tags are not generated** — the generator emits only `db:` struct tags. The examples in this file reflect that: all generated structs use only `db:` tags. If your application serialises generated structs to JSON, add `json:` tags manually or embed them in a wrapper struct. A `--json-tags` flag is not yet planned.

---

## Go type mappings

| SQL type | Go type | Notes |
|---|---|---|
| `uuid` | `uuid.UUID` | `github.com/google/uuid` |
| `varchar(n)`, `text`, `char(n)` | `string` | |
| `boolean` | `bool` | |
| `integer`, `serial` | `int32` | |
| `bigint`, `bigserial` | `int64` | |
| `numeric(p,s)` | `string` | Default; avoids precision loss. Configurable — DEVIATION:GAP (not designed) |
| `real` | `float32` | Type not yet in schema DSL — DEVIATION:GAP |
| `double precision` | `float64` | Type not yet in schema DSL — DEVIATION:GAP |
| `timestamp [with time zone]` | `time.Time` | |
| `date` | `time.Time` | Type not yet in schema DSL — DEVIATION:GAP |
| `json`, `jsonb` | `json.RawMessage` or custom type | Configurable via `.Type("T")` |
| `bytea` | `[]byte` | Type not yet in schema DSL — DEVIATION:GAP |
| `inet` | `netip.Addr` | Type not yet in schema DSL — DEVIATION:GAP |
| `interval` | `time.Duration` (approximate) | Type not yet in schema DSL — DEVIATION:GAP; full mapping TBD |
| enum | `string` or generated type | Type not yet in schema DSL — DEVIATION:GAP |
| array | `[]T` | Type not yet in schema DSL — DEVIATION:GAP |

---

## `grizzle pull` — schema definitions from live DB — DEVIATION:GAP (not designed)

`grizzle pull` is a separate command from `grizzle gen`. It connects to a live database, introspects the schema, and generates Go schema definition files (`pg.Table(...)` builder calls).

```sh
grizzle pull --db <dsn> --out ./schema/db [--dialect postgres|mysql|sqlite]
```

This is the Go equivalent of `drizzle-kit pull`. It is the inverse of `grizzle generate` (schema → SQL files), not the inverse of `grizzle gen` (schema definitions → typed structs).

Introspection exists internally in `kit/introspect`. The pull command — which translates introspection output into Go schema builder calls — is not yet implemented.

**Target output example.** Given a live PostgreSQL table, `grizzle pull` should produce:

```go
// users_gen.go — generated by grizzle pull; edit to taste then commit
package schema

import pg "github.com/sofired/grizzle/schema/pg"

var Users = pg.Table("users",
    pg.C("id",         pg.UUID().PrimaryKey().DefaultRandom().NotNull()),
    pg.C("username",   pg.Varchar(255).NotNull()),
    pg.C("email",      pg.Varchar(255)),
    pg.C("enabled",    pg.Boolean().NotNull().Default(true)),
    pg.C("created_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
    pg.C("deleted_at", pg.Timestamp().WithTimezone()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
    return []pg.Constraint{
        pg.UniqueIndex("users_username_idx").On(t.Col("username")).Build(),
    }
})
```

The generated file should carry a header comment indicating it was produced by `grizzle pull` and may be edited freely — it is not regenerated on subsequent `pull` runs unless `--force` is specified.
