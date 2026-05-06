# Relations Specification

Grizzle's relation system is the Go equivalent of [Drizzle's relations API](https://orm.drizzle.team/docs/relations).

## Design principle

Drizzle's relations are declared separately from the schema and wired to the `drizzle()` instance, enabling a high-level relational query API (`db.query.users.findMany({ with: { posts: true } })`). Grizzle provides relation definitions for JOIN construction but handles the "load related rows" pattern explicitly rather than through a magic `with` clause — until the relational query API is implemented.

## Relation definitions

**Drizzle:**
```typescript
export const usersRelations = relations(users, ({ one, many }) => ({
  realm: one(realms, { fields: [users.realmId], references: [realms.id] }),
  posts: many(posts),
}))
```

**Grizzle:**
```go
var UserRealm  = query.BelongsTo("realm", db.RealmsT, db.RealmsT.ID.EQCol(db.UsersT.RealmID))
var RealmUsers = query.HasMany("users",   db.UsersT,  db.UsersT.RealmID.EQCol(db.RealmsT.ID))
var UserProfile = query.HasOne("profile", db.ProfilesT, db.ProfilesT.UserID.EQCol(db.UsersT.ID))
```

### Relation kinds

| Drizzle | Grizzle | Status |
|---|---|---|
| `one(tbl, {fields, references})` | `query.BelongsTo(name, tbl, onExpr)` | Semantic parity; API is DEVIATION:LANGUAGE because Go carries the ON expression explicitly |
| `many(tbl)` | `query.HasMany(name, tbl, onExpr)` | Semantic parity; API is DEVIATION:LANGUAGE because Go carries the ON expression explicitly |
| `one` (HasOne direction) | `query.HasOne(name, tbl, onExpr)` | Semantic parity; API is DEVIATION:LANGUAGE because Go carries the ON expression explicitly |
| Self-referential relations | DEVIATION:GAP (not designed) | — |
| Many-to-many via junction table | DEVIATION:GAP (not designed) | — |

## Using relations in queries

### JOIN via relation

**Drizzle (core API):**
```typescript
db.select().from(users).leftJoin(realms, eq(users.realmId, realms.id))
```

**Grizzle:**
```go
query.Select(db.UsersT.ID, db.RealmsT.Name).From(db.UsersT).JoinRel(db.UserRealm)        // LEFT JOIN
query.Select(db.UsersT.ID, db.RealmsT.Name).From(db.UsersT).InnerJoinRel(db.UserRealm)   // INNER JOIN
```

**Status:** GRIZZLE-ONLY convenience helpers over PARITY join semantics.

Relation definitions themselves target Drizzle parity. `JoinRel` and `InnerJoinRel` are Go-specific shorthand for applying a relation's already-declared `ON` expression to the parity `LEFT JOIN` / `INNER JOIN` builders.

### Relational query API (`findMany` / `findFirst`) — DEVIATION:GAP (not designed)

**Drizzle:**
```typescript
const result = await db.query.users.findMany({
  with: { realm: true, posts: true },
  where: isNull(users.deletedAt),
  limit: 20,
})
// result[0].realm and result[0].posts are populated automatically
```

Drizzle's relational API automatically issues efficient SQL (or multiple queries with batch loading) and returns nested JavaScript objects with related rows attached.

**Grizzle:** No direct equivalent. Users compose the equivalent manually — see the preloading section below.

The relational query API is a significant usability gap. It should be designed and implemented after the Kit workflow is stabilized. The target API must:
- Accept a relation graph (`With` option specifying which relations to load)
- Issue the minimum number of queries (batch loading, not N+1)
- Return typed nested structs (via generics or code generation)
- Support `where`, `orderBy`, `limit`, `offset` on both root and nested queries

## Manual batch-loading (current approach)

Until the relational query API is implemented, users load related rows explicitly in two queries. This is a Grizzle-only interim pattern that avoids N+1 queries while preserving the same batched-loading goal as Drizzle's relational query API, but it requires more user code.

**Full example — BelongsTo (user → realm):**

```go
// Step 1: load primary rows
users, err := pgxdb.FromSelect[db.UserSelect](ctx, d,
    query.Select().From(db.UsersT).Where(db.UsersT.DeletedAt.IsNull()),
)

// Step 2: collect unique FK values
realmIDs := query.UniqueUUIDs(
    query.Pluck(users, func(u db.UserSelect) uuid.UUID { return u.RealmID }),
)

// Step 3: batch-load related rows (one query, not len(users) queries)
realms, err := pgxdb.FromSelect[db.RealmSelect](ctx, d,
    query.PreloadUUIDs(query.Select().From(db.RealmsT), db.RealmsT.ID, realmIDs),
)

// Step 4: index by PK for O(1) lookup
realmByID := query.Index(realms, func(r db.RealmSelect) uuid.UUID { return r.ID })

// Use
for _, user := range users {
    realm := realmByID[user.RealmID]
    fmt.Printf("%s is in realm %s\n", user.Username, realm.Name)
}
```

**Full example — HasMany (realm → users):**

```go
realms, err := pgxdb.FromSelect[db.RealmSelect](ctx, d, query.Select().From(db.RealmsT))

realmIDs := query.UniqueUUIDs(
    query.Pluck(realms, func(r db.RealmSelect) uuid.UUID { return r.ID }),
)
users, err := pgxdb.FromSelect[db.UserSelect](ctx, d,
    query.PreloadUUIDs(query.Select().From(db.UsersT), db.UsersT.RealmID, realmIDs),
)

usersByRealm := query.GroupBy(users, func(u db.UserSelect) uuid.UUID { return u.RealmID })

for _, realm := range realms {
    fmt.Printf("%s has %d users\n", realm.Name, len(usersByRealm[realm.ID]))
}
```

## Preloading utilities (GRIZZLE-ONLY)

These helpers in the `query` package have no Drizzle equivalent — Drizzle handles this automatically in its relational API. They are retained as the low-level building blocks that the higher-level relational query API will use internally.

| Function | Purpose |
|---|---|
| `query.Pluck(slice, fn)` | Extract a single field from every element of a slice |
| `query.UniqueUUIDs(ids)` | Deduplicate a UUID slice, preserving first-seen order |
| `query.UniqueStrings(strs)` | Deduplicate a string slice, preserving first-seen order |
| `query.PreloadUUIDs(q, col, ids)` | Adds `WHERE col IN (ids...)` to a query; emits `WHERE FALSE` for empty input |
| `query.PreloadStrings(q, col, strs)` | Same as above for string keys |
| `query.Index(slice, keyFn)` | Builds `map[K]T` — use for BelongsTo / HasOne |
| `query.GroupBy(slice, keyFn)` | Builds `map[K][]T` — use for HasMany |
| `query.First(slice)` | Returns pointer to first element, or nil if empty |
