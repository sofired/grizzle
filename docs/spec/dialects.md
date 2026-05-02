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
| `WITH` (CTE) | Yes | Yes (8.0+) | Yes (3.8.3+) |
| Window functions | Yes | Yes (8.0+) | Yes (3.25+) |
| `DISTINCT ON` | Yes | No | No |
| `FOR UPDATE` / `FOR SHARE` | Yes | Yes (limited) | No |
| `FOR NO KEY UPDATE` / `FOR KEY SHARE` | Yes | **No** | No |
| `FULL JOIN` | Yes | No | No |
| JSON operators | `->`, `->>`, `@>`, etc. | Limited | Limited |
| Regex match (`~`, `~*`, `!~`, `!~*`) | Yes | **No** (emits `FALSE`) | **No** (emits `FALSE`) |
| Full-text search (`@@`, `to_tsvector`, etc.) | Yes | **No** (emits `FALSE`/`NULL`) | **No** (emits `FALSE`/`NULL`) |

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

**Dialect interface — IMPLEMENTED (PR #174 + #186).** All feature-detection methods are present and enforced at build time.

```go
type Dialect interface {
    Placeholder(n int) string
    QuoteIdent(name string) string
    Name() string
    SupportsReturning() bool
    UpsertStyle() UpsertStyle
    InsertIgnoreClause() string
    SupportsCTE() bool
    SupportsWindowFunctions() bool
    SupportsDistinctOn() bool
    SupportsForUpdate() bool                // FOR UPDATE / FOR SHARE
    SupportsForNoKeyUpdate() bool           // PostgreSQL-only; false for MySQL/SQLite
    SupportsFullJoin() bool
    ForShareClause() string                 // "FOR SHARE" (Postgres) / "LOCK IN SHARE MODE" (MySQL)
    SupportsForShareOf() bool
    SupportsRegexpMatch() bool              // PostgreSQL-only (~, ~*, !~, !~*); false for MySQL/SQLite
    SupportsFullTextSearch() bool           // PostgreSQL-only (@@, to_tsvector, etc.); false for MySQL/SQLite
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

## Known bugs

**#110 — FIXED (PR #174).** `FOR NO KEY UPDATE` and `FOR KEY SHARE` were PostgreSQL-only locking clauses that previously emitted for MySQL. Fixed by adding `SupportsForNoKeyUpdate()` to the `Dialect` interface; MySQL and SQLite return `false` and the clauses are now suppressed at build time.

**#230 — FIXED. GRIZZLE-ONLY (safe-fail on non-PG dialects).** PostgreSQL-only regex operators (`~`, `~*`, `!~`, `!~*`) and full-text search expressions (`@@`, `to_tsvector`, `to_tsquery`, etc.) emitted unconditionally regardless of dialect. Fixed by adding `SupportsRegexpMatch()` and `SupportsFullTextSearch()` to the `Dialect` interface. On non-PostgreSQL dialects, predicate expressions (regex match, FTS `@@` operators) emit `FALSE` and scalar expressions (standalone `to_tsquery`, `to_tsvector`) emit `NULL`; no args are bound. Note: the NOT-match operators (`!~`, `!~*`) also emit `FALSE` (not `TRUE`) on unsupported dialects. This is intentional safe-failure behaviour — the query remains syntactically valid but returns no rows, preventing silent data corruption. Callers should check `ctx.Dialect().SupportsRegexpMatch()` or `ctx.Dialect().SupportsFullTextSearch()` before using these operators against non-PG dialects.

## PostgreSQL-specific features

### JSONB operators — DEVIATION:GAP (not designed)

`->`, `->>`, `@>`, `<@`, `?`, `?|`, `?&`, `#>`, `#-`. Tracked as part of #140.

### Full-text search — GRIZZLE-ONLY (safe-fail on non-PG dialects)

`tsvector`, `tsquery`, `@@`, `to_tsvector()`, `to_tsquery()`, `plainto_tsquery()`. These are PostgreSQL-specific; Grizzle supports them but gates their emission behind `SupportsFullTextSearch()`. On MySQL and SQLite, FTS predicate expressions emit `FALSE` and scalar FTS expressions emit `NULL` (issue #230, fixed). Callers must check `SupportsFullTextSearch()` before building FTS queries for portability.

### Regex match operators — GRIZZLE-ONLY (safe-fail on non-PG dialects)

`~`, `~*`, `!~`, `!~*`. PostgreSQL POSIX regex match operators exposed via `StringColumn.RegexpMatch()`, `RegexpMatchI()`, `NotRegexpMatch()`, `NotRegexpMatchI()`. On non-PostgreSQL dialects, these emit `FALSE` (issue #230, fixed). Callers must check `SupportsRegexpMatch()` for portability.

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

Backward compatibility is preserved for the common case: any code that already passes
individual `*pg.TableDef` values continues to work without change because `*pg.TableDef`
implements `pg.TableDefiner`.

**Breaking edge case:** Callers that collected `*pg.TableDef` values into a typed slice and
expanded it with `...` will need a one-time update:

```go
// Before (no longer compiles):
defs := []*pg.TableDef{schema.Users, schema.Realms}
kit.Push(ctx, pool, defs...)

// After — either change the slice element type:
defs := []pg.TableDefiner{schema.Users, schema.Realms}
kit.Push(ctx, pool, defs...)

// Or expand inline (no change needed if values are passed directly):
kit.Push(ctx, pool, schema.Users, schema.Realms)
```
