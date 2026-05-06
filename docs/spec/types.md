# Type System Specification

## Column handles

In TypeScript, Drizzle column definitions carry their type as a generic parameter, enabling type inference throughout the query builder. In Go, `grizzle gen` generates concrete column handle types for each column in each table. These handles are the Go equivalent of Drizzle's typed column objects.

### Column handle interface hierarchy

```
SelectableColumn          — can appear in SELECT list (aliasing through expr.ColAs or an equivalent .As(alias) convenience, plus .Asc(), .Desc())
    └── Expression        — can appear in WHERE/HAVING
            └── ColBase   — untyped SQL column reference for generated columns, aliases, CTEs, and subqueries
                    ├── UUIDColumn
                    ├── StringColumn
                    ├── IntColumn
                    ├── BigIntColumn
                    ├── BoolColumn
                    ├── TimestampColumn
                    ├── TimeColumn
                    ├── DateColumn
                    ├── BytesColumn
                    ├── EnumColumn
                    ├── ArrayColumn
                    ├── JSONColumn[T]  — plain JSON across supported dialects; no JSONB-only containment/existence helpers
                    ├── JSONBColumn[T]
                    └── NumericColumn
```

Concrete generated column handle types implement `sqlmeta.ParamColumn` when they carry schema/table-aware `ColumnMeta`, Go type, nullability, and encoder/cast metadata. Bare `expr.ColBase` values used for derived CTE/subquery columns are intentionally untyped SQL references and must not satisfy `sqlmeta.ParamColumn`; they can render SQL but cannot be used with mutation param-capable APIs such as `SetParam(column query.MutationColumn, name)`. Predicate param APIs may use `sqlmeta.ParamColumn`; mutation param APIs require the composed `query.MutationColumn` contract so builders can also validate target-table ownership and aliases.

All column handles embed `ColBase`, which provides:
- `.IsNull()` / `.IsNotNull()`
- `.Asc()` / `.Desc()`
- `.EQCol(other)` — column-to-column equality (for JOIN conditions)
- `.As(alias)` — aliased SELECT expression

**Status:** PARITY in structure. Per-type operator gaps are documented in [query-builder.md](./query-builder.md).

## `BuildContext` contract

`expr.BuildContext` is threaded through every expression render call. It accumulates bound parameters, carries the active dialect, and provides a stable error path for invalid expression construction. Any new expression type must use it; never format SQL strings directly.

Query-level rendering that needs statement scope, such as `SelectLike` rendering with visible CTE names, must wrap `BuildContext` in a query-owned render context rather than adding query-package state to every expression node. The target `query.SelectRenderContext` / `query.PreparedSelectRenderContext` carry the `expr` build context plus the active immutable CTE namespace for nested select rendering.

```go
// NewBuildContext creates a fresh context for a single query.
func NewBuildContext(d dialect.Dialect) *BuildContext

// Add appends an unencoded bound value and returns its placeholder ("$1", "?", etc.).
// Use Add only for values with no column/custom encoder metadata.
func (c *BuildContext) Add(val any) string

// AddEncoded applies a column/custom encoder before binding and returns the
// dialect placeholder, including any encoder-requested placeholder cast/wrapper.
func (c *BuildContext) AddEncoded(val any, goType reflect.Type, enc sqlmeta.ParamEncoder, nullable bool) (string, error)

// Quote escapes and wraps one identifier part in dialect-appropriate quote characters.
func (c *BuildContext) Quote(name string) (string, error)

// ColRef returns the qualified "table"."col" reference (or just "col" if table is empty).
func (c *BuildContext) ColRef(table, name string) (string, error)

// Args returns the ordered slice of bound parameter values, for passing to the driver.
func (c *BuildContext) Args() []any

// Dialect returns the active dialect for feature-detection checks.
func (c *BuildContext) Dialect() dialect.Dialect
```

**Rule:** Every expression's `RenderSQL` implementation must call `ctx.Add(val)` for unencoded bound values or `ctx.AddEncoded(...)` for column/custom-encoded values, then use the returned placeholder in the SQL string. Never embed values directly in the string. This is what prevents SQL injection across the query builder while preserving dialect codecs.

Prepared rendering uses a distinct context so runtime placeholders can enter the ordered prepared plan instead of being treated as fixed bound values:

```go
type RuntimeParamSpec struct {
    Name     string
    Type     reflect.Type // nil only for raw placeholders with no type metadata
    Nullable bool
    Encoder  sqlmeta.ParamEncoder // nil only for raw/no-op runtime slots
    Source   string
}

type PreparedArgKind string

const (
    PreparedArgFixed   PreparedArgKind = "fixed"
    PreparedArgRuntime PreparedArgKind = "runtime"
)

type PreparedArg struct {
    Kind       PreparedArgKind
    Name       string
    FixedValue any
    Type       reflect.Type
    Nullable   bool
    Encoder    sqlmeta.ParamEncoder
    Source     string
}

func NewPreparedBuildContext(d dialect.Dialect) *PreparedBuildContext
func (c *PreparedBuildContext) AddFixed(val any) string
func (c *PreparedBuildContext) AddFixedEncoded(val any, goType reflect.Type, enc sqlmeta.ParamEncoder, nullable bool) (string, error)
func (c *PreparedBuildContext) AddRuntime(spec sqlmeta.RuntimeParamSpec) (placeholder string, err error)
func (c *PreparedBuildContext) Plan() []sqlmeta.PreparedArg
```

`RuntimeParamSpec`, `PreparedArgKind`, and `PreparedArg` belong in the lower `sqlmeta` package, not `query`, so `expr.PreparedBuildContext` does not import `query`. The `query` package may re-export aliases for user-facing docs and driver helpers.

Expressions that can contain runtime params must implement the `PreparedExpression` contract below. Param-capable APIs such as `EQParam(name)` and `SetParam(column query.MutationColumn, name)` render through `AddRuntime`; ordinary fixed values render through `AddFixed` or `AddFixedEncoded`. Calling the normal `RenderSQL` path on an expression with unresolved runtime params must return `build_validation` rather than silently treating the placeholder as a fixed value.

Column-backed fixed values in predicates, inserts, updates, upserts, conflict assignments, and assignment wrappers must bind through `AddEncoded` / `AddFixedEncoded`, not raw `Add`. The encoder normalizes the Go value to the final driver value and may cast or wrap the placeholder through `CastPlaceholder`. `goType` must be non-nil, and non-nil values must be assignable to that type before `Encode` unless the encoder explicitly documents a safe conversion path. Nil and typed-nil values bypass `Encode` only when the target is nullable; non-nullable targets fail with `build_validation` before binding. Nil and typed-nil encoders fail with `build_validation`. Encoder failures return `param_encode` or a more specific stable code and must be redacted. Failed encoded binding must not append partial arguments or mutate the prepared plan.

Encoded-binding conformance tests must cover wrong-type values, nil and typed-nil nullable/non-nullable values, nil and typed-nil encoders, JSON/custom encoders, placeholder cast failures, encoder errors, redaction, and no partial args/plan mutation after failure.

Identifier quoting rule:

- `Quote` accepts exactly one identifier part, not a dotted path
- PostgreSQL and SQLite quote with `"` and escape embedded `"` as `""`
- MySQL quotes with `` ` `` and escapes embedded `` ` `` as `` `` ``
- NUL bytes and unsupported control characters in identifiers must fail before SQL rendering
- tests must cover rejection of dotted paths, LF/CR, DEL, NUL, and unsupported controls, plus embedded quote escaping for PostgreSQL/SQLite/MySQL

## Expression interface

All WHERE-clause elements implement `expr.Expression`:

```go
type Expression interface {
    RenderSQL(ctx *BuildContext) (string, error)
}

type PreparedExpression interface {
    Expression
    RenderPreparedSQL(ctx *PreparedBuildContext) (string, error)
}

type SelectableColumn interface {
    Expression
    RenderSelectSQL(ctx *BuildContext) (sql string, alias string, err error)
    RenderOrderSQL(ctx *BuildContext) (string, error)
    RenderGroupSQL(ctx *BuildContext) (string, error)
    BaseColumnRef() (sqlmeta.ColumnRef, bool)
    SelectionKey() string
    Alias() string
}

type PreparedSelectableColumn interface {
    SelectableColumn
    PreparedExpression
    RenderPreparedSelectSQL(ctx *PreparedBuildContext) (sql string, alias string, err error)
    RenderPreparedOrderSQL(ctx *PreparedBuildContext) (string, error)
    RenderPreparedGroupSQL(ctx *PreparedBuildContext) (string, error)
}
```

This is the Go equivalent of Drizzle's internal `SQL` type, which carries both the SQL fragment and its parameter bindings. The difference is that Drizzle's `SQL` is a value (fragment + args together); Grizzle's `Expression` is an interface that renders itself into a shared `BuildContext` and can return stable rendering errors.

The error return is required for cases such as `RawArgs` placeholder mismatches, unsupported dialect features exposed through shared builders, and redacted diagnostics. Implementations must not panic with raw SQL or silently drop requested SQL clauses.

All package-owned expression and selectable-column nodes must implement both normal and prepared render contracts, even when prepared rendering simply routes fixed values through `AddFixed` / `AddFixedEncoded`. External expression implementations that do not satisfy `PreparedExpression` are accepted only in normal `Build`; `BuildPrepared` must reject them with `build_validation` unless a specific wrapper converts them into trusted fixed SQL with no runtime params. This keeps ordered prepared plans implementable without parsing rendered SQL.

External implementations of `Expression`, `SelectableColumn`, `PreparedExpression`, or `PreparedSelectableColumn` are trusted raw-SQL extensions. They must never incorporate untrusted strings into rendered SQL, must bind dynamic values through `BuildContext` / `PreparedBuildContext`, must return redacted errors, and are outside strict RC.1 parity unless wrapped by a package-owned API. Applications that do not want this trust boundary should avoid accepting external expression implementations across package/plugin boundaries.

Selectable-column render rules:

- `RenderSelectSQL` includes an alias when present and returns that alias separately for result-shape construction
- `RenderOrderSQL` and `RenderGroupSQL` must strip `AS <alias>` and render the underlying expression or base column, matching the alias-stripping rules in [query-builder.md](./query-builder.md)
- `BaseColumnRef` returns `(ref, true)` for generated source-owned columns representing concrete tables, generated views, and read-only aliases preserving source identity; raw expressions, aggregates, window expressions, CTE/subquery `expr.ColBase` references, and other derived expressions return `false`
- `SelectionKey` returns the generated RC.1 table/view property key for generated source-owned columns, the explicit select alias for aliased expressions that define a result key, or empty when a selectable cannot provide a typed selected-field key. Generated column constructors must receive this key through `sqlmeta.ColumnMeta`. Normal SELECT rendering may include empty-key selectables, but `SelectedFieldKeys()` must report `(nil, false)` for that selection and `Insert.Select` must reject it unless the caller uses the explicit trusted raw-select path. No-arg select-all planning must fail if generated select-all metadata contains an empty key.
- read/query contexts may use alias-preserving base-column metadata where a source column identity is useful, but all mutation ownership checks must compare the full `BaseColumnRef().Table` to the unaliased mutation target identity and reject any non-empty or different `TableRef.Alias`, even when the column's SELECT-list alias is empty
- conflict targets, excluded-column updates, update setters, MySQL duplicate-key setters, and insert column validation must use `BaseColumnRef` plus target-table metadata rather than parsing rendered SQL
- lock `OF` validation is table-scoped, not column-scoped: it must use the selected table source's `TableRef` and render the active alias when present

PostgreSQL-only regex and full-text search helpers are GRIZZLE-ONLY APIs. On non-PostgreSQL dialects, builders must omit these helpers or return `unsupported_feature`; they must not render boolean stand-ins such as `FALSE`/`NULL` because those do not compose safely under `NOT`, `AND`, or `OR`.

**Status:** PARITY in concept.

## Query Build Contract

Every public query builder `Build(dialect)` method must return an error:

```go
Build(d dialect.Dialect) (sql string, args []any, err error)
```

Builders may accumulate validation errors before rendering, especially from reflection-based helpers such as `SetStruct` and `DoUpdateSetStruct`. `Build` must return those errors instead of panicking or silently changing the requested SQL. Driver execution helpers that accept builders directly must call the same build path and return the build error before sending SQL to the database.

**Status:** DEVIATION:LANGUAGE. Drizzle RC.1 exposes SQL objects and throws for invalid builder state; Go callers need an explicit error return so validation failures compose with normal Go control flow while preserving the same SQL behavior.

Query build and prepared-statement errors must use the shared Grizzle error contract rather than ad hoc strings.

Minimum query error shape:

```go
type ErrorCode string

const (
    CodeUnsupportedFeature ErrorCode = "unsupported_feature"
    CodeUnsupportedDialect ErrorCode = "unsupported_dialect"
    CodeInvalidIdentifier  ErrorCode = "invalid_identifier"
    CodePreparedNotReady   ErrorCode = "prepared_not_ready"
    CodeRegistryClosed     ErrorCode = "registry_closed"
    CodeMissingParam       ErrorCode = "missing_param"
    CodeInvalidParamType   ErrorCode = "invalid_param_type"
    CodeInvalidParamValue  ErrorCode = "invalid_param_value"
    CodeParamEncode        ErrorCode = "param_encode"
    CodeInvalidResultKind  ErrorCode = "invalid_prepared_result_kind"
    CodeDuplicateRegistry  ErrorCode = "duplicate_registry_name"
    CodePreparedTxMismatch ErrorCode = "prepared_tx_mismatch"
    CodeInvalidReceiver    ErrorCode = "invalid_receiver"
    CodeBuildValidation    ErrorCode = "build_validation"
    CodeNotFound           ErrorCode = "not_found"
    CodeTooManyRows        ErrorCode = "too_many_rows"
    CodeInvalidRows        ErrorCode = "invalid_rows"
    CodeScanDecode         ErrorCode = "scan_decode"
    CodeTransactionBegin   ErrorCode = "transaction_begin"
    CodeTransactionCommit  ErrorCode = "transaction_commit"
    CodeTransactionRollback ErrorCode = "transaction_rollback"
    CodeTransactionCallback ErrorCode = "transaction_callback"
)

type Error struct {
    Code    ErrorCode
    Op      string
    Message string
    Err     error
}
```

Rules:

- errors expose stable codes for programmatic handling and redacted messages for humans
- `Build` and `BuildPrepared` must reject nil and typed-nil dialect values before invoking dialect methods, returning a stable redacted `unsupported_dialect` error rather than panicking
- public pointer-receiver driver helpers must reject nil and typed-nil DB, Tx, registry, prepared-handle, and rows inputs before method calls; use `invalid_receiver` for invalid DB/Tx/registry/handle receivers and the more specific row/scan codes for invalid row inputs
- `Error.Unwrap()` may expose only redacted safe causes, stable sentinels, or the standard context sentinels `context.Canceled` and `context.DeadlineExceeded`
- package-level sentinels or equivalent `errors.Is` support must exist for stable codes such as unsupported feature/dialect and prepared-not-ready
- build validation failures such as invalid structs, malformed tags, empty assignment sets, invalid zero-value opaque assignment structs, invalid manually constructed hints, and bad builder state map to `build_validation` unless a more specific stable code applies
- scan helpers and synthesized-row helpers use `not_found`, `too_many_rows`, `invalid_rows`, and `scan_decode` for cardinality, nil/typed-nil rows, database row decoding failures, and pre-scan synthesized row conversion failures such as MySQL `.ReturningID()` integer overflow/sign-loss checks
- transaction helpers use `transaction_begin`, `transaction_commit`, `transaction_rollback`, and `transaction_callback` while preserving the callback error as primary when rollback follows a callback failure
- query, prepared, transaction, and pgx driver paths must preserve `errors.Is(err, context.Canceled)` and `errors.Is(err, context.DeadlineExceeded)` while preserving the outer Grizzle `ErrorCode` and redaction rules
- raw SQL, bind values, credentials, raw driver errors, and raw builder/validation errors must not be recoverable through `Error()`, `Unwrap()`, logs, verbose diagnostics, or formatted `%+v` output unless a specific spec explicitly permits a redacted summary

## Null handling

### Schema level

Drizzle: a column without `.notNull()` has type `T | null` in TypeScript.
Grizzle: a column without `.NotNull()` generates a nullable scan representation in `*Select` structs, typically a pointer or another null-capable scan type. Nullable assignable fields in `*Insert` and `*Update` structs use the tri-state assignment contract documented below so callers can distinguish omitted fields from explicit SQL `NULL`.

**Status:** Target semantics are PARITY. The Go assignable-field API is DEVIATION:LANGUAGE because nullable insert/update fields need `query.Assign[T]`; implementation remains DEVIATION:GAP until the codegen/query-builder work listed in [codegen.md](./codegen.md#status) lands.

### Query level

Drizzle RC.1: `eq(col, null)` does not automatically emit `IS NULL`; upstream docs and source direct users to `isNull(col)` / `isNotNull(col)` for NULL checks.
Grizzle: `.IsNull()` and `.IsNotNull()` are explicit methods on all column handles. There is no automatic `IS NULL` when comparing to `nil`.

**Status:** PARITY

## Typed aggregates

Drizzle RC.1 aggregate helper types vary by function. `count()` and `countDistinct()` map to `number`; `max()` and `min()` preserve the source column data type where possible; `avg()` and `sum()` return `SQL<string | null>` because many SQL drivers return numeric aggregates as strings.

Grizzle's aggregates return `AggregateExpr` which implements `SelectableColumn`. The Go type of the scanned result depends on the struct field the aggregate is scanned into.

**Status:** DEVIATION:LANGUAGE — TypeScript can encode each aggregate helper's static result type; Go callers must ensure their scan struct field type matches the aggregate result. Grizzle docs should not claim that `avg()` or `sum()` infer the input column's numeric type in RC.1.

## Generics usage

Go generics are used in:

- `pgxdb.FromSelect[T]`, `pgxdb.FromSelectOne[T]`, `pgxdb.FromSelectOpt[T]`, `pgxdb.ScanAll[T]`, `pgxdb.ScanOne[T]`, `pgxdb.ScanOneOpt[T]` — scan result type
- `query.Pluck[T, K]` — extract field from slice
- `query.Index[K, T]` — build map from slice
- `query.GroupBy[K, T]` — build multimap from slice
- `query.First[T]` — first element
- `DateColumn`, `TimeColumn`, `BytesColumn`, `EnumColumn`, and `ArrayColumn` — generated handles for date, time-of-day, bytea/blob, enum, and array storage families
- `JSONColumn[T]` — plain JSON column with typed scan target across supported dialects; extraction helpers require a separate helper spec, and JSONB-only containment/existence helpers must not be present
- `JSONBColumn[T]` — JSONB column with typed scan target and PostgreSQL-only JSONB helper eligibility

JSON handle contract:

```go
func NewJSONColumn[T any](meta sqlmeta.ColumnMeta) JSONColumn[T]
func NewJSONBColumn[T any](meta sqlmeta.ColumnMeta) JSONBColumn[T]

func (c JSONBColumn[T]) Arrow(key string) SelectableColumn
func (c JSONBColumn[T]) ArrowText(key string) SelectableColumn
func (c JSONBColumn[T]) Path(keys ...string) SelectableColumn
func (c JSONBColumn[T]) PathText(keys ...string) SelectableColumn
func (c JSONBColumn[T]) Contains(value any) Expression
func (c JSONBColumn[T]) ContainedBy(value any) Expression
func (c JSONBColumn[T]) HasKey(key string) Expression
func (c JSONBColumn[T]) HasKeyNot(key string) Expression
func (c JSONBColumn[T]) HasAnyKey(keys ...string) Expression
func (c JSONBColumn[T]) HasAllKeys(keys ...string) Expression
func (c JSONBColumn[T]) DeletePath(keys ...string) SelectableColumn
```

- generated plain JSON columns use `expr.JSONColumn[T]`; generated `pg.JSONB()` columns use `expr.JSONBColumn[T]`
- both handle types must implement `sqlmeta.ParamColumn` using the generated `sqlmeta.ColumnMeta`, Go scan type `T`, nullability, and a JSON encoder
- JSON encoders must marshal Go values deterministically enough for driver binding, reject unsupported values before execution, and use distinct identities for `pg_json` and `pg_jsonb`
- JSON encoder identities are dialect-specific: `pg_json`, `pg_jsonb`, `mysql_json`, `sqlite_json_text`, and `sqlite_json_blob` are distinct so prepared plans cannot accidentally reuse the wrong codec
- PostgreSQL placeholder casts must be dialect-specific: plain JSON slots cast as `json` when a cast is needed, and JSONB slots cast as `jsonb` when a cast is needed
- JSONB key and path helper arguments are data values and must be bound through `Add` / `AddFixed` / `AddRuntime`, not inlined into SQL. Path helpers bind the path as a dialect-encoded text-array value or an equivalent placeholder representation; examples must not show `ARRAY['literal']` inlining for dynamic helper arguments.
- negated JSONB key predicates render as `NOT (<jsonb> ? <placeholder>)` with parentheses so they compose safely inside larger boolean expressions
- MySQL JSON placeholders render as normal value placeholders and rely on the driver/dialect JSON encoder; SQLite text-backed JSON uses normal text placeholders, while SQLite blob-backed JSON remains DEVIATION:GAP until its storage and encoder behavior are specified
- `JSONColumn[T]` must not expose JSONB-only containment/existence/delete-path helpers (`Contains`, `ContainedBy`, `HasKey`, `HasKeyNot`, `HasAnyKey`, `HasAllKeys`, `DeletePath`) unless the API explicitly casts to JSONB or uses raw SQL
- `JSONBColumn[T]` may expose typed PostgreSQL-only helpers for extraction (`Arrow`, `ArrowText`, `Path`, `PathText`) and JSONB containment/existence/delete-path operators; all such helpers must be gated to PostgreSQL-compatible dialects

Where Drizzle achieves type safety through TypeScript inference, Grizzle achieves it through generics and code generation. The two mechanisms are equivalent in user experience; differences are **DEVIATION:LANGUAGE**.

## `db` struct tag

Grizzle uses the `db` struct tag for column name mapping in INSERT/UPDATE/SELECT scanning, matching the convention used by `sqlx` and `pgx`. Drizzle has no equivalent (TypeScript uses property names directly).

For INSERT structs, the `omitempty` modifier on the `db` tag marks a nil-pointer field as unset. INSERT uses the dialect-specific missing-value/default behavior defined in [query-builder.md](./query-builder.md#single-row-multiple-rows). For UPDATE and UPSERT SET structs, nil pointer fields are omitted from the explicit SET object regardless of `omitempty`; the tag modifier is not required on update structs. Omitted runtime `$onUpdate` columns are a special case: Drizzle RC.1 invokes the runtime hook when there is a non-empty SET, while Grizzle must fail with `unsupported_feature` until a Go hook API exists.

`db:"-"` means the field is ignored for INSERT/UPDATE struct reflection and SELECT scanning. Empty tag names, duplicate names, and unsupported tag options fail with `build_validation` for mutation structs or `scan_decode` for scan targets. The only initial option is `omitempty`, and it is meaningful only for INSERT nil-pointer omission; unsupported options must not be silently ignored.

Nullable assignable fields need a tri-state generated type, such as `query.Assign[T]`, rather than a plain pointer:

- INSERT unset/omitted fields use dialect-specific missing-value/default behavior
- UPDATE and UPSERT SET unset fields omit ordinary assignments; omitted runtime `$onUpdate` columns fail with `unsupported_feature` when Drizzle would invoke the hook
- null emits SQL `NULL`
- value binds the concrete value

This distinction is required because Drizzle can express both omitted fields and explicit `null` assignments.

Initial `query.Assign[T]` contract:

```go
type AssignState string

const (
    AssignUnset AssignState = "unset"
    AssignNull  AssignState = "null"
    AssignValue AssignState = "value"
)

type Assign[T any] struct {
    // unexported representation
}

func Unset[T any]() Assign[T]
func Null[T any]() Assign[T]
func Value[T any](v T) Assign[T]
```

`Assign[T]` must expose a package-internal non-generic inspection interface for `query.InsertBuilder.Values`, `query.SetStruct`, and `query.DoUpdateSetStruct`, equivalent to:

```go
type nullableAssignment interface {
    assignState() AssignState
    assignValue() any
}
```

The zero value of `Assign[T]` must report `AssignUnset`. Invalid or unknown assignment states must make the consuming builder record a validation error and return it from `Build`.

Only `query.Assign[T]` is supported in the initial generated-code contract. Generated schema packages must use that type instead of attempting to implement `nullableAssignment`, because unexported inspection methods intentionally prevent third-party types from spoofing nullable assignment state.

**Status:** DEVIATION:LANGUAGE — a Go-idiomatic representation of Drizzle's nullable assignment semantics. Implementation remains DEVIATION:GAP until `query.Assign[T]` and the generated insert/update struct paths are complete.
