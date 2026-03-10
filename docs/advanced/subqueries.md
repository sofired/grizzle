# Subqueries

Subquery helpers live in the `query` package. They let you compose SELECT builders into correlated or uncorrelated sub-expressions.

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
// WHERE "users"."id" IN (SELECT "posts"."author_id" FROM "posts" WHERE ...)
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

// Reference columns from the subquery using expr.Raw (no typed column for derived tables)
query.Select(expr.Raw(`"counts"."realm_id"`), expr.Raw(`"counts"."cnt"`)).
    From(counts).
    Where(expr.Raw(`"counts"."cnt" > 10`))
// SELECT "counts"."realm_id", "counts"."cnt"
// FROM (SELECT "users"."realm_id", COUNT(*) AS "cnt" FROM "users" GROUP BY "users"."realm_id") AS "counts"
// WHERE "counts"."cnt" > 10
```

## CTEs (Common Table Expressions)

Grizzle does not yet have a dedicated CTE builder. Use `expr.Raw` to prepend a WITH clause when needed:

```go
sql, args := query.Select(expr.Raw(`"active"."id"`), expr.Raw(`"active"."username"`)).
    From(expr.RawSource(`"active"`)).
    Build(dialect.Postgres)

// Prepend the CTE manually
fullSQL := `WITH active AS (
    SELECT id, username FROM users WHERE deleted_at IS NULL
) ` + sql
```

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
