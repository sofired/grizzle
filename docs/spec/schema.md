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
| `date(name)` | `pg.Date()` | PARITY — generates `expr.DateColumn`; Go type `time.Time` |
| `time(name, opts)` | `pg.Time()` / `pg.Time().WithTimezone()` | PARITY — generates `expr.TimestampColumn`; Go type `time.Time` |
| `interval(name)` | `pg.Interval()` | PARITY — Go type `string`; see codegen.md for rationale |
| `real(name)` | `pg.Real()` | PARITY — Go type `float64`; see codegen.md for rationale |
| `doublePrecision(name)` | `pg.DoublePrecision()` | PARITY — Go type `float64` |
| `char(name, {length})` | `pg.Char(n)` | PARITY — Go type `string` |
| `inet(name)` | `pg.Inet()` | PARITY — Go type `string`; see codegen.md for rationale |
| `cidr(name)` | `pg.Cidr()` | PARITY — Go type `string` |
| `macaddr(name)` | `pg.Macaddr()` | PARITY — Go type `string` |
| `bytea(name)` | `pg.Bytea()` | PARITY — Go type `[]byte` |
| `point(name)` | DEVIATION:GAP (not designed) | — |
| `line(name)` | DEVIATION:GAP (not designed) | — |
| `geometry(name)` | DEVIATION:GAP (not designed) | — |
| `pgEnum(name, vals)` (inline column) | `pg.Enum(typeName, vals...)` | PARITY — Go type `string`; references a named type defined with `pg.CreateEnum()` |
| `vector(name, {dim})` | DEVIATION:GAP (not designed) | — |
| `halfvec(name, {dim})` | DEVIATION:GAP (not designed) | — |
| `tsvector(name)` | `pg.Tsvector()` | PARITY — Go type `string`; `@@` matching already available via `Matches*` helpers on `TsvectorColumn`; additional FTS support tracked in #140 |
| Array types (`.array()`) | `pg.Array(inner)` | PARITY — Go type `any`; typed `[]T` generation tracked in #144 |
| Custom types (`.customType()`) | DEVIATION:GAP (not designed) | — |
| *(no Drizzle equivalent)* | `pg.Tsquery()` | GRIZZLE-ONLY — PostgreSQL `tsquery` storage column; Go type `string` |
| *(no Drizzle equivalent)* | `pg.Int4Range()`, `pg.Int8Range()`, `pg.NumRange()`, `pg.TsRange()`, `pg.TstzRange()`, `pg.DateRange()` | GRIZZLE-ONLY — PostgreSQL range types; Go type `string` |

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
| `real(name)` | DEVIATION:GAP (designed) | — |
| `blob(name)` | DEVIATION:GAP (designed) | — |
| `numeric(name)` | DEVIATION:GAP (designed) | — |

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

## Views — GRIZZLE-ONLY (kit migration support)

**Drizzle:**
```typescript
const activeUsers = pgView('active_users').as((qb) =>
  qb.select().from(users).where(eq(users.enabled, true))
)
```

**Grizzle:**
```go
var ActiveUsers = pg.CreateView("active_users",
    `SELECT id, username, email FROM users WHERE enabled = true`)
```

| Drizzle | Grizzle | Status |
|---|---|---|
| `pgView(name).as(qb)` or `.as(sql\`...\`)` | `pg.CreateView(name, sql)` | PARITY — raw SQL path; query builder form is DEVIATION:LANGUAGE (Go has no template literal types) |
| `pgSchema("s").view(name).as(...)` | `pg.SchemaView(schema, name, sql)` | PARITY |
| `pgMaterializedView(name)` | DEVIATION:GAP (not designed) | — |

**Note on kit support:** Drizzle Kit v0.30 does not support views in migrations — views must be managed manually. Grizzle's kit fully supports views in `Diff`, `Push`, and `Migrate` via `ChangeCreateView`, `ChangeReplaceView`, and `ChangeDropView`. This is **GRIZZLE-ONLY** capability.

`pg.CreateView(name, sql)` panics if `name` or `sql` is empty.

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

`pg.CreateEnum` and `pg.SchemaCreateEnum` panic if `name` is empty or if any value is empty. Values must be declared in the order they should appear in the database — PostgreSQL preserves declaration order and `ALTER TYPE ... ADD VALUE` uses `AFTER`/`BEFORE` anchors to maintain ordering when new values are inserted.

See also `pg.Enum(typeName, vals...)` in the column types table for the inline MySQL-style enum variant (no separate `CREATE TYPE` statement).

## `drizzle()` instance — DEVIATION:INTENTIONAL

**Drizzle:** `drizzle(client, { schema })` wires schema definitions to a database client and enables the relational query API via `db.query.users.findMany(...)`. It also establishes a query logger, custom type serializers, and query caching config.

**Grizzle:** No initialisation step exists. Query builders and relation definitions are used directly. This is **DEVIATION:INTENTIONAL**: Go does not need a runtime registry because type safety is enforced at compile time through generated types and the builder API.

The features enabled by `drizzle()` are handled as follows in Grizzle:
- Relational query API → not yet implemented (DEVIATION:GAP — see [relations.md](./relations.md))
- Query logger → not yet implemented (DEVIATION:GAP, not designed)
- Custom type serializers → handled via the `db` struct tag and `.Type("T")` column modifier
- Query caching → not planned
