package kit_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	pg "github.com/grizzle-orm/grizzle/schema/pg"
	"github.com/grizzle-orm/grizzle/kit"
)

// --- Test schema fixtures ---

var realmsDef = pg.Table("realms",
	pg.C("id",           pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("name",         pg.Varchar(255).NotNull()),
	pg.C("display_name", pg.Varchar(255)),
	pg.C("enabled",      pg.Boolean().NotNull().Default(true)),
	pg.C("settings",     pg.JSONB().DefaultEmpty()),
	pg.C("created_at",   pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
	pg.C("updated_at",   pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
	return []pg.Constraint{
		pg.UniqueIndex("realms_name_idx").On(t.Col("name")).Build(),
		pg.Check("settings_size_check", "pg_column_size(settings) <= 65536"),
	}
})

var usersDef = pg.Table("users",
	pg.C("id",             pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("realm_id",       pg.UUID().NotNull()),
	pg.C("username",       pg.Varchar(255).NotNull()),
	pg.C("email",          pg.Varchar(255)),
	pg.C("enabled",        pg.Boolean().NotNull().Default(true)),
	pg.C("created_at",     pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
	pg.C("deleted_at",     pg.Timestamp().WithTimezone()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
	return []pg.Constraint{
		pg.UniqueIndex("users_realm_username_idx").
			On(t.Col("realm_id"), t.Col("username")).
			Where(pg.IsNull(t.Col("deleted_at"))).
			Build(),
		pg.Index("users_realm_id_idx").On(t.Col("realm_id")).Build(),
	}
})

// --- Snapshot tests ---

func TestFromDefs_BasicStructure(t *testing.T) {
	snap := kit.FromDefs(realmsDef, usersDef)
	if snap.Version == "" {
		t.Error("version should be set")
	}
	if len(snap.Tables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(snap.Tables))
	}
	if _, ok := snap.Tables["realms"]; !ok {
		t.Error("missing 'realms' table")
	}
	if _, ok := snap.Tables["users"]; !ok {
		t.Error("missing 'users' table")
	}
}

func TestSnapshotJSON_RoundTrip(t *testing.T) {
	snap := kit.FromDefs(realmsDef, usersDef)
	dir := t.TempDir()
	path := filepath.Join(dir, "schema.snapshot.json")

	if err := kit.SaveJSON(snap, path); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := kit.LoadJSON(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Tables) != len(snap.Tables) {
		t.Errorf("table count mismatch after round-trip: got %d, want %d", len(loaded.Tables), len(snap.Tables))
	}
	// Verify column count preserved.
	if snap.Tables["realms"] != nil && loaded.Tables["realms"] != nil {
		if len(loaded.Tables["realms"].Columns) != len(snap.Tables["realms"].Columns) {
			t.Errorf("realms column count mismatch: got %d, want %d",
				len(loaded.Tables["realms"].Columns), len(snap.Tables["realms"].Columns))
		}
	}
	// Verify JSON is readable.
	data, _ := json.MarshalIndent(loaded, "", "  ")
	t.Logf("snapshot JSON (%d bytes):\n%s", len(data), data)
}

// --- Diff tests ---

func TestDiff_EmptyToSchema(t *testing.T) {
	old := kit.EmptySnapshot()
	new := kit.FromDefs(realmsDef, usersDef)
	changes := kit.Diff(old, new)

	creates := countKind(changes, kit.ChangeCreateTable)
	if creates != 2 {
		t.Errorf("expected 2 CreateTable changes, got %d", creates)
	}
	// No other change kinds expected when going from empty → full schema.
	for _, c := range changes {
		if c.Kind != kit.ChangeCreateTable {
			t.Errorf("unexpected change kind %q for table %s", c.Kind, c.TableName)
		}
	}
}

func TestDiff_NoChange(t *testing.T) {
	snap := kit.FromDefs(realmsDef, usersDef)
	changes := kit.Diff(snap, snap)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for identical snapshots, got %d: %v", len(changes), changes)
	}
}

func TestDiff_AddColumn(t *testing.T) {
	// Old: realms without "description"
	oldDef := pg.Table("realms",
		pg.C("id",   pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("name", pg.Varchar(255).NotNull()),
	).Build()

	// New: realms with "description" added
	newDef := pg.Table("realms",
		pg.C("id",          pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("name",        pg.Varchar(255).NotNull()),
		pg.C("description", pg.Text()),
	).Build()

	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))
	adds := countKind(changes, kit.ChangeAddColumn)
	if adds != 1 {
		t.Errorf("expected 1 AddColumn, got %d: %v", adds, changes)
	}
	if changes[0].NewCol == nil || changes[0].NewCol.Name != "description" {
		t.Errorf("expected description column, got: %+v", changes[0].NewCol)
	}
}

func TestDiff_DropColumn(t *testing.T) {
	oldDef := pg.Table("realms",
		pg.C("id",          pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("name",        pg.Varchar(255).NotNull()),
		pg.C("description", pg.Text()),
	).Build()
	newDef := pg.Table("realms",
		pg.C("id",   pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("name", pg.Varchar(255).NotNull()),
	).Build()

	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))
	drops := countKind(changes, kit.ChangeDropColumn)
	if drops != 1 {
		t.Errorf("expected 1 DropColumn, got %d", drops)
	}
}

func TestDiff_DropTable(t *testing.T) {
	old := kit.FromDefs(realmsDef, usersDef)
	new := kit.FromDefs(realmsDef)
	changes := kit.Diff(old, new)
	drops := countKind(changes, kit.ChangeDropTable)
	if drops != 1 {
		t.Errorf("expected 1 DropTable, got %d", drops)
	}
}

func TestDiff_AlterColumnType(t *testing.T) {
	oldDef := pg.Table("t", pg.C("code", pg.Varchar(50).NotNull())).Build()
	newDef := pg.Table("t", pg.C("code", pg.Varchar(100).NotNull())).Build()
	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))
	alters := countKind(changes, kit.ChangeAlterColumnType)
	if alters != 1 {
		t.Errorf("expected 1 AlterColumnType, got %d: %v", alters, changes)
	}
}

func TestDiff_AddConstraint(t *testing.T) {
	oldDef := pg.Table("users",
		pg.C("id",    pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("email", pg.Varchar(255)),
	).Build()

	newDef := pg.Table("users",
		pg.C("id",    pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("email", pg.Varchar(255)),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.UniqueIndex("users_email_idx").On(t.Col("email")).Build(),
		}
	})

	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))
	adds := countKind(changes, kit.ChangeAddConstraint)
	if adds != 1 {
		t.Errorf("expected 1 AddConstraint, got %d", adds)
	}
}

// --- SQL generation tests ---

func TestGenerateCreateSQL_BasicTable(t *testing.T) {
	sql := kit.GenerateCreateSQL(realmsDef)
	t.Logf("CREATE SQL:\n%s", sql)

	checks := []string{
		`CREATE TABLE IF NOT EXISTS "realms"`,
		`"id" uuid`,
		`PRIMARY KEY`,
		`DEFAULT gen_random_uuid()`,
		`"name" varchar(255) NOT NULL`,
		`"display_name" varchar(255)`,
		`"enabled" boolean NOT NULL`,
		`DEFAULT true`,
		`"settings" jsonb`,
		`CONSTRAINT "settings_size_check" CHECK`,
		`CREATE UNIQUE INDEX "realms_name_idx" ON "realms"`,
	}
	for _, want := range checks {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %q in:\n%s", want, sql)
		}
	}
}

func TestGenerateCreateSQL_PartialIndex(t *testing.T) {
	sql := kit.GenerateCreateSQL(usersDef)
	t.Logf("Users SQL:\n%s", sql)

	if !strings.Contains(sql, `WHERE deleted_at IS NULL`) {
		t.Error("expected partial index WHERE clause")
	}
	if !strings.Contains(sql, `CREATE UNIQUE INDEX "users_realm_username_idx"`) {
		t.Error("expected unique index for realm+username")
	}
	if !strings.Contains(sql, `CREATE INDEX "users_realm_id_idx"`) {
		t.Error("expected non-unique index for realm_id")
	}
}

func TestGenerateChangeSQL_AddColumn(t *testing.T) {
	snap := kit.FromDefs(usersDef)
	col := pg.ColumnDef{Name: "phone", SQLType: "varchar(20)"}
	change := kit.Change{
		Kind:      kit.ChangeAddColumn,
		TableName: "users",
		NewCol:    &col,
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `ALTER TABLE "users" ADD COLUMN "phone" varchar(20)`
	if stmts[0] != want {
		t.Errorf("got:\n  %s\nwant:\n  %s", stmts[0], want)
	}
}

func TestGenerateChangeSQL_DropColumn(t *testing.T) {
	snap := kit.FromDefs(usersDef)
	col := pg.ColumnDef{Name: "email"}
	change := kit.Change{Kind: kit.ChangeDropColumn, TableName: "users", OldCol: &col}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], `DROP COLUMN "email"`) {
		t.Errorf("unexpected SQL: %s", stmts[0])
	}
}

// --- Helpers ---

func countKind(changes []kit.Change, kind kit.ChangeKind) int {
	n := 0
	for _, c := range changes {
		if c.Kind == kind {
			n++
		}
	}
	return n
}

// -------------------------------------------------------------------
// Migration history tests (pure logic, no live DB)
// -------------------------------------------------------------------

func TestChecksumSQL_Deterministic(t *testing.T) {
	stmts := []string{
		`CREATE TABLE "users" ("id" UUID PRIMARY KEY)`,
		`CREATE INDEX "users_email_idx" ON "users" ("email")`,
	}
	// Two calls with the same input must produce the same checksum.
	a := kit.ChecksumSQL(stmts)
	b := kit.ChecksumSQL(stmts)
	if a != b {
		t.Errorf("checksum not deterministic: %q != %q", a, b)
	}
	// Order matters — reversing changes the checksum.
	reversed := []string{stmts[1], stmts[0]}
	c := kit.ChecksumSQL(reversed)
	if a == c {
		t.Error("expected different checksum for different order")
	}
}

func TestChecksumSQL_Length(t *testing.T) {
	sum := kit.ChecksumSQL([]string{"SELECT 1"})
	if len(sum) != 64 { // SHA-256 = 32 bytes = 64 hex chars
		t.Errorf("expected 64 hex chars, got %d: %s", len(sum), sum)
	}
}

func TestDescribeChanges_Labels(t *testing.T) {
	changes := []kit.Change{
		{Kind: kit.ChangeCreateTable, TableName: "users"},
		{Kind: kit.ChangeCreateTable, TableName: "realms"},
		{Kind: kit.ChangeAddColumn, TableName: "posts", NewCol: &pg.ColumnDef{Name: "title"}},
	}
	desc := kit.DescribeChanges(changes)
	if !strings.Contains(desc, "create_table") {
		t.Errorf("expected create_table in description: %s", desc)
	}
	if !strings.Contains(desc, "add_column") {
		t.Errorf("expected add_column in description: %s", desc)
	}
	if !strings.Contains(desc, "users") {
		t.Errorf("expected users in description: %s", desc)
	}
}

// -------------------------------------------------------------------
// MySQL DDL generation tests
// -------------------------------------------------------------------

func TestMySQLCreateSQL_TypeMapping(t *testing.T) {
	t.Run("uuid maps to CHAR(36)", func(t *testing.T) {
		sql := kit.GenerateCreateSQLMySQL(realmsDef)
		if !strings.Contains(sql, "CHAR(36)") {
			t.Errorf("expected CHAR(36) for UUID in MySQL DDL, got:\n%s", sql)
		}
	})

	t.Run("boolean maps to TINYINT(1)", func(t *testing.T) {
		sql := kit.GenerateCreateSQLMySQL(realmsDef)
		if !strings.Contains(sql, "TINYINT(1)") {
			t.Errorf("expected TINYINT(1) for boolean in MySQL DDL, got:\n%s", sql)
		}
	})

	t.Run("jsonb maps to JSON", func(t *testing.T) {
		sql := kit.GenerateCreateSQLMySQL(realmsDef)
		if !strings.Contains(sql, "JSON") {
			t.Errorf("expected JSON for jsonb in MySQL DDL, got:\n%s", sql)
		}
	})

	t.Run("timestamptz maps to DATETIME", func(t *testing.T) {
		sql := kit.GenerateCreateSQLMySQL(realmsDef)
		if !strings.Contains(sql, "DATETIME") {
			t.Errorf("expected DATETIME for timestamptz in MySQL DDL, got:\n%s", sql)
		}
	})
}

func TestMySQLCreateSQL_Backticks(t *testing.T) {
	sql := kit.GenerateCreateSQLMySQL(realmsDef)
	if !strings.Contains(sql, "`realms`") {
		t.Errorf("expected backtick-quoted table name in MySQL DDL, got:\n%s", sql)
	}
	if strings.Contains(sql, `"realms"`) {
		t.Errorf("unexpected double-quoted name in MySQL DDL:\n%s", sql)
	}
}

func TestMySQLCreateSQL_Engine(t *testing.T) {
	sql := kit.GenerateCreateSQLMySQL(realmsDef)
	if !strings.Contains(sql, "ENGINE=InnoDB") {
		t.Errorf("expected ENGINE=InnoDB in MySQL DDL, got:\n%s", sql)
	}
}

func TestMySQLChangeSQL_AddColumn(t *testing.T) {
	snap := kit.FromDefs(realmsDef)
	newCol := pg.ColumnDef{Name: "slug", SQLType: "varchar(255)"}
	change := kit.Change{
		Kind:      kit.ChangeAddColumn,
		TableName: "realms",
		NewCol:    &newCol,
	}
	stmts := kit.GenerateChangeSQLMySQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], "ADD COLUMN") || !strings.Contains(stmts[0], "`slug`") {
		t.Errorf("unexpected MySQL ADD COLUMN: %s", stmts[0])
	}
}

func TestMySQLChangeSQL_DropIndex(t *testing.T) {
	snap := kit.FromDefs(realmsDef)
	change := kit.Change{
		Kind:      kit.ChangeDropConstraint,
		TableName: "realms",
		Constraint: &pg.Constraint{
			Kind: pg.KindUniqueIndex,
			Name: "realms_name_idx",
		},
	}
	stmts := kit.GenerateChangeSQLMySQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	// MySQL: DROP INDEX name ON table (not DROP INDEX IF EXISTS name)
	if !strings.Contains(stmts[0], "DROP INDEX") || !strings.Contains(stmts[0], "ON `realms`") {
		t.Errorf("unexpected MySQL DROP INDEX: %s", stmts[0])
	}
}

func TestMySQLChangeSQL_AlterColumnType(t *testing.T) {
	snap := kit.FromDefs(realmsDef)
	newCol := pg.ColumnDef{Name: "name", SQLType: "varchar(512)", NotNull: true}
	change := kit.Change{
		Kind:      kit.ChangeAlterColumnType,
		TableName: "realms",
		NewCol:    &newCol,
	}
	stmts := kit.GenerateChangeSQLMySQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	// MySQL uses MODIFY COLUMN, not ALTER COLUMN … TYPE
	if !strings.Contains(stmts[0], "MODIFY COLUMN") {
		t.Errorf("MySQL alter type should use MODIFY COLUMN: %s", stmts[0])
	}
}
