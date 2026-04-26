# Query Builder Specification

Grizzle's query builder is the Go equivalent of [Drizzle ORM's query API](https://orm.drizzle.team/docs/select).

## Design principle

Drizzle's query builders are immutable — each method returns a new builder. Grizzle follows this exactly. All builder methods must return a copy; no mutation of the receiver.

## SELECT

### Basic

**Drizzle:**
```typescript
db.select().from(users)
db.select({ id: users.id, name: users.name }).from(users)
```

**Grizzle:**
```go
query.Select().From(db.UsersT)
query.Select(db.UsersT.ID, db.UsersT.Username).From(db.UsersT)
```

**Status:** PARITY

### WHERE

| Drizzle | Grizzle | Status |
|---|---|---|
| `eq(col, val)` | `col.EQ(val)` | PARITY |
| `ne(col, val)` | `col.NEQ(val)` | PARITY — renamed to `NEQ` to avoid ambiguity with Go's `!=` operator; intentional |
| `gt(col, val)` | `col.GT(val)` | PARITY |
| `gte(col, val)` | `col.GTE(val)` | PARITY |
| `lt(col, val)` | `col.LT(val)` | PARITY |
| `lte(col, val)` | `col.LTE(val)` | PARITY |
| `like(col, pattern)` | `col.Like(pattern)` | PARITY |
| `ilike(col, pattern)` | `col.ILike(pattern)` | PARITY |
| `notLike(col, pattern)` | DEVIATION:GAP (designed) — add `.NotLike()` to string column | — |
| `notIlike(col, pattern)` | DEVIATION:GAP (designed) — add `.NotILike()` to string column | — |
| `inArray(col, vals)` | `col.In(vals...)` | PARITY |
| `notInArray(col, vals)` | `col.NotIn(vals...)` | PARITY |
| `isNull(col)` | `col.IsNull()` | PARITY |
| `isNotNull(col)` | `col.IsNotNull()` | PARITY |
| `between(col, lo, hi)` | `col.Between(lo, hi)` | PARITY |
| `notBetween(col, lo, hi)` | DEVIATION:GAP (designed) — add `.NotBetween()` to numeric/timestamp columns | — |
| `and(...exprs)` | `expr.And(exprs...)` | PARITY |
| `or(...exprs)` | `expr.Or(exprs...)` | PARITY |
| `not(expr)` | `expr.Not(expr)` | PARITY |
| `sql\`raw\`` | `expr.Raw(str)` | PARITY for unparameterized strings |
| Parameterized `sql\`... ${val}\`` | DEVIATION:BROKEN — see bug #129 | — |
| `exists(subquery)` | `query.Exists(sub)` | PARITY |
| `notExists(subquery)` | `query.NotExists(sub)` | PARITY |

### ORDER BY

| Drizzle | Grizzle | Status |
|---|---|---|
| `asc(col)` / `col.asc()` | `col.Asc()` | PARITY |
| `desc(col)` / `col.desc()` | `col.Desc()` | PARITY |
| `nullsFirst(col.asc())` | DEVIATION:GAP (designed) — add `.NullsFirst()` / `.NullsLast()` to sort expressions | — |
| `nullsLast(col.asc())` | DEVIATION:GAP (designed) | — |

### LIMIT / OFFSET

**Status:** PARITY

### GROUP BY / HAVING

**Status:** PARITY

Note: Drizzle's TypeScript type system prevents passing an aliased column (e.g. `sql<...>.as("alias")`) to `.groupBy()` — the type only accepts `Column | SQL`. In Grizzle, `expr.ColAs` satisfies `SelectableColumn` and can be passed to `.GroupBy()`. Grizzle strips the alias before rendering so only the underlying column reference appears in the GROUP BY clause, matching the SQL Drizzle would generate. This is correct per standard SQL: `GROUP BY` does not accept `AS` aliases (fix #131).

### JOINs

| Drizzle | Grizzle | Status |
|---|---|---|
| `.leftJoin(tbl, on)` | `.LeftJoin(tbl, on)` | PARITY |
| `.innerJoin(tbl, on)` | `.InnerJoin(tbl, on)` | PARITY |
| `.rightJoin(tbl, on)` | `.RightJoin(tbl, on)` | PARITY |
| `.fullJoin(tbl, on)` | `.FullJoin(tbl, on)` | PARITY |
| `.crossJoin(tbl)` | DEVIATION:GAP (designed) — add `.CrossJoin(tbl)` with no ON condition | — |

### DISTINCT

| Drizzle | Grizzle | Status |
|---|---|---|
| `.distinct()` | `.Distinct()` | PARITY |
| `.distinctOn(cols)` (PostgreSQL) | `.DistinctOn(cols...)` | PARITY — degrades to `SELECT DISTINCT` on MySQL/SQLite |

### CTEs (Common Table Expressions) — PARITY

**Drizzle:**
```typescript
const sq = db.$with('sq').as(db.select({ userId: users.id }).from(users).where(isNull(users.deletedAt)))
db.with(sq).select().from(sq)
```

**Grizzle:**
```go
sub := query.Select(db.UsersT.ID).From(db.UsersT).Where(db.UsersT.DeletedAt.IsNull())
query.Select(expr.ColBase{TableAlias: "sq", ColName: "id"}).With("sq", sub).From(query.CTERef("sq"))
```

Both non-recursive (`With`) and recursive (`WithRecursive`) CTEs are implemented. Supported by all built-in dialects (PostgreSQL, MySQL 8.0+, SQLite 3.8.3+). When a custom dialect returns `SupportsCTE() = false`, the WITH clause is omitted and any CTERef becomes a dangling table name — producing a runtime database error (fail-loud by design).

**Status:** PARITY

### FOR UPDATE / row locking — PARITY

**Drizzle:**
```typescript
db.select().from(users).for('update')
db.select().from(users).for('update', { skipLocked: true })
db.select().from(users).for('share', { noWait: true })
```

**Grizzle:**
```go
query.Select().From(db.UsersT).ForUpdate()
query.Select().From(db.UsersT).ForShare()
query.Select().From(db.UsersT).ForNoKeyUpdate() // PostgreSQL only
query.Select().From(db.UsersT).ForKeyShare()    // PostgreSQL only
```

`ForUpdate()`, `ForShare()`, `ForNoKeyUpdate()`, and `ForKeyShare()` are convenience wrappers around `For()`.

Only PostgreSQL-valid clauses are emitted for the PostgreSQL dialect and MySQL-valid clauses for MySQL. `FOR NO KEY UPDATE` and `FOR KEY SHARE` are PostgreSQL-only and are silently dropped for other dialects. `Of()` works with all four lock modes.

### Set operations

| Drizzle | Grizzle | Status |
|---|---|---|
| `union(q1, q2)` | `query.Union(q1, q2)` | PARITY |
| `unionAll(q1, q2)` | `query.UnionAll(q1, q2)` | PARITY |
| `intersect(q1, q2)` | `query.Intersect(q1, q2)` | PARITY |
| `intersectAll(q1, q2)` | `query.IntersectAll(q1, q2)` | PARITY |
| `except(q1, q2)` | `query.Except(q1, q2)` | PARITY |
| `exceptAll(q1, q2)` | `query.ExceptAll(q1, q2)` | PARITY |
| `.orderBy()` on set op | `.OrderBy()` | PARITY — Drizzle strips table qualifiers from `PgColumn` refs automatically; Grizzle does the same via `ToSQLUnqualified` |
| `.limit()` on set op | `.Limit()` | PARITY |

### Subqueries

| Drizzle | Grizzle | Status |
|---|---|---|
| `db.select().from(subquery)` | `query.Select().From(subquery)` | PARITY |
| Correlated subquery in WHERE | `query.Exists(sub)` / scalar subquery | PARITY |
| Subquery in SELECT list | `sub.As(alias)` | PARITY |
| `inArray(col, subquery)` — col IN (SELECT ...) | `query.SubqueryIn(col, sub)` | PARITY — when `col` is an `expr.ColAs`, the alias is stripped before rendering; the IN left-hand side is a column reference and must not carry an AS clause (fix #131) |
| `notInArray(col, subquery)` — col NOT IN (SELECT ...) | `query.SubqueryNotIn(col, sub)` | PARITY — same alias-stripping behaviour as SubqueryIn (fix #131) |
| Lateral join | DEVIATION:GAP (not designed) | — |

### Window functions

| Drizzle | Grizzle | Status |
|---|---|---|
| `rank().over(...)` | `expr.Rank().Over(...)` | PARITY |
| `rowNumber().over(...)` | `expr.RowNumber().Over(...)` | PARITY |
| `denseRank().over(...)` | `expr.DenseRank().Over(...)` | PARITY |
| `lag(col, offset, def)` | `expr.Lag(col, offset, def)` | PARITY |
| `lead(col, offset, def)` | `expr.Lead(col, offset, def)` | PARITY |
| `firstValue(col)` | `expr.FirstValue(col)` | PARITY |
| `lastValue(col)` | `expr.LastValue(col)` | PARITY |
| `nthValue(col, n)` | `expr.NthValue(col, n)` | PARITY |
| `ntile(n)` | `expr.Ntile(n)` | PARITY |
| `percentRank()` | `expr.PercentRank()` | PARITY |
| `cumeDist()` | `expr.CumeDist()` | PARITY |
| `.partitionBy(cols)` | `.PartitionBy(cols...)` | PARITY |
| `.orderBy(cols)` | `.OrderBy(cols...)` | PARITY |
| Frame spec (ROWS/RANGE/GROUPS BETWEEN) | DEVIATION:GAP (designed) — tracked as #139 | — |

### Aggregates

| Drizzle | Grizzle | Status |
|---|---|---|
| `count()` | `expr.Count()` | PARITY |
| `count(col)` | `expr.CountCol(col)` | PARITY |
| `countDistinct(col)` | `expr.CountDistinct(col)` | PARITY |
| `sum(col)` | `expr.Sum(col)` | PARITY |
| `avg(col)` | `expr.Avg(col)` | PARITY |
| `max(col)` | `expr.Max(col)` | PARITY |
| `min(col)` | `expr.Min(col)` | PARITY |

### CASE expressions

**Status:** PARITY (searched CASE and simple CASE both implemented)

## INSERT

### Single row / multiple rows

**Status:** PARITY

### RETURNING

**Status:** PARITY — silently dropped for MySQL, matching Drizzle's behaviour

### ON CONFLICT (upsert)

| Drizzle | Grizzle | Status |
|---|---|---|
| `.onConflictDoNothing()` | `.OnConflictConstraint(name).DoNothing()` | PARITY |
| `.onConflictDoUpdate({target, set})` | `.OnConflict(cols...).DoUpdateSetExcluded(cols...)` | PARITY |
| Conflict target as columns | `.OnConflict(cols...)` | PARITY |
| Conflict target as constraint | `.OnConflictConstraint(name)` | PARITY |
| `set` with `excluded` reference | `.DoUpdateSetExcluded(cols...)` | PARITY |
| `set` with arbitrary value | `.DoUpdateSet(col, val)` | PARITY |
| `where` on the conflict clause | DEVIATION:GAP (designed) | — |

### INSERT IGNORE (MySQL / SQLite)

**Status:** PARITY — `.IgnoreConflicts()`

## UPDATE

### SET clause

| Drizzle | Grizzle | Status |
|---|---|---|
| `.set({ col: val })` | `.Set(col, val)` | PARITY |
| Struct-based set | `.SetStruct(struct)` | PARITY |
| `UPDATE … FROM` (PostgreSQL) | DEVIATION:GAP (designed) | — |

### RETURNING

**Status:** PARITY

## DELETE

**Status:** PARITY — WHERE + RETURNING

## Prepared statements — DEVIATION:GAP (not designed)

**Drizzle:**
```typescript
const prepared = db.select().from(users)
  .where(eq(users.id, sql.placeholder('id')))
  .prepare('get_user')

const result = await prepared.execute({ id: '...' })
```

**Grizzle target:** The target API needs to be designed. It must integrate with pgx's native prepared statement support.

## Raw SQL

| Drizzle | Grizzle | Status |
|---|---|---|
| `` sql`raw sql` `` | `expr.Raw(str)` | PARITY for literal strings |
| `` sql`... ${val} ...` `` parameterized | DEVIATION:BROKEN — bug #129: excess placeholders emit as literal `$?` | — |
| `db.execute(sql\`...\`)` | `db.ExecRaw(ctx, sql, args...)` | PARITY |

## Execution and scanning

| Drizzle | Grizzle | Status |
|---|---|---|
| `await db.select()...` → array | `pgxdb.FromSelect[T](ctx, d, q)` | PARITY |
| Single row or throw | `pgxdb.FromSelectOne[T](ctx, d, q)` | PARITY |
| Optional single row | `pgxdb.FromSelectOpt[T](ctx, d, q)` | PARITY |
| Cursor / streaming | DEVIATION:GAP (not designed) | — |
