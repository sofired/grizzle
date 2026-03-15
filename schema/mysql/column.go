// Package mysql provides the MySQL schema definition DSL for Grizzle.
//
// Schemas defined with this package use the same ColumnDef and TableDef types
// as schema/pg. The kit layer (kit.GenerateCreateSQLMySQL) translates canonical
// SQL types and default expressions to MySQL-native syntax at DDL-generation time:
//
//   - uuid          → CHAR(36)
//   - boolean       → TINYINT(1)
//   - timestamptz   → DATETIME(6)
//   - timestamp     → DATETIME
//   - json / jsonb  → JSON
//   - gen_random_uuid() → (UUID())
//   - now()         → CURRENT_TIMESTAMP(6)
//
// Example:
//
//	var Users = mysql.Table("users",
//	    mysql.C("id",         mysql.UUID().PrimaryKey().DefaultRandom()),
//	    mysql.C("username",   mysql.Varchar(255).NotNull()),
//	    mysql.C("enabled",    mysql.Boolean().NotNull().Default(true)),
//	    mysql.C("score",      mysql.Numeric(10, 2)),
//	    mysql.C("meta",       mysql.JSON()),
//	    mysql.C("created_at", mysql.Timestamp().WithTimezone().NotNull().DefaultNow()),
//	    mysql.C("deleted_at", mysql.Timestamp().WithTimezone()),
//	)
package mysql

import (
	"fmt"
	"strings"

	pg "github.com/sofired/grizzle/schema/pg"
)

// ---------------------------------------------------------------------------
// Type aliases — MySQL definitions share underlying types with schema/pg
// ---------------------------------------------------------------------------

type (
	// ColumnDef is the fully-resolved definition of a single column.
	ColumnDef = pg.ColumnDef

	// GoTypeHint describes the Go type used in generated code.
	GoTypeHint = pg.GoTypeHint

	// FKAction describes what happens on parent row delete/update.
	FKAction = pg.FKAction

	// FKRef holds an inline foreign key reference.
	FKRef = pg.FKRef

	// FKOption configures inline foreign key behaviour.
	FKOption = pg.FKOption

	// ColumnBuilder is satisfied by every column builder type.
	ColumnBuilder = pg.ColumnBuilder
)

// FKAction constants.
const (
	FKActionNoAction   = pg.FKActionNoAction
	FKActionRestrict   = pg.FKActionRestrict
	FKActionCascade    = pg.FKActionCascade
	FKActionSetNull    = pg.FKActionSetNull
	FKActionSetDefault = pg.FKActionSetDefault
)

// OnDelete returns an FKOption that sets the ON DELETE action.
var OnDelete = pg.OnDelete

// OnUpdate returns an FKOption that sets the ON UPDATE action.
var OnUpdate = pg.OnUpdate

// ---------------------------------------------------------------------------
// Standard column builders — re-exported from schema/pg
// ---------------------------------------------------------------------------
//
// These produce identical ColumnDef values. The kit layer handles all
// MySQL-specific DDL translation.

// UUID starts a UUID column (stored as CHAR(36) in MySQL).
var UUID = pg.UUID

// Varchar starts a VARCHAR(n) column.
var Varchar = pg.Varchar

// Text starts an unbounded text column (stored as LONGTEXT in MySQL).
var Text = pg.Text

// Boolean starts a boolean column (stored as TINYINT(1) in MySQL).
var Boolean = pg.Boolean

// Integer starts a 4-byte integer column (INT in MySQL).
var Integer = pg.Integer

// BigInt starts an 8-byte integer column (BIGINT in MySQL).
var BigInt = pg.BigInt

// Serial starts an auto-incrementing 4-byte integer (INT AUTO_INCREMENT in MySQL).
var Serial = pg.Serial

// BigSerial starts an auto-incrementing 8-byte integer (BIGINT AUTO_INCREMENT in MySQL).
var BigSerial = pg.BigSerial

// Timestamp starts a timestamp column.
// Chain .WithTimezone() to get DATETIME(6) in MySQL; plain Timestamp() gives DATETIME.
var Timestamp = pg.Timestamp

// Numeric starts a NUMERIC(precision, scale) column.
var Numeric = pg.Numeric

// JSON starts a JSON column. MySQL supports JSON natively since version 5.7.8.
// The column is stored as the MySQL JSON type with server-side validation.
var JSON = pg.JSON

// ---------------------------------------------------------------------------
// MySQL-specific column builders
// ---------------------------------------------------------------------------

// TinyIntBuilder builds a TINYINT column definition.
// Use Boolean() for boolean flags (TINYINT(1)); use TinyInt() for small
// integer values in the range -128 to 127.
type TinyIntBuilder struct {
	def pg.ColumnDef
}

// TinyInt starts a TINYINT column (1-byte signed integer).
func TinyInt() *TinyIntBuilder {
	b := &TinyIntBuilder{}
	b.def.SQLType = "tinyint"
	b.def.GoType = pg.GoTypeInt
	return b
}

func (b *TinyIntBuilder) NotNull() *TinyIntBuilder { b.def.NotNull = true; return b }
func (b *TinyIntBuilder) PrimaryKey() *TinyIntBuilder {
	b.def.PrimaryKey = true
	b.def.NotNull = true
	return b
}
func (b *TinyIntBuilder) Default(val int) *TinyIntBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = fmt.Sprintf("%d", val)
	return b
}
func (b *TinyIntBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// SmallIntBuilder builds a SMALLINT column definition.
type SmallIntBuilder struct {
	def pg.ColumnDef
}

// SmallInt starts a SMALLINT column (2-byte signed integer, -32768 to 32767).
func SmallInt() *SmallIntBuilder {
	b := &SmallIntBuilder{}
	b.def.SQLType = "smallint"
	b.def.GoType = pg.GoTypeInt
	return b
}

func (b *SmallIntBuilder) NotNull() *SmallIntBuilder { b.def.NotNull = true; return b }
func (b *SmallIntBuilder) Default(val int) *SmallIntBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = fmt.Sprintf("%d", val)
	return b
}
func (b *SmallIntBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// DoubleBuilder builds a DOUBLE column definition.
type DoubleBuilder struct {
	def pg.ColumnDef
}

// Double starts a DOUBLE (double-precision floating point) column.
func Double() *DoubleBuilder {
	b := &DoubleBuilder{}
	b.def.SQLType = "double precision"
	b.def.GoType = pg.GoTypeFloat64
	return b
}

func (b *DoubleBuilder) NotNull() *DoubleBuilder { b.def.NotNull = true; return b }
func (b *DoubleBuilder) Default(val float64) *DoubleBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = fmt.Sprintf("%g", val)
	return b
}
func (b *DoubleBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// MediumIntBuilder builds a MEDIUMINT column definition.
type MediumIntBuilder struct {
	def pg.ColumnDef
}

// MediumInt starts a MEDIUMINT column (3-byte signed integer, -8388608 to 8388607).
func MediumInt() *MediumIntBuilder {
	b := &MediumIntBuilder{}
	b.def.SQLType = "mediumint"
	b.def.GoType = pg.GoTypeInt
	return b
}

func (b *MediumIntBuilder) NotNull() *MediumIntBuilder { b.def.NotNull = true; return b }
func (b *MediumIntBuilder) PrimaryKey() *MediumIntBuilder {
	b.def.PrimaryKey = true
	b.def.NotNull = true
	return b
}
func (b *MediumIntBuilder) Default(val int) *MediumIntBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = fmt.Sprintf("%d", val)
	return b
}
func (b *MediumIntBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// YearBuilder builds a YEAR column definition.
type YearBuilder struct {
	def pg.ColumnDef
}

// Year starts a YEAR column (MySQL YEAR type, stores 1901–2155 as an integer).
func Year() *YearBuilder {
	b := &YearBuilder{}
	b.def.SQLType = "year"
	b.def.GoType = pg.GoTypeInt
	return b
}

func (b *YearBuilder) NotNull() *YearBuilder { b.def.NotNull = true; return b }
func (b *YearBuilder) Default(val int) *YearBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = fmt.Sprintf("%d", val)
	return b
}
func (b *YearBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// EnumBuilder builds a MySQL ENUM column definition.
type EnumBuilder struct {
	def pg.ColumnDef
}

// Enum starts a MySQL ENUM column with the given allowed values.
// The values are stored as part of the SQL type: enum('v1','v2',...).
// The Go type is string.
//
//	mysql.C("status", mysql.Enum("active", "inactive", "pending").NotNull())
func Enum(values ...string) *EnumBuilder {
	b := &EnumBuilder{}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	b.def.SQLType = "enum(" + strings.Join(parts, ",") + ")"
	b.def.GoType = pg.GoTypeString
	return b
}

func (b *EnumBuilder) NotNull() *EnumBuilder { b.def.NotNull = true; return b }
func (b *EnumBuilder) Default(val string) *EnumBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = fmt.Sprintf("'%s'", strings.ReplaceAll(val, "'", "''"))
	return b
}
func (b *EnumBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// SetBuilder builds a MySQL SET column definition.
type SetBuilder struct {
	def pg.ColumnDef
}

// Set starts a MySQL SET column with the given allowed values.
// The values are stored as part of the SQL type: set('v1','v2',...).
// A SET column can hold zero or more of the listed values; the Go type is string.
//
//	mysql.C("tags", mysql.Set("a", "b", "c"))
func Set(values ...string) *SetBuilder {
	b := &SetBuilder{}
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	b.def.SQLType = "set(" + strings.Join(parts, ",") + ")"
	b.def.GoType = pg.GoTypeString
	return b
}

func (b *SetBuilder) NotNull() *SetBuilder { b.def.NotNull = true; return b }
func (b *SetBuilder) Default(val string) *SetBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = fmt.Sprintf("'%s'", strings.ReplaceAll(val, "'", "''"))
	return b
}
func (b *SetBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}
