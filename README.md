# G-rizzle

A type-safe, code-generated query builder and migration toolkit for Go — inspired by [Drizzle ORM](https://orm.drizzle.team/).

G-rizzle generates Go structs that mirror your database schema. Every column is a strongly-typed handle: you can only compare a `UUIDColumn` with another UUID, a `StringColumn` with a string, and so on. Type mismatches become compile errors, not runtime surprises.

## Features

- **Type-safe query builders** — SELECT, INSERT, UPDATE, DELETE, UPSERT
- **Aggregate functions** — COUNT, SUM, AVG, MAX, MIN with HAVING / ORDER BY
- **Subquery support** — EXISTS, NOT EXISTS, IN (subquery), NOT IN (subquery), FROM (subquery)
- **Relations** — BelongsTo, HasMany, HasOne with JoinRel / InnerJoinRel
- **Eager loading** — batch preloading to avoid N+1 queries
- **JSONB operators** — ->, ->>, #>, #>>, @>, <@, ?, ?|, ?&
- **Code generator** — `grizzle generate` turns a schema file into Go table types
- **Migration kit** — diff live DB vs desired schema, apply DDL atomically with history tracking
- **Multi-dialect** — PostgreSQL (primary), MySQL / MariaDB, SQLite

---

## Installation

```sh
go get github.com/sofired/grizzle
```

Install the CLI:

```sh
go install github.com/sofired/grizzle/cmd/grizzle@latest
```

---

## Quick Start

### 1. Define your schema

```hcl
# schema.grizzle
table "users" {
  column "id"         { type = "uuid"        default = "gen_random_uuid()" primary_key = true }
  column "username"   { type = "varchar(80)" not_null = true unique = true }
  column "email"      { type = "varchar(255)" not_null = true }
  column "realm_id"   { type = "uuid"        not_null = true }
  column "created_at" { type = "timestamptz" default = "now()" not_null = true }

  index "users_username_idx" { columns = ["username"] }
}

table "realms" {
  column "id"   { type = "uuid"        default = "gen_random_uuid()" primary_key = true }
  column "name" { type = "varchar(80)" not_null = true unique = true }
}
```

### 2. Generate table types

```sh
grizzle generate --schema schema.grizzle --out db/schema.go --package db
```

This creates typed table handles like:

```go
var UsersT = UsersTable{
    ID:        expr.UUIDColumn{...},
    Username:  expr.StringColumn{...},
    Email:     expr.StringColumn{...},
    RealmID:   expr.UUIDColumn{...},
    CreatedAt: expr.TimestampColumn{...},
}
```

---

## Query Builders

All builders produce `(sql string, args []any)` via `.Build(dialect)`.

### SELECT

```go
import (
    "github.com/sofired/grizzle/query"
    "github.com/sofired/grizzle/dialect"
    db "myapp/db"
)

// SELECT * FROM "users"
q := query.Select().From(db.UsersT)

// SELECT "users"."id", "users"."username" FROM "users" WHERE "users"."email" = $1
q := query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(db.UsersT.Email.EQ("alice@example.com"))

sql, args := q.Build(dialect.Postgres)
```

#### Filtering

```go
// WHERE ... AND ...
Where(expr.And(
    db.UsersT.RealmID.EQ(realmID),
    db.UsersT.Username.ILike("%alice%"),
))

// WHERE ... OR ...
Where(expr.Or(
    db.UsersT.Email.EQ("a@x.com"),
    db.UsersT.Email.EQ("b@x.com"),
))

// nil conditions are dropped — safe for dynamic filters
Where(expr.And(
    whenPtr(req.RealmID, func(id uuid.UUID) expr.Expression {
        return db.UsersT.RealmID.EQ(id)
    }),
    whenPtr(req.Email, func(e string) expr.Expression {
        return db.UsersT.Email.EQ(e)
    }),
))
```

#### Ordering, pagination

```go
query.Select().From(db.UsersT).
    OrderBy(db.UsersT.CreatedAt.Desc(), db.UsersT.Username.Asc()).
    Limit(20).
    Offset(40)
```

#### IN / NOT IN

```go
Where(db.UsersT.ID.In(id1, id2, id3))
Where(db.UsersT.Username.NotIn("admin", "root"))
```

### Aggregates

```go
// SELECT "realm_id", COUNT(*) AS "cnt" FROM "users" GROUP BY "realm_id" HAVING COUNT(*) > $1
query.Select(db.UsersT.RealmID, expr.Count().As("cnt")).
    From(db.UsersT).
    GroupBy(db.UsersT.RealmID).
    Having(expr.Count().GT(5)).
    OrderBy(expr.Count().Desc())

// Other aggregate functions
expr.CountCol(db.UsersT.Email)         // COUNT("email")
expr.CountDistinct(db.UsersT.RealmID) // COUNT(DISTINCT "realm_id")
expr.Sum(db.UsersT.Score)             // SUM("score")
expr.Avg(db.UsersT.Score)             // AVG("score")
expr.Max(db.UsersT.CreatedAt)         // MAX("created_at")
expr.Min(db.UsersT.CreatedAt)         // MIN("created_at")
```

### Subqueries

```go
// WHERE EXISTS (SELECT * FROM realms WHERE realms.id = users.realm_id)
sub := query.Select().From(db.RealmsT).Where(db.RealmsT.ID.EQCol(db.UsersT.RealmID))
query.Select(db.UsersT.ID).From(db.UsersT).Where(query.Exists(sub))

// WHERE realm_id IN (SELECT id FROM realms WHERE name = $1)
sub := query.Select(db.RealmsT.ID).From(db.RealmsT).Where(db.RealmsT.Name.EQ("acme"))
query.Select(db.UsersT.ID).From(db.UsersT).Where(query.SubqueryIn(db.UsersT.RealmID, sub))

// FROM (SELECT realm_id, COUNT(*) AS cnt FROM users GROUP BY realm_id) AS sub
inner := query.Select(db.UsersT.RealmID, expr.Count().As("cnt")).
    From(db.UsersT).GroupBy(db.UsersT.RealmID)
query.Select().From(query.FromSubquery(inner, "sub"))
```

Parameter numbers are shared between outer and inner queries — no collisions.

### INSERT

```go
type NewUser struct {
    ID       uuid.UUID `db:"id"`
    Username string    `db:"username"`
    Email    string    `db:"email"`
}

q := query.InsertInto(db.UsersT).
    Values(NewUser{ID: uuid.New(), Username: "alice", Email: "alice@x.com"}).
    Returning(db.UsersT.ID, db.UsersT.CreatedAt)

sql, args := q.Build(dialect.Postgres)
```

### UPSERT

```go
// PostgreSQL: ON CONFLICT (username) DO UPDATE SET email = EXCLUDED.email
query.InsertInto(db.UsersT).
    Values(row).
    OnConflict(db.UsersT.Username).
    DoUpdateSetExcluded(db.UsersT.Email, db.UsersT.UpdatedAt)

// DO NOTHING
query.InsertInto(db.UsersT).Values(row).OnConflict(db.UsersT.Username).DoNothing()

// MySQL: ON DUPLICATE KEY UPDATE (auto-detected from dialect)
query.InsertInto(db.UsersT).
    Values(row).
    OnConflict(db.UsersT.Username).
    DoUpdateSetExcluded(db.UsersT.Email)
```

### UPDATE

```go
query.Update(db.UsersT).
    Set(db.UsersT.Email, "new@x.com").
    Set(db.UsersT.UpdatedAt, time.Now()).
    Where(db.UsersT.ID.EQ(userID)).
    Returning(db.UsersT.UpdatedAt)
```

### DELETE

```go
query.DeleteFrom(db.UsersT).
    Where(db.UsersT.ID.EQ(userID)).
    Returning(db.UsersT.ID)
```

---

## Relations

Define relations once, reuse in queries:

```go
// db/relations.go
var UserRealm = query.BelongsTo("realm", db.RealmsT, db.RealmsT.ID.EQCol(db.UsersT.RealmID))
var RealmUsers = query.HasMany("users", db.UsersT, db.UsersT.RealmID.EQCol(db.RealmsT.ID))
```

Use in SELECT:

```go
// LEFT JOIN "realms" ON "realms"."id" = "users"."realm_id"
query.Select(db.UsersT.ID, db.RealmsT.Name).
    From(db.UsersT).
    JoinRel(UserRealm)

// INNER JOIN
query.Select().From(db.UsersT).InnerJoinRel(UserRealm)
```

### Eager loading (avoid N+1)

```go
// Step 1: load users
users, _ := pgx.CollectRows(rows, pgx.RowToStructByName[User])

// Step 2: collect foreign keys
realmIDs := query.UniqueUUIDs(query.Pluck(users, func(u User) uuid.UUID { return u.RealmID }))

// Step 3: batch load realms
realmsQ := query.PreloadUUIDs(query.Select().From(db.RealmsT), db.RealmsT.ID, realmIDs)
realms, _ := pgx.CollectRows(realmRows, pgx.RowToStructByName[Realm])

// Step 4: index and attach
realmByID := query.Index(realms, func(r Realm) uuid.UUID { return r.ID })
for i, u := range users {
    users[i].Realm = realmByID[u.RealmID]
}
```

---

## JSONB Operators (PostgreSQL)

```go
// col -> 'key'        (returns JSON)
db.UsersT.Attributes.Arrow("role")

// col ->> 'key'       (returns text)
db.UsersT.Attributes.ArrowText("name")

// col #> ARRAY['a','b']   (path extraction, returns JSON)
db.UsersT.Attributes.Path("address", "city")

// col @> $1           (contains)
db.UsersT.Attributes.Contains(map[string]any{"role": "admin"})

// col ? $1            (key exists)
db.UsersT.Attributes.HasKey("role")

// col ?| $1           (any key exists)
db.UsersT.Attributes.HasAnyKey("role", "scope")

// col ?& $1           (all keys exist)
db.UsersT.Attributes.HasAllKeys("role", "region")
```

---

## Migration Kit

### Applying migrations

```go
import "github.com/sofired/grizzle/kit"

// Introspect live DB, diff, apply DDL, record in _grizzle_migrations
err := kit.Migrate(ctx, pool,
    db.UsersTableDef,
    db.RealmsTableDef,
)
```

### Dry run

```go
changes, err := kit.DryRun(ctx, pool, db.UsersTableDef, db.RealmsTableDef)
for _, c := range changes {
    fmt.Println(c)
}
```

### Status

```go
// Show applied migrations and any pending changes
kit.Status(ctx, pool, db.UsersTableDef, db.RealmsTableDef)
```

### MySQL migrations

```go
import "github.com/sofired/grizzle/kit"

stmts := kit.AllChangeSQLMySQL(snap, changes)
```

---

## CLI

```
grizzle generate --schema schema.grizzle --out db/schema.go --package db
grizzle migrate  --dsn "postgres://..." --schema schema.grizzle [--dry-run]
grizzle status   --dsn "postgres://..." --schema schema.grizzle
```

---

## Dialects

| Feature           | PostgreSQL | MySQL / MariaDB | SQLite |
|-------------------|:----------:|:---------------:|:------:|
| Named params      | $1, $2 … | ?               | ?      |
| RETURNING         | ✓          | ✗               | ✓      |
| ON CONFLICT       | ✓          | ✗               | ✓      |
| ON DUPLICATE KEY  | ✗          | ✓               | ✗      |
| JSONB operators   | ✓          | ✗               | ✗      |
| UUID native type  | ✓          | CHAR(36)        | TEXT   |

---

## License

MIT
