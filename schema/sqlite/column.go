// Package sqlite provides the SQLite schema definition DSL for Grizzle.
//
// Schemas defined with this package use the same ColumnDef and TableDef types
// as schema/pg. The kit layer translates canonical SQL types to SQLite-native
// types at DDL-generation time, matching Drizzle ORM's SQLite behavior:
//
//   - boolean/bool           → INTEGER (0/1)
//   - uuid                   → TEXT
//   - timestamp/timestamptz  → TEXT (ISO-8601)
//   - json/jsonb              → TEXT
//   - bigint/smallint/int*   → INTEGER
//   - varchar/char/text/*    → TEXT
//   - bytea                  → BLOB
//   - numeric/decimal        → NUMERIC
//   - real/float/double      → REAL
//   - serial/bigserial       → INTEGER PRIMARY KEY AUTOINCREMENT
//
// SQLite's native storage classes are NULL, INTEGER, REAL, TEXT, and BLOB.
// The affinity types INTEGER, REAL, TEXT, NUMERIC, and BLOB are also supported
// as first-class column builders.
//
// Example:
//
//	var Notes = sqlite.Table("notes",
//	    sqlite.C("id",         sqlite.Integer().PrimaryKey()),
//	    sqlite.C("title",      sqlite.Text().NotNull()),
//	    sqlite.C("body",       sqlite.Text()),
//	    sqlite.C("score",      sqlite.Real()),
//	    sqlite.C("data",       sqlite.Blob()),
//	    sqlite.C("created_at", sqlite.Text().NotNull()),
//	)
package sqlite

import (
	"fmt"

	pg "github.com/sofired/grizzle/schema/pg"
)

// ---------------------------------------------------------------------------
// Type aliases — SQLite definitions share underlying types with schema/pg
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
// Standard column builders — re-exported from schema/pg where applicable
// ---------------------------------------------------------------------------

// UUIDBuilder builds a UUID column definition for SQLite (stored as TEXT).
type UUIDBuilder struct {
	def pg.ColumnDef
}

// UUID starts a UUID column. In SQLite, UUIDs are stored as TEXT.
func UUID() *UUIDBuilder {
	b := &UUIDBuilder{}
	b.def.SQLType = "text"
	b.def.GoType = pg.GoTypeUUID
	return b
}

func (b *UUIDBuilder) NotNull() *UUIDBuilder { b.def.NotNull = true; return b }
func (b *UUIDBuilder) PrimaryKey() *UUIDBuilder {
	b.def.PrimaryKey = true
	b.def.NotNull = true
	return b
}

// DefaultRandom sets the column default to a SQLite-compatible UUID v4 expression.
func (b *UUIDBuilder) DefaultRandom() *UUIDBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = "lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || '4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))"
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot.
// Remove this call from your schema definition once the migration has been applied.
func (b *UUIDBuilder) RenamedFrom(oldName string) *UUIDBuilder {
	b.def.PreviousName = oldName
	return b
}

func (b *UUIDBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// Varchar starts a VARCHAR(n) column (TEXT affinity in SQLite).
var Varchar = pg.Varchar

// Text starts an unbounded TEXT column.
var Text = pg.Text

// BooleanBuilder builds a boolean column definition for SQLite (stored as INTEGER 0/1).
type BooleanBuilder struct {
	def pg.ColumnDef
}

// Boolean starts a boolean column. In SQLite, booleans are stored as INTEGER (0=false, 1=true).
func Boolean() *BooleanBuilder {
	b := &BooleanBuilder{}
	b.def.SQLType = "integer"
	b.def.GoType = pg.GoTypeBool
	return b
}

func (b *BooleanBuilder) NotNull() *BooleanBuilder { b.def.NotNull = true; return b }
func (b *BooleanBuilder) PrimaryKey() *BooleanBuilder {
	b.def.PrimaryKey = true
	b.def.NotNull = true
	return b
}

// Default sets the column default to 1 (true) or 0 (false).
func (b *BooleanBuilder) Default(val bool) *BooleanBuilder {
	b.def.HasDefault = true
	if val {
		b.def.DefaultExpr = "1"
	} else {
		b.def.DefaultExpr = "0"
	}
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot.
// Remove this call from your schema definition once the migration has been applied.
func (b *BooleanBuilder) RenamedFrom(oldName string) *BooleanBuilder {
	b.def.PreviousName = oldName
	return b
}

func (b *BooleanBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// Integer starts a 4-byte integer column (INTEGER affinity in SQLite).
var Integer = pg.Integer

// BigInt starts an 8-byte integer column (INTEGER affinity in SQLite).
var BigInt = pg.BigInt

// Serial starts an auto-incrementing integer column.
// In SQLite, INTEGER PRIMARY KEY is the canonical auto-increment idiom.
var Serial = pg.Serial

// BigSerial starts an auto-incrementing 8-byte integer column.
var BigSerial = pg.BigSerial

// SQLiteTimestampBuilder builds a timestamp column definition for SQLite (stored as TEXT).
type SQLiteTimestampBuilder struct {
	def pg.ColumnDef
}

// Timestamp starts a timestamp column. In SQLite, timestamps are stored as TEXT (ISO-8601).
// Both timestamp and timestamptz map to TEXT; timezone offsets are preserved in the string.
func Timestamp() *SQLiteTimestampBuilder {
	b := &SQLiteTimestampBuilder{}
	b.def.SQLType = "text"
	b.def.GoType = pg.GoTypeTime
	return b
}

func (b *SQLiteTimestampBuilder) NotNull() *SQLiteTimestampBuilder {
	b.def.NotNull = true
	return b
}
func (b *SQLiteTimestampBuilder) PrimaryKey() *SQLiteTimestampBuilder {
	b.def.PrimaryKey = true
	b.def.NotNull = true
	return b
}

// WithTimezone is a no-op for SQLite — both timestamp and timestamptz are stored as TEXT.
func (b *SQLiteTimestampBuilder) WithTimezone() *SQLiteTimestampBuilder { return b }

// DefaultNow sets the column default to CURRENT_TIMESTAMP (SQLite built-in).
func (b *SQLiteTimestampBuilder) DefaultNow() *SQLiteTimestampBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = "CURRENT_TIMESTAMP"
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot.
// Remove this call from your schema definition once the migration has been applied.
func (b *SQLiteTimestampBuilder) RenamedFrom(oldName string) *SQLiteTimestampBuilder {
	b.def.PreviousName = oldName
	return b
}

func (b *SQLiteTimestampBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// Numeric starts a NUMERIC(precision, scale) column (NUMERIC affinity in SQLite).
var Numeric = pg.Numeric

// SQLiteJSONBuilder builds a JSON/JSONB column definition for SQLite (stored as TEXT).
type SQLiteJSONBuilder struct {
	def pg.ColumnDef
}

// JSON starts a JSON column. In SQLite, JSON is stored as TEXT.
func JSON() *SQLiteJSONBuilder {
	b := &SQLiteJSONBuilder{}
	b.def.SQLType = "text"
	b.def.GoType = pg.GoTypeAny
	return b
}

// JSONB starts a JSONB column. In SQLite, JSONB is stored as TEXT.
func JSONB() *SQLiteJSONBuilder {
	b := &SQLiteJSONBuilder{}
	b.def.SQLType = "text"
	b.def.GoType = pg.GoTypeAny
	return b
}

func (b *SQLiteJSONBuilder) NotNull() *SQLiteJSONBuilder { b.def.NotNull = true; return b }
func (b *SQLiteJSONBuilder) PrimaryKey() *SQLiteJSONBuilder {
	b.def.PrimaryKey = true
	b.def.NotNull = true
	return b
}

// Default sets the column default to the given JSON expression literal.
func (b *SQLiteJSONBuilder) Default(jsonExpr string) *SQLiteJSONBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = fmt.Sprintf("'%s'", jsonExpr)
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot.
// Remove this call from your schema definition once the migration has been applied.
func (b *SQLiteJSONBuilder) RenamedFrom(oldName string) *SQLiteJSONBuilder {
	b.def.PreviousName = oldName
	return b
}

func (b *SQLiteJSONBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// ---------------------------------------------------------------------------
// SQLite-specific column builders
// ---------------------------------------------------------------------------

// RealBuilder builds a REAL column definition (64-bit IEEE 754 float).
type RealBuilder struct {
	def pg.ColumnDef
}

// Real starts a REAL column (8-byte IEEE 754 floating point number).
// This maps directly to SQLite's REAL storage class.
func Real() *RealBuilder {
	b := &RealBuilder{}
	b.def.SQLType = "real"
	b.def.GoType = pg.GoTypeFloat64
	return b
}

func (b *RealBuilder) NotNull() *RealBuilder { b.def.NotNull = true; return b }
func (b *RealBuilder) PrimaryKey() *RealBuilder {
	b.def.PrimaryKey = true
	b.def.NotNull = true
	return b
}
func (b *RealBuilder) Default(val float64) *RealBuilder {
	b.def.HasDefault = true
	b.def.DefaultExpr = fmt.Sprintf("%g", val)
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot.
// Remove this call from your schema definition once the migration has been applied.
func (b *RealBuilder) RenamedFrom(oldName string) *RealBuilder {
	b.def.PreviousName = oldName
	return b
}

func (b *RealBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}

// BlobBuilder builds a BLOB column definition.
type BlobBuilder struct {
	def pg.ColumnDef
}

// Blob starts a BLOB column (binary large object, stored as raw bytes).
// This maps directly to SQLite's BLOB storage class. The Go scan type is []byte.
func Blob() *BlobBuilder {
	b := &BlobBuilder{}
	b.def.SQLType = "blob"
	b.def.GoType = pg.GoTypeByteSlice
	return b
}

func (b *BlobBuilder) NotNull() *BlobBuilder { b.def.NotNull = true; return b }
func (b *BlobBuilder) PrimaryKey() *BlobBuilder {
	b.def.PrimaryKey = true
	b.def.NotNull = true
	return b
}

// RenamedFrom declares that this column was renamed from oldName.
// Diff() will emit ChangeRenameColumn instead of drop+add when oldName
// matches a dropped column in the old snapshot.
// Remove this call from your schema definition once the migration has been applied.
func (b *BlobBuilder) RenamedFrom(oldName string) *BlobBuilder {
	b.def.PreviousName = oldName
	return b
}

func (b *BlobBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}
