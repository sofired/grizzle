# Subqueries

Examples on this page use the error-returning `Build(dialect)` contract.
Unsupported requested features fail with a build error rather than being omitted.

Subquery helpers live in the `query` package. They let you compose SELECT builders into correlated or uncorrelated sub-expressions.

::: warning Trusted raw SQL
Raw SQL fragments in this page are trusted static SQL snippets used for constants, predicates, or identifier-level references. Do not derive raw SQL text from user input; use `RawArgs` or normal builder predicates for dynamic values. Custom external expressions/selectables are the same trust boundary and must bind values through the build context rather than interpolation.
:::

## EXISTS / NOT EXISTS

```go
// Users who have at least one published post
query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(
        query.Exists(
            query.Select(expr.Raw("1")).
                From(db.PostsT).
                Where(
                    expr.And(
                        db.PostsT.AuthorID.EQCol(db.UsersT.ID),
                        db.PostsT.Published.IsTrue(),
                    ),
                ),
        ),
    )
// WHERE EXISTS (SELECT 1 FROM "posts" WHERE "posts"."author_id" = "users"."id" AND "posts"."published" IS TRUE)
```

```go
// Users with no posts
query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(
        query.NotExists(
            query.Select(expr.Raw("1")).
                From(db.PostsT).
                Where(db.PostsT.AuthorID.EQCol(db.UsersT.ID)),
        ),
    )
```

## IN / NOT IN subquery

```go
// Users who authored any post in a specific realm
query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(
        query.SubqueryIn(
            db.UsersT.ID,
            query.Select(db.PostsT.AuthorID).
                From(db.PostsT).
                Where(db.PostsT.RealmID.EQ(realmID)),
        ),
    )
// Conceptual SQL: WHERE "users"."id" IN (SELECT "author_id" FROM "posts" WHERE ...)
```

```go
// Users who have NOT posted in the last 30 days
cutoff := time.Now().Add(-30 * 24 * time.Hour)

query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(
        query.SubqueryNotIn(
            db.UsersT.ID,
            query.Select(db.PostsT.AuthorID).
                From(db.PostsT).
                Where(db.PostsT.CreatedAt.GTE(cutoff)),
        ),
    )
```

## Subquery as FROM source

Use `query.FromSubquery` to treat a SELECT as a derived table:

```go
// Aggregate in a subquery, then filter the result
counts := query.FromSubquery(
    query.Select(
        db.UsersT.RealmID,
        expr.Count().As("cnt"),
    ).From(db.UsersT).GroupBy(db.UsersT.RealmID),
    "counts",
)

// Reference columns from the subquery using expr.ColBase for derived tables.
query.Select(
    expr.ColBase{TableAlias: "counts", ColName: "realm_id"},
    expr.ColBase{TableAlias: "counts", ColName: "cnt"},
).
    From(counts).
    Where(expr.RawArgs(`"counts"."cnt" > $?`, 10))
// Conceptual SQL: SELECT "counts"."realm_id", "counts"."cnt"
// FROM (SELECT "realm_id", COUNT(*) AS "cnt" FROM "users" GROUP BY "users"."realm_id") AS "counts"
// WHERE "counts"."cnt" > 10
```

## CTEs (Common Table Expressions)

Use `With` on a `SelectBuilder` to define non-recursive Common Table Expressions. This is the Drizzle RC.1 parity path for SELECT CTE SQL behavior; the Go API shape uses explicit `With(name, sub)` / `CTERef(name)` helpers instead of RC.1's `$with(alias).as(...)` object flow.

```go
// Non-recursive CTE
recent := query.Select(db.PostsT.ID, db.PostsT.AuthorID).
    From(db.PostsT).
    Where(db.PostsT.CreatedAt.GTE(cutoff))

sql, args, err := query.Select(expr.ColBase{TableAlias: "recent", ColName: "id"}).
    With("recent", recent).
    From(query.CTERef("recent")).
    Build(dialect.Postgres)
// Conceptual SQL: WITH "recent" AS (SELECT ...) SELECT "recent"."id" FROM "recent"
```

`CTERef(name)` is valid only when the active root statement's CTE namespace has registered `With(name, sub)`. That namespace is propagated through nested subquery and CTE-body rendering, so nested queries may reference visible root CTEs. A same-name real table is not enough to satisfy CTE registration, so misspelled or unregistered CTE references fail at build time.

Drizzle RC.1 also supports CTE lists on some mutation builders: PostgreSQL/Cockroach and SQLite expose insert/update/delete builders from `db.with(...)`, and MySQL exposes update/delete builders from `db.with(...)` in the reviewed source. Returning affects result typing, not whether `WITH` is available. Grizzle's initial public CTE API is SELECT-only; mutation CTE APIs are deferred until their dialect-scoped builder and result rules are specified. Recursive CTE helpers are also outside the initial target; if added later, they are Grizzle-only helpers over raw SQL capability rather than RC.1 public-helper parity.

CTE support requires a dialect where `SupportsCTE()` is true. All built-in dialects return `true` (PostgreSQL, MySQL 8.0+, SQLite 3.8.3+). When a custom dialect returns `false`, the builder must fail fast or omit the API rather than emitting dangling CTE references.

::: tip
For most CTE use cases, the batch preloading utilities (`query.PreloadUUIDs`, `query.Index`, `query.GroupBy`) are a simpler alternative that avoids raw SQL entirely. See [Preloading](/guide/preloading).
:::

## Combining subqueries

Subquery expressions are plain `expr.Expression` values and compose with `expr.And` / `expr.Or`:

```go
query.Select(db.UsersT.ID).
    From(db.UsersT).
    Where(
        expr.And(
            db.UsersT.DeletedAt.IsNull(),
            expr.Or(
                query.Exists(/* ... */),
                query.SubqueryIn(db.UsersT.RealmID, /* ... */),
            ),
        ),
    )
```
