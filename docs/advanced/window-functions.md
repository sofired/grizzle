# Window Functions

Window expressions (`fn OVER (PARTITION BY … ORDER BY …)`) are in the `expr` package. They implement `SelectableColumn` so they can appear in SELECT and ORDER BY.

## Ranking functions

```go
expr.RowNumber()   // ROW_NUMBER()
expr.Rank()        // RANK()
expr.DenseRank()   // DENSE_RANK()
```

## Navigation functions

```go
expr.Lead(db.UsersT.Score)        // LEAD("score")
expr.Lag(db.UsersT.Score)         // LAG("score")
expr.FirstValue(db.UsersT.Score)  // FIRST_VALUE("score")
expr.LastValue(db.UsersT.Score)   // LAST_VALUE("score")
expr.NthValue(db.UsersT.Score)    // NTH_VALUE("score")
```

## Aggregate window functions

```go
expr.WinSum(db.UsersT.Score)    // SUM("score") OVER (...)
expr.WinAvg(db.UsersT.Score)    // AVG("score") OVER (...)
expr.WinCount()                  // COUNT(*) OVER (...)
```

## PARTITION BY and ORDER BY

Chain `.PartitionBy(…)` and `.OrderBy(…)` to add the OVER clause:

```go
expr.RowNumber().
    PartitionBy(db.UsersT.RealmID).
    OrderBy(db.UsersT.CreatedAt.Asc()).
    As("rn")
// ROW_NUMBER() OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC) AS "rn"
```

## Example — ranking users within each realm

```go
type UserRanked struct {
    ID       uuid.UUID `db:"id"`
    Username string    `db:"username"`
    RealmID  uuid.UUID `db:"realm_id"`
    Rn       int64     `db:"rn"`
}

rows, err := d.Query(ctx,
    query.Select(
        db.UsersT.ID,
        db.UsersT.Username,
        db.UsersT.RealmID,
        expr.RowNumber().
            PartitionBy(db.UsersT.RealmID).
            OrderBy(db.UsersT.Username.Asc()).
            As("rn"),
    ).From(db.UsersT).Where(db.UsersT.DeletedAt.IsNull()),
)
ranked, err := pgxdb.ScanAll[UserRanked](rows, err)
```

## Example — running total

```go
type UserWithTotal struct {
    ID          uuid.UUID `db:"id"`
    Score       int       `db:"score"`
    RunningSum  float64   `db:"running_sum"`
}

rows, err := d.Query(ctx,
    query.Select(
        db.UsersT.ID,
        db.UsersT.Score,
        expr.WinSum(db.UsersT.Score).
            PartitionBy(db.UsersT.RealmID).
            OrderBy(db.UsersT.CreatedAt.Asc()).
            As("running_sum"),
    ).From(db.UsersT),
)
```

## Sorting by window result

Window expressions support `.Asc()` and `.Desc()` for use in the outer ORDER BY:

```go
rn := expr.RowNumber().PartitionBy(db.UsersT.RealmID).OrderBy(db.UsersT.Score.Desc())

query.Select(db.UsersT.ID, rn.As("rn")).
    From(db.UsersT).
    OrderBy(rn.Asc()) // ORDER BY ROW_NUMBER() OVER (...) ASC
```

## Dialect compatibility

| Dialect | Window functions |
|---|---|
| PostgreSQL | Fully supported |
| MySQL | Supported (8.0+) |
| SQLite | Supported (3.25+) |

When building against a dialect where `SupportsWindowFunctions()` returns false, the query builder **silently drops window function columns** from the SELECT list. Non-window columns in the same SELECT are kept. If every column in the SELECT list is a window function, the query falls back to `SELECT *`.

```go
// On a dialect that does not support window functions:
query.Select(
    db.UsersT.ID,         // kept
    expr.RowNumber().As("rn"),  // dropped — window fn
).From(db.UsersT)
// → SELECT "users"."id" FROM "users"

query.Select(
    expr.RowNumber().As("rn"),  // only column, dropped
).From(db.UsersT)
// → SELECT * FROM "users"  (fallback)
```

::: warning
The silent-drop behaviour exists for forward compatibility with older database versions. Relying on it in production means the dropped columns will be missing from query results without an error. If window functions are required for correctness (e.g. for pagination or ranking), check `d.SupportsWindowFunctions()` before building the query.
:::
