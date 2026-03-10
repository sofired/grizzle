# Aggregates

Aggregate expressions are in the `expr` package. They implement both `SelectableColumn` (usable in SELECT and ORDER BY) and `Expression` (usable in HAVING).

## Aggregate functions

```go
expr.Count()                           // COUNT(*)
expr.CountCol(db.UsersT.Email)         // COUNT("email")
expr.CountDistinct(db.UsersT.RealmID)  // COUNT(DISTINCT "realm_id")
expr.Sum(db.UsersT.Score)              // SUM("score")
expr.Avg(db.UsersT.Score)              // AVG("score")
expr.Max(db.UsersT.CreatedAt)          // MAX("created_at")
expr.Min(db.UsersT.CreatedAt)          // MIN("created_at")
```

## Using aggregates in SELECT

```go
query.Select(db.UsersT.RealmID, expr.Count().As("cnt")).
    From(db.UsersT).
    GroupBy(db.UsersT.RealmID)
// SELECT "users"."realm_id", COUNT(*) AS "cnt"
// FROM "users" GROUP BY "users"."realm_id"
```

## GROUP BY

```go
query.Select(db.UsersT.RealmID, expr.CountDistinct(db.UsersT.ID).As("active_users")).
    From(db.UsersT).
    Where(db.UsersT.DeletedAt.IsNull()).
    GroupBy(db.UsersT.RealmID)
```

## HAVING

```go
// Only realms with more than 5 active users
query.Select(db.UsersT.RealmID, expr.Count().As("cnt")).
    From(db.UsersT).
    Where(db.UsersT.DeletedAt.IsNull()).
    GroupBy(db.UsersT.RealmID).
    Having(expr.Count().GT(5))
```

HAVING operators: `.GT(n)`, `.GTE(n)`, `.LT(n)`, `.LTE(n)`, `.EQ(n)`, `.NEQ(n)`.

## ORDER BY aggregate

```go
query.Select(db.UsersT.RealmID, expr.Count().As("cnt")).
    From(db.UsersT).
    GroupBy(db.UsersT.RealmID).
    Having(expr.Count().GT(0)).
    OrderBy(expr.Count().Desc())
// ORDER BY COUNT(*) DESC
```

## Aliases

Call `.As("alias")` to give an aggregate an output name. The alias appears in the SELECT clause and is used for scanning results:

```go
// Scan into a struct with matching db tags
type RealmStats struct {
    RealmID uuid.UUID `db:"realm_id"`
    Cnt     int64     `db:"cnt"`
}

rows, err := d.Query(ctx,
    query.Select(db.UsersT.RealmID, expr.Count().As("cnt")).
        From(db.UsersT).
        GroupBy(db.UsersT.RealmID),
)
stats, err := pgxdb.ScanAll[RealmStats](rows, err)
```
