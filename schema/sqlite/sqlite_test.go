package sqlite_test

import (
	"testing"

	pg "github.com/sofired/grizzle/schema/pg"
	sqlite "github.com/sofired/grizzle/schema/sqlite"
)

// ---------------------------------------------------------------------------
// Column builder tests
// ---------------------------------------------------------------------------

func TestText_ColumnDef(t *testing.T) {
	col := sqlite.Text().NotNull().Build("title")
	if col.SQLType != "text" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "text")
	}
	if !col.NotNull {
		t.Error("expected NotNull=true")
	}
	if col.GoType != pg.GoTypeString {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeString)
	}
}

func TestInteger_ColumnDef(t *testing.T) {
	col := sqlite.Integer().PrimaryKey().Build("id")
	if col.SQLType != "integer" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "integer")
	}
	if !col.PrimaryKey {
		t.Error("expected PrimaryKey=true")
	}
	if !col.NotNull {
		t.Error("expected NotNull=true (PK is implicitly NOT NULL)")
	}
	if col.GoType != pg.GoTypeInt {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeInt)
	}
}

func TestReal_ColumnDef(t *testing.T) {
	col := sqlite.Real().NotNull().Default(0.0).Build("score")
	if col.SQLType != "real" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "real")
	}
	if !col.NotNull {
		t.Error("expected NotNull=true")
	}
	if col.DefaultExpr != "0" {
		t.Errorf("DefaultExpr: got %q, want %q", col.DefaultExpr, "0")
	}
	if col.GoType != pg.GoTypeFloat64 {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeFloat64)
	}
}

func TestBlob_ColumnDef(t *testing.T) {
	col := sqlite.Blob().NotNull().Build("data")
	if col.SQLType != "blob" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "blob")
	}
	if !col.NotNull {
		t.Error("expected NotNull=true")
	}
	if col.GoType != pg.GoTypeByteSlice {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeByteSlice)
	}
}

func TestBlob_Nullable(t *testing.T) {
	col := sqlite.Blob().Build("attachment")
	if col.NotNull {
		t.Error("expected NotNull=false for nullable Blob column")
	}
	if col.SQLType != "blob" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "blob")
	}
}

func TestUUID_ColumnDef(t *testing.T) {
	col := sqlite.UUID().PrimaryKey().DefaultRandom().Build("id")
	if col.SQLType != "text" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "text")
	}
	if !col.PrimaryKey {
		t.Error("expected PrimaryKey=true")
	}
}

func TestBoolean_ColumnDef(t *testing.T) {
	col := sqlite.Boolean().NotNull().Default(false).Build("active")
	if col.SQLType != "integer" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "integer")
	}
	if col.DefaultExpr != "0" {
		t.Errorf("DefaultExpr: got %q, want %q", col.DefaultExpr, "0")
	}
}

func TestNumeric_ColumnDef(t *testing.T) {
	col := sqlite.Numeric(10, 2).NotNull().Build("price")
	if col.SQLType != "numeric(10,2)" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "numeric(10,2)")
	}
	if col.GoType != pg.GoTypeFloat64 {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeFloat64)
	}
}

func TestVarchar_ColumnDef(t *testing.T) {
	col := sqlite.Varchar(255).NotNull().Build("name")
	if col.SQLType != "varchar(255)" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "varchar(255)")
	}
}

func TestTimestamp_ColumnDef(t *testing.T) {
	col := sqlite.Timestamp().NotNull().DefaultNow().Build("created_at")
	if col.SQLType != "text" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "text")
	}
	if col.DefaultExpr != "CURRENT_TIMESTAMP" {
		t.Errorf("DefaultExpr: got %q, want %q", col.DefaultExpr, "CURRENT_TIMESTAMP")
	}
}

func TestJSON_ColumnDef(t *testing.T) {
	col := sqlite.JSON().Build("meta")
	if col.SQLType != "text" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "text")
	}
}

func TestBigInt_ColumnDef(t *testing.T) {
	col := sqlite.BigInt().NotNull().Build("counter")
	if col.SQLType != "bigint" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "bigint")
	}
	if col.GoType != pg.GoTypeInt64 {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeInt64)
	}
}

func TestReal_PrimaryKey(t *testing.T) {
	col := sqlite.Real().PrimaryKey().Build("weight")
	if !col.PrimaryKey {
		t.Error("expected PrimaryKey=true")
	}
	if !col.NotNull {
		t.Error("expected NotNull=true (PK is implicitly NOT NULL)")
	}
}

func TestBlob_PrimaryKey(t *testing.T) {
	col := sqlite.Blob().PrimaryKey().Build("hash")
	if !col.PrimaryKey {
		t.Error("expected PrimaryKey=true")
	}
	if !col.NotNull {
		t.Error("expected NotNull=true (PK is implicitly NOT NULL)")
	}
}

// ---------------------------------------------------------------------------
// Table construction tests
// ---------------------------------------------------------------------------

func TestTable_Build(t *testing.T) {
	tbl := sqlite.Table("notes",
		sqlite.C("id", sqlite.Integer().PrimaryKey()),
		sqlite.C("title", sqlite.Text().NotNull()),
		sqlite.C("body", sqlite.Text()),
		sqlite.C("score", sqlite.Real()),
		sqlite.C("data", sqlite.Blob()),
	).Build()

	if tbl.Name != "notes" {
		t.Errorf("Name: got %q, want %q", tbl.Name, "notes")
	}
	if len(tbl.Columns) != 5 {
		t.Errorf("len(Columns): got %d, want %d", len(tbl.Columns), 5)
	}
	names := make([]string, len(tbl.Columns))
	for i, c := range tbl.Columns {
		names[i] = c.Name
	}
	want := []string{"id", "title", "body", "score", "data"}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("Column[%d].Name: got %q, want %q", i, names[i], w)
		}
	}
}

func TestTable_WithConstraints(t *testing.T) {
	tbl := sqlite.Table("users",
		sqlite.C("id", sqlite.Integer().PrimaryKey()),
		sqlite.C("email", sqlite.Text().NotNull()),
	).WithConstraints(func(t sqlite.TableRef) []sqlite.Constraint {
		return []sqlite.Constraint{
			sqlite.UniqueIndex("users_email_idx").On(t.Col("email")).Build(),
		}
	})

	if len(tbl.Constraints) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(tbl.Constraints))
	}
	c := tbl.Constraints[0]
	if c.Kind != sqlite.KindUniqueIndex {
		t.Errorf("Kind: got %v, want %v", c.Kind, sqlite.KindUniqueIndex)
	}
	if c.Name != "users_email_idx" {
		t.Errorf("Name: got %q, want %q", c.Name, "users_email_idx")
	}
}

func TestSchemaTable(t *testing.T) {
	tbl := sqlite.SchemaTable("main", "events",
		sqlite.C("id", sqlite.Integer().PrimaryKey()),
	).Build()

	if tbl.Schema != "main" {
		t.Errorf("Schema: got %q, want %q", tbl.Schema, "main")
	}
	if tbl.Name != "events" {
		t.Errorf("Name: got %q, want %q", tbl.Name, "events")
	}
	if tbl.QualifiedName() != "main.events" {
		t.Errorf("QualifiedName: got %q, want %q", tbl.QualifiedName(), "main.events")
	}
}

func TestTable_ForeignKey(t *testing.T) {
	tbl := sqlite.Table("posts",
		sqlite.C("id", sqlite.Integer().PrimaryKey()),
		sqlite.C("user_id", sqlite.Integer().NotNull().References("users", "id",
			sqlite.OnDelete(sqlite.FKActionCascade),
		)),
	).Build()

	if len(tbl.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(tbl.Columns))
	}
	fkCol := tbl.Columns[1]
	if fkCol.References == nil {
		t.Fatal("expected References to be non-nil")
	}
	if fkCol.References.Table != "users" {
		t.Errorf("References.Table: got %q, want %q", fkCol.References.Table, "users")
	}
	if fkCol.References.OnDelete != sqlite.FKActionCascade {
		t.Errorf("References.OnDelete: got %v, want %v", fkCol.References.OnDelete, sqlite.FKActionCascade)
	}
}
