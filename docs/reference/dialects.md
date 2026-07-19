# Dialects

The `dialect` package defines the `Dialect` interface and provides three built-in implementations. Every query builder accepts a dialect when producing final SQL, which keeps the same builder code portable across database engines.

Query builds fail fast when a requested feature is unsupported and return
`(sql, args, err)` from `Build(dialect)`.

## Built-in dialects

```go
import "github.com/sofired/grizzle/dialect"

dialect.Postgres  // PostgreSQL-compatible SQL; CockroachDB needs dedicated validation before initial support
dialect.MySQL     // MySQL / MariaDB
dialect.SQLite    // SQLite 3.35+ baseline; RIGHT/FULL JOIN requires SQLite 3.39+
```

## Comparison

SQLite RIGHT/FULL OUTER JOIN support starts in SQLite 3.39.0. The built-in SQLite dialect targets a 3.35+ baseline and therefore returns false from `SupportsRightJoin()` and `SupportsFullJoin()`; a version-aware custom dialect may return true for engines known to be 3.39+. See the [SQLite 3.39.0 release notes](https://www.sqlite.org/releaselog/3_39_0.html).

| Feature | Postgres | MySQL | SQLite |
|---|---|---|---|
| Placeholders | `$1`, `$2`, … | `?`, `?`, … | `?`, `?`, … |
| Identifier quoting | `"name"` | `` `name` `` | `"name"` |
| Normal `RETURNING` clause | Yes | No | Yes (3.35+) |
| Insert ID return | normal `RETURNING` | `.ReturningID()` parity for Drizzle `$returningId()` | normal `RETURNING` |
| Upsert style | `ON CONFLICT … DO UPDATE` | `ON DUPLICATE KEY UPDATE` | `ON CONFLICT … DO UPDATE` |
| Insert ignore / do-nothing conflict | `ON CONFLICT … DO NOTHING` | `INSERT IGNORE` | `ON CONFLICT … DO NOTHING` |
| Non-recursive SELECT CTEs (`With` / `CTERef`) | Yes; Go API shape is DEVIATION:LANGUAGE | Yes (8.0+); Go API shape is DEVIATION:LANGUAGE | Yes (3.8.3+); Go API shape is DEVIATION:LANGUAGE |
| Mutation CTE builders | DEVIATION:GAP (not designed) for insert/update/delete CTE APIs | DEVIATION:GAP (not designed) for update/delete CTE APIs; reviewed RC.1 MySQL insert path does not expose `withList` | DEVIATION:GAP (not designed) for insert/update/delete CTE APIs |
| Recursive CTE helper (`WithRecursive`) | Outside initial target; future GRIZZLE-ONLY helper if specified | Outside initial target; future GRIZZLE-ONLY helper if specified | Outside initial target; future GRIZZLE-ONLY helper if specified |
| Window function SQL capability (`OVER`) | Yes | Yes (8.0+) | Yes (3.25+) |
| `DISTINCT ON` | Yes | No API or fail-fast | No API or fail-fast |
| `RIGHT JOIN` | Yes | Yes | Built-in dialect fails fast; version-aware custom dialects may enable it for SQLite 3.39+ |
| `FULL JOIN` | Yes | No API or fail-fast | Built-in dialect fails fast; version-aware custom dialects may enable it for SQLite 3.39+ |
| `FOR UPDATE OF` | Accepts active `PGLockTableSource` handles; generated code emits the marker only for PostgreSQL table handles/aliases. Active-membership validation is DEVIATION:INTENTIONAL fail-fast hardening. | No RC.1 API; omit or fail-fast | No API or fail-fast |
| `FOR SHARE OF` | Accepts active `PGLockTableSource` handles; generated code emits the marker only for PostgreSQL table handles/aliases. Active-membership validation is DEVIATION:INTENTIONAL fail-fast hardening. | No RC.1 API; omit or fail-fast | No API or fail-fast |
| `NOWAIT` / `SKIP LOCKED` | Supported | Supported (8.0+) | No API or fail-fast |
| `LIMIT` on `UPDATE`/`DELETE` | No API or fail-fast | Yes | Yes only when the SQLite driver/engine exposes `SQLITE_ENABLE_UPDATE_DELETE_LIMIT`; otherwise fail fast |
| Regex match (`~`, `~*`, `!~`, `!~*`) | Yes | No API or `unsupported_feature` | No API or `unsupported_feature` |
| Full-text search (`@@`, `to_tsvector`, etc.) | Yes | No API or `unsupported_feature` | No API or `unsupported_feature` |

## Using a dialect

Pass the dialect to `.Build()` on any query builder:

```go
sql, args, err := query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(db.UsersT.DeletedAt.IsNull()).
    Build(dialect.Postgres)

// Conceptual Postgres: SELECT "id", "username" FROM "users" WHERE "users"."deleted_at" IS NULL
// Conceptual MySQL:    SELECT `id`, `username` FROM `users` WHERE `users`.`deleted_at` IS NULL
```

## Dialect interface

The shared query builders use this dialect interface. Custom dialects must implement every capability method explicitly so unsupported features fail closed.

```go
type UpsertStyle string

const (
    UpsertOnConflict   UpsertStyle = "on_conflict"
    UpsertDuplicateKey UpsertStyle = "duplicate_key"
    UpsertNone         UpsertStyle = "none"
)

type Dialect interface {
    // Placeholder returns "$n" (Postgres) or "?" (MySQL/SQLite) for the nth argument.
    Placeholder(n int) string

    // QuoteIdent wraps one already-validated identifier part.
    QuoteIdent(name string) string

    // Name returns "postgres", "mysql", or "sqlite".
    Name() string

    // SupportsReturning reports normal SQL RETURNING only; MySQL .ReturningID()
    // parity for Drizzle $returningId() does not make this true.
    SupportsReturning() bool

    // UpsertStyle returns the conflict-resolution style.
    UpsertStyle() UpsertStyle

    // Dialect-specific INSERT keyword for ignore-conflict syntax.
    InsertIgnoreClause() string
    SupportsIgnoreConflicts() bool

    // Feature-detection methods — unsupported features are omitted from
    // dialect-specific builders or returned as Build errors from shared builders.
    SupportsCTE() bool
    SupportsWindowFunctions() bool
    SupportsDistinctOn() bool
    SupportsRightJoin() bool     // true for PostgreSQL/MySQL; SQLite true only for engines known to support RIGHT/FULL JOIN, 3.39+
    SupportsFullJoin() bool      // false for MySQL; SQLite true only for engines known to support RIGHT/FULL JOIN, 3.39+
    SupportsForUpdate() bool
    SupportsForNoKeyUpdate() bool
    ForShareClause() string
    SupportsForShareOf() bool
    SupportsRegexpMatch() bool     // false → omit API or Build returns unsupported_feature
    SupportsFullTextSearch() bool  // false → omit API or Build returns unsupported_feature
    SupportsLimitOnMutate() bool   // SQLite must be driver/compile-option gated
}
```

The shared `IgnoreConflicts()` helper is enabled only when `SupportsIgnoreConflicts()` is true. `UpsertStyle()` and `InsertIgnoreClause()` then select the classified syntax without branching on `Name()`.

## Implementing a custom dialect

Any type that satisfies the `Dialect` interface can be used. For example, a PostgreSQL-compatible custom dialect can override identifier quoting. This is illustrative only; dedicated CockroachDB support remains outside the initial Grizzle scope until explicitly specified and tested.

```go
type CRDBDialect struct{}

func (CRDBDialect) Name() string                { return "crdb" }
func (CRDBDialect) Placeholder(n int) string    { return fmt.Sprintf("$%d", n) }
func (CRDBDialect) QuoteIdent(name string) string {
    return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
func (CRDBDialect) SupportsReturning() bool       { return true }
func (CRDBDialect) UpsertStyle() dialect.UpsertStyle { return dialect.UpsertOnConflict }
func (CRDBDialect) InsertIgnoreClause() string     { return "" }
func (CRDBDialect) SupportsIgnoreConflicts() bool  { return false }
func (CRDBDialect) SupportsCTE() bool             { return true }
func (CRDBDialect) SupportsWindowFunctions() bool { return true }
func (CRDBDialect) SupportsDistinctOn() bool      { return true }
func (CRDBDialect) SupportsRightJoin() bool       { return true }
func (CRDBDialect) SupportsFullJoin() bool        { return true }
func (CRDBDialect) SupportsForUpdate() bool       { return true }
func (CRDBDialect) SupportsForNoKeyUpdate() bool  { return true }
func (CRDBDialect) ForShareClause() string        { return "FOR SHARE" }
func (CRDBDialect) SupportsForShareOf() bool      { return true }
func (CRDBDialect) SupportsRegexpMatch() bool     { return true }  // CockroachDB supports PG regex syntax
func (CRDBDialect) SupportsFullTextSearch() bool  { return true }  // CockroachDB supports PG FTS
func (CRDBDialect) SupportsLimitOnMutate() bool   { return false }
```

## Feature detection

Use the dialect interface to write helper code that handles differences without branching on the name:

```go
func buildUserInsert(d dialect.Dialect, row db.UserInsert) (string, []any, error) {
    ib := query.InsertInto(db.UsersT).Values(row)
    if d.SupportsReturning() {
        ib = ib.Returning(db.UsersT.ID)
    }
    return ib.Build(d)
}
```
