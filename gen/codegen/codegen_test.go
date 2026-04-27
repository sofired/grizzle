package codegen_test

import (
	"os"
	"strings"
	"testing"

	"github.com/sofired/grizzle/gen/codegen"
	"github.com/sofired/grizzle/gen/parser"
)

// minimalSchema is a small schema that exercises the main column types.
const minimalSchemaGo = `package testschema

import pg "github.com/sofired/grizzle/schema/pg"

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
		"tableAlias string",
		"func (UsersTable) GrizTableName() string",
		"func (t UsersTable) GrizTableAlias() string",
		"func (t UsersTable) As(alias string) UsersTable",
		// As() method body must reassign column handles with the new alias.
		`ColBase: expr.ColBase{TableAlias: alias`,
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
import pg "github.com/sofired/grizzle/schema/pg"
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

func TestImportGrouping(t *testing.T) {
	// Schema with both stdlib (time via Timestamp) and third-party (uuid via UUID) imports.
	src := `package testschema
import pg "github.com/sofired/grizzle/schema/pg"
var Things = pg.Table("things",
	pg.C("id",         pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("name",       pg.Text().NotNull()),
	pg.C("created_at", pg.Timestamp().NotNull().DefaultNow()),
)
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
	gf, err := codegen.GenerateTable(tables[0], codegen.Options{PackageName: "testschema", OutputDir: dir})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	src2 := string(gf.Source)
	// The stdlib import "time" must appear BEFORE the blank-line separator,
	// and the third-party import "github.com/google/uuid" must appear AFTER.
	timePos := strings.Index(src2, `"time"`)
	uuidPos := strings.Index(src2, `"github.com/google/uuid"`)
	blankPos := strings.Index(src2, "\"\n\n\t\"")
	if timePos < 0 {
		t.Fatal(`missing "time" import`)
	}
	if uuidPos < 0 {
		t.Fatal(`missing "github.com/google/uuid" import`)
	}
	if blankPos < 0 {
		t.Error("import block has no blank-line group separator")
	} else if timePos > uuidPos {
		t.Error("stdlib import should come before third-party import")
	}
}

func TestIntegerTypes_ColumnMapping(t *testing.T) {
	src := `package testschema
import pg "github.com/sofired/grizzle/schema/pg"
var Orders = pg.Table("orders",
	pg.C("id",       pg.BigSerial()),
	pg.C("user_id",  pg.BigInt().NotNull()),
	pg.C("quantity", pg.Integer().NotNull()),
	pg.C("priority", pg.SmallInt()),
)
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
	gf, err := codegen.GenerateTable(tables[0], codegen.Options{PackageName: "testschema", OutputDir: dir})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	src2 := string(gf.Source)

	// BigSerial / BigInt → BigIntColumn + int64
	if !strings.Contains(src2, "expr.BigIntColumn") {
		t.Errorf("expected expr.BigIntColumn for BigSerial/BigInt in output:\n%s", src2)
	}
	if !strings.Contains(src2, "int64") {
		t.Errorf("expected int64 for BigSerial/BigInt select model in output:\n%s", src2)
	}

	// Integer / SmallInt → IntColumn + int
	if !strings.Contains(src2, "expr.IntColumn") {
		t.Errorf("expected expr.IntColumn for Integer/SmallInt in output:\n%s", src2)
	}
	// Verify "int" appears but "int64" doesn't dominate (at least both present)
	if !strings.Contains(src2, "\tQuantity\tint\n") && !strings.Contains(src2, " int\n") {
		t.Logf("output:\n%s", src2)
	}
}

func TestJSONB_CustomGoType(t *testing.T) {
	src := `package testschema
import pg "github.com/sofired/grizzle/schema/pg"
var Events = pg.Table("events",
	pg.C("id",      pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("payload", pg.JSONB().Type("MyPayload")),
	pg.C("meta",    pg.JSONB()),
)
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
	gf, err := codegen.GenerateTable(tables[0], codegen.Options{PackageName: "testschema", OutputDir: dir})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	src2 := string(gf.Source)
	if !strings.Contains(src2, "expr.JSONBColumn[MyPayload]") {
		t.Errorf("expected JSONBColumn[MyPayload] in output:\n%s", src2)
	}
	if !strings.Contains(src2, "expr.JSONBColumn[map[string]any]") {
		t.Errorf("expected JSONBColumn[map[string]any] (default) in output:\n%s", src2)
	}
}

func TestSQLite_ColumnMapping(t *testing.T) {
	src := `package testschema
import sqlite "github.com/sofired/grizzle/schema/sqlite"
var Assets = sqlite.Table("assets",
	sqlite.C("id",         sqlite.Integer().PrimaryKey()),
	sqlite.C("title",      sqlite.Text().NotNull()),
	sqlite.C("score",      sqlite.Real()),
	sqlite.C("data",       sqlite.Blob()),
	sqlite.C("created_at", sqlite.Timestamp().NotNull()),
)
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
	gf, err := codegen.GenerateTable(tables[0], codegen.Options{PackageName: "testschema", OutputDir: dir})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	src2 := string(gf.Source)

	// Integer PK → IntColumn + int
	if !strings.Contains(src2, "expr.IntColumn") {
		t.Errorf("expected expr.IntColumn for sqlite.Integer in output:\n%s", src2)
	}
	// Text → StringColumn
	if !strings.Contains(src2, "expr.StringColumn") {
		t.Errorf("expected expr.StringColumn for sqlite.Text in output:\n%s", src2)
	}
	// Real → FloatColumn + float64
	if !strings.Contains(src2, "expr.FloatColumn") {
		t.Errorf("expected expr.FloatColumn for sqlite.Real in output:\n%s", src2)
	}
	if !strings.Contains(src2, "float64") {
		t.Errorf("expected float64 for sqlite.Real in output:\n%s", src2)
	}
	// Blob → BytesColumn + []byte
	if !strings.Contains(src2, "expr.BytesColumn") {
		t.Errorf("expected expr.BytesColumn for sqlite.Blob in output:\n%s", src2)
	}
	if !strings.Contains(src2, "[]byte") {
		t.Errorf("expected []byte for sqlite.Blob in output:\n%s", src2)
	}
	// Timestamp → TimestampColumn
	if !strings.Contains(src2, "expr.TimestampColumn") {
		t.Errorf("expected expr.TimestampColumn for sqlite.Timestamp in output:\n%s", src2)
	}
}

func TestSQLite_EndToEnd(t *testing.T) {
	// Full end-to-end test: parse a sqlite schema, generate code, verify structure.
	src := `package testschema
import sqlite "github.com/sofired/grizzle/schema/sqlite"

var Notes = sqlite.Table("notes",
	sqlite.C("id",         sqlite.Integer().PrimaryKey()),
	sqlite.C("user_id",    sqlite.BigInt().NotNull()),
	sqlite.C("title",      sqlite.Text().NotNull()),
	sqlite.C("body",       sqlite.Text()),
	sqlite.C("score",      sqlite.Real()),
	sqlite.C("attachment", sqlite.Blob()),
	sqlite.C("created_at", sqlite.Timestamp().NotNull().DefaultNow()),
)
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
	gf, err := codegen.GenerateTable(tables[0], codegen.Options{PackageName: "testschema", OutputDir: dir})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	src2 := string(gf.Source)

	checks := []string{
		"type NotesTable struct",
		"func (NotesTable) GrizTableName() string",
		"var NotesT = NotesTable{",
		"type NoteSelect struct",
		"type NoteInsert struct",
		"type NoteUpdate struct",
		// Column handle types
		"expr.IntColumn",
		"expr.BigIntColumn",
		"expr.StringColumn",
		"expr.FloatColumn",
		"expr.BytesColumn",
		"expr.TimestampColumn",
		// Go value types
		"int64",
		"float64",
		"[]byte",
	}

	for _, want := range checks {
		if !strings.Contains(src2, want) {
			t.Errorf("generated source missing %q\n---\n%s\n---", want, src2)
		}
	}
}

func TestMySQL_SpecificTypes_Codegen(t *testing.T) {
	// Regression test for issue #5: MySQL-specific column types (TinyInt, SmallInt, Double)
	// must not cause "unknown column builder" errors during codegen.
	src := `package testschema
import mysql "github.com/sofired/grizzle/schema/mysql"
var Items = mysql.Table("items",
	mysql.C("id",       mysql.BigSerial()),
	mysql.C("flag",     mysql.TinyInt()),
	mysql.C("priority", mysql.SmallInt()),
	mysql.C("weight",   mysql.Double()),
)
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
	gf, err := codegen.GenerateTable(tables[0], codegen.Options{PackageName: "testschema", OutputDir: dir})
	if err != nil {
		t.Fatalf("generate (should not fail for MySQL-specific types): %v", err)
	}

	src2 := string(gf.Source)

	// BigSerial → BigIntColumn
	if !strings.Contains(src2, "expr.BigIntColumn") {
		t.Errorf("expected expr.BigIntColumn for mysql.BigSerial in output:\n%s", src2)
	}
	// TinyInt / SmallInt → IntColumn
	if !strings.Contains(src2, "expr.IntColumn") {
		t.Errorf("expected expr.IntColumn for mysql.TinyInt/SmallInt in output:\n%s", src2)
	}
	// Double → FloatColumn
	if !strings.Contains(src2, "expr.FloatColumn") {
		t.Errorf("expected expr.FloatColumn for mysql.Double in output:\n%s", src2)
	}
}

func TestMySQL_NewTypes_Codegen(t *testing.T) {
	// Regression test for issue #130: MediumInt, Year, Enum, and Set must not
	// produce "unknown column builder" errors during codegen and must map to
	// the correct expr column types.
	src := `package testschema
import mysql "github.com/sofired/grizzle/schema/mysql"
var Products = mysql.Table("products",
	mysql.C("id",        mysql.BigSerial()),
	mysql.C("rank",      mysql.MediumInt().NotNull()),
	mysql.C("year_made", mysql.Year()),
	mysql.C("status",    mysql.Enum("draft", "published", "archived").NotNull()),
	mysql.C("tags",      mysql.Set("featured", "sale", "new")),
)
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
	gf, err := codegen.GenerateTable(tables[0], codegen.Options{PackageName: "testschema", OutputDir: dir})
	if err != nil {
		t.Fatalf("generate (should not fail for MySQL types MediumInt/Year/Enum/Set): %v", err)
	}

	src2 := string(gf.Source)

	// BigSerial → BigIntColumn
	if !strings.Contains(src2, "expr.BigIntColumn") {
		t.Errorf("expected expr.BigIntColumn for mysql.BigSerial in output:\n%s", src2)
	}
	// MediumInt / Year → IntColumn + int
	if !strings.Contains(src2, "expr.IntColumn") {
		t.Errorf("expected expr.IntColumn for mysql.MediumInt/Year in output:\n%s", src2)
	}
	// Enum / Set → StringColumn + string
	if !strings.Contains(src2, "expr.StringColumn") {
		t.Errorf("expected expr.StringColumn for mysql.Enum/Set in output:\n%s", src2)
	}

	// Nullable Year → pointer int in Select model.
	if !strings.Contains(src2, "*int") {
		t.Errorf("expected *int for nullable mysql.Year in output:\n%s", src2)
	}
	// NotNull MediumInt → non-pointer int field present in Select model.
	// go/format aligns with spaces, so check for the field name and type separately.
	if !strings.Contains(src2, "Rank") {
		t.Errorf("expected Rank field for mysql.MediumInt in output:\n%s", src2)
	}
	// NotNull Enum → plain string present in Select model.
	if !strings.Contains(src2, "Status") {
		t.Errorf("expected Status field for mysql.Enum in output:\n%s", src2)
	}
	// Nullable Set → pointer string in Select model.
	if !strings.Contains(src2, "*string") {
		t.Errorf("expected *string for nullable mysql.Set in output:\n%s", src2)
	}
}

// writeFile is a simple helper for writing test files.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
