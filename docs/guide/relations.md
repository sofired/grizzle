# Relations

Relations encode a JOIN condition once and let you reuse it across many queries.

## Defining relations

```go
// db/relations.go
package db

import "github.com/sofired/grizzle/query"

// BelongsTo — FK lives on the left table (Users → Realms)
var UserRealm = query.BelongsTo("realm",
    RealmsT,
    RealmsT.ID.EQCol(UsersT.RealmID),
)

// HasMany — FK lives on the right table (Realm → many Users)
var RealmUsers = query.HasMany("users",
    UsersT,
    UsersT.RealmID.EQCol(RealmsT.ID),
)

// HasOne — like HasMany, but expects at most one match
var UserProfile = query.HasOne("profile",
    ProfilesT,
    ProfilesT.UserID.EQCol(UsersT.ID),
)
```

## Using relations in queries

### LEFT JOIN (default)

```go
query.Select(db.UsersT.ID, db.UsersT.Username, db.RealmsT.Name).
    From(db.UsersT).
    JoinRel(db.UserRealm)
// SELECT ... FROM "users" LEFT JOIN "realms" ON "realms"."id" = "users"."realm_id"
```

### INNER JOIN

```go
query.Select(db.UsersT.ID, db.RealmsT.Name).
    From(db.UsersT).
    InnerJoinRel(db.UserRealm)
// SELECT ... FROM "users" INNER JOIN "realms" ON "realms"."id" = "users"."realm_id"
```

### Multiple joins

```go
query.Select(
    db.UsersT.ID,
    db.UsersT.Username,
    db.RealmsT.Name,
    db.ProfilesT.AvatarURL,
).
    From(db.UsersT).
    InnerJoinRel(db.UserRealm).
    LeftJoinRel(db.UserProfile)
```

## Relation kinds

| Kind | Constructor | FK location | Use case |
|---|---|---|---|
| `BelongsTo` | `query.BelongsTo(…)` | Left table | User → Realm |
| `HasMany` | `query.HasMany(…)` | Right table | Realm → Users |
| `HasOne` | `query.HasOne(…)` | Right table | User → Profile |

The `Kind` field is informational — it doesn't change how the JOIN is rendered. It's available for introspection or tooling.

## Manual JOIN (without RelationDef)

If you don't have a pre-defined relation, use the standard join methods with an explicit ON condition:

```go
query.Select().
    From(db.OrdersT).
    LeftJoin(db.UsersT, db.UsersT.ID.EQCol(db.OrdersT.UserID)).
    InnerJoin(db.TenantsT, db.TenantsT.ID.EQCol(db.OrdersT.TenantID))
```
