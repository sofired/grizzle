package kit_test

import (
	"encoding/json"
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
	// Expect: only CREATE OR REPLACE VIEW (no preceding DROP — it is dangerous when
	// other views depend on this view and redundant since OR REPLACE handles the update).
	if len(changes) != 1 {
		t.Fatalf("expected 1 change (CREATE OR REPLACE VIEW), got %d: %v", len(changes), changes)
	}
	if changes[0].Kind != kit.ChangeCreateView {
		t.Errorf("expected ChangeCreateView, got %s", changes[0].Kind)
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
	if len(stmts) != 2 {
		t.Fatalf("expected 2 ALTER TYPE statements, got %d: %v", len(stmts), stmts)
	}
	if !strings.Contains(stmts[0], "'b'") || !strings.Contains(stmts[0], "AFTER 'a'") {
		t.Errorf("expected first stmt to add 'b' AFTER 'a', got: %s", stmts[0])
	}
	if !strings.Contains(stmts[1], "'c'") || !strings.Contains(stmts[1], "AFTER 'b'") {
		t.Errorf("expected second stmt to add 'c' AFTER 'b', got: %s", stmts[1])
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
		Kind: kit.ChangeDropView,
		View: &kit.ViewSnap{Name: "active_users"},
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
	if len(stmts) != 1 {
		t.Fatalf("expected 1 SQL statement, got %d", len(stmts))
	}
	sql := stmts[0]
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
	// One ALTER TYPE statement per added value
	if len(stmts) != 2 {
		t.Errorf("expected 2 ALTER TYPE statements, got %d: %v", len(stmts), stmts)
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
	if len(stmts) != 1 {
		t.Fatalf("expected 1 SQL statement, got %d: %v", len(stmts), stmts)
	}
	sql := stmts[0]
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
	if len(stmts) != 1 {
		t.Fatalf("expected 1 SQL statement, got %d: %v", len(stmts), stmts)
	}
	sql := stmts[0]
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
