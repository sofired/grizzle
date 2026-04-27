package kit_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofired/grizzle/kit"
	pg "github.com/sofired/grizzle/schema/pg"
)

// --- Test schema fixtures ---

var realmsDef = pg.Table("realms",
	pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("name", pg.Varchar(255).NotNull()),
	pg.C("display_name", pg.Varchar(255)),
	pg.C("enabled", pg.Boolean().NotNull().Default(true)),
	pg.C("settings", pg.JSONB().DefaultEmpty()),
	pg.C("created_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
	pg.C("updated_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
	return []pg.Constraint{
		pg.UniqueIndex("realms_name_idx").On(t.Col("name")).Build(),
		pg.Check("settings_size_check", "pg_column_size(settings) <= 65536"),
	}
})

var usersDef = pg.Table("users",
	pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("realm_id", pg.UUID().NotNull()),
	pg.C("username", pg.Varchar(255).NotNull()),
	pg.C("email", pg.Varchar(255)),
	pg.C("enabled", pg.Boolean().NotNull().Default(true)),
	pg.C("created_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
	pg.C("deleted_at", pg.Timestamp().WithTimezone()),
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
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("name", pg.Varchar(255).NotNull()),
	).Build()

	// New: realms with "description" added
	newDef := pg.Table("realms",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("name", pg.Varchar(255).NotNull()),
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
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("name", pg.Varchar(255).NotNull()),
		pg.C("description", pg.Text()),
	).Build()
	newDef := pg.Table("realms",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
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
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("email", pg.Varchar(255)),
	).Build()

	newDef := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
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

// -------------------------------------------------------------------
// ChangeRenameColumn — all three dialects
// -------------------------------------------------------------------

func makeRenameChange() kit.Change {
	old := pg.ColumnDef{Name: "username", SQLType: "varchar(255)", NotNull: true}
	newCol := pg.ColumnDef{Name: "login_name", SQLType: "varchar(255)", NotNull: true}
	return kit.Change{
		Kind:      kit.ChangeRenameColumn,
		TableName: "users",
		OldCol:    &old,
		NewCol:    &newCol,
	}
}

func TestRenameColumn_Postgres(t *testing.T) {
	snap := kit.FromDefs(usersDef)
	stmts := kit.GenerateChangeSQL(snap, makeRenameChange())
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	want := `ALTER TABLE "users" RENAME COLUMN "username" TO "login_name"`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestRenameColumn_MySQL(t *testing.T) {
	snap := kit.FromDefs(usersDef)
	stmts := kit.GenerateChangeSQLMySQL(snap, makeRenameChange())
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	want := "ALTER TABLE `users` RENAME COLUMN `username` TO `login_name`"
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestRenameColumn_SQLite(t *testing.T) {
	snap := kit.FromDefs(usersDef)
	stmts := kit.GenerateChangeSQLSQLite(snap, makeRenameChange())
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	want := `ALTER TABLE "users" RENAME COLUMN "username" TO "login_name"`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestRenameColumn_NilGuard(t *testing.T) {
	snap := kit.FromDefs(usersDef)
	// Missing OldCol / NewCol must not panic.
	c := kit.Change{Kind: kit.ChangeRenameColumn, TableName: "users"}
	if stmts := kit.GenerateChangeSQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil cols, got %v", stmts)
	}
}

// TestRenameColumn_SQLGen_Postgres verifies that GenerateChangeSQL emits the
// correct PostgreSQL RENAME COLUMN DDL for a ChangeRenameColumn change.
func TestRenameColumn_SQLGen_Postgres(t *testing.T) {
	snap := kit.FromDefs(usersDef)
	old := pg.ColumnDef{Name: "username", SQLType: "varchar(255)", NotNull: true}
	newCol := pg.ColumnDef{Name: "handle", SQLType: "varchar(255)", NotNull: true}
	change := kit.Change{
		Kind:      kit.ChangeRenameColumn,
		TableName: "users",
		OldCol:    &old,
		NewCol:    &newCol,
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	want := `ALTER TABLE "users" RENAME COLUMN "username" TO "handle"`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

// TestRenameColumn_SQLGen_MySQL verifies that GenerateChangeSQLMySQL emits the
// correct MySQL RENAME COLUMN DDL for a ChangeRenameColumn change.
func TestRenameColumn_SQLGen_MySQL(t *testing.T) {
	snap := kit.FromDefs(usersDef)
	old := pg.ColumnDef{Name: "username", SQLType: "varchar(255)", NotNull: true}
	newCol := pg.ColumnDef{Name: "handle", SQLType: "varchar(255)", NotNull: true}
	change := kit.Change{
		Kind:      kit.ChangeRenameColumn,
		TableName: "users",
		OldCol:    &old,
		NewCol:    &newCol,
	}
	stmts := kit.GenerateChangeSQLMySQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	want := "ALTER TABLE `users` RENAME COLUMN `username` TO `handle`"
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

// TestRenameColumn_SQLGen_SQLite verifies that GenerateChangeSQLSQLite emits
// the correct SQLite RENAME COLUMN DDL for a ChangeRenameColumn change.
func TestRenameColumn_SQLGen_SQLite(t *testing.T) {
	snap := kit.FromDefs(usersDef)
	old := pg.ColumnDef{Name: "username", SQLType: "varchar(255)", NotNull: true}
	newCol := pg.ColumnDef{Name: "handle", SQLType: "varchar(255)", NotNull: true}
	change := kit.Change{
		Kind:      kit.ChangeRenameColumn,
		TableName: "users",
		OldCol:    &old,
		NewCol:    &newCol,
	}
	stmts := kit.GenerateChangeSQLSQLite(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	want := `ALTER TABLE "users" RENAME COLUMN "username" TO "handle"`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

// -------------------------------------------------------------------
// Fix #8 — Diff() deterministic output
// -------------------------------------------------------------------

func TestDiff_Deterministic(t *testing.T) {
	// Run Diff many times and verify the output order is always the same.
	old := kit.EmptySnapshot()
	new := kit.FromDefs(realmsDef, usersDef)
	first := kit.Diff(old, new)
	for i := 0; i < 20; i++ {
		got := kit.Diff(old, new)
		if len(got) != len(first) {
			t.Fatalf("run %d: length mismatch: got %d, want %d", i, len(got), len(first))
		}
		for j := range first {
			if got[j].Kind != first[j].Kind || got[j].TableName != first[j].TableName {
				t.Errorf("run %d, change[%d]: got {%s %s}, want {%s %s}",
					i, j, got[j].Kind, got[j].TableName, first[j].Kind, first[j].TableName)
			}
		}
	}
}

// -------------------------------------------------------------------
// Fix #6 — constraintMap key collision
// -------------------------------------------------------------------

func TestDiff_ConstraintCollision_UnnamedConstraints(t *testing.T) {
	// Two constraints with the same Name field (e.g. both empty) on different
	// column sets must not collide in the constraint map.
	oldDef := pg.Table("t",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("a", pg.Varchar(50)),
		pg.C("b", pg.Varchar(50)),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.UniqueIndex("").On(t.Col("a")).Build(),
		}
	})
	newDef := pg.Table("t",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("a", pg.Varchar(50)),
		pg.C("b", pg.Varchar(50)),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.UniqueIndex("").On(t.Col("a")).Build(),
			pg.UniqueIndex("").On(t.Col("b")).Build(),
		}
	})
	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))
	adds := countKind(changes, kit.ChangeAddConstraint)
	if adds != 1 {
		t.Errorf("expected 1 AddConstraint for the new unnamed unique index, got %d: %v", adds, changes)
	}
}

// -------------------------------------------------------------------
// Fix #9 — constraintsEqual ignores FK fields
// -------------------------------------------------------------------

func TestDiff_FK_OnDeleteChange_DetectedAsChange(t *testing.T) {
	// Change FK ON DELETE action — must be detected as a drop+re-add.
	oldDef := pg.Table("posts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("user_id", pg.UUID().NotNull()),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.ForeignKey("posts_user_fk").
				From(t.Col("user_id")).
				References("users", "id").
				OnDelete(pg.FKActionNoAction).
				Build(),
		}
	})
	newDef := pg.Table("posts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("user_id", pg.UUID().NotNull()),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.ForeignKey("posts_user_fk").
				From(t.Col("user_id")).
				References("users", "id").
				OnDelete(pg.FKActionCascade). // changed
				Build(),
		}
	})
	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))
	drops := countKind(changes, kit.ChangeDropConstraint)
	adds := countKind(changes, kit.ChangeAddConstraint)
	if drops != 1 || adds != 1 {
		t.Errorf("expected 1 drop + 1 add for FK ON DELETE change, got %d drops %d adds: %v", drops, adds, changes)
	}
}

func TestDiff_FK_RefTableChange_DetectedAsChange(t *testing.T) {
	oldDef := pg.Table("posts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("author_id", pg.UUID().NotNull()),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.ForeignKey("posts_author_fk").
				From(t.Col("author_id")).
				References("users", "id").
				Build(),
		}
	})
	newDef := pg.Table("posts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("author_id", pg.UUID().NotNull()),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.ForeignKey("posts_author_fk").
				From(t.Col("author_id")).
				References("admins", "id"). // changed target table
				Build(),
		}
	})
	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))
	drops := countKind(changes, kit.ChangeDropConstraint)
	adds := countKind(changes, kit.ChangeAddConstraint)
	if drops != 1 || adds != 1 {
		t.Errorf("expected 1 drop + 1 add for FK table change, got %d drops %d adds: %v", drops, adds, changes)
	}
}

func TestDiff_FK_OnUpdateChange_DetectedAsChange(t *testing.T) {
	// Change FK ON UPDATE action — must be detected as a drop+re-add.
	oldDef := pg.Table("posts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("user_id", pg.UUID().NotNull()),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.ForeignKey("posts_user_fk").
				From(t.Col("user_id")).
				References("users", "id").
				OnUpdate(pg.FKActionNoAction).
				Build(),
		}
	})
	newDef := pg.Table("posts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("user_id", pg.UUID().NotNull()),
	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
		return []pg.Constraint{
			pg.ForeignKey("posts_user_fk").
				From(t.Col("user_id")).
				References("users", "id").
				OnUpdate(pg.FKActionRestrict). // changed
				Build(),
		}
	})
	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))
	drops := countKind(changes, kit.ChangeDropConstraint)
	adds := countKind(changes, kit.ChangeAddConstraint)
	if drops != 1 || adds != 1 {
		t.Errorf("expected 1 drop + 1 add for FK ON UPDATE change, got %d drops %d adds: %v", drops, adds, changes)
	}
}

// -------------------------------------------------------------------
// Issue #43 — Rename detection: table renames
// -------------------------------------------------------------------

func TestDiff_TableRename_DetectedAsRename(t *testing.T) {
	// Old snapshot has "accounts" table.
	oldDef := pg.Table("accounts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("name", pg.Varchar(255).NotNull()),
	).Build()

	// New snapshot declares "users" with PreviousName = "accounts".
	newDef := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("name", pg.Varchar(255).NotNull()),
	).RenamedFrom("accounts").Build()

	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))

	renames := countKind(changes, kit.ChangeRenameTable)
	drops := countKind(changes, kit.ChangeDropTable)
	creates := countKind(changes, kit.ChangeCreateTable)

	if renames != 1 {
		t.Errorf("expected 1 RenameTable, got %d: %v", renames, changes)
	}
	if drops != 0 {
		t.Errorf("expected 0 DropTable (no data loss), got %d: %v", drops, changes)
	}
	if creates != 0 {
		t.Errorf("expected 0 CreateTable (no data loss), got %d: %v", creates, changes)
	}

	// Verify the rename change carries correct old and new names.
	var renameChange kit.Change
	for _, c := range changes {
		if c.Kind == kit.ChangeRenameTable {
			renameChange = c
			break
		}
	}
	if renameChange.TableName != "accounts" {
		t.Errorf("expected TableName=accounts (old), got %q", renameChange.TableName)
	}
	if renameChange.RenameTarget != "users" {
		t.Errorf("expected RenameTarget=users (new), got %q", renameChange.RenameTarget)
	}
}

func TestDiff_TableRename_SQLGen_Postgres(t *testing.T) {
	snap := kit.FromDefs(pg.Table("accounts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build())

	change := kit.Change{
		Kind:         kit.ChangeRenameTable,
		TableName:    "accounts",
		RenameTarget: "users",
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	want := `ALTER TABLE "accounts" RENAME TO "users"`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestDiff_TableRename_SQLGen_MySQL(t *testing.T) {
	snap := kit.FromDefs(pg.Table("accounts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build())

	change := kit.Change{
		Kind:         kit.ChangeRenameTable,
		TableName:    "accounts",
		RenameTarget: "users",
	}
	stmts := kit.GenerateChangeSQLMySQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	want := "RENAME TABLE `accounts` TO `users`"
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestDiff_TableRename_SQLGen_SQLite(t *testing.T) {
	snap := kit.FromDefs(pg.Table("accounts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build())

	change := kit.Change{
		Kind:         kit.ChangeRenameTable,
		TableName:    "accounts",
		RenameTarget: "users",
	}
	stmts := kit.GenerateChangeSQLSQLite(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	want := `ALTER TABLE "accounts" RENAME TO "users"`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestDiff_TableRename_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	// Missing RenameTarget must not panic and returns nil.
	c := kit.Change{Kind: kit.ChangeRenameTable, TableName: "accounts"}
	if stmts := kit.GenerateChangeSQL(snap, c); stmts != nil {
		t.Errorf("expected nil for empty RenameTarget, got %v", stmts)
	}
}

func TestDiff_TableRename_UnrelatedDropCreateUnaffected(t *testing.T) {
	// Renaming one table must not suppress an unrelated drop or create.
	oldA := pg.Table("accounts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build()
	oldB := pg.Table("orders",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build()

	// "accounts" is renamed to "users"; "orders" is dropped; "products" is added.
	newA := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).RenamedFrom("accounts").Build()
	newC := pg.Table("products",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build()

	changes := kit.Diff(kit.FromDefs(oldA, oldB), kit.FromDefs(newA, newC))

	renames := countKind(changes, kit.ChangeRenameTable)
	drops := countKind(changes, kit.ChangeDropTable)
	creates := countKind(changes, kit.ChangeCreateTable)

	if renames != 1 {
		t.Errorf("expected 1 RenameTable, got %d: %v", renames, changes)
	}
	if drops != 1 {
		t.Errorf("expected 1 DropTable for orders, got %d: %v", drops, changes)
	}
	if creates != 1 {
		t.Errorf("expected 1 CreateTable for products, got %d: %v", creates, changes)
	}
}

// -------------------------------------------------------------------
// Issue #43 — Rename detection: column renames
// -------------------------------------------------------------------

func TestDiff_ColumnRename_DetectedAsRename(t *testing.T) {
	oldDef := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("username", pg.Varchar(255).NotNull()),
	).Build()

	newDef := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("login_name", pg.Varchar(255).NotNull().RenamedFrom("username")),
	).Build()

	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))

	renames := countKind(changes, kit.ChangeRenameColumn)
	drops := countKind(changes, kit.ChangeDropColumn)
	adds := countKind(changes, kit.ChangeAddColumn)

	if renames != 1 {
		t.Errorf("expected 1 RenameColumn, got %d: %v", renames, changes)
	}
	if drops != 0 {
		t.Errorf("expected 0 DropColumn (no data loss), got %d: %v", drops, changes)
	}
	if adds != 0 {
		t.Errorf("expected 0 AddColumn (no data loss), got %d: %v", adds, changes)
	}

	// Verify the rename change carries the correct old and new column names.
	var renameChange kit.Change
	for _, c := range changes {
		if c.Kind == kit.ChangeRenameColumn {
			renameChange = c
			break
		}
	}
	if renameChange.OldCol == nil || renameChange.OldCol.Name != "username" {
		t.Errorf("expected OldCol.Name=username, got: %+v", renameChange.OldCol)
	}
	if renameChange.NewCol == nil || renameChange.NewCol.Name != "login_name" {
		t.Errorf("expected NewCol.Name=login_name, got: %+v", renameChange.NewCol)
	}
}

// TestDiff_Phase1_RenamesBeforeCreates verifies that rename changes are always
// emitted before create-table changes, regardless of alphabetical ordering of
// the new table names. This ensures FK creation against the renamed table name
// is safe.
func TestDiff_Phase1_RenamesBeforeCreates(t *testing.T) {
	// "accounts" (old) renamed to "users" (new); "aardvark" (new) is a brand new
	// table. Alphabetically "aardvark" sorts before "users", so without explicit
	// ordering the CREATE would appear before the RENAME.
	oldDef := pg.Table("accounts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build()

	newAardvark := pg.Table("aardvark",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build()

	newUsers := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).RenamedFrom("accounts").Build()

	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newAardvark, newUsers))

	if len(changes) < 2 {
		t.Fatalf("expected at least 2 changes, got %d: %v", len(changes), changes)
	}
	if changes[0].Kind != kit.ChangeRenameTable {
		t.Errorf("expected first change to be RenameTable, got %s", changes[0].Kind)
	}
	if changes[1].Kind != kit.ChangeCreateTable {
		t.Errorf("expected second change to be CreateTable, got %s", changes[1].Kind)
	}
}

// TestDiff_ColumnRename_WithTypeChange verifies that when a column is renamed
// and its type (or nullability/default) also changes, both the ChangeRenameColumn
// and the appropriate AlterColumn* changes are emitted.
func TestDiff_ColumnRename_WithTypeChange(t *testing.T) {
	oldDef := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("username", pg.Varchar(255).NotNull()),
	).Build()

	// login_name was username; type widened and nullability relaxed.
	newDef := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("login_name", pg.Varchar(512).RenamedFrom("username")),
	).Build()

	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))

	renames := countKind(changes, kit.ChangeRenameColumn)
	typeAlters := countKind(changes, kit.ChangeAlterColumnType)
	nullAlters := countKind(changes, kit.ChangeAlterColumnNull)

	if renames != 1 {
		t.Errorf("expected 1 RenameColumn, got %d: %v", renames, changes)
	}
	if typeAlters != 1 {
		t.Errorf("expected 1 AlterColumnType (varchar(255)→varchar(512)), got %d: %v", typeAlters, changes)
	}
	if nullAlters != 1 {
		t.Errorf("expected 1 AlterColumnNull (NOT NULL→nullable), got %d: %v", nullAlters, changes)
	}
	// Rename must come before the alter changes.
	firstRename := -1
	for i, c := range changes {
		if c.Kind == kit.ChangeRenameColumn {
			firstRename = i
			break
		}
	}
	for _, c := range changes[firstRename+1:] {
		if c.Kind == kit.ChangeAlterColumnType || c.Kind == kit.ChangeAlterColumnNull {
			if c.NewCol == nil || c.NewCol.Name != "login_name" {
				t.Errorf("AlterColumn after rename should target new name 'login_name', got: %+v", c.NewCol)
			}
		}
	}
}

// TestRenameTable_SQLGen_SchemaQualified_Postgres verifies that PostgreSQL
// RENAME TO uses only the unqualified new name even when RenameTarget is
// schema-qualified.
func TestRenameTable_SQLGen_SchemaQualified_Postgres(t *testing.T) {
	snap := kit.EmptySnapshot()
	change := kit.Change{
		Kind:         kit.ChangeRenameTable,
		TableName:    "public.accounts",
		RenameTarget: "public.users",
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	// RENAME TO must carry only the unqualified target name.
	want := `ALTER TABLE "public"."accounts" RENAME TO "users"`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

// TestRenameTable_SQLGen_CrossSchema_Postgres verifies that a PostgreSQL rename
// spanning two schemas emits RENAME TO (within source schema) followed by
// SET SCHEMA, so the table lands in the target schema rather than silently
// staying in the source schema under the new name.
func TestRenameTable_SQLGen_CrossSchema_Postgres(t *testing.T) {
	snap := kit.EmptySnapshot()
	change := kit.Change{
		Kind:         kit.ChangeRenameTable,
		TableName:    "auth.accounts",
		RenameTarget: "public.users",
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements for cross-schema rename, got %d: %v", len(stmts), stmts)
	}
	wantRename := `ALTER TABLE "auth"."accounts" RENAME TO "users"`
	if stmts[0] != wantRename {
		t.Errorf("stmt[0] got:  %s\nwant: %s", stmts[0], wantRename)
	}
	wantSetSchema := `ALTER TABLE "auth"."users" SET SCHEMA "public"`
	if stmts[1] != wantSetSchema {
		t.Errorf("stmt[1] got:  %s\nwant: %s", stmts[1], wantSetSchema)
	}
}

// TestRenameTable_SQLGen_SchemaQualified_MySQL verifies that MySQL RENAME TABLE
// preserves the schema qualifier on both sides when the TableName and
// RenameTarget are schema-qualified.
func TestRenameTable_SQLGen_SchemaQualified_MySQL(t *testing.T) {
	snap := kit.EmptySnapshot()
	change := kit.Change{
		Kind:         kit.ChangeRenameTable,
		TableName:    "public.accounts",
		RenameTarget: "public.users",
	}
	stmts := kit.GenerateChangeSQLMySQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	// MySQL RENAME TABLE preserves the schema on both source and target.
	want := "RENAME TABLE `public`.`accounts` TO `public`.`users`"
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

// TestRenameTable_SQLGen_SchemaQualified_SQLite verifies that SQLite RENAME TO
// uses only the unqualified names even when TableName/RenameTarget are qualified.
func TestRenameTable_SQLGen_SchemaQualified_SQLite(t *testing.T) {
	snap := kit.EmptySnapshot()
	change := kit.Change{
		Kind:         kit.ChangeRenameTable,
		TableName:    "main.accounts",
		RenameTarget: "main.users",
	}
	stmts := kit.GenerateChangeSQLSQLite(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	want := `ALTER TABLE "accounts" RENAME TO "users"`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestDiff_ColumnRename_UnrelatedDropAddUnaffected(t *testing.T) {
	// Renaming one column must not suppress unrelated add/drop for other columns.
	oldDef := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("username", pg.Varchar(255).NotNull()),
		pg.C("bio", pg.Text()),
	).Build()

	newDef := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("login_name", pg.Varchar(255).NotNull().RenamedFrom("username")),
		pg.C("email", pg.Varchar(255)), // new column, not a rename
		// "bio" is dropped
	).Build()

	changes := kit.Diff(kit.FromDefs(oldDef), kit.FromDefs(newDef))

	renames := countKind(changes, kit.ChangeRenameColumn)
	drops := countKind(changes, kit.ChangeDropColumn)
	adds := countKind(changes, kit.ChangeAddColumn)

	if renames != 1 {
		t.Errorf("expected 1 RenameColumn, got %d: %v", renames, changes)
	}
	if drops != 1 {
		t.Errorf("expected 1 DropColumn for bio, got %d: %v", drops, changes)
	}
	if adds != 1 {
		t.Errorf("expected 1 AddColumn for email, got %d: %v", adds, changes)
	}
}
