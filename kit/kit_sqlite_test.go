package kit_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sofired/grizzle/kit"
	pg "github.com/sofired/grizzle/schema/pg"
)

// ---------------------------------------------------------------------------
// SQLite DDL generation tests (no live DB required)
// ---------------------------------------------------------------------------

func TestSQLiteCreateSQL_TypeTranslations(t *testing.T) {
	tbl := pg.Table("things",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("name", pg.Varchar(128).NotNull()),
		pg.C("bio", pg.Text()),
		pg.C("enabled", pg.Boolean().NotNull().Default(true)),
		pg.C("score", pg.Numeric(10, 2)),
		pg.C("meta", pg.JSONB()),
		pg.C("created_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
		pg.C("seq", pg.Serial().PrimaryKey()),
	).Build()

	ddl := kit.GenerateCreateSQLSQLite(tbl)

	checks := []struct{ desc, want string }{
		{"table header", `CREATE TABLE IF NOT EXISTS "things"`},
		{"uuid col present", `"id" uuid`},
		{"varchar col present", `"name" varchar(128)`},
		{"boolean col present", `"enabled" boolean`},
		{"serial → INTEGER PRIMARY KEY AUTOINCREMENT", "INTEGER PRIMARY KEY AUTOINCREMENT"},
		{"now() → CURRENT_TIMESTAMP", "CURRENT_TIMESTAMP"},
		{"boolean default true → 1", "DEFAULT 1"},
	}
	for _, c := range checks {
		if !strings.Contains(ddl, c.want) {
			t.Errorf("%s: DDL missing %q\n---\n%s\n---", c.desc, c.want, ddl)
		}
	}
}

func TestSQLiteCreateSQL_Indexes(t *testing.T) {
	tbl := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("realm_id", pg.UUID().NotNull()),
		pg.C("email", pg.Varchar(255)),
		pg.C("deleted_at", pg.Timestamp().WithTimezone()),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.UniqueIndex("users_realm_email_idx").On(t.Col("realm_id"), t.Col("email")).Build(),
			pg.Index("users_realm_idx").On(t.Col("realm_id")).Build(),
			// Partial index — supported in SQLite.
			pg.UniqueIndex("users_active_email_idx").
				On(t.Col("email")).
				Where(pg.IsNull(t.Col("deleted_at"))).
				Build(),
		}
	})

	ddl := kit.GenerateCreateSQLSQLite(tbl)

	if !strings.Contains(ddl, `CREATE UNIQUE INDEX IF NOT EXISTS "users_realm_email_idx"`) {
		t.Errorf("missing unique index\n%s", ddl)
	}
	if !strings.Contains(ddl, `CREATE INDEX IF NOT EXISTS "users_realm_idx"`) {
		t.Errorf("missing regular index\n%s", ddl)
	}
	// Partial index WHERE clause should be preserved.
	if !strings.Contains(ddl, "WHERE") {
		t.Errorf("partial index WHERE clause missing\n%s", ddl)
	}
}

func TestSQLiteChangeSQL_AlterColumnType_EmitsComment(t *testing.T) {
	snap := kit.FromDefs(realmsDef)
	newCol := pg.ColumnDef{Name: "name", SQLType: "varchar(512)", NotNull: true}
	change := kit.Change{
		Kind:      kit.ChangeAlterColumnType,
		TableName: "realms",
		NewCol:    &newCol,
	}
	stmts := kit.GenerateChangeSQLSQLite(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.HasPrefix(strings.TrimSpace(stmts[0]), "--") {
		t.Errorf("expected a SQL comment for unsupported ALTER COLUMN, got: %s", stmts[0])
	}
}

func TestSQLiteChangeSQL_AddColumn(t *testing.T) {
	snap := kit.FromDefs(realmsDef)
	newCol := pg.ColumnDef{Name: "bio", SQLType: "text"}
	change := kit.Change{
		Kind:      kit.ChangeAddColumn,
		TableName: "realms",
		NewCol:    &newCol,
	}
	stmts := kit.GenerateChangeSQLSQLite(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "ALTER TABLE") || !strings.Contains(stmts[0], "ADD COLUMN") {
		t.Errorf("unexpected ADD COLUMN SQL: %s", stmts[0])
	}
}

// ---------------------------------------------------------------------------
// SQLite integration tests using in-memory database
// ---------------------------------------------------------------------------

func openSQLiteMemory(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite3: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLite_MigrateAndStatus(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()

	schema := []*pg.TableDef{
		pg.Table("realms",
			pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
			pg.C("name", pg.Varchar(255).NotNull()),
		).Build(),
		pg.Table("users",
			pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
			pg.C("realm_id", pg.UUID().NotNull()),
			pg.C("username", pg.Varchar(255).NotNull()),
		).Build(),
	}

	// First migrate: should create both tables.
	result, err := kit.MigrateSQLite(ctx, db, schema...)
	if err != nil {
		t.Fatalf("MigrateSQLite: %v", err)
	}
	if result.AlreadyCurrent {
		t.Error("expected changes on first migrate, got AlreadyCurrent")
	}
	if len(result.Changes) == 0 {
		t.Error("expected at least one change")
	}

	// Second migrate: should be a no-op.
	result2, err := kit.MigrateSQLite(ctx, db, schema...)
	if err != nil {
		t.Fatalf("second MigrateSQLite: %v", err)
	}
	if !result2.AlreadyCurrent {
		t.Errorf("expected AlreadyCurrent on second migrate, got %d change(s)", len(result2.Changes))
	}

	// Status: should show one applied migration and no pending changes.
	status, err := kit.StatusSQLite(ctx, db, schema...)
	if err != nil {
		t.Fatalf("StatusSQLite: %v", err)
	}
	if len(status.Applied) != 1 {
		t.Errorf("expected 1 applied migration, got %d", len(status.Applied))
	}
	if len(status.Pending) != 0 {
		t.Errorf("expected 0 pending changes, got %d", len(status.Pending))
	}
}

func TestSQLite_DryRun(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()

	schema := []*pg.TableDef{
		pg.Table("things",
			pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
			pg.C("name", pg.Varchar(255).NotNull()),
		).Build(),
	}

	result, err := kit.DryRunSQLite(ctx, db, schema...)
	if err != nil {
		t.Fatalf("DryRunSQLite: %v", err)
	}
	if len(result.SQL) == 0 {
		t.Error("expected SQL statements in dry-run result")
	}

	// Verify nothing was actually created.
	var count int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='things'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("check table existence: %v", err)
	}
	if count != 0 {
		t.Error("dry-run should not create tables")
	}
}

func TestSQLite_AddColumn_Migration(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()

	// Initial schema.
	v1 := []*pg.TableDef{
		pg.Table("users",
			pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
			pg.C("username", pg.Varchar(255).NotNull()),
		).Build(),
	}
	if _, err := kit.MigrateSQLite(ctx, db, v1...); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}

	// Add a column.
	v2 := []*pg.TableDef{
		pg.Table("users",
			pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
			pg.C("username", pg.Varchar(255).NotNull()),
			pg.C("email", pg.Varchar(255)),
		).Build(),
	}
	result, err := kit.MigrateSQLite(ctx, db, v2...)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if result.AlreadyCurrent {
		t.Error("expected changes for ADD COLUMN")
	}

	// Verify the column exists.
	rows, err := db.QueryContext(ctx, `PRAGMA table_info("users")`)
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, name)
	}
	found := false
	for _, c := range cols {
		if c == "email" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'email' column after migration, got: %v", cols)
	}
}
