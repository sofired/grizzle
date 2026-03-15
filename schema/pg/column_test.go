package pg_test

import (
	"strings"
	"testing"

	pg "github.com/sofired/grizzle/schema/pg"
)

// ---------------------------------------------------------------------------
// Date
// ---------------------------------------------------------------------------

func TestDate_ColumnDef(t *testing.T) {
	col := pg.Date().NotNull().Build("published_on")
	if col.SQLType != "date" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "date")
	}
	if !col.NotNull {
		t.Error("expected NotNull=true")
	}
	if col.GoType != pg.GoTypeTime {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeTime)
	}
}

func TestDate_Default(t *testing.T) {
	col := pg.Date().Default("2025-01-01").Build("effective_date")
	if col.DefaultExpr != "'2025-01-01'" {
		t.Errorf("DefaultExpr: got %q, want %q", col.DefaultExpr, "'2025-01-01'")
	}
	if !col.HasDefault {
		t.Error("expected HasDefault=true")
	}
}

// ---------------------------------------------------------------------------
// Time / TimeTZ
// ---------------------------------------------------------------------------

func TestTime_ColumnDef(t *testing.T) {
	col := pg.Time().NotNull().Build("start_time")
	if col.SQLType != "time" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "time")
	}
	if col.GoType != pg.GoTypeTime {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeTime)
	}
}

func TestTimeTZ_WithTimezone_ColumnDef(t *testing.T) {
	col := pg.Time().WithTimezone().NotNull().Build("start_time_tz")
	if col.SQLType != "timetz" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "timetz")
	}
}

func TestTimeTZ_Constructor(t *testing.T) {
	col := pg.TimeTZ().NotNull().Build("start_time_tz")
	if col.SQLType != "timetz" {
		t.Errorf("TimeTZ() SQLType: got %q, want %q", col.SQLType, "timetz")
	}
	if col.GoType != pg.GoTypeTime {
		t.Errorf("TimeTZ() GoType: got %v, want %v", col.GoType, pg.GoTypeTime)
	}
	if !col.NotNull {
		t.Error("expected NotNull=true")
	}
	if col.Name != "start_time_tz" {
		t.Errorf("TimeTZ() Name: got %q, want %q", col.Name, "start_time_tz")
	}
}

// ---------------------------------------------------------------------------
// Bytea
// ---------------------------------------------------------------------------

func TestBytea_ColumnDef(t *testing.T) {
	col := pg.Bytea().NotNull().Build("data")
	if col.SQLType != "bytea" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "bytea")
	}
	if col.GoType != pg.GoTypeByteSlice {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeByteSlice)
	}
	if !col.NotNull {
		t.Error("expected NotNull=true")
	}
}

// ---------------------------------------------------------------------------
// Interval
// ---------------------------------------------------------------------------

func TestInterval_ColumnDef(t *testing.T) {
	col := pg.Interval().NotNull().Build("duration")
	if col.SQLType != "interval" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "interval")
	}
	if col.GoType != pg.GoTypeString {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeString)
	}
}

func TestInterval_Default(t *testing.T) {
	col := pg.Interval().Default("1 day").Build("ttl")
	if col.DefaultExpr != "'1 day'::interval" {
		t.Errorf("DefaultExpr: got %q, want %q", col.DefaultExpr, "'1 day'::interval")
	}
}

// ---------------------------------------------------------------------------
// Enum
// ---------------------------------------------------------------------------

func TestEnum_ColumnDef(t *testing.T) {
	col := pg.Enum("order_status").NotNull().Build("status")
	if col.SQLType != "order_status" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "order_status")
	}
	if col.GoType != pg.GoTypeString {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeString)
	}
}

func TestEnum_Default(t *testing.T) {
	col := pg.Enum("mood").Default("happy").Build("current_mood")
	if col.DefaultExpr != "'happy'::mood" {
		t.Errorf("DefaultExpr: got %q, want %q", col.DefaultExpr, "'happy'::mood")
	}
}

func TestEnum_Unique(t *testing.T) {
	col := pg.Enum("priority").Unique().Build("level")
	if !col.Unique {
		t.Error("expected Unique=true")
	}
}

// ---------------------------------------------------------------------------
// Array
// ---------------------------------------------------------------------------

func TestArray_TextElement(t *testing.T) {
	col := pg.Array(pg.Text()).NotNull().Build("tags")
	if col.SQLType != "text[]" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "text[]")
	}
	if col.GoType != pg.GoTypeAny {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeAny)
	}
}

func TestArray_IntegerElement(t *testing.T) {
	col := pg.Array(pg.Integer()).Build("scores")
	if col.SQLType != "integer[]" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "integer[]")
	}
}

func TestArray_UUIDElement(t *testing.T) {
	col := pg.Array(pg.UUID()).Build("related_ids")
	if col.SQLType != "uuid[]" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "uuid[]")
	}
}

// ---------------------------------------------------------------------------
// Inet / Cidr / Macaddr
// ---------------------------------------------------------------------------

func TestInet_ColumnDef(t *testing.T) {
	col := pg.Inet().NotNull().Build("ip_address")
	if col.SQLType != "inet" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "inet")
	}
	if col.GoType != pg.GoTypeString {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeString)
	}
}

func TestCidr_ColumnDef(t *testing.T) {
	col := pg.Cidr().Build("network")
	if col.SQLType != "cidr" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "cidr")
	}
}

func TestMacaddr_ColumnDef(t *testing.T) {
	col := pg.Macaddr().Build("mac")
	if col.SQLType != "macaddr" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "macaddr")
	}
}

// ---------------------------------------------------------------------------
// Tsvector / Tsquery
// ---------------------------------------------------------------------------

func TestTsvector_ColumnDef(t *testing.T) {
	col := pg.Tsvector().NotNull().Build("search_vector")
	if col.SQLType != "tsvector" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "tsvector")
	}
	if col.GoType != pg.GoTypeString {
		t.Errorf("GoType: got %v, want %v", col.GoType, pg.GoTypeString)
	}
}

func TestTsquery_ColumnDef(t *testing.T) {
	col := pg.Tsquery().NotNull().Build("search_query")
	if col.SQLType != "tsquery" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "tsquery")
	}
}

// ---------------------------------------------------------------------------
// Table integration: new types used in a full table declaration
// ---------------------------------------------------------------------------

func TestNewTypes_InTable(t *testing.T) {
	articles := pg.Table("articles",
		pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
		pg.C("published_on", pg.Date().NotNull()),
		pg.C("content_hash", pg.Bytea().NotNull()),
		pg.C("status", pg.Enum("article_status").NotNull()),
		pg.C("tags", pg.Array(pg.Text())),
		pg.C("ip_origin", pg.Inet()),
		pg.C("search_vec", pg.Tsvector()),
		pg.C("read_time", pg.Interval()),
	).Build()

	if articles.Name != "articles" {
		t.Errorf("Name: got %q, want %q", articles.Name, "articles")
	}
	if len(articles.Columns) != 8 {
		t.Errorf("expected 8 columns, got %d", len(articles.Columns))
	}

	colTypes := make(map[string]string, len(articles.Columns))
	for _, c := range articles.Columns {
		colTypes[c.Name] = c.SQLType
	}

	checks := map[string]string{
		"published_on": "date",
		"content_hash": "bytea",
		"status":       "article_status",
		"tags":         "text[]",
		"ip_origin":    "inet",
		"search_vec":   "tsvector",
		"read_time":    "interval",
	}
	for col, wantType := range checks {
		if got := colTypes[col]; got != wantType {
			t.Errorf("column %q: SQLType got %q, want %q", col, got, wantType)
		}
	}
}

// ---------------------------------------------------------------------------
// SQL type string format checks
// ---------------------------------------------------------------------------

func TestArray_NestedVarchar(t *testing.T) {
	col := pg.Array(pg.Varchar(50)).Build("names")
	if !strings.HasSuffix(col.SQLType, "[]") {
		t.Errorf("Array SQLType should end with []: got %q", col.SQLType)
	}
	if col.SQLType != "varchar(50)[]" {
		t.Errorf("SQLType: got %q, want %q", col.SQLType, "varchar(50)[]")
	}
}

// TestArray_InnerBuilderReuse verifies that passing an inner builder to Array()
// does not mutate the inner builder's Name, so the same builder instance can be
// used again afterwards without producing a stale "_elem" Name.
func TestArray_InnerBuilderReuse(t *testing.T) {
	inner := pg.Text()

	// First use: as the element type of an array column.
	arrCol := pg.Array(inner).Build("tags")
	if arrCol.SQLType != "text[]" {
		t.Fatalf("array SQLType: got %q, want %q", arrCol.SQLType, "text[]")
	}

	// Second use: the same builder used directly for a plain text column.
	// Before the fix, inner.def.Name would have been permanently set to "_elem",
	// causing the resulting ColumnDef.Name to be "_elem" instead of "bio".
	plainCol := inner.Build("bio")
	if plainCol.Name != "bio" {
		t.Errorf("inner builder Name after reuse: got %q, want %q (builder was mutated by Array())", plainCol.Name, "bio")
	}
	if plainCol.SQLType != "text" {
		t.Errorf("inner builder SQLType after reuse: got %q, want %q", plainCol.SQLType, "text")
	}
}
