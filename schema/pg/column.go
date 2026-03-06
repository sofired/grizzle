// Package pg provides the PostgreSQL schema definition DSL for G-rizzle.
// Use it to declare your database schema in Go; the g-rizzle code generator
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

import "fmt"

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
	FKActionNoAction FKAction = "NO ACTION"
	FKActionRestrict FKAction = "RESTRICT"
	FKActionCascade  FKAction = "CASCADE"
	FKActionSetNull  FKAction = "SET NULL"
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
	DefaultExpr  string     // SQL default expression, e.g. "gen_random_uuid()", "now()", "'true'"
	PrimaryKey   bool
	Unique       bool
	References   *FKRef
	JsonbGoType  string     // For JSONB columns: the Go type hint for $type<T> equivalent
	GeneratedAs  string     // For generated columns: the SQL expression
	OnUpdateExpr string     // Hint for app-layer: set this expression on every UPDATE
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
func (b *colBuilder) setPrimaryKey() {
	b.def.PrimaryKey = true
	b.def.NotNull = true   // PK is implicitly NOT NULL
	b.def.HasDefault = true // PK usually has a default (serial/uuid)
}
func (b *colBuilder) setReferences(table, col string, onDelete, onUpdate FKAction) {
	b.def.References = &FKRef{
		Table:    table,
		Column:   col,
		OnDelete: onDelete,
		OnUpdate: onUpdate,
	}
}

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
func (b *UUIDBuilder) PrimaryKey() *UUIDBuilder  { b.setPrimaryKey(); return b }
func (b *UUIDBuilder) Unique() *UUIDBuilder      { b.def.Unique = true; return b }

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
	b.setDefault(fmt.Sprintf("'%s'", val))
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

func (b *TimestampBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// JSONB
// -------------------------------------------------------------------

// JSONBBuilder builds a jsonb column definition.
type JSONBBuilder struct {
	colBuilder
	goTypeOverride string
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
	b.setDefault(fmt.Sprintf("'%s'::jsonb", jsonExpr))
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
func (b *JSONBBuilder) Build(name string) ColumnDef { return b.build(name) }

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
func (b *NumericBuilder) Build(name string) ColumnDef { return b.build(name) }

// -------------------------------------------------------------------
// ColumnBuilder interface — satisfied by all typed builders
// -------------------------------------------------------------------

// ColumnBuilder is implemented by every column builder type.
// It is used by Table() to finalise column names from the C() helper.
type ColumnBuilder interface {
	Build(name string) ColumnDef
}
