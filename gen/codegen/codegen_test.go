package codegen_test

import (
	"os"
	"strings"
	"testing"

	"github.com/grizzle-orm/grizzle/gen/codegen"
	"github.com/grizzle-orm/grizzle/gen/parser"
)

// minimalSchema is a small schema that exercises the main column types.
const minimalSchemaGo = `package testschema

import pg "github.com/grizzle-orm/grizzle/schema/pg"

var Users = pg.Table("users",
	pg.C("id",         pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("realm_id",   pg.UUID().NotNull()),
	pg.C("username",   pg.Varchar(255).NotNull()),
	pg.C("email",      pg.Varchar(255)),
	pg.C("enabled",    pg.Boolean().NotNull().Default(true)),
	pg.C("score",      pg.Numeric(10, 2)),
	pg.C("created_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
	pg.C("deleted_at", pg.Timestamp().WithTimezone()),
)
`

func TestGenerateTable_Smoke(t *testing.T) {
	// Write to a temp file, parse it, generate.
	dir := t.TempDir()
	schemaFile := dir + "/schema.go"
	if err := writeFile(schemaFile, minimalSchemaGo); err != nil {
		t.Fatalf("write schema file: %v", err)
	}

	tables, err := parser.ParseFile(schemaFile)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	t.Logf("parsed table: %s (%d cols)", tables[0].TableName, len(tables[0].Columns))

	gf, err := codegen.GenerateTable(tables[0], codegen.Options{
		PackageName: "testschema",
		OutputDir:   dir,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	src := string(gf.Source)
	t.Logf("generated %d bytes", len(src))

	// Structural assertions on the output.
	// Note: go/format aligns struct fields with spaces, so we check for the
	// type name and field name separately rather than "Field Type" as one string.
	checks := []string{
		"type UsersTable struct",
		"func (UsersTable) GRizTableName() string",
		"func (UsersTable) GRizTableAlias() string",
		"var UsersT = UsersTable{",
		"type UserSelect struct",
		"type UserInsert struct",
		"type UserUpdate struct",
		// Column handle types present in table struct
		"expr.UUIDColumn",
		"expr.StringColumn",
		"expr.BoolColumn",
		"expr.FloatColumn",
		"expr.TimestampColumn",
		// Field names present
		"ID", "RealmID", "Username", "Email", "Enabled", "Score", "CreatedAt", "DeletedAt",
		// Select model: nullable → pointer types
		`*string`,
		`*time.Time`,
		// Select model: not-null → plain types
		`uuid.UUID`,
		`time.Time`,
		// Insert model: omitempty tags
		`db:"id,omitempty"`,
		`db:"enabled,omitempty"`,
		// Insert model: required plain tags
		`db:"realm_id"`,
		`db:"username"`,
	}

	for _, want := range checks {
		if !strings.Contains(src, want) {
			t.Errorf("generated source missing %q\n---\n%s\n---", want, src)
		}
	}

	// ID should NOT be in UserUpdate (PKs excluded).
	updateStart := strings.Index(src, "type UserUpdate struct")
	if updateStart >= 0 {
		// Find the closing brace of UserUpdate.
		updateSection := src[updateStart:]
		braceClose := strings.Index(updateSection, "\n}")
		if braceClose >= 0 {
			updateBody := updateSection[:braceClose]
			// "ID " should not appear (field name ID with a space after)
			if strings.Contains(updateBody, "\tID ") {
				t.Error("UserUpdate should not contain the PK field ID")
			}
		}
	}
}

func TestGenerateTable_ConstrainedTable(t *testing.T) {
	// Ensure WithConstraints(...) is stripped properly.
	src := `package testschema
import pg "github.com/grizzle-orm/grizzle/schema/pg"
var Realms = pg.Table("realms",
	pg.C("id",   pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("name", pg.Varchar(255).NotNull()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
	return []pg.Constraint{
		pg.UniqueIndex("realms_name_idx").On(t.Col("name")).Build(),
	}
})
`
	dir := t.TempDir()
	f := dir + "/schema.go"
	if err := writeFile(f, src); err != nil {
		t.Fatal(err)
	}
	tables, err := parser.ParseFile(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if !tables[0].HasConstraints {
		t.Error("expected HasConstraints=true")
	}
	if len(tables[0].Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(tables[0].Columns))
	}
	_, err = codegen.GenerateTable(tables[0], codegen.Options{PackageName: "testschema", OutputDir: dir})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
}

func TestNamingHelpers(t *testing.T) {
	cases := []struct {
		input    string
		singular string
	}{
		{"users", "User"},
		{"realms", "Realm"},
		{"countries", "Country"},
		{"addresses", "Address"},
		{"credentials", "Credential"},
		{"admin_permission_grants", "AdminPermissionGrant"},
	}
	for _, tc := range cases {
		// We test through GenerateTable output indirectly.
		// Just verify ParsedTable → singular select model name.
		pt := &parser.ParsedTable{
			VarName:   tc.singular + "s",
			TableName: tc.input,
			Columns: []parser.ParsedColumn{
				{Name: "id", Chain: &parser.ChainResult{BasePkg: "pg", BaseFn: "UUID", Methods: []parser.MethodCall{{Name: "PrimaryKey"}, {Name: "DefaultRandom"}}}},
			},
		}
		dir := t.TempDir()
		gf, err := codegen.GenerateTable(pt, codegen.Options{PackageName: "x", OutputDir: dir})
		if err != nil {
			t.Errorf("table %q: %v", tc.input, err)
			continue
		}
		want := "type " + tc.singular + "Select struct"
		if !strings.Contains(string(gf.Source), want) {
			t.Errorf("table %q: expected %q in output:\n%s", tc.input, want, gf.Source)
		}
	}
}

// writeFile is a simple helper for writing test files.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
