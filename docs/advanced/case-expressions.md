# CASE Expressions

CASE expressions are in the `expr` package. Both `CaseExpr` and `SimpleCaseExpr` implement `SelectableColumn` (usable in SELECT and ORDER BY) and `Expression` (usable in WHERE/HAVING).

## Searched CASE

`expr.Case()` builds a searched CASE, where each WHEN tests an arbitrary boolean condition:

```go
expr.Case().
    When(db.UsersT.Score.GTE(90), expr.Lit("A")).
    When(db.UsersT.Score.GTE(75), expr.Lit("B")).
    When(db.UsersT.Score.GTE(60), expr.Lit("C")).
    Else(expr.Lit("F")).
    As("grade")
// CASE WHEN "users"."score" >= 90 THEN $1 WHEN ... ELSE $4 END AS "grade"
```

## Simple CASE

`expr.SimpleCase(col)` builds a simple CASE that compares a single column against a series of values:

```go
expr.SimpleCase(db.UsersT.Status).
    WhenVal("active",  expr.Lit("Active User")).
    WhenVal("banned",  expr.Lit("Banned")).
    WhenVal("pending", expr.Lit("Awaiting Verification")).
    Else(expr.Lit("Unknown")).
    As("status_label")
// CASE "users"."status" WHEN $1 THEN $2 WHEN $3 THEN $4 ... END AS "status_label"
```

## expr.Lit

`expr.Lit(v)` wraps any Go value as a bound-parameter expression. Use it wherever a CASE branch needs a literal value — string, int, bool, nil, etc.

```go
expr.Lit("active")    // string → bound parameter
expr.Lit(0)           // int → bound parameter
expr.Lit(nil)         // NULL
```

## Using CASE in SELECT

```go
type UserWithGrade struct {
    ID       uuid.UUID `db:"id"`
    Username string    `db:"username"`
    Score    int       `db:"score"`
    Grade    string    `db:"grade"`
}

rows, err := d.Query(ctx,
    query.Select(
        db.UsersT.ID,
        db.UsersT.Username,
        db.UsersT.Score,
        expr.Case().
            When(db.UsersT.Score.GTE(90), expr.Lit("A")).
            When(db.UsersT.Score.GTE(75), expr.Lit("B")).
            When(db.UsersT.Score.GTE(60), expr.Lit("C")).
            Else(expr.Lit("F")).
            As("grade"),
    ).From(db.UsersT),
)
users, err := pgxdb.ScanAll[UserWithGrade](rows, err)
```

## CASE in WHERE

A CASE expression used in WHERE must produce a boolean result:

```go
query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(
        expr.Case().
            When(db.UsersT.Role.EQ("admin"), db.UsersT.Active.IsTrue()).
            Else(db.UsersT.DeletedAt.IsNull()),
    )
```

## CASE in ORDER BY

```go
// Sort: active users first, then by username
query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    OrderBy(
        expr.SimpleCase(db.UsersT.Status).
            WhenVal("active", expr.Lit(0)).
            Else(expr.Lit(1)).
            Asc(),
        db.UsersT.Username.Asc(),
    )
```

## Column reference in THEN/ELSE

THEN and ELSE accept any `Expression` — including column references:

```go
// Return display_name if set, otherwise fall back to username
expr.Case().
    When(db.UsersT.DisplayName.IsNotNull(), db.UsersT.DisplayName).
    Else(db.UsersT.Username).
    As("name")
```

## No ELSE clause

Omitting `.Else()` results in `NULL` for unmatched rows (standard SQL behaviour):

```go
expr.Case().
    When(db.UsersT.Score.GTE(90), expr.Lit("A")).
    When(db.UsersT.Score.GTE(75), expr.Lit("B")).
    As("top_grade") // NULL for scores below 75
```
