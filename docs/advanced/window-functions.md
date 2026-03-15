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
expr.Lead(db.UsersT.Score)                              // LEAD("score")
expr.LeadWithOffset(db.UsersT.Score, 2)                 // LEAD("score", 2)
expr.LeadWithDefault(db.UsersT.Score, 1, expr.Lit(0))   // LEAD("score", 1, $1) — binds 0
expr.Lag(db.UsersT.Score)                               // LAG("score")
expr.LagWithOffset(db.UsersT.Score, 2)                  // LAG("score", 2)
expr.LagWithDefault(db.UsersT.Score, 1, expr.Lit(0))    // LAG("score", 1, $1) — binds 0
expr.FirstValue(db.UsersT.Score)                        // FIRST_VALUE("score")
expr.LastValue(db.UsersT.Score)                         // LAST_VALUE("score")
expr.NthValue(db.UsersT.Score, 3)                       // NTH_VALUE("score", 3)
```

The `defaultVal` argument for `LeadWithDefault` and `LagWithDefault` accepts any `Expression`. Use `expr.Lit(v)` to bind a Go value as a safe query parameter, or `expr.Raw("NULL")` for SQL keywords.

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

## Frame specification (ROWS / RANGE / GROUPS BETWEEN)

Chain `.Rows(start, end)`, `.Range(start, end)`, or `.Groups(start, end)` to add a frame clause. Frame boundaries are expressed using the package-level constants and constructors:

| Expression | SQL |
|---|---|
| `expr.UnboundedPreceding` | `UNBOUNDED PRECEDING` |
| `expr.CurrentRow` | `CURRENT ROW` |
| `expr.UnboundedFollowing` | `UNBOUNDED FOLLOWING` |
| `expr.Preceding(n)` | `n PRECEDING` |
| `expr.Following(n)` | `n FOLLOWING` |

```go
// Running total — ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
expr.WinSum(db.OrdersT.Amount).
    PartitionBy(db.OrdersT.CustomerID).
    OrderBy(db.OrdersT.CreatedAt.Asc()).
    Rows(expr.UnboundedPreceding, expr.CurrentRow).
    As("running_total")

// Last value in full partition — must use ROWS/RANGE UNBOUNDED FOLLOWING
expr.LastValue(db.UsersT.Score).
    PartitionBy(db.UsersT.RealmID).
    OrderBy(db.UsersT.CreatedAt.Asc()).
    Rows(expr.UnboundedPreceding, expr.UnboundedFollowing).
    As("last_score")

// 5-row moving average
expr.WinAvg(db.MetricsT.Value).
    OrderBy(db.MetricsT.RecordedAt.Asc()).
    Rows(expr.Preceding(2), expr.Following(2)).
    As("moving_avg")

// GROUPS mode (PostgreSQL 11+, SQLite 3.28+)
expr.WinSum(db.OrdersT.Amount).
    OrderBy(db.OrdersT.CreatedAt.Asc()).
    Groups(expr.UnboundedPreceding, expr.CurrentRow).
    As("groups_sum")
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
            Rows(expr.UnboundedPreceding, expr.CurrentRow).
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
