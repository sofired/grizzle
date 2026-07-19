# Dialects Specification

Grizzle's dialect system is the Go equivalent of [Drizzle's multi-dialect support](https://orm.drizzle.team/docs/get-started).

## Supported dialects

| Dialect | Drizzle package | Grizzle package | Status |
|---|---|---|---|
| PostgreSQL | `drizzle-orm/pg-core` | `schema/pg`, `dialect.Postgres` | PARITY target with listed gaps; required initial scope |
| MySQL 8.0+ | `drizzle-orm/mysql-core` | `schema/mysql`, `dialect.MySQL` | DEVIATION:GAP (designed) for remaining column type gaps; see [schema.md](./schema.md) |
| SQLite | `drizzle-orm/sqlite-core` | `schema/sqlite`, `dialect.SQLite` | DEVIATION:GAP (designed) for remaining column type gaps; see [schema.md](./schema.md) |
| CockroachDB | `drizzle-orm/cockroach-core` | Use `dialect.Postgres` where compatible only after dedicated validation | DEVIATION:GAP (not designed); file-migration support is out of initial scope |
| Neon, Supabase | Use `pg-core` | Use `dialect.Postgres` where driver-compatible | DEVIATION:GAP (not designed); file-migration support depends on driver capability |
| Turso (libSQL) | Uses `sqlite-core` | Use `dialect.SQLite` where driver-compatible | DEVIATION:GAP (not designed); file-migration support depends on driver capability |

## Dialect feature matrix

| Feature | PostgreSQL | MySQL | SQLite |
|---|---|---|---|
| Placeholders | `$1`, `$2`, … | `?` | `?` |
| Identifier quoting | `"name"` | `` `name` `` | `"name"` |
| Normal `RETURNING` | Yes | No | Yes (3.35+) |
| Insert ID return | normal `RETURNING` | Grizzle `.ReturningID()` as Drizzle `$returningId()` parity | normal `RETURNING` |
| Upsert | `ON CONFLICT … DO UPDATE` | `ON DUPLICATE KEY UPDATE` | `ON CONFLICT … DO UPDATE` |
| Do-nothing insert conflict | `ON CONFLICT … DO NOTHING` | `INSERT IGNORE` via MySQL `.ignore()` | `ON CONFLICT … DO NOTHING` via SQLite `.onConflictDoNothing()` |
| `WITH` (CTE) | Yes | Yes (8.0+) | Yes (3.8.3+) |
| Window function SQL capability | Yes | Yes (8.0+) | Yes (3.25+) |
| `DISTINCT ON` | Yes | No | No |
| `FOR UPDATE` / `FOR SHARE` | Yes | Yes (limited) | No |
| `FOR NO KEY UPDATE` / `FOR KEY SHARE` | Yes | **No** | No |
| `RIGHT JOIN` builder API | Yes | Yes | RC.1 builder-surface parity and SQL rendering on capable engines; DEVIATION:INTENTIONAL fail-fast capability hardening requires SQLite 3.39+ |
| `FULL JOIN` builder API | Yes | No | RC.1 builder-surface parity and SQL rendering on capable engines; DEVIATION:INTENTIONAL fail-fast capability hardening requires SQLite 3.39+ |
| `LIMIT` on `UPDATE`/`DELETE` | No API in RC.1 builders | Yes | Yes only when the SQLite driver/engine exposes `SQLITE_ENABLE_UPDATE_DELETE_LIMIT`; otherwise fail fast |
| JSON operators | extraction operators may be expressed through raw SQL/generic composition; JSONB containment/existence/delete-path operators (`@>`, `<@`, `?`, `?|`, `?&`, `#-`) require `jsonb` or an explicit cast; typed JSONB helpers are Grizzle-only | JSON column types only in initial typed helper surface; dialect-specific JSON functions require raw SQL or a future helper spec | JSON column types only in initial typed helper surface; dialect-specific JSON functions require raw SQL or a future helper spec |
| Regex match (`~`, `~*`, `!~`, `!~*`) | Yes | No API or `unsupported_feature` | No API or `unsupported_feature` |
| Full-text search (`@@`, `to_tsvector`, etc.) | Yes | No API or `unsupported_feature` | No API or `unsupported_feature` |

Window-function SQL capability is a database feature row, not a claim that Drizzle RC.1 exposes public typed window helper functions. Grizzle typed window helpers are `GRIZZLE-ONLY` conveniences if retained; Drizzle users use raw `sql` fragments for window expressions.

## Dialect interface

All query builder operations must route dialect-specific SQL through the `Dialect` interface — no dialect name checks (`if d.Name() == "postgres"`) inside query builder code.

Current interface (the authoritative definition is `dialect.Dialect` in
`dialect/dialect.go`):

```go
type UpsertStyle string

const (
    UpsertOnConflict   UpsertStyle = "on_conflict"
    UpsertDuplicateKey UpsertStyle = "duplicate_key"
    UpsertNone         UpsertStyle = "none"
)

type Dialect interface {
    Placeholder(n int) string
    QuoteIdent(name string) string
    Name() string
    SupportsReturning() bool
    UpsertStyle() UpsertStyle
    SupportsOnConflictConstraint() bool   // PostgreSQL ON CONFLICT ON CONSTRAINT only
    InsertIgnoreClause() string
    SupportsIgnoreConflicts() bool
    SupportsCTE() bool
    SupportsWindowFunctions() bool
    SupportsDistinctOn() bool
    SupportsForUpdate() bool
    SupportsForNoKeyUpdate() bool
    SupportsFullJoin() bool
    SupportsRightJoin() bool
    ForShareClause() string
    SupportsForShareOf() bool
    SupportsRegexpMatch() bool
    SupportsFullTextSearch() bool
    SupportsLimitOnMutate() bool
}
```

`SupportsReturning()` reports support for normal SQL `RETURNING` only. MySQL `.ReturningID()` / Drizzle `$returningId()` parity is an insert-execution helper capability and must not make `SupportsReturning()` return true.

## Driver parity

| Drizzle driver | Grizzle equivalent | Status |
|---|---|---|
| `drizzle-orm/node-postgres` | `driver/pgx` with `pgxpool.Pool` | PARITY |
| `drizzle-orm/postgres-js` | No equivalent | DEVIATION:GAP (not designed) |
| `drizzle-orm/mysql2` | `database/sql` + `go-sql-driver/mysql` | DEVIATION:LANGUAGE for Go driver substitution; remaining adapter behavior is DEVIATION:GAP (designed) until migration/session conformance tests cover it |
| `drizzle-orm/better-sqlite3` | `database/sql` + `mattn/go-sqlite3` | DEVIATION:LANGUAGE for Go driver substitution; remaining adapter behavior is DEVIATION:GAP (designed) until migration/session conformance tests cover it |
| Connection pool config | Via pgx / `database/sql` pool config | PARITY |

Initial `mysqldb` implementations using `database/sql` + `go-sql-driver/mysql` must leave MySQL `.ReturningID()` proof fields such as warning count, skipped rows, duplicate rows, inserted rows, and `AffectedRowsIsInsertedCount` invalid unless the adapter has a documented lower-level source for that exact value. Conformance tests must assert fail-fast `unsupported_feature` behavior for `.ReturningID()` plans with `INSERT IGNORE`, duplicate-key updates, or insert-select row-count synthesis when required proof fields are unavailable. Insert-select `.ReturningID()` succeeds only for reconstructable auto-increment keys with valid inserted-row-count proof.

## Target Dialect Gating Rules

The rules below are the RC.1-parity target. Any current silent dropping or suppression of unsupported clauses/operators is non-conforming implementation debt until fail-fast dialect gating and conformance tests are implemented.

`FOR NO KEY UPDATE`, `FOR KEY SHARE`, and `FOR ... OF <tables>` are PostgreSQL-compatible locking features in the reviewed RC.1 query builders, including PostgreSQL and Cockroach. Cockroach is outside the initial Grizzle dialect scope unless a dedicated dialect spec is added. MySQL RC.1 supports only lock strengths `update` and `share`, rendered as `FOR UPDATE` and `FOR SHARE`; it does not expose PostgreSQL's `of` lock config. Unsupported lock methods must either be absent from dialect-specific builders or fail fast with an unsupported-feature error. Grizzle must not silently drop a user-requested locking clause.

**GRIZZLE-ONLY (PostgreSQL-only):** PostgreSQL-only regex operators (`~`, `~*`, `!~`, `!~*`) and full-text search expressions (`@@`, `to_tsvector`, `to_tsquery`, etc.) are gated through `SupportsRegexpMatch()` and `SupportsFullTextSearch()`. On non-PostgreSQL dialects, builders must omit these APIs or return `unsupported_feature`; they must not render boolean stand-ins such as `FALSE`/`NULL`, because those do not compose safely through `NOT`, `AND`, and `OR`.

`LIMIT` on `UPDATE`/`DELETE` is gated by `SupportsLimitOnMutate()` rather than dialect-name checks inside query builders. PostgreSQL returns `false`. MySQL returns `true`. SQLite returns `true` only when the selected driver/engine exposes `SQLITE_ENABLE_UPDATE_DELETE_LIMIT`; otherwise SQLite builders must omit the method or fail fast. To stay in parity with Drizzle RC.1, PostgreSQL-specific builders should omit the method; if a shared Go builder exposes the method, it must fail fast instead of silently dropping the clause.

## PostgreSQL-specific features

### JSONB operators — GRIZZLE-ONLY (PostgreSQL-specific typed convenience)

`JSONBColumn[T]` and typed containment/existence/delete-path helpers (`@>`, `<@`, `?`, `?|`, `?&`, `#-`) are restricted to generated handles backed by `pg.JSONB()`. Drizzle RC.1 has distinct PostgreSQL `json()` and `jsonb()` builders, and PostgreSQL containment/existence/delete-path operators are JSONB-only. Plain `JSONColumn[T]` is the generated handle for plain JSON across supported dialects and must not expose JSONB-only helpers. Shared extraction helpers such as `->`, `->>`, `#>`, and `#>>` may be offered for PostgreSQL plain JSON if specified; otherwise plain JSON uses raw SQL or an explicit cast. Non-PostgreSQL builders should omit unsupported JSON helpers or return `unsupported_feature` rather than silently changing JSON semantics.

### Full-text search — GRIZZLE-ONLY (PostgreSQL-only)

`tsvector`, `tsquery`, `@@`, `to_tsvector()`, `to_tsquery()`, `plainto_tsquery()`. These are PostgreSQL-specific; Grizzle supports them but gates their emission behind `SupportsFullTextSearch()`. On MySQL and SQLite, builders must omit these helpers or return `unsupported_feature`.

### Regex match operators — GRIZZLE-ONLY (PostgreSQL-only)

`~`, `~*`, `!~`, `!~*`. PostgreSQL POSIX regex match operators exposed via `StringColumn.RegexpMatch()`, `RegexpMatchI()`, `NotRegexpMatch()`, `NotRegexpMatchI()`. On non-PostgreSQL dialects, builders must omit these helpers or return `unsupported_feature`.

### Array operators — mixed DEVIATION:GAP status

Broad array SQL features such as `ANY`, `ALL`, array subscripting, and typed array scan/generation are **DEVIATION:GAP (not designed)**. Public RC.1 condition helpers `arrayContains`, `arrayContained`, and `arrayOverlaps` are **DEVIATION:GAP (designed)**: when implemented, empty-array helper inputs must fail with `build_validation`, matching RC.1's throw behavior.

## Current Implementation Status: `*pg.TableDef` Leak Across Dialects

**Status:** Resolved current-state issue. This section is non-normative implementation history; the target file-migration contract is the dialect-agnostic schema/input model described in the file-migration specs.

### Problem

The CLI parser (`cmd/grizzle/main.go: parseSchemaDir`) was returning `[]*pg.TableDef` for all
dialects, including MySQL and SQLite. `kit/migrate_mysql.go` and `kit/migrate_sqlite.go` received
`*pg.TableDef` values even for non-PostgreSQL schemas, losing dialect identity at the type level.

### Resolution

The implementation uses a `pg.TableDefiner` interface in `schema/pg/tabledef_iface.go`:

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

Schema-definition-taking entry points (`Push`, `DryRun`, SQL generation, schema parsing, and their MySQL/SQLite variants)
now accept `...pg.TableDefiner` instead of `...*pg.TableDef`. The CLI `parseSchemaDir`
returns `[]pg.TableDefiner`, and `gen/parser.EvalTable` returns `pg.TableDefiner` (dispatching
to `*mysql.TableDef` or `*sqlite.TableDef` based on the `ParsedTable.Dialect` field).

`kit.Push` examples in this section document type compatibility only. `Push` remains a direct-sync development/control-plane shortcut and is not a production deployment default until a dedicated push spec defines destructive-change handling, locking, non-interactive behavior, and CI safety. Production-style schema changes should use generated, reviewed, committed migration artifacts with `check` and `migrate`.

`Migrate` and `Status` are not table-definition-taking APIs in the current branch; they take DB handles plus migration/status options. File-migration target behavior is defined in the dedicated file-migration specs.

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
