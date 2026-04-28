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
| `.$default(fn)` runtime function | No equivalent | DEVIATION:LANGUAGE |
| `.$defaultFn(fn)` | No equivalent | DEVIATION:LANGUAGE |
| `.$onUpdate(fn)` | `.OnUpdate()` marker; codegen emits a comment | DEVIATION:LANGUAGE |
| `.$onUpdateFn(fn)` | No equivalent | DEVIATION:LANGUAGE |
| `.references(tbl, col, opts)` | `.References(table, col, opts...)` | PARITY |
| `.generatedAlwaysAs(expr)` | DEVIATION:GAP (not designed) | — |
| *(no Drizzle equivalent)* | `.RenamedFrom(oldName)` | GRIZZLE-ONLY — see below |

## Column types

### PostgreSQL

In Drizzle, the column name is the first argument to the type function: `uuid('id')`. In Grizzle, the name is always passed to `pg.C()` and the type builder takes no name argument: `pg.C("id", pg.UUID())`. The table below shows only the type constructors.

| Drizzle | Grizzle | Status |
|---|---|---|
| `uuid(name)` | `pg.UUID()` | PARITY |
| `varchar(name, {length})` | `pg.Varchar(n)` | PARITY |
| `text(name)` | `pg.Text()` | PARITY |
| `boolean(name)` | `pg.Boolean()` | PARITY |
| `integer(name)` | `pg.Integer()` | PARITY |
| `bigint(name, {mode})` | `pg.BigInt()` | PARITY |
| `serial(name)` | `pg.Serial()` | PARITY |
| `bigserial(name)` | `pg.BigSerial()` | PARITY |
| `timestamp(name, opts)` | `pg.Timestamp().WithTimezone()` | PARITY |
| `numeric(name, {precision, scale})` | `pg.Numeric(p, s)` | PARITY |
| `json(name)` | `pg.JSON()` | PARITY |
| `jsonb(name)` | `pg.JSONB()` | PARITY |
| `date(name)` | DEVIATION:GAP (designed) | — |
| `time(name, opts)` | DEVIATION:GAP (designed) | — |
| `interval(name)` | DEVIATION:GAP (not designed) | — |
| `real(name)` | DEVIATION:GAP (designed) | — |
| `doublePrecision(name)` | DEVIATION:GAP (designed) | — |
| `char(name, {length})` | DEVIATION:GAP (designed) | — |
| `inet(name)` | DEVIATION:GAP (designed) | — |
| `cidr(name)` | DEVIATION:GAP (not designed) | — |
| `macaddr(name)` | DEVIATION:GAP (not designed) | — |
| `bytea(name)` | DEVIATION:GAP (designed) | — |
| `point(name)` | DEVIATION:GAP (not designed) | — |
| `line(name)` | DEVIATION:GAP (not designed) | — |
| `geometry(name)` | DEVIATION:GAP (not designed) | — |
| `pgEnum(name, vals)` | DEVIATION:GAP (not designed) | — |
| `vector(name, {dim})` | DEVIATION:GAP (not designed) | — |
| `halfvec(name, {dim})` | DEVIATION:GAP (not designed) | — |
| `tsvector(name)` | DEVIATION:GAP (not designed) — tracked as #140 | — |
| Array types (`.array()`) | DEVIATION:GAP (not designed) — tracked as #144 | — |
| Custom types (`.customType()`) | DEVIATION:GAP (not designed) | — |

### MySQL

| Drizzle | Grizzle | Status |
|---|---|---|
| `mysqlTable(name, cols)` | `mysql.Table(name, cols...)` | PARITY |
| `int(name)` | `mysql.Int()` | PARITY |
| `varchar(name, {length})` | `mysql.Varchar(n)` | PARITY |
| `text(name)` | `mysql.Text()` | PARITY |
| `boolean(name)` | `mysql.Boolean()` | PARITY |
| `timestamp(name)` | `mysql.Timestamp()` | PARITY |
| `bigint(name, opts)` | `mysql.BigInt()` | PARITY |
| `serial(name)` | `mysql.Serial()` | PARITY |
| `datetime(name, opts)` | DEVIATION:GAP (designed) | — |
| `date(name)` | DEVIATION:GAP (designed) | — |
| `time(name)` | DEVIATION:GAP (designed) | — |
| `year(name)` | `mysql.Year()` | PARITY |
| `float(name)` | DEVIATION:GAP (designed) | — |
| `double(name)` | DEVIATION:GAP (designed) | — |
| `decimal(name, opts)` | DEVIATION:GAP (designed) | — |
| `json(name)` | DEVIATION:GAP (designed) | — |
| `mediumint(name)` | `mysql.MediumInt()` | PARITY |
| `smallint(name)` | DEVIATION:GAP (designed) | — |
| `tinyint(name)` | DEVIATION:GAP (designed) | — |
| `binary(name)` | DEVIATION:GAP (not designed) | — |
| `varbinary(name)` | DEVIATION:GAP (not designed) | — |
| `char(name)` | DEVIATION:GAP (designed) | — |
| `mysqlEnum(name, vals)` | `mysql.Enum(vals...)` | PARITY |
| `mysqlSet(name, vals)` | `mysql.Set(vals...)` | PARITY |

### SQLite

| Drizzle | Grizzle | Status |
|---|---|---|
| `sqliteTable(name, cols)` | `sqlite.Table(name, cols...)` | PARITY |
| `text(name, opts)` | `sqlite.Text()` | PARITY |
| `integer(name, opts)` | `sqlite.Integer()` | PARITY |
| `real(name)` | `sqlite.Real()` | PARITY |
| `blob(name)` | `sqlite.Blob()` | PARITY |
| `numeric(name)` | `sqlite.Numeric(p, s)` | PARITY |
| `text(name, { mode: 'json' })` | `sqlite.JSON()` | PARITY — both store as TEXT; `.JSON()` sets the Go scan type to `any` |
| `blob(name, { mode: 'json' })` or `text(name, { mode: 'json' })` | `sqlite.JSONB()` | PARITY — stored as TEXT; use `.JSONB()` for schemas migrated from PostgreSQL |

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
| `primaryKey({columns})` composite | `pg.CompositePrimaryKey(cols...)` | PARITY |
| `unique({columns})` | `pg.UniqueConstraint(name, cols...)` | PARITY |
| `check(name, expr)` | `pg.Check(name, exprStr)` | PARITY for string expressions; typed expression form is DEVIATION:GAP (not designed) |
| `foreignKey({cols, refs})` composite | `pg.ForeignKey(name).From(cols).References(tbl, cols).Build()` | PARITY |

**Note on FK actions for non-PostgreSQL dialects:** FK `ON DELETE`/`ON UPDATE` actions are now evaluated for all dialects (pg, mysql, sqlite). Issue **#114** (previously dropped for MySQL/SQLite) was fixed in this PR by updating `gen/parser/eval.go` to accept FK options from any of the three dialect packages.

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

## `RenamedFrom()` — GRIZZLE-ONLY (kit rename detection)

**Status:** GRIZZLE-ONLY — there is no Drizzle TypeScript equivalent; Drizzle Kit infers renames interactively at diff time. Grizzle uses a schema annotation because Go has no runtime inference.

`.RenamedFrom(oldName)` can be called on any column or table builder to declare that the entity was renamed from `oldName` in the current migration step. The kit diff engine (`kit.Diff`) reads `PreviousName` from the snapshot and emits a `RENAME COLUMN` or `RENAME TABLE` change instead of drop+add.

```go
// Column rename: "email" was previously "email_address"
pg.C("email", pg.Varchar(255).NotNull().RenamedFrom("email_address"))

// Table rename: "users" was previously "accounts"
var Users = pg.Table("users", ...).RenamedFrom("accounts").Build()
```

**Rules:**
- Call `.RenamedFrom()` only in the schema version used to generate the migration. Once the migration has been applied, remove the call — it must not persist across snapshot saves. (The `PreviousName` field is tagged `json:"-"` to prevent it from appearing in committed snapshots.)
- `oldName` for columns is the bare column name. For tables without a schema, pass the bare table name; for schema-qualified tables, pass `"schema.tablename"` to match the snapshot key.
- If `oldName` does not match a dropped entity in the old snapshot, Diff falls back to drop+add silently.

**Rationale:** Drizzle Kit detects renames interactively (the user chooses "rename" vs "drop+add" during `drizzle-kit push`). In Grizzle the diff engine is non-interactive; the annotation is the only way to communicate intent without user prompting.

## `drizzle()` instance — DEVIATION:INTENTIONAL

**Drizzle:** `drizzle(client, { schema })` wires schema definitions to a database client and enables the relational query API via `db.query.users.findMany(...)`. It also establishes a query logger, custom type serializers, and query caching config.

**Grizzle:** No initialisation step exists. Query builders and relation definitions are used directly. This is **DEVIATION:INTENTIONAL**: Go does not need a runtime registry because type safety is enforced at compile time through generated types and the builder API.

The features enabled by `drizzle()` are handled as follows in Grizzle:
- Relational query API → not yet implemented (DEVIATION:GAP — see [relations.md](./relations.md))
- Query logger → not yet implemented (DEVIATION:GAP, not designed)
- Custom type serializers → handled via the `db` struct tag and `.Type("T")` column modifier
- Query caching → not planned
