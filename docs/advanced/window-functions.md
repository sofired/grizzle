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
