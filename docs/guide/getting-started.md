# Getting Started

## Installation

Add Grizzle to your Go module:

```sh
go get github.com/sofired/grizzle
```

Install the CLI (used for code generation and migrations):

```sh
go install github.com/sofired/grizzle/cmd/grizzle@latest
```

## How it works

Grizzle has three layers:

| Layer | Package | What it does |
|---|---|---|
| Schema DSL | `schema/pg` | Declare tables and columns in Go |
| Query builders | `query`, `expr` | Build type-safe SQL — returns `(string, []any)` |
| Driver adapter | `driver/pgx` | Execute builders against a `pgxpool.Pool` |

Code generation bridges the first two layers: `grizzle gen` reads your `schema/pg` declarations and emits typed table handles (`UsersT`, `RealmsT`, …) that the query builders consume.

## 1. Define your schema

Create a `db/schema.go` file using the `schema/pg` DSL:

```go
package db

import pg "github.com/sofired/grizzle/schema/pg"

var Realms = pg.Table("realms",
    pg.C("id",           pg.UUID().PrimaryKey().DefaultRandom()),
    pg.C("name",         pg.Varchar(255).NotNull()),
    pg.C("enabled",      pg.Boolean().NotNull().Default(true)),
    pg.C("created_at",   pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
    return []pg.Constraint{
        pg.UniqueIndex("realms_name_idx").On(t.Col("name")).Build(),
    }
})

var Users = pg.Table("users",
    pg.C("id",         pg.UUID().PrimaryKey().DefaultRandom()),
    pg.C("realm_id",   pg.UUID().NotNull().References("realms", "id", pg.OnDelete(pg.FKActionRestrict))),
    pg.C("username",   pg.Varchar(255).NotNull()),
    pg.C("email",      pg.Varchar(255)),
    pg.C("enabled",    pg.Boolean().NotNull().Default(true)),
    pg.C("created_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
    pg.C("deleted_at", pg.Timestamp().WithTimezone()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
    return []pg.Constraint{
        pg.UniqueIndex("users_realm_username_idx").
            On(t.Col("realm_id"), t.Col("username")).
            Where(pg.IsNull(t.Col("deleted_at"))).
            Build(),
    }
})
```

See [Schema DSL](/guide/schema) for the full column and constraint reference.

## 2. Generate table handles

```sh
grizzle gen --schema db/schema.go --out db/gen.go --package db
```

This produces typed table handles in `db/gen.go`:

```go
// db/gen.go (generated — do not edit)

type UsersTable struct {
    ID        expr.UUIDColumn
    RealmID   expr.UUIDColumn
    Username  expr.StringColumn
    Email     expr.StringColumn
    Enabled   expr.BoolColumn
    CreatedAt expr.TimestampColumn
    DeletedAt expr.TimestampColumn
}

func (UsersTable) GRizTableName() string  { return "users" }
func (UsersTable) GRizTableAlias() string { return "users" }

var UsersT = UsersTable{
    ID:        expr.UUIDColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "id"}},
    // ...
}

// Also generated: UserSelect, UserInsert, UserUpdate structs
```

Re-run `grizzle gen` whenever you change your schema.

## 3. Connect to your database

```go
package main

import (
    "context"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
    pgxdb "github.com/sofired/grizzle/driver/pgx"
)

func main() {
    pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    db := pgxdb.New(pool)
    _ = db
}
```

## 4. Run your first query

```go
import (
    "context"
    "fmt"

    pgxdb "github.com/sofired/grizzle/driver/pgx"
    "github.com/sofired/grizzle/query"
    "myapp/db"
)

func listActiveUsers(ctx context.Context, d *pgxdb.DB) ([]db.UserSelect, error) {
    return pgxdb.FromSelect[db.UserSelect](ctx, d,
        query.Select(db.UsersT.ID, db.UsersT.Username, db.UsersT.Email).
            From(db.UsersT).
            Where(db.UsersT.DeletedAt.IsNull()).
            OrderBy(db.UsersT.Username.Asc()).
            Limit(50),
    )
}
```

## Next steps

- [Schema DSL](/guide/schema) — column types, constraints, foreign keys
- [Querying](/guide/querying) — WHERE, JOIN, ORDER BY, pagination
- [Mutations](/guide/mutations) — INSERT, UPDATE, DELETE, UPSERT
