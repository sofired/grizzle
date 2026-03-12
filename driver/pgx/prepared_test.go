package pgx_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	pgxdb "github.com/sofired/grizzle/driver/pgx"
	"github.com/sofired/grizzle/internal/testschema"
	"github.com/sofired/grizzle/query"
)

// TestPreparedSelect_SQLBuiltOnce verifies that RegisterSelect pre-builds
// the SQL correctly without needing a live database connection.
func TestPreparedSelect_SQLBuiltOnce(t *testing.T) {
	b := query.Select(
		testschema.UsersT.ID,
		testschema.UsersT.Username,
		testschema.UsersT.Email,
	).From(testschema.UsersT).
		Where(testschema.UsersT.DeletedAt.IsNull()).
		OrderBy(testschema.UsersT.CreatedAt.Desc())

	// NewRegistry(nil) is safe as long as PrepareAll is not called.
	reg := pgxdb.NewRegistry(nil)
	stmt := pgxdb.RegisterSelect[testschema.UserSelect](reg, "active_users", b)

	if stmt.Name() != "active_users" {
		t.Errorf("Name() = %q, want %q", stmt.Name(), "active_users")
	}
	sql := stmt.SQL()
	if sql == "" {
		t.Fatal("SQL() is empty")
	}
	t.Logf("pre-built SQL: %s", sql)

	for _, want := range []string{
		`"users"."id"`,
		`"users"."username"`,
		`"users"."email"`,
		`FROM "users"`,
		`"users"."deleted_at" IS NULL`,
		`ORDER BY "users"."created_at" DESC`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q\ngot: %s", want, sql)
		}
	}
}

func TestPreparedExec_SQLBuiltOnce(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := query.Update(testschema.UsersT).
		Set("enabled", false).
		Where(testschema.UsersT.ID.EQ(id))

	reg := pgxdb.NewRegistry(nil)
	stmt := pgxdb.RegisterExec(reg, "disable_user", b)

	if stmt.Name() != "disable_user" {
		t.Errorf("Name() = %q, want %q", stmt.Name(), "disable_user")
	}
	sql := stmt.SQL()
	if sql == "" {
		t.Fatal("SQL() is empty")
	}
	t.Logf("pre-built SQL: %s", sql)

	for _, want := range []string{
		`UPDATE "users"`,
		`SET "enabled"`,
		`WHERE`,
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q\ngot: %s", want, sql)
		}
	}
}

// TestPreparedSelect_SQLNotName guards against regression where execution used
// the statement name instead of the SQL string. Passing a name (e.g. "active_users")
// as the query to pgxpool fails on any connection that hasn't explicitly called
// Prepare — which is every connection except the one used by validateStatement.
func TestPreparedSelect_SQLNotName(t *testing.T) {
	b := query.Select(testschema.UsersT.ID).From(testschema.UsersT)

	reg := pgxdb.NewRegistry(nil)
	stmt := pgxdb.RegisterSelect[testschema.UserSelect](reg, "active_users", b)

	if stmt.SQL() == stmt.Name() {
		t.Errorf("SQL() must not equal Name(): got %q for both; "+
			"execution must use the SQL string, not the statement name", stmt.SQL())
	}
	if !strings.Contains(stmt.SQL(), "SELECT") {
		t.Errorf("SQL() does not look like SQL: %q", stmt.SQL())
	}
}

// TestPreparedExec_SQLNotName is the same guard for PreparedExec.
func TestPreparedExec_SQLNotName(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := query.Update(testschema.UsersT).
		Set("enabled", false).
		Where(testschema.UsersT.ID.EQ(id))

	reg := pgxdb.NewRegistry(nil)
	stmt := pgxdb.RegisterExec(reg, "disable_user", b)

	if stmt.SQL() == stmt.Name() {
		t.Errorf("SQL() must not equal Name(): got %q for both; "+
			"execution must use the SQL string, not the statement name", stmt.SQL())
	}
	if !strings.Contains(stmt.SQL(), "UPDATE") {
		t.Errorf("SQL() does not look like SQL: %q", stmt.SQL())
	}
}

func TestRegistry_MultipleStatements(t *testing.T) {
	reg := pgxdb.NewRegistry(nil)

	s1 := pgxdb.RegisterSelect[testschema.UserSelect](reg, "all_users",
		query.Select(testschema.UsersT.ID).From(testschema.UsersT))

	s2 := pgxdb.RegisterSelect[testschema.RealmSelect](reg, "all_realms",
		query.Select(testschema.RealmsT.ID).From(testschema.RealmsT))

	if s1.Name() != "all_users" {
		t.Errorf("s1.Name() = %q", s1.Name())
	}
	if s2.Name() != "all_realms" {
		t.Errorf("s2.Name() = %q", s2.Name())
	}
	if s1.SQL() == s2.SQL() {
		t.Error("two different queries produced identical SQL")
	}
}
