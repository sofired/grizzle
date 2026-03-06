package query

import (
	"github.com/google/uuid"
	"github.com/grizzle-orm/g-rizzle/expr"
)

// -------------------------------------------------------------------
// Eager loading — batch query helpers and result collectors
// -------------------------------------------------------------------
//
// G-rizzle's approach to N+1 prevention is explicit and composable:
//
//  1. Load primary rows with a regular SELECT.
//  2. Pluck FK values from those rows.
//  3. Batch-load related rows with a single WHERE fk IN (...) query.
//  4. Group related rows by FK for O(1) lookup in Go.
//
// Example (users with their realm):
//
//	// Step 1: primary query
//	users, _ := pgxdb.QueryAll[UserSelect](ctx, db,
//	    query.Select().From(UsersT))
//
//	// Step 2: collect FK values
//	realmIDs := query.Pluck(users, func(u UserSelect) uuid.UUID { return u.RealmID })
//
//	// Step 3: batch load related rows (one query, not len(users) queries)
//	realms, _ := pgxdb.QueryAll[RealmSelect](ctx, db,
//	    query.PreloadUUIDs(query.Select().From(RealmsT), RealmsT.ID, realmIDs))
//
//	// Step 4: index for fast lookup
//	realmByID := query.Index(realms, func(r RealmSelect) uuid.UUID { return r.ID })
//
//	// Use
//	for _, u := range users {
//	    realm := realmByID[u.RealmID]
//	    fmt.Printf("%s belongs to realm %s\n", u.Username, realm.Name)
//	}

// -------------------------------------------------------------------
// Batch query builders
// -------------------------------------------------------------------

// PreloadUUIDs returns a copy of q with a WHERE col IN (ids...) clause added.
// If ids is empty, it adds a WHERE FALSE clause so the query returns no rows
// (avoids a syntax error from an empty IN list).
//
//	query.PreloadUUIDs(query.Select().From(PostsT), PostsT.UserID, userIDs)
func PreloadUUIDs(q *SelectBuilder, col expr.UUIDColumn, ids []uuid.UUID) *SelectBuilder {
	if len(ids) == 0 {
		return q.Where(expr.Raw("FALSE"))
	}
	return q.Where(col.In(ids...))
}

// PreloadStrings returns a copy of q with a WHERE col IN (vals...) clause added.
//
//	query.PreloadStrings(query.Select().From(TagsT), TagsT.Slug, slugs)
func PreloadStrings(q *SelectBuilder, col expr.StringColumn, vals []string) *SelectBuilder {
	if len(vals) == 0 {
		return q.Where(expr.Raw("FALSE"))
	}
	return q.Where(col.In(vals...))
}

// -------------------------------------------------------------------
// Generic slice / map utilities
// -------------------------------------------------------------------

// Pluck transforms a slice into a slice of a single field, using fn to extract
// the field from each element.
//
//	ids := query.Pluck(users, func(u UserSelect) uuid.UUID { return u.ID })
func Pluck[T any, F any](items []T, fn func(T) F) []F {
	out := make([]F, len(items))
	for i, item := range items {
		out[i] = fn(item)
	}
	return out
}

// UniqueUUIDs returns a deduplicated slice of UUIDs preserving first-seen order.
// Useful before passing FK values to PreloadUUIDs to avoid redundant WHERE IN entries.
//
//	realmIDs := query.UniqueUUIDs(query.Pluck(users, func(u UserSelect) uuid.UUID { return u.RealmID }))
func UniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := ids[:0:len(ids)]
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

// UniqueStrings returns a deduplicated slice of strings preserving first-seen order.
func UniqueStrings(vals []string) []string {
	seen := make(map[string]struct{}, len(vals))
	out := vals[:0:len(vals)]
	for _, v := range vals {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// GroupBy groups a slice of items by a key function, returning a map from key
// to all items sharing that key. Preserves the relative order of items within
// each group. Used for HasMany relations.
//
//	postsByUser := query.GroupBy(posts, func(p PostSelect) uuid.UUID { return p.UserID })
//	userPosts   := postsByUser[userID]
func GroupBy[T any, K comparable](items []T, key func(T) K) map[K][]T {
	m := make(map[K][]T)
	for _, item := range items {
		k := key(item)
		m[k] = append(m[k], item)
	}
	return m
}

// Index builds a map from a key function to the first matching item. Use for
// HasOne / BelongsTo relations where each key maps to at most one row.
//
//	realmByID := query.Index(realms, func(r RealmSelect) uuid.UUID { return r.ID })
//	realm     := realmByID[user.RealmID]
func Index[T any, K comparable](items []T, key func(T) K) map[K]T {
	m := make(map[K]T, len(items))
	for _, item := range items {
		m[key(item)] = item
	}
	return m
}

// First returns a pointer to the first element of a slice, or nil if the slice
// is empty. Convenient for HasOne preload results where you expect 0 or 1 row.
//
//	profile := query.First(profilesByUser[userID])
func First[T any](items []T) *T {
	if len(items) == 0 {
		return nil
	}
	return &items[0]
}

