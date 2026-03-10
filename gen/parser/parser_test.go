package parser_test

import (
	"os"
	"path/filepath"
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

	write("schema.go", src)          // should be parsed
	write("schema_test.go", src)     // should be skipped
	write("schema_gen.go", src)      // should be skipped
	write("other_gen.go", src)       // should be skipped

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
	def, err := parser.EvalTable(tbl)
	if err != nil {
		t.Fatalf("EvalTable: %v", err)
	}
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
	def, err := parser.EvalTable(tbl)
	if err != nil {
		t.Fatalf("EvalTable: %v", err)
	}
	if def.Schema != "audit" {
		t.Errorf("Schema: got %q, want audit", def.Schema)
	}
	if def.Name != "logs" {
		t.Errorf("Name: got %q, want logs", def.Name)
	}
}
