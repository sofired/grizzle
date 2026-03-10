package query_test

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/sofired/grizzle/dialect"
	ts "github.com/sofired/grizzle/internal/testschema"
	"github.com/sofired/grizzle/query"
)

// ExamplePreloadUUIDs shows the batch-load helper that adds a WHERE col IN (...)
// clause to an existing SelectBuilder. When ids is empty, WHERE FALSE is emitted
// so the query returns no rows without a syntax error.
func ExamplePreloadUUIDs() {
	ids := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	}
	sql, _ := query.PreloadUUIDs(
		query.Select().From(ts.RealmsT),
		ts.RealmsT.ID,
		ids,
	).Build(dialect.Postgres)
	fmt.Println(sql)
	// Output:
	// SELECT * FROM "realms" WHERE "realms"."id" IN ($1, $2)
}

// ExamplePluck extracts a single field from every element of a slice.
func ExamplePluck() {
	type item struct {
		ID   int
		Name string
	}
	items := []item{{1, "alice"}, {2, "bob"}, {3, "carol"}}
	names := query.Pluck(items, func(i item) string { return i.Name })
	fmt.Println(names)
	// Output:
	// [alice bob carol]
}

// ExampleIndex builds a map from a key function to the first matching element.
// Use this for BelongsTo / HasOne relations where each key maps to at most one row.
func ExampleIndex() {
	type user struct {
		ID   int
		Name string
	}
	users := []user{{1, "alice"}, {2, "bob"}}
	byID := query.Index(users, func(u user) int { return u.ID })
	fmt.Println(byID[1].Name)
	fmt.Println(byID[2].Name)
	// Output:
	// alice
	// bob
}

// ExampleGroupBy groups a slice by a key function, producing a map from key to
// all elements sharing that key. Use this for HasMany relations.
func ExampleGroupBy() {
	type post struct {
		AuthorID int
		Title    string
	}
	posts := []post{
		{1, "First"},
		{2, "Second"},
		{1, "Third"},
	}
	byAuthor := query.GroupBy(posts, func(p post) int { return p.AuthorID })
	fmt.Println(len(byAuthor[1]), "posts by author 1")
	fmt.Println(len(byAuthor[2]), "posts by author 2")
	// Output:
	// 2 posts by author 1
	// 1 posts by author 2
}

// ExampleFirst returns a pointer to the first element of a slice, or nil if empty.
// Convenient for HasOne preload results where 0 or 1 rows are expected.
func ExampleFirst() {
	items := []string{"a", "b", "c"}
	fmt.Println(*query.First(items))

	empty := []string{}
	fmt.Println(query.First(empty))
	// Output:
	// a
	// <nil>
}

// ExampleUniqueStrings deduplicates a string slice while preserving first-seen order.
func ExampleUniqueStrings() {
	slugs := []string{"go", "rust", "go", "python", "rust"}
	fmt.Println(query.UniqueStrings(slugs))
	// Output:
	// [go rust python]
}
