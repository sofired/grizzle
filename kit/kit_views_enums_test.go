package kit_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sofired/grizzle/kit"
	pg "github.com/sofired/grizzle/schema/pg"
)

// -------------------------------------------------------------------
// Fixtures
// -------------------------------------------------------------------

var activeUsersView = pg.CreateView("active_users",
	`SELECT id, username, email FROM users WHERE enabled = true`)

var reportingView = pg.SchemaView("reporting", "recent_orders",
	`SELECT * FROM orders WHERE created_at > now() - interval '7 days'`)

var statusEnum = pg.CreateEnum("status", "pending", "active", "archived")

var roleEnum = pg.SchemaCreateEnum("auth", "role", "admin", "user", "guest")

// -------------------------------------------------------------------
// pg DSL: ViewDef
// -------------------------------------------------------------------

func TestCreateView_QualifiedName(t *testing.T) {
	v := pg.CreateView("active_users", "SELECT 1")
	if v.QualifiedName() != "active_users" {
		t.Errorf("expected 'active_users', got %q", v.QualifiedName())
	}
}

func TestSchemaView_QualifiedName(t *testing.T) {
	v := pg.SchemaView("reporting", "recent_orders", "SELECT 1")
	if v.QualifiedName() != "reporting.recent_orders" {
		t.Errorf("expected 'reporting.recent_orders', got %q", v.QualifiedName())
	}
}

func TestSchemaView_PublicSchema_IsUnqualified(t *testing.T) {
	v := pg.SchemaView("public", "v", "SELECT 1")
	if v.QualifiedName() != "v" {
		t.Errorf("expected 'v' for public schema (matches introspection keying), got %q", v.QualifiedName())
	}
}

// -------------------------------------------------------------------
// pg DSL: EnumDef
// -------------------------------------------------------------------

func TestCreateEnum_Values(t *testing.T) {
	e := pg.CreateEnum("status", "pending", "active", "archived")
	if e.Name != "status" {
		t.Errorf("expected name 'status', got %q", e.Name)
	}
	if len(e.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(e.Values))
	}
	if e.Values[0] != "pending" || e.Values[1] != "active" || e.Values[2] != "archived" {
		t.Errorf("unexpected values: %v", e.Values)
	}
}

func TestSchemaCreateEnum_QualifiedName(t *testing.T) {
	e := pg.SchemaCreateEnum("auth", "role", "admin", "user")
	if e.QualifiedName() != "auth.role" {
		t.Errorf("expected 'auth.role', got %q", e.QualifiedName())
	}
}

func TestSchemaCreateEnum_PublicSchema_IsUnqualified(t *testing.T) {
	e := pg.SchemaCreateEnum("public", "status", "a", "b")
	if e.QualifiedName() != "status" {
		t.Errorf("expected 'status' for public schema (matches introspection keying), got %q", e.QualifiedName())
	}
}

func TestCreateEnum_PanicsOnEmptyValue(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty value, got none")
		}
	}()
	pg.CreateEnum("bad", "ok", "", "also ok")
}

func TestCreateEnum_PanicsOnZeroValues(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on zero values, got none")
		}
	}()
	pg.CreateEnum("bad")
}

func TestSchemaCreateEnum_PanicsOnZeroValues(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on zero values, got none")
		}
	}()
	pg.SchemaCreateEnum("auth", "bad")
}

// -------------------------------------------------------------------
// pg DSL: EnumColumn
// -------------------------------------------------------------------

func TestEnumColumn_Build(t *testing.T) {
	col := pg.EnumColumn("status").NotNull().Default("pending").Build("state")
	if col.Name != "state" {
		t.Errorf("expected name 'state', got %q", col.Name)
	}
	if col.SQLType != "status" {
		t.Errorf("expected SQLType 'status', got %q", col.SQLType)
	}
	if !col.NotNull {
		t.Error("expected NotNull=true")
	}
	if !col.HasDefault {
		t.Error("expected HasDefault=true")
	}
	// Default should be quoted and cast
	if !strings.Contains(col.DefaultExpr, "pending") {
		t.Errorf("unexpected DefaultExpr: %q", col.DefaultExpr)
	}
}

func TestEnumColumn_DefaultEscapesSingleQuotes(t *testing.T) {
	col := pg.EnumColumn("mood").Default("it's fine").Build("m")
	if !strings.Contains(col.DefaultExpr, "it''s fine") {
		t.Errorf("single quote not escaped in default: %q", col.DefaultExpr)
	}
}

// -------------------------------------------------------------------
// Snapshot: FromSchema
// -------------------------------------------------------------------

func TestFromSchema_Tables(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
	})
	if len(snap.Tables) != 1 {
		t.Errorf("expected 1 table, got %d", len(snap.Tables))
	}
	if len(snap.Views) != 0 {
		t.Errorf("expected 0 views, got %d", len(snap.Views))
	}
	if len(snap.Enums) != 0 {
		t.Errorf("expected 0 enums, got %d", len(snap.Enums))
	}
}

func TestFromSchema_ViewsAndEnums(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Views:  []*pg.ViewDef{activeUsersView},
		Enums:  []*pg.EnumDef{statusEnum},
	})
	if len(snap.Tables) != 1 {
		t.Errorf("expected 1 table, got %d", len(snap.Tables))
	}
	if len(snap.Views) != 1 {
		t.Errorf("expected 1 view, got %d", len(snap.Views))
	}
	if _, ok := snap.Views["active_users"]; !ok {
		t.Error("missing 'active_users' view in snapshot")
	}
	if len(snap.Enums) != 1 {
		t.Errorf("expected 1 enum, got %d", len(snap.Enums))
	}
	if _, ok := snap.Enums["status"]; !ok {
		t.Error("missing 'status' enum in snapshot")
	}
}

func TestFromSchema_SchemaQualifiedObjects(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{reportingView},
		Enums: []*pg.EnumDef{roleEnum},
	})
	if _, ok := snap.Views["reporting.recent_orders"]; !ok {
		t.Errorf("expected schema-qualified view key 'reporting.recent_orders', got keys: %v", snapViewKeys(snap))
	}
	if _, ok := snap.Enums["auth.role"]; !ok {
		t.Errorf("expected schema-qualified enum key 'auth.role', got keys: %v", snapEnumKeys(snap))
	}
}

func TestFromSchema_PreviousName_RenameDetection(t *testing.T) {
	oldDef := pg.Table("accounts",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build()
	newDef := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).RenamedFrom("accounts").Build()

	old := kit.FromSchema(kit.SchemaObjects{Tables: []pg.TableDefiner{oldDef}})
	new := kit.FromSchema(kit.SchemaObjects{Tables: []pg.TableDefiner{newDef}})
	changes := kit.Diff(old, new)

	var renames, drops, creates int
	for _, c := range changes {
		switch c.Kind {
		case kit.ChangeRenameTable:
			renames++
		case kit.ChangeDropTable:
			drops++
		case kit.ChangeCreateTable:
			creates++
		}
	}
	if renames != 1 || drops != 0 || creates != 0 {
		t.Errorf("expected 1 rename, 0 drops, 0 creates; got %d/%d/%d — FromSchema must copy PreviousName", renames, drops, creates)
	}
}

// -------------------------------------------------------------------
// Snapshot: JSON round-trip with views and enums
// -------------------------------------------------------------------

func TestSnapshot_JSONRoundTrip_WithViewsAndEnums(t *testing.T) {
	orig := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Views:  []*pg.ViewDef{activeUsersView},
		Enums:  []*pg.EnumDef{statusEnum},
	})

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var loaded kit.Snapshot
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(loaded.Views) != 1 {
		t.Errorf("expected 1 view after round-trip, got %d", len(loaded.Views))
	}
	if v, ok := loaded.Views["active_users"]; !ok {
		t.Error("missing 'active_users' after round-trip")
	} else if v.SQL != activeUsersView.SQL {
		t.Errorf("view SQL mismatch: got %q", v.SQL)
	}

	if len(loaded.Enums) != 1 {
		t.Errorf("expected 1 enum after round-trip, got %d", len(loaded.Enums))
	}
	if e, ok := loaded.Enums["status"]; !ok {
		t.Error("missing 'status' enum after round-trip")
	} else if len(e.Values) != 3 || e.Values[0] != "pending" {
		t.Errorf("enum values mismatch: %v", e.Values)
	}
}

// -------------------------------------------------------------------
// Diff: views
// -------------------------------------------------------------------

func TestDiff_CreateView(t *testing.T) {
	old := kit.EmptySnapshot()
	new := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{activeUsersView},
	})

	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	c := changes[0]
	if c.Kind != kit.ChangeCreateView {
		t.Errorf("expected ChangeCreateView, got %s", c.Kind)
	}
	if c.ObjectName != "active_users" {
		t.Errorf("expected ObjectName 'active_users', got %q", c.ObjectName)
	}
	if c.View == nil {
		t.Fatal("expected View to be set")
	}
	if c.View.SQL != activeUsersView.SQL {
		t.Errorf("view SQL mismatch: got %q", c.View.SQL)
	}
}

func TestDiff_DropView(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{activeUsersView},
	})
	new := kit.EmptySnapshot()

	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	c := changes[0]
	if c.Kind != kit.ChangeDropView {
		t.Errorf("expected ChangeDropView, got %s", c.Kind)
	}
	if c.View == nil || c.View.Name != "active_users" {
		t.Errorf("unexpected view: %+v", c.View)
	}
}

func TestDiff_AlterView(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT 1")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT 2")},
	})

	changes := kit.Diff(old, new)
	// Expect ChangeReplaceView (not ChangeCreateView): modified views need DROP + CREATE
	// because CREATE OR REPLACE VIEW cannot handle incompatible column changes in PostgreSQL.
	if len(changes) != 1 {
		t.Fatalf("expected 1 change (ChangeReplaceView), got %d: %v", len(changes), changes)
	}
	if changes[0].Kind != kit.ChangeReplaceView {
		t.Errorf("expected ChangeReplaceView, got %s", changes[0].Kind)
	}
}

func TestGenerateChangeSQL_ReplaceView_DropsAndCreates(t *testing.T) {
	// PostgreSQL: ChangeReplaceView must emit DROP VIEW IF EXISTS + CREATE VIEW
	// so incompatible column changes (renames, type changes, reordering) converge correctly.
	old := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT id FROM users")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT id, email FROM users")},
	})
	changes := kit.Diff(old, new)
	if len(changes) != 1 || changes[0].Kind != kit.ChangeReplaceView {
		t.Fatalf("expected 1 ChangeReplaceView, got %v", changes)
	}
	stmts := kit.GenerateChangeSQL(new, changes[0])
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements (DROP + CREATE), got %d: %v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "DROP VIEW IF EXISTS") {
		t.Errorf("first statement must be DROP VIEW IF EXISTS, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "CREATE VIEW") || strings.Contains(stmts[1], "OR REPLACE") {
		t.Errorf("second statement must be CREATE VIEW (not OR REPLACE), got: %s", stmts[1])
	}
}

func TestGenerateChangeSQLMySQL_ReplaceView_UsesCreateOrReplace(t *testing.T) {
	// MySQL: CREATE OR REPLACE VIEW handles incompatible column changes natively.
	old := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT id FROM users")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT id, email FROM users")},
	})
	changes := kit.Diff(old, new)
	if len(changes) != 1 || changes[0].Kind != kit.ChangeReplaceView {
		t.Fatalf("expected 1 ChangeReplaceView, got %v", changes)
	}
	stmts := kit.GenerateChangeSQLMySQL(new, changes[0])
	if len(stmts) != 1 || !strings.Contains(stmts[0], "CREATE OR REPLACE VIEW") {
		t.Errorf("expected CREATE OR REPLACE VIEW, got: %v", stmts)
	}
}

func TestGenerateChangeSQLSQLite_ReplaceView_DropsAndCreates(t *testing.T) {
	// SQLite: same DROP + CREATE sequence as ChangeCreateView.
	old := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT id FROM users")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT id, email FROM users")},
	})
	changes := kit.Diff(old, new)
	if len(changes) != 1 || changes[0].Kind != kit.ChangeReplaceView {
		t.Fatalf("expected 1 ChangeReplaceView, got %v", changes)
	}
	stmts := kit.GenerateChangeSQLSQLite(new, changes[0])
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements (DROP + CREATE), got %d: %v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "DROP VIEW IF EXISTS") {
		t.Errorf("first statement must be DROP VIEW IF EXISTS, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "CREATE VIEW") {
		t.Errorf("second statement must be CREATE VIEW, got: %s", stmts[1])
	}
}

func TestGenerateChangeSQL_ReplaceView_SchemaQualified_PG(t *testing.T) {
	// PostgreSQL: ChangeReplaceView with a schema-qualified view must emit
	// correctly double-quoted DROP VIEW IF EXISTS + CREATE VIEW.
	view := pg.SchemaView("reporting", "recent_orders", `SELECT * FROM orders WHERE created_at > now() - interval '7 days'`)
	oldSnap := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{view}})
	newView := pg.SchemaView("reporting", "recent_orders", `SELECT id FROM orders`)
	newSnap := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{newView}})
	changes := kit.Diff(oldSnap, newSnap)
	if len(changes) != 1 || changes[0].Kind != kit.ChangeReplaceView {
		t.Fatalf("expected 1 ChangeReplaceView, got %v", changes)
	}
	stmts := kit.GenerateChangeSQL(newSnap, changes[0])
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements (DROP + CREATE), got %d: %v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], `"reporting"."recent_orders"`) {
		t.Errorf("DROP VIEW must contain schema-qualified quoted name, got: %s", stmts[0])
	}
	if !strings.HasPrefix(stmts[0], "DROP VIEW IF EXISTS") {
		t.Errorf("first statement must be DROP VIEW IF EXISTS, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], `"reporting"."recent_orders"`) {
		t.Errorf("CREATE VIEW must contain schema-qualified quoted name, got: %s", stmts[1])
	}
	if !strings.Contains(stmts[1], "CREATE VIEW") || strings.Contains(stmts[1], "OR REPLACE") {
		t.Errorf("second statement must be CREATE VIEW (not OR REPLACE), got: %s", stmts[1])
	}
}

func TestGenerateChangeSQL_ReplaceView_SchemaQualified_MySQL(t *testing.T) {
	// MySQL: ChangeReplaceView with a schema-qualified view emits CREATE OR REPLACE VIEW
	// with the correctly quoted schema-qualified name.
	view := pg.SchemaView("reporting", "recent_orders", `SELECT * FROM orders WHERE created_at > now() - interval '7 days'`)
	oldSnap := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{view}})
	newView := pg.SchemaView("reporting", "recent_orders", `SELECT id FROM orders`)
	newSnap := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{newView}})
	changes := kit.Diff(oldSnap, newSnap)
	if len(changes) != 1 || changes[0].Kind != kit.ChangeReplaceView {
		t.Fatalf("expected 1 ChangeReplaceView, got %v", changes)
	}
	stmts := kit.GenerateChangeSQLMySQL(newSnap, changes[0])
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "CREATE OR REPLACE VIEW") {
		t.Errorf("expected CREATE OR REPLACE VIEW, got: %s", stmts[0])
	}
	// MySQL uses backtick quoting
	if !strings.Contains(stmts[0], "`reporting`") || !strings.Contains(stmts[0], "`recent_orders`") {
		t.Errorf("expected backtick-quoted schema-qualified name, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQL_ReplaceView_SchemaQualified_SQLite(t *testing.T) {
	// SQLite: ChangeReplaceView with a schema-qualified view emits
	// DROP VIEW IF EXISTS + CREATE VIEW; SQLite has no schema namespace so
	// the schema prefix is stripped and only the unqualified name is quoted.
	view := pg.SchemaView("reporting", "recent_orders", `SELECT * FROM orders WHERE created_at > now() - interval '7 days'`)
	oldSnap := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{view}})
	newView := pg.SchemaView("reporting", "recent_orders", `SELECT id FROM orders`)
	newSnap := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{newView}})
	changes := kit.Diff(oldSnap, newSnap)
	if len(changes) != 1 || changes[0].Kind != kit.ChangeReplaceView {
		t.Fatalf("expected 1 ChangeReplaceView, got %v", changes)
	}
	stmts := kit.GenerateChangeSQLSQLite(newSnap, changes[0])
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements (DROP + CREATE), got %d: %v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "DROP VIEW IF EXISTS") {
		t.Errorf("first statement must be DROP VIEW IF EXISTS, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[0], `"recent_orders"`) {
		t.Errorf("DROP VIEW must contain quoted view name, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "CREATE VIEW") {
		t.Errorf("second statement must be CREATE VIEW, got: %s", stmts[1])
	}
	if !strings.Contains(stmts[1], `"recent_orders"`) {
		t.Errorf("CREATE VIEW must contain quoted view name, got: %s", stmts[1])
	}
}

func TestDiff_NoChange_View_SameSQL(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT 1")},
	})
	changes := kit.Diff(snap, snap)
	if len(changes) != 0 {
		t.Errorf("expected no changes for identical views, got %d", len(changes))
	}
}

func TestDiff_NoChange_View_TrailingSemicolon(t *testing.T) {
	// view_definition from PostgreSQL sometimes omits trailing semicolons.
	snap1 := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT 1;")},
	})
	snap2 := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{pg.CreateView("v", "SELECT 1")},
	})
	changes := kit.Diff(snap1, snap2)
	if len(changes) != 0 {
		t.Errorf("expected no changes when trailing semicolon differs, got %d", len(changes))
	}
}

// -------------------------------------------------------------------
// Diff: enums
// -------------------------------------------------------------------

func TestDiff_CreateEnum(t *testing.T) {
	old := kit.EmptySnapshot()
	new := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{statusEnum},
	})

	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	c := changes[0]
	if c.Kind != kit.ChangeCreateEnum {
		t.Errorf("expected ChangeCreateEnum, got %s", c.Kind)
	}
	if c.NewEnum == nil {
		t.Fatal("expected NewEnum to be set")
	}
	if len(c.NewEnum.Values) != 3 {
		t.Errorf("expected 3 values, got %d", len(c.NewEnum.Values))
	}
}

func TestDiff_DropEnum(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{statusEnum},
	})
	new := kit.EmptySnapshot()

	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	c := changes[0]
	if c.Kind != kit.ChangeDropEnum {
		t.Errorf("expected ChangeDropEnum, got %s", c.Kind)
	}
	if c.OldEnum == nil || c.OldEnum.Name != "status" {
		t.Errorf("unexpected OldEnum: %+v", c.OldEnum)
	}
}

func TestDiff_AlterEnum_AddValues(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("status", "pending", "active")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("status", "pending", "active", "archived")},
	})

	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	c := changes[0]
	if c.Kind != kit.ChangeAlterEnum {
		t.Errorf("expected ChangeAlterEnum, got %s", c.Kind)
	}
	if c.OldEnum == nil || c.NewEnum == nil {
		t.Fatal("expected OldEnum and NewEnum to be set")
	}
}

func TestDiff_NoChange_Enum(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{statusEnum},
	})
	changes := kit.Diff(snap, snap)
	if len(changes) != 0 {
		t.Errorf("expected no changes for identical enum, got %d", len(changes))
	}
}

func TestDiff_AlterEnum_RemovedValue(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("status", "pending", "active", "archived")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("status", "pending", "active")},
	})
	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change for removed enum value, got %d: %v", len(changes), changes)
	}
	if changes[0].Kind != kit.ChangeAlterEnum {
		t.Errorf("expected ChangeAlterEnum, got %s", changes[0].Kind)
	}
	// SQL should include a warning comment for the removed value.
	snap := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{pg.CreateEnum("status", "pending", "active")}})
	stmts := kit.GenerateChangeSQL(snap, changes[0])
	if len(stmts) == 0 {
		t.Fatal("expected at least one SQL statement (warning comment)")
	}
	found := false
	for _, s := range stmts {
		if strings.Contains(s, "-- WARNING") && strings.Contains(s, "archived") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected WARNING comment mentioning 'archived', got: %v", stmts)
	}
}

func TestDiff_AlterEnum_ReorderedValues(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("status", "a", "b", "c")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("status", "a", "c", "b")},
	})
	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change for reordered enum values, got %d: %v", len(changes), changes)
	}
	if changes[0].Kind != kit.ChangeAlterEnum {
		t.Errorf("expected ChangeAlterEnum, got %s", changes[0].Kind)
	}
	snap := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{pg.CreateEnum("status", "a", "c", "b")}})
	stmts := kit.GenerateChangeSQL(snap, changes[0])
	found := false
	for _, s := range stmts {
		if strings.Contains(s, "-- WARNING") && strings.Contains(s, "reordered") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected WARNING comment about reordering, got: %v", stmts)
	}
}

// -------------------------------------------------------------------
// Diff: phase ordering
// -------------------------------------------------------------------

func TestDiff_Ordering_AlterEnumBeforeCreateTable(t *testing.T) {
	// A new table is added at the same time an existing enum gains a value.
	// ALTER TYPE must precede CREATE TABLE so a column default referencing the
	// new label succeeds in PostgreSQL.
	oldEnum := pg.CreateEnum("status", "pending", "active")
	newEnum := pg.CreateEnum("status", "pending", "active", "archived")

	old := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{oldEnum}})
	new := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Enums:  []*pg.EnumDef{newEnum},
	})

	changes := kit.Diff(old, new)

	alterIdx, createIdx := -1, -1
	for i, c := range changes {
		if c.Kind == kit.ChangeAlterEnum {
			alterIdx = i
		}
		if c.Kind == kit.ChangeCreateTable {
			createIdx = i
		}
	}
	if alterIdx < 0 || createIdx < 0 {
		t.Fatalf("missing expected changes: alter=%d create=%d", alterIdx, createIdx)
	}
	if alterIdx >= createIdx {
		t.Errorf("expected ALTER ENUM (idx %d) before CREATE TABLE (idx %d)", alterIdx, createIdx)
	}
}

func TestDiff_Ordering_AlterEnumBeforeCreateView(t *testing.T) {
	// A new view is added at the same time an existing enum gains a value.
	// ALTER TYPE must precede CREATE VIEW so a view predicate referencing the
	// new label succeeds in PostgreSQL.
	oldEnum := pg.CreateEnum("status", "pending", "active")
	newEnum := pg.CreateEnum("status", "pending", "active", "archived")

	old := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Enums:  []*pg.EnumDef{oldEnum},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Views:  []*pg.ViewDef{activeUsersView},
		Enums:  []*pg.EnumDef{newEnum},
	})

	changes := kit.Diff(old, new)

	alterIdx, createIdx := -1, -1
	for i, c := range changes {
		if c.Kind == kit.ChangeAlterEnum {
			alterIdx = i
		}
		if c.Kind == kit.ChangeCreateView {
			createIdx = i
		}
	}
	if alterIdx < 0 || createIdx < 0 {
		t.Fatalf("missing expected changes: alter=%d create=%d", alterIdx, createIdx)
	}
	if alterIdx >= createIdx {
		t.Errorf("expected ALTER ENUM (idx %d) before CREATE VIEW (idx %d)", alterIdx, createIdx)
	}
}

func TestDiff_Ordering_EnumBeforeTable(t *testing.T) {
	old := kit.EmptySnapshot()
	new := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Enums:  []*pg.EnumDef{statusEnum},
	})

	changes := kit.Diff(old, new)

	enumIdx, tableIdx := -1, -1
	for i, c := range changes {
		if c.Kind == kit.ChangeCreateEnum {
			enumIdx = i
		}
		if c.Kind == kit.ChangeCreateTable {
			tableIdx = i
		}
	}
	if enumIdx < 0 || tableIdx < 0 {
		t.Fatalf("missing expected changes: enum=%d table=%d", enumIdx, tableIdx)
	}
	if enumIdx >= tableIdx {
		t.Errorf("expected CREATE ENUM (idx %d) before CREATE TABLE (idx %d)", enumIdx, tableIdx)
	}
}

func TestDiff_Ordering_ViewCreatedAfterTable(t *testing.T) {
	old := kit.EmptySnapshot()
	new := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Views:  []*pg.ViewDef{activeUsersView},
	})

	changes := kit.Diff(old, new)

	viewIdx, tableIdx := -1, -1
	for i, c := range changes {
		if c.Kind == kit.ChangeCreateView {
			viewIdx = i
		}
		if c.Kind == kit.ChangeCreateTable {
			tableIdx = i
		}
	}
	if viewIdx < 0 || tableIdx < 0 {
		t.Fatalf("missing expected changes: view=%d table=%d", viewIdx, tableIdx)
	}
	if tableIdx >= viewIdx {
		t.Errorf("expected CREATE TABLE (idx %d) before CREATE VIEW (idx %d)", tableIdx, viewIdx)
	}
}

func TestDiff_Ordering_ViewDroppedBeforeTable(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Views:  []*pg.ViewDef{activeUsersView},
	})
	new := kit.EmptySnapshot()

	changes := kit.Diff(old, new)

	viewIdx, tableIdx := -1, -1
	for i, c := range changes {
		if c.Kind == kit.ChangeDropView {
			viewIdx = i
		}
		if c.Kind == kit.ChangeDropTable {
			tableIdx = i
		}
	}
	if viewIdx < 0 || tableIdx < 0 {
		t.Fatalf("missing expected changes: view=%d table=%d", viewIdx, tableIdx)
	}
	if viewIdx >= tableIdx {
		t.Errorf("expected DROP VIEW (idx %d) before DROP TABLE (idx %d)", viewIdx, tableIdx)
	}
}

func TestDiff_Ordering_EnumDroppedAfterTable(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Enums:  []*pg.EnumDef{statusEnum},
	})
	new := kit.EmptySnapshot()

	changes := kit.Diff(old, new)

	enumIdx, tableIdx := -1, -1
	for i, c := range changes {
		if c.Kind == kit.ChangeDropEnum {
			enumIdx = i
		}
		if c.Kind == kit.ChangeDropTable {
			tableIdx = i
		}
	}
	if enumIdx < 0 || tableIdx < 0 {
		t.Fatalf("missing expected changes: enum=%d table=%d", enumIdx, tableIdx)
	}
	if tableIdx >= enumIdx {
		t.Errorf("expected DROP TABLE (idx %d) before DROP ENUM (idx %d)", tableIdx, enumIdx)
	}
}

func TestDiff_Ordering_CreateViewAfterAlterTable(t *testing.T) {
	// A new view is added at the same time an existing table gains a column.
	// The ALTER TABLE must precede CREATE VIEW so the view can reference the
	// new column without error.
	oldTable := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build()
	newTable := pg.Table("users",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("email", pg.Varchar(255)),
	).Build()
	newView := pg.CreateView("user_emails", `SELECT id, email FROM users`)

	old := kit.FromSchema(kit.SchemaObjects{Tables: []pg.TableDefiner{oldTable}})
	new := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{newTable},
		Views:  []*pg.ViewDef{newView},
	})

	changes := kit.Diff(old, new)

	addColIdx, createViewIdx := -1, -1
	for i, c := range changes {
		if c.Kind == kit.ChangeAddColumn {
			addColIdx = i
		}
		if c.Kind == kit.ChangeCreateView {
			createViewIdx = i
		}
	}
	if addColIdx < 0 || createViewIdx < 0 {
		t.Fatalf("missing expected changes: addCol=%d createView=%d", addColIdx, createViewIdx)
	}
	if addColIdx >= createViewIdx {
		t.Errorf("expected ADD COLUMN (idx %d) before CREATE VIEW (idx %d)", addColIdx, createViewIdx)
	}
}

func TestDiff_Ordering_CreateViewAfterDropTable_SameName(t *testing.T) {
	// A table named "summary" is replaced by a view of the same name.
	// ChangeDropTable must precede ChangeCreateView: PostgreSQL requires a view
	// name to be distinct from all existing relations, including the old table.
	oldTable := pg.Table("summary",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build()
	newView := pg.CreateView("summary", `SELECT id FROM users`)

	old := kit.FromSchema(kit.SchemaObjects{Tables: []pg.TableDefiner{oldTable}})
	new := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{newView}})

	changes := kit.Diff(old, new)

	dropTableIdx, createViewIdx := -1, -1
	for i, c := range changes {
		if c.Kind == kit.ChangeDropTable && c.ObjectName == "summary" {
			dropTableIdx = i
		}
		if c.Kind == kit.ChangeCreateView && c.ObjectName == "summary" {
			createViewIdx = i
		}
	}
	if dropTableIdx < 0 || createViewIdx < 0 {
		t.Fatalf("missing expected changes: dropTable=%d createView=%d", dropTableIdx, createViewIdx)
	}
	if dropTableIdx >= createViewIdx {
		t.Errorf("expected DROP TABLE (idx %d) before CREATE VIEW (idx %d)", dropTableIdx, createViewIdx)
	}
}

func TestDiff_Ordering_CreateEnumAfterDropTable_SameName(t *testing.T) {
	// A table named "status" is removed and a new enum named "status" is added.
	// ChangeDropTable must precede ChangeCreateEnum: PostgreSQL tables carry an
	// associated row type that blocks a same-named CREATE TYPE until the table
	// is dropped.
	oldTable := pg.Table("status",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	).Build()
	newEnum := pg.CreateEnum("status", "pending", "active", "archived")

	old := kit.FromSchema(kit.SchemaObjects{Tables: []pg.TableDefiner{oldTable}})
	new := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{newEnum}})

	changes := kit.Diff(old, new)

	dropTableIdx, createEnumIdx := -1, -1
	for i, c := range changes {
		if c.Kind == kit.ChangeDropTable && c.ObjectName == "status" {
			dropTableIdx = i
		}
		if c.Kind == kit.ChangeCreateEnum && c.ObjectName == "status" {
			createEnumIdx = i
		}
	}
	if dropTableIdx < 0 || createEnumIdx < 0 {
		t.Fatalf("missing expected changes: dropTable=%d createEnum=%d", dropTableIdx, createEnumIdx)
	}
	if dropTableIdx >= createEnumIdx {
		t.Errorf("expected DROP TABLE (idx %d) before CREATE ENUM (idx %d)", dropTableIdx, createEnumIdx)
	}
}

func TestGenerateChangeSQL_AlterEnum_ConsecutiveInsertion(t *testing.T) {
	// old=[a,d] → new=[a,b,c,d]: two labels inserted consecutively after 'a'.
	// 'b' must be AFTER 'a'; 'c' must be AFTER 'b' (not AFTER 'a').
	// If both anchor to 'a', PostgreSQL produces [a,c,b,d] instead of [a,b,c,d].
	old := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("s", "a", "d")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("s", "a", "b", "c", "d")},
	})
	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(new, changes[0])
	// stmts[0] is the PG<12 transaction warning; stmts[1] and stmts[2] are the ALTER TYPE statements.
	if len(stmts) != 3 {
		t.Fatalf("expected 3 statements (1 warning + 2 ALTER TYPE), got %d: %v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "WARNING") || !strings.Contains(stmts[0], "transactional") {
		t.Errorf("expected PG<12 warning comment as first statement, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "'b'") || !strings.Contains(stmts[1], "AFTER 'a'") {
		t.Errorf("expected second stmt to add 'b' AFTER 'a', got: %s", stmts[1])
	}
	if !strings.Contains(stmts[2], "'c'") || !strings.Contains(stmts[2], "AFTER 'b'") {
		t.Errorf("expected third stmt to add 'c' AFTER 'b', got: %s", stmts[2])
	}
}

// -------------------------------------------------------------------
// SQL generation: views (PostgreSQL)
// -------------------------------------------------------------------

func TestGenerateChangeSQL_CreateView(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{activeUsersView},
	})
	changes := kit.Diff(kit.EmptySnapshot(), snap)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(snap, changes[0])
	if len(stmts) != 1 {
		t.Fatalf("expected 1 SQL statement, got %d", len(stmts))
	}
	sql := stmts[0]
	if !strings.HasPrefix(sql, "CREATE OR REPLACE VIEW") {
		t.Errorf("expected CREATE OR REPLACE VIEW, got: %s", sql)
	}
	if !strings.Contains(sql, `"active_users"`) {
		t.Errorf("expected quoted view name, got: %s", sql)
	}
	if !strings.Contains(sql, activeUsersView.SQL) {
		t.Errorf("missing view SQL body: %s", sql)
	}
}

func TestGenerateChangeSQL_DropView(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{activeUsersView},
	})
	c := kit.Change{
		Kind:       kit.ChangeDropView,
		ObjectName: "active_users",
		View:       &kit.ViewSnap{Name: "active_users"},
	}
	stmts := kit.GenerateChangeSQL(snap, c)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 SQL statement, got %d", len(stmts))
	}
	if !strings.HasPrefix(stmts[0], "DROP VIEW IF EXISTS") {
		t.Errorf("expected DROP VIEW IF EXISTS, got: %s", stmts[0])
	}
}

// -------------------------------------------------------------------
// SQL generation: enums (PostgreSQL)
// -------------------------------------------------------------------

func TestGenerateChangeSQL_CreateEnum(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{statusEnum},
	})
	changes := kit.Diff(kit.EmptySnapshot(), snap)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(snap, changes[0])
	if len(stmts) != 1 {
		t.Fatalf("expected 1 SQL statement, got %d", len(stmts))
	}
	sql := stmts[0]
	if !strings.HasPrefix(sql, "CREATE TYPE") {
		t.Errorf("expected CREATE TYPE, got: %s", sql)
	}
	if !strings.Contains(sql, "AS ENUM") {
		t.Errorf("expected AS ENUM, got: %s", sql)
	}
	if !strings.Contains(sql, "'pending'") {
		t.Errorf("expected 'pending' value, got: %s", sql)
	}
}

func TestGenerateChangeSQL_CreateEnum_EscapesSingleQuotes(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("mood", "it's fine", "great")},
	})
	changes := kit.Diff(kit.EmptySnapshot(), snap)
	stmts := kit.GenerateChangeSQL(snap, changes[0])
	if len(stmts) != 1 {
		t.Fatalf("expected 1 SQL statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], "it''s fine") {
		t.Errorf("expected escaped single quote in enum value, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQL_AlterEnum_AddValue(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("status", "pending", "active")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("status", "pending", "active", "archived")},
	})
	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(new, changes[0])
	// stmts[0] is the PG<12 warning; stmts[1] is the ALTER TYPE statement.
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements (1 warning + 1 ALTER TYPE), got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], "WARNING") {
		t.Errorf("expected PG<12 warning as first statement, got: %s", stmts[0])
	}
	sql := stmts[1]
	if !strings.HasPrefix(sql, "ALTER TYPE") {
		t.Errorf("expected ALTER TYPE, got: %s", sql)
	}
	if !strings.Contains(sql, "ADD VALUE IF NOT EXISTS") {
		t.Errorf("expected ADD VALUE IF NOT EXISTS, got: %s", sql)
	}
	if !strings.Contains(sql, "'archived'") {
		t.Errorf("expected 'archived' in ALTER TYPE, got: %s", sql)
	}
}

func TestGenerateChangeSQL_AlterEnum_MultipleValues(t *testing.T) {
	old := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("s", "a")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("s", "a", "b", "c")},
	})
	changes := kit.Diff(old, new)
	stmts := kit.GenerateChangeSQL(new, changes[0])
	// stmts[0] is the PG<12 warning; stmts[1] and stmts[2] are ALTER TYPE per added value.
	if len(stmts) != 3 {
		t.Errorf("expected 3 statements (1 warning + 2 ALTER TYPE), got %d: %v", len(stmts), stmts)
	}
}

func TestGenerateChangeSQL_AlterEnum_MiddleInsertion(t *testing.T) {
	// old=[a,c] → new=[a,b,c]: 'b' must be placed AFTER 'a', not appended to end.
	old := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("s", "a", "c")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("s", "a", "b", "c")},
	})
	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(new, changes[0])
	// stmts[0] is the PG<12 warning; stmts[1] is the ALTER TYPE statement.
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements (1 warning + 1 ALTER TYPE), got %d: %v", len(stmts), stmts)
	}
	sql := stmts[1]
	if !strings.Contains(sql, "AFTER 'a'") {
		t.Errorf("expected AFTER 'a' for middle insertion, got: %s", sql)
	}
	if !strings.Contains(sql, "'b'") {
		t.Errorf("expected value 'b' in statement, got: %s", sql)
	}
}

func TestGenerateChangeSQL_AlterEnum_PrependInsertion(t *testing.T) {
	// old=[c] → new=[a,c]: 'a' must be placed BEFORE 'c', not appended to end.
	old := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("s", "c")},
	})
	new := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{pg.CreateEnum("s", "a", "c")},
	})
	changes := kit.Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(new, changes[0])
	// stmts[0] is the PG<12 warning; stmts[1] is the ALTER TYPE statement.
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements (1 warning + 1 ALTER TYPE), got %d: %v", len(stmts), stmts)
	}
	sql := stmts[1]
	if !strings.Contains(sql, "BEFORE 'c'") {
		t.Errorf("expected BEFORE 'c' for prepend insertion, got: %s", sql)
	}
	if !strings.Contains(sql, "'a'") {
		t.Errorf("expected value 'a' in statement, got: %s", sql)
	}
}

func TestGenerateChangeSQL_DropEnum(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{statusEnum},
	})
	c := kit.Change{
		Kind:    kit.ChangeDropEnum,
		OldEnum: &kit.EnumSnap{Name: "status"},
	}
	stmts := kit.GenerateChangeSQL(snap, c)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 SQL statement, got %d", len(stmts))
	}
	if !strings.HasPrefix(stmts[0], "DROP TYPE IF EXISTS") {
		t.Errorf("expected DROP TYPE IF EXISTS, got: %s", stmts[0])
	}
}

// -------------------------------------------------------------------
// SQL generation: MySQL view stubs
// -------------------------------------------------------------------

func TestMySQLChangeSQL_CreateView(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{activeUsersView},
	})
	changes := kit.Diff(kit.EmptySnapshot(), snap)
	stmts := kit.GenerateChangeSQLMySQL(snap, changes[0])
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], "CREATE OR REPLACE VIEW") {
		t.Errorf("expected CREATE OR REPLACE VIEW for MySQL, got: %s", stmts[0])
	}
}

func TestMySQLChangeSQL_CreateEnum_IsComment(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{statusEnum},
	})
	changes := kit.Diff(kit.EmptySnapshot(), snap)
	stmts := kit.GenerateChangeSQLMySQL(snap, changes[0])
	if len(stmts) != 1 || !strings.HasPrefix(stmts[0], "--") {
		t.Errorf("expected SQL comment for MySQL enum type, got: %v", stmts)
	}
}

// -------------------------------------------------------------------
// SQL generation: SQLite view stubs
// -------------------------------------------------------------------

func TestSQLiteChangeSQL_CreateView(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Views: []*pg.ViewDef{activeUsersView},
	})
	changes := kit.Diff(kit.EmptySnapshot(), snap)
	stmts := kit.GenerateChangeSQLSQLite(snap, changes[0])
	// SQLite emits DROP VIEW IF EXISTS + CREATE VIEW (no IF NOT EXISTS) so that
	// the same change kind handles both new views and replaced views correctly.
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements (DROP + CREATE), got %d: %v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "DROP VIEW IF EXISTS") {
		t.Errorf("expected DROP VIEW IF EXISTS as first statement, got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "CREATE VIEW") || strings.Contains(stmts[1], "IF NOT EXISTS") {
		t.Errorf("expected CREATE VIEW (without IF NOT EXISTS) as second statement, got: %s", stmts[1])
	}
}

func TestSQLiteChangeSQL_CreateEnum_IsComment(t *testing.T) {
	snap := kit.FromSchema(kit.SchemaObjects{
		Enums: []*pg.EnumDef{statusEnum},
	})
	changes := kit.Diff(kit.EmptySnapshot(), snap)
	stmts := kit.GenerateChangeSQLSQLite(snap, changes[0])
	if len(stmts) != 1 || !strings.HasPrefix(stmts[0], "--") {
		t.Errorf("expected SQL comment for SQLite enum type, got: %v", stmts)
	}
}

// -------------------------------------------------------------------
// SQLiteApplyableChanges: new change kinds
// -------------------------------------------------------------------

func TestSQLiteApplyableChanges_NewChangeKinds(t *testing.T) {
	// All six new change kinds (views + enums) must pass through SQLiteApplyableChanges
	// unchanged — none of them are column-level changes that require a table rebuild.
	viewSnap := &kit.ViewSnap{Name: "v", SQL: "SELECT 1"}
	enumSnapVal := &kit.EnumSnap{Name: "status", Values: []string{"a", "b"}}

	cases := []struct {
		name   string
		change kit.Change
	}{
		{
			"ChangeCreateView",
			kit.Change{Kind: kit.ChangeCreateView, ObjectName: "v", View: viewSnap},
		},
		{
			"ChangeReplaceView",
			kit.Change{Kind: kit.ChangeReplaceView, ObjectName: "v", View: viewSnap},
		},
		{
			"ChangeDropView",
			kit.Change{Kind: kit.ChangeDropView, ObjectName: "v", View: viewSnap},
		},
		{
			"ChangeCreateEnum",
			kit.Change{Kind: kit.ChangeCreateEnum, ObjectName: "status", NewEnum: enumSnapVal},
		},
		{
			"ChangeAlterEnum",
			kit.Change{Kind: kit.ChangeAlterEnum, ObjectName: "status", OldEnum: enumSnapVal, NewEnum: enumSnapVal},
		},
		{
			"ChangeDropEnum",
			kit.Change{Kind: kit.ChangeDropEnum, ObjectName: "status", OldEnum: enumSnapVal},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := kit.SQLiteApplyableChanges([]kit.Change{tc.change})
			if len(result) != 1 {
				t.Errorf("%s: expected change to pass through SQLiteApplyableChanges, got %d results", tc.name, len(result))
			}
		})
	}
}

// -------------------------------------------------------------------
// AllChangeSQL with mixed objects
// -------------------------------------------------------------------

func TestAllChangeSQL_MixedObjects(t *testing.T) {
	new := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Views:  []*pg.ViewDef{activeUsersView},
		Enums:  []*pg.EnumDef{statusEnum},
	})
	changes := kit.Diff(kit.EmptySnapshot(), new)
	stmts := kit.AllChangeSQL(new, changes)

	hasEnum := false
	hasTable := false
	hasView := false
	for _, s := range stmts {
		if strings.Contains(s, "CREATE TYPE") {
			hasEnum = true
		}
		if strings.Contains(s, "CREATE TABLE") {
			hasTable = true
		}
		if strings.Contains(s, "CREATE OR REPLACE VIEW") {
			hasView = true
		}
	}
	if !hasEnum {
		t.Error("expected CREATE TYPE statement")
	}
	if !hasTable {
		t.Error("expected CREATE TABLE statement")
	}
	if !hasView {
		t.Error("expected CREATE OR REPLACE VIEW statement")
	}
}

func TestAllChangeSQLMySQL_MixedObjects(t *testing.T) {
	newSnap := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Views:  []*pg.ViewDef{activeUsersView},
		Enums:  []*pg.EnumDef{statusEnum},
	})
	changes := kit.Diff(kit.EmptySnapshot(), newSnap)
	stmts := kit.AllChangeSQLMySQL(newSnap, changes)

	hasTable := false
	hasView := false
	hasEnumComment := false
	for _, s := range stmts {
		if strings.Contains(s, "CREATE TABLE") {
			hasTable = true
		}
		if strings.Contains(s, "CREATE OR REPLACE VIEW") {
			hasView = true
		}
		if strings.HasPrefix(s, "--") && strings.Contains(s, "MySQL") && strings.Contains(s, "ENUM") {
			hasEnumComment = true
		}
	}
	if !hasTable {
		t.Error("expected CREATE TABLE statement in MySQL output")
	}
	if !hasView {
		t.Error("expected CREATE OR REPLACE VIEW statement in MySQL output")
	}
	if !hasEnumComment {
		t.Error("expected MySQL stub comment for enum type in output")
	}
}

func TestAllChangeSQLSQLite_MixedObjects(t *testing.T) {
	newSnap := kit.FromSchema(kit.SchemaObjects{
		Tables: []pg.TableDefiner{usersDef},
		Views:  []*pg.ViewDef{activeUsersView},
		Enums:  []*pg.EnumDef{statusEnum},
	})
	changes := kit.Diff(kit.EmptySnapshot(), newSnap)
	stmts := kit.AllChangeSQLSQLite(newSnap, changes)

	hasTable := false
	hasViewDrop := false
	hasViewCreate := false
	hasEnumComment := false
	for _, s := range stmts {
		if strings.Contains(s, "CREATE TABLE") {
			hasTable = true
		}
		if strings.HasPrefix(s, "DROP VIEW IF EXISTS") {
			hasViewDrop = true
		}
		if strings.Contains(s, "CREATE VIEW") {
			hasViewCreate = true
		}
		if strings.HasPrefix(s, "--") && strings.Contains(s, "SQLite") && strings.Contains(s, "ENUM") {
			hasEnumComment = true
		}
	}
	if !hasTable {
		t.Error("expected CREATE TABLE statement in SQLite output")
	}
	if !hasViewDrop {
		t.Error("expected DROP VIEW IF EXISTS statement in SQLite output (SQLite uses DROP+CREATE for new views)")
	}
	if !hasViewCreate {
		t.Error("expected CREATE VIEW statement in SQLite output")
	}
	if !hasEnumComment {
		t.Error("expected SQLite stub comment for enum type in output")
	}
}

// -------------------------------------------------------------------
// EmptySnapshot initialises Views and Enums
// -------------------------------------------------------------------

func TestEmptySnapshot_HasViewsAndEnums(t *testing.T) {
	snap := kit.EmptySnapshot()
	if snap.Views == nil {
		t.Error("expected Views map to be initialised")
	}
	if snap.Enums == nil {
		t.Error("expected Enums map to be initialised")
	}
}

// -------------------------------------------------------------------
// pg DSL: panic guards for empty name / sql
// -------------------------------------------------------------------

func TestSchemaCreateEnum_PanicsOnEmptyName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty name")
		}
	}()
	pg.SchemaCreateEnum("auth", "", "a", "b")
}

func TestSchemaCreateEnum_PanicsOnEmptyValue(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty value")
		}
	}()
	pg.SchemaCreateEnum("auth", "role", "admin", "", "user")
}

func TestCreateView_PanicsOnEmptyName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty name")
		}
	}()
	pg.CreateView("", "SELECT 1")
}

func TestCreateView_PanicsOnEmptySQL(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty sql")
		}
	}()
	pg.CreateView("my_view", "")
}

func TestSchemaView_PanicsOnEmptyName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty name")
		}
	}()
	pg.SchemaView("reporting", "", "SELECT 1")
}

func TestSchemaView_PanicsOnEmptySQL(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty sql")
		}
	}()
	pg.SchemaView("reporting", "my_view", "")
}

func TestCreateEnum_PanicsOnEmptyName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty name")
		}
	}()
	pg.CreateEnum("", "a", "b")
}

func TestSchemaView_PanicsOnDotInSchema(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for schema containing dot")
		}
	}()
	pg.SchemaView("a.b", "my_view", "SELECT 1")
}

func TestSchemaCreateEnum_PanicsOnDotInSchema(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for schema containing dot")
		}
	}()
	pg.SchemaCreateEnum("a.b", "status", "active", "inactive")
}

// -------------------------------------------------------------------
// Test group A — nil-guard tests for view/enum change kinds (all 3 dialects)
// -------------------------------------------------------------------

func TestGenerateChangeSQL_CreateView_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeCreateView, ObjectName: "active_users", View: nil}
	if stmts := kit.GenerateChangeSQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil View, got %v", stmts)
	}
}

func TestGenerateChangeSQL_ReplaceView_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeReplaceView, ObjectName: "active_users", View: nil}
	if stmts := kit.GenerateChangeSQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil View, got %v", stmts)
	}
}

func TestGenerateChangeSQL_DropView_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeDropView, ObjectName: "active_users", View: nil}
	if stmts := kit.GenerateChangeSQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil View, got %v", stmts)
	}
}

func TestGenerateChangeSQL_CreateEnum_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeCreateEnum, ObjectName: "status", NewEnum: nil}
	if stmts := kit.GenerateChangeSQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil NewEnum, got %v", stmts)
	}
}

func TestGenerateChangeSQL_AlterEnum_NilOldEnum_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	newEnum := &kit.EnumSnap{Name: "status", Values: []string{"a", "b"}}
	c := kit.Change{Kind: kit.ChangeAlterEnum, ObjectName: "status", OldEnum: nil, NewEnum: newEnum}
	if stmts := kit.GenerateChangeSQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil OldEnum, got %v", stmts)
	}
}

func TestGenerateChangeSQL_AlterEnum_NilNewEnum_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	oldEnum := &kit.EnumSnap{Name: "status", Values: []string{"a", "b"}}
	c := kit.Change{Kind: kit.ChangeAlterEnum, ObjectName: "status", OldEnum: oldEnum, NewEnum: nil}
	if stmts := kit.GenerateChangeSQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil NewEnum, got %v", stmts)
	}
}

func TestGenerateChangeSQL_AlterEnum_NoOp_NilGuard(t *testing.T) {
	// No-op path: added==0, removedVals==0, reordered==false → return nil
	snap := kit.EmptySnapshot()
	enum := &kit.EnumSnap{Name: "status", Values: []string{"a", "b"}}
	c := kit.Change{Kind: kit.ChangeAlterEnum, ObjectName: "status", OldEnum: enum, NewEnum: enum}
	if stmts := kit.GenerateChangeSQL(snap, c); stmts != nil {
		t.Errorf("expected nil for no-op alter enum (identical old/new), got %v", stmts)
	}
}

func TestGenerateChangeSQL_DropEnum_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeDropEnum, ObjectName: "status", OldEnum: nil}
	if stmts := kit.GenerateChangeSQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil OldEnum, got %v", stmts)
	}
}

func TestGenerateChangeSQLMySQL_CreateView_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeCreateView, ObjectName: "v", View: nil}
	if stmts := kit.GenerateChangeSQLMySQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil View, got %v", stmts)
	}
}

func TestGenerateChangeSQLMySQL_ReplaceView_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeReplaceView, ObjectName: "v", View: nil}
	if stmts := kit.GenerateChangeSQLMySQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil View, got %v", stmts)
	}
}

func TestGenerateChangeSQLMySQL_DropView_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeDropView, ObjectName: "v", View: nil}
	if stmts := kit.GenerateChangeSQLMySQL(snap, c); stmts != nil {
		t.Errorf("expected nil for nil View, got %v", stmts)
	}
}

func TestGenerateChangeSQLSQLite_CreateView_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeCreateView, ObjectName: "v", View: nil}
	if stmts := kit.GenerateChangeSQLSQLite(snap, c); stmts != nil {
		t.Errorf("expected nil for nil View, got %v", stmts)
	}
}

func TestGenerateChangeSQLSQLite_ReplaceView_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeReplaceView, ObjectName: "v", View: nil}
	if stmts := kit.GenerateChangeSQLSQLite(snap, c); stmts != nil {
		t.Errorf("expected nil for nil View, got %v", stmts)
	}
}

func TestGenerateChangeSQLSQLite_DropView_NilGuard(t *testing.T) {
	snap := kit.EmptySnapshot()
	c := kit.Change{Kind: kit.ChangeDropView, ObjectName: "v", View: nil}
	if stmts := kit.GenerateChangeSQLSQLite(snap, c); stmts != nil {
		t.Errorf("expected nil for nil View, got %v", stmts)
	}
}

// -------------------------------------------------------------------
// Test group B — MySQL/SQLite dialect tests for DropView, AlterEnum, DropEnum
// -------------------------------------------------------------------

func TestGenerateChangeSQLMySQL_DropView_EmitsDropIfExists(t *testing.T) {
	snap := kit.EmptySnapshot()
	view := &kit.ViewSnap{Name: "active_users"}
	c := kit.Change{Kind: kit.ChangeDropView, ObjectName: "active_users", View: view}
	stmts := kit.GenerateChangeSQLMySQL(snap, c)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "DROP VIEW IF EXISTS") {
		t.Errorf("expected DROP VIEW IF EXISTS, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQLMySQL_AlterEnum_IsComment(t *testing.T) {
	snap := kit.EmptySnapshot()
	oldEnum := &kit.EnumSnap{Name: "status", Values: []string{"pending", "active"}}
	newEnum := &kit.EnumSnap{Name: "status", Values: []string{"pending", "active", "archived"}}
	c := kit.Change{Kind: kit.ChangeAlterEnum, ObjectName: "status", OldEnum: oldEnum, NewEnum: newEnum}
	stmts := kit.GenerateChangeSQLMySQL(snap, c)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement (comment), got %d: %v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "--") {
		t.Errorf("expected SQL comment for MySQL AlterEnum, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQLMySQL_DropEnum_IsComment(t *testing.T) {
	snap := kit.EmptySnapshot()
	oldEnum := &kit.EnumSnap{Name: "status", Values: []string{"pending", "active"}}
	c := kit.Change{Kind: kit.ChangeDropEnum, ObjectName: "status", OldEnum: oldEnum}
	stmts := kit.GenerateChangeSQLMySQL(snap, c)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement (comment), got %d: %v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "--") {
		t.Errorf("expected SQL comment for MySQL DropEnum, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQLSQLite_DropView_EmitsDropIfExists(t *testing.T) {
	snap := kit.EmptySnapshot()
	view := &kit.ViewSnap{Name: "active_users"}
	c := kit.Change{Kind: kit.ChangeDropView, ObjectName: "active_users", View: view}
	stmts := kit.GenerateChangeSQLSQLite(snap, c)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "DROP VIEW IF EXISTS") {
		t.Errorf("expected DROP VIEW IF EXISTS, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQLSQLite_AlterEnum_IsComment(t *testing.T) {
	snap := kit.EmptySnapshot()
	oldEnum := &kit.EnumSnap{Name: "status", Values: []string{"pending", "active"}}
	newEnum := &kit.EnumSnap{Name: "status", Values: []string{"pending", "active", "archived"}}
	c := kit.Change{Kind: kit.ChangeAlterEnum, ObjectName: "status", OldEnum: oldEnum, NewEnum: newEnum}
	stmts := kit.GenerateChangeSQLSQLite(snap, c)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement (comment), got %d: %v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "--") {
		t.Errorf("expected SQL comment for SQLite AlterEnum, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQLSQLite_DropEnum_IsComment(t *testing.T) {
	snap := kit.EmptySnapshot()
	oldEnum := &kit.EnumSnap{Name: "status", Values: []string{"pending", "active"}}
	c := kit.Change{Kind: kit.ChangeDropEnum, ObjectName: "status", OldEnum: oldEnum}
	stmts := kit.GenerateChangeSQLSQLite(snap, c)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement (comment), got %d: %v", len(stmts), stmts)
	}
	if !strings.HasPrefix(stmts[0], "--") {
		t.Errorf("expected SQL comment for SQLite DropEnum, got: %s", stmts[0])
	}
}

// -------------------------------------------------------------------
// Test group C — backward-compatibility: load snapshot JSON without views/enums
// -------------------------------------------------------------------

func TestLoadJSON_BackwardCompat_NoViewsOrEnums(t *testing.T) {
	// Write a JSON snapshot that only has "version", "created_at", and "tables" — no "views" or "enums"
	dir := t.TempDir()
	path := dir + "/schema.snapshot.json"
	jsonContent := `{
		"version": "1",
		"created_at": "2024-01-01T00:00:00Z",
		"tables": {
			"users": {
				"name": "users",
				"columns": [{"name": "id", "sqltype": "uuid", "not_null": true, "primary_key": true, "has_default": true, "default_expr": "gen_random_uuid()"}]
			}
		}
	}`
	if err := os.WriteFile(path, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	loaded, err := kit.LoadJSON(path)
	if err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}
	if loaded.Views == nil {
		t.Error("expected Views to be non-nil after loading snapshot without views key")
	}
	if loaded.Enums == nil {
		t.Error("expected Enums to be non-nil after loading snapshot without enums key")
	}
	if len(loaded.Views) != 0 {
		t.Errorf("expected empty Views map, got %d entries", len(loaded.Views))
	}
	if len(loaded.Enums) != 0 {
		t.Errorf("expected empty Enums map, got %d entries", len(loaded.Enums))
	}
	// Must not panic when diffing against an empty snapshot
	_ = kit.Diff(loaded, kit.EmptySnapshot())
}

// -------------------------------------------------------------------
// Test group D — schema-qualified names through SQL generators
// -------------------------------------------------------------------

func TestGenerateChangeSQL_CreateView_SchemaQualified(t *testing.T) {
	view := pg.SchemaView("reporting", "recent_orders",
		`SELECT * FROM orders WHERE created_at > now() - interval '7 days'`)
	snap := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{view}})
	changes := kit.Diff(kit.EmptySnapshot(), snap)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(snap, changes[0])
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], `"reporting"."recent_orders"`) {
		t.Errorf("expected schema-qualified quoted name in SQL, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQL_CreateEnum_SchemaQualified(t *testing.T) {
	enum := pg.SchemaCreateEnum("auth", "role", "admin", "user", "guest")
	snap := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{enum}})
	changes := kit.Diff(kit.EmptySnapshot(), snap)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(snap, changes[0])
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], `"auth"."role"`) {
		t.Errorf("expected schema-qualified quoted name in SQL, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQL_DropView_SchemaQualified(t *testing.T) {
	view := pg.SchemaView("reporting", "recent_orders", `SELECT 1`)
	snap := kit.FromSchema(kit.SchemaObjects{Views: []*pg.ViewDef{view}})
	changes := kit.Diff(snap, kit.EmptySnapshot())
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(snap, changes[0])
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], `"reporting"."recent_orders"`) {
		t.Errorf("expected schema-qualified quoted name in DROP VIEW, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQL_DropEnum_SchemaQualified(t *testing.T) {
	enum := pg.SchemaCreateEnum("auth", "role", "admin", "user")
	snap := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{enum}})
	changes := kit.Diff(snap, kit.EmptySnapshot())
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(snap, changes[0])
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
	if !strings.Contains(stmts[0], `"auth"."role"`) {
		t.Errorf("expected schema-qualified quoted name in DROP TYPE, got: %s", stmts[0])
	}
}

func TestGenerateChangeSQL_AlterEnum_SchemaQualified(t *testing.T) {
	oldEnum := pg.SchemaCreateEnum("auth", "role", "admin", "user")
	newEnum := pg.SchemaCreateEnum("auth", "role", "admin", "user", "guest")
	oldSnap := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{oldEnum}})
	newSnap := kit.FromSchema(kit.SchemaObjects{Enums: []*pg.EnumDef{newEnum}})
	changes := kit.Diff(oldSnap, newSnap)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	stmts := kit.GenerateChangeSQL(newSnap, changes[0])
	if len(stmts) == 0 {
		t.Fatal("expected at least 1 statement")
	}
	found := false
	for _, s := range stmts {
		if strings.Contains(s, `"auth"."role"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected schema-qualified name in ALTER TYPE statements, got: %v", stmts)
	}
}

// -------------------------------------------------------------------
// Test group F — SchemaView and SchemaCreateEnum empty-schema panic tests
// -------------------------------------------------------------------

func TestSchemaView_PanicsOnEmptySchema(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty schema")
		}
	}()
	pg.SchemaView("", "my_view", "SELECT 1")
}

func TestSchemaCreateEnum_PanicsOnEmptySchema(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty schema")
		}
	}()
	pg.SchemaCreateEnum("", "status", "active", "inactive")
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func snapViewKeys(s kit.Snapshot) []string {
	keys := make([]string, 0, len(s.Views))
	for k := range s.Views {
		keys = append(keys, k)
	}
	return keys
}

func snapEnumKeys(s kit.Snapshot) []string {
	keys := make([]string, 0, len(s.Enums))
	for k := range s.Enums {
		keys = append(keys, k)
	}
	return keys
}
