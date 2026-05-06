# Schema DSL Specification

The schema DSL is the Go equivalent of [Drizzle's schema declaration](https://orm.drizzle.team/docs/sql-schema-declaration).

## Design principle

Drizzle schema definitions are plain TypeScript objects — no decorators, no class inheritance, no magic. Grizzle follows the same principle: schema definitions are plain Go values constructed with builder functions.

## Tables

**Drizzle:**
```typescript
const users = pgTable('users', {
  id: uuid('id').primaryKey().defaultRandom(),
  username: varchar('username', { length: 255 }).notNull(),
})
```

**Grizzle:**
```go
var Users = pg.Table("users",
    pg.C("id",       pg.UUID().PrimaryKey().DefaultRandom()),
    pg.C("username", pg.Varchar(255).NotNull()),
)
```

`pg.C(name, builder)` pairs a column name with its type builder. In Drizzle the name is passed to the type function directly (`uuid('id')`); in Grizzle the name is always the first argument to `pg.C()` and the builder is the type only (`pg.UUID()`). The structures are equivalent.

Drizzle RC.1 table-creator helpers such as `pgTableCreator`, `mysqlTableCreator`, and `sqliteTableCreator` customize generated table names. They are **DEVIATION:GAP (not designed)** in the initial Grizzle static schema loader; schema files must call the dialect table builders directly so table names are explicit at load time.

**Status:** PARITY in structure; see column types below for per-type status.

## Column modifier parity

| Drizzle modifier | Grizzle equivalent | Status |
|---|---|---|
| `.notNull()` | `.NotNull()` | PARITY |
| `.primaryKey()` | `.PrimaryKey()` | PARITY |
| `.unique()` | `.Unique()` | PARITY |
| `.default(val)` static value | `.Default(val)` | PARITY |
| `.defaultNow()` | `.DefaultNow()` | PARITY |
| `.defaultRandom()` | `.DefaultRandom()` | PARITY |
| `.$defaultFn(fn)` / `.$default(fn)` runtime insert hook aliases | No DDL equivalent; codegen emits metadata/comment, and INSERT builders fail with `unsupported_feature` when Drizzle would invoke the hook | DEVIATION:GAP (designed) |
| `.$onUpdateFn(fn)` / `.$onUpdate(fn)` runtime update/insert hook aliases | `.OnUpdate()` marker; codegen emits metadata/comment, and INSERT plus UPDATE/UPSERT builders fail with `unsupported_feature` when Drizzle would invoke the hook | DEVIATION:GAP (designed) |
| `.references(tbl, col, opts)` | `.References(table, col, opts...)` | PARITY |
| `.generatedAlwaysAs(expr)` | Recognized in RC.1 snapshots but unsupported in initial Grizzle file-migration schema input | DEVIATION:GAP (designed); must fail with `unsupported_feature` |
| MySQL `.autoincrement()` | `.AutoIncrement()` on integer-family builders | PARITY target; missing builder coverage is DEVIATION:GAP until implemented |
| MySQL date/timestamp `.onUpdateNow({ fsp })` | `.OnUpdateNow(fsp?)` marker only | DEVIATION:GAP (designed); initial file migrations validate and reject `onUpdateNow` / `onUpdateNowFsp` with `unsupported_feature` rather than serializing them |
| SQLite integer `.primaryKey({ autoIncrement })` | `.PrimaryKey(sqlite.AutoIncrement())` or equivalent | PARITY target; represented by `ColumnDef.AutoIncrement` in the file-migration schema input |
| PostgreSQL unique `nullsNotDistinct()` | `.NullsNotDistinct()` on unique constraints | DEVIATION:GAP (designed) until represented in schema input and snapshot serialization |

## Column types

### PostgreSQL

In Drizzle, the column name is the first argument to the type function: `uuid('id')`. In Grizzle, the name is always passed to `pg.C()` and the type builder takes no name argument: `pg.C("id", pg.UUID())`. The table below shows only the type constructors.

| Drizzle | Grizzle | Status |
|---|---|---|
| `uuid(name)` | `pg.UUID()` | PARITY |
| `varchar(name, { length?, enum? })` | `pg.Varchar(n?)` plus Go enum/type policy | PARITY for DDL; Drizzle `enum` is TypeScript type narrowing and maps to Go codegen/type policy as DEVIATION:LANGUAGE until represented |
| `text(name, { enum? })` | `pg.Text()` plus Go enum/type policy | PARITY for DDL; Drizzle `enum` is TypeScript type narrowing and maps to Go codegen/type policy as DEVIATION:LANGUAGE until represented |
| `boolean(name)` | `pg.Boolean()` | PARITY |
| `integer(name)` | `pg.Integer()` | PARITY |
| `smallint(name)` | `pg.SmallInt()` | PARITY target; DEVIATION:GAP until implemented if missing in current builders |
| `bigint(name, {mode})` | `pg.BigInt()` plus mode-specific Go type mapping | PARITY for DDL; type/API mode coverage must distinguish `number` vs `bigint` as DEVIATION:GAP until implemented |
| `serial(name)` | `pg.Serial()` | PARITY |
| `smallserial(name)` | `pg.SmallSerial()` | PARITY target; DEVIATION:GAP until implemented if missing in current builders |
| `bigserial(name, { mode })` | `pg.BigSerial()` plus mode-specific Go type mapping | PARITY for DDL; type/API mode coverage must distinguish `number` vs `bigint` as DEVIATION:GAP until implemented |
| `timestamp(name, { mode?, precision?, withTimezone? })` | `pg.Timestamp()` / `.WithTimezone()` plus precision support | PARITY for DDL; precision and mode-specific Go scan behavior are DEVIATION:GAP until implemented |
| `numeric(name, { precision?, scale?, mode? })` / `decimal(...)` | `pg.Numeric(precision?, scale?)` plus mode-specific Go type mapping | PARITY for DDL; precision-only and mode/type API coverage are DEVIATION:GAP until implemented |
| `json(name)` | `pg.JSON()` | PARITY |
| `jsonb(name)` | `pg.JSONB()` | PARITY |
| `date(name, {mode})` | `pg.Date()` | PARITY for DDL; Drizzle mode-specific TypeScript type behavior maps to Go codegen policy and is DEVIATION:LANGUAGE where not represented |
| `time(name, { precision?, withTimezone? })` | `pg.Time()` / `pg.Time().WithTimezone()` plus precision support | PARITY for DDL; precision support is DEVIATION:GAP until implemented |
| `interval(name, { fields?, precision? })` | `pg.Interval()` plus fields/precision options | PARITY target for DDL; fields/precision API coverage is DEVIATION:GAP until specified/implemented; Go type `string`; see codegen.md for rationale |
| `real(name)` | `pg.Real()` | PARITY — Go type `float64`; see codegen.md for rationale |
| `doublePrecision(name)` | `pg.DoublePrecision()` | PARITY — Go type `float64` |
| `char(name, { length?, enum? })` | `pg.Char(n?)` plus Go enum/type policy | PARITY for DDL; Drizzle `enum` is TypeScript type narrowing and maps to Go codegen/type policy as DEVIATION:LANGUAGE until represented |
| `inet(name)` | `pg.Inet()` | PARITY — Go type `string`; see codegen.md for rationale |
| `cidr(name)` | `pg.Cidr()` | PARITY — Go type `string` |
| `macaddr(name)` | `pg.Macaddr()` | PARITY — Go type `string` |
| `macaddr8(name)` | `pg.Macaddr8()` | PARITY target; Go type `string`; DEVIATION:GAP until implemented if missing in current builders |
| `bytea(name)` | `pg.Bytea()` | PARITY — Go type `[]byte` |
| `point(name)` | DEVIATION:GAP (not designed) | — |
| `line(name)` | DEVIATION:GAP (not designed) | — |
| `geometry(name)` | DEVIATION:GAP (not designed) | — |
| column using a declared `pgEnum` factory | `pg.EnumColumn(typeName)` | PARITY — Go type `string`; references a named type defined with `pg.CreateEnum()` |
| `vector(name, { dimensions })` | DEVIATION:GAP (not designed) | — |
| `halfvec(name, { dimensions })` | DEVIATION:GAP (not designed) | — |
| `bit(name, { dimensions })` | DEVIATION:GAP (not designed) | — |
| `sparsevec(name, { dimensions })` | DEVIATION:GAP (not designed) | — |
| *(no exported RC.1 PostgreSQL column builder)* | `pg.Tsvector()` | GRIZZLE-ONLY — PostgreSQL `tsvector` storage column/source-generation support; Go type `string`; typed FTS helpers remain PostgreSQL-only Grizzle conveniences |
| Array types (`.array()` / `.array('[][]')`) | `pg.Array(inner, dimensions?)` or equivalent | PARITY target for one-dimensional arrays; explicit multidimensional dimension strings are DEVIATION:GAP until specified/implemented; Go type `any`; typed `[]T` generation tracked in #144 |
| Custom types (`.customType()`) | DEVIATION:GAP (not designed) | — |
| *(no Drizzle equivalent)* | `pg.Tsquery()` | GRIZZLE-ONLY — PostgreSQL `tsquery` storage column; Go type `string` |
| *(no Drizzle equivalent)* | `pg.Int4Range()`, `pg.Int8Range()`, `pg.NumRange()`, `pg.TsRange()`, `pg.TstzRange()`, `pg.DateRange()` | GRIZZLE-ONLY — PostgreSQL range types; Go type `string` |

### MySQL

| Drizzle | Grizzle | Status |
|---|---|---|
| `mysqlTable(name, cols)` | `mysql.Table(name, cols...)` | PARITY |
| `int(name, { unsigned? })` | `mysql.Int()` plus unsigned option | PARITY for DDL and generated signed/unsigned Go type mapping; see [codegen.md](./codegen.md#go-type-mappings) |
| `varchar(name, { length, enum? })` | `mysql.Varchar(n)` plus Go enum/type policy | PARITY for DDL; Drizzle `enum` is TypeScript type narrowing and maps to Go codegen/type policy as DEVIATION:LANGUAGE until represented |
| `text(name, { enum? })` | `mysql.Text()` plus Go enum/type policy | PARITY for DDL; Drizzle `enum` is TypeScript type narrowing and maps to Go codegen/type policy as DEVIATION:LANGUAGE until represented |
| `boolean(name)` | `mysql.Boolean()` | PARITY |
| `timestamp(name, { mode?, fsp? })` | `mysql.Timestamp()` plus mode/fsp mapping | PARITY for DDL; mode/fsp API coverage is DEVIATION:GAP until specified |
| `bigint(name, { mode, unsigned? })` | `mysql.BigInt()` plus mode/unsigned mapping | PARITY for DDL and generated signed/unsigned Go type mapping; Drizzle mode-specific API behavior remains DEVIATION:GAP until represented |
| `serial(name)` | `mysql.Serial()` | PARITY for DDL; generated Go type `uint64` is DEVIATION:LANGUAGE from RC.1's JavaScript `number`/uint53 surface to preserve MySQL's unsigned physical range |
| `datetime(name, { mode?, fsp? })` | `mysql.DateTime()` plus mode/fsp mapping | PARITY target; DEVIATION:GAP until API/options are specified/implemented |
| `date(name, { mode? })` | `mysql.Date()` plus mode-specific Go type mapping | PARITY target for DDL; mode is a type/API mapping concern and is DEVIATION:GAP until specified/implemented |
| `time(name, { fsp? })` | `mysql.Time()` plus fsp option | PARITY target; DEVIATION:GAP until API/options are specified/implemented |
| `year(name)` | `mysql.Year()` | PARITY |
| `float(name, { precision?, scale?, unsigned? })` | `mysql.Float()` plus precision/scale/unsigned options | PARITY target; DEVIATION:GAP until API/options are specified/implemented |
| `double(name, { precision?, scale?, unsigned? })` | `mysql.Double()` plus precision/scale/unsigned options | PARITY target; DEVIATION:GAP until API/options are specified/implemented |
| `real(name, { precision?, scale? })` | `mysql.Real()` plus precision/scale options | PARITY target; DEVIATION:GAP until API/options are specified/implemented |
| `decimal(name, { precision?, scale?, unsigned?, mode? })` | `mysql.Decimal(precision?, scale?)` plus unsigned/mode-specific Go type mapping | PARITY target; DEVIATION:GAP until API/options are specified/implemented |
| `json(name)` | `mysql.JSON()` | PARITY target; DEVIATION:GAP until implemented |
| `mediumint(name, { unsigned? })` | `mysql.MediumInt()` plus unsigned option | PARITY for DDL and generated signed/unsigned Go type mapping |
| `smallint(name, { unsigned? })` | `mysql.SmallInt()` plus unsigned option | PARITY target for DDL and generated signed/unsigned Go type mapping; DEVIATION:GAP until builder coverage is implemented if missing |
| `tinyint(name, { unsigned? })` | `mysql.TinyInt()` plus unsigned option | PARITY target for DDL and generated signed/unsigned Go type mapping; DEVIATION:GAP until builder coverage is implemented if missing |
| `binary(name)` | DEVIATION:GAP (not designed) | — |
| `varbinary(name)` | DEVIATION:GAP (not designed) | — |
| `blob(name)` | DEVIATION:GAP (not designed) | — |
| `longblob(name)` | DEVIATION:GAP (not designed) | — |
| `mediumblob(name)` | DEVIATION:GAP (not designed) | — |
| `tinyblob(name)` | DEVIATION:GAP (not designed) | — |
| `char(name, { length?, enum? })` | `mysql.Char(n?)` plus Go enum/type policy | PARITY target for DDL; DEVIATION:GAP until API/options are specified/implemented |
| `longtext(name, { enum? })` | `mysql.LongText()` plus Go enum/type policy | PARITY target for DDL; DEVIATION:GAP until implemented |
| `mediumtext(name, { enum? })` | `mysql.MediumText()` plus Go enum/type policy | PARITY target for DDL; DEVIATION:GAP until implemented |
| `tinytext(name, { enum? })` | `mysql.TinyText()` plus Go enum/type policy | PARITY target for DDL; DEVIATION:GAP until implemented |
| `mysqlEnum(name, vals)` | `mysql.Enum(vals...)` | PARITY |
| `customType(...)` | DEVIATION:GAP (not designed) | — |
| *(no RC.1 builder)* | `mysql.Set(vals...)` | GRIZZLE-ONLY existing extension; not Drizzle RC.1 parity |

### SQLite

| Drizzle | Grizzle | Status |
|---|---|---|
| `sqliteTable(name, cols)` | `sqlite.Table(name, cols...)` | PARITY |
| `text(name, { mode?, length?, enum? })` | `sqlite.Text()` plus mode/length/enum mapping | PARITY for DDL; full mode/type API coverage is DEVIATION:GAP until specified |
| `integer(name, { mode })` | `sqlite.Integer()` plus timestamp/timestamp_ms/boolean mode mapping | PARITY for DDL; full mode/type API coverage is DEVIATION:GAP until specified |
| `real(name)` | `sqlite.Real()` | PARITY |
| `blob(name, { mode })` | `sqlite.Blob()` plus buffer/json/bigint mode mapping | PARITY for DDL; full mode/type API coverage is DEVIATION:GAP until specified |
| `numeric(name, { mode })` | `sqlite.Numeric()` plus explicit mode mapping | PARITY target; current `sqlite.Numeric(p, s)` is DEVIATION:GAP until revised because RC.1 SQLite numeric models mode, not precision/scale |
| `text(name, { mode: 'json' })` | `sqlite.JSON()` | PARITY target — text-backed JSON |
| `blob(name, { mode: 'json' })` | dedicated blob-backed JSON mapping required | DEVIATION:GAP (designed); current `sqlite.JSONB()` text storage is a Grizzle/PostgreSQL-migration convenience, not RC.1 SQLite parity |
| `customType(...)` | DEVIATION:GAP (not designed) | — |

## Table-level constraints

### `pg.TableRef`

The `.WithConstraints(fn)` callback receives a `pg.TableRef` value. `TableRef` provides `.Col(name string)` which returns a column reference for use in index and constraint builders. It does not provide typed column handles — it is only for name resolution within the constraint definition.

```go
var Users = pg.Table("users", ...).WithConstraints(func(t pg.TableRef) []pg.Constraint {
    return []pg.Constraint{
        pg.UniqueIndex("users_username_idx").On(t.Col("username")).Build(),
    }
})
```

### Indexes

| Drizzle | Grizzle | Status |
|---|---|---|
| `index(name).on(cols)` | `pg.Index(name).On(t.Col(name)...).Build()` | PARITY |
| `uniqueIndex(name).on(cols)` | `pg.UniqueIndex(name).On(t.Col(name)...).Build()` | PARITY |
| `.where(expr)` partial index | `.Where(expr)` | PARITY |
| `primaryKey({ columns })` / `primaryKey({ name?, columns })` composite | `pg.CompositePrimaryKey(cols...)` / `pg.CompositePrimaryKeyNamed(name, cols...)` | PARITY target with `nameExplicit` preservation |
| `unique({ columns })` / `unique(name).on(...)` | unnamed unique builder plus `pg.UniqueConstraint(name, cols...)` | PARITY target with `nameExplicit` preservation |
| `check(name, expr)` | `pg.Check(name, ddl.Expression)` | PARITY target; raw-string `pg.Check(name, exprStr)` is trusted-internal only and not public file-migration input |
| `foreignKey({ columns, foreignColumns, name? })` composite | unnamed FK builder plus `pg.ForeignKey(name).From(cols).References(tbl, cols).Build()` | PARITY target with `nameExplicit` preservation |

Constraint naming must preserve RC.1's explicit-vs-generated-name distinction in snapshots:

- unnamed primary keys use `pg.CompositePrimaryKey(cols...)` and serialize `nameExplicit=false`
- explicitly named primary keys use a named variant such as `pg.CompositePrimaryKeyNamed(name, cols...)` and serialize `nameExplicit=true`
- unique and foreign-key builders need both unnamed and explicitly named forms; explicit-name-only support is not parity and must be labeled `DEVIATION:GAP` until unnamed builders are implemented
- empty names are not accepted as a stand-in for unnamed constraints because they are ambiguous in Go APIs and diagnostics

Typed check-expression contract:

- public file-migration schema input must represent checks with the shared typed expression model, but render through `schema/ddl` / `ddl.BuildContext` with literalization rather than the query placeholder renderer
- rendered check SQL is stored in `snapshot.json` as the RC.1 `value` / expression string after dialect validation
- raw string check expressions are not Drizzle RC.1 parity because RC.1 uses typed `SQL` values
- raw string checks are treated as a temporary trusted-internal API only and must not be accepted from untrusted schema input

**Note on FK actions for non-PostgreSQL dialects:** FK `ON DELETE`/`ON UPDATE` actions are evaluated for PostgreSQL, MySQL, and SQLite schema definitions.

## Schema namespaces

**Drizzle:**
```typescript
const mySchema = pgSchema('myschema')
const users = mySchema.table('users', { ... })
```

**Grizzle:**
```go
var Users = pg.SchemaTable("myschema", "users", ...).Build()
```

**Status:** PARITY for basic schema qualification. The `pgSchema` shared-object pattern (reusable schema reference) is DEVIATION:GAP (not designed).

Generated query handles for schema-qualified tables must expose schema and table as separate identifier parts through the table identity contract in [query-builder.md](./query-builder.md#table-identity-contract). Dotted table names are invalid because they would be quoted as one identifier part.

## Views

**Drizzle:**
```typescript
const activeUsers = pgView('active_users').as((qb) =>
  qb.select().from(users).where(eq(users.enabled, true))
)
```

**Grizzle:**
```go
var ActiveUsers = pg.CreateView("active_users").As(
    ddl.Select(
        ddl.Table("users").Col("id"),
        ddl.Table("users").Col("username"),
        ddl.Table("users").Col("email"),
    ).From(ddl.Table("users")).Where(ddl.EQ(ddl.Table("users").Col("enabled"), ddl.Lit(true))),
)
```

| Drizzle | Grizzle | Status |
|---|---|---|
| `pgView(name).as(qb)` | `pg.CreateView(name).As(expr)` typed DDL-expression view definition | PARITY target |
| `pgView(name, columns).as(sql\`...\`)` manual/raw-SQL view definition | `pg.CreateView(name).Columns(cols...).As(ddl.RawTrusted(sql))` with explicit column metadata | PARITY target for trusted raw SQL plus explicit generated-column metadata; raw SQL without column metadata is unsupported for generated view handles |
| `pgSchema("s").view(name).as(...)` | `pg.SchemaView(schema, name).As(expr)` typed DDL-expression schema view definition | PARITY target |
| `mysqlView(name).as(...)` | `mysql.CreateView(name).As(expr)` typed DDL-expression view definition | PARITY target |
| `mysqlSchema("s").view(name).as(...)` | query-surface schema view reference only; not accepted as initial file-migration schema input | DEVIATION:GAP for file migrations; RC.1 Kit MySQL snapshots do not serialize schema-qualified views |
| `sqliteView(name).as(...)` | `sqlite.CreateView(name).As(expr)` typed DDL-expression view definition | PARITY target |
| `.existing()` on views | Query-only existing-view declaration; not initial strict file-migration schema input | PARITY for query references if implemented; DEVIATION:GAP for file-migration input |
| `pgMaterializedView(name)` | Recognized in RC.1 snapshots but unsupported in initial Grizzle file-migration schema input | DEVIATION:GAP (designed); must fail with `unsupported_feature` |

Untyped raw view SQL strings are not public file-migration schema input. Public view definitions should render through the typed DDL-expression path. When typed DDL cannot express an RC.1-compatible view yet, callers may use `ddl.RawTrusted(sql)` as an explicit trusted-input escape hatch; the SQL must be a literal/constant under the same static-loader and redaction rules as other DDL expressions.

Schema-authored views that should generate query handles must have explicit view-column metadata with deterministic property keys. `ViewColumnDef.PropertyKey` is the generated source property key used for `ColumnMeta.SelectionKey` and `GrizSelectAllFieldKeys()`; when omitted for explicit column metadata, loaders derive it from `Name` using Grizzle's schema/codegen default identifier casing, `camel`, and must reject empty keys or collisions before code generation. Typed `ddl.Select(...)` view definitions may derive view-column metadata from selected generated columns and explicit aliases. Trusted raw view definitions without explicit column metadata must fail for generated view-handle output; they may still be represented as unsupported/file-migration DDL input until a safe generated-column contract is provided.

Exact target API rule:

- file-migration-facing view constructors return a builder requiring `.As(ddl.Expression)` before serialization
- `.Existing()` view declarations are allowed only as query/source-generation references in the initial target. Strict file-migration schema loading must reject `IsExisting` view inputs with `unsupported_feature` rather than silently dropping them, including PostgreSQL/MySQL existing views that RC.1 serializers skip and SQLite `isExisting` records that RC.1 can serialize.
- the legacy current constructors `pg.CreateView(name, sql string)` and `pg.SchemaView(schema, name, sql string)` are trusted-internal or legacy compatibility APIs only; they must not be accepted by the strict file-migration schema loader unless converted to `ddl.RawTrusted(sql)` by a trusted introspection path or rewritten to a typed `ddl.Expression`
- empty names, empty definitions, unsupported view options, and unsupported materialized-view fields must become typed validation errors in file-migration flows, not panics
- MySQL schema-qualified tables/views may exist in the ORM/query surface, but RC.1 Kit file-migration serialization skips schema-qualified MySQL tables and has no schema field for MySQL views. Initial Grizzle file migrations must not label schema-qualified MySQL table/view artifacts as RC.1 Kit parity; accept them only in query/codegen surfaces or fail with `unsupported_feature` before snapshot serialization.

**Note on kit support:** Drizzle RC.1 snapshot and DDL models include views for PostgreSQL, MySQL, and SQLite. Grizzle's current legacy live-diff helpers have partial PostgreSQL-shaped view support and should not be treated as the target file-migration contract. RC.1-aligned file migrations must add dialect view definitions/renderers for all supported initial dialects or fail with `unsupported_feature` for view options they cannot represent.

Current legacy `pg.CreateView(name, sql string)` panics if `name` or `sql` is empty. That panic behavior is not the file-migration contract; file-migration schema loading and serialization must surface stable validation errors such as `unsupported_schema_construct`, `unsupported_feature`, or `invalid_identifier`.

## Named enum types (PostgreSQL)

**Drizzle:**
```typescript
const statusEnum = pgEnum('status', ['pending', 'active', 'archived'])

export const orders = pgTable('orders', {
  status: statusEnum(),
})
```

**Grizzle:**
```go
var StatusEnum = pg.CreateEnum("status", "pending", "active", "archived")

var Orders = pg.Table("orders",
    pg.C("status", pg.EnumColumn("status").NotNull()),
)
```

| Drizzle | Grizzle | Status |
|---|---|---|
| `pgEnum(name, vals)` | `pg.CreateEnum(name, vals...)` | PARITY |
| `pgSchema("s").enum(name, vals)` | `pg.SchemaCreateEnum(schema, name, vals...)` | PARITY |
| `enumCol()` — column referencing named type | `pg.EnumColumn(typeName)` | PARITY |

`pg.CreateEnum` and `pg.SchemaCreateEnum` currently panic if `name` is empty or if any value is empty. File-migration schema loading must convert those invalid inputs into stable validation errors rather than relying on panics. Values must be declared in the order they should appear in the database — PostgreSQL preserves declaration order and `ALTER TYPE ... ADD VALUE` uses `AFTER`/`BEFORE` anchors to maintain ordering when new values are inserted.

See also the PostgreSQL column-type table row for columns that reference a declared enum factory.

## Rename Resolution

Schema-level rename annotations are not part of the initial RC.1-aligned target.

Drizzle RC.1 resolves migration renames interactively during `generate`, not through schema annotations. Grizzle must match that behavior for the initial file-migrations implementation. See [file-migrations-generate.md](./file-migrations-generate.md#rename-handling).

## `drizzle()` instance — DEVIATION:INTENTIONAL

**Drizzle:** `drizzle(client, { schema })` wires schema definitions to a database client and enables the relational query API via `db.query.users.findMany(...)`. It also establishes a query logger, custom type serializers, and query caching config.

**Grizzle:** No initialization step exists. Query builders and relation definitions are used directly. This is **DEVIATION:INTENTIONAL**: Go does not need a runtime registry because type safety is enforced at compile time through generated types and the builder API.

The features enabled by `drizzle()` are handled as follows in Grizzle:
- Relational query API → not yet implemented (DEVIATION:GAP — see [relations.md](./relations.md))
- Query logger → not yet implemented (DEVIATION:GAP, not designed)
- Custom type serializers/codecs → out of initial scope (DEVIATION:GAP, not designed); `db` struct tags map scan fields, and `.Type("T")` is only a generated Go type/scan hint, not Drizzle `customType(...)` serializer parity
- Query caching → not planned in the initial target (**DEVIATION:GAP (not designed)**)
