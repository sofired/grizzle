# Preloading

Grizzle avoids N+1 queries with an explicit, composable batch-loading pattern. There's no magic — you control exactly when and what is loaded.

## The N+1 problem

The naive approach loads related rows in a loop:

```go
// ❌ N+1: one query per user
for _, user := range users {
    realm, _ := loadRealm(ctx, user.RealmID) // N extra queries
}
```

## The batch approach

Load everything in two queries, then join in Go:

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

## Utility functions

### Pluck

Extracts a single field from every element of a slice:

```go
ids  := query.Pluck(users, func(u UserSelect) uuid.UUID { return u.ID })
names := query.Pluck(users, func(u UserSelect) string   { return u.Username })
```

### UniqueUUIDs / UniqueStrings

Deduplicates a slice while preserving first-seen order:

```go
// Avoids redundant WHERE IN entries when multiple users share a realm
realmIDs := query.UniqueUUIDs(
    query.Pluck(users, func(u UserSelect) uuid.UUID { return u.RealmID }),
)

slugs := query.UniqueStrings(query.Pluck(tags, func(t Tag) string { return t.Slug }))
```

### PreloadUUIDs / PreloadStrings

Adds a `WHERE col IN (ids...)` clause to an existing `SelectBuilder`. Returns `WHERE FALSE` when the slice is empty (avoiding a syntax error from an empty IN list):

```go
q := query.PreloadUUIDs(
    query.Select().From(db.RealmsT),
    db.RealmsT.ID,
    realmIDs, // safe even when empty
)
```

### Index

Builds a `map[K]T` from a key function. Use for BelongsTo / HasOne relations (each key maps to at most one row):

```go
realmByID := query.Index(realms, func(r RealmSelect) uuid.UUID { return r.ID })
realm     := realmByID[user.RealmID]
```

### GroupBy

Builds a `map[K][]T` from a key function. Use for HasMany relations:

```go
postsByAuthor := query.GroupBy(posts, func(p PostSelect) uuid.UUID { return p.AuthorID })
userPosts     := postsByAuthor[user.ID] // []PostSelect
```

### First

Returns a pointer to the first element, or nil if the slice is empty. Useful for HasOne results:

```go
profile := query.First(profilesByUser[user.ID]) // *ProfileSelect or nil
```

## HasMany example

```go
// Load posts per user in two queries
users, _ := pgxdb.FromSelect[db.UserSelect](ctx, d, query.Select().From(db.UsersT))

userIDs := query.UniqueUUIDs(query.Pluck(users, func(u db.UserSelect) uuid.UUID { return u.ID }))
posts, _ := pgxdb.FromSelect[db.PostSelect](ctx, d,
    query.PreloadUUIDs(query.Select().From(db.PostsT), db.PostsT.AuthorID, userIDs),
)

postsByAuthor := query.GroupBy(posts, func(p db.PostSelect) uuid.UUID { return p.AuthorID })

for _, user := range users {
    fmt.Printf("%s has %d posts\n", user.Username, len(postsByAuthor[user.ID]))
}
```
