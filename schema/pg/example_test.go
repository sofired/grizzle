package pg_test

import (
	"fmt"

	pg "github.com/sofired/grizzle/schema/pg"
)

// ExampleTable demonstrates declaring a PostgreSQL table with typed columns,
// a foreign key reference, and a table-level index constraint.
// In production, pass the resulting *TableDef to grizzle gen to produce
// typed query helpers.
func ExampleTable() {
	posts := pg.Table("posts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("author_id", pg.UUID().NotNull().References("users", "id", pg.OnDelete(pg.FKActionCascade))),
		pg.C("title", pg.Varchar(255).NotNull()),
		pg.C("body", pg.Text().NotNull()),
		pg.C("published", pg.Boolean().NotNull().Default(false)),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.Index("posts_author_id_idx").On(t.Col("author_id")).Build(),
		}
	})

	fmt.Printf("table: %s, %d columns, %d constraints\n",
		posts.Name, len(posts.Columns), len(posts.Constraints))
	fmt.Println("first column PK:", posts.Columns[0].PrimaryKey)
	fmt.Println("FK table:", posts.Columns[1].References.Table)
	// Output:
	// table: posts, 5 columns, 1 constraints
	// first column PK: true
	// FK table: users
}

// ExampleConstraint_ToCreateIndexSQL demonstrates generating a CREATE INDEX statement
// from a constraint definition.
func ExampleConstraint_ToCreateIndexSQL() {
	c := pg.UniqueIndex("users_realm_username_idx").
		On("realm_id", "username").
		Where(pg.IsNull("deleted_at")).
		Build()
	fmt.Println(c.ToCreateIndexSQL("users"))
	// Output:
	// CREATE UNIQUE INDEX users_realm_username_idx ON users (realm_id, username) WHERE deleted_at IS NULL
}
