package pg_test

import (
	"math"
	"testing"

	pg "github.com/sofired/grizzle/schema/pg"
)

func TestDate(t *testing.T) {
	def := pg.Date().NotNull().Build("birth_date")
	if def.SQLType != "date" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "date")
	}
	if def.GoType != pg.GoTypeTime {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeTime)
	}
	if !def.NotNull {
		t.Error("NotNull should be true")
	}
}

func TestDateDefault(t *testing.T) {
	def := pg.Date().Default("2024-01-01").Build("col")
	if !def.HasDefault {
		t.Error("HasDefault should be true")
	}
	if def.DefaultExpr != "'2024-01-01'" {
		t.Errorf("DefaultExpr = %q, want %q", def.DefaultExpr, "'2024-01-01'")
	}
}

func TestTime(t *testing.T) {
	def := pg.Time().NotNull().Build("start_time")
	if def.SQLType != "time" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "time")
	}
	if def.GoType != pg.GoTypeTime {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeTime)
	}
}

func TestTimeWithTimezone(t *testing.T) {
	def := pg.Time().WithTimezone().NotNull().Build("col")
	if def.SQLType != "timetz" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "timetz")
	}
}

func TestInterval(t *testing.T) {
	def := pg.Interval().NotNull().Build("duration")
	if def.SQLType != "interval" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "interval")
	}
	if def.GoType != pg.GoTypeString {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeString)
	}
}

func TestIntervalDefault(t *testing.T) {
	def := pg.Interval().Default("1 hour").Build("col")
	if def.DefaultExpr != "'1 hour'::interval" {
		t.Errorf("DefaultExpr = %q, want %q", def.DefaultExpr, "'1 hour'::interval")
	}
}

func TestReal(t *testing.T) {
	def := pg.Real().NotNull().Build("score")
	if def.SQLType != "real" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "real")
	}
	if def.GoType != pg.GoTypeFloat64 {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeFloat64)
	}
}

func TestDoublePrecision(t *testing.T) {
	def := pg.DoublePrecision().NotNull().Build("ratio")
	if def.SQLType != "double precision" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "double precision")
	}
	if def.GoType != pg.GoTypeFloat64 {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeFloat64)
	}
}

func TestFloatDefault(t *testing.T) {
	def := pg.DoublePrecision().Default(3.14).Build("col")
	if !def.HasDefault {
		t.Error("HasDefault should be true")
	}
}

func TestFloatDefaultSpecialValues(t *testing.T) {
	tests := []struct {
		name    string
		builder func() *pg.FloatBuilder
		val     float64
		want    string
	}{
		{"real NaN", pg.Real, math.NaN(), "'NaN'::real"},
		{"real +Inf", pg.Real, math.Inf(1), "'Infinity'::real"},
		{"real -Inf", pg.Real, math.Inf(-1), "'-Infinity'::real"},
		{"double NaN", pg.DoublePrecision, math.NaN(), "'NaN'::double precision"},
		{"double +Inf", pg.DoublePrecision, math.Inf(1), "'Infinity'::double precision"},
		{"double -Inf", pg.DoublePrecision, math.Inf(-1), "'-Infinity'::double precision"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := tt.builder().Default(tt.val).Build("col")
			if !def.HasDefault {
				t.Error("HasDefault should be true")
			}
			if def.DefaultExpr != tt.want {
				t.Errorf("DefaultExpr = %q, want %q", def.DefaultExpr, tt.want)
			}
		})
	}
}

func TestChar(t *testing.T) {
	def := pg.Char(10).NotNull().Build("code")
	if def.SQLType != "char(10)" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "char(10)")
	}
	if def.GoType != pg.GoTypeString {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeString)
	}
}

func TestBytea(t *testing.T) {
	def := pg.Bytea().NotNull().Build("data")
	if def.SQLType != "bytea" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "bytea")
	}
	if def.GoType != pg.GoTypeByteSlice {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeByteSlice)
	}
}

func TestInet(t *testing.T) {
	def := pg.Inet().NotNull().Build("ip_addr")
	if def.SQLType != "inet" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "inet")
	}
	if def.GoType != pg.GoTypeString {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeString)
	}
}

func TestCidr(t *testing.T) {
	def := pg.Cidr().Build("network")
	if def.SQLType != "cidr" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "cidr")
	}
}

func TestMacaddr(t *testing.T) {
	def := pg.Macaddr().Build("mac")
	if def.SQLType != "macaddr" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "macaddr")
	}
}

func TestNetworkDefault(t *testing.T) {
	def := pg.Inet().Default("127.0.0.1").Build("col")
	if def.DefaultExpr != "'127.0.0.1'" {
		t.Errorf("DefaultExpr = %q, want %q", def.DefaultExpr, "'127.0.0.1'")
	}
}

func TestTsvector(t *testing.T) {
	def := pg.Tsvector().NotNull().Build("doc")
	if def.SQLType != "tsvector" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "tsvector")
	}
	if def.GoType != pg.GoTypeString {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeString)
	}
}

func TestTsquery(t *testing.T) {
	def := pg.Tsquery().Build("q")
	if def.SQLType != "tsquery" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "tsquery")
	}
}

func TestRangeTypes(t *testing.T) {
	cases := []struct {
		fn      func() *pg.RangeBuilder
		sqlType string
	}{
		{pg.Int4Range, "int4range"},
		{pg.Int8Range, "int8range"},
		{pg.NumRange, "numrange"},
		{pg.TsRange, "tsrange"},
		{pg.TstzRange, "tstzrange"},
		{pg.DateRange, "daterange"},
	}
	for _, tc := range cases {
		def := tc.fn().Build("col")
		if def.SQLType != tc.sqlType {
			t.Errorf("SQLType = %q, want %q", def.SQLType, tc.sqlType)
		}
		if def.GoType != pg.GoTypeString {
			t.Errorf("%s GoType = %q, want string", tc.sqlType, def.GoType)
		}
	}
}

func TestEnum(t *testing.T) {
	def := pg.Enum("mood", "happy", "sad", "neutral").NotNull().Build("status")
	if def.SQLType != "mood" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "mood")
	}
	if def.GoType != pg.GoTypeString {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeString)
	}
}

func TestEnumDefault(t *testing.T) {
	def := pg.Enum("mood", "happy", "sad").Default("happy").Build("col")
	if def.DefaultExpr != "'happy'::mood" {
		t.Errorf("DefaultExpr = %q, want %q", def.DefaultExpr, "'happy'::mood")
	}
}

func TestEnumDefaultEscapesSingleQuote(t *testing.T) {
	def := pg.Enum("mood", "it's good", "bad").Default("it's good").Build("col")
	if def.DefaultExpr != "'it''s good'::mood" {
		t.Errorf("DefaultExpr = %q, want %q", def.DefaultExpr, "'it''s good'::mood")
	}
}

func TestEnumPanicsOnEmpty(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty values")
		}
	}()
	pg.Enum("mood")
}

func TestEnumPanicsOnEmptyValue(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty string value")
		}
	}()
	pg.Enum("mood", "happy", "")
}

func TestArray(t *testing.T) {
	def := pg.Array(pg.Text()).NotNull().Build("tags")
	if def.SQLType != "text[]" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "text[]")
	}
	if def.GoType != pg.GoTypeAny {
		t.Errorf("GoType = %q, want %q", def.GoType, pg.GoTypeAny)
	}
}

func TestArrayNested(t *testing.T) {
	def := pg.Array(pg.Integer()).Build("scores")
	if def.SQLType != "integer[]" {
		t.Errorf("SQLType = %q, want %q", def.SQLType, "integer[]")
	}
}

func TestArrayDefaultEmpty(t *testing.T) {
	def := pg.Array(pg.Text()).DefaultEmpty().Build("tags")
	if def.DefaultExpr != "ARRAY[]::text[]" {
		t.Errorf("DefaultExpr = %q, want %q", def.DefaultExpr, "ARRAY[]::text[]")
	}
}

func TestArrayDefault(t *testing.T) {
	def := pg.Array(pg.Integer()).Default("{1,2,3}").Build("ids")
	if def.DefaultExpr != "'{1,2,3}'::integer[]" {
		t.Errorf("DefaultExpr = %q, want %q", def.DefaultExpr, "'{1,2,3}'::integer[]")
	}
}

func TestCheck(t *testing.T) {
	c := pg.Check("age_check", "age >= 0")
	if c.Kind != pg.KindCheck {
		t.Errorf("Kind = %q, want %q", c.Kind, pg.KindCheck)
	}
	if c.Name != "age_check" {
		t.Errorf("Name = %q, want %q", c.Name, "age_check")
	}
	if c.CheckExpr != "age >= 0" {
		t.Errorf("CheckExpr = %q, want %q", c.CheckExpr, "age >= 0")
	}
}

func TestDefaultEscapesSingleQuote(t *testing.T) {
	tests := []struct {
		name string
		def  pg.ColumnDef
		want string
	}{
		{"Date", pg.Date().Default("2024-01-'01").Build("col"), "'2024-01-''01'"},
		{"Time", pg.Time().Default("12:34'Z").Build("col"), "'12:34''Z'"},
		{"Char", pg.Char(10).Default("it's").Build("col"), "'it''s'"},
		{"Network", pg.Inet().Default("192.0.2.'1").Build("col"), "'192.0.2.''1'"},
		{"Interval", pg.Interval().Default("1 hour'").Build("col"), "'1 hour'''::interval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.def.HasDefault {
				t.Error("HasDefault should be true")
			}
			if tt.def.DefaultExpr != tt.want {
				t.Errorf("DefaultExpr = %q, want %q", tt.def.DefaultExpr, tt.want)
			}
		})
	}
}

func TestColumnNameInjected(t *testing.T) {
	for _, tc := range []struct {
		name string
		def  pg.ColumnDef
	}{
		{"birth_date", pg.Date().Build("birth_date")},
		{"start_time", pg.Time().Build("start_time")},
		{"dur", pg.Interval().Build("dur")},
		{"score", pg.Real().Build("score")},
		{"ratio", pg.DoublePrecision().Build("ratio")},
		{"code", pg.Char(3).Build("code")},
		{"data", pg.Bytea().Build("data")},
		{"ip", pg.Inet().Build("ip")},
		{"net", pg.Cidr().Build("net")},
		{"mac", pg.Macaddr().Build("mac")},
		{"doc", pg.Tsvector().Build("doc")},
		{"q", pg.Tsquery().Build("q")},
		{"rng", pg.Int4Range().Build("rng")},
		{"tags", pg.Array(pg.Text()).Build("tags")},
		{"status", pg.Enum("mood", "ok").Build("status")},
	} {
		if tc.def.Name != tc.name {
			t.Errorf("column %q: Name = %q", tc.name, tc.def.Name)
		}
	}
}
