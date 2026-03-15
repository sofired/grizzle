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
| `year(name)` | DEVIATION:GAP (not designed) — tracked as #130 | — |
| `float(name)` | DEVIATION:GAP (designed) | — |
| `double(name)` | DEVIATION:GAP (designed) | — |
| `decimal(name, opts)` | DEVIATION:GAP (designed) | — |
| `json(name)` | DEVIATION:GAP (designed) | — |
| `mediumint(name)` | DEVIATION:GAP (not designed) — tracked as #130 | — |
| `smallint(name)` | DEVIATION:GAP (designed) | — |
| `tinyint(name)` | DEVIATION:GAP (designed) | — |
| `binary(name)` | DEVIATION:GAP (not designed) | — |
| `varbinary(name)` | DEVIATION:GAP (not designed) | — |
| `char(name)` | DEVIATION:GAP (designed) | — |
| `mysqlEnum(name, vals)` | DEVIATION:GAP (not designed) — tracked as #130 | — |

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

**Note on FK actions for non-PostgreSQL dialects:** FK `ON DELETE`/`ON UPDATE` actions are silently dropped for SQLite and MySQL schemas — bug **#114**. The parser only evaluates these when `BasePkg == "pg"`.

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

## `drizzle()` instance — DEVIATION:INTENTIONAL

**Drizzle:** `drizzle(client, { schema })` wires schema definitions to a database client and enables the relational query API via `db.query.users.findMany(...)`. It also establishes a query logger, custom type serializers, and query caching config.

**Grizzle:** No initialisation step exists. Query builders and relation definitions are used directly. This is **DEVIATION:INTENTIONAL**: Go does not need a runtime registry because type safety is enforced at compile time through generated types and the builder API.

The features enabled by `drizzle()` are handled as follows in Grizzle:
- Relational query API → not yet implemented (DEVIATION:GAP — see [relations.md](./relations.md))
- Query logger → not yet implemented (DEVIATION:GAP, not designed)
- Custom type serializers → handled via the `db` struct tag and `.Type("T")` column modifier
- Query caching → not planned
