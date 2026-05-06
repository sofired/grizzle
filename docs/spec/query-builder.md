# Query Builder Specification

Grizzle's query builder is the Go equivalent of [Drizzle ORM's query API](https://orm.drizzle.team/docs/select).

## Design principle

Drizzle RC.1 query builders mutate internal builder configuration and return `this` while TypeScript's type system narrows which methods remain callable. Grizzle may still use value-copying builders to avoid surprising aliasing in Go, but that is a Go API design choice rather than RC.1 runtime parity.

**Status:** DEVIATION:LANGUAGE if Grizzle keeps immutable/copying builders. The SQL behavior should remain parity; receiver mutability is allowed to differ.

## String Identifier Inputs

Any query API that accepts a string for a column, constraint, alias, table alias, conflict target, or similar SQL identifier must treat that string as trusted schema metadata, not user input.

Normative rules:

- callers must pass compile-time literals, generated constants, or trusted values derived from Grizzle schema metadata
- builders must validate string identifiers as single identifier parts unless the API explicitly accepts a structured multi-part identifier
- builders must quote identifiers through the dialect identifier helper and return stable build errors for invalid identifiers
- user-controlled strings must never be passed as identifier names; use bound values for data
- raw SQL helpers are the only escape hatch for unsupported identifier shapes, and the raw SQL trusted-input rules still apply

## Table Identity Contract

Schema-qualified tables must never be represented as a dotted string. Query rendering must quote each identifier part separately so `audit.users` becomes `"audit"."users"`, not `"audit.users"`.

Target shared metadata shape:

```go
// Package sqlmeta is a lower shared package imported by expr, query, driver,
// and generated code. Keeping this contract below query avoids expr -> query
// import cycles for generated column handles.
package sqlmeta

type TableRef struct {
    Schema string // empty for unqualified tables, CTEs, and subqueries
    Name   string // base table name, CTE name, or subquery alias
    Alias  string // empty means use Name
}

type ColumnRef struct {
    Table TableRef
    Name  string
}

type ColumnMeta struct {
    Ref          ColumnRef
    GoType       reflect.Type
    Nullable     bool
    Encoder      ParamEncoder
    SelectionKey string
}

type ParamEncoder interface {
    Encode(value any) (driverValue any, err error)
    Identity() string
    CastPlaceholder(dialect.Dialect, placeholder string) (string, error)
}

type ParamColumn interface {
    ColumnRef() ColumnRef
    ParamType() reflect.Type
    ParamEncoder() ParamEncoder
    Nullable() bool
}

// Package query must re-export these as type aliases for generated code and public APIs:
// type TableRef = sqlmeta.TableRef
// type ColumnRef = sqlmeta.ColumnRef
// type ColumnMeta = sqlmeta.ColumnMeta
// type ParamEncoder = sqlmeta.ParamEncoder
// type ParamColumn = sqlmeta.ParamColumn
```

Target mutation-column shape:

```go
// Package query owns this composition so typed mutation APIs can validate
// column ownership/aliases and bind values through the generated encoder.
type MutationColumn interface {
    sqlmeta.ParamColumn
    expr.SelectableColumn
}
```

Target table-source shape:

```go
type TableSource interface {
    GrizTableRef() TableRef
}

type SelectAllSource interface {
    TableSource
    GrizSelectAllColumns() []expr.SelectableColumn
    GrizSelectAllFieldKeys() []string
}
```

Rules:

- generated param-capable column metadata must have a non-empty generated `SelectionKey`, a non-nil `GoType`, and a non-nil, non-typed-nil `ParamEncoder`; use an explicit no-op encoder for raw driver binding rather than nil. Bare derived columns that cannot provide a valid property key/type/encoder must not implement `ParamColumn`.
- `InsertColumnMeta`, `MySQLReturningIDColumnMeta`, and `UpdateColumnMeta` must carry non-empty valid SQL names, non-nil `GoType`, and non-nil, non-typed-nil encoders. `InsertColumnMeta.PropertyKey`, `MySQLReturningIDColumnMeta.PropertyKey`, and `GrizTableColumnKeys()` entries are generated table-property keys in RC.1 order. Generated select-all field keys are generated source property keys: table property keys for table sources and view property keys for view sources. Explicit SELECT aliases are selection keys and are also rendered SQL labels for generic `db`-tag scanning, but they are not generated table/view property keys. `MySQLReturningIDColumnMeta.Primary` must be true; non-primary returning-id metadata fails with `build_validation`. Invalid metadata from generated or custom table sources fails with `build_validation` before rendering, planning, encoding, or validation.
- generated base table handles for `pg.SchemaTable("audit", "logs", ...)` must return `TableRef{Schema: "audit", Name: "logs", Alias: ""}`. Rendering falls back to `Name` when `Alias` is empty; non-empty `Alias` is reserved for explicit alias-bearing handles, including `As(alias)`, subquery aliases, and CTE references.
- `As(alias)` must preserve `Schema` and `Name` while changing only `Alias`
- CTEs and subqueries must return an empty `Schema` and use their alias/name as `Name` and `Alias`
- `FROM`, `JOIN`, `INSERT`, `UPDATE`, `DELETE`, relation joins, lock `OF`, and codegen examples must render table identity through `TableRef`; no query builder may concatenate schema and table into one identifier string
- normal table-source SQL rendering is package-owned. Generated tables/views and aliases expose `TableRef` identity only; query builders render them through identifier quoting and `Alias` fallback rules. Package-owned `CTERef` and `FromSubquery` wrappers implement an unexported internal renderer for CTE and subquery SQL so external `TableSource` implementations cannot render arbitrary raw SQL through `FROM` / `JOIN`.
- `SelectAllSource` is the generated/source-metadata contract for Drizzle RC.1 no-arg `select().from(source)` parity. Generated base tables, generated views, and read-only aliases must implement it with RC.1 selected-field order, duplicate-free field keys, and caller-owned slices or builder-side deep copies. Query builders must validate that the column and key slice lengths match, that every selected column is non-nil and renderable, that every key is non-empty and duplicate-free, and that `cols[i].SelectionKey() == keys[i]` for every entry before rendering or result mapping. `GrizSelectAllFieldKeys()` is the authoritative key slice for no-arg select-all metadata, with column `SelectionKey()` used as a consistency check. Every select-all column must be a generated source-owned column handle whose `BaseColumnRef().Table` matches the active source `TableRef`, including the alias for read-only alias handles; raw expressions, derived columns, wrappers, wrong-source columns, and mutation-only handles must fail with `build_validation` unless a future trusted custom-source escape hatch is explicitly specified.
- `GrizSelectAllFieldKeys()` returns RC.1 selected-field source property keys used for selected-field identity, insert-from-select key comparison, and generated result-shape planning. It does not change SQL column labels. Normal SQL row scanners still map returned column labels to Go struct `db` tags; property-key scanning is used only by APIs that explicitly say so, such as MySQL `.ReturningID()` synthesized rows.
- derived sources created by `FromSubquery` or `CTERef` do not initially implement `SelectAllSource`; no-arg `query.Select().From(query.FromSubquery(...))` and no-arg `query.Select().From(query.CTERef(...))` are **DEVIATION:GAP (designed)** until Grizzle has a generated/typed selected-field propagation model for derived sources. Initial callers must project derived columns explicitly with `expr.ColBase{TableAlias: alias, ColName: name}` or another explicit selectable expression.
- SELECT builders must maintain an active source registry keyed by the rendered source key, `TableRef.Alias` with `Name` fallback. Duplicate joined aliases matching RC.1's duplicate-alias case fail with `build_validation`. Grizzle must also reject broader `FROM`/`JOIN` source-key collisions as **DEVIATION:INTENTIONAL** fail-fast safety hardening to keep lock `OF`, joins, CTE references, and result mapping unambiguous.
- legacy `GrizTableName()` / `GrizTableAlias()` helpers may remain as compatibility shims for unqualified current code, but they are not sufficient for the target schema-qualified contract
- conformance tests must prove that base columns with empty `TableRef.Alias` pass mutation ownership checks, `As("users")` alias columns fail mutation checks even when the alias equals the base table name, wrappers fail, and schema/name mismatches fail

Package-owned table-source helpers:

```go
func CTERef(name string) TableSource
func FromSubquery(sel SelectLike, alias string) TableSource
func (b *SelectBuilder) With(name string, sub SelectLike) *SelectBuilder
```

`CTERef(name)` creates a package-owned source for a CTE name registered in the active statement CTE namespace. `FromSubquery(sel, alias)` creates a package-owned source that renders the prepared-capable nested select in parentheses plus the quoted alias, propagating fixed and runtime params through the parent build context. `With(name, sub)` registers a CTE for the root select statement and validates the same identifier grammar as `CTERef`. During rendering, the root statement's CTE namespace is propagated into nested `SelectLike` rendering so subqueries and CTE bodies may reference visible root CTEs, matching RC.1's alias-based `WithSubquery` behavior. Empty names, invalid aliases, nil or typed-nil `SelectLike` values, missing prepared-render support, duplicate CTE/source keys in the active namespace, `CTERef` names that do not match a visible `With` registration, and use of these wrappers as mutation/count/index/lock sources fail with `build_validation` when the root builder is built. A same-name real table does not satisfy `CTERef` registration; `query.CTERef("users")` must still fail unless `With("users", ...)` is visible in the active CTE namespace.

**Status:** PARITY for schema-qualified table identity with Drizzle RC.1 `pgSchema(...).table(...)`; generated select-all sources, derived-source gaps, CTE namespace rules, and source-key hardening are labeled separately above.

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

No-arg `query.Select().From(source)` is PARITY for generated tables, generated views, and generated read-only aliases that implement `SelectAllSource`. No-arg derived-source selection is a designed gap; callers must explicitly project derived columns until selected-field propagation for `FromSubquery` and `CTERef` is specified.

**Status:** PARITY for generated select-all sources; **DEVIATION:GAP (designed)** for no-arg derived-source select-all propagation.

### Column aliasing in SELECT

| Drizzle | Grizzle | Status |
|---|---|---|
| `col.as('alias')` in select object | `expr.ColAs(col, "alias")` | PARITY — Drizzle v1.0.0-beta.1+ exposes `.as()` on columns; Grizzle uses `expr.ColAs(col, alias)` |

`expr.ColAs` is the canonical initial public helper. Generated columns may also expose `.As(alias)` as an equivalent convenience, but docs and conformance tests should use one canonical form unless both are intentionally implemented. The alias wrapper adds an `AS alias` clause in the SELECT list. In ORDER BY and GROUP BY contexts the alias is stripped and only the underlying column reference is emitted, matching SQL standard behavior. See the GROUP BY and subquery sections for details.

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
| `arrayContains(col, vals)` | DEVIATION:GAP (designed) — add array-column containment helper with empty-array validation | — |
| `arrayContained(col, vals)` | DEVIATION:GAP (designed) — add array-column contained-by helper with empty-array validation | — |
| `arrayOverlaps(col, vals)` | DEVIATION:GAP (designed) — add array-column overlap helper with empty-array validation | — |
| `isNull(col)` | `col.IsNull()` | PARITY |
| `isNotNull(col)` | `col.IsNotNull()` | PARITY |
| `between(col, lo, hi)` | `col.Between(lo, hi)` | PARITY |
| `notBetween(col, lo, hi)` | DEVIATION:GAP (designed) — add `.NotBetween()` to numeric/timestamp columns | — |
| `and(...exprs)` | `expr.And(exprs...)` | PARITY |
| `or(...exprs)` | `expr.Or(exprs...)` | PARITY |
| `not(expr)` | `expr.Not(expr)` | PARITY |
| `sql\`raw\`` | `expr.Raw(str)` | PARITY for unparameterized strings |
| Parameterized `sql\`... ${val}\`` | `expr.RawArgs(sql, args...)` | DEVIATION:LANGUAGE — see below |
| `exists(subquery)` | `query.Exists(sub)` | PARITY |
| `notExists(subquery)` | `query.NotExists(sub)` | PARITY |

RC.1 condition edge behavior:

- `col.In()` with zero values renders a false predicate; `col.NotIn()` with zero values renders a true predicate, matching Drizzle's empty `inArray` / `notInArray` handling
- array condition helpers reject empty arrays with `build_validation` when implemented; they must not silently render a broad predicate
- `expr.And(...)` and `expr.Or(...)` drop nil conditions, return nil/no predicate for zero effective conditions, and return the single condition unchanged for one effective condition
- `Where(nil)` and `Having(nil)` omit the clause; nil conditions inside `Where(expr.And(...))` / `Having(expr.And(...))` follow the same filtering rule
- nil or typed-nil `expr.Expression`, `SelectableColumn`, `TableSource`, `ParamColumn`, `SelectLike`, or other interface values are omitted only in positions explicitly documented as optional conditions. In required positions, builders must detect nil/typed-nil values before calling interface methods and record `build_validation`.

### ORDER BY

| Drizzle | Grizzle | Status |
|---|---|---|
| `asc(col)` / `col.asc()` | `col.Asc()` | PARITY |
| `desc(col)` / `col.desc()` | `col.Desc()` | PARITY |

RC.1 query `ORDER BY` does not expose `nullsFirst()` / `nullsLast()` helpers. Those names belong to PostgreSQL index extra configuration, not query ordering. If Grizzle adds query-order null placement helpers later, they are `GRIZZLE-ONLY` / future extension rather than RC.1 parity.

### LIMIT / OFFSET

**Status:** PARITY

### GROUP BY / HAVING

**Status:** PARITY

Note: Drizzle's TypeScript type system prevents passing an aliased column (e.g. `sql<...>.as("alias")`) to `.groupBy()` — the type only accepts `Column | SQL`. In Grizzle, `expr.ColAs` satisfies `SelectableColumn` and can be passed to `.GroupBy()`. Grizzle strips the alias before rendering so only the underlying column reference appears in the GROUP BY clause, matching the SQL Drizzle would generate. This is correct per standard SQL: `GROUP BY` does not accept `AS` aliases (fix #131).

Untyped `expr.ColBase{TableAlias, ColName}` references are allowed for derived CTE/subquery columns and join aliases when only SQL rendering is needed. They are not `sqlmeta.ParamColumn` values and cannot provide prepared-parameter type/encoder/nullability metadata.

### JOINs

| Drizzle | Grizzle | Status |
|---|---|---|
| `.leftJoin(tbl, on)` | `.LeftJoin(tbl, on)` | PARITY |
| `.innerJoin(tbl, on)` | `.InnerJoin(tbl, on)` | PARITY |
| `.rightJoin(tbl, on)` (PostgreSQL / MySQL / SQLite) | `.RightJoin(tbl, on)` for PostgreSQL, MySQL, and SQLite engines known to support RIGHT JOIN (SQLite 3.39+) | PARITY for RC.1 builder surface and rendered SQL on capable SQLite engines; **DEVIATION:INTENTIONAL** capability hardening for fail-fast gating on older SQLite |
| `.fullJoin(tbl, on)` (PostgreSQL / SQLite in the initial Grizzle-supported dialect set) | `.FullJoin(tbl, on)` for PostgreSQL and SQLite engines known to support RIGHT/FULL JOIN (SQLite 3.39+) | PARITY for RC.1 builder surface and rendered SQL on capable SQLite engines; **DEVIATION:INTENTIONAL** capability hardening for fail-fast gating on older SQLite |
| no MySQL `.fullJoin()` | no MySQL `.FullJoin()` parity surface, or a MySQL fast-fail if a shared Go builder exposes it | PARITY for omission; **DEVIATION:LANGUAGE** if exposed only to fail fast |
| `.crossJoin(tbl)` | DEVIATION:GAP (designed) — add `.CrossJoin(tbl)` with no ON condition | — |
| *(no Drizzle equivalent)* | `.JoinRel(rel)` | GRIZZLE-ONLY — see below |
| *(no Drizzle equivalent)* | `.InnerJoinRel(rel)` | GRIZZLE-ONLY — see below |

RC.1 also exposes `rightJoin` / `fullJoin` for additional dialect packages such as Cockroach, MSSQL, and SingleStore where applicable. Those dialects are outside the initial Grizzle implementation scope unless their dedicated dialect specs are added.

Explicit-column joins are PARITY for the initial target. Drizzle RC.1 no-arg joins also derive a nested selected-field result shape from `selectedFields`, move the base source under its source key on the first join, add joined source fields under their source keys, and adjust nullability by join type. Grizzle does not yet specify the Go result-shape/scanner model for that nested no-arg join selection. Therefore `query.Select().From(db.UsersT).InnerJoin(db.RealmsT, ...)` and other no-arg joined selects are **DEVIATION:GAP (not designed)** until selected-field nesting, generated scan structs or projection structs, SQL labels, and join nullability mapping are specified. Initial examples and conformance tests must use explicit projections for joined queries.

Dialect capability methods must split right and full join support:

```go
type JoinCapabilities interface {
    SupportsRightJoin() bool
    SupportsFullJoin() bool
}
```

PostgreSQL returns true for both. MySQL returns true for `SupportsRightJoin` and false for `SupportsFullJoin`. SQLite returns true for both only when the selected driver/engine is known to be SQLite 3.39+; otherwise shared builders must fail with `unsupported_feature`.

### MySQL index hints

Drizzle RC.1 MySQL select builders support index hints on `from(table, config)` and table joins through `useIndex`, `forceIndex`, and `ignoreIndex`.

Target Go shape:

```go
type MySQLIndexHintKind string

const (
    MySQLUseIndex    MySQLIndexHintKind = "use"
    MySQLForceIndex  MySQLIndexHintKind = "force"
    MySQLIgnoreIndex MySQLIndexHintKind = "ignore"
)

type MySQLIndexHint struct {
    Kind    MySQLIndexHintKind
    Indexes []MySQLIndexRef
}

type MySQLIndexRef interface {
    MySQLIndexName() string
}

type MySQLTableSource interface {
    TableSource
    GrizMySQLTableSource()
}

func MySQLSelect(cols ...expr.SelectableColumn) *MySQLSelectBuilder
func IndexName(name string) MySQLIndexRef
func UseIndex(indexes ...MySQLIndexRef) MySQLIndexHint
func ForceIndex(indexes ...MySQLIndexRef) MySQLIndexHint
func IgnoreIndex(indexes ...MySQLIndexRef) MySQLIndexHint

func (b *MySQLSelectBuilder) FromWithIndexHints(table MySQLTableSource, hints ...MySQLIndexHint) *MySQLSelectBuilder
func (b *MySQLSelectBuilder) LeftJoinWithIndexHints(table MySQLTableSource, on expr.Expression, hints ...MySQLIndexHint) *MySQLSelectBuilder
func (b *MySQLSelectBuilder) InnerJoinWithIndexHints(table MySQLTableSource, on expr.Expression, hints ...MySQLIndexHint) *MySQLSelectBuilder
func (b *MySQLSelectBuilder) RightJoinWithIndexHints(table MySQLTableSource, on expr.Expression, hints ...MySQLIndexHint) *MySQLSelectBuilder
func (b *MySQLSelectBuilder) CrossJoinWithIndexHints(table MySQLTableSource, hints ...MySQLIndexHint) *MySQLSelectBuilder
```

Rules:

- index references may come from generated index/unique handles or from trusted `IndexName(name)` literals; string names are a Go-language escape hatch and must follow the string identifier rules in this spec
- index hints apply only to MySQL table-or-alias handles that implement the read-only `MySQLTableSource` marker, matching Drizzle RC.1's `MySqlTable`/alias path for SELECT and JOIN hints. Subqueries, views, CTEs, raw table expressions, and custom insert-only sources that are not generated base-table/read-alias handles must not satisfy `MySQLTableSource`, and any shared escape hatch must reject them with `unsupported_feature`.
- `MySQLTableSource` is an exported generated-code convention, not a security boundary. Only generated base-table and read-only alias handles should implement it; custom implementations are trusted extensions outside strict RC.1 parity. Builders must still validate nil/typed-nil sources, dialect support, and `TableRef` identifier components before rendering.
- hints are MySQL-only; non-MySQL builders must omit these APIs, and shared builders must fail with `unsupported_feature`
- `CrossJoinWithIndexHints` is part of MySQL index-hint parity, but its implementation is blocked by the general `CrossJoin` gap above
- index hint validation is performed during `Build`: every index ref must be non-nil, must not be a typed-nil interface value, and must have a valid name; zero-value/manual `MySQLIndexHint` values with unknown `Kind` fail `build_validation`
- empty hint groups are omitted; multiple hint groups of the same kind are merged in call order; rendering order is RC.1-compatible: `USE INDEX`, then `FORCE INDEX`, then `IGNORE INDEX`
- exact Go method names are DEVIATION:LANGUAGE because Go lacks TypeScript-style config object overloads, but the rendered SQL behavior is an RC.1 parity target

**Status:** DEVIATION:GAP (designed) until implemented; required for full MySQL RC.1 SELECT/JOIN parity.

### DISTINCT

| Drizzle | Grizzle | Status |
|---|---|---|
| `.distinct()` | `.Distinct()` | PARITY |
| `.distinctOn(cols)` (PostgreSQL / Cockroach) | `.DistinctOn(cols...)` | PARITY only for PostgreSQL-compatible dialects; MySQL and SQLite must reject or omit this API rather than silently degrading |

### CTEs (Common Table Expressions)

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

Non-recursive SELECT CTE SQL behavior is Drizzle RC.1 parity. The Go API shape, `With(name, sub)` plus `CTERef(name)`, is **DEVIATION:LANGUAGE** from RC.1's `$with(alias).as(...)` and `db.with(...queries)` object flow.

Supported by all built-in dialects (PostgreSQL, MySQL 8.0+, SQLite 3.8.3+) for non-recursive CTEs. When a custom dialect returns `SupportsCTE() = false`, the builder must fail fast or omit the API; it must not silently emit dangling CTE references.

Drizzle RC.1 also carries `withList` into mutation builders where the dialect builder exposes that path: PostgreSQL/Cockroach and SQLite expose insert/update/delete builders from `db.with(...)`; returning/selectability affects result typing, not whether `WITH` is available. MySQL exposes update/delete builders from `db.with(...)` in the reviewed source, while insert CTE flow is not exposed in the reviewed MySQL builder. Grizzle's initial public `With` target is SELECT-only; mutation CTE builders are **DEVIATION:GAP (not designed)** until dialect-scoped insert/update/delete CTE APIs and return/result rules are specified.

`WithRecursive` has no RC.1 public database API equivalent and is not part of the initial target. If Grizzle later adds a recursive CTE helper, it must be documented as GRIZZLE-ONLY and specified separately.

**Status:** PARITY for non-recursive SELECT CTE SQL behavior; **DEVIATION:LANGUAGE** for the Go `With`/`CTERef` API shape; **DEVIATION:GAP (not designed)** for mutation CTE builders. Recursive CTE helpers are outside the initial target and would be future GRIZZLE-ONLY helpers if specified.

### FOR UPDATE / row locking

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
query.Select().From(db.UsersT).ForNoKeyUpdate() // PostgreSQL-compatible; Cockroach RC.1 also supports, out of initial scope
query.Select().From(db.UsersT).ForKeyShare()    // PostgreSQL-compatible; Cockroach RC.1 also supports, out of initial scope
query.Select().From(db.UsersT).For(query.LockForUpdate, query.SkipLocked())
query.Select().From(db.UsersT).For(query.LockForShare, query.NoWait())
query.Select().From(db.UsersT).ForUpdate().Of(db.UsersT)
```

`ForUpdate()`, `ForShare()`, `ForNoKeyUpdate()`, and `ForKeyShare()` are convenience wrappers around `For(lockStrength, opts...)`.

Only PostgreSQL-compatible clauses are emitted for PostgreSQL-family dialects and MySQL-valid clauses for MySQL. MySQL RC.1 supports only `update` and `share` lock strengths, rendered as `FOR UPDATE` and `FOR SHARE`; it supports `noWait` and `skipLocked` but does not expose PostgreSQL's `of` lock config. SQLite has no row-locking `for` API. `FOR NO KEY UPDATE`, `FOR KEY SHARE`, and lock `OF` table lists are PostgreSQL-compatible features in the reviewed RC.1 builders, including PostgreSQL and Cockroach; Cockroach is outside the initial Grizzle scope unless a dedicated dialect spec is added. Grizzle must gate unsupported lock modes and options explicitly rather than documenting silent degradation as Drizzle parity.

Target lock `OF` API:

```go
type PGLockTableSource interface {
    TableSource
    GrizPGLockTableSource()
}

func (b *SelectBuilder) Of(tables ...PGLockTableSource) *SelectBuilder
```

`Of(...)` accepts active selected table sources in the query's `FROM` or `JOIN` tree that implement `PGLockTableSource`. Generated code emits this marker only for PostgreSQL table handles and aliases; it does not emit it for views, CTEs, subqueries, raw table expressions, or non-PostgreSQL table handles. `PGLockTableSource` is an exported generated-code convention, not a security boundary; custom implementations are trusted extensions outside strict RC.1 parity. Builders must still verify the source against the active query source registry and must not rely on the marker alone. Nil/typed-nil table sources, empty table lists, tables not present in the active query, CTE/subquery alias mismatches, and invalid table identifiers fail with `build_validation`. Active-query membership validation is **DEVIATION:INTENTIONAL** fail-fast safety hardening over RC.1, which renders the supplied `PgTable` and leaves invalid membership to PostgreSQL. Builders must validate `SupportsLockOf()` after lock mode resolution and return `unsupported_feature` for dialects that do not support lock `OF`; they must not silently omit the table list. Rendering uses each table source's active `TableRef.Alias`, falling back to `TableRef.Name`, and never a column `BaseColumnRef`.

Calling `Of(...)` without a lock mode must produce a build validation error, or the typed builder chain must make that call order impossible.

**Status:** PARITY for dialect-supported lock strengths; **DEVIATION:LANGUAGE** only where Go convenience wrappers expose a broader method set and fail fast on unsupported dialects.

### Set operations

| Drizzle | Grizzle | Status |
|---|---|---|
| `union(q1, q2)` | `query.Union(q1, q2)` | PARITY |
| `unionAll(q1, q2)` | `query.UnionAll(q1, q2)` | PARITY |
| `intersect(q1, q2)` | `query.Intersect(q1, q2)` | PARITY |
| `intersectAll(q1, q2)` (PostgreSQL / MySQL) | `query.IntersectAll(q1, q2)` for PostgreSQL / MySQL | PARITY |
| `except(q1, q2)` | `query.Except(q1, q2)` | PARITY |
| `exceptAll(q1, q2)` (PostgreSQL / MySQL) | `query.ExceptAll(q1, q2)` for PostgreSQL / MySQL | PARITY |
| no SQLite `intersectAll` / `exceptAll` exports | no SQLite `IntersectAll` / `ExceptAll` parity surface, or SQLite fast-fail if a shared Go builder exposes them | PARITY for omission; **DEVIATION:LANGUAGE** if exposed only to fail fast |
| `.orderBy()` on set op | `.OrderBy()` | PARITY — Drizzle strips table qualifiers from `PgColumn` refs automatically; Grizzle does the same via `ToSQLUnqualified` |
| `.limit()` on set op | `.Limit()` | PARITY |

### Subqueries

| Drizzle | Grizzle | Status |
|---|---|---|
| `db.select().from(subquery)` | `query.Select(cols...).From(query.FromSubquery(sub, "alias"))` | PARITY for explicit projections from a derived source; no-arg derived-source select-all is **DEVIATION:GAP (designed)** |
| Correlated subquery in WHERE | `query.Exists(sub)` / `query.NotExists(sub)` | PARITY |
| Subquery in SELECT list | DEVIATION:GAP (not designed) | RC.1 supports `sub.As(alias)` / aliased SQL in the select object; Grizzle needs a scalar-subquery selectable helper with single-field validation, alias handling, result-key mapping, and scan type rules before claiming parity |
| `inArray(col, subquery)` — col IN (SELECT ...) | `query.SubqueryIn(col, sub)` | PARITY — when `col` is an `expr.ColAs`, the alias is stripped before rendering; the IN left-hand side is a column reference and must not carry an AS clause (fix #131) |
| `notInArray(col, subquery)` — col NOT IN (SELECT ...) | `query.SubqueryNotIn(col, sub)` | PARITY — same alias-stripping behavior as SubqueryIn (fix #131) |
| PostgreSQL `leftJoinLateral`, `innerJoinLateral`, `crossJoinLateral` | DEVIATION:GAP (not designed) | Required for full PostgreSQL RC.1 parity |
| MySQL `leftJoinLateral`, `innerJoinLateral`, `crossJoinLateral` | DEVIATION:GAP (not designed) | Required for full MySQL RC.1 parity |
| SQLite lateral join helpers | no RC.1 parity surface in reviewed source | PARITY for omission |

### Window functions

Drizzle RC.1 does not expose public typed helpers such as `rowNumber().over(...)` or `rank().over(...)`. Users express window functions through raw `sql` fragments. Grizzle's typed window builders are optional Go conveniences over that raw SQL capability, not RC.1 public-helper parity.

| Drizzle RC.1 | Grizzle | Status |
|---|---|---|
| raw `` sql`row_number() over (...)` `` / `` sql`rank() over (...)` `` | typed wrappers such as `expr.RowNumber().PartitionBy(...).OrderBy(...)` and `expr.Rank()` | GRIZZLE-ONLY typed convenience over raw SQL parity |
| raw `` sql`lag(${col}) over (...)` `` / `` sql`lead(${col}) over (...)` `` | `expr.Lag(col, ...)`, `expr.Lead(col, ...)` | GRIZZLE-ONLY typed convenience over raw SQL parity |
| raw `` sql`first_value(${col}) over (...)` `` and related navigation functions | `expr.FirstValue(col)`, `expr.LastValue(col)`, `expr.NthValue(col, n)`, `expr.Ntile(n)`, `expr.PercentRank()`, `expr.CumeDist()` | GRIZZLE-ONLY typed convenience over raw SQL parity |
| raw SQL `PARTITION BY` / `ORDER BY` inside `OVER (...)` | `.PartitionBy(cols...)` / `.OrderBy(cols...)` on typed window expressions | GRIZZLE-ONLY typed convenience |
| raw SQL frame spec (`ROWS` / `RANGE` / `GROUPS BETWEEN`) | typed frame API | GRIZZLE-ONLY future extension; DEVIATION:GAP (not designed) until the frame API is specified (#139) |
| *(no Drizzle equivalent)* | `expr.WinSum(col)` | GRIZZLE-ONLY — see below |
| *(no Drizzle equivalent)* | `expr.WinAvg(col)` | GRIZZLE-ONLY — see below |
| *(no Drizzle equivalent)* | `expr.WinCount()` | GRIZZLE-ONLY — see below |
| *(no Drizzle equivalent)* | `expr.UnboundedPreceding()` / `expr.CurrentRow()` / `expr.UnboundedFollowing()` | GRIZZLE-ONLY — sentinels for future frame API; see #139 |

### Aggregates

| Drizzle | Grizzle | Status |
|---|---|---|
| `count()` | `expr.Count()` | PARITY |
| `count(col)` | `expr.CountCol(col)` | PARITY |
| `countDistinct(col)` | `expr.CountDistinct(col)` | PARITY |
| `sum(col)` | `expr.Sum(col)` | PARITY |
| `sumDistinct(col)` | `expr.SumDistinct(col)` | PARITY required |
| `avg(col)` | `expr.Avg(col)` | PARITY |
| `avgDistinct(col)` | `expr.AvgDistinct(col)` | PARITY required |
| `max(col)` | `expr.Max(col)` | PARITY |
| `min(col)` | `expr.Min(col)` | PARITY |

### DB-level `$count`

Drizzle RC.1 exposes `db.$count(source, filters?)` across PostgreSQL, MySQL, and SQLite. It can execute directly and can also be used as a selectable subquery, for example inside a larger `select({ postsCount: db.$count(posts, ...) })`.

Target Go shape:

```go
type CountSource interface {
    TableSource
    GrizCountSource()
}

type RawCountSource interface {
    grizRawCountSource()
    RenderRawCountSourceSQL(ctx *expr.BuildContext) (string, error)
    RenderPreparedRawCountSourceSQL(ctx *expr.PreparedBuildContext) (string, error)
}

type CountQuery struct {
    // opaque
}

func CountRows(source CountSource, filters ...expr.Expression) CountQuery
func CountRowsRaw(source RawCountSource, filters ...expr.Expression) CountQuery
func RawCountSQL(sql string, args ...any) RawCountSource
func RawCountSubquery(source SelectLike, alias string) RawCountSource
func (q CountQuery) As(alias string) expr.SelectableColumn
func (q CountQuery) Build(d dialect.Dialect) (sql string, args []any, err error)
func (q CountQuery) grizPreparedBuilder()
func (q CountQuery) BuildPrepared(d dialect.Dialect) (sql string, plan []PreparedArg, err error)
func (q CountQuery) PreparedResultKind() PreparedResultKind
```

Generated tables and views implement `CountSource` through table/view identity, not arbitrary SQL rendering. `CountRows` renders the source `TableRef` with dialect identifier quoting and rejects nil/typed-nil sources or invalid identifier parts before SQL generation. Raw SQL and subquery sources are available only through sealed `RawCountSource` wrappers constructed by helpers such as `RawCountSQL` and `RawCountSubquery`. This covers RC.1's `SQL | SQLWrapper` source categories through package-owned wrappers; the restriction against arbitrary external SQLWrapper-equivalent implementations is **DEVIATION:INTENTIONAL** safety hardening to preserve the raw-source trust boundary. `RawCountSQL` follows the same trust and placeholder rules as `expr.RawArgs`: the SQL template is trusted schema/application code, values are passed through `args`, and prepared raw params use the documented raw-param path. `RawCountSubquery` requires a non-nil prepared-capable `SelectLike` source and a non-empty alias that validates as one identifier part; rendering wraps the subquery in parentheses and quotes the alias for PostgreSQL/MySQL/SQLite correctness. Constructors reject nil/typed-nil sources, invalid aliases, and unsupported raw argument shapes with `build_validation`.

The zero value `query.CountQuery{}` is invalid. `Build`, `BuildPrepared`, and direct driver scalar count helpers must reject zero-value or otherwise uninitialized `CountQuery` values with `build_validation`. Because `As(alias string)` has no error return, calling `As` on a zero-value `CountQuery` returns an invalid selectable that must fail with `build_validation` when its render/build path is reached.

Driver packages for supported dialects must expose a direct scalar helper that accepts only a prebuilt `query.CountQuery`, for example:

```go
func Count(ctx context.Context, db *DB, q query.CountQuery) (int64, error)
func CountTx(ctx context.Context, tx *Tx, q query.CountQuery) (int64, error)
```

Callers construct table/view counts with `query.CountRows(source, filters...)` and raw/subquery counts with `query.CountRowsRaw(rawSource, filters...)`; driver helpers must not accept `any`, `string`, `expr.Expression`, `CountSource`, or `RawCountSource` directly. This keeps scalar execution parity for both source families without weakening the trust boundary or nil/typed-nil validation. The query package must also expose the selectable `CountQuery` shape so correlated count subqueries have an RC.1 parity path. `CountQuery` is a row-returning prepared builder when built directly; `PreparedResultKind()` returns `PreparedResultRows`. `filters` follow the same nil-filter and prepared-rendering rules as `Where`.

**Status:** DEVIATION:GAP (designed) until `CountRows`, raw count-source wrappers, and mandatory driver direct scalar helpers exist; DEVIATION:LANGUAGE for returning Go `int64` instead of JavaScript `number`.

Type note:

- Drizzle RC.1 maps `count()` / `countDistinct()` to `number`
- Drizzle RC.1 maps `avg()` / `avgDistinct()` and `sum()` / `sumDistinct()` to `SQL<string | null>`
- Drizzle RC.1 maps `max()` / `min()` to the source column data type where possible
- Grizzle scan result types are determined by the Go destination field, so docs must not claim that `avg()` or `sum()` infer the input column's numeric type

### CASE expressions

**Status:** PARITY (searched CASE and simple CASE both implemented)

## INSERT

### Single row / multiple rows

Shared insert value contract:

```go
type InsertColumnMeta struct {
    Name              string
    PropertyKey       string
    GoType            reflect.Type
    Encoder           ParamEncoder
    Required          bool
    Nullable          bool
    Primary           bool
    AutoIncrement     bool
    HasStaticDefault  bool
    StaticDefaultSQL  string
    HasRuntimeDefaultUnsupported bool
    HasRuntimeOnUpdateUnsupported bool
}

type MySQLReturningIDColumnMeta struct {
    Name              string
    PropertyKey       string
    GoType            reflect.Type
    Encoder           ParamEncoder
    Nullable          bool
    Primary           bool
    AutoIncrement     bool
    HasRuntimeDefaultUnsupported bool
}

type UpdateColumnMeta struct {
    Name                            string
    GoType                          reflect.Type
    Encoder                         ParamEncoder
    Nullable                        bool
    HasRuntimeOnUpdateUnsupported   bool
}

type InsertTableSource interface {
    TableSource
    GrizInsertTableSource()
    GrizInsertColumns() []InsertColumnMeta
    GrizTableColumnKeys() []string
}

type MySQLInsertTableSource interface {
    InsertTableSource
    GrizMySQLInsertTableSource()
    GrizMySQLReturningIDColumns() []MySQLReturningIDColumnMeta
}

type UpdateTableSource interface {
    TableSource
    GrizUpdateTableSource()
    GrizUpdateColumns() []UpdateColumnMeta
}

func InsertInto(table InsertTableSource) *InsertBuilder
func (b *InsertBuilder) Values(rows ...any) *InsertBuilder
func (b *InsertBuilder) ValueSlice(rows any) *InsertBuilder
```

`Values(rows ...any)` accepts one or more row structs/maps, appends them to the builder, rejects zero arguments, and rejects slice/array arguments so typed slices must use `ValueSlice(rows)`. Repeated `Values(...)` calls are a Go-language convenience that append rows to the same insert builder; Drizzle RC.1 accepts either one object or an array in a single `.values(...)` call. `ValueSlice(rows)` accepts only non-nil slice or array values of row structs/maps and records build validation errors for non-slice, nil, empty, or invalid element inputs.

`InsertColumnMeta.PropertyKey` is the generated table-property key corresponding to Drizzle's table-definition object key; `InsertColumnMeta.Name` is the SQL column name. Initial public INSERT struct tags and map keys remain SQL-name based unless a future property-key row API is explicitly specified. Builders use insert property keys for RC.1 parity metadata such as runtime generated ID tracking and insert-from-select table-key validation, not as an alternate user-input namespace.

Generated `GrizInsertColumns()`, `GrizTableColumnKeys()`, and `GrizMySQLReturningIDColumns()` methods must return caller-owned slices, or builders must deep-copy the returned metadata before validation/rendering. Caller mutation of returned metadata must not affect later builder behavior.

`InsertTableSource` and `UpdateTableSource` are concrete mutation-target interfaces. Generated base table handles implement them; generated aliases, views, CTEs, subqueries, raw table expressions, and read-only wrappers must not implement them. Rejecting generated aliases as mutation targets is **DEVIATION:INTENTIONAL** fail-fast hardening over RC.1's alias-proxy permissiveness and prevents `InsertInto(alias)` / `Update(alias)` from rendering ambiguous mutation SQL against an alias by accident. If a future dialect-specific mutation-alias feature is added, it must be specified separately and labeled as a divergence.

For MySQL generated tables, `GrizMySQLReturningIDColumns()` returns the RC.1 returning-id metadata in generated table-property order. `PropertyKey` is the generated table-property key used by Drizzle `$returningId()` and by Grizzle synthesized returning-id rows; `Name` is the SQL column name. Only primary-key columns that RC.1 can return through auto-increment metadata or RC.1 runtime `$defaultFn` generatedIds metadata belong in this slice; static SQL defaults do not qualify by themselves. Generated code must populate `Primary`, `AutoIncrement`, nullability, Go type, encoder, and runtime-default markers from schema metadata rather than leaving builders to infer returning-id eligibility from SQL column names. Builders use this metadata for `.ReturningID()` planning, property-key scans, and synthesized row validation.

Prepared insert parameter target shape:

```go
type InsertParamValue[T any] struct {
    // opaque; construct through query.InsertParam[T]
}

func InsertParam[T any](name string) InsertParamValue[T]
```

`InsertParamValue[T]` exposes only a package-internal inspection interface to insert builders, equivalent to:

```go
type insertParamCarrier interface {
    grizInsertParam()
    insertParamName() string
    insertParamType() reflect.Type
}
```

Zero-value or manually constructed `InsertParamValue[T]{}` values fail with `build_validation` because the runtime name is empty. `InsertParam[T](name)` validates the runtime-name grammar during build, not at construction, so builders can collect errors consistently. The wrapper is valid only as a direct map/assignment row value; it is not valid inside `query.Assign[T]`. Builders attach column metadata from `InsertColumnMeta` when rendering, require `reflect.TypeFor[T]()` to equal or be assignable to `InsertColumnMeta.GoType` under the same conversion policy as prepared runtime slots, validate nullability and encoder metadata, and call `PreparedBuildContext.AddRuntime` with that metadata. Nullable nil runtime behavior is governed only by the target column metadata. Normal `Build` fails with `build_validation` if unresolved insert params remain.

Accepted row forms:

- structs with `db` tags, using the nullable assignment rules in [types.md](./types.md)
- maps keyed by trusted column-name literals or generated column-name constants
- unknown struct tags, unknown map keys, malformed tags, invalid identifiers, and unsupported field values must record build validation errors
- omitted optional/defaultable fields are allowed to vary across rows; Grizzle must still produce one deterministic table-derived column list for the insert
- map values may be concrete values, `nil` for explicit SQL `NULL`, `expr.Expression` for trusted SQL expressions, `query.Assign[T]`, or the explicit prepared placeholder wrapper `query.InsertParam[T](name)`; `AssignUnset` follows omitted-field behavior, `AssignNull` inserts SQL `NULL`, `AssignValue` binds the concrete value, and `InsertParam` records an encoded runtime parameter for `BuildPrepared`
- explicit SQL `NULL` for a non-null column must record a build validation error before SQL execution
- struct insert rows cannot contain prepared placeholders in the initial target. Map/assignment insert values may use `query.InsertParam[T](name)` only when the target column metadata is known; builders must validate the column, Go type, nullability, encoder, and safe runtime name, then render through `PreparedBuildContext.AddRuntime`. Normal `Build` fails with `build_validation` if unresolved insert params remain.
- PostgreSQL/MySQL missing fields without runtime hook requirements render SQL `DEFAULT`, including nullable fields with no static default
- SQLite missing static-default fields render the generated static default expression/value
- SQLite missing nullable fields without static defaults render SQL `NULL`
- runtime default/on-update hooks have no initial Go equivalent; if an omitted field would require a runtime default or runtime on-update hook, `Build` must fail with `unsupported_feature` rather than inventing a value or silently changing semantics. This is DEVIATION:GAP until a Go hook API is designed.
- if a dialect cannot express the required mixed default/value matrix for a supported row set, `Build` must fail with `unsupported_feature`
- omitted required non-default fields follow Drizzle RC.1 missing-value rendering rather than early rejection: PostgreSQL/MySQL render `DEFAULT` where the dialect supports it, and SQLite uses the documented missing-field expression (`NULL` when no static default is available)

For multi-row inserts, Grizzle must produce a deterministic insertable-column list from generated table metadata, not from the first row's keys. Missing fields in individual rows are represented by the dialect-specific missing-value expression above rather than changing the column list per row, matching Drizzle RC.1's generated insert semantics.

**Status:** PARITY for rendered single-row and multi-row SQL semantics in supported static-default/no-runtime-hook cases; DEVIATION:LANGUAGE for the Go `Values`/`ValueSlice` API split, repeated `Values(...)` append shape, SQL-name `db` tags/map keys instead of RC.1 row-object property keys, and explicit `query.InsertParam[T]` placeholder wrapper; DEVIATION:GAP for omitted runtime default/on-update hooks and struct-row placeholders until a broader generated-row wrapper design exists.

### INSERT ... SELECT

Drizzle RC.1 supports insert-from-select for PostgreSQL, MySQL, and SQLite. MySQL `.ignore()` is staged before `.select(...)`, the same as before `.values(...)`.

Target shape:

```go
type SelectLike interface {
    grizSelectLike()
    RenderSelect(ctx SelectRenderContext) (sql string, err error)
    RenderPreparedSelect(ctx PreparedSelectRenderContext) (sql string, err error)
    SelectedFieldKeys() (keys []string, ok bool)
}

type CTENameSet map[string]struct{}

type SelectRenderContext struct {
    Expr        *expr.BuildContext
    VisibleCTEs CTENameSet
}

type PreparedSelectRenderContext struct {
    Expr        *expr.PreparedBuildContext
    VisibleCTEs CTENameSet
}

func RawSelectSQL(sql string, args ...any) SelectLike
func (b *InsertBuilder) Select(sel SelectLike) *InsertBuilder
func (b *MySQLInsertStartBuilder) Select(sel SelectLike) *MySQLInsertBuilder
```

Rules:

- typed query-builder selects whose selected expressions all provide non-empty `SelectionKey()` values return `(keys, true)` in rendered selection order and must implement both normal and prepared select rendering so `INSERT ... SELECT` can collect fixed and runtime params during `BuildPrepared`. Typed selects containing any empty `SelectionKey()` return `(nil, false)` from `SelectedFieldKeys()` while remaining valid for normal SELECT, EXISTS, CTE, and derived-table rendering.
- `SelectRenderContext.VisibleCTEs` / `PreparedSelectRenderContext.VisibleCTEs` carry an immutable active root CTE namespace through nested select rendering. A builder rendering nested `SelectLike` values must pass a defensive copy of the visible namespace plus its own local `With` names. `CTERef` validation consults this namespace rather than only the immediately rendered builder.
- Direct calls to `RenderSelect` / `RenderPreparedSelect` must validate the context before rendering. Nil `Expr` fails with `build_validation`; nil `VisibleCTEs` means an empty namespace; renderers must never mutate the caller's `VisibleCTEs` map and must defensively copy before adding local `With` names. These render methods are exported only because `SelectLike` is a package-owned interface used by generated/raw wrappers; they are not a user-facing extension point.
- CTE names must be unique within a builder and must not collide with inherited visible CTE names; rejecting local shadowing is **DEVIATION:INTENTIONAL** ambiguity hardening. Existence validation only proves that a `CTERef` name is visible; dependency ordering, forward references, and recursive cycles are left to the database or future `check` graph validation unless a dialect-specific validation rule is explicitly added. Conformance tests must cover root-visible nested CTE refs, CTE-body refs to visible CTEs, missing CTE refs, duplicate local CTE names, and local names colliding with inherited visible names.
- `SelectLike` is intentionally sealed with an unexported method so external callers cannot bypass field-key validation by implementing `SelectedFieldKeys(nil, false)` directly; this sealed raw-SQL trust boundary is **DEVIATION:INTENTIONAL** safety hardening over Drizzle's open SQL overload shape
- `RawSelectSQL(sql, args...)` is the package-owned trusted raw select wrapper for Drizzle RC.1's `.select(SQL)` overload. Its SQL string is trusted application/schema code, must be non-empty, and follows the raw SQL rules in this spec. Literal values must be passed through `args`; prepared raw placeholders may use the documented raw-param path. It implements normal and prepared rendering, returns `SelectedFieldKeys(nil, false)`, and is the only raw SQL shape accepted by `Select`.
- keyed typed select builders used by `Insert.Select` must expose selected-field keys that exactly match the target table definition keys in the same order, matching Drizzle RC.1's `haveSameKeys(tableColumns, selectedFields)` runtime validation. If a typed select returns `(nil, false)` because one or more selected expressions have no key, `Insert.Select` must fail with `build_validation` unless the caller uses the explicit trusted `RawSelectSQL` path whose shape is left to the database.
- insert target table handles expose all table definition keys through `GrizTableColumnKeys()` in RC.1 table-column order separately from insertable-column metadata; `GrizInsertColumns()` remains the insertable-column list used for `VALUES`, while insert-from-select validation uses the all-definition key list
- trusted raw SQL `SelectLike` inputs cannot be structurally validated; they must be accepted only through an explicit raw/trusted API and any field-count/order mismatch is left to the database
- `Values`/`ValueSlice` and `Select` are mutually exclusive on one insert builder
- MySQL `Ignore().Select(sel)` must render `INSERT IGNORE INTO ... SELECT ...`

**Status:** DEVIATION:GAP (designed) until implemented; required for full RC.1 insert parity.

### RETURNING

| Dialect | Drizzle RC.1 behavior | Grizzle target | Status |
|---|---|---|---|
| PostgreSQL | `.returning()` | `.Returning(cols...)` | PARITY |
| SQLite | `.returning()` | `.Returning(cols...)` | PARITY |
| MySQL | no normal `.returning()`; insert builder exposes `$returningId()` | no normal `.Returning()` parity surface for MySQL; `.ReturningID()` helper on the MySQL insert builder | PARITY for omission of normal `.Returning()`; `.ReturningID()` is required target surface with fail-fast synthesis rules below |

Grizzle must not document silent MySQL dropping as parity. If Grizzle keeps a cross-dialect `.Returning()` method that is ignored on MySQL, that is **DEVIATION:LANGUAGE** and must be surfaced as a Go API convenience, not as Drizzle behavior.

MySQL `.ReturningID()` target:

```go
type ReturningIDRow map[string]any // lower-level fallback; see DEVIATION:LANGUAGE note below

type MySQLExecMetadata struct {
    InsertID                         int64
    InsertIDValid                    bool
    AffectedRows                     int64
    AffectedRowsValid                bool
    AffectedRowsIsInsertedCount      bool
    AffectedRowsIsInsertedCountValid bool
    InsertedRows                     int64
    InsertedRowsValid                bool
    DuplicateRows                    int64
    DuplicateRowsValid               bool
    WarningCount                     int64
    WarningCountValid                bool
    SkippedRows                      int64
    SkippedRowsValid                 bool
}

type MySQLReturningIDPlan struct {
    // opaque
}

func (b *MySQLInsertBuilder) ReturningID() *MySQLInsertReturningIDBuilder
func (b *MySQLInsertReturningIDBuilder) OnDuplicateKeyUpdateSet(assignments ...MySQLSetAssignment) *MySQLInsertReturningIDBuilder
func (b *MySQLInsertReturningIDBuilder) GrizRowBuilder()
func (b *MySQLInsertReturningIDBuilder) ResultKind() ResultKind
func (b *MySQLInsertReturningIDBuilder) grizPreparedBuilder()
func (b *MySQLInsertReturningIDBuilder) Build(d dialect.Dialect) (sql string, args []any, err error)
func (b *MySQLInsertReturningIDBuilder) BuildPrepared(d dialect.Dialect) (sql string, plan []PreparedArg, err error)
func (b *MySQLInsertReturningIDBuilder) PreparedResultKind() PreparedResultKind
func (b *MySQLInsertReturningIDBuilder) MySQLReturningIDPlan() (MySQLReturningIDPlan, error)
func (p MySQLReturningIDPlan) Synthesize(meta MySQLExecMetadata) ([]ReturningIDRow, error)
```

Execution helpers for a `MySQLInsertReturningIDBuilder` synthesize one returned row per input row only when the plan and MySQL execution metadata can prove that full mapping. Each returned row contains all plan-declared RC.1 returning-id primary-key field names in generated table-property form, matching RC.1 `$returningId()` object shape. For composite returning-id metadata, Grizzle aggregates all returning-id keys into one `ReturningIDRow` per input row; this is **DEVIATION:INTENTIONAL** source-bug hardening over any RC.1 source path that appends generated IDs once per returning key. If `GrizMySQLReturningIDColumns()` returns no primary-key metadata, `.ReturningID()` follows tagged RC.1 source runtime behavior and returns an empty non-nil result slice; it must not synthesize empty maps per inserted row. `MySQLInsertReturningIDBuilder` is a row-returning direct and prepared builder: `ResultKind()` returns `ResultRows`, and `PreparedResultKind()` returns `PreparedResultRows`, so non-returning-id runtime params in values or duplicate-key assignments remain executable after `.ReturningID()`. `OnDuplicateKeyUpdateSet` is available both before and after `ReturningID()`, matching RC.1's chainability; duplicate-key returning-id execution must reconstruct rows from MySQL execution metadata, not a normal result-row stream.

Auto-increment primary keys are reconstructed from the driver-reported `insertId` and an inserted-row count. For `VALUES` inserts, `MySQLReturningIDPlan` carries a known input row count. For `INSERT ... SELECT`, the plan marks the input row count as unknown/select-derived; `Synthesize` may use `AffectedRows` as the synthesized row count only when `AffectedRowsValid`, `AffectedRowsIsInsertedCountValid`, and `AffectedRowsIsInsertedCount` are all true. A proven inserted-row count of zero returns an empty non-nil result slice without requiring `InsertIDValid`; nonzero auto-increment synthesis requires valid `InsertID`. Select-derived `.ReturningID()` plans must fail with `unsupported_feature` unless every returned key is reconstructable from `insertId` plus row count; non-auto-increment runtime-default/generated-id keys have no per-input `generatedIds` in an insert-select plan.

`MySQLReturningIDPlan` also carries the returning-key metadata, explicit fixed generated-id values for `VALUES` rows, duplicate-key state, insert-ignore state, and nullability/type information needed by driver packages. Drivers must call `Build` or `BuildPrepared` and `MySQLReturningIDPlan` from the same builder state. Explicitly provided fixed values for primary-key fields that RC.1 can carry through `generatedIds` may be returned when they are present in the validated insert row values. This explicit-value path is limited to runtime-default/generated-id returning keys; explicit fixed values for auto-increment returning keys fail with `unsupported_feature` unless a future per-row proof path is specified, because `insertId` plus row count cannot prove arbitrary explicit auto-increment values. Runtime placeholder values for returning-id primary-key fields are not resolved into synthesized rows; if a returning-id key depends on a runtime placeholder and cannot be obtained from execution metadata, `MySQLReturningIDPlan()` or equivalent pre-execution validation fails with `unsupported_feature` before sending SQL. This is **DEVIATION:INTENTIONAL** fail-fast hardening over RC.1's `generatedIds` timing, which can carry unresolved placeholder objects. Omitted JavaScript `$defaultFn` generation has no Go equivalent; Grizzle must fail with `unsupported_feature` for that case until the Go runtime-hook API exists. Arbitrary explicit primary-key row-value reconstruction beyond RC.1 returning-id metadata is not part of the initial Grizzle target.

Synthesized auto-increment IDs start as driver `int64` values and must be checked against `MySQLReturningIDColumnMeta.GoType` before map creation or typed scanning. The generated Go type follows the MySQL column's declared integer type and unsigned policy, not PostgreSQL `serial` defaults. Unsigned values must still be representable by the initial signed `int64` metadata; larger unsigned IDs fail with `unsupported_feature` until the driver metadata contract is widened. Overflow, sign loss, incompatible integer kinds, or non-integer returning metadata fail with redacted `scan_decode` errors rather than truncating.

`Synthesize` must reject zero-value plans, plans not produced by `MySQLReturningIDPlan()`, missing metadata, negative metadata, overflowing metadata, and metadata whose validity flags are false when those fields are required. Duplicate-key updates and `INSERT IGNORE` partial successes can change MySQL `affectedRows` semantics; if the driver metadata cannot map safely to one returned row per input row and all RC.1 returning-id keys, execution fails with `unsupported_feature` rather than fabricating IDs. When a plan contains `INSERT IGNORE`, synthesis requires valid proof that no rows were skipped and no warning-only outcome affected row identity, such as `SkippedRowsValid && SkippedRows == 0` plus `WarningCountValid && WarningCount == 0`; if the input row count is known, `InsertedRowsValid && InsertedRows == inputRowCount` is also sufficient proof. When a plan contains duplicate-key update, synthesis requires explicit inserted-row proof such as `InsertedRowsValid && InsertedRows == inputRowCount` for known input row counts, or `DuplicateRowsValid && DuplicateRows == 0`; an adapter-specific updated-row count of zero alone is never sufficient because no-op duplicate-key updates such as `id = id` can leave updated-row counts at zero while still avoiding insertion. Adapters that cannot obtain the required proof must fail those plan states with `unsupported_feature`. `INSERT IGNORE` skipped rows or ambiguous warning-only outcomes must never return a shortened result set; they either synthesize all input rows or fail. This fail-fast requirement is **DEVIATION:INTENTIONAL** safety hardening for unreconstructable driver result shapes. Generated driver helpers should expose typed returning-id result structs where table metadata is available; the `map[string]any` row is a lower-level **DEVIATION:LANGUAGE** fallback for dynamic code paths. Every `ReturningIDRow` returned by `Synthesize` or `ReturningIDRows*` is a fresh caller-owned map containing only plan-declared RC.1 property keys and decoded values; it must not include SQL identifiers, table names, warnings, raw metadata, or diagnostic payload fields.

### ON CONFLICT / upsert

PostgreSQL and SQLite:

| Drizzle | Grizzle | Status |
|---|---|---|
| `.onConflictDoNothing()` with no target | `.OnConflictDoNothing()` | PARITY |
| `.onConflictDoNothing({ target })` | `.OnConflict(query.ConflictColumn(col), ...).DoNothing()` | PARITY |
| PostgreSQL `.onConflictDoNothing({ target, where })` | `.OnConflict(query.ConflictColumn(col), ...).TargetWhere(expr).DoNothing()` | DEVIATION:GAP (designed) until implemented |
| SQLite `.onConflictDoNothing({ target, where })` | `.OnConflict(query.ConflictColumn(col), ...).DoNothingWhere(expr)` rendering `DO NOTHING WHERE ...` | DEVIATION:GAP (designed) until implemented; preserves RC.1 SQLite placement |
| `.onConflictDoUpdate({target, set})` | `.OnConflict(query.ConflictColumn(col), ...).DoUpdateSetExcluded(cols...)` | PARITY |
| PostgreSQL conflict target as columns | `query.ConflictColumn(col)` | PARITY |
| SQLite conflict target as columns or trusted SQL expression | `query.ConflictColumn(col)` or `query.SQLiteConflictExpr(expr)` | PARITY target for both target shapes; `SQLiteConflictExpr` is DEVIATION:GAP (designed) until implemented |
| Constraint-name conflict target | `.OnConflictConstraint(name)` | GRIZZLE-ONLY / future extension; not RC.1 parity |
| `set` with `excluded` reference | `.DoUpdateSetExcluded(cols...)` | PARITY |
| `set` with arbitrary value | `query.SetValue(columnHandle, val)` passed to `.DoUpdateSet(...)` | PARITY |
| `set` with expression | `query.SetExpr(columnHandle, expr)` passed to `.DoUpdateSet(...)` | PARITY |
| update conflict `targetWhere`, `setWhere`; deprecated RC.1 `where` alias intentionally omitted | `.OnConflict(...).TargetWhere(expr).SetWhere(expr).DoUpdateSet(...)` | DEVIATION:GAP (designed) until implemented; required for full RC.1 parity |
| *(no Drizzle equivalent)* | `.DoUpdateSetStruct(row)` | DEVIATION:LANGUAGE — see below |

`query.SetValue` always binds values. `query.SetExpr` renders trusted expressions. String-name assignment constructors are not part of the canonical examples; if retained, they must be separate trusted-identifier helpers such as `query.SetNameValue(name, val)` / `query.SetNameExpr(name, expr)` and are **DEVIATION:LANGUAGE** conveniences with the string identifier validation rules from this spec.

Conflict-where target API:

```go
type ConflictTarget interface {
    grizConflictTarget()
}

type SetAssignment struct {
    // opaque; construct through query.SetValue/query.SetExpr/query.SetParam helpers
}

type ConflictBuilder struct {
    // opaque
}

func ConflictColumn(column expr.SelectableColumn) ConflictTarget
func ConflictColumnName(name string) ConflictTarget
func SQLiteConflictExpr(target expr.Expression) ConflictTarget
func SetValue(column MutationColumn, value any) SetAssignment
func SetExpr(column MutationColumn, value expr.Expression) SetAssignment
func SetParam(column MutationColumn, name string) SetAssignment

func (b *InsertBuilder) OnConflictDoNothing() *InsertBuilder
func (b *InsertBuilder) OnConflict(first ConflictTarget, rest ...ConflictTarget) *ConflictBuilder
func (c *ConflictBuilder) TargetWhere(where expr.Expression) *ConflictBuilder
func (c *ConflictBuilder) SetWhere(where expr.Expression) *ConflictBuilder
func (c *ConflictBuilder) DoNothing() *InsertBuilder
func (c *ConflictBuilder) DoNothingWhere(where expr.Expression) *InsertBuilder
func (c *ConflictBuilder) DoUpdateSetExcluded(cols ...MutationColumn) *InsertBuilder
func (c *ConflictBuilder) DoUpdateSet(assignments ...SetAssignment) *InsertBuilder
func (c *ConflictBuilder) DoUpdateSetStruct(row any) *InsertBuilder
```

`OnConflictDoNothing()` is the no-target PostgreSQL/SQLite do-nothing path and is separate from `OnConflict(first, rest...)` so zero-target conflict builders are impossible at compile time. `TargetWhere` renders the partial-index conflict target predicate before PostgreSQL `DO NOTHING` or before PostgreSQL/SQLite `DO UPDATE`. `DoNothingWhere` is SQLite-only and renders RC.1's do-nothing predicate after `DO NOTHING`. `SetWhere` renders the update predicate after the `SET` clause and must be called before `DoUpdateSet...`, so `DoUpdateSet...` returns the normal insert builder terminal surface for `Build`, `BuildPrepared`, `Returning`, comments, and driver execution helpers. The deprecated RC.1 update config key `where` is not exposed as a separate Go method in the initial target; callers must choose `TargetWhere` or `SetWhere`, which is a **DEVIATION:LANGUAGE** API cleanup while preserving the non-deprecated RC.1 behavior.

Conflict validation rules:

- nil or typed-nil conflict targets fail with `build_validation`; callers use `OnConflictDoNothing()` for targetless do-nothing conflicts
- `ConflictColumn`, `SetValue`, `SetExpr`, `SetParam`, and `DoUpdateSetExcluded` require concrete generated columns whose `BaseColumnRef().Table` exactly matches the unaliased insert target identity and whose `TableRef.Alias` is empty. SELECT-list alias state is not sufficient for this check. Aliased table columns, alias wrappers, derived expressions, CTE columns, subquery columns, and columns from other tables fail with `build_validation`.
- `ConflictColumnName` is a trusted Go convenience for generated constants or compile-time column-name literals; it must resolve to a column on the insert target table before rendering. It is **DEVIATION:LANGUAGE** from RC.1's typed column object target, not a constraint-name API.
- `SQLiteConflictExpr` is valid only for SQLite conflict targets because RC.1 SQLite accepts `SQLiteColumn | SQL`; PostgreSQL builders must reject it with `unsupported_feature`
- `SetAssignment` is opaque; direct struct literals and zero-value assignments fail with `build_validation`
- `SetWhere(...).DoNothing()` and `SetWhere(...).DoNothingWhere(...)` fail with `build_validation`; `SetWhere` is update-only and valid only with `DoUpdateSetExcluded`, `DoUpdateSet`, or `DoUpdateSetStruct`
- `TargetWhere(...).DoNothing()` is valid only for PostgreSQL; SQLite do-nothing with a predicate must use `DoNothingWhere(...)`, and PostgreSQL rejects `DoNothingWhere(...)` with `unsupported_feature`
- `TargetWhere(...).DoNothingWhere(...)` fails with `build_validation`; SQLite RC.1 do-nothing predicates render only after `DO NOTHING`
- `DoNothingWhere(...)` requires at least one conflict target and is valid only for SQLite; empty-target do-nothing uses `OnConflictDoNothing()`
- `DoUpdateSetExcluded`, `DoUpdateSet`, and `DoUpdateSetStruct` require the non-empty `OnConflict(first, rest...)` target list and fail with `build_validation` for empty update assignments
- all conflict predicates and assignment expressions must implement prepared rendering for `BuildPrepared`; `SetParam` records an encoded runtime parameter and normal `Build` fails with `build_validation` if it reaches rendering with an unresolved runtime parameter
- alias/wrapper/wrong-table rejection for conflict and mutation setters is **DEVIATION:INTENTIONAL** safety hardening over RC.1's runtime permissiveness; it prevents accidental writes through derived or aliased same-table expressions.

MySQL:

| Drizzle RC.1 | Grizzle target | Status |
|---|---|---|
| `.onDuplicateKeyUpdate({ set })` | MySQL insert `.OnDuplicateKeyUpdateSet(assignments...)` with no conflict target | PARITY |
| do-nothing via no-op update such as `{ id: sql\`id\` }` | `.OnDuplicateKeyUpdateSet(query.MySQLSetSelf("id"))` or `.OnDuplicateKeyUpdateSet(query.MySQLSetColSelf(table.ID))`, rendering `` ON DUPLICATE KEY UPDATE `id` = `id` `` | PARITY through the MySQL duplicate-key update surface |

MySQL does not accept a conflict target. Grizzle must not present `.OnConflict(cols...)` as MySQL parity.

Required MySQL insert API shape:

```go
type MySQLSetKind string

const (
    MySQLSetValueKind MySQLSetKind = "value"
    MySQLSetExprKind  MySQLSetKind = "expr"
    MySQLSetSelfKind  MySQLSetKind = "self"
    MySQLSetParamKind MySQLSetKind = "param"
)

type MySQLSetAssignment struct {
    // opaque; construct only through exported MySQLSet* constructors
}

func MySQLSetValue(column string, value any) MySQLSetAssignment
func MySQLSetExpr(column string, value expr.Expression) MySQLSetAssignment
func MySQLSetSelf(column string) MySQLSetAssignment
func MySQLSetColValue(column MutationColumn, value any) MySQLSetAssignment
func MySQLSetColExpr(column MutationColumn, value expr.Expression) MySQLSetAssignment
func MySQLSetColSelf(column MutationColumn) MySQLSetAssignment
func MySQLSetColParam(column MutationColumn, name string) MySQLSetAssignment

func MySQLInsertInto(table MySQLInsertTableSource) *MySQLInsertStartBuilder
func (b *MySQLInsertStartBuilder) Ignore() *MySQLInsertStartBuilder
func (b *MySQLInsertStartBuilder) Values(rows ...any) *MySQLInsertBuilder
func (b *MySQLInsertStartBuilder) ValueSlice(rows any) *MySQLInsertBuilder
func (b *MySQLInsertStartBuilder) Select(sel SelectLike) *MySQLInsertBuilder
func (b *MySQLInsertBuilder) Values(rows ...any) *MySQLInsertBuilder
func (b *MySQLInsertBuilder) ValueSlice(rows any) *MySQLInsertBuilder
func (b *MySQLInsertBuilder) OnDuplicateKeyUpdateSet(assignments ...MySQLSetAssignment) *MySQLInsertBuilder
func (b *MySQLInsertBuilder) GrizExecBuilder()
func (b *MySQLInsertBuilder) ResultKind() ResultKind
func (b *MySQLInsertBuilder) grizPreparedBuilder()
func (b *MySQLInsertBuilder) Build(d dialect.Dialect) (sql string, args []any, err error)
func (b *MySQLInsertBuilder) BuildPrepared(d dialect.Dialect) (sql string, plan []PreparedArg, err error)
func (b *MySQLInsertBuilder) PreparedResultKind() PreparedResultKind
```

`Ignore()` is available on the pre-values/pre-select builder and must render `INSERT IGNORE INTO ...`, matching Drizzle RC.1 `.ignore()` order. `OnDuplicateKeyUpdateSet` is available after values or select, before or after `.ReturningID()`, and must render `ON DUPLICATE KEY UPDATE`; it must never accept or infer a conflict target. The duplicate-key update set may be applied at most once across the whole MySQL insert chain. A typed chain may remove the method after one call; otherwise a second call records `build_validation`. Empty assignment lists, assignments whose fields are all unset, invalid column identifiers, unsupported expression values, direct `MySQLSetAssignment` struct literals, zero-value assignments, or malformed assignment structs must be recorded as builder validation errors and returned from `Build`. `MySQLSetSelf(column)` is the specified no-op-update form for MySQL; it quotes the same single identifier on both sides of the assignment and exists to avoid raw SQL for the documented Drizzle no-op pattern. Typed variants `MySQLSetColValue`, `MySQLSetColExpr`, `MySQLSetColSelf`, and `MySQLSetColParam` mirror the string value/expression/self or runtime-param semantics while deriving type and encoder metadata from the embedded `sqlmeta.ParamColumn` portion of `MutationColumn` and validating ownership/alias state through the selectable/base-column contract.

The implementation must store a discriminator equivalent to `MySQLSetKind` internally so literal values, runtime params, and `expr.Expression` values are not inferred from `any` at render time. `MySQLSetValue` always binds the value as a fixed parameter, even if the value's concrete type implements `expr.Expression`; `MySQLSetExpr` renders the expression; `MySQLSetSelf` renders a quoted self-assignment; `MySQLSetColParam` records an encoded runtime parameter. Normal `Build` fails with `build_validation` for unresolved runtime params; `BuildPrepared` renders them through `PreparedBuildContext.AddRuntime` using the `MutationColumn` encoder/type/nullability metadata.

`MySQLInsertBuilder` specializes the normal insert builder for MySQL's RC.1 surface. It must support the same `Values` and `ValueSlice` shapes as the normal insert builder and the same error-returning `Build` contract. It is a prepared builder so `MySQLSetColParam` can be executed through `BuildPrepared`; `PreparedResultKind()` returns `PreparedResultExec` for non-returning MySQL inserts. MySQL `.ReturningID()` uses its own returning-id builder path documented above and is not implied by this non-returning prepared result kind. `Values(rows ...any)` accepts one or more row structs/maps, rejects zero arguments, and rejects slice/array arguments so typed slices must use `ValueSlice(rows)` unless a future generic overload is specified. `ValueSlice(rows)` accepts only non-nil slice or array values of row structs/maps and records build validation errors for non-slice, nil, empty, or invalid element inputs.

`Build` must return `unsupported_dialect` unless the supplied dialect is MySQL-compatible. MySQL compatibility means `UpsertStyle() == UpsertDuplicateKey` and `MySQLInsertIgnoreKeyword() != ""`; custom dialects must satisfy both capability checks rather than relying on `Name()`. PostgreSQL/SQLite-only methods such as `Returning`, `OnConflict`, and `OnConflictDoNothing` must be absent from this builder or must record `unsupported_feature` build errors if exposed through a shared Go type.

String column names in these helpers must be compile-time literals or generated constants. They are validated and quoted as single identifiers; user input must not be accepted as a column name.

### Insert-Conflict Helpers

| Drizzle RC.1 | Grizzle target | Status |
|---|---|---|
| MySQL `.ignore()` | MySQL `.Ignore()` rendering `INSERT IGNORE` | PARITY |
| SQLite `.onConflictDoNothing()` | SQLite `.OnConflictDoNothing()` | PARITY |
| unified cross-dialect `.IgnoreConflicts()` helper | optional shared wrapper over dialect-specific APIs | DEVIATION:LANGUAGE if retained |

SQLite RC.1 stores conflict clauses as an ordered list and allows multiple `onConflictDoNothing` / `onConflictDoUpdate` clauses on one insert. Initial Grizzle must either preserve that ordered multi-clause model for SQLite or mark it `DEVIATION:GAP` in the implementation status before claiming full SQLite conflict parity.

Conflict feature notes:

- `OnConflictDoNothing()` with no target is PARITY
- PostgreSQL `OnConflict(...).TargetWhere(...).DoNothing()` target + `where` support is DEVIATION:GAP (designed) until implemented
- SQLite `OnConflict(...).DoNothingWhere(...)` target + `where` support is DEVIATION:GAP (designed) until implemented and renders `DO NOTHING WHERE ...`
- `OnConflict(...).DoUpdate...` target support shares the PostgreSQL upsert target model where SQL overlaps
- SQLite `targetWhere` and `setWhere` update clauses are DEVIATION:GAP (designed) until the shared conflict-where API is implemented
- multiple ordered SQLite conflict clauses are DEVIATION:GAP unless the implementation adds an explicit ordered clause model

If a shared `.IgnoreConflicts()` helper is retained, its render/error matrix must be explicit:

- PostgreSQL: render `ON CONFLICT DO NOTHING` with no target
- MySQL: render `INSERT IGNORE`
- SQLite: render `ON CONFLICT DO NOTHING` with no target, matching RC.1's `onConflictDoNothing()` SQL form rather than broadening to unrelated SQLite conflict algorithms
- unsupported/custom dialects: return a build error rather than silently changing insert behavior

`Ignore()`, `DoNothing`, and optional `IgnoreConflicts()` can hide data-quality or integrity failures. MySQL `INSERT IGNORE` is especially broad because the database may downgrade additional constraint and data errors to warnings. Callers should observe row counts, warnings where the driver exposes them, or application-level reconciliation when skipped rows matter.

## UPDATE

### SET clause

Target shape:

```go
func Update(table UpdateTableSource) *UpdateBuilder
func (b *UpdateBuilder) Set(column string, value any) *UpdateBuilder
func (b *UpdateBuilder) SetCol(column MutationColumn, value any) *UpdateBuilder
func (b *UpdateBuilder) SetParam(column MutationColumn, name string) *UpdateBuilder
func (b *UpdateBuilder) SetExpr(column string, value expr.Expression) *UpdateBuilder
func (b *UpdateBuilder) SetColExpr(column MutationColumn, value expr.Expression) *UpdateBuilder
```

| Drizzle | Grizzle | Status |
|---|---|---|
| `.set({ col: val })` | `.Set(columnName, val)` or `.SetCol(columnHandle, val)` | PARITY for explicit assignments; omitted runtime `$onUpdateFn` behavior is DEVIATION:GAP as below |
| Struct-based set | `.SetStruct(struct)` | DEVIATION:LANGUAGE — Go struct/tag adaptation over parity SET semantics |
| `UPDATE … FROM` plus update joins (PostgreSQL / SQLite) | `.From(...)` and dialect-gated update join helpers on PostgreSQL / SQLite builders | PARITY target; SQLite `FULL JOIN` remains version/capability-gated like SELECT |
| no MySQL update `.from()` / update-join parity surface | no MySQL update `.From(...)` / update-join helpers | PARITY for omission |
| `.limit(n)` (MySQL / SQLite only) | `.Limit(n)` only on MySQL / SQLite update builders | PARITY |
| `.orderBy(cols)` (MySQL / SQLite only) | `.OrderBy(cols...)` only on MySQL / SQLite update builders | PARITY |
| no PostgreSQL update `.limit()` method | no PostgreSQL update `.Limit()` method, or a PostgreSQL-specific fast-fail if a shared Go builder exposes it | PARITY for omission; **DEVIATION:LANGUAGE** if exposed only to fail fast |
| no PostgreSQL update `.orderBy()` method | no PostgreSQL update `.OrderBy()` method, or a PostgreSQL-specific fast-fail if a shared Go builder exposes it | PARITY for omission; **DEVIATION:LANGUAGE** if exposed only to fail fast |
| *(Go adaptation over Drizzle explicit `set` object)* | `.SetStruct(struct)` with struct-based ON CONFLICT via `DoUpdateSetStruct` | DEVIATION:LANGUAGE — see INSERT section |

SQLite mutation `Limit` support is driver/compile-option gated through `SupportsLimitOnMutate()`. SQLite builders must fail fast if the selected driver/engine does not expose `SQLITE_ENABLE_UPDATE_DELETE_LIMIT`.

Drizzle RC.1 adds omitted columns with `onUpdateFn` to UPDATE/UPSERT SET rendering when at least one explicit assignment remains after filtering `undefined`; an empty explicit set still throws before SQL generation. Go has no initial runtime hook equivalent. Grizzle must therefore:

- use distinct Go method names instead of overloads: `Set(string, any)` for generated column-name constants or trusted literals, `SetCol(MutationColumn, any)` for generated typed column handles, `SetParam(MutationColumn, string)` for runtime params, `SetExpr(string, expr.Expression)` for trusted expression assignments, and `SetColExpr(MutationColumn, expr.Expression)` for typed-column expression assignments
- `Set` and `SetCol` always bind the provided value, even if the value's concrete type implements `expr.Expression`; `SetParam` records an encoded runtime parameter and fails normal `Build` if unresolved; expression rendering is available only through the explicit `SetExpr` / `SetColExpr` methods
- builders cannot prove string provenance at runtime; callers must not pass user-controlled column strings. Builders are responsible for validating identifier shape, length, and control characters and for quoting accepted identifiers safely.
- `Set`, `SetCol`, `SetParam`, `SetExpr`, `SetColExpr`, `DoUpdateSet*`, and MySQL duplicate-key setter helpers must validate requested columns against the target table's update metadata, rejecting unknown, non-updatable, non-insert-table, wrong-table, aliased, alias-wrapper, CTE/subquery, or derived columns with `build_validation`. Typed column setters require `BaseColumnRef().Table` to exactly match the unaliased mutation target identity and require an empty `TableRef.Alias`; SELECT-list alias state is not sufficient for this check. This alias/wrapper rejection is **DEVIATION:INTENTIONAL** safety hardening over RC.1's runtime permissiveness.
- upsert and MySQL duplicate-key builders require update metadata from the insert target. If the table source does not expose `GrizUpdateColumns()` or equivalent update metadata, the builder must fail with `build_validation` before rendering.
- qualify UPDATE/UPSERT parity as explicit-assignment parity for tables without unsupported runtime `$onUpdate` hooks
- generate update metadata such as `GrizUpdateColumns() []query.UpdateColumnMeta` so builders can detect runtime-on-update columns and nullability
- reject `query.Null[T]()` / explicit SQL `NULL` assignments for non-null update columns before SQL execution with a build-validation error
- fail with `unsupported_feature` if a non-empty UPDATE/UPSERT SET would require an omitted runtime `$onUpdate` value
- continue to fail empty assignment sets as validation errors rather than using runtime-on-update metadata to invent a SET clause

This is **DEVIATION:GAP (designed)** until a Go runtime hook API exists.

### RETURNING

| Dialect | Drizzle RC.1 behavior | Grizzle target | Status |
|---|---|---|---|
| PostgreSQL | `.returning()` | `.Returning(cols...)` | PARITY |
| SQLite | `.returning()` | `.Returning(cols...)` | PARITY |
| MySQL | no normal `.returning()` | no normal `.Returning()` parity surface for MySQL | PARITY for omission |

## DELETE

| Drizzle | Grizzle | Status |
|---|---|---|
| `db.delete(tbl).where(cond)` | `query.DeleteFrom(tbl).Where(cond)` | PARITY |
| `RETURNING` (PostgreSQL / SQLite only) | `.Returning(cols...)` only on PostgreSQL / SQLite delete builders | PARITY |
| no MySQL delete `.returning()` method | no MySQL delete `.Returning()` method | PARITY |
| `.limit(n)` (MySQL / SQLite only) | `.Limit(n)` only on MySQL / SQLite delete builders | PARITY |
| `.orderBy(cols)` (MySQL / SQLite only) | `.OrderBy(cols...)` only on MySQL / SQLite delete builders | PARITY |
| no PostgreSQL delete `.limit()` method | no PostgreSQL delete `.Limit()` method, or a PostgreSQL-specific fast-fail if a shared Go builder exposes it | PARITY for omission; **DEVIATION:LANGUAGE** if exposed only to fail fast |
| no PostgreSQL delete `.orderBy()` method | no PostgreSQL delete `.OrderBy()` method, or a PostgreSQL-specific fast-fail if a shared Go builder exposes it | PARITY for omission; **DEVIATION:LANGUAGE** if exposed only to fail fast |

## Prepared statements

Drizzle RC.1 driver-bound query builders support prepared execution and named placeholders through `sql.placeholder(name)` / deprecated `placeholder(name)`. PostgreSQL-family builders expose `prepare(name?)`; MySQL and SQLite builders expose parameterless `prepare()`. Execution receives a value map; missing placeholder values throw before driver execution. Raw placeholder params are filled as raw values, while placeholder params wrapped in encoded `Param` slots apply the slot encoder while filling the ordered driver parameter list.

**Drizzle example:**
```typescript
const prepared = db.select().from(users)
  .where(eq(users.id, sql.placeholder('id')))
  .prepare('get_user')

const result = await prepared.execute({ id: '...' })
```

**Grizzle target:**

```go
type Params map[string]any

type PreparedArgKind = sqlmeta.PreparedArgKind

const (
    PreparedArgFixed   PreparedArgKind = sqlmeta.PreparedArgFixed
    PreparedArgRuntime PreparedArgKind = sqlmeta.PreparedArgRuntime
)

type PreparedResultKind string

const (
    PreparedResultRows PreparedResultKind = "rows"
    PreparedResultExec PreparedResultKind = "exec"
)

type PreparedArg = sqlmeta.PreparedArg

type PreparedBuilder interface {
    grizPreparedBuilder()
    Build(dialect.Dialect) (sql string, args []any, err error)
    BuildPrepared(dialect.Dialect) (sql string, plan []PreparedArg, err error)
    PreparedResultKind() PreparedResultKind
}

// Package sqlmeta owns the PreparedArg carrier types to avoid expr -> query cycles.
// Package query owns Params, PreparedBuilder, and result-kind contracts.
// Package driver/pgx owns reusable prepared handles, validation, and registries.
func PrepareSelect[T any](ctx context.Context, db *pgxdb.DB, name string, b *query.SelectBuilder) (*pgxdb.PreparedSelect[T], error)
func (p *pgxdb.PreparedSelect[T]) QueryAll(ctx context.Context, params query.Params) ([]T, error)
func (p *pgxdb.PreparedSelect[T]) QueryOne(ctx context.Context, params query.Params) (T, error)
func (p *pgxdb.PreparedSelect[T]) QueryOpt(ctx context.Context, params query.Params) (*T, error)
func (p *pgxdb.PreparedSelect[T]) QueryAllTx(ctx context.Context, tx *pgxdb.Tx, params query.Params) ([]T, error)
func (p *pgxdb.PreparedSelect[T]) QueryOneTx(ctx context.Context, tx *pgxdb.Tx, params query.Params) (T, error)
func (p *pgxdb.PreparedSelect[T]) QueryOptTx(ctx context.Context, tx *pgxdb.Tx, params query.Params) (*T, error)

func PrepareQuery[T any](ctx context.Context, db *pgxdb.DB, name string, b query.PreparedBuilder) (*pgxdb.PreparedQuery[T], error)
func (p *pgxdb.PreparedQuery[T]) QueryAll(ctx context.Context, params query.Params) ([]T, error)
func (p *pgxdb.PreparedQuery[T]) QueryOne(ctx context.Context, params query.Params) (T, error)
func (p *pgxdb.PreparedQuery[T]) QueryOpt(ctx context.Context, params query.Params) (*T, error)
func (p *pgxdb.PreparedQuery[T]) QueryAllTx(ctx context.Context, tx *pgxdb.Tx, params query.Params) ([]T, error)
func (p *pgxdb.PreparedQuery[T]) QueryOneTx(ctx context.Context, tx *pgxdb.Tx, params query.Params) (T, error)
func (p *pgxdb.PreparedQuery[T]) QueryOptTx(ctx context.Context, tx *pgxdb.Tx, params query.Params) (*T, error)

func PrepareExec(ctx context.Context, db *pgxdb.DB, name string, b query.PreparedBuilder) (*pgxdb.PreparedExec, error)
func (p *pgxdb.PreparedExec) Exec(ctx context.Context, params query.Params) (int64, error)
func (p *pgxdb.PreparedExec) ExecTx(ctx context.Context, tx *pgxdb.Tx, params query.Params) (int64, error)

func QueryAllPrepared[T any](ctx context.Context, db *pgxdb.DB, b query.PreparedBuilder, params query.Params) ([]T, error)
func QueryOnePrepared[T any](ctx context.Context, db *pgxdb.DB, b query.PreparedBuilder, params query.Params) (T, error)
func QueryOptPrepared[T any](ctx context.Context, db *pgxdb.DB, b query.PreparedBuilder, params query.Params) (*T, error)
func ExecPrepared(ctx context.Context, db *pgxdb.DB, b query.PreparedBuilder, params query.Params) (int64, error)
func QueryAllPreparedTx[T any](ctx context.Context, tx *pgxdb.Tx, b query.PreparedBuilder, params query.Params) ([]T, error)
func QueryOnePreparedTx[T any](ctx context.Context, tx *pgxdb.Tx, b query.PreparedBuilder, params query.Params) (T, error)
func QueryOptPreparedTx[T any](ctx context.Context, tx *pgxdb.Tx, b query.PreparedBuilder, params query.Params) (*T, error)
func ExecPreparedTx(ctx context.Context, tx *pgxdb.Tx, b query.PreparedBuilder, params query.Params) (int64, error)

type Registry struct { /* opaque */ }
func NewRegistry(db *pgxdb.DB) (*Registry, error)
func RegisterSelect[T any](reg *Registry, name string, b *query.SelectBuilder) (*pgxdb.PreparedSelect[T], error)
func RegisterQuery[T any](reg *Registry, name string, b query.PreparedBuilder) (*pgxdb.PreparedQuery[T], error)
func RegisterExec(reg *Registry, name string, b query.PreparedBuilder) (*pgxdb.PreparedExec, error)
func (r *Registry) PrepareAll(ctx context.Context) error
```

PostgreSQL, MySQL, and SQLite driver packages must expose equivalent one-time prepared execution helpers for their supported DB/Tx types. The package names differ (`pgxdb`, `mysqldb`, `sqlitedb`), but the shape is the same: row helpers accept only builders whose `PreparedResultKind()` is `PreparedResultRows`, exec helpers accept only `PreparedResultExec`, and all helpers fill the ordered plan from `query.Params` before execution. Reusable named handles/registries are pgx-specific in the initial target; MySQL and SQLite one-time helpers still own their own DB/Tx nil and typed-nil receiver validation.

For MySQL, `MySQLInsertReturningIDBuilder` is row-returning even though the database does not produce a normal result-row stream. MySQL row helpers that receive this builder must execute through MySQL exec/result metadata, synthesize the returning-id rows using the `.ReturningID()` contract above, and then scan or return those synthesized rows. They must not try to execute the insert through a normal row-query API.

Rules:

- Go must not require concrete-value methods such as `UUIDColumn.EQ(uuid.UUID)` or generated insert structs to also accept placeholder objects; placeholder support must use explicit param-capable APIs such as `EQParam(name)`, `SetParam(mutationColumn, name)`, `LimitParam(name)`, or an explicitly designed typed value-wrapper redesign
- minimum param-capable API surface includes column predicates such as `EQParam(name)`, `NEQParam(name)`, comparison params for ordered column types, `InParams(names...)`, mutation setters such as `SetParam(column MutationColumn, name string)`, insert map/assignment params such as `query.InsertParam[T](name)`, raw runtime placeholders such as `expr.RawParam(name)` for `RawArgs` templates, and `LimitParam(name string)` / `OffsetParam(name string)` where the dialect supports bind parameters in those positions
- generated typed columns used with predicate param-capable APIs must implement `ParamColumn`, exposing the SQL column reference, Go type, and param encoder/cast behavior; generated typed columns used with mutation param APIs must implement `MutationColumn` so builders can also enforce target-table ownership and empty `BaseColumnRef().Table.Alias`. String-only column names are insufficient for prepared params because they cannot provide type, encoder, or ownership metadata
- `InsertColumnMeta` and `UpdateColumnMeta` must carry enough type/encoder metadata for param-capable map/assignment APIs; column-handle param APIs should derive type and encoder metadata directly from the typed generated column handle
- fixed column-backed values in predicates, insert rows/maps, update sets, upserts, MySQL duplicate-key assignments, and conflict assignments must use the encoder/cast path defined by `BuildContext.AddEncoded` or `PreparedBuildContext.AddFixedEncoded`; builders must pass the column `GoType`, reject nil/typed-nil encoders, check value assignability before encoding, and leave args/plans unchanged on encode or cast failure. Raw `Add` / `AddFixed` is only for values with no column/custom encoder metadata.
- struct insert rows cannot contain prepared placeholders in the initial design; prepared insert placeholders are allowed only through explicitly param-capable map/assignment APIs until a broader generated-row wrapper design exists
- `BuildPrepared` is distinct from normal `Build`: it renders through the prepared-aware context defined in [types.md](./types.md#buildcontext-contract), emits normal dialect placeholders in SQL, and returns one ordered `PreparedArg` plan containing both fixed bound arguments and runtime named slots in the exact order `AddFixed` / `AddFixedEncoded` / `AddRuntime` were called. Fixed args carry final encoded driver values and no encoder; runtime args carry name/type plus an optional encoder and are encoded exactly once per execution.
- `PreparedBuilder` is sealed with an unexported method so only package-owned builders can produce prepared SQL/plan pairs. Helpers still validate plan metadata defensively before execution: `PreparedArg.Kind` is one of the documented constants, fixed args and runtime fields are mutually exclusive, runtime args have non-empty safe names, typed-nil encoders are rejected, nil encoders are allowed only for raw/no-op runtime slots, raw slots may have nil `Type`, encoded slots must have non-nil `Type`, and `Nullable=false` rejects nil params before execution. Malformed plans fail with `build_validation` or a more specific prepared-param code before SQL execution.
- prepared runtime param names must match `^[A-Za-z_][A-Za-z0-9_]{0,63}$`; empty names, controls, whitespace, non-ASCII names, and shell/SQL metacharacters fail with `build_validation` before rendering or execution. This is **DEVIATION:INTENTIONAL** safety hardening from RC.1's arbitrary placeholder-name strings.
- `ParamEncoder.CastPlaceholder` is the build-time counterpart to runtime `Encode`, matching Drizzle's separate `castParam` and `normalizeParam` codec hooks. It may return the placeholder unchanged for pgx inference, but column types that require casts or dialect-specific placeholder wrapping must express that during `BuildPrepared` and must be covered by conformance tests.
- normal direct `Build` must return an error if the builder contains unresolved prepared params; ordinary no-param direct execution helpers must use `Build`
- one-time parameterized execution helpers such as `QueryAllPrepared` and `ExecPrepared` provide the Go equivalent of Drizzle RC.1 direct parameterized execution paths: PostgreSQL/MySQL `.execute(params)` and SQLite `.run(params)` / `.all(params)` / `.get(params)` / `.values(params)`. They call `BuildPrepared`, fill the ordered plan from `query.Params`, execute once, and do not create a reusable prepared handle.
- `PrepareSelect`, `PrepareQuery`, `PrepareExec`, one-time prepared execution helpers, and registry validation must use `BuildPrepared`
- `BuildPrepared` requires every expression, selectable column, assignment expression, conflict predicate, raw fragment wrapper, insert-select source, CTE, join predicate, returning expression, order/group expression, and limit/offset expression in the builder tree to implement `expr.PreparedExpression` / `expr.PreparedSelectableColumn` or an explicitly documented prepared-capable query interface. Normal-only external expressions fail with `build_validation` in prepared builds; the builder must not fall back to parsing normal rendered SQL to infer a prepared plan.
- execution accepts exactly one `query.Params` map; nil is equivalent to an empty map, extra keys are ignored with diagnostics only in verbose/debug contexts, and the implementation must copy needed values before encoding
- duplicate runtime names are allowed. Each `PreparedArgRuntime` occurrence reads the same raw value from `query.Params` and applies that slot's own optional encoder independently, matching Drizzle's placeholder-fill behavior for raw placeholders versus encoded `Param(Placeholder, encoder)` slots.
- nil and typed-nil runtime values represent SQL `NULL` for nullable slots. Typed nils are detected before type checks/encoding; they fail for non-nullable slots and otherwise bypass `ParamEncoder.Encode`, matching Drizzle's `Param(Placeholder, encoder)` null behavior.
- non-nil runtime values for typed slots must be assignable to the slot `Type`; values convertible without loss may be accepted only if the encoder explicitly documents that conversion. Raw slots with nil `Type` do not perform type checks. Missing parameter names, nil/typed-nil values for non-nullable slots, incompatible values, values that cannot be encoded for their target column, or unresolved named params in direct non-prepared execution must return errors before SQL execution.
- all `Prepare*`, one-time prepared, Tx prepared, and registry APIs must reject nil and typed-nil builders before calling builder methods, returning a stable redacted Grizzle error such as `build_validation`
- all prepared, registry, and prepared-handle APIs must reject nil and typed-nil DB, Tx, registry, and prepared-handle receivers before method calls. Each driver package applies this to its own DB/Tx types; invalid DB/Tx/registry/handle inputs return `invalid_receiver`, while pgx registry readiness and Tx/DB identity failures keep using `prepared_not_ready`, `registry_closed`, or `prepared_tx_mismatch` as applicable
- `PrepareSelect` is a convenience for SELECT builders; `PrepareQuery[T]` covers builders whose `PreparedResultKind()` is `PreparedResultRows`, including SELECT and returning INSERT/UPDATE/DELETE builders; `PrepareExec` accepts only builders whose result kind is `PreparedResultExec` and must reject row-returning builders before validation
- all prepared helper paths validate `PreparedResultKind()` before SQL execution. Row-query helpers reject `PreparedResultExec`; row-count execution helpers reject `PreparedResultRows`; mismatches return `invalid_prepared_result_kind`.
- pgx prepared handles are bound to the `*pgxdb.DB` used during `PrepareSelect`, `PrepareQuery`, or `PrepareExec`; normal `Exec`/`Query*` use that DB. `ExecTx` / `Query*Tx` are allowed only for transactions associated with the same pool/DB identity, otherwise they must fail before SQL execution.
- for pgx, startup validation may prepare the SQL on a single acquired connection to force server-side parse/describe, then execute later by SQL text so pgx v5's per-connection statement cache handles prepared-plan reuse on each pool connection
- Grizzle statement names are human-readable labels for diagnostics/registry lookup and a pgx validation label; a cross-dialect named-statement registry is DEVIATION:LANGUAGE because RC.1 MySQL/SQLite builders use parameterless `prepare()`
- statement and registry names are trusted static diagnostic labels, not secret storage; they must be non-empty, must not contain credentials or personal data, must reject controls/newlines, and must enforce a finite implementation-defined maximum length
- registry names must be non-empty and unique; nil builders fail registration; registered handles are not executable until `PrepareAll` succeeds and must return `prepared_not_ready` if executed early or after failed preparation
- `PrepareAll` validates all registered statements against the registry DB and is all-or-nothing for registry readiness. After successful `PrepareAll`, the registry is closed; later registration fails with `registry_closed`. After failed `PrepareAll`, the registry remains not-ready and callers may retry the same registrations after fixing external causes such as schema or connection state; changing the registered statement set requires creating a new registry.
- registry APIs batch validation, but logged diagnostics must use statement names and redacted summaries rather than full SQL, raw params, or credentials
- the Go API shape is DEVIATION:LANGUAGE; the placeholder/execute semantics are PARITY targets with Drizzle RC.1

**Status:** DEVIATION:GAP (designed) until explicit param-capable operators/builders, `BuildPrepared`, ordered prepared-argument plans, and per-execution `query.Params` are implemented consistently across SELECT, INSERT, UPDATE, DELETE, raw expressions, and driver helpers. Static no-parameter pgx validation/execution helpers may exist as an incremental subset, but they are not full Drizzle prepared-statement parity.

## Raw SQL

Raw SQL APIs are trusted-input escape hatches.

`expr.Raw(str)` returns a raw expression that may be used anywhere an `expr.Expression` or `SelectableColumn` is accepted. Use `expr.ColAs(expr.Raw(str), alias)` when selecting a raw expression with a scan alias.

Rules:

- never pass untrusted strings to `expr.Raw`, `expr.RawArgs` SQL templates, `query.RawSelectSQL`, `query.RawCountSQL`, `db.QueryRaw`, `db.ExecRaw`, `tx.QueryRaw`, `tx.ExecRaw`, or custom external `expr.Expression` / `expr.SelectableColumn` implementations
- use `expr.RawArgs`, `query.RawSelectSQL`, `query.RawCountSQL`, `QueryRaw`, and `ExecRaw` arguments for values rather than interpolating values into SQL text
- external expression/selectable implementations are trusted raw-SQL extensions. They must bind dynamic values through `expr.BuildContext` / `expr.PreparedBuildContext` and must not render user-controlled strings directly.
- raw SQL may contain dialect-specific syntax that the query builder cannot validate
- generated diagnostics and examples must prefer placeholders over literal secret or personal data values

| Drizzle | Grizzle | Status |
|---|---|---|
| `` sql`raw sql` `` | `expr.Raw(str)` | PARITY for literal strings |
| `` sql`... ${val} ...` `` parameterized | `expr.RawArgs(sql, args...)` | DEVIATION:LANGUAGE — see below |
| PostgreSQL/MySQL `db.execute(sql\`...\`)` row/result execution | `db.QueryRaw(ctx, sql, args...)` for row-returning SQL and `db.ExecRaw(ctx, sql, args...)` for exec-only SQL | DEVIATION:LANGUAGE API split; RC.1 result semantics must be preserved |
| SQLite raw `.run()` / `.all()` / `.get()` / `.values()` | `ExecRaw`, `QueryRaw` plus `Scan*` helpers or future raw-value helpers | DEVIATION:LANGUAGE API split over SQLite's dialect-specific raw methods |

## Execution and scanning

| Drizzle | Grizzle | Status |
|---|---|---|
| `await db.select()...` → array | `pgxdb.FromSelect[T](ctx, d, q)` | PARITY |
| Single row helper | `pgxdb.FromSelectOne[T](ctx, d, q)` | DEVIATION:LANGUAGE — Go cardinality convenience over row-array parity; SQLite `.get()` is dialect-specific upstream behavior, not a cross-dialect RC.1 core select helper |
| Optional single row helper | `pgxdb.FromSelectOpt[T](ctx, d, q)` | DEVIATION:LANGUAGE — Go cardinality convenience over row-array parity; relational `findFirst` is a separate future surface |
| Cursor / streaming | DEVIATION:GAP (not designed) | — |

Target helper shape:

```go
type ResultKind string

const (
    ResultRows ResultKind = "rows"
    ResultExec ResultKind = "exec"
)

type RowBuilder interface {
    Build(dialect.Dialect) (sql string, args []any, err error)
    ResultKind() ResultKind
    GrizRowBuilder()
}

type ExecBuilder interface {
    Build(dialect.Dialect) (sql string, args []any, err error)
    ResultKind() ResultKind
    GrizExecBuilder()
}

func FromSelect[T any](ctx context.Context, db *pgxdb.DB, q *query.SelectBuilder) ([]T, error)
func FromSelectOne[T any](ctx context.Context, db *pgxdb.DB, q *query.SelectBuilder) (T, error)
func FromSelectOpt[T any](ctx context.Context, db *pgxdb.DB, q *query.SelectBuilder) (*T, error)

func ScanAll[T any](rows pgx.Rows, queryErr error) ([]T, error)
func ScanOne[T any](rows pgx.Rows, queryErr error) (T, error)
func ScanOneOpt[T any](rows pgx.Rows, queryErr error) (*T, error)
```

MySQL returning-id inserts use synthesized rows rather than `pgx.Rows`-style row streams. Target MySQL helper shape:

```go
func FromReturningID[T any](ctx context.Context, db *mysqldb.DB, b *query.MySQLInsertReturningIDBuilder) ([]T, error)
func FromReturningIDTx[T any](ctx context.Context, tx *mysqldb.Tx, b *query.MySQLInsertReturningIDBuilder) ([]T, error)
func FromReturningIDPrepared[T any](ctx context.Context, db *mysqldb.DB, b *query.MySQLInsertReturningIDBuilder, params query.Params) ([]T, error)
func FromReturningIDPreparedTx[T any](ctx context.Context, tx *mysqldb.Tx, b *query.MySQLInsertReturningIDBuilder, params query.Params) ([]T, error)
func ReturningIDRows(ctx context.Context, db *mysqldb.DB, b *query.MySQLInsertReturningIDBuilder) ([]query.ReturningIDRow, error)
func ReturningIDRowsTx(ctx context.Context, tx *mysqldb.Tx, b *query.MySQLInsertReturningIDBuilder) ([]query.ReturningIDRow, error)
func ReturningIDRowsPrepared(ctx context.Context, db *mysqldb.DB, b *query.MySQLInsertReturningIDBuilder, params query.Params) ([]query.ReturningIDRow, error)
func ReturningIDRowsPreparedTx(ctx context.Context, tx *mysqldb.Tx, b *query.MySQLInsertReturningIDBuilder, params query.Params) ([]query.ReturningIDRow, error)
```

These helpers validate nil and typed-nil DB/Tx/builder inputs before calling methods; invalid DB/Tx receivers return `invalid_receiver`, and nil or typed-nil builders return `build_validation`. They build a `MySQLReturningIDPlan` before execution and must fail before sending SQL if the plan contains known unreconstructable returning-id dependencies such as runtime placeholder primary-key values. After pre-execution validation, they execute the insert through MySQL exec/result APIs, call `Synthesize`, and either scan synthesized `ReturningIDRow` values into generated typed structs (`FromReturningID*`) or return the lower-level map rows directly (`ReturningIDRows*`). `FromReturningID*` uses the generated returning-id property-key scanner described in [codegen.md](./codegen.md); it does not use the generic SQL `db`-tag scanner and does not accept arbitrary map scan targets. Callers that want maps use `ReturningIDRows*`. Generic `mysqldb` row-query helpers must detect `MySQLInsertReturningIDBuilder` and route to this synthesized-row path; they must not try to obtain rows from MySQL for an INSERT statement.

`FromReturningID*` scan target `T` must be a non-pointer struct with exported fields tagged `grizzle:"<propertyKey>"` when synthesized rows contain keys. Pointer targets, scalar targets, map targets, anonymous structs without tags, and structs with no scan-visible `grizzle` tags fail with `scan_decode` unless the returning-id plan has no keys and produces the RC.1 source-compatible empty non-nil result slice. The property-key scanner treats `grizzle:"-"` as an ignored field, rejects tag options in the initial target, and must reject empty tags, duplicate tags, unknown tags, missing fields for synthesized keys, and extra scan-visible `grizzle`-tagged fields not present in the synthesized key set. Decoding must enforce the plan-declared Go type and nullability for every key; type mismatches, non-null field targets receiving null, and unsupported conversions fail with redacted `scan_decode` errors.

Rules:

- `RowBuilder` and `ExecBuilder` marker methods provide package-level grouping, but mutation builders whose result kind changes after `.Returning(...)` may implement both interfaces; `ResultKind()` is authoritative and driver helpers must validate it before SQL execution
- `SELECT` builders return `ResultRows`; non-returning mutation builders return `ResultExec`; mutation builders after `.Returning(...)` return `ResultRows`
- `FromSelect*` must call the error-returning `Build(dialect)` path before driver execution and return any build error without opening rows
- `FromSelect*` must reject nil and typed-nil DB/query inputs before calling methods; invalid DB receivers return `invalid_receiver`, while nil or typed-nil query builders return `build_validation`
- `Scan*` helpers accept the `(rows, err)` pair returned by query helpers. Typed-nil rows must be detected on every path and treated as absent. If `queryErr` is non-nil, helpers close non-nil, non-typed-nil rows if present, return the original query error as primary, and do not scan.
- if `queryErr` is nil and `rows` is nil or typed-nil, `Scan*` must fail with a stable redacted `invalid_rows` error
- `Scan*` owns any non-nil `pgx.Rows` it receives and must close it exactly once on every success, scan error, context cancellation, or cardinality error path
- row-to-struct scanning is strict: every returned SQL column label must map to exactly one exported field by explicit `db` tag. Fields without `db` tags are ignored unless a future scanner spec defines a name-normalization mode. Duplicate/ambiguous labels fail, and every exported target field with a scan-visible `db` tag must be present in the result. `GrizSelectAllFieldKeys()` property keys are not SQL labels and are not used by this generic scanner. No optional-field scan marker is defined in the initial target.
- scan target `T` must be a non-pointer struct with at least one exported field carrying a `db` tag in the initial target. Pointer targets, scalar targets, maps, slices, embedded-field flattening, anonymous structs without tags, and structs with no scan-visible `db` tags fail with `scan_decode` unless a future scanner spec explicitly adds support.
- partial projections and partial `RETURNING` clauses must scan into matching projection structs instead of full generated `*Select` structs; missing required scan fields fail with `scan_decode` rather than silently zero-filling
- `ScanAll` returns all rows, including an empty non-nil slice for zero rows
- `ScanOne` returns a stable redacted `not_found`-class error for zero rows and `too_many_rows` for more than one row
- `ScanOneOpt` returns `nil, nil` for zero rows and `too_many_rows` for more than one row
- row decoding errors must be redacted so raw SQL, credentials, and bind values are not recoverable through error strings, logs, verbose diagnostics, or `%+v`
- execution helpers must check context before opening rows and preserve cancellation/deadline errors from query execution, scanning, `Rows.Next`, and `Rows.Err`. Direct `Build(dialect)` and `BuildPrepared(dialect)` calls have no context parameter and cannot observe cancellation; helpers that build and execute should check context before and after build. `pgx.Rows.Close()` is exactly-once cleanup and does not return a context/error sentinel.

---

## Go Adaptations And Additions

The following APIs are either Go-language adaptations of Drizzle behavior or Grizzle-only additions. Each retained item must be labeled as `DEVIATION:LANGUAGE`, `GRIZZLE-ONLY`, or both.

### `expr.RawArgs` — parameterized raw SQL (DEVIATION:LANGUAGE)

Drizzle uses template literals (`` sql`SELECT ${col} WHERE id = ${val}` ``) to interpolate bound parameters into raw SQL. Go has no template literal types. `expr.RawArgs` is the idiomatic Go equivalent: it accepts a SQL fragment with `$?` placeholders and a matching list of arguments.

```go
// Drizzle:
//   sql`ST_DWithin(location, ST_MakePoint(${lon}, ${lat}), ${radius})`
//
// Grizzle:
expr.RawArgs("ST_DWithin(location, ST_MakePoint($?, $?), $?)", lon, lat, radius)
```

Each `$?` token is replaced with the next dialect placeholder (`$1`, `?`, etc.) and the corresponding argument is bound. Placeholder count must exactly match argument count. A mismatch must fail with a redacted diagnostic that includes expected/actual counts and a stable internal error code only; it must not include hashes, fingerprints, or raw SQL text that could leak secrets into logs.

For prepared raw SQL, `expr.RawParam(name)` is the only initial raw runtime-placeholder API. It may appear as a `$?` argument to `RawArgs`; normal `Build` fails with `build_validation`, while `BuildPrepared` records a raw runtime slot with nil type/encoder and nullable semantics. Column-backed prepared params should use typed column APIs instead of raw params so encoders and casts are preserved.

Implementation note: this relies on the error-propagating expression contract in [types.md](./types.md). Do not reintroduce a string-only renderer for `RawArgs`, because placeholder mismatches must fail with redacted diagnostics rather than panicking or leaking raw SQL.

### `query.JoinRel` and `query.InnerJoinRel` — relation-based JOIN (GRIZZLE-ONLY)

Drizzle requires an explicit ON expression for every JOIN: `.leftJoin(posts, eq(users.id, posts.userId))`. In Go, relation definitions (`RelationDef`) already encode the foreign table and ON condition. `JoinRel`/`InnerJoinRel` reuse this definition to avoid repeating the ON expression at every query call site.

```go
// Define once at schema level:
var UserRealm = query.BelongsTo("realm", RealmsT, RealmsT.ID.EQCol(UsersT.RealmID))

// Use at query call sites — ON condition not repeated:
query.Select(UsersT.ID, RealmsT.Name).From(UsersT).JoinRel(UserRealm)
// equivalent to: .LeftJoin(RealmsT, RealmsT.ID.EQCol(UsersT.RealmID))

query.Select(UsersT.ID, RealmsT.Name).From(UsersT).InnerJoinRel(UserRealm)
// equivalent to: .InnerJoin(RealmsT, RealmsT.ID.EQCol(UsersT.RealmID))
```

`JoinRel` produces a LEFT JOIN; `InnerJoinRel` produces an INNER JOIN.

### `InsertBuilder.DoUpdateSetStruct` — struct-based ON CONFLICT DO UPDATE (DEVIATION:LANGUAGE)

Drizzle accepts an explicit `set` object for conflict updates; TypeScript constrains the allowed keys and runtime `mapUpdateSet` filters `undefined` values and throws if no values remain. In Go, `DoUpdateSetStruct` is a language adaptation that derives that explicit assignment set from a db-tagged struct. It adds assignments using the same rules as `UpdateBuilder.SetStruct`: non-nil pointer fields set concrete values, nil pointer fields are omitted, and nullable generated fields using `query.Assign[T]` can represent unset, explicit SQL `NULL`, or concrete values.

Status: target semantics are Drizzle parity, the Go reflection/struct API is DEVIATION:LANGUAGE, and implementation remains DEVIATION:GAP until `query.Assign[T]`, `SetStruct`, `DoUpdateSetStruct`, and generated nullable assignment structs are complete.

All reflection validation failures from `SetStruct` or `DoUpdateSetStruct` must be stored on the builder and returned from the error-returning `Build(dialect)` contract defined in [types.md](./types.md#query-build-contract). This includes invalid structs, unsupported field types, malformed tags, and invalid `query.Assign[T]` states.

```go
type UserUpsert struct {
    Email   query.Assign[string] `db:"email"`
    Enabled *bool   `db:"enabled"`
}

query.InsertInto(UsersT).Values(row).
    OnConflict(query.ConflictColumn(UsersT.RealmID), query.ConflictColumn(UsersT.Username)).
    DoUpdateSetStruct(UserUpsert{Email: query.Value("alice@example.com")})
// emits: ON CONFLICT ("realm_id", "username") DO UPDATE SET "email" = $N
```

Nil pointer fields and unset `query.Assign[T]` fields are skipped. `query.Null[T]()` emits an explicit SQL `NULL` assignment. If a valid struct produces an empty assignment set, the builder must record a validation error and return it from `Build`; it must not silently change the conflict action to `DO NOTHING`. Callers that want no-op conflict behavior must choose an explicit `DoNothing`, dialect-specific ignore helper, or MySQL no-op update helper where supported. Invalid structs, unsupported field types, malformed tags, or invalid `query.Assign[T]` states must also return an error rather than silently changing the conflict action.

### `expr.WinSum`, `expr.WinAvg`, `expr.WinCount` — window aggregates (GRIZZLE-ONLY)

Drizzle users write `sql\`SUM(${col}) OVER (PARTITION BY ...)\`` for window aggregates. In Grizzle, `WinSum`, `WinAvg`, and `WinCount` return `WindowExpr` values that can be chained with `.PartitionBy()` and `.OrderBy()`, keeping the type-safe builder pattern consistent with the other window functions.

```go
expr.WinSum(OrdersT.Amount).PartitionBy(OrdersT.CustomerID).As("running_total")
// → SUM("orders"."amount") OVER (PARTITION BY "orders"."customer_id") AS "running_total"

expr.WinAvg(ScoresT.Value).PartitionBy(ScoresT.RealmID).OrderBy(ScoresT.CreatedAt.Asc()).As("avg_score")
expr.WinCount().PartitionBy(UsersT.RealmID).As("realm_count")
```

### `expr.TsRank` and `expr.TsRankCd` — FTS ranking (GRIZZLE-ONLY)

Drizzle users write `sql\`ts_rank(${col}, ${query})\`` for full-text search ranking. In Grizzle, `TsRank` and `TsRankCd` are typed wrappers that integrate with the existing FTS expression builders (`ToTsvector`, `PlainToTsquery`, etc.), keeping the whole FTS pipeline in a single consistent API.

```go
tsq := expr.PlainToTsquery("grizzle orm")
expr.TsRank(ArticlesT.SearchVector, tsq).Desc()
// → TS_RANK("articles"."search_vector", plainto_tsquery($1)) DESC

expr.TsRankCd(ArticlesT.SearchVector, tsq).Desc()
// → TS_RANK_CD("articles"."search_vector", plainto_tsquery($1)) DESC
```

### Window frame sentinels (GRIZZLE-ONLY — partial; see #139)

`expr.UnboundedPreceding()`, `expr.CurrentRow()`, and `expr.UnboundedFollowing()` return `WindowFrameBound` sentinel values for specifying `ROWS/RANGE BETWEEN … AND …` frame boundaries. These types are exported and designed but are not yet wired to the `WindowExpr` builder (no `.Frame()` method exists). Full frame support is tracked in issue #139.

```go
// Intended future usage (not yet wired):
expr.WinSum(col).Over(
    expr.Frame("ROWS", expr.UnboundedPreceding(), expr.CurrentRow()),
)
```
