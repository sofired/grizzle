# Dialects

The `dialect` package defines the `Dialect` interface and provides three built-in implementations. Every query builder accepts a dialect when producing final SQL, which keeps the same builder code portable across database engines.

## Built-in dialects

```go
import "github.com/sofired/grizzle/dialect"

dialect.Postgres  // PostgreSQL / CockroachDB
dialect.MySQL     // MySQL / MariaDB
dialect.SQLite    // SQLite 3.35+
```

## Comparison

| Feature | Postgres | MySQL | SQLite |
|---|---|---|---|
| Placeholders | `$1`, `$2`, … | `?`, `?`, … | `?`, `?`, … |
| Identifier quoting | `"name"` | `` `name` `` | `"name"` |
| `RETURNING` clause | Yes | No | Yes (3.35+) |
| Upsert style | `ON CONFLICT … DO UPDATE` | `ON DUPLICATE KEY UPDATE` | `ON CONFLICT … DO UPDATE` |
| Insert ignore | `ON CONFLICT … DO NOTHING` | `INSERT IGNORE` | `INSERT OR IGNORE` |
| CTEs (`With` / `WithRecursive`) | Yes | Yes (8.0+) | Yes (3.8.3+) |
| Window functions (`OVER`) | Yes | Yes (8.0+) | Yes (3.25+) |
| `DISTINCT ON` | Yes | No (degrades to `DISTINCT`) | No (degrades to `DISTINCT`) |
| `FULL JOIN` | Yes | No (silently dropped) | No (silently dropped) |
| `FOR UPDATE OF` | All tables emitted | All tables emitted (8.0+) | Silently ignored |
| `FOR SHARE OF` | All tables emitted | Not emitted (`LOCK IN SHARE MODE`) | Silently ignored |
| `NOWAIT` / `SKIP LOCKED` | Supported | Supported (8.0+) | Silently ignored |
| Regex match (`~`, `~*`, `!~`, `!~*`) | Yes | **No** (emits `FALSE`) | **No** (emits `FALSE`) |
| Full-text search (`@@`, `to_tsvector`, etc.) | Yes | **No** (emits `FALSE`/`NULL`) | **No** (emits `FALSE`/`NULL`) |

## Using a dialect

Pass the dialect to `.Build()` on any query builder:

```go
sql, args := query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(db.UsersT.DeletedAt.IsNull()).
    Build(dialect.Postgres)

// Postgres:  SELECT "users"."id", "users"."username" FROM "users" WHERE "users"."deleted_at" IS NULL
// MySQL:     SELECT `users`.`id`, `users`.`username` FROM `users` WHERE `users`.`deleted_at` IS NULL
```

## Dialect interface

```go
type Dialect interface {
    // Placeholder returns "$n" (Postgres) or "?" (MySQL/SQLite) for the nth argument.
    Placeholder(n int) string

    // QuoteIdent wraps a name in the appropriate quote characters.
    QuoteIdent(name string) string

    // Name returns "postgres", "mysql", or "sqlite".
    Name() string

    // SupportsReturning reports whether RETURNING is available.
    SupportsReturning() bool

    // UpsertStyle returns the conflict-resolution style.
    UpsertStyle() UpsertStyle

    // InsertIgnoreClause returns the INSERT-ignore keyword phrase.
    InsertIgnoreClause() string

    // Feature-detection methods — the query builder enforces these at Build() time.
    SupportsCTE() bool             // false → WITH clause silently dropped
    SupportsWindowFunctions() bool // false → window fn columns silently dropped
    SupportsDistinctOn() bool      // false → DistinctOn() degrades to DISTINCT
    SupportsFullJoin() bool        // false → FULL JOIN silently dropped
    SupportsForUpdate() bool       // false → FOR UPDATE / FOR SHARE dropped
    SupportsForNoKeyUpdate() bool  // false → FOR NO KEY UPDATE / FOR KEY SHARE dropped
    ForShareClause() string        // "FOR SHARE" or "LOCK IN SHARE MODE"
    SupportsForShareOf() bool      // false → OF table list omitted from FOR SHARE
    SupportsRegexpMatch() bool     // false → regex exprs emit FALSE (pg-only: ~, ~*, !~, !~*)
    SupportsFullTextSearch() bool  // false → FTS predicates emit FALSE, scalars emit NULL
}
```

## Implementing a custom dialect

Any type that satisfies the `Dialect` interface can be used. For example, to target CockroachDB with a custom identifier quoting rule:

```go
type CRDBDialect struct{}

func (CRDBDialect) Name() string                { return "crdb" }
func (CRDBDialect) Placeholder(n int) string    { return fmt.Sprintf("$%d", n) }
func (CRDBDialect) QuoteIdent(name string) string {
    return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
func (CRDBDialect) SupportsReturning() bool       { return true }
func (CRDBDialect) UpsertStyle() UpsertStyle      { return dialect.UpsertOnConflict }
func (CRDBDialect) InsertIgnoreClause() string    { return "" }
func (CRDBDialect) SupportsCTE() bool             { return true }
func (CRDBDialect) SupportsWindowFunctions() bool { return true }
func (CRDBDialect) SupportsDistinctOn() bool      { return true }
func (CRDBDialect) SupportsFullJoin() bool        { return true }
func (CRDBDialect) SupportsForUpdate() bool       { return true }
func (CRDBDialect) SupportsForNoKeyUpdate() bool  { return true }
func (CRDBDialect) ForShareClause() string        { return "FOR SHARE" }
func (CRDBDialect) SupportsForShareOf() bool      { return true }
func (CRDBDialect) SupportsRegexpMatch() bool     { return true }  // CockroachDB supports PG regex syntax
func (CRDBDialect) SupportsFullTextSearch() bool  { return true }  // CockroachDB supports PG FTS
```

## Feature detection

Use the dialect interface to write helper code that handles differences without branching on the name:

```go
func buildInsert(d dialect.Dialect, t *pg.TableDef, row any) (string, []any) {
    ib := query.InsertInto(t).Values(row)
    if d.SupportsReturning() {
        ib = ib.Returning(t.ID)
    }
    return ib.Build(d)
}
```
