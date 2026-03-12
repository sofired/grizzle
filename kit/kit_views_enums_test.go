package kit_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofired/grizzle/kit"
	pg "github.com/sofired/grizzle/schema/pg"
)

// --- Schema fixtures ---

var statusEnum = pg.Enum("status", "pending", "active", "archived")
var roleEnum = pg.Enum("role", "admin", "user", "guest")

var activeUsersView = pg.CreateView("active_users",
	`SELECT id, username, email FROM users WHERE enabled = true`)

var recentOrdersView = pg.CreateView("recent_orders",
	`SELECT * FROM orders WHERE created_at > now() - interval '7 days'`)

// --- FromSchema tests ---

func TestFromSchema_IncludesViewsAndEnums(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Tables: []*pg.TableDef{realmsDef, usersDef},
		Views:  []*pg.ViewDef{activeUsersView},
		Enums:  []*pg.EnumDef{statusEnum},
	})

	if len(snap.Tables) != 2 {
		t.Errorf("expected 2 tables, got %d", len(snap.Tables))
	}
	if len(snap.Views) != 1 {
		t.Errorf("expected 1 view, got %d", len(snap.Views))
	}
	if v, ok := snap.Views["active_users"]; !ok || v.SQL == "" {
		t.Error("missing or empty active_users view")
	}
	if len(snap.Enums) != 1 {
		t.Errorf("expected 1 enum, got %d", len(snap.Enums))
	}
	if e, ok := snap.Enums["status"]; !ok || len(e.Values) != 3 {
		t.Errorf("missing or wrong status enum: %+v", snap.Enums["status"])
	}
}

func TestFromSchema_EmptyViewsEnums(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Tables: []*pg.TableDef{realmsDef},
	})
	if len(snap.Views) != 0 {
		t.Errorf("expected 0 views, got %d", len(snap.Views))
	}
	if len(snap.Enums) != 0 {
		t.Errorf("expected 0 enums, got %d", len(snap.Enums))
	}
}

// --- Diff tests for views ---

func TestDiff_CreateView(t *testing.T) {
	old := kit.EmptySnapshot()
	newSnap := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{activeUsersView},
	})
	changes := kit.Diff(old, newSnap)

	creates := countKind(changes, kit.ChangeCreateView)
	if creates != 1 {
		t.Errorf("expected 1 ChangeCreateView, got %d", creates)
	}
	if changes[0].View == nil || changes[0].View.Name != "active_users" {
		t.Errorf("expected active_users view, got %+v", changes[0].View)
	}
}

func TestDiff_DropView(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{activeUsersView},
	})
	newSnap := kit.EmptySnapshot()
	changes := kit.Diff(old, newSnap)

	drops := countKind(changes, kit.ChangeDropView)
	if drops != 1 {
		t.Errorf("expected 1 ChangeDropView, got %d", drops)
	}
}

func TestDiff_NoChangeView(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{activeUsersView},
	})
	changes := kit.Diff(snap, snap)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for identical view snapshots, got %d", len(changes))
	}
}

func TestDiff_NoChangeView_TrailingSemicolon(t *testing.T) {
	// Trailing semicolons and whitespace differences should not produce spurious diffs.
	v1 := pg.CreateView("my_view", `SELECT id FROM users`)
	v2 := pg.CreateView("my_view", `SELECT id FROM users;`)

	old := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{v1}})
	newSnap := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{v2}})
	changes := kit.Diff(old, newSnap)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for trailing-semicolon-only difference, got %d: %v", len(changes), changes)
	}
}

func TestDiff_ModifiedView(t *testing.T) {
	v1 := pg.CreateView("my_view", `SELECT id FROM users`)
	v2 := pg.CreateView("my_view", `SELECT id, username FROM users`)

	old := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{v1}})
	newSnap := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{v2}})
	changes := kit.Diff(old, newSnap)

	drops := countKind(changes, kit.ChangeDropView)
	creates := countKind(changes, kit.ChangeCreateView)
	if drops != 1 || creates != 1 {
		t.Errorf("expected 1 DropView + 1 CreateView for modified view, got drops=%d creates=%d", drops, creates)
	}
}

// --- Diff tests for enums ---

func TestDiff_CreateEnum(t *testing.T) {
	old := kit.EmptySnapshot()
	newSnap := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{statusEnum},
	})
	changes := kit.Diff(old, newSnap)

	creates := countKind(changes, kit.ChangeCreateEnum)
	if creates != 1 {
		t.Errorf("expected 1 ChangeCreateEnum, got %d", creates)
	}
	if changes[0].NewEnum == nil || changes[0].NewEnum.Name != "status" {
		t.Errorf("expected status enum, got %+v", changes[0].NewEnum)
	}
	if len(changes[0].NewEnum.Values) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(changes[0].NewEnum.Values))
	}
}

func TestDiff_DropEnum(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{statusEnum},
	})
	newSnap := kit.EmptySnapshot()
	changes := kit.Diff(old, newSnap)

	drops := countKind(changes, kit.ChangeDropEnum)
	if drops != 1 {
		t.Errorf("expected 1 ChangeDropEnum, got %d", drops)
	}
}

func TestDiff_NoChangeEnum(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{statusEnum},
	})
	changes := kit.Diff(snap, snap)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes for identical enum snapshots, got %d", len(changes))
	}
}

func TestDiff_AlterEnum_AddValues(t *testing.T) {
	e1 := pg.Enum("mood", "happy", "sad")
	e2 := pg.Enum("mood", "happy", "sad", "excited", "anxious")

	old := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{e1}})
	newSnap := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{e2}})
	changes := kit.Diff(old, newSnap)

	alters := countKind(changes, kit.ChangeAlterEnum)
	if alters != 1 {
		t.Errorf("expected 1 ChangeAlterEnum, got %d", alters)
	}
}

func TestDiff_AlterEnum_NoNewValues(t *testing.T) {
	// Same values: no alter change should be emitted.
	e1 := pg.Enum("mood", "happy", "sad")
	e2 := pg.Enum("mood", "happy", "sad")

	old := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{e1}})
	newSnap := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{e2}})
	changes := kit.Diff(old, newSnap)
	if len(changes) != 0 {
		t.Errorf("expected 0 changes, got %d: %v", len(changes), changes)
	}
}

// --- SQL generation tests for views ---

func TestGenerateChangeSQL_CreateView(t *testing.T) {
	snap := kit.EmptySnapshot()
	v := &kit.ViewSnap{Name: "active_users", SQL: `SELECT id FROM users WHERE enabled = true`}
	change := kit.Change{
		Kind:      kit.ChangeCreateView,
		TableName: "active_users",
		View:      v,
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], "CREATE OR REPLACE VIEW") {
		t.Errorf("expected CREATE OR REPLACE VIEW, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[0], `"active_users"`) {
		t.Errorf("expected quoted view name, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[0], "SELECT id FROM users") {
		t.Errorf("expected view SQL body, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQL_DropView(t *testing.T) {
	snap := kit.EmptySnapshot()
	v := &kit.ViewSnap{Name: "active_users", SQL: `SELECT id FROM users`}
	change := kit.Change{
		Kind:      kit.ChangeDropView,
		TableName: "active_users",
		View:      v,
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `DROP VIEW IF EXISTS "active_users"`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestGenerateChangeSQL_CreateView_Schemaed(t *testing.T) {
	snap := kit.EmptySnapshot()
	v := &kit.ViewSnap{Name: "active_users", Schema: "reporting", SQL: `SELECT id FROM users`}
	change := kit.Change{
		Kind:      kit.ChangeCreateView,
		TableName: "reporting.active_users",
		View:      v,
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], `"reporting"."active_users"`) {
		t.Errorf("expected schema-qualified view name, got: %s", stmts[0])
	}
}

// --- SQL generation tests for enums ---

func TestGenerateChangeSQL_CreateEnum(t *testing.T) {
	snap := kit.EmptySnapshot()
	e := &kit.EnumSnap{Name: "status", Values: []string{"pending", "active", "archived"}}
	change := kit.Change{
		Kind:      kit.ChangeCreateEnum,
		TableName: "status",
		NewEnum:   e,
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `CREATE TYPE "status" AS ENUM ('pending', 'active', 'archived')`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestGenerateChangeSQL_DropEnum(t *testing.T) {
	snap := kit.EmptySnapshot()
	e := &kit.EnumSnap{Name: "status", Values: []string{"pending"}}
	change := kit.Change{
		Kind:      kit.ChangeDropEnum,
		TableName: "status",
		OldEnum:   e,
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	want := `DROP TYPE IF EXISTS "status"`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestGenerateChangeSQL_AlterEnum_AddValues(t *testing.T) {
	snap := kit.EmptySnapshot()
	old := &kit.EnumSnap{Name: "mood", Values: []string{"happy", "sad"}}
	newE := &kit.EnumSnap{Name: "mood", Values: []string{"happy", "sad", "excited"}}
	change := kit.Change{
		Kind:      kit.ChangeAlterEnum,
		TableName: "mood",
		OldEnum:   old,
		NewEnum:   newE,
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement for 1 new value, got %d: %v", len(stmts), stmts)
	}
	want := `ALTER TYPE "mood" ADD VALUE IF NOT EXISTS 'excited'`
	if stmts[0] != want {
		t.Errorf("got:  %s\nwant: %s", stmts[0], want)
	}
}

func TestGenerateChangeSQL_AlterEnum_MultipleValues(t *testing.T) {
	snap := kit.EmptySnapshot()
	old := &kit.EnumSnap{Name: "mood", Values: []string{"happy"}}
	newE := &kit.EnumSnap{Name: "mood", Values: []string{"happy", "sad", "excited"}}
	change := kit.Change{
		Kind:      kit.ChangeAlterEnum,
		TableName: "mood",
		OldEnum:   old,
		NewEnum:   newE,
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements for 2 new values, got %d: %v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "'sad'") {
		t.Errorf("expected 'sad' in first statement: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "'excited'") {
		t.Errorf("expected 'excited' in second statement: %s", stmts[1])
	}
}

func TestGenerateChangeSQL_CreateEnum_SingleQuoteEscaping(t *testing.T) {
	snap := kit.EmptySnapshot()
	e := &kit.EnumSnap{Name: "tricky", Values: []string{"it's", "fine"}}
	change := kit.Change{
		Kind:      kit.ChangeCreateEnum,
		TableName: "tricky",
		NewEnum:   e,
	}
	stmts := kit.GenerateChangeSQL(snap, change)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	// Single quotes inside enum values must be escaped as ''
	if !strings.Contains(stmts[0], "it''s") {
		t.Errorf("expected escaped single quote in enum value, got: %s", stmts[0])
	}
}

// --- EnumColumn builder tests ---

func TestEnumColumn_ColumnDef(t *testing.T) {
	col := pg.EnumColumn("status").NotNull().Default("pending").Build("state")
	if col.Name != "state" {
		t.Errorf("expected name 'state', got %q", col.Name)
	}
	if col.SQLType != "status" {
		t.Errorf("expected SQLType 'status', got %q", col.SQLType)
	}
	if !col.NotNull {
		t.Error("expected NotNull")
	}
	if col.DefaultExpr != "'pending'" {
		t.Errorf("expected default \"'pending'\", got %q", col.DefaultExpr)
	}
}

func TestEnumColumn_InTable(t *testing.T) {
	tbl := pg.Table("orders",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("status", pg.EnumColumn("order_status").NotNull().Default("pending")),
	).Build()

	sql := kit.GenerateCreateSQL(tbl)
	t.Logf("Orders SQL:\n%s", sql)

	if !strings.Contains(sql, `"status" order_status NOT NULL`) {
		t.Errorf("expected enum column type in CREATE TABLE, got:\n%s", sql)
	}
}

// --- View DSL tests ---

func TestCreateView_QualifiedName(t *testing.T) {
	v := pg.CreateView("active_users", `SELECT id FROM users`)
	if v.QualifiedName() != "active_users" {
		t.Errorf("expected 'active_users', got %q", v.QualifiedName())
	}
}

func TestSchemaView_QualifiedName(t *testing.T) {
	v := pg.SchemaView("reporting", "active_users", `SELECT id FROM users`)
	if v.QualifiedName() != "reporting.active_users" {
		t.Errorf("expected 'reporting.active_users', got %q", v.QualifiedName())
	}
}

// --- Enum DSL tests ---

func TestEnum_QualifiedName(t *testing.T) {
	e := pg.Enum("status", "a", "b")
	if e.QualifiedName() != "status" {
		t.Errorf("expected 'status', got %q", e.QualifiedName())
	}
}

func TestSchemaEnum_QualifiedName(t *testing.T) {
	e := pg.SchemaEnum("auth", "role", "admin", "user")
	if e.QualifiedName() != "auth.role" {
		t.Errorf("expected 'auth.role', got %q", e.QualifiedName())
	}
}

// --- Snapshot round-trip with views and enums ---

func TestSnapshotJSON_RoundTrip_WithViewsEnums(t *testing.T) {
	dir := t.TempDir()
	snap := kit.FromSchema(kit.SchemaObjects{
		Tables: []*pg.TableDef{realmsDef},
		Views:  []*pg.ViewDef{activeUsersView},
		Enums:  []*pg.EnumDef{statusEnum},
	})

	path := filepath.Join(dir, "schema.snapshot.json")
	if err := kit.SaveJSON(snap, path); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := kit.LoadJSON(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Views) != 1 {
		t.Errorf("expected 1 view after round-trip, got %d", len(loaded.Views))
	}
	if len(loaded.Enums) != 1 {
		t.Errorf("expected 1 enum after round-trip, got %d", len(loaded.Enums))
	}
	if loaded.Enums["status"] == nil || len(loaded.Enums["status"].Values) != 3 {
		t.Errorf("enum values not preserved in round-trip: %+v", loaded.Enums["status"])
	}
}
