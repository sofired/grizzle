package query_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	ts "github.com/sofired/grizzle/internal/testschema"
	"github.com/sofired/grizzle/query"
)

// assertSQL is a small helper that builds a query and compares the output.
func assertSQL(t *testing.T, name string, b interface {
	Build(dialect.Dialect) (string, []any)
}, wantSQL string, wantArgs []any) {
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

func TestInsert_IgnoreConflicts_MySQL(t *testing.T) {
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	b := query.InsertInto(ts.RealmsT).Values(row).IgnoreConflicts()
	sql, args := b.Build(dialect.MySQL)
	if sql != "INSERT IGNORE INTO `realms` (`name`) VALUES (?)" {
		t.Errorf("unexpected SQL: %s", sql)
	}
	if len(args) != 1 || args[0] != name {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestInsert_IgnoreConflicts_SQLite(t *testing.T) {
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	b := query.InsertInto(ts.RealmsT).Values(row).IgnoreConflicts()
	sql, args := b.Build(dialect.SQLite)
	if sql != `INSERT OR IGNORE INTO "realms" ("name") VALUES (?)` {
		t.Errorf("unexpected SQL: %s", sql)
	}
	if len(args) != 1 || args[0] != name {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestInsert_IgnoreConflicts_Postgres_Noop(t *testing.T) {
	// PostgreSQL has no INSERT IGNORE equivalent; flag is silently ignored.
	name := "test-realm"
	row := ts.RealmInsert{Name: name}
	b := query.InsertInto(ts.RealmsT).Values(row).IgnoreConflicts()
	sql, _ := b.Build(dialect.Postgres)
	if sql != `INSERT INTO "realms" ("name") VALUES ($1)` {
		t.Errorf("unexpected SQL: %s", sql)
	}
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

// TestUpsert_DoUpdateSetStruct_NilInput verifies that passing nil to
// DoUpdateSetStruct falls back to DO NOTHING rather than emitting an invalid
// empty DO UPDATE SET clause.
func TestUpsert_DoUpdateSetStruct_NilInput(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	username := "alice"
	row := ts.UserInsert{RealmID: realmID, Username: username}
	assertSQL(t, "upsert do update set struct nil falls back to do nothing",
		query.InsertInto(ts.UsersT).
			Values(row).
			OnConflict("realm_id", "username").
			DoUpdateSetStruct(nil),
		`INSERT INTO "users" ("realm_id", "username") VALUES ($1, $2) ON CONFLICT ("realm_id", "username") DO NOTHING`,
		[]any{realmID, username},
	)
}

// TestUpsert_DoUpdateSetStruct_NilPointer verifies that passing a nil pointer
// to DoUpdateSetStruct falls back to DO NOTHING.
func TestUpsert_DoUpdateSetStruct_NilPointer(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	username := "alice"
	row := ts.UserInsert{RealmID: realmID, Username: username}
	var upd *ts.UserUpdate // nil pointer
	assertSQL(t, "upsert do update set struct nil pointer falls back to do nothing",
		query.InsertInto(ts.UsersT).
			Values(row).
			OnConflict("realm_id", "username").
			DoUpdateSetStruct(upd),
		`INSERT INTO "users" ("realm_id", "username") VALUES ($1, $2) ON CONFLICT ("realm_id", "username") DO NOTHING`,
		[]any{realmID, username},
	)
}

// TestUpsert_DoUpdateSetStruct_NonStruct verifies that passing a non-struct
// to DoUpdateSetStruct falls back to DO NOTHING.
func TestUpsert_DoUpdateSetStruct_NonStruct(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	username := "alice"
	row := ts.UserInsert{RealmID: realmID, Username: username}
	assertSQL(t, "upsert do update set struct non-struct falls back to do nothing",
		query.InsertInto(ts.UsersT).
			Values(row).
			OnConflict("realm_id", "username").
			DoUpdateSetStruct("not-a-struct"),
		`INSERT INTO "users" ("realm_id", "username") VALUES ($1, $2) ON CONFLICT ("realm_id", "username") DO NOTHING`,
		[]any{realmID, username},
	)
}

// TestUpsert_DoUpdateSetStruct_AllNilFields verifies that passing a struct
// where all pointer fields are nil (producing no SET assignments) falls back
// to DO NOTHING rather than an empty SET list.
func TestUpsert_DoUpdateSetStruct_AllNilFields(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	username := "alice"
	row := ts.UserInsert{RealmID: realmID, Username: username}
	upd := ts.UserUpdate{} // all pointer fields are nil
	assertSQL(t, "upsert do update set struct all nil fields falls back to do nothing",
		query.InsertInto(ts.UsersT).
			Values(row).
			OnConflict("realm_id", "username").
			DoUpdateSetStruct(upd),
		`INSERT INTO "users" ("realm_id", "username") VALUES ($1, $2) ON CONFLICT ("realm_id", "username") DO NOTHING`,
		[]any{realmID, username},
	)
}

// TestUpsert_DoUpdateSetStruct_NilInput_MySQL verifies that on MySQL dialects,
// DoUpdateSetStruct with nil input omits the ON DUPLICATE KEY UPDATE clause
// (emitting a plain INSERT) rather than an invalid empty assignment list.
func TestUpsert_DoUpdateSetStruct_NilInput_MySQL(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	username := "alice"
	row := ts.UserInsert{RealmID: realmID, Username: username}
	sql, _ := query.InsertInto(ts.UsersT).
		Values(row).
		OnConflict("realm_id", "username").
		DoUpdateSetStruct(nil).
		Build(dialect.MySQL)
	if strings.Contains(sql, "ON DUPLICATE KEY UPDATE") {
		t.Errorf("expected no ON DUPLICATE KEY UPDATE clause, got: %s", sql)
	}
	if strings.Contains(sql, "UPDATE ") {
		t.Errorf("expected no UPDATE clause of any kind, got: %s", sql)
	}
}

// TestUpsert_DoUpdateSetStruct_AllNilFields_WithPriorSets verifies that when
// DoUpdateSetStruct receives a valid struct with all nil pointer fields, it
// does not clear SET assignments already accumulated via DoUpdateSet.
func TestUpsert_DoUpdateSetStruct_AllNilFields_WithPriorSets(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	username := "alice"
	enabled := true
	row := ts.UserInsert{RealmID: realmID, Username: username}
	upd := ts.UserUpdate{} // all pointer fields are nil
	assertSQL(t, "prior DoUpdateSet not cleared by all-nil DoUpdateSetStruct",
		query.InsertInto(ts.UsersT).
			Values(row).
			OnConflict("realm_id", "username").
			DoUpdateSet("enabled", enabled).
			DoUpdateSetStruct(upd),
		`INSERT INTO "users" ("realm_id", "username") VALUES ($1, $2) ON CONFLICT ("realm_id", "username") DO UPDATE SET "enabled" = $3`,
		[]any{realmID, username, enabled},
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
	if rel.Table.GrizTableName() != "realms" {
		t.Errorf("table: got %q, want %q", rel.Table.GrizTableName(), "realms")
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
	if rel.Table.GrizTableName() != "users" {
		t.Errorf("table: got %q, want %q", rel.Table.GrizTableName(), "users")
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

	t.Run("unique keys", func(t *testing.T) {
		realms := []Realm{{ID: r1, Name: "Alpha"}, {ID: r2, Name: "Beta"}}
		idx := query.Index(realms, func(r Realm) uuid.UUID { return r.ID })
		if idx[r1].Name != "Alpha" {
			t.Errorf("expected Alpha, got %s", idx[r1].Name)
		}
		if idx[r2].Name != "Beta" {
			t.Errorf("expected Beta, got %s", idx[r2].Name)
		}
	})

	t.Run("duplicate keys use first-wins semantics", func(t *testing.T) {
		realms := []Realm{
			{ID: r1, Name: "Alpha"},
			{ID: r2, Name: "Beta"},
			{ID: r1, Name: "AlphaDuplicate"},
		}
		idx := query.Index(realms, func(r Realm) uuid.UUID { return r.ID })
		if idx[r1].Name != "Alpha" {
			t.Errorf("first-wins: expected Alpha, got %s", idx[r1].Name)
		}
		if len(idx) != 2 {
			t.Errorf("expected map length 2, got %d", len(idx))
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		idx := query.Index([]Realm{}, func(r Realm) uuid.UUID { return r.ID })
		if len(idx) != 0 {
			t.Errorf("expected empty map, got %d entries", len(idx))
		}
	})

	t.Run("single item", func(t *testing.T) {
		realms := []Realm{{ID: r1, Name: "Only"}}
		idx := query.Index(realms, func(r Realm) uuid.UUID { return r.ID })
		if idx[r1].Name != "Only" {
			t.Errorf("expected Only, got %s", idx[r1].Name)
		}
	})
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

func TestSubquery_In_AliasedCol_NoASClause(t *testing.T) {
	// If an AliasedCol is passed as the SubqueryIn column reference, the AS
	// alias must not appear in the rendered SQL (Fix #131).
	sub := query.Select(ts.RealmsT.ID).From(ts.RealmsT).Where(ts.RealmsT.Name.EQ("acme"))
	assertSQL(t, "AliasedCol IN (subquery) strips alias",
		query.Select(ts.UsersT.ID).From(ts.UsersT).
			Where(query.SubqueryIn(expr.ColAs(ts.UsersT.RealmID, "realm"), sub)),
		`SELECT "users"."id" FROM "users" WHERE "users"."realm_id" IN (SELECT "realms"."id" FROM "realms" WHERE "realms"."name" = $1)`,
		[]any{"acme"},
	)
}

func TestSubquery_NotIn_AliasedCol_NoASClause(t *testing.T) {
	// If an AliasedCol is passed as the SubqueryNotIn column reference, the AS
	// alias must not appear in the rendered SQL (Fix #131 — NOT IN path).
	sub := query.Select(ts.RealmsT.ID).From(ts.RealmsT).Where(ts.RealmsT.Name.EQ("banned"))
	assertSQL(t, "AliasedCol NOT IN (subquery) strips alias",
		query.Select(ts.UsersT.ID).From(ts.UsersT).
			Where(query.SubqueryNotIn(expr.ColAs(ts.UsersT.RealmID, "realm"), sub)),
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

// -------------------------------------------------------------------
// Fix #131 — AliasedCol in GROUP BY must not emit AS alias
// -------------------------------------------------------------------

func TestGroupBy_AliasedCol_NoASClause(t *testing.T) {
	// expr.ColAs wraps a column with a SELECT-list alias. When passed to
	// GroupBy, only the underlying column reference should be rendered —
	// GROUP BY does not accept AS clauses.
	assertSQL(t, "GROUP BY aliased col strips AS",
		query.Select(
			expr.ColAs(ts.UsersT.RealmID, "realm"),
			expr.Count().As("cnt"),
		).
			From(ts.UsersT).
			GroupBy(expr.ColAs(ts.UsersT.RealmID, "realm")),
		`SELECT "users"."realm_id" AS "realm", COUNT(*) AS "cnt" FROM "users" GROUP BY "users"."realm_id"`,
		nil,
	)
}

func TestGroupBy_AliasedCol_MultipleColumns(t *testing.T) {
	// Multiple AliasedCol columns in GROUP BY — each must be rendered without
	// its AS alias.
	assertSQL(t, "GROUP BY multiple aliased cols",
		query.Select(
			expr.ColAs(ts.UsersT.RealmID, "realm"),
			expr.ColAs(ts.UsersT.Enabled, "active"),
			expr.Count().As("cnt"),
		).
			From(ts.UsersT).
			GroupBy(
				expr.ColAs(ts.UsersT.RealmID, "realm"),
				expr.ColAs(ts.UsersT.Enabled, "active"),
			),
		`SELECT "users"."realm_id" AS "realm", "users"."enabled" AS "active", COUNT(*) AS "cnt" FROM "users" GROUP BY "users"."realm_id", "users"."enabled"`,
		nil,
	)
}

func TestGroupBy_PlainCol_Unchanged(t *testing.T) {
	// Non-aliased columns in GROUP BY must continue to work unchanged.
	assertSQL(t, "GROUP BY plain column",
		query.Select(ts.UsersT.RealmID, expr.Count().As("cnt")).
			From(ts.UsersT).
			GroupBy(ts.UsersT.RealmID),
		`SELECT "users"."realm_id", COUNT(*) AS "cnt" FROM "users" GROUP BY "users"."realm_id"`,
		nil,
	)
}

// -------------------------------------------------------------------
// Window function tests
// -------------------------------------------------------------------

func TestWindow_RowNumber_PartitionOrderBy(t *testing.T) {
	assertSQL(t, "ROW_NUMBER() OVER (PARTITION BY realm_id ORDER BY username ASC)",
		query.Select(
			ts.UsersT.ID,
			expr.RowNumber().
				PartitionBy(ts.UsersT.RealmID).
				OrderBy(ts.UsersT.Username.Asc()).
				As("rn"),
		).From(ts.UsersT),
		`SELECT "users"."id", ROW_NUMBER() OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."username" ASC) AS "rn" FROM "users"`,
		nil,
	)
}

func TestWindow_Rank_PartitionOnly(t *testing.T) {
	assertSQL(t, "RANK() OVER (PARTITION BY realm_id)",
		query.Select(
			ts.UsersT.ID,
			expr.Rank().PartitionBy(ts.UsersT.RealmID).As("rnk"),
		).From(ts.UsersT),
		`SELECT "users"."id", RANK() OVER (PARTITION BY "users"."realm_id") AS "rnk" FROM "users"`,
		nil,
	)
}

func TestWindow_DenseRank_NoPartition(t *testing.T) {
	assertSQL(t, "DENSE_RANK() OVER (ORDER BY created_at DESC)",
		query.Select(
			ts.UsersT.ID,
			expr.DenseRank().OrderBy(ts.UsersT.CreatedAt.Desc()).As("dr"),
		).From(ts.UsersT),
		`SELECT "users"."id", DENSE_RANK() OVER (ORDER BY "users"."created_at" DESC) AS "dr" FROM "users"`,
		nil,
	)
}

func TestWindow_Lead_WithColumn(t *testing.T) {
	assertSQL(t, "LEAD(username) OVER (ORDER BY created_at ASC)",
		query.Select(
			ts.UsersT.ID,
			expr.Lead(ts.UsersT.Username).OrderBy(ts.UsersT.CreatedAt.Asc()).As("next_user"),
		).From(ts.UsersT),
		`SELECT "users"."id", LEAD("users"."username") OVER (ORDER BY "users"."created_at" ASC) AS "next_user" FROM "users"`,
		nil,
	)
}

func TestWindow_EmptyOver(t *testing.T) {
	// Window with no PARTITION BY and no ORDER BY — valid SQL: OVER ()
	assertSQL(t, "ROW_NUMBER() OVER ()",
		query.Select(
			ts.UsersT.ID,
			expr.RowNumber().As("rn"),
		).From(ts.UsersT),
		`SELECT "users"."id", ROW_NUMBER() OVER () AS "rn" FROM "users"`,
		nil,
	)
}

func TestWindow_WinSum_PartitionBy(t *testing.T) {
	assertSQL(t, "SUM(col) OVER (PARTITION BY realm_id)",
		query.Select(
			ts.UsersT.ID,
			expr.WinSum(ts.UsersT.RealmID).PartitionBy(ts.UsersT.RealmID).As("realm_count"),
		).From(ts.UsersT),
		`SELECT "users"."id", SUM("users"."realm_id") OVER (PARTITION BY "users"."realm_id") AS "realm_count" FROM "users"`,
		nil,
	)
}

// -------------------------------------------------------------------
// CASE expression tests
// -------------------------------------------------------------------

func TestCase_SearchedCase_MultipleWhen(t *testing.T) {
	assertSQL(t, "CASE WHEN ... THEN ... WHEN ... THEN ... END",
		query.Select(
			ts.UsersT.ID,
			expr.Case().
				When(ts.UsersT.Enabled.IsTrue(), expr.Lit("active")).
				When(ts.UsersT.DeletedAt.IsNotNull(), expr.Lit("deleted")).
				Else(expr.Lit("inactive")).
				As("status"),
		).From(ts.UsersT),
		`SELECT "users"."id", CASE WHEN "users"."enabled" = $1 THEN $2 WHEN "users"."deleted_at" IS NOT NULL THEN $3 ELSE $4 END AS "status" FROM "users"`,
		[]any{true, "active", "deleted", "inactive"},
	)
}

func TestCase_SearchedCase_NoElse(t *testing.T) {
	assertSQL(t, "CASE WHEN ... THEN ... END (no ELSE)",
		query.Select(
			ts.UsersT.ID,
			expr.Case().
				When(ts.UsersT.Enabled.IsTrue(), expr.Lit(1)).
				As("flag"),
		).From(ts.UsersT),
		`SELECT "users"."id", CASE WHEN "users"."enabled" = $1 THEN $2 END AS "flag" FROM "users"`,
		[]any{true, 1},
	)
}

func TestCase_UsedInWhere(t *testing.T) {
	// CASE expressions are valid in WHERE (though unusual); verify it renders.
	q := query.Select(ts.UsersT.ID).
		From(ts.UsersT).
		Where(expr.Case().
			When(ts.UsersT.Enabled.IsTrue(), expr.Lit(1)).
			Else(expr.Lit(0)))
	got, _ := q.Build(dialect.Postgres)
	want := `SELECT "users"."id" FROM "users" WHERE CASE WHEN "users"."enabled" = $1 THEN $2 ELSE $3 END`
	if got != want {
		t.Errorf("SQL mismatch\n got:  %s\nwant: %s", got, want)
	}
}

func TestLit_BoundParameter(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Lit(42).ToSQL(ctx)
	if got != "$1" {
		t.Errorf("Lit(42) should produce $1, got %s", got)
	}
	if len(ctx.Args()) != 1 || ctx.Args()[0] != 42 {
		t.Errorf("expected arg 42, got %v", ctx.Args())
	}
}

// -------------------------------------------------------------------
// CTE (WITH clause) tests
// -------------------------------------------------------------------

func TestCTE_SimpleWith(t *testing.T) {
	recent := query.Select(ts.UsersT.ID, ts.UsersT.RealmID).
		From(ts.UsersT).
		Where(ts.UsersT.DeletedAt.IsNull())

	assertSQL(t, "WITH recent AS (SELECT ...) SELECT *",
		query.Select().
			With("recent", recent).
			From(query.CTERef("recent")),
		`WITH "recent" AS (SELECT "users"."id", "users"."realm_id" FROM "users" WHERE "users"."deleted_at" IS NULL) SELECT * FROM "recent"`,
		nil,
	)
}

func TestCTE_MultipleWith(t *testing.T) {
	activeUsers := query.Select(ts.UsersT.ID).
		From(ts.UsersT).
		Where(ts.UsersT.Enabled.IsTrue())

	activeRealms := query.Select(ts.RealmsT.ID).
		From(ts.RealmsT).
		Where(ts.RealmsT.Enabled.IsTrue())

	q := query.Select().
		With("au", activeUsers).
		With("ar", activeRealms).
		From(query.CTERef("au"))
	got, _ := q.Build(dialect.Postgres)

	if !strings.Contains(got, `WITH "au" AS (`) {
		t.Errorf("missing first CTE in: %s", got)
	}
	if !strings.Contains(got, `, "ar" AS (`) {
		t.Errorf("missing second CTE in: %s", got)
	}
	if !strings.Contains(got, `FROM "au"`) {
		t.Errorf("missing FROM reference in: %s", got)
	}
}

func TestCTE_ParametersSharedAcrossCTE(t *testing.T) {
	// Parameters in the CTE subquery and outer query should use sequential numbering.
	sub := query.Select(ts.UsersT.ID).
		From(ts.UsersT).
		Where(ts.UsersT.Username.EQ("alice"))

	outer := query.Select().
		With("u", sub).
		From(query.CTERef("u")).
		Where(expr.Raw(`"u"."id" IS NOT NULL`))

	got, args := outer.Build(dialect.Postgres)
	want := `WITH "u" AS (SELECT "users"."id" FROM "users" WHERE "users"."username" = $1) SELECT * FROM "u" WHERE "u"."id" IS NOT NULL`
	if got != want {
		t.Errorf("SQL mismatch\n got:  %s\nwant: %s", got, want)
	}
	if len(args) != 1 || args[0] != "alice" {
		t.Errorf("expected [alice], got %v", args)
	}
}

// -------------------------------------------------------------------
// IntColumn / FloatColumn NotIn tests
// -------------------------------------------------------------------

func TestIntColumn_NotIn(t *testing.T) {
	// Need an IntColumn — add one inline using ColBase directly.
	col := expr.IntColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "score"}}
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := col.NotIn(1, 2, 3).ToSQL(ctx)
	want := `"users"."score" NOT IN ($1, $2, $3)`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestIntColumn_NotIn_Empty(t *testing.T) {
	col := expr.IntColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "score"}}
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := col.NotIn().ToSQL(ctx)
	if got != "TRUE" {
		t.Errorf("empty NotIn should produce TRUE, got %s", got)
	}
}

func TestFloatColumn_In(t *testing.T) {
	col := expr.FloatColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "score"}}
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := col.In(1.5, 2.5).ToSQL(ctx)
	want := `"users"."score" IN ($1, $2)`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestFloatColumn_NotIn(t *testing.T) {
	col := expr.FloatColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "score"}}
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := col.NotIn(1.5, 2.5).ToSQL(ctx)
	want := `"users"."score" NOT IN ($1, $2)`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// -------------------------------------------------------------------
// Simple CASE tests
// -------------------------------------------------------------------

func TestSimpleCase_WhenVal(t *testing.T) {
	assertSQL(t, "CASE col WHEN val THEN result ELSE default END",
		query.Select(
			ts.UsersT.ID,
			expr.SimpleCase(ts.UsersT.Username).
				WhenVal("alice", expr.Lit("Alice")).
				WhenVal("bob", expr.Lit("Bob")).
				Else(expr.Lit("Other")).
				As("display_name"),
		).From(ts.UsersT),
		`SELECT "users"."id", CASE "users"."username" WHEN $1 THEN $2 WHEN $3 THEN $4 ELSE $5 END AS "display_name" FROM "users"`,
		[]any{"alice", "Alice", "bob", "Bob", "Other"},
	)
}

func TestSimpleCase_NoElse(t *testing.T) {
	assertSQL(t, "CASE col WHEN val THEN result END (no ELSE)",
		query.Select(
			ts.UsersT.ID,
			expr.SimpleCase(ts.UsersT.Username).
				WhenVal("admin", expr.Lit(1)).
				As("is_admin"),
		).From(ts.UsersT),
		`SELECT "users"."id", CASE "users"."username" WHEN $1 THEN $2 END AS "is_admin" FROM "users"`,
		[]any{"admin", 1},
	)
}

// -------------------------------------------------------------------
// DISTINCT tests
// -------------------------------------------------------------------

func TestSelect_Distinct(t *testing.T) {
	assertSQL(t, "SELECT DISTINCT",
		query.Select(ts.UsersT.RealmID).From(ts.UsersT).Distinct(),
		`SELECT DISTINCT "users"."realm_id" FROM "users"`,
		nil,
	)
}

func TestSelect_Distinct_Star(t *testing.T) {
	assertSQL(t, "SELECT DISTINCT *",
		query.Select().From(ts.UsersT).Distinct(),
		`SELECT DISTINCT * FROM "users"`,
		nil,
	)
}

// -------------------------------------------------------------------
// NULLS FIRST / NULLS LAST tests
// -------------------------------------------------------------------

func TestOrderBy_NullsFirst(t *testing.T) {
	assertSQL(t, "ORDER BY col ASC NULLS FIRST",
		query.Select().From(ts.UsersT).OrderBy(ts.UsersT.DeletedAt.Asc().NullsFirst()),
		`SELECT * FROM "users" ORDER BY "users"."deleted_at" ASC NULLS FIRST`,
		nil,
	)
}

func TestOrderBy_NullsLast(t *testing.T) {
	assertSQL(t, "ORDER BY col DESC NULLS LAST",
		query.Select().From(ts.UsersT).OrderBy(ts.UsersT.DeletedAt.Desc().NullsLast()),
		`SELECT * FROM "users" ORDER BY "users"."deleted_at" DESC NULLS LAST`,
		nil,
	)
}

// -------------------------------------------------------------------
// FOR UPDATE / FOR SHARE tests
// -------------------------------------------------------------------

func TestSelect_ForUpdate(t *testing.T) {
	assertSQL(t, "FOR UPDATE",
		query.Select().From(ts.UsersT).Where(ts.UsersT.ID.EQ(uuid.Nil)).ForUpdate(),
		`SELECT * FROM "users" WHERE "users"."id" = $1 FOR UPDATE`,
		[]any{uuid.Nil},
	)
}

func TestSelect_ForShare_Postgres(t *testing.T) {
	q := query.Select().From(ts.UsersT).ForShare()
	got, _ := q.Build(dialect.Postgres)
	if !strings.Contains(got, "FOR SHARE") {
		t.Errorf("expected FOR SHARE in: %s", got)
	}
}

func TestSelect_ForShare_MySQL(t *testing.T) {
	q := query.Select().From(ts.UsersT).ForShare()
	got, _ := q.Build(dialect.MySQL)
	if !strings.Contains(got, "LOCK IN SHARE MODE") {
		t.Errorf("expected LOCK IN SHARE MODE in: %s", got)
	}
}

// TestSelect_ForUpdate_SQLite verifies that FOR UPDATE is silently dropped for
// SQLite, which uses file-level locking and does not support row-level locking.
func TestSelect_ForUpdate_SQLite(t *testing.T) {
	q := query.Select().From(ts.UsersT).ForUpdate()
	got, _ := q.Build(dialect.SQLite)
	if strings.Contains(got, "FOR UPDATE") {
		t.Errorf("FOR UPDATE should not be emitted for SQLite, got: %s", got)
	}
}

// TestSelect_ForShare_SQLite verifies that FOR SHARE is silently dropped for
// SQLite, which uses file-level locking and does not support row-level locking.
func TestSelect_ForShare_SQLite(t *testing.T) {
	q := query.Select().From(ts.UsersT).ForShare()
	got, _ := q.Build(dialect.SQLite)
	if strings.Contains(got, "FOR SHARE") || strings.Contains(got, "LOCK IN") {
		t.Errorf("locking clause should not be emitted for SQLite, got: %s", got)
	}
}

func TestSelect_ForNoKeyUpdate_Postgres(t *testing.T) {
	q := query.Select().From(ts.UsersT).ForNoKeyUpdate()
	got, _ := q.Build(dialect.Postgres)
	if !strings.Contains(got, "FOR NO KEY UPDATE") {
		t.Errorf("expected FOR NO KEY UPDATE in: %s", got)
	}
}

func TestSelect_ForNoKeyUpdate_MySQL(t *testing.T) {
	q := query.Select().From(ts.UsersT).ForNoKeyUpdate()
	got, _ := q.Build(dialect.MySQL)
	if strings.Contains(got, "NO KEY") {
		t.Errorf("FOR NO KEY UPDATE should not be emitted for MySQL, got: %s", got)
	}
}

func TestSelect_ForKeyShare_Postgres(t *testing.T) {
	q := query.Select().From(ts.UsersT).ForKeyShare()
	got, _ := q.Build(dialect.Postgres)
	if !strings.Contains(got, "FOR KEY SHARE") {
		t.Errorf("expected FOR KEY SHARE in: %s", got)
	}
}

func TestSelect_ForKeyShare_MySQL(t *testing.T) {
	q := query.Select().From(ts.UsersT).ForKeyShare()
	got, _ := q.Build(dialect.MySQL)
	if strings.Contains(got, "KEY SHARE") {
		t.Errorf("FOR KEY SHARE should not be emitted for MySQL, got: %s", got)
	}
}

// aliasedTable is a minimal TableSource whose alias differs from its name,
// used to verify that the OF clause renders the alias and not the base name.
type aliasedTable struct{ name, alias string }

func (a aliasedTable) GrizTableName() string  { return a.name }
func (a aliasedTable) GrizTableAlias() string { return a.alias }

func TestSelect_ForUpdate_Of_Alias(t *testing.T) {
	tbl := aliasedTable{name: "orders", alias: "o"}
	q := query.Select().From(tbl).ForUpdate().Of(tbl)
	got, _ := q.Build(dialect.Postgres)
	want := `SELECT * FROM "orders" AS "o" FOR UPDATE OF "o"`
	if got != want {
		t.Errorf("OF alias\ngot:  %s\nwant: %s", got, want)
	}
}

func TestSelect_ForUpdate_Of_NoAlias(t *testing.T) {
	q := query.Select().From(ts.UsersT).ForUpdate().Of(ts.UsersT)
	assertSQL(t, "FOR UPDATE OF no-alias",
		q,
		`SELECT * FROM "users" FOR UPDATE OF "users"`,
		nil,
	)
}

func TestSelect_ForShare_Of_Postgres(t *testing.T) {
	tbl := aliasedTable{name: "orders", alias: "o"}
	q := query.Select().From(tbl).ForShare().Of(tbl)
	got, _ := q.Build(dialect.Postgres)
	want := `SELECT * FROM "orders" AS "o" FOR SHARE OF "o"`
	if got != want {
		t.Errorf("FOR SHARE OF\ngot:  %s\nwant: %s", got, want)
	}
}

func TestSelect_ForShare_Of_MySQL_Dropped(t *testing.T) {
	// MySQL LOCK IN SHARE MODE does not support OF; it must be dropped.
	tbl := aliasedTable{name: "orders", alias: "o"}
	q := query.Select().From(tbl).ForShare().Of(tbl)
	got, _ := q.Build(dialect.MySQL)
	if strings.Contains(got, " OF ") {
		t.Errorf("MySQL LOCK IN SHARE MODE must not emit OF clause, got: %s", got)
	}
	if !strings.Contains(got, "LOCK IN SHARE MODE") {
		t.Errorf("expected LOCK IN SHARE MODE in: %s", got)
	}
}

func TestSelect_ForUpdate_Of_MySQL_MultiTable(t *testing.T) {
	// MySQL 8.0+ FOR UPDATE supports OF with multiple tables.
	t1 := aliasedTable{name: "orders", alias: "o"}
	t2 := aliasedTable{name: "items", alias: "i"}
	q := query.Select().From(t1).ForUpdate().Of(t1, t2)
	got, _ := q.Build(dialect.MySQL)
	if strings.Count(got, " OF ") != 1 {
		t.Fatalf("expected exactly one OF clause in: %s", got)
	}
	// MySQL uses backtick quoting.
	if !strings.Contains(got, "`o`") {
		t.Errorf("expected first table alias `o` in OF clause: %s", got)
	}
	if !strings.Contains(got, "`i`") {
		t.Errorf("expected second table alias `i` in OF clause: %s", got)
	}
}

func TestSelect_Of_BeforeForUpdate(t *testing.T) {
	// Of() can precede ForUpdate(); call order does not matter.
	tbl := aliasedTable{name: "orders", alias: "o"}
	q := query.Select().From(tbl).Of(tbl).ForUpdate()
	got, _ := q.Build(dialect.Postgres)
	want := `SELECT * FROM "orders" AS "o" FOR UPDATE OF "o"`
	if got != want {
		t.Errorf("Of before ForUpdate\ngot:  %s\nwant: %s", got, want)
	}
}

func TestSelect_ForUpdate_Of_SQLite_Dropped(t *testing.T) {
	// SQLite has no row-level locking; the entire clause must be dropped.
	tbl := aliasedTable{name: "orders", alias: "o"}
	q := query.Select().From(tbl).ForUpdate().Of(tbl)
	got, _ := q.Build(dialect.SQLite)
	if strings.Contains(got, "FOR UPDATE") || strings.Contains(got, " OF ") {
		t.Errorf("SQLite must drop all locking clauses, got: %s", got)
	}
}

// -------------------------------------------------------------------
// For(strength, opts...) — SKIP LOCKED / NOWAIT tests
// -------------------------------------------------------------------

func TestSelect_For_SkipLocked_Postgres(t *testing.T) {
	q := query.Select().From(ts.UsersT).For(query.LockForUpdate, query.SkipLocked)
	got, _ := q.Build(dialect.Postgres)
	want := `SELECT * FROM "users" FOR UPDATE SKIP LOCKED`
	if got != want {
		t.Errorf("got:  %s\nwant: %s", got, want)
	}
}

func TestSelect_For_NoWait_Postgres(t *testing.T) {
	q := query.Select().From(ts.UsersT).For(query.LockForUpdate, query.NoWait)
	got, _ := q.Build(dialect.Postgres)
	want := `SELECT * FROM "users" FOR UPDATE NOWAIT`
	if got != want {
		t.Errorf("got:  %s\nwant: %s", got, want)
	}
}

func TestSelect_For_SkipLocked_MySQL(t *testing.T) {
	q := query.Select().From(ts.UsersT).For(query.LockForUpdate, query.SkipLocked)
	got, _ := q.Build(dialect.MySQL)
	// MySQL uses backtick quoting and appends SKIP LOCKED after FOR UPDATE.
	if !strings.Contains(got, "FOR UPDATE") {
		t.Errorf("expected FOR UPDATE in: %s", got)
	}
	if !strings.Contains(got, "SKIP LOCKED") {
		t.Errorf("expected SKIP LOCKED in: %s", got)
	}
}

func TestSelect_For_NoWait_MySQL(t *testing.T) {
	q := query.Select().From(ts.UsersT).For(query.LockForUpdate, query.NoWait)
	got, _ := q.Build(dialect.MySQL)
	if !strings.Contains(got, "FOR UPDATE") {
		t.Errorf("expected FOR UPDATE in: %s", got)
	}
	if !strings.Contains(got, "NOWAIT") {
		t.Errorf("expected NOWAIT in: %s", got)
	}
}

func TestSelect_For_SkipLocked_SQLite_Dropped(t *testing.T) {
	// SQLite has no row-level locking; the entire clause including opts must be dropped.
	q := query.Select().From(ts.UsersT).For(query.LockForUpdate, query.SkipLocked)
	got, _ := q.Build(dialect.SQLite)
	if strings.Contains(got, "FOR UPDATE") || strings.Contains(got, "SKIP LOCKED") {
		t.Errorf("SQLite must drop all locking clauses, got: %s", got)
	}
}

func TestSelect_For_NoKeyUpdate_Of_Postgres(t *testing.T) {
	tbl := aliasedTable{name: "orders", alias: "o"}
	q := query.Select().From(tbl).For(query.LockForNoKeyUpdate).Of(tbl)
	got, _ := q.Build(dialect.Postgres)
	want := `SELECT * FROM "orders" AS "o" FOR NO KEY UPDATE OF "o"`
	if got != want {
		t.Errorf("got:  %s\nwant: %s", got, want)
	}
}

func TestSelect_For_KeyShare_Of_Postgres(t *testing.T) {
	tbl := aliasedTable{name: "orders", alias: "o"}
	q := query.Select().From(tbl).For(query.LockForKeyShare).Of(tbl)
	got, _ := q.Build(dialect.Postgres)
	want := `SELECT * FROM "orders" AS "o" FOR KEY SHARE OF "o"`
	if got != want {
		t.Errorf("got:  %s\nwant: %s", got, want)
	}
}

func TestSelect_For_WithOptsAndOf(t *testing.T) {
	// FOR UPDATE OF "o" NOWAIT — combined Of+opts on postgres.
	tbl := aliasedTable{name: "orders", alias: "o"}
	q := query.Select().From(tbl).For(query.LockForUpdate, query.NoWait).Of(tbl)
	got, _ := q.Build(dialect.Postgres)
	want := `SELECT * FROM "orders" AS "o" FOR UPDATE OF "o" NOWAIT`
	if got != want {
		t.Errorf("got:  %s\nwant: %s", got, want)
	}
}

// -------------------------------------------------------------------
// UPDATE / DELETE LIMIT tests
// -------------------------------------------------------------------

// -------------------------------------------------------------------
// Update nil / invalid struct guard (Fix #3 / Fix #120)
// -------------------------------------------------------------------

func TestUpdate_SetStruct_Nil(t *testing.T) {
	t.Run("nil interface", func(t *testing.T) {
		sql, args := query.Update(ts.UsersT).SetStruct(nil).Build(dialect.Postgres)
		if sql != "" || args != nil {
			t.Errorf("expected empty result for nil SetStruct, got sql=%q args=%v", sql, args)
		}
	})

	t.Run("nil pointer exercises !rv.IsValid() guard", func(t *testing.T) {
		var p *ts.UserUpdate // nil pointer — Elem() returns invalid reflect.Value
		sql, args := query.Update(ts.UsersT).SetStruct(p).Build(dialect.Postgres)
		if sql != "" || args != nil {
			t.Errorf("expected empty result for nil pointer SetStruct, got sql=%q args=%v", sql, args)
		}
	})

	t.Run("non-struct value", func(t *testing.T) {
		sql, args := query.Update(ts.UsersT).SetStruct(42).Build(dialect.Postgres)
		if sql != "" || args != nil {
			t.Errorf("expected empty result for non-struct SetStruct, got sql=%q args=%v", sql, args)
		}
	})
}

func TestUpdate_Limit_MySQL(t *testing.T) {
	q := query.Update(ts.UsersT).
		Set("enabled", false).
		Where(ts.UsersT.DeletedAt.IsNotNull()).
		Limit(100)
	got, _ := q.Build(dialect.MySQL)
	want := "UPDATE `users` SET `enabled` = ? WHERE `users`.`deleted_at` IS NOT NULL LIMIT 100"
	if got != want {
		t.Errorf("SQL mismatch\n got:  %s\nwant: %s", got, want)
	}
}

func TestUpdate_Limit_Postgres_Ignored(t *testing.T) {
	q := query.Update(ts.UsersT).
		Set("enabled", false).
		Limit(100)
	got, _ := q.Build(dialect.Postgres)
	if strings.Contains(got, "LIMIT") {
		t.Errorf("LIMIT should not appear in Postgres UPDATE: %s", got)
	}
}

func TestDelete_Limit_MySQL(t *testing.T) {
	q := query.DeleteFrom(ts.UsersT).
		Where(ts.UsersT.DeletedAt.IsNotNull()).
		Limit(50)
	got, _ := q.Build(dialect.MySQL)
	want := "DELETE FROM `users` WHERE `users`.`deleted_at` IS NOT NULL LIMIT 50"
	if got != want {
		t.Errorf("SQL mismatch\n got:  %s\nwant: %s", got, want)
	}
}

func TestDelete_Limit_Postgres_Ignored(t *testing.T) {
	q := query.DeleteFrom(ts.UsersT).Where(ts.UsersT.DeletedAt.IsNotNull()).Limit(50)
	got, _ := q.Build(dialect.Postgres)
	if strings.Contains(got, "LIMIT") {
		t.Errorf("LIMIT should not appear in Postgres DELETE: %s", got)
	}
}

// -------------------------------------------------------------------
// Column-to-column operator tests
// -------------------------------------------------------------------

func TestTimestampColumn_EQCol(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := ts.UsersT.CreatedAt.EQCol(ts.UsersT.UpdatedAt).ToSQL(ctx)
	want := `"users"."created_at" = "users"."updated_at"`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestTimestampColumn_LTCol(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := ts.UsersT.CreatedAt.LTCol(ts.UsersT.DeletedAt).ToSQL(ctx)
	want := `"users"."created_at" < "users"."deleted_at"`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestFloatColumn_GTCol(t *testing.T) {
	a := expr.FloatColumn{ColBase: expr.ColBase{TableAlias: "products", ColName: "price"}}
	b := expr.FloatColumn{ColBase: expr.ColBase{TableAlias: "products", ColName: "cost"}}
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := a.GTCol(b).ToSQL(ctx)
	want := `"products"."price" > "products"."cost"`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// -------------------------------------------------------------------
// UNION / UNION ALL / INTERSECT / EXCEPT
// -------------------------------------------------------------------

func TestSetOp_Union(t *testing.T) {
	a := query.Select(ts.UsersT.Username).From(ts.UsersT).Where(ts.UsersT.Enabled.IsTrue())
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT)
	assertSQL(t, "union",
		a.Union(b),
		`(SELECT "users"."username" FROM "users" WHERE "users"."enabled" = $1) UNION (SELECT "realms"."name" FROM "realms")`,
		[]any{true},
	)
}

func TestSetOp_UnionAll(t *testing.T) {
	a := query.Select(ts.UsersT.Username).From(ts.UsersT)
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT)
	assertSQL(t, "union all",
		a.UnionAll(b),
		`(SELECT "users"."username" FROM "users") UNION ALL (SELECT "realms"."name" FROM "realms")`,
		nil,
	)
}

func TestSetOp_Intersect(t *testing.T) {
	a := query.Select(ts.UsersT.Username).From(ts.UsersT)
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT)
	assertSQL(t, "intersect",
		a.Intersect(b),
		`(SELECT "users"."username" FROM "users") INTERSECT (SELECT "realms"."name" FROM "realms")`,
		nil,
	)
}

func TestSetOp_Except(t *testing.T) {
	a := query.Select(ts.UsersT.Username).From(ts.UsersT)
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT)
	assertSQL(t, "except",
		a.Except(b),
		`(SELECT "users"."username" FROM "users") EXCEPT (SELECT "realms"."name" FROM "realms")`,
		nil,
	)
}

func TestSetOp_UnionAll_ThreeParts(t *testing.T) {
	a := query.Select(ts.UsersT.Username).From(ts.UsersT)
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT)
	c := query.Select(ts.UsersT.Email).From(ts.UsersT).Where(ts.UsersT.Email.IsNotNull())
	assertSQL(t, "union all three parts",
		a.UnionAll(b).UnionAll(c),
		`(SELECT "users"."username" FROM "users") UNION ALL (SELECT "realms"."name" FROM "realms") UNION ALL (SELECT "users"."email" FROM "users" WHERE "users"."email" IS NOT NULL)`,
		nil,
	)
}

func TestSetOp_Union_WithLimitOrderBy(t *testing.T) {
	a := query.Select(ts.UsersT.Username).From(ts.UsersT)
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT)
	// Set operation ORDER BY must strip the table qualifier (Fix #11).
	assertSQL(t, "union with limit and order",
		a.Union(b).OrderBy(ts.UsersT.Username.Asc()).Limit(10),
		`(SELECT "users"."username" FROM "users") UNION (SELECT "realms"."name" FROM "realms") ORDER BY "username" ASC LIMIT 10`,
		nil,
	)
}

// Fix #134 — SetOpBuilder.OrderByCols edge cases
func TestSetOp_Intersect_WithOrderBy(t *testing.T) {
	a := query.Select(ts.UsersT.Username).From(ts.UsersT)
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT)
	assertSQL(t, "intersect with order by",
		a.Intersect(b).OrderBy(ts.UsersT.Username.Asc()),
		`(SELECT "users"."username" FROM "users") INTERSECT (SELECT "realms"."name" FROM "realms") ORDER BY "username" ASC`,
		nil,
	)
}

func TestSetOp_Except_WithOrderBy(t *testing.T) {
	a := query.Select(ts.UsersT.Username).From(ts.UsersT)
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT)
	assertSQL(t, "except with order by",
		a.Except(b).OrderBy(ts.UsersT.Username.Desc()),
		`(SELECT "users"."username" FROM "users") EXCEPT (SELECT "realms"."name" FROM "realms") ORDER BY "username" DESC`,
		nil,
	)
}

func TestSetOp_EmptyOrderBy(t *testing.T) {
	// No OrderBy call — result must not contain ORDER BY.
	a := query.Select(ts.UsersT.Username).From(ts.UsersT)
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT)
	sql, _ := a.Union(b).Build(dialect.Postgres)
	if strings.Contains(sql, "ORDER BY") {
		t.Errorf("expected no ORDER BY in result without OrderBy() call, got: %s", sql)
	}
}

func TestSetOp_LimitOffset(t *testing.T) {
	a := query.Select(ts.UsersT.Username).From(ts.UsersT)
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT)
	assertSQL(t, "union with limit and offset",
		a.Union(b).OrderBy(ts.UsersT.Username.Asc()).Limit(5).Offset(10),
		`(SELECT "users"."username" FROM "users") UNION (SELECT "realms"."name" FROM "realms") ORDER BY "username" ASC LIMIT 5 OFFSET 10`,
		nil,
	)
}

func TestSetOp_SharedParameters(t *testing.T) {
	a := query.Select(ts.UsersT.Username).From(ts.UsersT).Where(ts.UsersT.Enabled.EQ(true))
	b := query.Select(ts.RealmsT.Name).From(ts.RealmsT).Where(ts.RealmsT.Enabled.EQ(false))
	sql, args := a.UnionAll(b).Build(dialect.Postgres)
	if !strings.Contains(sql, "$1") || !strings.Contains(sql, "$2") {
		t.Errorf("expected shared parameter numbering, got: %s", sql)
	}
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d: %v", len(args), args)
	}
}

// -------------------------------------------------------------------
// Arithmetic expressions
// -------------------------------------------------------------------

var (
	scoreCol    = expr.IntColumn{ColBase: expr.ColBase{TableAlias: "products", ColName: "score"}}
	quantCol    = expr.IntColumn{ColBase: expr.ColBase{TableAlias: "orders", ColName: "quantity"}}
	priceCol    = expr.FloatColumn{ColBase: expr.ColBase{TableAlias: "orders", ColName: "price"}}
	discountCol = expr.FloatColumn{ColBase: expr.ColBase{TableAlias: "orders", ColName: "discount"}}
)

func TestArith_IntColumn_Add(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := scoreCol.Add(10).ToSQL(ctx)
	want := `("products"."score" + $1)`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if ctx.Args()[0] != 10 {
		t.Errorf("arg = %v, want 10", ctx.Args()[0])
	}
}

func TestArith_IntColumn_Sub(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := scoreCol.Sub(5).ToSQL(ctx)
	if got != `("products"."score" - $1)` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestArith_IntColumn_Mul(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := scoreCol.Mul(3).ToSQL(ctx)
	if got != `("products"."score" * $1)` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestArith_IntColumn_Div(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := scoreCol.Div(2).ToSQL(ctx)
	if got != `("products"."score" / $1)` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestArith_IntColumn_AddCol(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := quantCol.AddCol(scoreCol).ToSQL(ctx)
	if got != `("orders"."quantity" + "products"."score")` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestArith_FloatColumn_Mul(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := priceCol.Mul(1.1).ToSQL(ctx)
	if got != `("orders"."price" * $1)` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestArith_FloatColumn_MulCol(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := priceCol.MulCol(discountCol).ToSQL(ctx)
	if got != `("orders"."price" * "orders"."discount")` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestArith_Chain(t *testing.T) {
	// (score + 5) * 2
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := scoreCol.Add(5).Mul(2).ToSQL(ctx)
	if got != `(("products"."score" + $1) * $2)` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestArith_As_SelectAlias(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := priceCol.Mul(0.9).As("discounted").ToSQL(ctx)
	if got != `("orders"."price" * $1) AS "discounted"` {
		t.Errorf("unexpected: %s", got)
	}
}

func TestArith_GTE_InWhere(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := scoreCol.Add(10).GTE(100).ToSQL(ctx)
	// ("products"."score" + $1) >= $2
	if !strings.Contains(got, ">=") {
		t.Errorf("expected >= operator, got: %s", got)
	}
}

func TestArith_UsedInSelect(t *testing.T) {
	type ordersTable struct{ expr.ColBase }
	orders := ordersTable{}
	_ = orders
	assertSQL(t, "arith in select",
		query.Select(priceCol.MulCol(discountCol).As("total")).From(ts.UsersT),
		`SELECT ("orders"."price" * "orders"."discount") AS "total" FROM "users"`,
		nil,
	)
}

// -------------------------------------------------------------------
// CAST
// -------------------------------------------------------------------

func TestCast_ColAsText(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Cast(ts.UsersT.ID, "text").ToSQL(ctx)
	want := `CAST("users"."id" AS text)`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestCast_WithAlias(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Cast(scoreCol, "bigint").As("big_score").ToSQL(ctx)
	want := `CAST("products"."score" AS bigint) AS "big_score"`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestCast_EQ_InWhere(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Cast(ts.UsersT.ID, "text").EQ("abc").ToSQL(ctx)
	want := `CAST("users"."id" AS text) = $1`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// -------------------------------------------------------------------
// COALESCE / NULLIF
// -------------------------------------------------------------------

func TestCoalesce_TwoColumns(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Coalesce(expr.Col(ts.UsersT.Email), expr.Col(ts.UsersT.Username)).ToSQL(ctx)
	want := `COALESCE("users"."email", "users"."username")`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestCoalesce_ColumnAndLiteral(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Coalesce(expr.Col(ts.UsersT.Email), expr.Lit("anon")).ToSQL(ctx)
	want := `COALESCE("users"."email", $1)`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestCoalesce_WithAlias(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Coalesce(expr.Col(ts.UsersT.Email), expr.Lit("anon")).As("display_email").ToSQL(ctx)
	if !strings.Contains(got, `AS "display_email"`) {
		t.Errorf("alias not rendered: %s", got)
	}
}

func TestNullIf(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.NullIf(expr.Col(scoreCol), expr.Lit(0)).ToSQL(ctx)
	want := `NULLIF("products"."score", $1)`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// -------------------------------------------------------------------
// String functions
// -------------------------------------------------------------------

func TestUpper(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Upper(ts.UsersT.Username).ToSQL(ctx)
	want := `UPPER("users"."username")`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestLower(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Lower(ts.UsersT.Email).ToSQL(ctx)
	want := `LOWER("users"."email")`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestLength(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Length(ts.UsersT.Username).GT(3).ToSQL(ctx)
	if !strings.Contains(got, "LENGTH") || !strings.Contains(got, ">") {
		t.Errorf("unexpected: %s", got)
	}
}

func TestTrim(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Trim(ts.UsersT.Username).ToSQL(ctx)
	want := `TRIM("users"."username")`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestConcat(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Concat(expr.Col(ts.UsersT.Username), expr.Lit(" "), expr.Col(ts.UsersT.Email)).ToSQL(ctx)
	want := `CONCAT("users"."username", $1, "users"."email")`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestConcatCols(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.ConcatCols(ts.UsersT.Username, ts.UsersT.Email).ToSQL(ctx)
	want := `CONCAT("users"."username", "users"."email")`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestLower_Like_InWhere(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Lower(ts.UsersT.Email).Like("%@example.com").ToSQL(ctx)
	if !strings.Contains(got, "LOWER") || !strings.Contains(got, "LIKE") {
		t.Errorf("unexpected: %s", got)
	}
}

func TestFuncExpr_Asc_Desc(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	asc := expr.Lower(ts.UsersT.Username).Asc()
	desc := expr.Upper(ts.UsersT.Email).Desc()
	gotAsc := asc.ToSQL(ctx)
	gotDesc := desc.ToSQL(ctx)
	if !strings.Contains(gotAsc, "ASC") || !strings.Contains(gotAsc, "LOWER") {
		t.Errorf("asc: unexpected: %s", gotAsc)
	}
	if !strings.Contains(gotDesc, "DESC") || !strings.Contains(gotDesc, "UPPER") {
		t.Errorf("desc: unexpected: %s", gotDesc)
	}
}

// -------------------------------------------------------------------
// Numeric functions
// -------------------------------------------------------------------

func TestAbs(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Abs(priceCol).ToSQL(ctx)
	if got != `ABS("orders"."price")` {
		t.Errorf("got %s", got)
	}
}

func TestRound_NoDecimals(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Round(priceCol).ToSQL(ctx)
	if got != `ROUND("orders"."price")` {
		t.Errorf("got %s", got)
	}
}

func TestRound_WithDecimals(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := expr.Round(priceCol, 2).ToSQL(ctx)
	// ROUND("orders"."price", $1)
	if !strings.Contains(got, "ROUND") || !strings.Contains(got, "$1") {
		t.Errorf("got %s", got)
	}
}

// -------------------------------------------------------------------
// Recursive CTEs
// -------------------------------------------------------------------

func TestWithRecursive_Basic(t *testing.T) {
	idCol := expr.IntColumn{ColBase: expr.ColBase{TableAlias: "nodes", ColName: "id"}}
	parentCol := expr.IntColumn{ColBase: expr.ColBase{TableAlias: "nodes", ColName: "parent_id"}}
	treeIDCol := expr.IntColumn{ColBase: expr.ColBase{TableAlias: "tree", ColName: "id"}}

	anchor := query.Select(idCol, parentCol).
		From(ts.UsersT). // using any table as stand-in
		Where(idCol.EQ(1))

	recursive := query.Select(idCol, parentCol).
		From(ts.UsersT).
		InnerJoin(query.CTERef("tree"), idCol.EQCol(treeIDCol))

	sql, args := query.Select().
		WithRecursive("tree", anchor, recursive).
		From(query.CTERef("tree")).
		Build(dialect.Postgres)

	if !strings.Contains(sql, "WITH RECURSIVE") {
		t.Errorf("expected WITH RECURSIVE, got: %s", sql)
	}
	if !strings.Contains(sql, "UNION ALL") {
		t.Errorf("expected UNION ALL in recursive CTE body, got: %s", sql)
	}
	if !strings.HasPrefix(sql, "WITH RECURSIVE") {
		t.Errorf("expected query to start with WITH RECURSIVE, got: %s", sql)
	}
	_ = args
}

func TestWith_NonRecursive_UsesWITH(t *testing.T) {
	sub := query.Select(ts.UsersT.ID).From(ts.UsersT).Where(ts.UsersT.Enabled.IsTrue())
	sql, _ := query.Select().
		With("active", sub).
		From(query.CTERef("active")).
		Build(dialect.Postgres)
	if !strings.HasPrefix(sql, "WITH ") || strings.HasPrefix(sql, "WITH RECURSIVE") {
		t.Errorf("expected plain WITH, got: %s", sql)
	}
}

func TestWithRecursive_AndRegularCTE(t *testing.T) {
	// Mixed: one regular CTE + one recursive CTE → WITH RECURSIVE for the whole block.
	sub := query.Select(ts.UsersT.ID).From(ts.UsersT)
	anchor := query.Select(ts.UsersT.ID).From(ts.UsersT).Where(ts.UsersT.Enabled.IsTrue())
	recursive := query.Select(ts.UsersT.ID).From(ts.UsersT)

	sql, _ := query.Select().
		With("regular", sub).
		WithRecursive("tree", anchor, recursive).
		From(query.CTERef("tree")).
		Build(dialect.Postgres)

	if !strings.HasPrefix(sql, "WITH RECURSIVE") {
		t.Errorf("expected WITH RECURSIVE (any recursive CTE triggers it), got: %s", sql)
	}
}

// -------------------------------------------------------------------
// Dialect-gating tests (issue #178)
// -------------------------------------------------------------------

// noCTEDialect is a test-only dialect that reports SupportsCTE() = false.
type noCTEDialect struct{}

func (noCTEDialect) Name() string               { return "no_cte" }
func (noCTEDialect) Placeholder(_ int) string   { return "?" }
func (noCTEDialect) QuoteIdent(n string) string { return `"` + n + `"` }
func (noCTEDialect) SupportsReturning() bool    { return false }
func (noCTEDialect) UpsertStyle() dialect.UpsertStyle {
	return dialect.UpsertOnConflict
}
func (noCTEDialect) InsertIgnoreClause() string    { return "" }
func (noCTEDialect) SupportsCTE() bool             { return false }
func (noCTEDialect) SupportsWindowFunctions() bool { return true }
func (noCTEDialect) SupportsDistinctOn() bool      { return false }
func (noCTEDialect) SupportsForUpdate() bool       { return false }
func (noCTEDialect) SupportsForNoKeyUpdate() bool  { return false }
func (noCTEDialect) SupportsForShareOf() bool      { return false }
func (noCTEDialect) SupportsFullJoin() bool        { return false }
func (noCTEDialect) ForShareClause() string        { return "" }

// noWindowDialect is a test-only dialect that reports SupportsWindowFunctions() = false.
type noWindowDialect struct{ noCTEDialect }

func (noWindowDialect) SupportsCTE() bool             { return true }
func (noWindowDialect) SupportsWindowFunctions() bool { return false }

func TestCTE_DroppedWhenNotSupported(t *testing.T) {
	sub := query.Select(ts.UsersT.ID).From(ts.UsersT)
	sql, _ := query.Select(ts.UsersT.ID).
		With("recent", sub).
		From(query.CTERef("recent")).
		Build(noCTEDialect{})

	if strings.Contains(sql, "WITH") {
		t.Errorf("expected WITH clause to be dropped, got: %s", sql)
	}
	// The CTERef FROM reference is preserved as a plain table name; at runtime
	// the database will raise an unknown-table error — the intended fail-loud
	// behaviour rather than silently returning wrong rows.
	want := `SELECT "users"."id" FROM "recent"`
	if sql != want {
		t.Errorf("CTE dropped: want %q, got %q", want, sql)
	}
}

func TestCTE_RecursiveDroppedWhenNotSupported(t *testing.T) {
	anchor := query.Select(ts.UsersT.ID).From(ts.UsersT).Where(ts.UsersT.Enabled.IsTrue())
	recursive := query.Select(ts.UsersT.ID).From(ts.UsersT)
	sql, _ := query.Select(ts.UsersT.ID).
		WithRecursive("tree", anchor, recursive).
		From(query.CTERef("tree")).
		Build(noCTEDialect{})

	// The CTERef FROM reference is preserved as a plain table name; at runtime
	// the database will raise an unknown-table error — the intended fail-loud
	// behaviour rather than silently returning wrong rows.
	want := `SELECT "users"."id" FROM "tree"`
	if sql != want {
		t.Errorf("recursive CTE dropped: want %q, got %q", want, sql)
	}
}

func TestCTE_EmittedWhenSupported(t *testing.T) {
	sub := query.Select(ts.UsersT.ID).From(ts.UsersT)
	sql, _ := query.Select(ts.UsersT.ID).
		With("recent", sub).
		From(query.CTERef("recent")).
		Build(dialect.Postgres)

	if !strings.HasPrefix(sql, `WITH "recent" AS (`) {
		t.Errorf("expected WITH clause, got: %s", sql)
	}
}

func TestDistinctOn_PostgresRendered(t *testing.T) {
	sql, _ := query.Select(ts.UsersT.RealmID, ts.UsersT.Username).
		From(ts.UsersT).
		DistinctOn(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.RealmID.Asc(), ts.UsersT.CreatedAt.Desc()).
		Build(dialect.Postgres)

	want := `SELECT DISTINCT ON ("users"."realm_id") "users"."realm_id", "users"."username" FROM "users" ORDER BY "users"."realm_id" ASC, "users"."created_at" DESC`
	if sql != want {
		t.Errorf("SQL mismatch\n got:  %s\nwant: %s", sql, want)
	}
}

func TestDistinctOn_DegradesToDistinctOnUnsupportedDialect(t *testing.T) {
	// MySQL does not support DISTINCT ON — should degrade to SELECT DISTINCT.
	sql, _ := query.Select(ts.UsersT.RealmID, ts.UsersT.Username).
		From(ts.UsersT).
		DistinctOn(ts.UsersT.RealmID).
		Build(dialect.MySQL)

	if strings.Contains(sql, "DISTINCT ON") {
		t.Errorf("DISTINCT ON should be dropped for MySQL, got: %s", sql)
	}
	if !strings.Contains(sql, "SELECT DISTINCT") {
		t.Errorf("expected SELECT DISTINCT fallback, got: %s", sql)
	}
}

func TestDistinctOn_DegradesToDistinctForSQLite(t *testing.T) {
	// SQLite does not support DISTINCT ON — should degrade to SELECT DISTINCT.
	sql, _ := query.Select(ts.UsersT.RealmID, ts.UsersT.Username).
		From(ts.UsersT).
		DistinctOn(ts.UsersT.RealmID).
		Build(dialect.SQLite)

	if strings.Contains(sql, "DISTINCT ON") {
		t.Errorf("DISTINCT ON should be dropped for SQLite, got: %s", sql)
	}
	if !strings.Contains(sql, "SELECT DISTINCT") {
		t.Errorf("expected SELECT DISTINCT fallback, got: %s", sql)
	}
}

func TestDistinctOn_MultipleCols(t *testing.T) {
	sql, _ := query.Select(ts.UsersT.RealmID, ts.UsersT.Username, ts.UsersT.ID).
		From(ts.UsersT).
		DistinctOn(ts.UsersT.RealmID, ts.UsersT.Username).
		Build(dialect.Postgres)

	if !strings.Contains(sql, `DISTINCT ON ("users"."realm_id", "users"."username")`) {
		t.Errorf("expected DISTINCT ON with two cols, got: %s", sql)
	}
}

func TestWindowFunctions_DroppedWhenNotSupported(t *testing.T) {
	// Window functions should be silently removed from the SELECT list.
	sql, _ := query.Select(
		ts.UsersT.ID,
		expr.RowNumber().PartitionBy(ts.UsersT.RealmID).As("rn"),
	).From(ts.UsersT).Build(noWindowDialect{})

	if strings.Contains(sql, "ROW_NUMBER") {
		t.Errorf("expected ROW_NUMBER to be dropped, got: %s", sql)
	}
	if !strings.Contains(sql, `"users"."id"`) {
		t.Errorf("expected non-window column to remain, got: %s", sql)
	}
}

func TestWindowFunctions_AllDroppedFallsBackToStar(t *testing.T) {
	// When all selected columns are window functions and dialect drops them, fall back to *.
	sql, _ := query.Select(
		expr.RowNumber().As("rn"),
		expr.Rank().As("rnk"),
	).From(ts.UsersT).Build(noWindowDialect{})

	if !strings.Contains(sql, "SELECT *") {
		t.Errorf("expected SELECT * fallback when all window cols dropped, got: %s", sql)
	}
}

func TestWindowFunctions_EmittedWhenSupported(t *testing.T) {
	sql, _ := query.Select(
		ts.UsersT.ID,
		expr.RowNumber().PartitionBy(ts.UsersT.RealmID).As("rn"),
	).From(ts.UsersT).Build(dialect.Postgres)

	if !strings.Contains(sql, "ROW_NUMBER()") {
		t.Errorf("expected ROW_NUMBER in output, got: %s", sql)
	}
}

func TestFullJoin_DroppedOnMySQL(t *testing.T) {
	sql, _ := query.Select(ts.UsersT.ID, ts.RealmsT.ID).
		From(ts.UsersT).
		FullJoin(ts.RealmsT, ts.UsersT.RealmID.EQCol(ts.RealmsT.ID)).
		Build(dialect.MySQL)

	if strings.Contains(sql, "FULL JOIN") {
		t.Errorf("FULL JOIN should be dropped for MySQL, got: %s", sql)
	}
}

func TestFullJoin_DroppedOnSQLite(t *testing.T) {
	sql, _ := query.Select(ts.UsersT.ID).
		From(ts.UsersT).
		FullJoin(ts.RealmsT, ts.UsersT.RealmID.EQCol(ts.RealmsT.ID)).
		Build(dialect.SQLite)

	if strings.Contains(sql, "FULL JOIN") {
		t.Errorf("FULL JOIN should be dropped for SQLite, got: %s", sql)
	}
}

func TestFullJoin_EmittedOnPostgres(t *testing.T) {
	sql, _ := query.Select(ts.UsersT.ID, ts.RealmsT.ID).
		From(ts.UsersT).
		FullJoin(ts.RealmsT, ts.UsersT.RealmID.EQCol(ts.RealmsT.ID)).
		Build(dialect.Postgres)

	if !strings.Contains(sql, "FULL JOIN") {
		t.Errorf("expected FULL JOIN in PostgreSQL output, got: %s", sql)
	}
}

func TestFullJoin_MixedJoins_OnlyFullDropped(t *testing.T) {
	// Inner join should remain; full join should be dropped on MySQL.
	sql, _ := query.Select(ts.UsersT.ID).
		From(ts.UsersT).
		InnerJoin(ts.RealmsT, ts.UsersT.RealmID.EQCol(ts.RealmsT.ID)).
		FullJoin(ts.RealmsT, ts.UsersT.RealmID.EQCol(ts.RealmsT.ID)).
		Build(dialect.MySQL)

	if strings.Contains(sql, "FULL JOIN") {
		t.Errorf("FULL JOIN should be dropped for MySQL, got: %s", sql)
	}
	if !strings.Contains(sql, "INNER JOIN") {
		t.Errorf("INNER JOIN should remain, got: %s", sql)
	}
}

func TestWindowFunctions_AliasedColWrappingWindowExpr_DroppedWhenNotSupported(t *testing.T) {
	// expr.ColAs(windowExpr, alias) wraps a WindowExpr in an AliasedCol.
	// The window-function gate must unwrap AliasedCol to detect the inner WindowExpr
	// and drop it on dialects that do not support window functions.
	sql, _ := query.Select(
		ts.UsersT.ID,
		expr.ColAs(expr.RowNumber(), "rn"), // AliasedCol wrapping a WindowExpr
	).From(ts.UsersT).Build(noWindowDialect{})

	if strings.Contains(sql, "ROW_NUMBER") {
		t.Errorf("ColAs-wrapped WindowExpr should be dropped on no-window dialect, got: %s", sql)
	}
	if !strings.Contains(sql, `"users"."id"`) {
		t.Errorf("non-window column should remain after dropping ColAs-wrapped WindowExpr, got: %s", sql)
	}
}

func TestWindowFunctions_AliasedColWrappingWindowExpr_AllDroppedFallsBackToStar(t *testing.T) {
	// When all columns are ColAs-wrapped WindowExprs and the dialect drops them,
	// the query should fall back to SELECT * (same as bare WindowExpr).
	sql, _ := query.Select(
		expr.ColAs(expr.RowNumber(), "rn"),
		expr.ColAs(expr.Rank(), "rnk"),
	).From(ts.UsersT).Build(noWindowDialect{})

	if !strings.Contains(sql, "SELECT *") {
		t.Errorf("expected SELECT * fallback when all ColAs-wrapped window cols dropped, got: %s", sql)
	}
}

func TestDistinctOn_EmptyColsIsNoOp(t *testing.T) {
	// DistinctOn() with no arguments sets distinct=true but distinctOn stays empty.
	// On non-supporting dialects this should degrade to SELECT DISTINCT (not panic).
	sql, _ := query.Select(ts.UsersT.ID).
		From(ts.UsersT).
		DistinctOn(). // empty variadic
		Build(dialect.MySQL)

	if !strings.Contains(sql, "SELECT DISTINCT") {
		t.Errorf("empty DistinctOn() should degrade to SELECT DISTINCT on MySQL, got: %s", sql)
	}
	if strings.Contains(sql, "DISTINCT ON") {
		t.Errorf("DISTINCT ON must not appear for empty DistinctOn() on MySQL, got: %s", sql)
	}
}
