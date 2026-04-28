package parser_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sofired/grizzle/gen/parser"
	pg "github.com/sofired/grizzle/schema/pg"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseSource writes src to a temp file and parses it.
func parseSource(t *testing.T, src string) []*parser.ParsedTable {
	t.Helper()
	f := filepath.Join(t.TempDir(), "schema.go")
	if err := os.WriteFile(f, []byte(src), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tables, err := parser.ParseFile(f)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	return tables
}

// oneTable asserts exactly one table was parsed.
func oneTable(t *testing.T, tables []*parser.ParsedTable) *parser.ParsedTable {
	t.Helper()
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	return tables[0]
}

// col returns the parsed column at index i.
func col(t *testing.T, tbl *parser.ParsedTable, i int) parser.ParsedColumn {
	t.Helper()
	if i >= len(tbl.Columns) {
		t.Fatalf("column index %d out of range (have %d)", i, len(tbl.Columns))
	}
	return tbl.Columns[i]
}

func methodNames(chain *parser.ChainResult) []string {
	names := make([]string, len(chain.Methods))
	for i, m := range chain.Methods {
		names[i] = m.Name
	}
	return names
}

// ---------------------------------------------------------------------------
// UnwrapChain tests
// ---------------------------------------------------------------------------

func TestUnwrapChain_BaseOnly(t *testing.T) {
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", pg.C("id", pg.UUID()))`))

	c := col(t, tbl, 0)
	ch := c.Chain
	if ch.BasePkg != "pg" {
		t.Errorf("BasePkg: got %q, want %q", ch.BasePkg, "pg")
	}
	if ch.BaseFn != "UUID" {
		t.Errorf("BaseFn: got %q, want %q", ch.BaseFn, "UUID")
	}
	if len(ch.BaseArgs) != 0 {
		t.Errorf("BaseArgs: got %v, want none", ch.BaseArgs)
	}
	if len(ch.Methods) != 0 {
		t.Errorf("Methods: got %v, want none", ch.Methods)
	}
}

func TestUnwrapChain_BaseWithIntArg(t *testing.T) {
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", pg.C("name", pg.Varchar(128)))`))

	ch := col(t, tbl, 0).Chain
	if ch.BaseFn != "Varchar" {
		t.Errorf("BaseFn: got %q, want Varchar", ch.BaseFn)
	}
	if len(ch.BaseArgs) != 1 || ch.BaseArgs[0] != int64(128) {
		t.Errorf("BaseArgs: got %v, want [128]", ch.BaseArgs)
	}
}

func TestUnwrapChain_MultipleBaseArgs(t *testing.T) {
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", pg.C("price", pg.Numeric(12, 4)))`))

	ch := col(t, tbl, 0).Chain
	if ch.BaseFn != "Numeric" {
		t.Errorf("BaseFn: got %q, want Numeric", ch.BaseFn)
	}
	if len(ch.BaseArgs) != 2 || ch.BaseArgs[0] != int64(12) || ch.BaseArgs[1] != int64(4) {
		t.Errorf("BaseArgs: got %v, want [12 4]", ch.BaseArgs)
	}
}

func TestUnwrapChain_SingleMethod(t *testing.T) {
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", pg.C("name", pg.Text().NotNull()))`))

	ch := col(t, tbl, 0).Chain
	if ch.BaseFn != "Text" {
		t.Errorf("BaseFn: got %q, want Text", ch.BaseFn)
	}
	names := methodNames(ch)
	if len(names) != 1 || names[0] != "NotNull" {
		t.Errorf("Methods: got %v, want [NotNull]", names)
	}
}

func TestUnwrapChain_MethodChainOrdering(t *testing.T) {
	// Verify methods are in left-to-right order (not reversed).
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()))`))

	names := methodNames(col(t, tbl, 0).Chain)
	want := []string{"PrimaryKey", "DefaultRandom"}
	if len(names) != 2 || names[0] != want[0] || names[1] != want[1] {
		t.Errorf("Methods: got %v, want %v", names, want)
	}
}

func TestUnwrapChain_MethodWithStringArg(t *testing.T) {
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", pg.C("status", pg.Varchar(32).NotNull().Default("active")))`))

	ch := col(t, tbl, 0).Chain
	names := methodNames(ch)
	if len(names) != 2 {
		t.Fatalf("Methods: got %v, want [NotNull Default]", names)
	}
	def := ch.Methods[1]
	if def.Name != "Default" {
		t.Errorf("method name: got %q, want Default", def.Name)
	}
	if len(def.Args) != 1 || def.Args[0] != "active" {
		t.Errorf("method args: got %v, want [active]", def.Args)
	}
}

func TestUnwrapChain_MethodWithBoolArg(t *testing.T) {
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", pg.C("enabled", pg.Boolean().NotNull().Default(true)))`))

	def := col(t, tbl, 0).Chain.Methods[1]
	if def.Args[0] != true {
		t.Errorf("Default arg: got %v, want true", def.Args[0])
	}
}

func TestUnwrapChain_MethodWithNegativeInt(t *testing.T) {
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", pg.C("offset", pg.Integer().Default(-1)))`))

	def := col(t, tbl, 0).Chain.Methods[0]
	if def.Args[0] != int64(-1) {
		t.Errorf("Default arg: got %v (%T), want -1", def.Args[0], def.Args[0])
	}
}

func TestUnwrapChain_NestedCallArg(t *testing.T) {
	// pg.OnDelete(pg.FKActionCascade) should be parsed as a nested *ChainResult.
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t",
	pg.C("realm_id", pg.UUID().NotNull().References("realms", "id", pg.OnDelete(pg.FKActionCascade))))`))

	refs := col(t, tbl, 0).Chain.Methods[1] // References
	if refs.Name != "References" {
		t.Fatalf("expected References method, got %q", refs.Name)
	}
	if len(refs.Args) < 3 {
		t.Fatalf("expected >= 3 args, got %d", len(refs.Args))
	}
	nested, ok := refs.Args[2].(*parser.ChainResult)
	if !ok {
		t.Fatalf("third arg should be *ChainResult, got %T", refs.Args[2])
	}
	if nested.BaseFn != "OnDelete" {
		t.Errorf("nested BaseFn: got %q, want OnDelete", nested.BaseFn)
	}
	if len(nested.BaseArgs) != 1 || nested.BaseArgs[0] != "pg.FKActionCascade" {
		t.Errorf("nested BaseArgs: got %v, want [pg.FKActionCascade]", nested.BaseArgs)
	}
}

// ---------------------------------------------------------------------------
// ParseFile — table structure tests
// ---------------------------------------------------------------------------

func TestParseFile_SimpleTable(t *testing.T) {
	tables := parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var Users = pg.Table("users",
	pg.C("id",   pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("name", pg.Text().NotNull()),
)`)

	tbl := oneTable(t, tables)
	if tbl.VarName != "Users" {
		t.Errorf("VarName: got %q, want Users", tbl.VarName)
	}
	if tbl.TableName != "users" {
		t.Errorf("TableName: got %q, want users", tbl.TableName)
	}
	if tbl.SchemaName != "" {
		t.Errorf("SchemaName: got %q, want empty", tbl.SchemaName)
	}
	if tbl.HasConstraints {
		t.Error("HasConstraints: got true, want false")
	}
	if len(tbl.Columns) != 2 {
		t.Errorf("Columns: got %d, want 2", len(tbl.Columns))
	}
	if tbl.Columns[0].Name != "id" || tbl.Columns[1].Name != "name" {
		t.Errorf("column names: got [%s %s], want [id name]",
			tbl.Columns[0].Name, tbl.Columns[1].Name)
	}
}

func TestParseFile_SchemaTable(t *testing.T) {
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.SchemaTable("public", "events", pg.C("id", pg.UUID().PrimaryKey()))`))

	if tbl.SchemaName != "public" {
		t.Errorf("SchemaName: got %q, want public", tbl.SchemaName)
	}
	if tbl.TableName != "events" {
		t.Errorf("TableName: got %q, want events", tbl.TableName)
	}
}

func TestParseFile_WithConstraints(t *testing.T) {
	tbl := oneTable(t, parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", pg.C("id", pg.UUID().PrimaryKey())).WithConstraints(func(t pg.TableRef) []pg.Constraint {
	return []pg.Constraint{pg.UniqueIndex("idx").On(t.Col("id")).Build()}
})`))

	if !tbl.HasConstraints {
		t.Error("HasConstraints: got false, want true")
	}
	if len(tbl.Columns) != 1 {
		t.Errorf("Columns: got %d, want 1", len(tbl.Columns))
	}
}

func TestParseFile_MultipleTables(t *testing.T) {
	tables := parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var Realms = pg.Table("realms", pg.C("id", pg.UUID().PrimaryKey()))
var Users  = pg.Table("users",  pg.C("id", pg.UUID().PrimaryKey()))`)

	if len(tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(tables))
	}
	names := map[string]bool{tables[0].TableName: true, tables[1].TableName: true}
	if !names["realms"] || !names["users"] {
		t.Errorf("unexpected table names: %v", names)
	}
}

func TestParseFile_NonTableVarIgnored(t *testing.T) {
	tables := parseSource(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var Foo = 42
var Bar = "hello"
var T = pg.Table("t", pg.C("id", pg.UUID().PrimaryKey()))`)

	if len(tables) != 1 {
		t.Errorf("expected 1 table, got %d (non-table vars should be ignored)", len(tables))
	}
}

func TestParseDir_SkipsTestAndGenFiles(t *testing.T) {
	dir := t.TempDir()
	src := `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", pg.C("id", pg.UUID().PrimaryKey()))`

	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("schema.go", src)      // should be parsed
	write("schema_test.go", src) // should be skipped
	write("schema_gen.go", src)  // should be skipped
	write("other_gen.go", src)   // should be skipped

	tables, err := parser.ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	if len(tables) != 1 {
		t.Errorf("expected 1 table (only schema.go), got %d", len(tables))
	}
}

// ---------------------------------------------------------------------------
// EvalTable — column type mapping
// ---------------------------------------------------------------------------

func evalOne(t *testing.T, colDecl string) pg.ColumnDef {
	t.Helper()
	src := `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table("t", ` + colDecl + `)`
	tbl := oneTable(t, parseSource(t, src))
	td, err := parser.EvalTable(tbl)
	if err != nil {
		t.Fatalf("EvalTable: %v", err)
	}
	def := td.Def()
	if len(def.Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(def.Columns))
	}
	return def.Columns[0]
}

func TestEvalTable_UUID(t *testing.T) {
	c := evalOne(t, `pg.C("id", pg.UUID().PrimaryKey().DefaultRandom())`)
	if c.SQLType != "uuid" {
		t.Errorf("SQLType: got %q, want uuid", c.SQLType)
	}
	if !c.PrimaryKey {
		t.Error("PrimaryKey: want true")
	}
	if c.DefaultExpr != "gen_random_uuid()" {
		t.Errorf("DefaultExpr: got %q, want gen_random_uuid()", c.DefaultExpr)
	}
}

func TestEvalTable_Varchar(t *testing.T) {
	c := evalOne(t, `pg.C("name", pg.Varchar(64).NotNull())`)
	if c.SQLType != "varchar(64)" {
		t.Errorf("SQLType: got %q, want varchar(64)", c.SQLType)
	}
	if !c.NotNull {
		t.Error("NotNull: want true")
	}
}

func TestEvalTable_VarcharDefaultLength(t *testing.T) {
	c := evalOne(t, `pg.C("name", pg.Varchar())`)
	if c.SQLType != "varchar(255)" {
		t.Errorf("SQLType: got %q, want varchar(255)", c.SQLType)
	}
}

func TestEvalTable_Text(t *testing.T) {
	c := evalOne(t, `pg.C("bio", pg.Text())`)
	if c.SQLType != "text" {
		t.Errorf("SQLType: got %q, want text", c.SQLType)
	}
}

func TestEvalTable_Boolean(t *testing.T) {
	c := evalOne(t, `pg.C("active", pg.Boolean().NotNull().Default(false))`)
	if c.SQLType != "boolean" {
		t.Errorf("SQLType: got %q, want boolean", c.SQLType)
	}
	if c.DefaultExpr != "false" {
		t.Errorf("DefaultExpr: got %q, want false", c.DefaultExpr)
	}
}

func TestEvalTable_IntegerTypes(t *testing.T) {
	for _, tc := range []struct {
		decl    string
		sqlType string
	}{
		{`pg.C("n", pg.Integer())`, "integer"},
		{`pg.C("n", pg.BigInt())`, "bigint"},
		{`pg.C("n", pg.SmallInt())`, "smallint"},
		{`pg.C("n", pg.Serial())`, "serial"},
		{`pg.C("n", pg.BigSerial())`, "bigserial"},
	} {
		t.Run(tc.sqlType, func(t *testing.T) {
			c := evalOne(t, tc.decl)
			if c.SQLType != tc.sqlType {
				t.Errorf("SQLType: got %q, want %q", c.SQLType, tc.sqlType)
			}
		})
	}
}

func TestEvalTable_Numeric(t *testing.T) {
	c := evalOne(t, `pg.C("price", pg.Numeric(10, 2))`)
	if c.SQLType != "numeric(10,2)" {
		t.Errorf("SQLType: got %q, want numeric(10,2)", c.SQLType)
	}
}

func TestEvalTable_Timestamp(t *testing.T) {
	c := evalOne(t, `pg.C("created_at", pg.Timestamp().NotNull().DefaultNow())`)
	if c.SQLType != "timestamp" {
		t.Errorf("SQLType: got %q, want timestamp", c.SQLType)
	}
	if c.DefaultExpr != "now()" {
		t.Errorf("DefaultExpr: got %q, want now()", c.DefaultExpr)
	}
}

func TestEvalTable_TimestampWithTimezone(t *testing.T) {
	c := evalOne(t, `pg.C("ts", pg.Timestamp().WithTimezone())`)
	if c.SQLType != "timestamptz" {
		t.Errorf("SQLType: got %q, want timestamptz", c.SQLType)
	}
}

func TestEvalTable_JSONB(t *testing.T) {
	c := evalOne(t, `pg.C("meta", pg.JSONB())`)
	if c.SQLType != "jsonb" {
		t.Errorf("SQLType: got %q, want jsonb", c.SQLType)
	}
}

func TestEvalTable_Default_StringLiteral(t *testing.T) {
	c := evalOne(t, `pg.C("status", pg.Varchar(32).Default("pending"))`)
	if c.DefaultExpr != "'pending'" {
		t.Errorf("DefaultExpr: got %q, want 'pending'", c.DefaultExpr)
	}
}

func TestEvalTable_Default_IntLiteral(t *testing.T) {
	c := evalOne(t, `pg.C("count", pg.Integer().Default(0))`)
	if c.DefaultExpr != "0" {
		t.Errorf("DefaultExpr: got %q, want 0", c.DefaultExpr)
	}
}

func TestEvalTable_Default_FloatLiteral(t *testing.T) {
	c := evalOne(t, `pg.C("rate", pg.Numeric(5,2).Default(1.5))`)
	if c.DefaultExpr != "1.5" {
		t.Errorf("DefaultExpr: got %q, want 1.5", c.DefaultExpr)
	}
}

func TestEvalTable_DefaultEmpty(t *testing.T) {
	c := evalOne(t, `pg.C("tags", pg.JSONB().DefaultEmpty())`)
	if c.DefaultExpr != "'{}'::jsonb" {
		t.Errorf("DefaultExpr: got %q, want '{}'::jsonb", c.DefaultExpr)
	}
}

func TestEvalTable_DefaultEmptyArray(t *testing.T) {
	c := evalOne(t, `pg.C("items", pg.JSONB().DefaultEmptyArray())`)
	if c.DefaultExpr != "'[]'::jsonb" {
		t.Errorf("DefaultExpr: got %q, want '[]'::jsonb", c.DefaultExpr)
	}
}

func TestEvalTable_Array_DefaultEmpty(t *testing.T) {
	c := evalOne(t, `pg.C("tags", pg.Array(pg.Text()).DefaultEmpty())`)
	if c.DefaultExpr != "ARRAY[]::text[]" {
		t.Errorf("DefaultExpr: got %q, want \"ARRAY[]::text[]\"", c.DefaultExpr)
	}
}

func TestEvalTable_Array_DefaultEmpty_Int(t *testing.T) {
	c := evalOne(t, `pg.C("scores", pg.Array(pg.Integer()).DefaultEmpty())`)
	if c.DefaultExpr != "ARRAY[]::integer[]" {
		t.Errorf("DefaultExpr: got %q, want \"ARRAY[]::integer[]\"", c.DefaultExpr)
	}
}

func TestEvalTable_Unique(t *testing.T) {
	c := evalOne(t, `pg.C("email", pg.Varchar(255).NotNull().Unique())`)
	if !c.Unique {
		t.Error("Unique: want true")
	}
}

func TestEvalTable_Serial_HasDefault(t *testing.T) {
	c := evalOne(t, `pg.C("seq", pg.Serial())`)
	if !c.HasDefault {
		t.Error("Serial should set HasDefault=true")
	}
}

func TestEvalTable_References_WithOnDelete(t *testing.T) {
	c := evalOne(t, `pg.C("realm_id", pg.UUID().NotNull().References("realms", "id", pg.OnDelete(pg.FKActionCascade)))`)
	if c.References == nil {
		t.Fatal("References: want non-nil FKRef")
	}
	if c.References.Table != "realms" {
		t.Errorf("FK Table: got %q, want realms", c.References.Table)
	}
	if c.References.Column != "id" {
		t.Errorf("FK Column: got %q, want id", c.References.Column)
	}
	if c.References.OnDelete != pg.FKActionCascade {
		t.Errorf("OnDelete: got %v, want FKActionCascade", c.References.OnDelete)
	}
}

func TestEvalTable_References_WithOnUpdate(t *testing.T) {
	c := evalOne(t, `pg.C("x", pg.UUID().References("other", "id", pg.OnUpdate(pg.FKActionSetNull)))`)
	if c.References == nil {
		t.Fatal("References: want non-nil FKRef")
	}
	if c.References.OnUpdate != pg.FKActionSetNull {
		t.Errorf("OnUpdate: got %v, want FKActionSetNull", c.References.OnUpdate)
	}
}

func TestEvalTable_UnknownMethod_Silently_Ignored(t *testing.T) {
	// Methods not known to the evaluator should be silently skipped,
	// not cause an error.
	c := evalOne(t, `pg.C("x", pg.Text().NotNull().FutureModifier())`)
	if c.SQLType != "text" {
		t.Errorf("SQLType: got %q, want text", c.SQLType)
	}
	if !c.NotNull {
		t.Error("NotNull should still be set despite unknown method after it")
	}
}

func TestEvalTable_SchemaName_Propagated(t *testing.T) {
	src := `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.SchemaTable("audit", "logs", pg.C("id", pg.UUID().PrimaryKey()))`
	tbl := oneTable(t, parseSource(t, src))
	td, err := parser.EvalTable(tbl)
	if err != nil {
		t.Fatalf("EvalTable: %v", err)
	}
	def := td.Def()
	if def.Schema != "audit" {
		t.Errorf("Schema: got %q, want audit", def.Schema)
	}
	if def.Name != "logs" {
		t.Errorf("Name: got %q, want logs", def.Name)
	}
}

func TestParseFile_MysqlTable(t *testing.T) {
	src := `package s
import mysql "github.com/sofired/grizzle/schema/mysql"
var Orders = mysql.Table("orders",
	mysql.C("id",       mysql.BigSerial()),
	mysql.C("user_id",  mysql.BigInt().NotNull()),
	mysql.C("quantity", mysql.Integer().NotNull()),
)`
	tables := parseSource(t, src)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	tbl := tables[0]
	if tbl.TableName != "orders" {
		t.Errorf("TableName: got %q, want orders", tbl.TableName)
	}
	if len(tbl.Columns) != 3 {
		t.Errorf("Columns: got %d, want 3", len(tbl.Columns))
	}
}

func TestParseFile_MysqlSpecificTypes(t *testing.T) {
	src := `package s
import mysql "github.com/sofired/grizzle/schema/mysql"
var T = mysql.Table("items",
	mysql.C("id",       mysql.BigSerial()),
	mysql.C("flag",     mysql.TinyInt()),
	mysql.C("priority", mysql.SmallInt()),
	mysql.C("weight",   mysql.Double()),
)`
	tables := parseSource(t, src)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	td, err := parser.EvalTable(tables[0])
	if err != nil {
		t.Fatalf("EvalTable: %v", err)
	}
	// Verify EvalTable returns the correct dialect type for MySQL tables.
	if td.Dialect() != "mysql" {
		t.Errorf("Dialect: got %q, want mysql", td.Dialect())
	}
	def := td.Def()
	if len(def.Columns) != 4 {
		t.Errorf("expected 4 columns, got %d", len(def.Columns))
	}
}

func TestParseFile_SqliteTable(t *testing.T) {
	src := `package s
import sqlite "github.com/sofired/grizzle/schema/sqlite"
var Notes = sqlite.Table("notes",
	sqlite.C("id",    sqlite.Integer().PrimaryKey()),
	sqlite.C("title", sqlite.Text().NotNull()),
	sqlite.C("body",  sqlite.Text()),
)`
	tables := parseSource(t, src)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	tbl := tables[0]
	if tbl.TableName != "notes" {
		t.Errorf("TableName: got %q, want notes", tbl.TableName)
	}
	if len(tbl.Columns) != 3 {
		t.Errorf("Columns: got %d, want 3", len(tbl.Columns))
	}
}

func TestParseFile_SqliteSpecificTypes(t *testing.T) {
	src := `package s
import sqlite "github.com/sofired/grizzle/schema/sqlite"
var T = sqlite.Table("assets",
	sqlite.C("id",    sqlite.Integer().PrimaryKey()),
	sqlite.C("score", sqlite.Real()),
	sqlite.C("data",  sqlite.Blob()),
)`
	tables := parseSource(t, src)
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	td, err := parser.EvalTable(tables[0])
	if err != nil {
		t.Fatalf("EvalTable: %v", err)
	}
	// Verify EvalTable returns the correct dialect type for SQLite tables.
	if td.Dialect() != "sqlite" {
		t.Errorf("Dialect: got %q, want sqlite", td.Dialect())
	}
	def := td.Def()
	if len(def.Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(def.Columns))
	}
	// id: INTEGER
	if def.Columns[0].SQLType != "integer" {
		t.Errorf("id SQLType: got %q, want integer", def.Columns[0].SQLType)
	}
	// score: REAL
	if def.Columns[1].SQLType != "real" {
		t.Errorf("score SQLType: got %q, want real", def.Columns[1].SQLType)
	}
	// data: BLOB
	if def.Columns[2].SQLType != "blob" {
		t.Errorf("data SQLType: got %q, want blob", def.Columns[2].SQLType)
	}
}

func TestParseFile_SqliteSchemaTable(t *testing.T) {
	tbl := oneTable(t, parseSource(t, `package s
import sqlite "github.com/sofired/grizzle/schema/sqlite"
var T = sqlite.SchemaTable("main", "events", sqlite.C("id", sqlite.Integer().PrimaryKey()))`))

	if tbl.SchemaName != "main" {
		t.Errorf("SchemaName: got %q, want main", tbl.SchemaName)
	}
	if tbl.TableName != "events" {
		t.Errorf("TableName: got %q, want events", tbl.TableName)
	}
}

// ---------------------------------------------------------------------------
// Error message dialect prefix tests
// ---------------------------------------------------------------------------

// parseSourceExpectError writes src to a temp file, parses it, and asserts
// that the returned error contains the expected substring.
func parseSourceExpectError(t *testing.T, src, wantErrContains string) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "schema.go")
	if err := os.WriteFile(f, []byte(src), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := parser.ParseFile(f)
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantErrContains)
	}
	if !strings.Contains(err.Error(), wantErrContains) {
		t.Errorf("error %q does not contain %q", err.Error(), wantErrContains)
	}
}

func TestParseFile_ErrorMsg_PgTable_NoArgs(t *testing.T) {
	parseSourceExpectError(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.Table()`, "pg.Table:")
}

func TestParseFile_ErrorMsg_MysqlTable_NoArgs(t *testing.T) {
	parseSourceExpectError(t, `package s
import mysql "github.com/sofired/grizzle/schema/mysql"
var T = mysql.Table()`, "mysql.Table:")
}

func TestParseFile_ErrorMsg_SqliteTable_NoArgs(t *testing.T) {
	parseSourceExpectError(t, `package s
import sqlite "github.com/sofired/grizzle/schema/sqlite"
var T = sqlite.Table()`, "sqlite.Table:")
}

func TestParseFile_ErrorMsg_PgSchemaTable_TooFewArgs(t *testing.T) {
	parseSourceExpectError(t, `package s
import pg "github.com/sofired/grizzle/schema/pg"
var T = pg.SchemaTable("only_one_arg")`, "pg.SchemaTable:")
}

func TestParseFile_ErrorMsg_MysqlSchemaTable_TooFewArgs(t *testing.T) {
	parseSourceExpectError(t, `package s
import mysql "github.com/sofired/grizzle/schema/mysql"
var T = mysql.SchemaTable("only_one_arg")`, "mysql.SchemaTable:")
}

func TestParseFile_ErrorMsg_SqliteSchemaTable_TooFewArgs(t *testing.T) {
	parseSourceExpectError(t, `package s
import sqlite "github.com/sofired/grizzle/schema/sqlite"
var T = sqlite.SchemaTable("only_one_arg")`, "sqlite.SchemaTable:")
}

func TestParseFile_ErrorMsg_MysqlTable_NotPg(t *testing.T) {
	// Ensure a mysql.Table error does NOT contain "pg." to verify we're not
	// regressing to the hardcoded prefix.
	f := filepath.Join(t.TempDir(), "schema.go")
	src := `package s
import mysql "github.com/sofired/grizzle/schema/mysql"
var T = mysql.Table()`
	if err := os.WriteFile(f, []byte(src), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := parser.ParseFile(f)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "pg.") {
		t.Errorf("mysql error message should not contain pg. prefix, got: %q", err.Error())
	}
}

func TestParseFile_ErrorMsg_SqliteTable_NotPg(t *testing.T) {
	// Ensure a sqlite.Table error does NOT contain "pg." to verify we're not
	// regressing to the hardcoded prefix.
	f := filepath.Join(t.TempDir(), "schema.go")
	src := `package s
import sqlite "github.com/sofired/grizzle/schema/sqlite"
var T = sqlite.Table()`
	if err := os.WriteFile(f, []byte(src), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	_, err := parser.ParseFile(f)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), "pg.") {
		t.Errorf("sqlite error message should not contain pg. prefix, got: %q", err.Error())
	}
}

// TestEvalTable_MySQL_OnDelete verifies that mysql.OnDelete(mysql.FKActionCascade)
// is correctly parsed — fixing the DEVIATION:BROKEN where BasePkg != "pg" caused
// FK options to be silently dropped for non-PostgreSQL schemas (#156, #114).
func TestEvalTable_MySQL_OnDelete(t *testing.T) {
	src := `package s
import mysql "github.com/sofired/grizzle/schema/mysql"
var T = mysql.Table("t", mysql.C("realm_id", mysql.UUID().NotNull().References("realms", "id", mysql.OnDelete(mysql.FKActionCascade))))`
	tbl := oneTable(t, parseSource(t, src))
	td, err := parser.EvalTable(tbl)
	if err != nil {
		t.Fatalf("EvalTable: %v", err)
	}
	// Verify EvalTable returns mysql.TableDef for MySQL schema tables.
	if td.Dialect() != "mysql" {
		t.Errorf("Dialect: got %q, want mysql", td.Dialect())
	}
	c := td.Def().Columns[0]
	if c.References == nil {
		t.Fatal("References: want non-nil FKRef")
	}
	if c.References.OnDelete != pg.FKActionCascade {
		t.Errorf("OnDelete: got %v, want FKActionCascade", c.References.OnDelete)
	}
}

// TestEvalTable_SQLite_OnDelete verifies that sqlite.OnDelete(sqlite.FKActionRestrict)
// is correctly parsed for SQLite schemas (#156, #114).
func TestEvalTable_SQLite_OnDelete(t *testing.T) {
	src := `package s
import sqlite "github.com/sofired/grizzle/schema/sqlite"
var T = sqlite.Table("t", sqlite.C("parent_id", sqlite.Integer().References("parents", "id", sqlite.OnDelete(sqlite.FKActionRestrict))))`
	tbl := oneTable(t, parseSource(t, src))
	td, err := parser.EvalTable(tbl)
	if err != nil {
		t.Fatalf("EvalTable: %v", err)
	}
	// Verify EvalTable returns sqlite.TableDef for SQLite schema tables.
	if td.Dialect() != "sqlite" {
		t.Errorf("Dialect: got %q, want sqlite", td.Dialect())
	}
	c := td.Def().Columns[0]
	if c.References == nil {
		t.Fatal("References: want non-nil FKRef")
	}
	if c.References.OnDelete != pg.FKActionRestrict {
		t.Errorf("OnDelete: got %v, want FKActionRestrict", c.References.OnDelete)
	}
}

// TestParsedTable_Dialect verifies that the Dialect field is populated from the
// schema package name (pg, mysql, sqlite) for each dialect (#156).
func TestParsedTable_Dialect(t *testing.T) {
	cases := []struct {
		src     string
		dialect string
	}{
		{
			src:     "package s\nimport pg \"github.com/sofired/grizzle/schema/pg\"\nvar T = pg.Table(\"t\", pg.C(\"id\", pg.UUID().PrimaryKey()))",
			dialect: "pg",
		},
		{
			src:     "package s\nimport mysql \"github.com/sofired/grizzle/schema/mysql\"\nvar T = mysql.Table(\"t\", mysql.C(\"id\", mysql.UUID().PrimaryKey()))",
			dialect: "mysql",
		},
		{
			src:     "package s\nimport sqlite \"github.com/sofired/grizzle/schema/sqlite\"\nvar T = sqlite.Table(\"t\", sqlite.C(\"id\", sqlite.Integer().PrimaryKey()))",
			dialect: "sqlite",
		},
	}
	for _, c := range cases {
		tbl := oneTable(t, parseSource(t, c.src))
		if tbl.Dialect != c.dialect {
			t.Errorf("Dialect: got %q, want %q", tbl.Dialect, c.dialect)
		}
	}
}

// TestEvalTable_MySQL_OnUpdate verifies that mysql.OnUpdate(mysql.FKActionCascade)
// is correctly parsed for MySQL schemas (#156, #114).
func TestEvalTable_MySQL_OnUpdate(t *testing.T) {
	src := `package s
import mysql "github.com/sofired/grizzle/schema/mysql"
var T = mysql.Table("t", mysql.C("ref_id", mysql.UUID().References("other", "id", mysql.OnUpdate(mysql.FKActionSetNull))))`
	tbl := oneTable(t, parseSource(t, src))
	td, err := parser.EvalTable(tbl)
	if err != nil {
		t.Fatalf("EvalTable: %v", err)
	}
	c := td.Def().Columns[0]
	if c.References == nil {
		t.Fatal("References: want non-nil FKRef")
	}
	if c.References.OnUpdate != pg.FKActionSetNull {
		t.Errorf("OnUpdate: got %v, want FKActionSetNull", c.References.OnUpdate)
	}
}

// ---------------------------------------------------------------------------
// MySQL Enum/Set EvalTable validation tests
// ---------------------------------------------------------------------------

func TestEvalTable_MysqlEnum_Valid(t *testing.T) {
	src := `package s
import mysql "github.com/sofired/grizzle/schema/mysql"
var T = mysql.Table("t", mysql.C("status", mysql.Enum("active", "inactive")))`
	tables := parseSource(t, src)
	td, err := parser.EvalTable(tables[0])
	if err != nil {
		t.Fatalf("EvalTable: unexpected error: %v", err)
	}
	want := "enum('active','inactive')"
	if td.Def().Columns[0].SQLType != want {
		t.Errorf("SQLType: got %q, want %q", td.Def().Columns[0].SQLType, want)
	}
}

func TestEvalTable_MysqlSet_Valid(t *testing.T) {
	src := `package s
import mysql "github.com/sofired/grizzle/schema/mysql"
var T = mysql.Table("t", mysql.C("perms", mysql.Set("read", "write")))`
	tables := parseSource(t, src)
	td, err := parser.EvalTable(tables[0])
	if err != nil {
		t.Fatalf("EvalTable: unexpected error: %v", err)
	}
	want := "set('read','write')"
	if td.Def().Columns[0].SQLType != want {
		t.Errorf("SQLType: got %q, want %q", td.Def().Columns[0].SQLType, want)
	}
}

func TestEvalTable_MysqlEnum_EmptyArgs_ReturnsError(t *testing.T) {
	// Simulate a parsed column with Enum but no args (e.g. from a schema snapshot).
	pt := &parser.ParsedTable{
		TableName: "t",
		Columns: []parser.ParsedColumn{
			{
				Name: "status",
				Chain: &parser.ChainResult{
					BasePkg:  "mysql",
					BaseFn:   "Enum",
					BaseArgs: []any{},
				},
			},
		},
	}
	_, err := parser.EvalTable(pt)
	if err == nil {
		t.Fatal("expected error for Enum with zero args, got nil")
	}
}

func TestEvalTable_MysqlSet_EmptyArgs_ReturnsError(t *testing.T) {
	pt := &parser.ParsedTable{
		TableName: "t",
		Columns: []parser.ParsedColumn{
			{
				Name: "perms",
				Chain: &parser.ChainResult{
					BasePkg:  "mysql",
					BaseFn:   "Set",
					BaseArgs: []any{},
				},
			},
		},
	}
	_, err := parser.EvalTable(pt)
	if err == nil {
		t.Fatal("expected error for Set with zero args, got nil")
	}
}

func TestEvalTable_MysqlEnum_NonStringArg_ReturnsError(t *testing.T) {
	pt := &parser.ParsedTable{
		TableName: "t",
		Columns: []parser.ParsedColumn{
			{
				Name: "status",
				Chain: &parser.ChainResult{
					BasePkg:  "mysql",
					BaseFn:   "Enum",
					BaseArgs: []any{int64(1)},
				},
			},
		},
	}
	_, err := parser.EvalTable(pt)
	if err == nil {
		t.Fatal("expected error for Enum with non-string arg, got nil")
	}
}

func TestEvalTable_MysqlSet_NonStringArg_ReturnsError(t *testing.T) {
	pt := &parser.ParsedTable{
		TableName: "t",
		Columns: []parser.ParsedColumn{
			{
				Name: "perms",
				Chain: &parser.ChainResult{
					BasePkg:  "mysql",
					BaseFn:   "Set",
					BaseArgs: []any{int64(42)},
				},
			},
		},
	}
	_, err := parser.EvalTable(pt)
	if err == nil {
		t.Fatal("expected error for Set with non-string arg, got nil")
	}
}

// ---------------------------------------------------------------------------
// EvalTable — PostgreSQL builders added in this PR
// ---------------------------------------------------------------------------

func TestEvalTable_PG_Date(t *testing.T) {
	c := evalOne(t, `pg.C("created", pg.Date().NotNull())`)
	if c.SQLType != "date" {
		t.Errorf("SQLType = %q, want date", c.SQLType)
	}
	if c.GoType != pg.GoTypeTime {
		t.Errorf("GoType = %q, want %q", c.GoType, pg.GoTypeTime)
	}
}

func TestEvalTable_PG_Time(t *testing.T) {
	c := evalOne(t, `pg.C("slot", pg.Time())`)
	if c.SQLType != "time" {
		t.Errorf("SQLType = %q, want time", c.SQLType)
	}
}

func TestEvalTable_PG_TimeWithTimezone(t *testing.T) {
	c := evalOne(t, `pg.C("slot", pg.Time().WithTimezone())`)
	if c.SQLType != "timetz" {
		t.Errorf("SQLType = %q, want timetz", c.SQLType)
	}
}

func TestEvalTable_PG_Interval(t *testing.T) {
	c := evalOne(t, `pg.C("dur", pg.Interval())`)
	if c.SQLType != "interval" {
		t.Errorf("SQLType = %q, want interval", c.SQLType)
	}
	if c.GoType != pg.GoTypeString {
		t.Errorf("GoType = %q, want string", c.GoType)
	}
}

func TestEvalTable_PG_DoublePrecision(t *testing.T) {
	c := evalOne(t, `pg.C("score", pg.DoublePrecision())`)
	if c.SQLType != "double precision" {
		t.Errorf("SQLType = %q, want double precision", c.SQLType)
	}
	if c.GoType != pg.GoTypeFloat64 {
		t.Errorf("GoType = %q, want float64", c.GoType)
	}
}

func TestEvalTable_PG_Char(t *testing.T) {
	c := evalOne(t, `pg.C("code", pg.Char(3))`)
	if c.SQLType != "char(3)" {
		t.Errorf("SQLType = %q, want char(3)", c.SQLType)
	}
}

func TestEvalTable_PG_Bytea(t *testing.T) {
	c := evalOne(t, `pg.C("data", pg.Bytea())`)
	if c.SQLType != "bytea" {
		t.Errorf("SQLType = %q, want bytea", c.SQLType)
	}
	if c.GoType != pg.GoTypeByteSlice {
		t.Errorf("GoType = %q, want []byte", c.GoType)
	}
}

func TestEvalTable_PG_NetworkTypes(t *testing.T) {
	for _, tc := range []struct{ decl, sqlType string }{
		{`pg.C("ip", pg.Inet())`, "inet"},
		{`pg.C("net", pg.Cidr())`, "cidr"},
		{`pg.C("mac", pg.Macaddr())`, "macaddr"},
	} {
		t.Run(tc.sqlType, func(t *testing.T) {
			c := evalOne(t, tc.decl)
			if c.SQLType != tc.sqlType {
				t.Errorf("SQLType = %q, want %q", c.SQLType, tc.sqlType)
			}
		})
	}
}

func TestEvalTable_PG_TextSearch(t *testing.T) {
	for _, tc := range []struct{ decl, sqlType string }{
		{`pg.C("doc", pg.Tsvector())`, "tsvector"},
		{`pg.C("q", pg.Tsquery())`, "tsquery"},
	} {
		t.Run(tc.sqlType, func(t *testing.T) {
			c := evalOne(t, tc.decl)
			if c.SQLType != tc.sqlType {
				t.Errorf("SQLType = %q, want %q", c.SQLType, tc.sqlType)
			}
		})
	}
}

func TestEvalTable_PG_RangeTypes(t *testing.T) {
	for _, tc := range []struct{ decl, sqlType string }{
		{`pg.C("r", pg.Int4Range())`, "int4range"},
		{`pg.C("r", pg.Int8Range())`, "int8range"},
		{`pg.C("r", pg.NumRange())`, "numrange"},
		{`pg.C("r", pg.TsRange())`, "tsrange"},
		{`pg.C("r", pg.TstzRange())`, "tstzrange"},
		{`pg.C("r", pg.DateRange())`, "daterange"},
	} {
		t.Run(tc.sqlType, func(t *testing.T) {
			c := evalOne(t, tc.decl)
			if c.SQLType != tc.sqlType {
				t.Errorf("SQLType = %q, want %q", c.SQLType, tc.sqlType)
			}
			if c.GoType != pg.GoTypeString {
				t.Errorf("GoType = %q, want string", c.GoType)
			}
		})
	}
}

func TestEvalTable_PG_Array(t *testing.T) {
	c := evalOne(t, `pg.C("tags", pg.Array(pg.Text()))`)
	if c.SQLType != "text[]" {
		t.Errorf("SQLType = %q, want text[]", c.SQLType)
	}
	if c.GoType != pg.GoTypeAny {
		t.Errorf("GoType = %q, want any", c.GoType)
	}
}

func TestEvalTable_PG_ArrayOfUUID(t *testing.T) {
	c := evalOne(t, `pg.C("ids", pg.Array(pg.UUID()))`)
	if c.SQLType != "uuid[]" {
		t.Errorf("SQLType = %q, want uuid[]", c.SQLType)
	}
}

func TestEvalTable_PG_Enum(t *testing.T) {
	c := evalOne(t, `pg.C("status", pg.Enum("status_type", "active", "inactive"))`)
	if c.SQLType != "status_type" {
		t.Errorf("SQLType = %q, want status_type", c.SQLType)
	}
	if c.GoType != pg.GoTypeString {
		t.Errorf("GoType = %q, want string", c.GoType)
	}
}

func TestEvalTable_PGEnum_TooFewArgs_ReturnsError(t *testing.T) {
	pt := &parser.ParsedTable{
		TableName: "t",
		Columns: []parser.ParsedColumn{
			{
				Name: "status",
				Chain: &parser.ChainResult{
					BasePkg:  "pg",
					BaseFn:   "Enum",
					BaseArgs: []any{"only_type_name"},
				},
			},
		},
	}
	_, err := parser.EvalTable(pt)
	if err == nil {
		t.Fatal("expected error for pg.Enum with only type name and no values")
	}
}
