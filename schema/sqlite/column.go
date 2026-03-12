// Package sqlite provides the SQLite schema definition DSL for Grizzle.
//
// Schemas defined with this package use the same ColumnDef and TableDef types
// as schema/pg. The kit layer preserves canonical SQL type names in generated
// DDL and relies on SQLite's flexible type affinity system for storage. In
// practice, common canonical types will be stored with the following effective
// affinities in SQLite:
//
//   - uuid        → TEXT affinity (SQLite has no native UUID type)
//   - boolean     → INTEGER affinity (0/1)
//   - timestamptz → TEXT affinity (ISO-8601 strings are the idiomatic approach)
//   - timestamp   → TEXT affinity
//   - json/jsonb  → TEXT affinity
//
// Default expressions for canonical types may be translated to SQLite-compatible
// literals (for example, boolean defaults become 0/1).
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

// UUID starts a UUID column (stored as TEXT in SQLite).
var UUID = pg.UUID

// Varchar starts a VARCHAR(n) column (TEXT affinity in SQLite).
var Varchar = pg.Varchar

// Text starts an unbounded TEXT column.
var Text = pg.Text

// Boolean starts a boolean column (stored as INTEGER 0/1 in SQLite).
var Boolean = pg.Boolean

// Integer starts a 4-byte integer column (INTEGER affinity in SQLite).
var Integer = pg.Integer

// BigInt starts an 8-byte integer column (INTEGER affinity in SQLite).
var BigInt = pg.BigInt

// Serial starts an auto-incrementing integer column.
// In SQLite, INTEGER PRIMARY KEY is the canonical auto-increment idiom.
var Serial = pg.Serial

// BigSerial starts an auto-incrementing 8-byte integer column.
var BigSerial = pg.BigSerial

// Timestamp starts a timestamp column (stored as TEXT in SQLite).
var Timestamp = pg.Timestamp

// Numeric starts a NUMERIC(precision, scale) column (NUMERIC affinity in SQLite).
var Numeric = pg.Numeric

// JSON starts a JSON column (stored as TEXT in SQLite).
var JSON = pg.JSON

// JSONB starts a JSONB column (stored as TEXT in SQLite).
var JSONB = pg.JSONB

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
func (b *BlobBuilder) Build(name string) pg.ColumnDef {
	if b.def.Name == "" {
		b.def.Name = name
	}
	return b.def
}
