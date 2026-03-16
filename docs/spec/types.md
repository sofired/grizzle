# Type System Specification

## Column handles

In TypeScript, Drizzle column definitions carry their type as a generic parameter, enabling type inference throughout the query builder. In Go, `grizzle gen` generates concrete column handle types for each column in each table. These handles are the Go equivalent of Drizzle's typed column objects.

### Column handle interface hierarchy

```
SelectableColumn          — can appear in SELECT list (has .As(alias), .Asc(), .Desc())
    └── Expression        — can appear in WHERE/HAVING
            └── ColBase   — embedded by all concrete column types
                    ├── UUIDColumn
                    ├── StringColumn
                    ├── IntColumn
                    ├── BigIntColumn
                    ├── BoolColumn
                    ├── TimestampColumn
                    ├── JSONBColumn[T]
                    └── NumericColumn
```

All column handles embed `ColBase`, which provides:
- `.IsNull()` / `.IsNotNull()`
- `.Asc()` / `.Desc()`
- `.EQCol(other)` — column-to-column equality (for JOIN conditions)
- `.As(alias)` — aliased SELECT expression

**Status:** PARITY in structure. Per-type operator gaps are documented in [query-builder.md](./query-builder.md).

## `BuildContext` contract

`expr.BuildContext` is threaded through every `ToSQL(ctx)` call. It accumulates bound parameters and carries the active dialect. Any new expression type must use it — never format SQL strings directly.

```go
// NewBuildContext creates a fresh context for a single query.
func NewBuildContext(d dialect.Dialect) *BuildContext

// Add appends a bound value and returns its placeholder ("$1", "?", etc.).
// Always use Add rather than formatting values into the SQL string.
func (c *BuildContext) Add(val any) string

// Quote wraps an identifier in dialect-appropriate quote characters.
func (c *BuildContext) Quote(name string) string

// ColRef returns the qualified "table"."col" reference (or just "col" if table is empty).
func (c *BuildContext) ColRef(table, name string) string

// Args returns the ordered slice of bound parameter values, for passing to the driver.
func (c *BuildContext) Args() []any

// Dialect returns the active dialect for feature-detection checks.
func (c *BuildContext) Dialect() dialect.Dialect
```

**Rule:** Every expression's `ToSQL` implementation must call `ctx.Add(val)` for each bound value and use the returned placeholder in the SQL string. Never embed values directly in the string. This is what prevents SQL injection across the query builder.

## Expression interface

All WHERE-clause elements implement `expr.Expression`:

```go
type Expression interface {
    ToSQL(ctx *BuildContext) string
}
```

This is the Go equivalent of Drizzle's internal `SQL` type, which carries both the SQL fragment and its parameter bindings. The difference is that Drizzle's `SQL` is a value (fragment + args together); Grizzle's `Expression` is an interface that renders itself into a shared `BuildContext`.

**Status:** PARITY in concept.

## Null handling

### Schema level

Drizzle: a column without `.notNull()` has type `T | null` in TypeScript.
Grizzle: a column without `.NotNull()` generates a pointer type (`*T`) in `*Select` and `*Insert` structs.

**Status:** PARITY

### Query level

Drizzle: `eq(col, null)` automatically emits `IS NULL`.
Grizzle: `.IsNull()` and `.IsNotNull()` are explicit methods on all column handles. There is no automatic `IS NULL` when comparing to `nil`. This is **DEVIATION:INTENTIONAL** — Go's type system makes overloading equality on nil behaviour error-prone; explicit `.IsNull()` is clearer.

## Typed aggregates

Drizzle's aggregate return types are inferred from the column type. Grizzle's aggregates return `AggregateExpr` which implements `SelectableColumn`. The Go type of the scanned result depends on the struct field the aggregate is scanned into.

**Status:** DEVIATION:LANGUAGE — TypeScript can infer the return type of `sum(users.score)` as `number`; Go cannot. Callers must ensure their scan struct field type matches the aggregate result.

## Generics usage

Go generics are used in:

- `pgxdb.FromSelect[T]`, `pgxdb.ScanAll[T]`, `pgxdb.ScanOne[T]`, `pgxdb.ScanOneOpt[T]` — scan result type
- `query.Pluck[T, K]` — extract field from slice
- `query.Index[K, T]` — build map from slice
- `query.GroupBy[K, T]` — build multimap from slice
- `query.First[T]` — first element
- `JSONBColumn[T]` — JSONB column with typed scan target

Where Drizzle achieves type safety through TypeScript inference, Grizzle achieves it through generics and code generation. The two mechanisms are equivalent in user experience; differences are **DEVIATION:LANGUAGE**.

## `db` struct tag

Grizzle uses the `db` struct tag for column name mapping in INSERT/UPDATE/SELECT scanning, matching the convention used by `sqlx` and `pgx`. Drizzle has no equivalent (TypeScript uses property names directly).

The `omitempty` modifier on the `db` tag causes nil-pointer fields to be omitted from INSERT/UPDATE SET clauses. This is Grizzle's equivalent of Drizzle's partial update pattern.

**Status:** GRIZZLE-ONLY — a Go-idiomatic necessity with no Drizzle equivalent.
