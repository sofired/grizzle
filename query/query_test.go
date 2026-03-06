package query_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/grizzle-orm/g-rizzle/dialect"
	"github.com/grizzle-orm/g-rizzle/expr"
	ts "github.com/grizzle-orm/g-rizzle/internal/testschema"
	"github.com/grizzle-orm/g-rizzle/query"
)

// assertSQL is a small helper that builds a query and compares the output.
func assertSQL(t *testing.T, name string, b interface{ Build(dialect.Dialect) (string, []any) }, wantSQL string, wantArgs []any) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		gotSQL, gotArgs := b.Build(dialect.Postgres)
		if gotSQL != wantSQL {
			t.Errorf("SQL mismatch\n got:  %s\nwant: %s", gotSQL, wantSQL)
		}
		if len(gotArgs) != len(wantArgs) {
			t.Errorf("args length mismatch: got %d, want %d\n got:  %v\nwant: %v", len(gotArgs), len(wantArgs), gotArgs, wantArgs)
			return
		}
		for i := range wantArgs {
			if fmt.Sprintf("%v", gotArgs[i]) != fmt.Sprintf("%v", wantArgs[i]) {
				t.Errorf("arg[%d] mismatch: got %v (%T), want %v (%T)", i, gotArgs[i], gotArgs[i], wantArgs[i], wantArgs[i])
			}
		}
	})
}

// -------------------------------------------------------------------
// SELECT tests
// -------------------------------------------------------------------

func TestSelect_StarFromTable(t *testing.T) {
	assertSQL(t, "select star",
		query.Select().From(ts.UsersT),
		`SELECT * FROM "users"`,
		nil,
	)
}

func TestSelect_SpecificColumns(t *testing.T) {
	assertSQL(t, "select specific cols",
		query.Select(ts.UsersT.ID, ts.UsersT.Username, ts.UsersT.Email).
			From(ts.UsersT),
		`SELECT "users"."id", "users"."username", "users"."email" FROM "users"`,
		nil,
	)
}

func TestSelect_WhereEQ(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	assertSQL(t, "where uuid eq",
		query.Select(ts.UsersT.ID, ts.UsersT.Username).
			From(ts.UsersT).
			Where(ts.UsersT.ID.EQ(id)),
		`SELECT "users"."id", "users"."username" FROM "users" WHERE "users"."id" = $1`,
		[]any{id},
	)
}

func TestSelect_WhereIsNull(t *testing.T) {
	assertSQL(t, "where is null",
		query.Select().
			From(ts.UsersT).
			Where(ts.UsersT.DeletedAt.IsNull()),
		`SELECT * FROM "users" WHERE "users"."deleted_at" IS NULL`,
		nil,
	)
}

func TestSelect_WhereIsNotNull(t *testing.T) {
	assertSQL(t, "where is not null",
		query.Select().
			From(ts.UsersT).
			Where(ts.UsersT.DeletedAt.IsNotNull()),
		`SELECT * FROM "users" WHERE "users"."deleted_at" IS NOT NULL`,
		nil,
	)
}

func TestSelect_WhereAnd(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	assertSQL(t, "where and",
		query.Select(ts.UsersT.ID).
			From(ts.UsersT).
			Where(expr.And(
				ts.UsersT.RealmID.EQ(realmID),
				ts.UsersT.DeletedAt.IsNull(),
				ts.UsersT.Enabled.IsTrue(),
			)),
		`SELECT "users"."id" FROM "users" WHERE ("users"."realm_id" = $1 AND "users"."deleted_at" IS NULL AND "users"."enabled" = $2)`,
		[]any{realmID, true},
	)
}

func TestSelect_WhereOr(t *testing.T) {
	assertSQL(t, "where or",
		query.Select(ts.UsersT.ID).
			From(ts.UsersT).
			Where(expr.Or(
				ts.UsersT.Username.ILike("%alice%"),
				ts.UsersT.Email.ILike("%alice%"),
			)),
		`SELECT "users"."id" FROM "users" WHERE ("users"."username" ILIKE $1 OR "users"."email" ILIKE $2)`,
		[]any{"%alice%", "%alice%"},
	)
}

func TestSelect_WhereNilDropped(t *testing.T) {
	// nil conditions inside And() must be silently dropped
	assertSQL(t, "nil conditions dropped",
		query.Select().
			From(ts.UsersT).
			Where(expr.And(
				ts.UsersT.DeletedAt.IsNull(),
				nil,
				nil,
			)),
		`SELECT * FROM "users" WHERE "users"."deleted_at" IS NULL`,
		nil,
	)
}

func TestSelect_WhereNilAndReturnsNil(t *testing.T) {
	// And() with only nils should produce nil, which means no WHERE clause
	q := query.Select().From(ts.UsersT).Where(expr.And(nil, nil))
	sql, _ := q.Build(dialect.Postgres)
	want := `SELECT * FROM "users"`
	if sql != want {
		t.Errorf("SQL mismatch\n got:  %s\nwant: %s", sql, want)
	}
}

func TestSelect_OrderBy(t *testing.T) {
	assertSQL(t, "order by asc desc",
		query.Select().
			From(ts.UsersT).
			OrderBy(ts.UsersT.Username.Asc(), ts.UsersT.CreatedAt.Desc()),
		`SELECT * FROM "users" ORDER BY "users"."username" ASC, "users"."created_at" DESC`,
		nil,
	)
}

func TestSelect_LimitOffset(t *testing.T) {
	assertSQL(t, "limit offset",
		query.Select().
			From(ts.UsersT).
			Where(ts.UsersT.DeletedAt.IsNull()).
			Limit(20).
			Offset(40),
		`SELECT * FROM "users" WHERE "users"."deleted_at" IS NULL LIMIT 20 OFFSET 40`,
		nil,
	)
}

func TestSelect_LeftJoin(t *testing.T) {
	assertSQL(t, "left join",
		query.Select(ts.UsersT.ID, ts.UsersT.Username, ts.RealmsT.Name).
			From(ts.UsersT).
			LeftJoin(ts.RealmsT, ts.UsersT.RealmID.EQCol(ts.RealmsT.ID)),
		`SELECT "users"."id", "users"."username", "realms"."name" FROM "users" LEFT JOIN "realms" ON "users"."realm_id" = "realms"."id"`,
		nil,
	)
}

func TestSelect_In(t *testing.T) {
	ids := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	}
	assertSQL(t, "uuid in",
		query.Select().From(ts.UsersT).Where(ts.UsersT.ID.In(ids...)),
		`SELECT * FROM "users" WHERE "users"."id" IN ($1, $2)`,
		[]any{ids[0], ids[1]},
	)
}

func TestSelect_StringIn(t *testing.T) {
	assertSQL(t, "string in",
		query.Select().From(ts.UsersT).Where(ts.UsersT.Username.In("alice", "bob", "carol")),
		`SELECT * FROM "users" WHERE "users"."username" IN ($1, $2, $3)`,
		[]any{"alice", "bob", "carol"},
	)
}

func TestSelect_TimestampLT(t *testing.T) {
	cutoff := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	assertSQL(t, "timestamp lt",
		query.Select(ts.UsersT.ID).
			From(ts.UsersT).
			Where(expr.And(
				ts.UsersT.DeletedAt.IsNotNull(),
				ts.UsersT.PurgedAt.IsNull(),
				ts.UsersT.DeletedAt.LT(cutoff),
			)),
		`SELECT "users"."id" FROM "users" WHERE ("users"."deleted_at" IS NOT NULL AND "users"."purged_at" IS NULL AND "users"."deleted_at" < $1)`,
		[]any{cutoff},
	)
}

func TestSelect_DynamicSearch(t *testing.T) {
	// Simulates the dynamic WHERE pattern from the dynamic search discussion.
	// Only non-nil params contribute conditions.
	type SearchParams struct {
		RealmID  *uuid.UUID
		Username *string
		Enabled  *bool
	}

	buildQuery := func(p SearchParams) *query.SelectBuilder {
		return query.Select(ts.UsersT.ID, ts.UsersT.Username, ts.UsersT.Email).
			From(ts.UsersT).
			Where(expr.And(
				ts.UsersT.DeletedAt.IsNull(),
				whenUUID(p.RealmID, func(v uuid.UUID) expr.Expression { return ts.UsersT.RealmID.EQ(v) }),
				whenString(p.Username, func(v string) expr.Expression { return ts.UsersT.Username.ILike("%" + v + "%") }),
				whenBool(p.Enabled, func(v bool) expr.Expression { return ts.UsersT.Enabled.EQ(v) }),
			))
	}

	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	t.Run("all params nil → only base condition", func(t *testing.T) {
		sql, args := buildQuery(SearchParams{}).Build(dialect.Postgres)
		want := `SELECT "users"."id", "users"."username", "users"."email" FROM "users" WHERE "users"."deleted_at" IS NULL`
		if sql != want {
			t.Errorf("SQL mismatch\n got:  %s\nwant: %s", sql, want)
		}
		if len(args) != 0 {
			t.Errorf("expected no args, got %v", args)
		}
	})

	t.Run("realm + username filter", func(t *testing.T) {
		name := "alice"
		sql, args := buildQuery(SearchParams{RealmID: &realmID, Username: &name}).Build(dialect.Postgres)
		wantSQL := `SELECT "users"."id", "users"."username", "users"."email" FROM "users" WHERE ("users"."deleted_at" IS NULL AND "users"."realm_id" = $1 AND "users"."username" ILIKE $2)`
		if sql != wantSQL {
			t.Errorf("SQL mismatch\n got:  %s\nwant: %s", sql, wantSQL)
		}
		if len(args) != 2 || args[0] != realmID || args[1] != "%alice%" {
			t.Errorf("args mismatch: %v", args)
		}
	})
}

// -------------------------------------------------------------------
// INSERT tests
// -------------------------------------------------------------------

func TestInsert_Struct(t *testing.T) {
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	assertSQL(t, "insert struct",
		query.InsertInto(ts.RealmsT).Values(row),
		`INSERT INTO "realms" ("name") VALUES ($1)`,
		[]any{name},
	)
}

func TestInsert_WithReturning(t *testing.T) {
	name := "test-realm"
	enabled := true
	row := ts.RealmInsert{Name: name, Enabled: &enabled}
	assertSQL(t, "insert with returning",
		query.InsertInto(ts.RealmsT).
			Values(row).
			Returning(ts.RealmsT.ID, ts.RealmsT.CreatedAt),
		`INSERT INTO "realms" ("name", "enabled") VALUES ($1, $2) RETURNING "realms"."id", "realms"."created_at"`,
		[]any{name, enabled},
	)
}

// -------------------------------------------------------------------
// Upsert tests
// -------------------------------------------------------------------

func TestUpsert_DoNothing(t *testing.T) {
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	assertSQL(t, "on conflict do nothing",
		query.InsertInto(ts.RealmsT).
			Values(row).
			OnConflict("name").DoNothing(),
		`INSERT INTO "realms" ("name") VALUES ($1) ON CONFLICT ("name") DO NOTHING`,
		[]any{name},
	)
}

func TestUpsert_DoNothing_NoTarget(t *testing.T) {
	// No conflict target = ON CONFLICT DO NOTHING (blind upsert).
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	assertSQL(t, "on conflict do nothing no target",
		query.InsertInto(ts.RealmsT).
			Values(row).
			DoNothing(),
		`INSERT INTO "realms" ("name") VALUES ($1) ON CONFLICT DO NOTHING`,
		[]any{name},
	)
}

func TestUpsert_DoUpdateSetExcluded(t *testing.T) {
	name := "test-realm"
	enabled := true
	row := ts.RealmInsert{Name: name, Enabled: &enabled}
	assertSQL(t, "on conflict do update set excluded",
		query.InsertInto(ts.RealmsT).
			Values(row).
			OnConflict("name").
			DoUpdateSetExcluded("enabled"),
		`INSERT INTO "realms" ("name", "enabled") VALUES ($1, $2) ON CONFLICT ("name") DO UPDATE SET "enabled" = EXCLUDED."enabled"`,
		[]any{name, enabled},
	)
}

func TestUpsert_DoUpdateSetExplicit(t *testing.T) {
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	enabled := true
	assertSQL(t, "on conflict do update set explicit",
		query.InsertInto(ts.RealmsT).
			Values(row).
			OnConflict("name").
			DoUpdateSet("enabled", enabled),
		`INSERT INTO "realms" ("name") VALUES ($1) ON CONFLICT ("name") DO UPDATE SET "enabled" = $2`,
		[]any{name, enabled},
	)
}

func TestUpsert_DoUpdateSetMixed(t *testing.T) {
	// Both explicit and EXCLUDED columns.
	name := "test-realm"
	displayName := "Test Realm"
	row := ts.RealmInsert{Name: name}
	assertSQL(t, "on conflict do update set mixed",
		query.InsertInto(ts.RealmsT).
			Values(row).
			OnConflict("name").
			DoUpdateSet("display_name", displayName).
			DoUpdateSetExcluded("enabled"),
		`INSERT INTO "realms" ("name") VALUES ($1) ON CONFLICT ("name") DO UPDATE SET "display_name" = $2, "enabled" = EXCLUDED."enabled"`,
		[]any{name, displayName},
	)
}

func TestUpsert_OnConflictConstraint(t *testing.T) {
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	assertSQL(t, "on conflict on constraint",
		query.InsertInto(ts.RealmsT).
			Values(row).
			OnConflictConstraint("realms_name_idx").
			DoNothing(),
		`INSERT INTO "realms" ("name") VALUES ($1) ON CONFLICT ON CONSTRAINT "realms_name_idx" DO NOTHING`,
		[]any{name},
	)
}

func TestUpsert_MultiColConflictTarget(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	username := "alice"
	row := ts.UserInsert{RealmID: realmID, Username: username}
	assertSQL(t, "multi-col conflict target",
		query.InsertInto(ts.UsersT).
			Values(row).
			OnConflict("realm_id", "username").
			DoUpdateSetExcluded("email", "enabled"),
		`INSERT INTO "users" ("realm_id", "username") VALUES ($1, $2) ON CONFLICT ("realm_id", "username") DO UPDATE SET "email" = EXCLUDED."email", "enabled" = EXCLUDED."enabled"`,
		[]any{realmID, username},
	)
}

func TestUpsert_WithReturning(t *testing.T) {
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	assertSQL(t, "upsert with returning",
		query.InsertInto(ts.RealmsT).
			Values(row).
			OnConflict("name").
			DoUpdateSetExcluded("name").
			Returning(ts.RealmsT.ID),
		`INSERT INTO "realms" ("name") VALUES ($1) ON CONFLICT ("name") DO UPDATE SET "name" = EXCLUDED."name" RETURNING "realms"."id"`,
		[]any{name},
	)
}

func TestUpsert_DoUpdateSetStruct(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	username := "alice"
	newEmail := "alice@example.com"
	row := ts.UserInsert{RealmID: realmID, Username: username}
	upd := ts.UserUpdate{Email: &newEmail}
	assertSQL(t, "upsert do update set struct",
		query.InsertInto(ts.UsersT).
			Values(row).
			OnConflict("realm_id", "username").
			DoUpdateSetStruct(upd),
		`INSERT INTO "users" ("realm_id", "username") VALUES ($1, $2) ON CONFLICT ("realm_id", "username") DO UPDATE SET "email" = $3`,
		[]any{realmID, username, newEmail},
	)
}

// -------------------------------------------------------------------
// UPDATE tests
// -------------------------------------------------------------------

func TestUpdate_SetExplicit(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	assertSQL(t, "update set",
		query.Update(ts.UsersT).
			Set("username", "[deleted]").
			Set("enabled", false).
			Where(ts.UsersT.ID.EQ(id)),
		`UPDATE "users" SET "username" = $1, "enabled" = $2 WHERE "users"."id" = $3`,
		[]any{"[deleted]", false, id},
	)
}

func TestUpdate_SetStruct(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	now := time.Now().UTC().Truncate(time.Second)
	uname := "[deleted]"
	enabled := false

	assertSQL(t, "update set struct",
		query.Update(ts.UsersT).
			SetStruct(ts.UserUpdate{
				Username:  &uname,
				Enabled:   &enabled,
				DeletedAt: &now,
				UpdatedAt: &now,
			}).
			Where(expr.And(
				ts.UsersT.ID.EQ(id),
				ts.UsersT.DeletedAt.IsNotNull(),
				ts.UsersT.PurgedAt.IsNull(),
			)).
			Returning(ts.UsersT.ID),
		`UPDATE "users" SET "username" = $1, "enabled" = $2, "deleted_at" = $3, "updated_at" = $4 WHERE ("users"."id" = $5 AND "users"."deleted_at" IS NOT NULL AND "users"."purged_at" IS NULL) RETURNING "users"."id"`,
		[]any{uname, enabled, now, now, id},
	)
}

// -------------------------------------------------------------------
// DELETE tests
// -------------------------------------------------------------------

func TestDelete_WhereEQ(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	assertSQL(t, "delete where eq",
		query.DeleteFrom(ts.UsersT).
			Where(ts.UsersT.ID.EQ(id)),
		`DELETE FROM "users" WHERE "users"."id" = $1`,
		[]any{id},
	)
}

func TestDelete_WithReturning(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	assertSQL(t, "delete with returning",
		query.DeleteFrom(ts.UsersT).
			Where(ts.UsersT.RealmID.EQ(realmID)).
			Returning(ts.UsersT.ID),
		`DELETE FROM "users" WHERE "users"."realm_id" = $1 RETURNING "users"."id"`,
		[]any{realmID},
	)
}

// -------------------------------------------------------------------
// Schema DSL tests
// -------------------------------------------------------------------

func TestSchema_TableDef(t *testing.T) {
	t.Run("realms table has correct columns", func(t *testing.T) {
		if ts.Realms.Name != "realms" {
			t.Errorf("table name: got %q, want %q", ts.Realms.Name, "realms")
		}
		if len(ts.Realms.Columns) != 7 {
			t.Errorf("column count: got %d, want 7", len(ts.Realms.Columns))
		}
		colMap := ts.Realms.ColMap()
		if colMap["id"].SQLType != "uuid" {
			t.Errorf("id SQLType: got %q, want %q", colMap["id"].SQLType, "uuid")
		}
		if !colMap["id"].PrimaryKey {
			t.Error("id should be primary key")
		}
		if !colMap["name"].NotNull {
			t.Error("name should be NOT NULL")
		}
		if colMap["display_name"].NotNull {
			t.Error("display_name should be nullable")
		}
	})

	t.Run("realms table has correct constraints", func(t *testing.T) {
		if len(ts.Realms.Constraints) != 2 {
			t.Errorf("constraint count: got %d, want 2", len(ts.Realms.Constraints))
		}
		idx := ts.Realms.Constraints[0]
		if idx.Kind != "unique_index" {
			t.Errorf("first constraint kind: got %q, want %q", idx.Kind, "unique_index")
		}
		if idx.Name != "realms_name_idx" {
			t.Errorf("index name: got %q, want %q", idx.Name, "realms_name_idx")
		}
	})

	t.Run("users partial index has where clause", func(t *testing.T) {
		colMap := ts.Users.ColMap()
		if colMap["deleted_at"].NotNull {
			t.Error("deleted_at should be nullable")
		}
		// Find the partial unique index
		var found bool
		for _, c := range ts.Users.Constraints {
			if c.Name == "users_realm_username_idx" {
				found = true
				if c.WhereExpr != "deleted_at IS NULL" {
					t.Errorf("partial index WHERE: got %q, want %q", c.WhereExpr, "deleted_at IS NULL")
				}
			}
		}
		if !found {
			t.Error("users_realm_username_idx constraint not found")
		}
	})
}

// -------------------------------------------------------------------
// Relation tests
// -------------------------------------------------------------------

func TestRelation_BelongsTo_Fields(t *testing.T) {
	rel := ts.UserRealm
	if rel.Kind != query.RelBelongsTo {
		t.Errorf("kind: got %q, want %q", rel.Kind, query.RelBelongsTo)
	}
	if rel.Name != "realm" {
		t.Errorf("name: got %q, want %q", rel.Name, "realm")
	}
	if rel.Table.GRizTableName() != "realms" {
		t.Errorf("table: got %q, want %q", rel.Table.GRizTableName(), "realms")
	}
	if rel.On == nil {
		t.Error("On expression must not be nil")
	}
}

func TestRelation_HasMany_Fields(t *testing.T) {
	rel := ts.RealmUsers
	if rel.Kind != query.RelHasMany {
		t.Errorf("kind: got %q, want %q", rel.Kind, query.RelHasMany)
	}
	if rel.Name != "users" {
		t.Errorf("name: got %q, want %q", rel.Name, "users")
	}
	if rel.Table.GRizTableName() != "users" {
		t.Errorf("table: got %q, want %q", rel.Table.GRizTableName(), "users")
	}
}

func TestSelect_JoinRel_LeftJoin(t *testing.T) {
	// JoinRel(UserRealm) should produce the same SQL as LeftJoin(RealmsT, on).
	wantSQL := `SELECT "users"."id", "users"."username", "realms"."name" FROM "users" LEFT JOIN "realms" ON "realms"."id" = "users"."realm_id"`
	assertSQL(t, "JoinRel belongs_to",
		query.Select(ts.UsersT.ID, ts.UsersT.Username, ts.RealmsT.Name).
			From(ts.UsersT).
			JoinRel(ts.UserRealm),
		wantSQL,
		nil,
	)
}

func TestSelect_InnerJoinRel(t *testing.T) {
	wantSQL := `SELECT "users"."id", "users"."username", "realms"."name" FROM "users" INNER JOIN "realms" ON "realms"."id" = "users"."realm_id"`
	assertSQL(t, "InnerJoinRel belongs_to",
		query.Select(ts.UsersT.ID, ts.UsersT.Username, ts.RealmsT.Name).
			From(ts.UsersT).
			InnerJoinRel(ts.UserRealm),
		wantSQL,
		nil,
	)
}

func TestSelect_JoinRel_HasMany(t *testing.T) {
	// HasMany from realms → users (realm-centric query).
	wantSQL := `SELECT "realms"."id", "realms"."name", "users"."username" FROM "realms" LEFT JOIN "users" ON "users"."realm_id" = "realms"."id"`
	assertSQL(t, "JoinRel has_many",
		query.Select(ts.RealmsT.ID, ts.RealmsT.Name, ts.UsersT.Username).
			From(ts.RealmsT).
			JoinRel(ts.RealmUsers),
		wantSQL,
		nil,
	)
}

func TestSelect_JoinRel_WithWhere(t *testing.T) {
	// Combining JoinRel with a WHERE clause.
	wantSQL := `SELECT "users"."id", "users"."username" FROM "users" LEFT JOIN "realms" ON "realms"."id" = "users"."realm_id" WHERE "users"."enabled" = $1`
	assertSQL(t, "JoinRel with WHERE",
		query.Select(ts.UsersT.ID, ts.UsersT.Username).
			From(ts.UsersT).
			JoinRel(ts.UserRealm).
			Where(ts.UsersT.Enabled.IsTrue()),
		wantSQL,
		[]any{true},
	)
}

func TestSelect_MultipleJoinRels(t *testing.T) {
	// Chaining two JoinRel calls should produce two JOIN clauses.
	q := query.Select(ts.UsersT.ID).
		From(ts.UsersT).
		JoinRel(ts.UserRealm).
		JoinRel(ts.RealmUsers) // contrived but valid structurally
	sql, _ := q.Build(dialect.Postgres)
	if !containsN(sql, "LEFT JOIN", 2) {
		t.Errorf("expected 2 LEFT JOINs in SQL, got: %s", sql)
	}
}

// containsN reports whether substr appears exactly n times in s.
func containsN(s, substr string, n int) bool {
	count := 0
	for {
		idx := len(s) - len(s[len(substr)-1:])
		if idx < 0 {
			break
		}
		i := strings.Index(s, substr)
		if i < 0 {
			break
		}
		count++
		s = s[i+len(substr):]
	}
	return count == n
}

// -------------------------------------------------------------------
// MySQL dialect tests
// -------------------------------------------------------------------

func TestMySQL_Placeholder(t *testing.T) {
	// MySQL uses ? placeholders, not $1
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	q := query.Select().From(ts.UsersT).Where(ts.UsersT.ID.EQ(id))
	sql, args := q.Build(dialect.MySQL)
	if !strings.Contains(sql, "?") {
		t.Errorf("expected ? placeholder for MySQL, got: %s", sql)
	}
	if strings.Contains(sql, "$1") {
		t.Errorf("unexpected $1 placeholder for MySQL: %s", sql)
	}
	if len(args) != 1 {
		t.Errorf("expected 1 arg, got %d", len(args))
	}
}

func TestMySQL_QuoteIdent(t *testing.T) {
	q := query.Select().From(ts.UsersT)
	sql, _ := q.Build(dialect.MySQL)
	if !strings.Contains(sql, "`users`") {
		t.Errorf("expected backtick quoting for MySQL, got: %s", sql)
	}
}

func TestMySQL_NoReturning(t *testing.T) {
	// RETURNING should be silently dropped for MySQL
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	sql, _ := query.InsertInto(ts.RealmsT).
		Values(row).
		Returning(ts.RealmsT.ID).
		Build(dialect.MySQL)
	if strings.Contains(sql, "RETURNING") {
		t.Errorf("MySQL INSERT should not have RETURNING clause: %s", sql)
	}
}

func TestMySQL_UpdateNoReturning(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	sql, _ := query.Update(ts.UsersT).
		Set("username", "alice").
		Where(ts.UsersT.ID.EQ(id)).
		Returning(ts.UsersT.ID).
		Build(dialect.MySQL)
	if strings.Contains(sql, "RETURNING") {
		t.Errorf("MySQL UPDATE should not have RETURNING clause: %s", sql)
	}
}

func TestMySQL_DeleteNoReturning(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	sql, _ := query.DeleteFrom(ts.UsersT).
		Where(ts.UsersT.ID.EQ(id)).
		Returning(ts.UsersT.ID).
		Build(dialect.MySQL)
	if strings.Contains(sql, "RETURNING") {
		t.Errorf("MySQL DELETE should not have RETURNING clause: %s", sql)
	}
}

func TestMySQL_UpsertDuplicateKey(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	username := "alice"
	row := ts.UserInsert{RealmID: realmID, Username: username}
	sql, args := query.InsertInto(ts.UsersT).
		Values(row).
		OnConflict("realm_id", "username").
		DoUpdateSetExcluded("email", "enabled").
		Build(dialect.MySQL)
	if !strings.Contains(sql, "ON DUPLICATE KEY UPDATE") {
		t.Errorf("MySQL upsert should use ON DUPLICATE KEY UPDATE, got: %s", sql)
	}
	if strings.Contains(sql, "ON CONFLICT") {
		t.Errorf("MySQL should not have ON CONFLICT: %s", sql)
	}
	// VALUES(col) syntax for excluded columns
	if !strings.Contains(sql, "VALUES(`email`)") {
		t.Errorf("MySQL upsert should use VALUES(col) syntax, got: %s", sql)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d: %v", len(args), args)
	}
}

func TestMySQL_UpsertExplicitSet(t *testing.T) {
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	enabled := true
	sql, _ := query.InsertInto(ts.RealmsT).
		Values(row).
		OnConflict("name").
		DoUpdateSet("enabled", enabled).
		Build(dialect.MySQL)
	if !strings.Contains(sql, "ON DUPLICATE KEY UPDATE") {
		t.Errorf("expected ON DUPLICATE KEY UPDATE, got: %s", sql)
	}
	if !strings.Contains(sql, "`enabled` = ?") {
		t.Errorf("expected explicit col = ?, got: %s", sql)
	}
}

// -------------------------------------------------------------------
// Eager loading / preload tests
// -------------------------------------------------------------------

func TestPreloadUUIDs_BuildsWhereIn(t *testing.T) {
	id1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	id2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	ids := []uuid.UUID{id1, id2}

	assertSQL(t, "preload uuids",
		query.PreloadUUIDs(query.Select().From(ts.UsersT), ts.UsersT.RealmID, ids),
		`SELECT * FROM "users" WHERE "users"."realm_id" IN ($1, $2)`,
		[]any{id1, id2},
	)
}

func TestPreloadUUIDs_EmptyReturnsWhereFalse(t *testing.T) {
	assertSQL(t, "preload uuids empty",
		query.PreloadUUIDs(query.Select().From(ts.UsersT), ts.UsersT.RealmID, nil),
		`SELECT * FROM "users" WHERE FALSE`,
		nil,
	)
}

func TestPreloadStrings_BuildsWhereIn(t *testing.T) {
	assertSQL(t, "preload strings",
		query.PreloadStrings(query.Select().From(ts.UsersT), ts.UsersT.Username, []string{"alice", "bob"}),
		`SELECT * FROM "users" WHERE "users"."username" IN ($1, $2)`,
		[]any{"alice", "bob"},
	)
}

func TestPreloadStrings_EmptyReturnsWhereFalse(t *testing.T) {
	assertSQL(t, "preload strings empty",
		query.PreloadStrings(query.Select().From(ts.UsersT), ts.UsersT.Username, nil),
		`SELECT * FROM "users" WHERE FALSE`,
		nil,
	)
}

func TestPluck(t *testing.T) {
	type Row struct{ ID int }
	rows := []Row{{1}, {2}, {3}}
	got := query.Pluck(rows, func(r Row) int { return r.ID })
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("Pluck result: %v", got)
	}
}

func TestGroupBy(t *testing.T) {
	type Row struct {
		RealmID uuid.UUID
		Name    string
	}
	r1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	r2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	rows := []Row{
		{RealmID: r1, Name: "alice"},
		{RealmID: r2, Name: "bob"},
		{RealmID: r1, Name: "carol"},
	}
	groups := query.GroupBy(rows, func(r Row) uuid.UUID { return r.RealmID })
	if len(groups[r1]) != 2 {
		t.Errorf("expected 2 rows for r1, got %d", len(groups[r1]))
	}
	if len(groups[r2]) != 1 {
		t.Errorf("expected 1 row for r2, got %d", len(groups[r2]))
	}
}

func TestIndex(t *testing.T) {
	type Realm struct {
		ID   uuid.UUID
		Name string
	}
	r1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	r2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	realms := []Realm{{ID: r1, Name: "Alpha"}, {ID: r2, Name: "Beta"}}
	idx := query.Index(realms, func(r Realm) uuid.UUID { return r.ID })
	if idx[r1].Name != "Alpha" {
		t.Errorf("expected Alpha, got %s", idx[r1].Name)
	}
	if idx[r2].Name != "Beta" {
		t.Errorf("expected Beta, got %s", idx[r2].Name)
	}
}

func TestFirst(t *testing.T) {
	items := []string{"a", "b", "c"}
	p := query.First(items)
	if p == nil || *p != "a" {
		t.Errorf("First: got %v", p)
	}
	empty := query.First([]string{})
	if empty != nil {
		t.Errorf("First of empty should be nil, got %v", empty)
	}
}

func TestUniqueUUIDs(t *testing.T) {
	r1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	r2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	input := []uuid.UUID{r1, r2, r1, r1, r2}
	got := query.UniqueUUIDs(input)
	if len(got) != 2 {
		t.Errorf("expected 2 unique UUIDs, got %d: %v", len(got), got)
	}
}

func TestUniqueStrings(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b"}
	got := query.UniqueStrings(input)
	if len(got) != 3 {
		t.Errorf("expected 3 unique strings, got %d: %v", len(got), got)
	}
}

// -------------------------------------------------------------------
// JSONB operator tests
// -------------------------------------------------------------------

func TestJSONB_Arrow(t *testing.T) {
	ctx := newBuildCtx()
	sql := ts.UsersT.Attributes.Arrow("role").ToSQL(ctx)
	want := `"users"."attributes" -> $1`
	if sql != want {
		t.Errorf("Arrow SQL: got %q, want %q", sql, want)
	}
}

func TestJSONB_ArrowText(t *testing.T) {
	ctx := newBuildCtx()
	sql := ts.UsersT.Attributes.ArrowText("email").ToSQL(ctx)
	want := `"users"."attributes" ->> $1`
	if sql != want {
		t.Errorf("ArrowText SQL: got %q, want %q", sql, want)
	}
}

func TestJSONB_Path(t *testing.T) {
	ctx := newBuildCtx()
	sql := ts.UsersT.Attributes.Path("address", "city").ToSQL(ctx)
	want := `"users"."attributes" #> ARRAY['address', 'city']`
	if sql != want {
		t.Errorf("Path SQL: got %q, want %q", sql, want)
	}
}

func TestJSONB_PathText(t *testing.T) {
	ctx := newBuildCtx()
	sql := ts.UsersT.Attributes.PathText("address", "city").ToSQL(ctx)
	want := `"users"."attributes" #>> ARRAY['address', 'city']`
	if sql != want {
		t.Errorf("PathText SQL: got %q, want %q", sql, want)
	}
}

func TestJSONB_Contains_InWhere(t *testing.T) {
	assertSQL(t, "jsonb contains in where",
		query.Select().From(ts.UsersT).
			Where(ts.UsersT.Attributes.Contains(map[string]any{"role": "admin"})),
		`SELECT * FROM "users" WHERE "users"."attributes" @> $1`,
		[]any{map[string]any{"role": "admin"}},
	)
}

func TestJSONB_HasKey_InWhere(t *testing.T) {
	assertSQL(t, "jsonb has key",
		query.Select().From(ts.UsersT).
			Where(ts.UsersT.Attributes.HasKey("verified")),
		`SELECT * FROM "users" WHERE "users"."attributes" ? $1`,
		[]any{"verified"},
	)
}

func TestJSONB_HasKeyNot_InWhere(t *testing.T) {
	assertSQL(t, "jsonb has key not",
		query.Select().From(ts.UsersT).
			Where(ts.UsersT.Attributes.HasKeyNot("banned")),
		`SELECT * FROM "users" WHERE NOT "users"."attributes" ? $1`,
		[]any{"banned"},
	)
}

func TestJSONB_HasAnyKey_InWhere(t *testing.T) {
	assertSQL(t, "jsonb has any key",
		query.Select().From(ts.UsersT).
			Where(ts.UsersT.Attributes.HasAnyKey("admin", "moderator")),
		`SELECT * FROM "users" WHERE "users"."attributes" ?| $1`,
		[]any{[]string{"admin", "moderator"}},
	)
}

func TestJSONB_HasAllKeys_InWhere(t *testing.T) {
	assertSQL(t, "jsonb has all keys",
		query.Select().From(ts.UsersT).
			Where(ts.UsersT.Attributes.HasAllKeys("role", "verified")),
		`SELECT * FROM "users" WHERE "users"."attributes" ?& $1`,
		[]any{[]string{"role", "verified"}},
	)
}

func TestJSONB_ContainedBy(t *testing.T) {
	ctx := newBuildCtx()
	val := map[string]any{"role": "admin", "region": "us"}
	sql := ts.UsersT.Attributes.ContainedBy(val).ToSQL(ctx)
	// val @> col — the value is on the left
	if !strings.Contains(sql, "@>") {
		t.Errorf("ContainedBy SQL missing @>: %s", sql)
	}
	if !strings.Contains(sql, `"users"."attributes"`) {
		t.Errorf("ContainedBy SQL missing column ref: %s", sql)
	}
}

// newBuildCtx creates a Postgres build context for direct expression testing.
func newBuildCtx() *expr.BuildContext {
	return expr.NewBuildContext(dialect.Postgres)
}

// -------------------------------------------------------------------
// Helpers for dynamic search test
// -------------------------------------------------------------------

func whenUUID(ptr *uuid.UUID, f func(uuid.UUID) expr.Expression) expr.Expression {
	if ptr == nil {
		return nil
	}
	return f(*ptr)
}

func whenString(ptr *string, f func(string) expr.Expression) expr.Expression {
	if ptr == nil {
		return nil
	}
	return f(*ptr)
}

// -------------------------------------------------------------------
// Subquery tests
// -------------------------------------------------------------------

func TestSubquery_Exists(t *testing.T) {
	// EXISTS (SELECT * FROM realms WHERE realms.id = users.realm_id)
	sub := query.Select().
		From(ts.RealmsT).
		Where(ts.RealmsT.ID.EQCol(ts.UsersT.RealmID))
	assertSQL(t, "EXISTS subquery",
		query.Select(ts.UsersT.ID).From(ts.UsersT).Where(query.Exists(sub)),
		`SELECT "users"."id" FROM "users" WHERE EXISTS (SELECT * FROM "realms" WHERE "realms"."id" = "users"."realm_id")`,
		nil,
	)
}

func TestSubquery_NotExists(t *testing.T) {
	sub := query.Select().
		From(ts.RealmsT).
		Where(ts.RealmsT.ID.EQCol(ts.UsersT.RealmID))
	assertSQL(t, "NOT EXISTS subquery",
		query.Select(ts.UsersT.ID).From(ts.UsersT).Where(query.NotExists(sub)),
		`SELECT "users"."id" FROM "users" WHERE NOT EXISTS (SELECT * FROM "realms" WHERE "realms"."id" = "users"."realm_id")`,
		nil,
	)
}

func TestSubquery_In(t *testing.T) {
	// SELECT id FROM users WHERE realm_id IN (SELECT id FROM realms WHERE ...)
	sub := query.Select(ts.RealmsT.ID).From(ts.RealmsT).Where(ts.RealmsT.Name.EQ("acme"))
	assertSQL(t, "col IN (subquery)",
		query.Select(ts.UsersT.ID).From(ts.UsersT).
			Where(query.SubqueryIn(ts.UsersT.RealmID, sub)),
		`SELECT "users"."id" FROM "users" WHERE "users"."realm_id" IN (SELECT "realms"."id" FROM "realms" WHERE "realms"."name" = $1)`,
		[]any{"acme"},
	)
}

func TestSubquery_NotIn(t *testing.T) {
	sub := query.Select(ts.RealmsT.ID).From(ts.RealmsT).Where(ts.RealmsT.Name.EQ("banned"))
	assertSQL(t, "col NOT IN (subquery)",
		query.Select(ts.UsersT.ID).From(ts.UsersT).
			Where(query.SubqueryNotIn(ts.UsersT.RealmID, sub)),
		`SELECT "users"."id" FROM "users" WHERE "users"."realm_id" NOT IN (SELECT "realms"."id" FROM "realms" WHERE "realms"."name" = $1)`,
		[]any{"banned"},
	)
}

func TestSubquery_SharedParams(t *testing.T) {
	// Outer query has a param, inner query also has a param — numbers must not collide.
	sub := query.Select(ts.RealmsT.ID).From(ts.RealmsT).Where(ts.RealmsT.Name.EQ("acme"))
	assertSQL(t, "shared param numbering",
		query.Select(ts.UsersT.ID).From(ts.UsersT).
			Where(expr.And(
				ts.UsersT.Enabled.EQ(true),
				query.SubqueryIn(ts.UsersT.RealmID, sub),
			)),
		`SELECT "users"."id" FROM "users" WHERE ("users"."enabled" = $1 AND "users"."realm_id" IN (SELECT "realms"."id" FROM "realms" WHERE "realms"."name" = $2))`,
		[]any{true, "acme"},
	)
}

func TestSubquery_FromSubquery(t *testing.T) {
	// SELECT * FROM (SELECT realm_id, COUNT(*) AS cnt FROM users GROUP BY realm_id) AS sub
	inner := query.Select(ts.UsersT.RealmID, expr.Count().As("cnt")).
		From(ts.UsersT).
		GroupBy(ts.UsersT.RealmID)
	sub := query.FromSubquery(inner, "sub")
	assertSQL(t, "FROM subquery",
		query.Select().From(sub),
		`SELECT * FROM (SELECT "users"."realm_id", COUNT(*) AS "cnt" FROM "users" GROUP BY "users"."realm_id") AS "sub"`,
		nil,
	)
}

func TestSubquery_FromSubquery_SharedParams(t *testing.T) {
	// Params in inner and outer query share numbering.
	inner := query.Select(ts.UsersT.RealmID, expr.Count().As("cnt")).
		From(ts.UsersT).
		Where(ts.UsersT.Enabled.EQ(true)). // $1
		GroupBy(ts.UsersT.RealmID)
	sub := query.FromSubquery(inner, "sub")
	outerQ := query.Select(ts.UsersT.RealmID).From(sub).
		Where(ts.UsersT.Username.EQ("alice")) // $2
	gotSQL, gotArgs := outerQ.Build(dialect.Postgres)
	wantSQL := `SELECT "users"."realm_id" FROM (SELECT "users"."realm_id", COUNT(*) AS "cnt" FROM "users" WHERE "users"."enabled" = $1 GROUP BY "users"."realm_id") AS "sub" WHERE "users"."username" = $2`
	if gotSQL != wantSQL {
		t.Errorf("SQL mismatch\n got:  %s\nwant: %s", gotSQL, wantSQL)
	}
	if len(gotArgs) != 2 || fmt.Sprintf("%v", gotArgs[0]) != "true" || fmt.Sprintf("%v", gotArgs[1]) != "alice" {
		t.Errorf("args mismatch: %v", gotArgs)
	}
}

func whenBool(ptr *bool, f func(bool) expr.Expression) expr.Expression {
	if ptr == nil {
		return nil
	}
	return f(*ptr)
}

// -------------------------------------------------------------------
// Aggregate function tests
// -------------------------------------------------------------------

func TestAgg_CountStar_InSelect(t *testing.T) {
	assertSQL(t, "COUNT(*) in SELECT",
		query.Select(expr.Count()).From(ts.UsersT),
		`SELECT COUNT(*) FROM "users"`,
		nil,
	)
}

func TestAgg_CountStar_WithAlias(t *testing.T) {
	assertSQL(t, "COUNT(*) AS cnt",
		query.Select(expr.Count().As("cnt")).From(ts.UsersT),
		`SELECT COUNT(*) AS "cnt" FROM "users"`,
		nil,
	)
}

func TestAgg_CountCol(t *testing.T) {
	assertSQL(t, "COUNT(col)",
		query.Select(expr.CountCol(ts.UsersT.Username)).From(ts.UsersT),
		`SELECT COUNT("users"."username") FROM "users"`,
		nil,
	)
}

func TestAgg_CountDistinct(t *testing.T) {
	assertSQL(t, "COUNT(DISTINCT col)",
		query.Select(expr.CountDistinct(ts.UsersT.RealmID)).From(ts.UsersT),
		`SELECT COUNT(DISTINCT "users"."realm_id") FROM "users"`,
		nil,
	)
}

func TestAgg_GroupByHaving(t *testing.T) {
	assertSQL(t, "GROUP BY + HAVING COUNT(*) > n",
		query.Select(ts.UsersT.RealmID, expr.Count().As("cnt")).
			From(ts.UsersT).
			GroupBy(ts.UsersT.RealmID).
			Having(expr.Count().GT(5)),
		`SELECT "users"."realm_id", COUNT(*) AS "cnt" FROM "users" GROUP BY "users"."realm_id" HAVING COUNT(*) > $1`,
		[]any{5},
	)
}

func TestAgg_OrderByCountDesc(t *testing.T) {
	assertSQL(t, "ORDER BY COUNT(*) DESC",
		query.Select(ts.UsersT.RealmID, expr.Count()).
			From(ts.UsersT).
			GroupBy(ts.UsersT.RealmID).
			OrderBy(expr.Count().Desc()),
		`SELECT "users"."realm_id", COUNT(*) FROM "users" GROUP BY "users"."realm_id" ORDER BY COUNT(*) DESC`,
		nil,
	)
}

func TestAgg_SumAvgMaxMin(t *testing.T) {
	ctx := newBuildCtx()
	for _, tc := range []struct {
		name string
		agg  expr.AggExpr
		want string
	}{
		{"SUM", expr.Sum(ts.UsersT.RealmID), `SUM("users"."realm_id")`},
		{"MAX", expr.Max(ts.UsersT.Username), `MAX("users"."username")`},
		{"MIN", expr.Min(ts.UsersT.Username), `MIN("users"."username")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.agg.ToSQL(ctx)
			if got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestAgg_HavingGTE(t *testing.T) {
	assertSQL(t, "HAVING COUNT(*) >= 3",
		query.Select(ts.UsersT.RealmID).
			From(ts.UsersT).
			GroupBy(ts.UsersT.RealmID).
			Having(expr.Count().GTE(3)),
		`SELECT "users"."realm_id" FROM "users" GROUP BY "users"."realm_id" HAVING COUNT(*) >= $1`,
		[]any{3},
	)
}
