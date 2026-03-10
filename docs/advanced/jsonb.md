# JSONB (PostgreSQL)

`JSONBColumn[T]` is the column handle for PostgreSQL `JSONB` / `JSON` columns. The type parameter `T` is the Go type the value will be scanned into — it doesn't affect SQL generation, but it makes generated code self-documenting.

## Defining a JSONB column

```go
// In your schema file
var UsersT = pg.Table("users",
    // ...
    pg.JSONB("attributes"),  // → JSONBColumn[map[string]any]
)
```

## Navigation operators

### Arrow (`->`) — object field as JSONB

```go
db.UsersT.Attributes.Arrow("role")
// "users"."attributes" -> $1
```

### ArrowText (`->>`) — object field as text

```go
db.UsersT.Attributes.ArrowText("role")
// "users"."attributes" ->> $1
```

### Path (`#>`) — nested value as JSONB

```go
db.UsersT.Attributes.Path("address", "city")
// "users"."attributes" #> ARRAY['address', 'city']
```

### PathText (`#>>`) — nested value as text

```go
db.UsersT.Attributes.PathText("address", "city")
// "users"."attributes" #>> ARRAY['address', 'city']
```

## Containment operators

### Contains (`@>`)

True when the column value contains the given JSON fragment:

```go
db.UsersT.Attributes.Contains(map[string]any{"role": "admin"})
// "users"."attributes" @> $1
```

### ContainedBy (`<@`)

True when the column value is contained within the given JSON fragment:

```go
db.UsersT.Attributes.ContainedBy(map[string]any{"role": "admin", "active": true})
// $1 @> "users"."attributes"
```

## Key existence operators

### HasKey (`?`)

```go
db.UsersT.Attributes.HasKey("role")
// "users"."attributes" ? $1
```

### HasKeyNot (NOT `?`)

```go
db.UsersT.Attributes.HasKeyNot("suspended_until")
// NOT "users"."attributes" ? $1
```

### HasAnyKey (`?|`)

True when the object has at least one of the given keys:

```go
db.UsersT.Attributes.HasAnyKey("role", "permissions")
// "users"."attributes" ?| $1
```

### HasAllKeys (`?&`)

True when the object has all of the given keys:

```go
db.UsersT.Attributes.HasAllKeys("role", "email_verified")
// "users"."attributes" ?& $1
```

## Using JSONB in SELECT

Navigation expressions return `expr.Expression`, which can be used in SELECT via `expr.Raw` or aliased for scanning:

```go
type UserAttrs struct {
    ID   uuid.UUID `db:"id"`
    Role string    `db:"role"`
}

rows, err := d.Query(ctx,
    query.Select(
        db.UsersT.ID,
        // Extract a text field and alias it for scanning
        expr.RawSelectAs(
            `"users"."attributes" ->> 'role'`,
            "role",
        ),
    ).From(db.UsersT),
)
```

::: tip
For simple filtering on known JSON fields, the containment operators (`Contains`, `HasKey`) are usually the clearest. For reading JSON values back into structured types, consider storing the whole JSONB column and scanning into a Go struct with `encoding/json`.
:::

## WHERE with JSONB

```go
// Users with role = admin (containment check)
query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(db.UsersT.Attributes.Contains(map[string]any{"role": "admin"}))

// Users that have a "suspended_until" key
query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(db.UsersT.Attributes.HasKey("suspended_until"))

// Users missing the "onboarded" key
query.Select(db.UsersT.ID, db.UsersT.Username).
    From(db.UsersT).
    Where(db.UsersT.Attributes.HasKeyNot("onboarded"))
```
