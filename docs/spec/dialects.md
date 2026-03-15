# Dialects Specification

Grizzle's dialect system is the Go equivalent of [Drizzle's multi-dialect support](https://orm.drizzle.team/docs/get-started).

## Supported dialects

| Dialect | Drizzle package | Grizzle package | Status |
|---|---|---|---|
| PostgreSQL | `drizzle-orm/pg-core` | `schema/pg`, `dialect.Postgres` | Core dialect; most complete |
| MySQL / MariaDB | `drizzle-orm/mysql-core` | `schema/mysql`, `dialect.MySQL` | Partial — see column type gaps in [schema.md](./schema.md) |
| SQLite | `drizzle-orm/sqlite-core` | `schema/sqlite`, `dialect.SQLite` | Partial — see column type gaps in [schema.md](./schema.md) |
| CockroachDB | Uses `pg-core` | Use `dialect.Postgres` | PARITY — no additional changes needed |
| Neon, Supabase | Use `pg-core` | Use `dialect.Postgres` | Not verified; assumed PARITY |
| Turso (libSQL) | Uses `sqlite-core` | Use `dialect.SQLite` | Not verified |

## Dialect feature matrix

| Feature | PostgreSQL | MySQL | SQLite |
|---|---|---|---|
| Placeholders | `$1`, `$2`, … | `?` | `?` |
| Identifier quoting | `"name"` | `` `name` `` | `"name"` |
| `RETURNING` | Yes | No (silently dropped) | Yes (3.35+) |
| Upsert | `ON CONFLICT … DO UPDATE` | `ON DUPLICATE KEY UPDATE` | `ON CONFLICT … DO UPDATE` |
| Insert ignore | `ON CONFLICT … DO NOTHING` | `INSERT IGNORE` | `INSERT OR IGNORE` |
| `WITH` (CTE) | Yes | Yes (8.0+) | Yes (3.35+) |
| Window functions | Yes | Yes (8.0+) | Yes (3.25+) |
| `DISTINCT ON` | Yes | No | No |
| `FOR UPDATE` / `FOR SHARE` | Yes | Yes (limited) | No |
| `FOR NO KEY UPDATE` / `FOR KEY SHARE` | Yes | **No** | No |
| `FULL JOIN` | Yes | No | No |
| JSON operators | `->`, `->>`, `@>`, etc. | Limited | Limited |

## Dialect interface

All query builder operations must route dialect-specific SQL through the `Dialect` interface — no dialect name checks (`if d.Name() == "postgres"`) inside query builder code.

```go
type Dialect interface {
    Placeholder(n int) string       // "$1" (Postgres) or "?" (MySQL/SQLite)
    QuoteIdent(name string) string  // `"name"` or `` `name` ``
    Name() string                   // "postgres", "mysql", "sqlite"
    SupportsReturning() bool
    UpsertStyle() UpsertStyle
    InsertIgnoreClause() string
}
```

**Target interface — DEVIATION:GAP (designed).** The interface needs additional feature-detection methods to prevent PostgreSQL-only clauses leaking into other dialects (see bug #110). The full target interface:

```go
type Dialect interface {
    Placeholder(n int) string
    QuoteIdent(name string) string
    Name() string
    SupportsReturning() bool
    UpsertStyle() UpsertStyle
    InsertIgnoreClause() string
    // To be added:
    SupportsCTE() bool
    SupportsWindowFunctions() bool
    SupportsDistinctOn() bool
    SupportsForUpdate() bool                // FOR UPDATE / FOR SHARE
    SupportsForNoKeyUpdate() bool           // PostgreSQL-only; false for MySQL/SQLite
    SupportsFullJoin() bool
}
```

## Driver parity

| Drizzle driver | Grizzle equivalent | Status |
|---|---|---|
| `drizzle-orm/node-postgres` | `driver/pgx` with `pgxpool.Pool` | PARITY |
| `drizzle-orm/postgres-js` | No equivalent | DEVIATION:GAP (not designed) |
| `drizzle-orm/mysql2` | `database/sql` + `go-sql-driver/mysql` | Partial |
| `drizzle-orm/better-sqlite3` | `database/sql` + `mattn/go-sqlite3` | Partial |
| Connection pool config | Via pgx / `database/sql` pool config | PARITY |

## Known bug

**#110** — `FOR NO KEY UPDATE` and `FOR KEY SHARE` are PostgreSQL-only locking clauses. They currently emit for MySQL as well, producing invalid SQL. Fix requires adding `SupportsForNoKeyUpdate()` to the `Dialect` interface and guarding the locking clause SQL generation behind it.

## PostgreSQL-specific features

### JSONB operators — DEVIATION:GAP (not designed)

`->`, `->>`, `@>`, `<@`, `?`, `?|`, `?&`, `#>`, `#-`. Tracked as part of #140.

### Full-text search — DEVIATION:GAP (designed)

`tsvector`, `tsquery`, `@@`, `to_tsvector()`, `to_tsquery()`, `plainto_tsquery()`. Tracked as #140.

### Array operators — DEVIATION:GAP (not designed)

`ANY`, `ALL`, `@>`, `<@`, `&&`, array subscripting. Tracked as #144.

## `*pg.TableDef` leak across dialects — FIXED (issue #156)

**Previously DEVIATION:BROKEN — resolved in PR #156.**

### Problem

The CLI parser (`cmd/grizzle/main.go: parseSchemaDir`) was returning `[]*pg.TableDef` for all
dialects, including MySQL and SQLite. `kit/migrate_mysql.go` and `kit/migrate_sqlite.go` received
`*pg.TableDef` values even for non-PostgreSQL schemas, losing dialect identity at the type level.

### Resolution — Option 1: `pg.TableDefiner` interface

The fix introduces a `pg.TableDefiner` interface in `schema/pg/tabledef_iface.go`:

```go
// TableDefiner is the dialect-agnostic table descriptor interface.
// All dialect-specific TableDef types implement this interface.
type TableDefiner interface {
    Def() *TableDef   // returns the underlying column/constraint data
    Dialect() string  // "postgres", "mysql", or "sqlite"
}
```

Dialect identity is preserved through distinct concrete types:

| Package | Concrete type | `Dialect()` |
|---|---|---|
| `schema/pg` | `*pg.TableDef` | `"postgres"` |
| `schema/mysql` | `*mysql.TableDef` | `"mysql"` |
| `schema/sqlite` | `*sqlite.TableDef` | `"sqlite"` |

`*mysql.TableDef` and `*sqlite.TableDef` are distinct structs that embed `*pg.TableDef`
(not type aliases), giving them promoted field access while carrying their own `Dialect()` method.

### Propagation

All kit entry points (`Push`, `DryRun`, `Migrate`, `Status` and their MySQL/SQLite variants)
now accept `...pg.TableDefiner` instead of `...*pg.TableDef`. The CLI `parseSchemaDir`
returns `[]pg.TableDefiner`, and `gen/parser.EvalTable` returns `pg.TableDefiner` (dispatching
to `*mysql.TableDef` or `*sqlite.TableDef` based on the `ParsedTable.Dialect` field).

Backward compatibility is preserved: any code that already passes `*pg.TableDef` values
continues to work without change because `*pg.TableDef` implements `pg.TableDefiner`.
