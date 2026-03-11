package mysql_test

import (
	"strings"
	"testing"

	"github.com/sofired/grizzle/kit"
	mysql "github.com/sofired/grizzle/schema/mysql"
	pg "github.com/sofired/grizzle/schema/pg"
)

// ---------------------------------------------------------------------------
// Column builder tests
// ---------------------------------------------------------------------------

func TestUUID_ColumnDef(t *testing.T) {
	col := mysql.UUID().PrimaryKey().DefaultRandom().Build("id")
	if col.SQLType != "uuid" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "uuid")
	}
	if !col.PrimaryKey {
		t.Error("expected PrimaryKey=true")
	}
	if !col.NotNull {
		t.Error("expected NotNull=true (PK is implicitly NOT NULL)")
	}
	if col.DefaultExpr != "gen_random_uuid()" {
		t.Errorf("DefaultExpr: got %q, want %q", col.DefaultExpr, "gen_random_uuid()")
	}
	if col.GoType != pg.GoTypeUUID {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeUUID)
	}
}

func TestVarchar_ColumnDef(t *testing.T) {
	col := mysql.Varchar(255).NotNull().Build("username")
	if col.SQLType != "varchar(255)" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "varchar(255)")
	}
	if !col.NotNull {
		t.Error("expected NotNull=true")
	}
	if col.GoType != pg.GoTypeString {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeString)
	}
}

func TestBoolean_ColumnDef(t *testing.T) {
	col := mysql.Boolean().NotNull().Default(true).Build("enabled")
	if col.SQLType != "boolean" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "boolean")
	}
	if col.DefaultExpr != "true" {
		t.Errorf("DefaultExpr: got %q, want %q", col.DefaultExpr, "true")
	}
}

func TestTimestamp_ColumnDef(t *testing.T) {
	col := mysql.Timestamp().WithTimezone().NotNull().DefaultNow().Build("created_at")
	if col.SQLType != "timestamptz" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "timestamptz")
	}
	if col.DefaultExpr != "now()" {
		t.Errorf("DefaultExpr: got %q, want %q", col.DefaultExpr, "now()")
	}
}

func TestTimestamp_WithoutTimezone(t *testing.T) {
	col := mysql.Timestamp().NotNull().DefaultNow().Build("created_at")
	if col.SQLType != "timestamp" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "timestamp")
	}
}

func TestJSON_ColumnDef(t *testing.T) {
	col := mysql.JSON().Build("meta")
	if col.SQLType != "json" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "json")
	}
}

func TestNumeric_ColumnDef(t *testing.T) {
	col := mysql.Numeric(10, 2).NotNull().Build("score")
	if col.SQLType != "numeric(10,2)" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "numeric(10,2)")
	}
}

func TestTinyInt_ColumnDef(t *testing.T) {
	col := mysql.TinyInt().NotNull().Default(0).Build("flags")
	if col.SQLType != "tinyint" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "tinyint")
	}
	if col.DefaultExpr != "0" {
		t.Errorf("DefaultExpr: got %q, want %q", col.DefaultExpr, "0")
	}
	if !col.NotNull {
		t.Error("expected NotNull=true")
	}
}

func TestSmallInt_ColumnDef(t *testing.T) {
	col := mysql.SmallInt().NotNull().Build("priority")
	if col.SQLType != "smallint" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "smallint")
	}
}

func TestDouble_ColumnDef(t *testing.T) {
	col := mysql.Double().NotNull().Build("latitude")
	if col.SQLType != "double precision" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "double precision")
	}
}

func TestUUID_References(t *testing.T) {
	col := mysql.UUID().NotNull().References("realms", "id", mysql.OnDelete(mysql.FKActionRestrict)).Build("realm_id")
	if col.References == nil {
		t.Fatal("expected References to be non-nil")
	}
	if col.References.Table != "realms" {
		t.Errorf("References.Table: got %q, want %q", col.References.Table, "realms")
	}
	if col.References.OnDelete != mysql.FKActionRestrict {
		t.Errorf("References.OnDelete: got %v, want %v", col.References.OnDelete, mysql.FKActionRestrict)
	}
}

// ---------------------------------------------------------------------------
// Table construction tests
// ---------------------------------------------------------------------------

func TestTable_Build(t *testing.T) {
	tbl := mysql.Table("users",
		mysql.C("id", mysql.UUID().PrimaryKey().DefaultRandom()),
		mysql.C("username", mysql.Varchar(255).NotNull()),
		mysql.C("enabled", mysql.Boolean().NotNull().Default(true)),
		mysql.C("created_at", mysql.Timestamp().WithTimezone().NotNull().DefaultNow()),
	).Build()

	if tbl.Name != "users" {
		t.Errorf("Name: got %q, want %q", tbl.Name, "users")
	}
	if len(tbl.Columns) != 4 {
		t.Errorf("len(Columns): got %d, want %d", len(tbl.Columns), 4)
	}
	// Verify column order is preserved.
	names := make([]string, len(tbl.Columns))
	for i, c := range tbl.Columns {
		names[i] = c.Name
	}
	want := []string{"id", "username", "enabled", "created_at"}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("Column[%d].Name: got %q, want %q", i, names[i], w)
		}
	}
}

func TestTable_WithConstraints(t *testing.T) {
	tbl := mysql.Table("users",
		mysql.C("id", mysql.UUID().PrimaryKey().DefaultRandom()),
		mysql.C("realm_id", mysql.UUID().NotNull()),
		mysql.C("username", mysql.Varchar(255).NotNull()),
	).WithConstraints(func(t mysql.TableRef) []mysql.Constraint {
		return []mysql.Constraint{
			mysql.UniqueIndex("users_realm_username_idx").
				On(t.Col("realm_id"), t.Col("username")).
				Build(),
		}
	})

	if len(tbl.Constraints) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(tbl.Constraints))
	}
	c := tbl.Constraints[0]
	if c.Kind != mysql.KindUniqueIndex {
		t.Errorf("Kind: got %v, want %v", c.Kind, mysql.KindUniqueIndex)
	}
	if c.Name != "users_realm_username_idx" {
		t.Errorf("Name: got %q, want %q", c.Name, "users_realm_username_idx")
	}
}

func TestSchemaTable(t *testing.T) {
	tbl := mysql.SchemaTable("auth", "users",
		mysql.C("id", mysql.UUID().PrimaryKey().DefaultRandom()),
	).Build()

	if tbl.Schema != "auth" {
		t.Errorf("Schema: got %q, want %q", tbl.Schema, "auth")
	}
	if tbl.Name != "users" {
		t.Errorf("Name: got %q, want %q", tbl.Name, "users")
	}
	if tbl.QualifiedName() != "auth.users" {
		t.Errorf("QualifiedName: got %q, want %q", tbl.QualifiedName(), "auth.users")
	}
}

// ---------------------------------------------------------------------------
// DDL generation tests — exercises the full kit.GenerateCreateSQLMySQL path
// ---------------------------------------------------------------------------

func TestDDL_TypeTranslations(t *testing.T) {
	tbl := mysql.Table("things",
		mysql.C("id", mysql.UUID().PrimaryKey().DefaultRandom()),
		mysql.C("name", mysql.Varchar(128).NotNull()),
		mysql.C("bio", mysql.Text()),
		mysql.C("enabled", mysql.Boolean().NotNull().Default(true)),
		mysql.C("score", mysql.Numeric(10, 2)),
		mysql.C("meta", mysql.JSON()),
		mysql.C("created_at", mysql.Timestamp().WithTimezone().NotNull().DefaultNow()),
		mysql.C("updated_at", mysql.Timestamp().NotNull().DefaultNow()),
		mysql.C("flags", mysql.TinyInt().NotNull().Default(0)),
		mysql.C("priority", mysql.SmallInt()),
		mysql.C("lat", mysql.Double()),
	).Build()

	ddl := kit.GenerateCreateSQLMySQL(tbl)

	type check struct {
		desc string
		want string
	}
	checks := []check{
		{"table header", "CREATE TABLE IF NOT EXISTS `things`"},
		{"uuid → CHAR(36)", "CHAR(36)"},
		{"varchar → VARCHAR(128)", "VARCHAR(128)"},
		{"text → LONGTEXT", "LONGTEXT"},
		{"boolean → TINYINT(1)", "TINYINT(1)"},
		{"numeric → NUMERIC(10,2)", "NUMERIC(10,2)"},
		{"json → JSON", "JSON"},
		{"timestamptz → DATETIME(6)", "DATETIME(6)"},
		{"timestamp → DATETIME", "DATETIME"},
		{"tinyint passes through", "TINYINT"},
		{"smallint passes through", "SMALLINT"},
		{"double precision → DOUBLE", "DOUBLE"},
		{"uuid default → (UUID())", "(UUID())"},
		{"now() → CURRENT_TIMESTAMP(6)", "CURRENT_TIMESTAMP(6)"},
		{"boolean default true → 1", "DEFAULT 1"},
		{"engine clause", "ENGINE=InnoDB"},
	}
	for _, c := range checks {
		if !strings.Contains(ddl, c.want) {
			t.Errorf("%s: DDL missing %q\n---\n%s\n---", c.desc, c.want, ddl)
		}
	}
}

func TestDDL_Indexes(t *testing.T) {
	tbl := mysql.Table("users",
		mysql.C("id", mysql.UUID().PrimaryKey().DefaultRandom()),
		mysql.C("realm_id", mysql.UUID().NotNull()),
		mysql.C("email", mysql.Varchar(255)),
	).WithConstraints(func(t mysql.TableRef) []mysql.Constraint {
		return []mysql.Constraint{
			mysql.UniqueIndex("users_realm_email_idx").On(t.Col("realm_id"), t.Col("email")).Build(),
			mysql.Index("users_realm_idx").On(t.Col("realm_id")).Build(),
		}
	})

	ddl := kit.GenerateCreateSQLMySQL(tbl)

	if !strings.Contains(ddl, "CREATE UNIQUE INDEX `users_realm_email_idx`") {
		t.Errorf("missing unique index DDL\n---\n%s\n---", ddl)
	}
	if !strings.Contains(ddl, "CREATE INDEX `users_realm_idx`") {
		t.Errorf("missing index DDL\n---\n%s\n---", ddl)
	}
}

func TestDDL_ForeignKey(t *testing.T) {
	tbl := mysql.Table("users",
		mysql.C("id", mysql.UUID().PrimaryKey().DefaultRandom()),
		mysql.C("realm_id", mysql.UUID().NotNull().References("realms", "id",
			mysql.OnDelete(mysql.FKActionCascade),
		)),
	).Build()

	ddl := kit.GenerateCreateSQLMySQL(tbl)

	if !strings.Contains(ddl, "REFERENCES `realms` (`id`)") {
		t.Errorf("missing FK reference\n---\n%s\n---", ddl)
	}
	if !strings.Contains(ddl, "ON DELETE CASCADE") {
		t.Errorf("missing ON DELETE CASCADE\n---\n%s\n---", ddl)
	}
}

func TestDDL_CheckConstraint(t *testing.T) {
	tbl := mysql.Table("products",
		mysql.C("id", mysql.UUID().PrimaryKey().DefaultRandom()),
		mysql.C("price", mysql.Numeric(10, 2).NotNull()),
	).WithConstraints(func(t mysql.TableRef) []mysql.Constraint {
		return []mysql.Constraint{
			mysql.Check("price_positive", "price > 0"),
		}
	})

	ddl := kit.GenerateCreateSQLMySQL(tbl)

	if !strings.Contains(ddl, "CONSTRAINT `price_positive` CHECK (price > 0)") {
		t.Errorf("missing CHECK constraint\n---\n%s\n---", ddl)
	}
}
