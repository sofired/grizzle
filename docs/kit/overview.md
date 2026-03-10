# Migration Kit

The `kit` package provides database migration tooling — schema introspection, diff computation, SQL generation, and migration history — all driven by the same Go schema definitions you use for queries.

## How it works

1. **Introspect** the live database schema
2. **Diff** it against your `schema/pg` table definitions
3. **Generate** the SQL needed to bring the live schema up to date
4. **Apply** that SQL, optionally recording history

Because the schema source of truth is Go code (not separate migration files), there is no drift between your query builders and your database columns.

## Push

`kit.Push` compares the live schema to your table definitions and applies all required DDL in a single transaction. It does **not** record migration history.

```go
import (
    "github.com/sofired/grizzle/kit"
    db "your-module/schema"
)

result, err := kit.Push(ctx, pool, db.UsersT, db.RealmsT, db.PostsT)
if err != nil {
    log.Fatal(err)
}
for _, stmt := range result.SQL {
    fmt.Println(stmt)
}
```

Use `Push` in development or CI environments where you want an always-up-to-date schema without tracking history.

## DryRun

`kit.DryRun` computes the required changes and returns the SQL without applying it:

```go
result, err := kit.DryRun(ctx, pool, db.UsersT, db.RealmsT)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Pending changes:", len(result.Changes))
for _, stmt := range result.SQL {
    fmt.Println(stmt)
}
```

## Migrate (with history)

`kit.Migrate` is like `Push` but records every applied SQL batch in a `_grizzle_migrations` history table. Calling `Migrate` twice with an unchanged schema is a safe no-op.

```go
result, err := kit.Migrate(ctx, pool, db.UsersT, db.RealmsT, db.PostsT)
if err != nil {
    log.Fatal(err)
}
if result.AlreadyCurrent {
    fmt.Println("schema is up to date")
} else {
    fmt.Printf("applied %d change(s) — checksum %s\n", len(result.Changes), result.Checksum)
}
```

The `_grizzle_migrations` table is created automatically on first use.

## Status

`kit.Status` shows applied migration history and any pending changes without modifying the database:

```go
status, err := kit.Status(ctx, pool, db.UsersT, db.RealmsT)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("applied migrations: %d\n", len(status.Applied))
for _, r := range status.Applied {
    fmt.Printf("  [%s] %s\n", r.AppliedAt.Format(time.RFC3339), r.Description)
}
fmt.Printf("pending changes: %d\n", len(status.Pending))
```

## Generating SQL without a live DB

`kit.GenerateCreateSQL` generates the full `CREATE TABLE` SQL from table definitions — no database connection needed:

```go
sql := kit.GenerateCreateSQL(db.UsersT, db.RealmsT)
fmt.Println(sql)
// CREATE TABLE "users" ( ... );
//
// CREATE TABLE "realms" ( ... );
```

Useful for seeding a new database, writing integration test fixtures, or auditing the expected schema.

## Change types

The diff engine detects and generates SQL for:

| Change | DDL generated |
|---|---|
| New table | `CREATE TABLE …` |
| Dropped table | `DROP TABLE IF EXISTS …` |
| New column | `ALTER TABLE … ADD COLUMN …` |
| Dropped column | `ALTER TABLE … DROP COLUMN …` |
| New index | `CREATE [UNIQUE] INDEX …` |
| Dropped index | `DROP INDEX …` |

::: warning Column type changes
The differ does not currently generate `ALTER COLUMN … TYPE` statements. Rename columns or change their types with a manual migration when needed.
:::

## `grizzle gen` — code generation

The CLI companion to the migration kit generates typed Go table definitions from an existing database schema:

```sh
# Install the CLI
go install github.com/sofired/grizzle/cmd/grizzle@latest

# Generate schema code from a live PostgreSQL database
grizzle gen \
  --dsn "postgres://user:pass@localhost/mydb" \
  --out ./schema/db
```

The generated file contains `TableDef` values, typed column handles, and `Insert` / `Select` structs ready for use with `pgxdb.FromSelect` and `pgxdb.ScanAll`.

See [Getting Started](/guide/getting-started) for a full walkthrough.
