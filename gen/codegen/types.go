package codegen

import (
	"fmt"

	"github.com/sofired/grizzle/gen/parser"
)

// ColumnInfo is the processed, codegen-ready description of a single column.
type ColumnInfo struct {
	ColName    string // SQL column name, e.g. "display_name"
	FieldName  string // Go struct field name, e.g. "DisplayName"
	ColType    string // expr column type, e.g. "expr.StringColumn"
	GoType     string // Go value type, e.g. "string", "*string", "uuid.UUID"
	GoTypePtr  string // Pointer form of GoType, e.g. "*string", "*uuid.UUID"
	IsNullable bool   // True if the column is nullable (no NOT NULL)
	HasDefault bool   // True if the column has a DB default
	IsPK       bool   // True if PRIMARY KEY
	IsOmitEmpty bool  // True if insert model field should use omitempty
	JSONBGeneric string // Non-empty if this is a JSONBColumn[T]; holds T e.g. "map[string]any"
	NeedsImport string  // Non-empty if a special import is needed (e.g. "github.com/google/uuid")
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
		case "Default", "DefaultRandom", "DefaultNow", "DefaultEmpty", "DefaultFalse", "DefaultTrue":
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

	case "Varchar", "Text", "Char":
		info.ColType = "expr.StringColumn"
		info.GoType = "string"
		info.GoTypePtr = "*string"

	case "Boolean":
		info.ColType = "expr.BoolColumn"
		info.GoType = "bool"
		info.GoTypePtr = "*bool"

	case "Integer", "BigInt", "SmallInt", "Serial", "BigSerial", "SmallSerial":
		info.ColType = "expr.IntColumn"
		info.GoType = "int64"
		info.GoTypePtr = "*int64"

	case "Numeric", "Real", "DoublePrecision":
		info.ColType = "expr.FloatColumn"
		info.GoType = "float64"
		info.GoTypePtr = "*float64"

	case "Timestamp", "Date", "Time":
		info.ColType = "expr.TimestampColumn"
		info.GoType = "time.Time"
		info.GoTypePtr = "*time.Time"
		info.NeedsImport = "time"

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
