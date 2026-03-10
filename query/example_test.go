package query_test

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	ts "github.com/sofired/grizzle/internal/testschema"
	"github.com/sofired/grizzle/query"
)

// ExampleSelect demonstrates a basic SELECT with WHERE, ORDER BY, and LIMIT.
func ExampleSelect() {
	sql, _ := query.Select(ts.UsersT.ID, ts.UsersT.Username, ts.UsersT.Email).
		From(ts.UsersT).
		Where(ts.UsersT.DeletedAt.IsNull()).
		OrderBy(ts.UsersT.Username.Asc()).
		Limit(20).
		Build(dialect.Postgres)
	fmt.Println(sql)
	// Output:
	// SELECT "users"."id", "users"."username", "users"."email" FROM "users" WHERE "users"."deleted_at" IS NULL ORDER BY "users"."username" ASC LIMIT 20
}

// ExampleSelect_join demonstrates an INNER JOIN using a pre-declared RelationDef.
// JoinRel reuses the ON condition encoded in the relation — no repetition needed.
func ExampleSelect_join() {
	sql, _ := query.Select(ts.UsersT.ID, ts.UsersT.Username, ts.RealmsT.Name).
		From(ts.UsersT).
		InnerJoinRel(ts.UserRealm).
		Where(ts.RealmsT.Enabled.IsTrue()).
		Build(dialect.Postgres)
	fmt.Println(sql)
	// Output:
	// SELECT "users"."id", "users"."username", "realms"."name" FROM "users" INNER JOIN "realms" ON "realms"."id" = "users"."realm_id" WHERE "realms"."enabled" = $1
}

// ExampleSelect_aggregate demonstrates COUNT with GROUP BY, HAVING, and ORDER BY.
func ExampleSelect_aggregate() {
	sql, _ := query.Select(ts.UsersT.RealmID, expr.Count().As("cnt")).
		From(ts.UsersT).
		Where(ts.UsersT.DeletedAt.IsNull()).
		GroupBy(ts.UsersT.RealmID).
		Having(expr.Count().GT(0)).
		OrderBy(expr.Count().Desc()).
		Build(dialect.Postgres)
	fmt.Println(sql)
	// Output:
	// SELECT "users"."realm_id", COUNT(*) AS "cnt" FROM "users" WHERE "users"."deleted_at" IS NULL GROUP BY "users"."realm_id" HAVING COUNT(*) > $1 ORDER BY COUNT(*) DESC
}

// ExampleSelect_windowFunction demonstrates ROW_NUMBER() with PARTITION BY and ORDER BY.
func ExampleSelect_windowFunction() {
	sql, _ := query.Select(
		ts.UsersT.ID,
		ts.UsersT.Username,
		expr.RowNumber().
			PartitionBy(ts.UsersT.RealmID).
			OrderBy(ts.UsersT.Username.Asc()).
			As("rn"),
	).From(ts.UsersT).Build(dialect.Postgres)
	fmt.Println(sql)
	// Output:
	// SELECT "users"."id", "users"."username", ROW_NUMBER() OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."username" ASC) AS "rn" FROM "users"
}

// ExampleInsertInto demonstrates INSERT using a db-tagged struct.
// Fields tagged omitempty are omitted when nil or zero.
func ExampleInsertInto() {
	row := ts.UserInsert{
		RealmID:  uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Username: "alice",
		// Email, Enabled, Attributes are nil/omitempty — omitted from INSERT
	}
	sql, args := query.InsertInto(ts.UsersT).
		Values(row).
		Returning(ts.UsersT.ID).
		Build(dialect.Postgres)
	fmt.Println(sql)
	fmt.Println(len(args), "bound args")
	// Output:
	// INSERT INTO "users" ("realm_id", "username") VALUES ($1, $2) RETURNING "users"."id"
	// 2 bound args
}

// ExampleInsertInto_upsert demonstrates ON CONFLICT … DO UPDATE SET (PostgreSQL upsert).
func ExampleInsertInto_upsert() {
	row := ts.UserInsert{
		RealmID:  uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Username: "alice",
	}
	sql, _ := query.InsertInto(ts.UsersT).
		Values(row).
		OnConflict("realm_id", "username").
		DoUpdateSetExcluded("email", "enabled").
		Build(dialect.Postgres)
	fmt.Println(sql)
	// Output:
	// INSERT INTO "users" ("realm_id", "username") VALUES ($1, $2) ON CONFLICT ("realm_id", "username") DO UPDATE SET "email" = EXCLUDED."email", "enabled" = EXCLUDED."enabled"
}

// ExampleUpdate demonstrates UPDATE using SetStruct for partial updates.
// Only non-nil pointer fields in the struct are included in the SET clause.
func ExampleUpdate() {
	enabled := true
	sql, _ := query.Update(ts.UsersT).
		SetStruct(ts.UserUpdate{Enabled: &enabled}).
		Where(ts.UsersT.ID.EQ(uuid.MustParse("00000000-0000-0000-0000-000000000001"))).
		Returning(ts.UsersT.UpdatedAt).
		Build(dialect.Postgres)
	fmt.Println(sql)
	// Output:
	// UPDATE "users" SET "enabled" = $1 WHERE "users"."id" = $2 RETURNING "users"."updated_at"
}

// ExampleDeleteFrom demonstrates DELETE FROM with a WHERE clause.
func ExampleDeleteFrom() {
	sql, _ := query.DeleteFrom(ts.UsersT).
		Where(ts.UsersT.ID.EQ(uuid.MustParse("00000000-0000-0000-0000-000000000001"))).
		Build(dialect.Postgres)
	fmt.Println(sql)
	// Output:
	// DELETE FROM "users" WHERE "users"."id" = $1
}
