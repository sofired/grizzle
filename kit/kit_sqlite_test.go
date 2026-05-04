package kit_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
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
		// Canonical PG types must be translated to SQLite-native types.
		{"uuid → TEXT", `"id" TEXT`},
		{"varchar → TEXT", `"name" TEXT`},
		{"text → TEXT", `"bio" TEXT`},
		{"boolean → INTEGER", `"enabled" INTEGER`},
		{"numeric → NUMERIC", `"score" NUMERIC`},
		{"jsonb → TEXT", `"meta" TEXT`},
		{"timestamptz → TEXT", `"created_at" TEXT`},
		{"serial → INTEGER PRIMARY KEY AUTOINCREMENT", "INTEGER PRIMARY KEY AUTOINCREMENT"},
		// Default expressions must also be translated.
		{"now() → CURRENT_TIMESTAMP", "CURRENT_TIMESTAMP"},
		{"boolean default true → 1", "DEFAULT 1"},
		// Canonical type names must NOT appear in DDL.
	}
	for _, c := range checks {
		if !strings.Contains(ddl, c.want) {
			t.Errorf("%s: DDL missing %q\n---\n%s\n---", c.desc, c.want, ddl)
		}
	}

	// Ensure no raw canonical PostgreSQL type names leak into the DDL.
	// Strip known-good occurrences of "timestamp" within "CURRENT_TIMESTAMP" before
	// checking, so that the forbidden-word scan doesn't false-positive on it.
	ddlForCheck := strings.ReplaceAll(strings.ToLower(ddl), "current_timestamp", "")
	forbidden := []string{"uuid", "boolean", "timestamptz", "timestamp", "jsonb", "varchar", "bigint"}
	for _, f := range forbidden {
		if strings.Contains(ddlForCheck, f) {
			t.Errorf("canonical type %q must not appear in SQLite DDL\n---\n%s\n---", f, ddl)
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
		Kind:       kit.ChangeAlterColumnType,
		ObjectName: "realms",
		NewCol:     &newCol,
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
		Kind:       kit.ChangeAddColumn,
		ObjectName: "realms",
		NewCol:     &newCol,
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
// SQLite integration tests using in-memory database (file-based workflow)
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

// writeMigrationFile writes content to <dir>/<name>.sql and returns the path.
func writeMigrationFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write migration file %s: %v", name, err)
	}
	return path
}

func TestSQLite_MigrateAndStatus(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	// Write migration files.
	writeMigrationFile(t, dir, "0001_create_realms.sql",
		`CREATE TABLE "realms" ("id" TEXT PRIMARY KEY, "name" TEXT NOT NULL)`)
	writeMigrationFile(t, dir, "0002_create_users.sql",
		`CREATE TABLE "users" ("id" TEXT PRIMARY KEY, "realm_id" TEXT NOT NULL, "username" TEXT NOT NULL)`)

	opts := kit.MigrateOptions{MigrationsDir: dir}

	// First migrate: should apply both files.
	result, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("MigrateSQLite: %v", err)
	}
	if result.AlreadyCurrent {
		t.Error("expected changes on first migrate, got AlreadyCurrent")
	}
	if len(result.Applied) != 2 {
		t.Errorf("expected 2 applied migrations, got %d", len(result.Applied))
	}

	// Second migrate: should be a no-op.
	result2, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("second MigrateSQLite: %v", err)
	}
	if !result2.AlreadyCurrent {
		t.Errorf("expected AlreadyCurrent on second migrate, got %d applied", len(result2.Applied))
	}

	// Status: should show two applied migrations and no pending.
	status, err := kit.StatusSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("StatusSQLite: %v", err)
	}
	// Count only tag-based (file-based) rows.
	var taggedRows int
	for _, r := range status.Applied {
		if r.Tag != "" {
			taggedRows++
		}
	}
	if taggedRows != 2 {
		t.Errorf("expected 2 applied file-based migrations, got %d", taggedRows)
	}
	if len(status.Pending) != 0 {
		t.Errorf("expected 0 pending migrations, got %d", len(status.Pending))
	}
}

func TestSQLite_IdempotentOnSecondRun(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_init.sql",
		`CREATE TABLE "things" ("id" TEXT PRIMARY KEY, "name" TEXT NOT NULL)`)

	opts := kit.MigrateOptions{MigrationsDir: dir}

	if _, err := kit.MigrateSQLite(ctx, db, opts); err != nil {
		t.Fatalf("first migrate: %v", err)
	}

	result, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if !result.AlreadyCurrent {
		t.Error("expected AlreadyCurrent on second run")
	}
}

func TestSQLite_PendingNewFile(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_create_things.sql",
		`CREATE TABLE "things" ("id" TEXT PRIMARY KEY, "name" TEXT NOT NULL)`)

	opts := kit.MigrateOptions{MigrationsDir: dir}

	if _, err := kit.MigrateSQLite(ctx, db, opts); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}

	// Add a second migration file.
	writeMigrationFile(t, dir, "0002_add_email.sql",
		`ALTER TABLE "things" ADD COLUMN "email" TEXT`)

	result, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if result.AlreadyCurrent {
		t.Error("expected new migration to be applied")
	}
	if len(result.Applied) != 1 {
		t.Errorf("expected 1 applied migration, got %d", len(result.Applied))
	}
	if result.Applied[0].Tag != "0002_add_email" {
		t.Errorf("expected tag 0002_add_email, got %s", result.Applied[0].Tag)
	}
}

func TestSQLite_Baseline(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	// Simulate existing deployment: create the table manually (as if old migrate did it).
	if _, err := db.ExecContext(ctx, `CREATE TABLE "things" ("id" TEXT PRIMARY KEY, "name" TEXT NOT NULL)`); err != nil {
		t.Fatalf("create things: %v", err)
	}

	writeMigrationFile(t, dir, "0001_create_things.sql",
		`CREATE TABLE "things" ("id" TEXT PRIMARY KEY, "name" TEXT NOT NULL)`)
	writeMigrationFile(t, dir, "0002_add_email.sql",
		`ALTER TABLE "things" ADD COLUMN "email" TEXT`)

	// Baseline the first migration (do not execute it).
	opts := kit.MigrateOptions{
		MigrationsDir: dir,
		Baseline:      "0001_create_things",
	}
	result, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("baseline migrate: %v", err)
	}
	if len(result.Baselined) != 1 {
		t.Errorf("expected 1 baselined migration, got %d", len(result.Baselined))
	}
	// The second file should have been applied.
	if len(result.Applied) != 1 {
		t.Errorf("expected 1 applied migration (0002), got %d", len(result.Applied))
	}
	if result.Applied[0].Tag != "0002_add_email" {
		t.Errorf("expected 0002_add_email applied, got %s", result.Applied[0].Tag)
	}

	// Verify the column exists.
	rows, err := db.QueryContext(ctx, `PRAGMA table_info("things")`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	var colNames []string
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		colNames = append(colNames, name)
	}
	found := false
	for _, c := range colNames {
		if c == "email" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'email' column after applying 0002, got: %v", colNames)
	}
}

func TestSQLite_Baseline_IdempotentRerun(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	if _, err := db.ExecContext(ctx, `CREATE TABLE "things" ("id" TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create things: %v", err)
	}

	writeMigrationFile(t, dir, "0001_create_things.sql",
		`CREATE TABLE "things" ("id" TEXT PRIMARY KEY)`)

	opts := kit.MigrateOptions{
		MigrationsDir: dir,
		Baseline:      "0001_create_things",
	}

	// First run: baseline inserted.
	r1, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("first baseline: %v", err)
	}
	if len(r1.Baselined) != 1 {
		t.Errorf("expected 1 baselined on first run, got %d", len(r1.Baselined))
	}

	// Second run: already recorded, should be a no-op.
	r2, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("second baseline: %v", err)
	}
	if !r2.AlreadyCurrent {
		t.Errorf("expected AlreadyCurrent on second baseline run")
	}
}

func TestSQLite_Baseline_UnknownTag_Error(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_init.sql", `CREATE TABLE "t" ("id" TEXT PRIMARY KEY)`)

	opts := kit.MigrateOptions{
		MigrationsDir: dir,
		Baseline:      "9999_nonexistent",
	}
	_, err := kit.MigrateSQLite(ctx, db, opts)
	if err == nil {
		t.Fatal("expected error for unknown baseline tag, got nil")
	}
	if !strings.Contains(err.Error(), "9999_nonexistent") {
		t.Errorf("error should mention the tag: %v", err)
	}
}

func TestSQLite_SkipSchemaUpgrade_MissingColumns_Error(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	// Manually create the old-style table (no tag/is_baseline columns).
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE _grizzle_migrations (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			applied_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			checksum   TEXT    NOT NULL,
			sql_batch  TEXT    NOT NULL,
			description TEXT   NOT NULL DEFAULT ''
		)`); err != nil {
		t.Fatalf("create old table: %v", err)
	}

	writeMigrationFile(t, dir, "0001_init.sql", `CREATE TABLE "t" ("id" TEXT PRIMARY KEY)`)

	opts := kit.MigrateOptions{
		MigrationsDir:     dir,
		SkipSchemaUpgrade: true,
	}
	_, err := kit.MigrateSQLite(ctx, db, opts)
	if err == nil {
		t.Fatal("expected error when --skip-schema-upgrade and columns absent, got nil")
	}
	if !strings.Contains(err.Error(), "tag") && !strings.Contains(err.Error(), "is_baseline") {
		t.Errorf("error should mention missing columns: %v", err)
	}
}

func TestSQLite_SchemaUpgrade_Idempotent(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	// Manually create old-style table.
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE _grizzle_migrations (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			applied_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			checksum   TEXT    NOT NULL DEFAULT '',
			sql_batch  TEXT    NOT NULL DEFAULT '',
			description TEXT   NOT NULL DEFAULT ''
		)`); err != nil {
		t.Fatalf("create old table: %v", err)
	}

	writeMigrationFile(t, dir, "0001_init.sql", `CREATE TABLE "t" ("id" TEXT PRIMARY KEY)`)

	opts := kit.MigrateOptions{MigrationsDir: dir}

	// First run: upgrades schema and applies migration.
	if _, err := kit.MigrateSQLite(ctx, db, opts); err != nil {
		t.Fatalf("first migrate after upgrade: %v", err)
	}

	// Second run: upgrade is idempotent (columns already exist).
	result, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("second migrate after upgrade: %v", err)
	}
	if !result.AlreadyCurrent {
		t.Error("expected AlreadyCurrent on second run")
	}
}

func TestSQLite_HistoryRecords_Tag_IsBaseline(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	if _, err := db.ExecContext(ctx, `CREATE TABLE "t" ("id" TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create t: %v", err)
	}

	writeMigrationFile(t, dir, "0001_init.sql", `CREATE TABLE "t" ("id" TEXT PRIMARY KEY)`)
	writeMigrationFile(t, dir, "0002_add_col.sql", `ALTER TABLE "t" ADD COLUMN "name" TEXT`)

	// Baseline 0001, apply 0002.
	opts := kit.MigrateOptions{
		MigrationsDir: dir,
		Baseline:      "0001_init",
	}
	if _, err := kit.MigrateSQLite(ctx, db, opts); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	history, err := kit.LoadHistorySQLite(ctx, db)
	if err != nil {
		t.Fatalf("LoadHistorySQLite: %v", err)
	}

	var baselineRows, normalRows int
	for _, r := range history {
		if r.Tag == "" {
			continue // skip legacy rows
		}
		if r.IsBaseline {
			baselineRows++
			if r.SQLBatch != "" {
				t.Errorf("baseline row %q should have empty sql_batch, got %q", r.Tag, r.SQLBatch)
			}
			if r.Checksum == "" {
				t.Errorf("baseline row %q should have non-empty checksum (file SHA-256)", r.Tag)
			}
		} else {
			normalRows++
			if r.SQLBatch == "" {
				t.Errorf("normal row %q should have non-empty sql_batch", r.Tag)
			}
		}
	}
	if baselineRows != 1 {
		t.Errorf("expected 1 baseline row, got %d", baselineRows)
	}
	if normalRows != 1 {
		t.Errorf("expected 1 normal row, got %d", normalRows)
	}
}

func TestSQLite_DuplicateSequencePrefix_Error(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_create_a.sql", `CREATE TABLE "a" ("id" TEXT PRIMARY KEY)`)
	writeMigrationFile(t, dir, "0001_create_b.sql", `CREATE TABLE "b" ("id" TEXT PRIMARY KEY)`)

	opts := kit.MigrateOptions{MigrationsDir: dir}
	_, err := kit.MigrateSQLite(ctx, db, opts)
	if err == nil {
		t.Fatal("expected error for duplicate sequence prefix, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error should mention duplicate: %v", err)
	}
}

func TestSQLite_InvalidTagFormat_Error(t *testing.T) {
	// Verify ValidateTag itself.
	err := kit.ValidateTag("0001 bad tag!")
	if err == nil {
		t.Fatal("expected validation error for invalid tag")
	}
	if !strings.Contains(err.Error(), "^[a-zA-Z0-9_-]+$") {
		t.Errorf("error should show pattern: %v", err)
	}

	// Verify that MigrateSQLite also rejects a file with an invalid stem.
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()
	// Write a file whose stem contains a space (invalid tag char).
	if err := os.WriteFile(filepath.Join(dir, "0001_bad tag.sql"), []byte("SELECT 1"), 0644); err != nil {
		t.Fatal(err)
	}
	opts := kit.MigrateOptions{MigrationsDir: dir}
	if _, err := kit.MigrateSQLite(ctx, db, opts); err == nil {
		t.Fatal("expected error for invalid filename tag through MigrateSQLite, got nil")
	}
}

func TestSQLite_InvalidBaselineTag_Error(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_init.sql", `CREATE TABLE "t" ("id" TEXT PRIMARY KEY)`)

	opts := kit.MigrateOptions{
		MigrationsDir: dir,
		Baseline:      "invalid tag!",
	}
	_, err := kit.MigrateSQLite(ctx, db, opts)
	if err == nil {
		t.Fatal("expected validation error for invalid baseline tag")
	}
	if !strings.Contains(err.Error(), "baseline") {
		t.Errorf("error should mention baseline: %v", err)
	}
}

func TestSQLite_EmptyMigrationsDir(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	opts := kit.MigrateOptions{MigrationsDir: dir}
	result, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("unexpected error for empty dir: %v", err)
	}
	if !result.AlreadyCurrent {
		t.Error("expected AlreadyCurrent for empty migrations dir")
	}
}

func TestSQLite_StatusPending_ShowsNewFile(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_create_things.sql",
		`CREATE TABLE "things" ("id" TEXT PRIMARY KEY)`)

	opts := kit.MigrateOptions{MigrationsDir: dir}

	// Apply first migration.
	if _, err := kit.MigrateSQLite(ctx, db, opts); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}

	// Add a second file without applying it.
	writeMigrationFile(t, dir, "0002_add_col.sql",
		`ALTER TABLE "things" ADD COLUMN "name" TEXT`)

	// Status should show the second file as pending.
	status, err := kit.StatusSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("StatusSQLite: %v", err)
	}
	if len(status.Pending) != 1 {
		t.Fatalf("expected 1 pending migration, got %d", len(status.Pending))
	}
	if status.Pending[0].Tag != "0002_add_col" {
		t.Errorf("expected pending tag 0002_add_col, got %s", status.Pending[0].Tag)
	}
}

func TestSQLite_MigrateAndStatus_AssertTags(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_create_a.sql", `CREATE TABLE "a" ("id" TEXT PRIMARY KEY)`)
	writeMigrationFile(t, dir, "0002_create_b.sql", `CREATE TABLE "b" ("id" TEXT PRIMARY KEY)`)

	opts := kit.MigrateOptions{MigrationsDir: dir}

	result, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("MigrateSQLite: %v", err)
	}
	if len(result.Applied) != 2 {
		t.Fatalf("expected 2 applied, got %d", len(result.Applied))
	}
	if result.Applied[0].Tag != "0001_create_a" {
		t.Errorf("Applied[0].Tag = %q, want 0001_create_a", result.Applied[0].Tag)
	}
	if result.Applied[1].Tag != "0002_create_b" {
		t.Errorf("Applied[1].Tag = %q, want 0002_create_b", result.Applied[1].Tag)
	}
}

func TestSQLite_SkipSchemaUpgrade_ColumnsPresent_Succeeds(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_init.sql", `CREATE TABLE "t" ("id" TEXT PRIMARY KEY)`)

	// First run creates the table with all columns.
	opts := kit.MigrateOptions{MigrationsDir: dir}
	if _, err := kit.MigrateSQLite(ctx, db, opts); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}

	// Second run with SkipSchemaUpgrade: columns are present, should succeed.
	opts2 := kit.MigrateOptions{MigrationsDir: dir, SkipSchemaUpgrade: true}
	result, err := kit.MigrateSQLite(ctx, db, opts2)
	if err != nil {
		t.Fatalf("SkipSchemaUpgrade with present columns: %v", err)
	}
	if !result.AlreadyCurrent {
		t.Error("expected AlreadyCurrent")
	}
}

func TestSQLite_MultiStatement_Migration(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_multi.sql",
		`CREATE TABLE "a" ("id" TEXT PRIMARY KEY);`+
			`CREATE TABLE "b" ("id" TEXT PRIMARY KEY)`)

	opts := kit.MigrateOptions{MigrationsDir: dir}
	result, err := kit.MigrateSQLite(ctx, db, opts)
	if err != nil {
		t.Fatalf("multi-statement migration: %v", err)
	}
	if len(result.Applied) != 1 {
		t.Errorf("expected 1 applied migration, got %d", len(result.Applied))
	}

	// Verify both tables were created.
	for _, tbl := range []string{"a", "b"} {
		var count int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&count); err != nil {
			t.Fatalf("check table %q: %v", tbl, err)
		}
		if count != 1 {
			t.Errorf("table %q not created after multi-statement migration", tbl)
		}
	}
}

func TestSQLite_Baseline_IsNormalRow_False(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_init.sql", `CREATE TABLE "t" ("id" TEXT PRIMARY KEY)`)

	if _, err := db.ExecContext(ctx, `CREATE TABLE "t" ("id" TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("pre-create table: %v", err)
	}

	opts := kit.MigrateOptions{MigrationsDir: dir, Baseline: "0001_init"}
	if _, err := kit.MigrateSQLite(ctx, db, opts); err != nil {
		t.Fatalf("baseline migrate: %v", err)
	}

	history, err := kit.LoadHistorySQLite(ctx, db)
	if err != nil {
		t.Fatalf("LoadHistorySQLite: %v", err)
	}
	for _, r := range history {
		if r.Tag == "0001_init" {
			if !r.IsBaseline {
				t.Error("baseline row should have IsBaseline = true")
			}
			if r.SQLBatch != "" {
				t.Error("baseline row should have empty SQLBatch")
			}
			return
		}
	}
	t.Error("baseline row for 0001_init not found in history")
}

func TestSQLite_NormalRow_IsBaselineFalse(t *testing.T) {
	db := openSQLiteMemory(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeMigrationFile(t, dir, "0001_init.sql", `CREATE TABLE "t" ("id" TEXT PRIMARY KEY)`)

	opts := kit.MigrateOptions{MigrationsDir: dir}
	if _, err := kit.MigrateSQLite(ctx, db, opts); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	history, err := kit.LoadHistorySQLite(ctx, db)
	if err != nil {
		t.Fatalf("LoadHistorySQLite: %v", err)
	}
	for _, r := range history {
		if r.Tag == "0001_init" {
			if r.IsBaseline {
				t.Error("normal row should have IsBaseline = false")
			}
			if r.SQLBatch == "" {
				t.Error("normal row should have non-empty SQLBatch")
			}
			return
		}
	}
	t.Error("row for 0001_init not found in history")
}

func TestSQLite_PushSQLite_StillWorks(t *testing.T) {
	// PushSQLite uses the live-introspect workflow; verify it still compiles and runs.
	db := openSQLiteMemory(t)
	ctx := context.Background()

	schema := []pg.TableDefiner{
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
