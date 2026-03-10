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
func (CRDBDialect) SupportsReturning() bool     { return true }
func (CRDBDialect) UpsertStyle() UpsertStyle    { return dialect.UpsertOnConflict }
func (CRDBDialect) InsertIgnoreClause() string  { return "" }
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
