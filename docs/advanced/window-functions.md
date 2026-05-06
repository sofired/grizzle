# Window Functions

::: warning Target query API
Examples on this page use the target error-returning `Build(dialect)` and fail-fast unsupported-feature behavior. The current branch may still expose older two-return builders; any silent dialect fallback is non-conforming implementation debt until those target query contracts land.
:::

Window expressions (`fn OVER (PARTITION BY … ORDER BY …)`) are in the `expr` package. They implement `SelectableColumn` so they can appear in SELECT and ORDER BY.

Drizzle RC.1 parity for window expressions is raw `sql` fragment support. The typed helpers on this page are Grizzle-only conveniences over that SQL capability.

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
    RunningSum  string    `db:"running_sum"`
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
totals, err := pgxdb.ScanAll[UserWithTotal](rows, err)
```

Aggregate window scan types are driver/dialect dependent. Numeric aggregates commonly scan as strings, matching Drizzle's conservative `sum()`/`avg()` result typing; use the destination type required by the selected driver.

## Sorting by window result

Window expressions support `.Asc()` and `.Desc()` for use in the outer ORDER BY:

```go
rn := expr.RowNumber().PartitionBy(db.UsersT.RealmID).OrderBy(db.UsersT.Score.Desc())

query.Select(db.UsersT.ID, rn.As("rn")).
    From(db.UsersT).
    OrderBy(rn.Asc()) // ORDER BY ROW_NUMBER() OVER (...) ASC
```

## Dialect compatibility

| Dialect | Window function SQL capability |
|---|---|
| PostgreSQL | Fully supported |
| MySQL | Supported (8.0+) |
| SQLite | Supported (3.25+) |

When building against a dialect where `SupportsWindowFunctions()` returns false, Grizzle must fail at `Build` with an unsupported-feature error or omit the window-function API from that dialect-specific builder. It must not drop window expressions or fall back to `SELECT *`.

```go
// On a dialect that does not support window functions:
sql, args, err := query.Select(
    db.UsersT.ID,
    expr.RowNumber().As("rn"),
).From(db.UsersT).Build(legacyDialect)
// err: unsupported_feature
```

::: warning
If window functions are required for correctness, such as pagination or ranking, check `d.SupportsWindowFunctions()` before constructing portable queries.
:::
