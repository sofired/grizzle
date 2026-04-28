// Package pg provides the PostgreSQL schema definition DSL for Grizzle.
// Use it to declare your database schema in Go; the grizzle code generator
// reads these declarations to produce typed query helpers and migration snapshots.
//
// Example:
//
//	var Users = pg.Table("users",
//	    pg.C("id",         pg.UUID().PrimaryKey().DefaultRandom()),
//	    pg.C("realm_id",   pg.UUID().NotNull().References("realms", "id", pg.OnDelete("restrict"))),
//	    pg.C("username",   pg.Varchar(255).NotNull()),
//	    pg.C("email",      pg.Varchar(255)),
//	    pg.C("enabled",    pg.Boolean().NotNull().Default("true")),
//	    pg.C("created_at", pg.Timestamp().WithTimezone().NotNull().DefaultNow()),
//	    pg.C("deleted_at", pg.Timestamp().WithTimezone()),
//	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
//	    return []pg.Constraint{
//	        pg.UniqueIndex("users_realm_username_idx").
//	            On(t.Col("realm_id"), t.Col("username")).
//	            Where(pg.IsNull(t.Col("deleted_at"))),
//	    }
//	})
package pg

import (
	"fmt"
	"math"
	"strings"
)

// -------------------------------------------------------------------
// ColumnDef — the result of building a column
// -------------------------------------------------------------------

// GoTypeHint describes the Go type to use in generated code for a column.
type GoTypeHint string

const (
	GoTypeString    GoTypeHint = "string"
	GoTypeInt       GoTypeHint = "int"
	GoTypeInt64     GoTypeHint = "int64"
	GoTypeBool      GoTypeHint = "bool"
	GoTypeTime      GoTypeHint = "time.Time"
	GoTypeUUID      GoTypeHint = "uuid.UUID"
	GoTypeByteSlice GoTypeHint = "[]byte"
	GoTypeFloat64   GoTypeHint = "float64"
	GoTypeAny       GoTypeHint = "any"
)

// FKAction describes what happens on parent row delete/update.
type FKAction string

const (
	FKActionNoAction   FKAction = "NO ACTION"
	FKActionRestrict   FKAction = "RESTRICT"
	FKActionCascade    FKAction = "CASCADE"
	FKActionSetNull    FKAction = "SET NULL"
	FKActionSetDefault FKAction = "SET DEFAULT"
)

// FKRef holds a single inline foreign key reference.
type FKRef struct {
	Table    string
	Column   string
	OnDelete FKAction
	OnUpdate FKAction
}

// ColumnDef is the fully-resolved definition of a single column.
// It contains everything needed for code generation and migration snapshots.
type ColumnDef struct {
	Name         string
	SQLType      string     // e.g. "uuid", "varchar(255)", "timestamptz"
	GoType       GoTypeHint // Go type for generated select model
	NotNull      bool
	HasDefault   bool
	DefaultExpr  string // SQL default expression, e.g. "gen_random_uuid()", "now()", "'true'"
	PrimaryKey   bool
	Unique       bool
	References   *FKRef
	JsonbGoType  string // For JSONB columns: the Go type hint for $type<T> equivalent
	GeneratedAs  string // For generated columns: the SQL expression
	OnUpdateExpr string // Hint for app-layer: set this expression on every UPDATE
	// PreviousName is intentionally excluded from JSON snapshots — it is only
	// meaningful as a schema definition annotation for the current migration step
	// and must not persist across snapshot saves. If it were persisted, a future
	// column that happens to share the old name would trigger a spurious RENAME
	// instead of an ADD.
	PreviousName string `json:"-"`
}

// -------------------------------------------------------------------
// Builder base — all typed builders embed this
// -------------------------------------------------------------------

type colBuilder struct {
	def ColumnDef
}

func (b *colBuilder) setNotNull() { b.def.NotNull = true }
func (b *colBuilder) setDefault(expr string) {
	b.def.HasDefault = true
	b.def.DefaultExpr = expr
}

// quoteSQLLiteral wraps val in single quotes, doubling any embedded single quotes.
func quoteSQLLiteral(val string) string {
	return "'" + strings.ReplaceAll(val, "'", "''") + "'"
}

func (b *colBuilder) setPrimaryKey() {
	b.def.PrimaryKey = true
	b.def.NotNull = true    // PK is implicitly NOT NULL
	b.def.HasDefault = true // PK usually has a default (serial/uuid)
}
func (b *colBuilder) setRenamedFrom(oldName string) { b.def.PreviousName = oldName }

// build finalises the column definition, injecting the column name from the map key.
func (b *colBuilder) build(name string) ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// -------------------------------------------------------------------
// FKOption configures inline foreign key behaviour
// -------------------------------------------------------------------

// FKOption is a functional option for inline FK references.
type FKOption func(*FKRef)

// OnDelete sets the ON DELETE action for an inline FK.
func OnDelete(action FKAction) FKOption {
	return func(r *FKRef) { r.OnDelete = action }
}

// OnUpdate sets the ON UPDATE action for an inline FK.
func OnUpdate(action FKAction) FKOption {
	return func(r *FKRef) { r.OnUpdate = action }
}

// -------------------------------------------------------------------
// UUID
// -------------------------------------------------------------------

// UUIDBuilder builds a uuid column definition.
type UUIDBuilder struct{ colBuilder }

// UUID starts a UUID column.
func UUID() *UUIDBuilder {
	b := &UUIDBuilder{}
	b.def.SQLType = "uuid"
	b.def.GoType = GoTypeUUID
	return b
}

func (b *UUIDBuilder) NotNull() *UUIDBuilder    { b.setNotNull(); return b }
func (b *UUIDBuilder) PrimaryKey() *UUIDBuilder { b.setPrimaryKey(); return b }
func (b *UUIDBuilder) Unique() *UUIDBuilder     { b.def.Unique = true; return b }

// DefaultRandom sets the column default to gen_random_uuid() (PostgreSQL 13+).
func (b *UUIDBuilder) DefaultRandom() *UUIDBuilder {
	b.setDefault("gen_random_uuid()")
	return b
}

// References adds an inline FK constraint.
func (b *UUIDBuilder) References(table, col string, opts ...FKOption) *UUIDBuilder {
	ref := &FKRef{Table: table, Column: col, OnDelete: FKActionNoAction, OnUpdate: FKActionNoAction}
	for _, o := range opts {
		o(ref)
	}
	b.def.References = ref
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot. Leave empty for new columns
// or columns whose name has not changed.
// Remove this call from your schema definition once the migration has been applied.
func (b *UUIDBuilder) RenamedFrom(oldName string) *UUIDBuilder {
	b.setRenamedFrom(oldName)
	return b
}
func (b *UUIDBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Varchar / Text / Char
// -------------------------------------------------------------------

// VarcharBuilder builds a varchar(n) column definition.
type VarcharBuilder struct{ colBuilder }

// Varchar starts a varchar(length) column.
func Varchar(length int) *VarcharBuilder {
	b := &VarcharBuilder{}
	b.def.SQLType = fmt.Sprintf("varchar(%d)", length)
	b.def.GoType = GoTypeString
	return b
}

// Text starts an unbounded text column.
func Text() *VarcharBuilder {
	b := &VarcharBuilder{}
	b.def.SQLType = "text"
	b.def.GoType = GoTypeString
	return b
}

func (b *VarcharBuilder) NotNull() *VarcharBuilder { b.setNotNull(); return b }
func (b *VarcharBuilder) Unique() *VarcharBuilder  { b.def.Unique = true; return b }
func (b *VarcharBuilder) Default(val string) *VarcharBuilder {
	b.setDefault(quoteSQLLiteral(val))
	return b
}
func (b *VarcharBuilder) References(table, col string, opts ...FKOption) *VarcharBuilder {
	ref := &FKRef{Table: table, Column: col}
	for _, o := range opts {
		o(ref)
	}
	b.def.References = ref
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot. Leave empty for new columns
// or columns whose name has not changed.
// Remove this call from your schema definition once the migration has been applied.
func (b *VarcharBuilder) RenamedFrom(oldName string) *VarcharBuilder {
	b.setRenamedFrom(oldName)
	return b
}
func (b *VarcharBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Boolean
// -------------------------------------------------------------------

// BooleanBuilder builds a boolean column definition.
type BooleanBuilder struct{ colBuilder }

// Boolean starts a boolean column.
func Boolean() *BooleanBuilder {
	b := &BooleanBuilder{}
	b.def.SQLType = "boolean"
	b.def.GoType = GoTypeBool
	return b
}

func (b *BooleanBuilder) NotNull() *BooleanBuilder { b.setNotNull(); return b }
func (b *BooleanBuilder) Default(val bool) *BooleanBuilder {
	if val {
		b.setDefault("true")
	} else {
		b.setDefault("false")
	}
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot. Leave empty for new columns
// or columns whose name has not changed.
// Remove this call from your schema definition once the migration has been applied.
func (b *BooleanBuilder) RenamedFrom(oldName string) *BooleanBuilder {
	b.setRenamedFrom(oldName)
	return b
}
func (b *BooleanBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Integer / BigInt / Serial
// -------------------------------------------------------------------

// IntegerBuilder builds an integer column definition.
type IntegerBuilder struct{ colBuilder }

// Integer starts a 4-byte integer column.
func Integer() *IntegerBuilder {
	b := &IntegerBuilder{}
	b.def.SQLType = "integer"
	b.def.GoType = GoTypeInt
	return b
}

// BigInt starts an 8-byte integer column.
func BigInt() *IntegerBuilder {
	b := &IntegerBuilder{}
	b.def.SQLType = "bigint"
	b.def.GoType = GoTypeInt64
	return b
}

// Serial starts an auto-incrementing 4-byte integer column (implicit sequence).
func Serial() *IntegerBuilder {
	b := &IntegerBuilder{}
	b.def.SQLType = "serial"
	b.def.GoType = GoTypeInt
	b.def.HasDefault = true // serial always has a default
	return b
}

// BigSerial starts an auto-incrementing 8-byte integer column.
func BigSerial() *IntegerBuilder {
	b := &IntegerBuilder{}
	b.def.SQLType = "bigserial"
	b.def.GoType = GoTypeInt64
	b.def.HasDefault = true
	return b
}

func (b *IntegerBuilder) NotNull() *IntegerBuilder    { b.setNotNull(); return b }
func (b *IntegerBuilder) PrimaryKey() *IntegerBuilder { b.setPrimaryKey(); return b }
func (b *IntegerBuilder) Default(val int) *IntegerBuilder {
	b.setDefault(fmt.Sprintf("%d", val))
	return b
}
func (b *IntegerBuilder) References(table, col string, opts ...FKOption) *IntegerBuilder {
	ref := &FKRef{Table: table, Column: col}
	for _, o := range opts {
		o(ref)
	}
	b.def.References = ref
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot. Leave empty for new columns
// or columns whose name has not changed.
// Remove this call from your schema definition once the migration has been applied.
func (b *IntegerBuilder) RenamedFrom(oldName string) *IntegerBuilder {
	b.setRenamedFrom(oldName)
	return b
}
func (b *IntegerBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Timestamp
// -------------------------------------------------------------------

// TimestampBuilder builds a timestamp / timestamptz column definition.
type TimestampBuilder struct{ colBuilder }

// Timestamp starts a timestamp (without timezone) column.
func Timestamp() *TimestampBuilder {
	b := &TimestampBuilder{}
	b.def.SQLType = "timestamp"
	b.def.GoType = GoTypeTime
	return b
}

// WithTimezone switches the column to TIMESTAMPTZ.
func (b *TimestampBuilder) WithTimezone() *TimestampBuilder {
	b.def.SQLType = "timestamptz"
	return b
}

func (b *TimestampBuilder) NotNull() *TimestampBuilder { b.setNotNull(); return b }

// DefaultNow sets DEFAULT now().
func (b *TimestampBuilder) DefaultNow() *TimestampBuilder {
	b.setDefault("now()")
	return b
}

// OnUpdate marks this column as an application-managed updated_at field.
// The code generator emits a comment reminding the developer to set this
// field on every UPDATE. (Go has no runtime hook equivalent to Drizzle's $onUpdate.)
func (b *TimestampBuilder) OnUpdate() *TimestampBuilder {
	b.def.OnUpdateExpr = "now()"
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot. Leave empty for new columns
// or columns whose name has not changed.
// Remove this call from your schema definition once the migration has been applied.
func (b *TimestampBuilder) RenamedFrom(oldName string) *TimestampBuilder {
	b.setRenamedFrom(oldName)
	return b
}
func (b *TimestampBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// JSONB
// -------------------------------------------------------------------

// JSONBBuilder builds a jsonb column definition.
type JSONBBuilder struct {
	colBuilder
}

// JSONB starts a jsonb column.
func JSONB() *JSONBBuilder {
	b := &JSONBBuilder{}
	b.def.SQLType = "jsonb"
	b.def.GoType = GoTypeAny
	return b
}

// Type overrides the Go type hint for generated code, equivalent to
// Drizzle's .$type<T>(). The typeExpr string should be a valid Go type
// expression, e.g. "map[string]any", "[]string", "*MyStruct".
func (b *JSONBBuilder) Type(typeExpr string) *JSONBBuilder {
	b.def.JsonbGoType = typeExpr
	return b
}

func (b *JSONBBuilder) NotNull() *JSONBBuilder { b.setNotNull(); return b }
func (b *JSONBBuilder) Default(jsonExpr string) *JSONBBuilder {
	b.setDefault(quoteSQLLiteral(jsonExpr) + "::jsonb")
	return b
}
func (b *JSONBBuilder) DefaultEmpty() *JSONBBuilder {
	b.setDefault("'{}' ::jsonb")
	return b
}
func (b *JSONBBuilder) DefaultEmptyArray() *JSONBBuilder {
	b.setDefault("'[]'::jsonb")
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot. Leave empty for new columns
// or columns whose name has not changed.
// Remove this call from your schema definition once the migration has been applied.
func (b *JSONBBuilder) RenamedFrom(oldName string) *JSONBBuilder {
	b.setRenamedFrom(oldName)
	return b
}
func (b *JSONBBuilder) Build(name string) ColumnDef { return b.build(name) }

// JSON starts a json column (plain JSON, not the binary JSONB format).
// In PostgreSQL schemas prefer JSONB() for indexing support.
// In MySQL-targeted schemas use schema/mysql.JSON() which resolves to this.
func JSON() *JSONBBuilder {
	b := &JSONBBuilder{}
	b.def.SQLType = "json"
	b.def.GoType = GoTypeAny
	return b
}

// -------------------------------------------------------------------
// Numeric / Decimal
// -------------------------------------------------------------------

// NumericBuilder builds a numeric(precision, scale) column.
type NumericBuilder struct{ colBuilder }

// Numeric starts a numeric(precision, scale) column.
func Numeric(precision, scale int) *NumericBuilder {
	b := &NumericBuilder{}
	b.def.SQLType = fmt.Sprintf("numeric(%d,%d)", precision, scale)
	b.def.GoType = GoTypeFloat64
	return b
}

func (b *NumericBuilder) NotNull() *NumericBuilder { b.setNotNull(); return b }
func (b *NumericBuilder) Default(val string) *NumericBuilder {
	b.setDefault(val)
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot. Leave empty for new columns
// or columns whose name has not changed.
// Remove this call from your schema definition once the migration has been applied.
func (b *NumericBuilder) RenamedFrom(oldName string) *NumericBuilder {
	b.setRenamedFrom(oldName)
	return b
}
func (b *NumericBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Date
// -------------------------------------------------------------------

// DateBuilder builds a date column definition.
type DateBuilder struct{ colBuilder }

// Date starts a SQL date (date-only, no time component) column.
// Scans into time.Time; callers should use only the date portion.
func Date() *DateBuilder {
	b := &DateBuilder{}
	b.def.SQLType = "date"
	b.def.GoType = GoTypeTime
	return b
}

func (b *DateBuilder) NotNull() *DateBuilder { b.setNotNull(); return b }
func (b *DateBuilder) Default(val string) *DateBuilder {
	b.setDefault(quoteSQLLiteral(val))
	return b
}
func (b *DateBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Time / TimeTZ
// -------------------------------------------------------------------

// TimeBuilder builds a time / timetz column definition.
type TimeBuilder struct{ colBuilder }

// Time starts a SQL time (time-of-day without timezone) column.
func Time() *TimeBuilder {
	b := &TimeBuilder{}
	b.def.SQLType = "time"
	b.def.GoType = GoTypeTime
	return b
}

// WithTimezone switches the column to TIMETZ (time with timezone).
func (b *TimeBuilder) WithTimezone() *TimeBuilder {
	b.def.SQLType = "timetz"
	return b
}

func (b *TimeBuilder) NotNull() *TimeBuilder { b.setNotNull(); return b }
func (b *TimeBuilder) Default(val string) *TimeBuilder {
	b.setDefault(quoteSQLLiteral(val))
	return b
}
func (b *TimeBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Interval
// -------------------------------------------------------------------

// IntervalBuilder builds an interval column definition.
type IntervalBuilder struct{ colBuilder }

// Interval starts a SQL interval (duration) column. Scans as a string
// because Go has no native interval type; use pgtype.Interval for richer handling.
func Interval() *IntervalBuilder {
	b := &IntervalBuilder{}
	b.def.SQLType = "interval"
	b.def.GoType = GoTypeString
	return b
}

func (b *IntervalBuilder) NotNull() *IntervalBuilder { b.setNotNull(); return b }
func (b *IntervalBuilder) Default(val string) *IntervalBuilder {
	b.setDefault(quoteSQLLiteral(val) + "::interval")
	return b
}
func (b *IntervalBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Real / DoublePrecision
// -------------------------------------------------------------------

// FloatBuilder builds a real or double precision column definition.
type FloatBuilder struct{ colBuilder }

// Real starts a real (single-precision 4-byte) column. Go type is float64 for API consistency with DoublePrecision and FloatBuilder.Default.
func Real() *FloatBuilder {
	b := &FloatBuilder{}
	b.def.SQLType = "real"
	b.def.GoType = GoTypeFloat64
	return b
}

// DoublePrecision starts a double-precision (8-byte) floating-point column.
func DoublePrecision() *FloatBuilder {
	b := &FloatBuilder{}
	b.def.SQLType = "double precision"
	b.def.GoType = GoTypeFloat64
	return b
}

func (b *FloatBuilder) NotNull() *FloatBuilder { b.setNotNull(); return b }
func (b *FloatBuilder) Default(val float64) *FloatBuilder {
	var s string
	switch {
	case math.IsNaN(val):
		s = fmt.Sprintf("'NaN'::%s", b.def.SQLType)
	case math.IsInf(val, 1):
		s = fmt.Sprintf("'Infinity'::%s", b.def.SQLType)
	case math.IsInf(val, -1):
		s = fmt.Sprintf("'-Infinity'::%s", b.def.SQLType)
	default:
		s = fmt.Sprintf("%g", val)
	}
	b.setDefault(s)
	return b
}
func (b *FloatBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Char
// -------------------------------------------------------------------

// CharBuilder builds a char(n) fixed-length column definition.
type CharBuilder struct{ colBuilder }

// Char starts a char(n) fixed-length character column.
func Char(length int) *CharBuilder {
	b := &CharBuilder{}
	b.def.SQLType = fmt.Sprintf("char(%d)", length)
	b.def.GoType = GoTypeString
	return b
}

func (b *CharBuilder) NotNull() *CharBuilder { b.setNotNull(); return b }
func (b *CharBuilder) Default(val string) *CharBuilder {
	b.setDefault(quoteSQLLiteral(val))
	return b
}
func (b *CharBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Bytea
// -------------------------------------------------------------------

// ByteaBuilder builds a bytea column definition.
type ByteaBuilder struct{ colBuilder }

// Bytea starts a bytea (binary data) column. Scans into []byte.
func Bytea() *ByteaBuilder {
	b := &ByteaBuilder{}
	b.def.SQLType = "bytea"
	b.def.GoType = GoTypeByteSlice
	return b
}

func (b *ByteaBuilder) NotNull() *ByteaBuilder      { b.setNotNull(); return b }
func (b *ByteaBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Inet / Cidr / Macaddr
// -------------------------------------------------------------------

// NetworkBuilder builds network-address column definitions (inet, cidr, macaddr).
type NetworkBuilder struct{ colBuilder }

// Inet starts an inet (IP address, IPv4 or IPv6, with optional subnet) column.
func Inet() *NetworkBuilder {
	b := &NetworkBuilder{}
	b.def.SQLType = "inet"
	b.def.GoType = GoTypeString
	return b
}

// Cidr starts a cidr (network address, host bits must be zero) column.
func Cidr() *NetworkBuilder {
	b := &NetworkBuilder{}
	b.def.SQLType = "cidr"
	b.def.GoType = GoTypeString
	return b
}

// Macaddr starts a macaddr (MAC address) column.
func Macaddr() *NetworkBuilder {
	b := &NetworkBuilder{}
	b.def.SQLType = "macaddr"
	b.def.GoType = GoTypeString
	return b
}

func (b *NetworkBuilder) NotNull() *NetworkBuilder { b.setNotNull(); return b }
func (b *NetworkBuilder) Default(val string) *NetworkBuilder {
	b.setDefault(quoteSQLLiteral(val))
	return b
}
func (b *NetworkBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Tsvector / Tsquery
// -------------------------------------------------------------------

// TextSearchBuilder builds tsvector / tsquery column definitions.
type TextSearchBuilder struct{ colBuilder }

// Tsvector starts a tsvector (full-text search document vector) column.
func Tsvector() *TextSearchBuilder {
	b := &TextSearchBuilder{}
	b.def.SQLType = "tsvector"
	b.def.GoType = GoTypeString
	return b
}

// Tsquery starts a tsquery (full-text search query) column.
func Tsquery() *TextSearchBuilder {
	b := &TextSearchBuilder{}
	b.def.SQLType = "tsquery"
	b.def.GoType = GoTypeString
	return b
}

func (b *TextSearchBuilder) NotNull() *TextSearchBuilder { b.setNotNull(); return b }
func (b *TextSearchBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Range types
// -------------------------------------------------------------------

// RangeBuilder builds PostgreSQL range-type column definitions.
type RangeBuilder struct{ colBuilder }

// Int4Range starts an int4range column (range of integers).
func Int4Range() *RangeBuilder {
	b := &RangeBuilder{}
	b.def.SQLType = "int4range"
	b.def.GoType = GoTypeString
	return b
}

// Int8Range starts an int8range column (range of bigints).
func Int8Range() *RangeBuilder {
	b := &RangeBuilder{}
	b.def.SQLType = "int8range"
	b.def.GoType = GoTypeString
	return b
}

// NumRange starts a numrange column (range of numerics).
func NumRange() *RangeBuilder {
	b := &RangeBuilder{}
	b.def.SQLType = "numrange"
	b.def.GoType = GoTypeString
	return b
}

// TsRange starts a tsrange column (range of timestamps without timezone).
func TsRange() *RangeBuilder {
	b := &RangeBuilder{}
	b.def.SQLType = "tsrange"
	b.def.GoType = GoTypeString
	return b
}

// TstzRange starts a tstzrange column (range of timestamps with timezone).
func TstzRange() *RangeBuilder {
	b := &RangeBuilder{}
	b.def.SQLType = "tstzrange"
	b.def.GoType = GoTypeString
	return b
}

// DateRange starts a daterange column (range of dates).
func DateRange() *RangeBuilder {
	b := &RangeBuilder{}
	b.def.SQLType = "daterange"
	b.def.GoType = GoTypeString
	return b
}

func (b *RangeBuilder) NotNull() *RangeBuilder      { b.setNotNull(); return b }
func (b *RangeBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Enum
// -------------------------------------------------------------------

// EnumColumnBuilder builds a column whose type is a PostgreSQL custom enum type.
type EnumColumnBuilder struct {
	colBuilder
	typeName string
}

// Enum starts an enum column using the given PostgreSQL enum type name.
// values must be non-empty and no individual value may be the empty string.
//
// Panics if values is empty or any value is the empty string.
func Enum(typeName string, values ...string) *EnumColumnBuilder {
	if len(values) == 0 {
		panic("pg.Enum: values must not be empty")
	}
	for i, v := range values {
		if v == "" {
			panic(fmt.Sprintf("pg.Enum: value at index %d must not be empty", i))
		}
	}
	b := &EnumColumnBuilder{typeName: typeName}
	b.def.SQLType = typeName
	b.def.GoType = GoTypeString
	return b
}

func (b *EnumColumnBuilder) NotNull() *EnumColumnBuilder { b.setNotNull(); return b }

// Default sets the DEFAULT value for the enum column.
// Single quotes in the value are doubled to produce valid SQL.
func (b *EnumColumnBuilder) Default(val string) *EnumColumnBuilder {
	b.setDefault(quoteSQLLiteral(val) + "::" + b.typeName)
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot. Leave empty for new columns
// or columns whose name has not changed.
// Remove this call from your schema definition once the migration has been applied.
func (b *EnumColumnBuilder) RenamedFrom(oldName string) *EnumColumnBuilder {
	b.setRenamedFrom(oldName)
	return b
}
func (b *EnumColumnBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// Array
// -------------------------------------------------------------------

// ArrayBuilder builds a PostgreSQL array-type column definition.
// The element type is derived from the inner ColumnBuilder.
type ArrayBuilder struct {
	colBuilder
	inner ColumnBuilder
}

// Array starts a PostgreSQL array column with the given element type.
// Example: pg.Array(pg.Text()) produces a text[] column.
func Array(inner ColumnBuilder) *ArrayBuilder {
	innerDef := inner.Build("")
	b := &ArrayBuilder{inner: inner}
	b.def.SQLType = innerDef.SQLType + "[]"
	b.def.GoType = GoTypeAny
	return b
}

func (b *ArrayBuilder) NotNull() *ArrayBuilder { b.setNotNull(); return b }
func (b *ArrayBuilder) Default(val string) *ArrayBuilder {
	b.setDefault(quoteSQLLiteral(val) + "::" + b.def.SQLType)
	return b
}
func (b *ArrayBuilder) DefaultEmpty() *ArrayBuilder {
	b.setDefault(fmt.Sprintf("ARRAY[]::%s", b.def.SQLType))
	return b
}
func (b *ArrayBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// ColumnBuilder interface — satisfied by all typed builders
// -------------------------------------------------------------------

// ColumnBuilder is implemented by every column builder type.
// It is used by Table() to finalise column names from the C() helper.
type ColumnBuilder interface {
	// Build finalises the column definition, injecting its name and returning
	// an immutable ColumnDef. Typically called via C() rather than directly.
	Build(name string) ColumnDef
}
