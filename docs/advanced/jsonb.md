# JSONB (PostgreSQL)

`JSONBColumn[T]` is the column handle for PostgreSQL `JSONB` columns. Plain `JSONColumn[T]` handles across supported dialects must not receive JSONB-only containment/existence/delete-path helpers by default. The type parameter `T` is the Go type the value will be scanned into — it doesn't affect SQL generation, but it makes generated code self-documenting.

**Status:** GRIZZLE-ONLY PostgreSQL typed convenience over JSONB SQL operators. Drizzle RC.1 exposes distinct JSON and JSONB column types plus generic `sql` composition, but not this exact Go helper surface. Builders must omit these helpers on non-PostgreSQL dialects or fail fast rather than silently changing JSON semantics.

JSON key and path helper arguments are bound data values, not SQL identifiers or raw trusted literals. `Arrow("role")`, `HasKey("role")`, and path helpers must call the build context's parameter binding path so user-provided keys cannot alter SQL structure.

## Defining a JSONB column

```go
// In your schema file
var Users = pg.Table("users",
    // ...
    pg.C("attributes", pg.JSONB()), // generated handle: JSONBColumn[map[string]any]
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
// "users"."attributes" #> $1
```

### PathText (`#>>`) — nested value as text

```go
db.UsersT.Attributes.PathText("address", "city")
// "users"."attributes" #>> $1
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
// "users"."attributes" <@ $1
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
// NOT ("users"."attributes" ? $1)
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

## Delete-path operator

```go
db.UsersT.Attributes.DeletePath("private", "token")
// "users"."attributes" #- $1
```

## Using JSONB in SELECT

Navigation helpers return selectable expressions. Containment and existence helpers return predicates. Raw fragments must be compile-time trusted strings; use `RawArgs` or normal builder predicates for dynamic values.

```go
type UserAttrs struct {
    ID   uuid.UUID `db:"id"`
    Role string    `db:"role"`
}

rows, err := d.Query(ctx,
    query.Select(
        db.UsersT.ID,
        // Extract a text field and alias it for scanning
        db.UsersT.Attributes.ArrowText("role").As("role"),
    ).From(db.UsersT),
)
attrs, err := pgxdb.ScanAll[UserAttrs](rows, err)
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
