# Getting Started

## Installation

**Requires Go 1.25 or later.**

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
| Query builders | `query`, `expr` | Build type-safe SQL. Target API: `Build(dialect)` returns `(string, []any, error)`; current branch may still expose the older two-return shape. |
| Driver adapter | `driver/pgx` | Target behavior: execute builders against a `pgxpool.Pool` and surface build errors before execution after the error-returning `Build` contract lands |

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
            Build(),
    }
})
```

See [Schema DSL](/guide/schema) for the full column and constraint reference.

## 2. Generate table handles

```sh
grizzle gen --schema ./db --out ./db --package db
```

This produces typed table-handle files in `./db`, for example `users_gen.go`:

```go
// db/users_gen.go (generated — do not edit)

type UsersTable struct {
    ID        expr.UUIDColumn
    RealmID   expr.UUIDColumn
    Username  expr.StringColumn
    Email     expr.StringColumn
    Enabled   expr.BoolColumn
    CreatedAt expr.TimestampColumn
    DeletedAt expr.TimestampColumn
}

func (UsersTable) GrizTableRef() query.TableRef {
    return query.TableRef{Name: "users"}
}

var UsersT = UsersTable{
    // Generated typed column handles initialized with sqlmeta.ColumnMeta.
}

// Also generated: UserSelect, UserInsert, UserUpdate structs
```

Re-run `grizzle gen` whenever you change your schema.

## 3. Connect to your database

```go
package main

import (
	"context"
	"log"
	"os"
	"time"

    "github.com/jackc/pgx/v5/pgxpool"
    pgxdb "github.com/sofired/grizzle/driver/pgx"
)

func main() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatal("connect database failed")
    }
    defer pool.Close()
    if err := pool.Ping(ctx); err != nil {
        log.Fatal("connect database failed")
    }

    db := pgxdb.New(pool)
    _ = db
}
```

## 4. Run your first query

```go
import (
    "context"

    pgxdb "github.com/sofired/grizzle/driver/pgx"
    "github.com/sofired/grizzle/query"
    "myapp/db"
)

func listActiveUsers(ctx context.Context, d *pgxdb.DB) ([]db.UserSelect, error) {
    return pgxdb.FromSelect[db.UserSelect](ctx, d,
        query.Select().
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
