# Schema DSL

Grizzle schemas are plain Go code in the `schema/pg` package. A schema file defines tables that are passed to `grizzle gen` for code generation and to the [migration kit](/kit/overview) for DDL diffing.

## Declaring a table

```go
import pg "github.com/sofired/grizzle/schema/pg"

var Users = pg.Table("users",
    pg.C("id",         pg.UUID().PrimaryKey().DefaultRandom()),
    pg.C("username",   pg.Varchar(255).NotNull()),
    pg.C("email",      pg.Varchar(255)),
    pg.C("enabled",    pg.Boolean().NotNull().Default(true)),
    pg.C("created_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
    pg.C("deleted_at", pg.Timestamp().WithTimezone()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
    return []pg.Constraint{
        pg.UniqueIndex("users_username_idx").On(t.Col("username")).Build(),
    }
})
```

`pg.C(name, builder)` pairs a column name with its builder. Column order is preserved.

`.WithConstraints(fn)` receives a `TableRef` for column name resolution and returns a slice of constraints. Use `.Build()` if no constraints are needed.

## Column types

### UUID

```go
pg.UUID().PrimaryKey().DefaultRandom()          // uuid PRIMARY KEY DEFAULT gen_random_uuid()
pg.UUID().NotNull().References("realms", "id",  // uuid NOT NULL REFERENCES realms(id)
    pg.OnDelete(pg.FKActionRestrict))
pg.UUID().NotNull()                             // uuid NOT NULL
```

### Text / Varchar

```go
pg.Varchar(255).NotNull()                       // varchar(255) NOT NULL
pg.Varchar(255).NotNull().Unique()              // varchar(255) NOT NULL UNIQUE
pg.Text().NotNull()                             // text NOT NULL
pg.Text().Default("draft")                      // text DEFAULT 'draft'
```

### Boolean

```go
pg.Boolean().NotNull().Default(true)            // boolean NOT NULL DEFAULT true
pg.Boolean().NotNull().Default(false)           // boolean NOT NULL DEFAULT false
```

### Integer / BigInt

```go
pg.Integer().NotNull().Default(0)               // integer NOT NULL DEFAULT 0
pg.BigInt().NotNull()                           // bigint NOT NULL
pg.Serial()                                     // serial (auto-increment)
pg.BigSerial()                                  // bigserial (auto-increment 8-byte)
```

### Timestamp

```go
pg.Timestamp().WithTimezone().NotNull().DefaultNow()   // timestamptz NOT NULL DEFAULT now()
pg.Timestamp().WithTimezone()                          // timestamptz (nullable)
pg.Timestamp().WithTimezone().OnUpdate()               // marks as app-managed updated_at
```

::: tip OnUpdate
Go has no runtime hook equivalent to Drizzle's `$onUpdate`. The `.OnUpdate()` marker tells `grizzle gen` to emit a comment reminding you to set this column on every UPDATE, but it doesn't enforce it automatically.
:::

### JSONB

```go
pg.JSONB().NotNull().DefaultEmpty()             // jsonb NOT NULL DEFAULT '{}'::jsonb
pg.JSONB().DefaultEmptyArray()                  // jsonb DEFAULT '[]'::jsonb
pg.JSONB().Type("map[string]any")               // jsonb — sets Go scan type in generated code
pg.JSON()                                       // json (plain, not binary)
```

### Numeric

```go
pg.Numeric(10, 2).NotNull()                     // numeric(10,2) NOT NULL
```

## Foreign keys

Inline foreign keys are the most common pattern:

```go
pg.C("realm_id", pg.UUID().NotNull().References("realms", "id",
    pg.OnDelete(pg.FKActionRestrict),
    pg.OnUpdate(pg.FKActionNoAction),
))
```

Available FK actions: `FKActionNoAction`, `FKActionRestrict`, `FKActionCascade`, `FKActionSetNull`, `FKActionSetDefault`.

For composite FKs, use a table-level constraint (see below).

## Table-level constraints

Constraints are declared in the `.WithConstraints` callback, which receives a `TableRef` for column name resolution.

### Unique index

```go
pg.UniqueIndex("users_realm_username_idx").
    On(t.Col("realm_id"), t.Col("username")).
    Where(pg.IsNull(t.Col("deleted_at"))).  // partial index
    Build()
```

### Non-unique index

```go
pg.Index("users_realm_id_idx").On(t.Col("realm_id")).Build()
```

### CHECK constraint

```go
pg.Check("price_positive", "price > 0")
```

### Composite primary key

```go
pg.CompositePrimaryKey("user_id", "role_id")
```

### Composite foreign key

```go
pg.ForeignKey("fk_order_items_order").
    From("order_id", "tenant_id").
    References("orders", "id", "tenant_id").
    OnDelete(pg.FKActionCascade).
    Build()
```

### Named UNIQUE constraint (not an index)

```go
pg.UniqueConstraint("users_email_unique", "email")
```

## Schema namespaces

For tables outside the `public` schema:

```go
var AuditLogs = pg.SchemaTable("audit", "logs",
    pg.C("id",         pg.BigSerial()),
    pg.C("table_name", pg.Text().NotNull()),
    pg.C("action",     pg.Text().NotNull()),
    pg.C("occurred_at",pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
).Build()

// DDL: CREATE TABLE audit.logs (...)
```
