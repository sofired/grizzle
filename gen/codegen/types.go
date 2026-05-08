package codegen

import (
	"fmt"

	"github.com/sofired/grizzle/gen/parser"
)

// ColumnInfo is the processed, codegen-ready description of a single column.
type ColumnInfo struct {
	ColName      string // SQL column name, e.g. "display_name"
	FieldName    string // Go struct field name, e.g. "DisplayName"
	ColType      string // expr column type, e.g. "expr.StringColumn"
	GoType       string // Go value type, e.g. "string", "*string", "uuid.UUID"
	GoTypePtr    string // Pointer form of GoType, e.g. "*string", "*uuid.UUID"
	IsNullable   bool   // True if the column is nullable (no NOT NULL)
	HasDefault   bool   // True if the column has a DB default
	IsPK         bool   // True if PRIMARY KEY
	IsOmitEmpty  bool   // True if insert model field should use omitempty
	JSONBGeneric string // Non-empty if this is a JSONBColumn[T]; holds T e.g. "map[string]any"
	NeedsImport  string // Non-empty if a special import is needed (e.g. "github.com/google/uuid")
}

// ResolveColumn converts a ParsedColumn into a ColumnInfo by interpreting
// the builder chain (base type + method modifiers).
func ResolveColumn(col parser.ParsedColumn) (ColumnInfo, error) {
	chain := col.Chain
	info := ColumnInfo{
		ColName:   col.Name,
		FieldName: snakeToPascal(col.Name),
	}

	// Read modifier flags from method calls.
	for _, m := range chain.Methods {
		switch m.Name {
		case "NotNull":
			// handled below — absence of NotNull means nullable
		case "PrimaryKey":
			info.IsPK = true
		case "Unique":
			// no effect on codegen types
		case "Default", "DefaultRandom", "DefaultNow", "DefaultFalse", "DefaultTrue", "DefaultEmpty", "DefaultEmptyArray":
			info.HasDefault = true
		case "References":
			// FK — no type change
		case "WithTimezone", "Precision":
			// no effect on codegen types
		case "Generated", "OnUpdate":
			info.HasDefault = true
		}
	}

	// Check if NOT NULL appears in chain.
	hasNotNull := info.IsPK // PKs are implicitly NOT NULL
	for _, m := range chain.Methods {
		if m.Name == "NotNull" {
			hasNotNull = true
			break
		}
	}
	info.IsNullable = !hasNotNull

	// Determine whether insert model should use omitempty:
	// omitempty if the column has a DB default OR is nullable.
	info.IsOmitEmpty = info.HasDefault || info.IsNullable

	// Map base function to types.
	if err := applyBaseType(&info, chain); err != nil {
		return ColumnInfo{}, fmt.Errorf("column %q: %w", col.Name, err)
	}

	return info, nil
}

// applyBaseType fills ColType, GoType, GoTypePtr, NeedsImport, JSONBGeneric
// based on the builder's base function name (UUID, Varchar, Boolean, etc.).
func applyBaseType(info *ColumnInfo, chain *parser.ChainResult) error {
	switch chain.BaseFn {
	case "UUID":
		info.ColType = "expr.UUIDColumn"
		info.GoType = "uuid.UUID"
		info.GoTypePtr = "*uuid.UUID"
		info.NeedsImport = "github.com/google/uuid"

	// Set is MySQL-specific; Varchar, Text, and Char are cross-dialect. Enum is handled separately below.
	case "Varchar", "Text", "Char", "Set":
		info.ColType = "expr.StringColumn"
		info.GoType = "string"
		info.GoTypePtr = "*string"

	case "Boolean":
		info.ColType = "expr.BoolColumn"
		info.GoType = "bool"
		info.GoTypePtr = "*bool"

	// MySQL-specific: TinyInt (1-byte), MediumInt (3-byte), Year map to IntColumn.
	case "Integer", "SmallInt", "Serial", "SmallSerial", "TinyInt", "MediumInt", "Year":
		info.ColType = "expr.IntColumn"
		info.GoType = "int"
		info.GoTypePtr = "*int"

	case "BigInt", "BigSerial":
		info.ColType = "expr.BigIntColumn"
		info.GoType = "int64"
		info.GoTypePtr = "*int64"

	// Numeric(p,s) maps to string to avoid precision loss. float64 cannot
	// represent arbitrary decimal values without rounding; string preserves the
	// exact representation returned by the database driver. See codegen.md,
	// "Go type mappings" section, and issue #236.
	case "Numeric":
		info.ColType = "expr.StringColumn"
		info.GoType = "string"
		info.GoTypePtr = "*string"

	// MySQL-specific: Double maps to FloatColumn.
	// SQLite-specific: Real maps to FloatColumn (SQLite REAL storage class).
	case "Real", "DoublePrecision", "Double":
		info.ColType = "expr.FloatColumn"
		info.GoType = "float64"
		info.GoTypePtr = "*float64"

	case "Timestamp", "Time":
		info.ColType = "expr.TimestampColumn"
		info.GoType = "time.Time"
		info.GoTypePtr = "*time.Time"
		info.NeedsImport = "time"

	case "Date":
		info.ColType = "expr.DateColumn"
		info.GoType = "time.Time"
		info.GoTypePtr = "*time.Time"
		info.NeedsImport = "time"

	// SQLite-specific: Blob maps to BytesColumn (raw binary data, []byte in Go).
	// PostgreSQL: Bytea maps to BytesColumn (binary data).
	case "Blob", "Bytea":
		info.ColType = "expr.BytesColumn"
		info.GoType = "[]byte"
		info.GoTypePtr = "*[]byte"

	case "Interval":
		info.ColType = "expr.IntervalColumn"
		info.GoType = "string"
		info.GoTypePtr = "*string"

	case "Enum":
		// MySQL inline enum is string-typed; PostgreSQL custom enum uses EnumColumn.
		if chain.BasePkg == "mysql" {
			info.ColType = "expr.StringColumn"
		} else {
			info.ColType = "expr.EnumColumn"
		}
		info.GoType = "string"
		info.GoTypePtr = "*string"

	case "Inet", "Cidr", "Macaddr":
		info.ColType = "expr.InetColumn"
		info.GoType = "string"
		info.GoTypePtr = "*string"

	case "Tsvector":
		info.ColType = "expr.TsvectorColumn"
		info.GoType = "string"
		info.GoTypePtr = "*string"

	case "Tsquery":
		info.ColType = "expr.StringColumn"
		info.GoType = "string"
		info.GoTypePtr = "*string"

	case "Int4Range", "Int8Range", "NumRange", "TsRange", "TstzRange", "DateRange":
		info.ColType = "expr.StringColumn"
		info.GoType = "string"
		info.GoTypePtr = "*string"

	case "Array":
		info.ColType = "expr.ArrayColumn"
		info.GoType = "any"
		info.GoTypePtr = "*any"

	case "JSONB", "JSON":
		// Default JSONB generic type is map[string]any.
		// If the user called .Type("MyStruct") in the chain, honour that type.
		goType := "map[string]any"
		for _, m := range chain.Methods {
			if m.Name == "Type" && len(m.Args) == 1 {
				if s, ok := m.Args[0].(string); ok && s != "" {
					goType = s
				}
			}
		}
		info.ColType = "expr.JSONBColumn[" + goType + "]"
		info.GoType = goType
		info.GoTypePtr = "*" + goType
		info.JSONBGeneric = goType

	default:
		return fmt.Errorf("unknown column builder %q", chain.BaseFn)
	}

	return nil
}

// SelectGoType returns the Go type for this column in a Select model.
// Nullable columns become pointer types.
func (c ColumnInfo) SelectGoType() string {
	if c.IsNullable {
		return c.GoTypePtr
	}
	return c.GoType
}

// InsertGoType returns the Go type for this column in an Insert model.
// Columns with defaults (or nullable) become pointer types so they can be omitted.
func (c ColumnInfo) InsertGoType() string {
	if c.IsOmitEmpty {
		return c.GoTypePtr
	}
	return c.GoType
}

// InsertTag returns the `db:"..."` struct tag for the Insert model.
func (c ColumnInfo) InsertTag() string {
	if c.IsOmitEmpty {
		return fmt.Sprintf(`db:"%s,omitempty"`, c.ColName)
	}
	return fmt.Sprintf(`db:"%s"`, c.ColName)
}
