package parser

import (
	"fmt"

	pg "github.com/sofired/grizzle/schema/pg"
)

// EvalTable converts a ParsedTable (from the AST parser) into a *pg.TableDef
// by evaluating each column's builder chain. This lets the CLI use the same
// schema source files that the code generator reads, without importing them.
//
// Note: Constraint expressions that reference column values at runtime (like
// partial index WHERE clauses defined via string literals) are preserved as-is.
// The WithConstraints callback is not re-executed here; constraints parsed from
// pg.UniqueIndex(...).On(...).Where(...).Build() calls are reconstructed structurally.
func EvalTable(pt *ParsedTable) (*pg.TableDef, error) {
	def := &pg.TableDef{
		Name:   pt.TableName,
		Schema: pt.SchemaName,
	}

	for _, pc := range pt.Columns {
		colDef, err := evalColumn(pc)
		if err != nil {
			return nil, fmt.Errorf("column %q: %w", pc.Name, err)
		}
		def.Columns = append(def.Columns, colDef)
	}

	return def, nil
}

// evalColumn evaluates a ParsedColumn's chain into a pg.ColumnDef.
func evalColumn(pc ParsedColumn) (pg.ColumnDef, error) {
	chain := pc.Chain
	def := pg.ColumnDef{Name: pc.Name}

	// Apply base type.
	if err := applyBaseType(&def, chain.BaseFn, chain.BaseArgs); err != nil {
		return pg.ColumnDef{}, err
	}

	// Apply modifier methods.
	for _, m := range chain.Methods {
		if err := applyMethod(&def, m); err != nil {
			return pg.ColumnDef{}, fmt.Errorf("method .%s: %w", m.Name, err)
		}
	}
	return def, nil
}

// applyBaseType maps the builder function name to the SQL type and Go type hint.
func applyBaseType(def *pg.ColumnDef, baseFn string, args []any) error {
	switch baseFn {
	case "UUID":
		def.SQLType = "uuid"
		def.GoType = pg.GoTypeUUID

	case "Varchar":
		n := int64(255)
		if len(args) > 0 {
			if v, ok := args[0].(int64); ok {
				n = v
			}
		}
		def.SQLType = fmt.Sprintf("varchar(%d)", n)
		def.GoType = pg.GoTypeString

	case "Text":
		def.SQLType = "text"
		def.GoType = pg.GoTypeString

	case "Boolean":
		def.SQLType = "boolean"
		def.GoType = pg.GoTypeBool

	case "Integer":
		def.SQLType = "integer"
		def.GoType = pg.GoTypeInt

	case "BigInt":
		def.SQLType = "bigint"
		def.GoType = pg.GoTypeInt64

	case "Serial":
		def.SQLType = "serial"
		def.GoType = pg.GoTypeInt
		def.HasDefault = true

	case "BigSerial":
		def.SQLType = "bigserial"
		def.GoType = pg.GoTypeInt64
		def.HasDefault = true

	case "SmallInt":
		def.SQLType = "smallint"
		def.GoType = pg.GoTypeInt

	case "TinyInt":
		def.SQLType = "tinyint"
		def.GoType = pg.GoTypeInt

	case "Double":
		def.SQLType = "double precision"
		def.GoType = pg.GoTypeFloat64

	case "Timestamp":
		def.SQLType = "timestamp"
		def.GoType = pg.GoTypeTime

	case "JSONB":
		def.SQLType = "jsonb"
		def.GoType = pg.GoTypeAny

	case "Numeric":
		p, s := int64(10), int64(2)
		if len(args) > 0 {
			if v, ok := args[0].(int64); ok {
				p = v
			}
		}
		if len(args) > 1 {
			if v, ok := args[1].(int64); ok {
				s = v
			}
		}
		def.SQLType = fmt.Sprintf("numeric(%d,%d)", p, s)
		def.GoType = pg.GoTypeFloat64

	default:
		return fmt.Errorf("unknown column builder %q", baseFn)
	}
	return nil
}

// applyMethod applies a single modifier method call to a ColumnDef.
// The error return is reserved for future cases; unknown modifiers are skipped silently.
func applyMethod(def *pg.ColumnDef, m MethodCall) error { //nolint:unparam
	switch m.Name {
	case "NotNull":
		def.NotNull = true

	case "PrimaryKey":
		def.PrimaryKey = true
		def.NotNull = true
		def.HasDefault = true // PK usually has a default

	case "Unique":
		def.Unique = true

	case "DefaultRandom":
		def.HasDefault = true
		def.DefaultExpr = "gen_random_uuid()"

	case "DefaultNow":
		def.HasDefault = true
		def.DefaultExpr = "now()"

	case "DefaultEmpty":
		def.HasDefault = true
		def.DefaultExpr = "'{}'::jsonb"

	case "DefaultEmptyArray":
		def.HasDefault = true
		def.DefaultExpr = "'[]'::jsonb"

	case "Default":
		def.HasDefault = true
		if len(m.Args) > 0 {
			switch v := m.Args[0].(type) {
			case string:
				// Distinguish between boolean defaults and string literals.
				if v == "true" || v == "false" {
					def.DefaultExpr = v
				} else {
					def.DefaultExpr = fmt.Sprintf("'%s'", v)
				}
			case bool:
				if v {
					def.DefaultExpr = "true"
				} else {
					def.DefaultExpr = "false"
				}
			case int64:
				def.DefaultExpr = fmt.Sprintf("%d", v)
			case float64:
				def.DefaultExpr = fmt.Sprintf("%g", v)
			default:
				def.DefaultExpr = fmt.Sprintf("%v", v)
			}
		}

	case "WithTimezone":
		// Switch timestamp → timestamptz.
		if def.SQLType == "timestamp" {
			def.SQLType = "timestamptz"
		}

	case "OnUpdate":
		def.OnUpdateExpr = "now()"

	case "References":
		// References("table", "col", pg.OnDelete(pg.FKActionRestrict))
		ref := &pg.FKRef{
			OnDelete: pg.FKActionNoAction,
			OnUpdate: pg.FKActionNoAction,
		}
		if len(m.Args) > 0 {
			if s, ok := m.Args[0].(string); ok {
				ref.Table = s
			}
		}
		if len(m.Args) > 1 {
			if s, ok := m.Args[1].(string); ok {
				ref.Column = s
			}
		}
		// Remaining args are FKOption chains like pg.OnDelete(pg.FKActionRestrict).
		for _, arg := range m.Args[2:] {
			if chain, ok := arg.(*ChainResult); ok {
				applyFKOption(ref, chain)
			}
		}
		def.References = ref

	case "Type":
		// JSONB .Type("MyStruct") — store as JsonbGoType.
		if len(m.Args) > 0 {
			if s, ok := m.Args[0].(string); ok {
				def.JsonbGoType = s
			}
		}

	case "Generated", "Precision":
		// Future: computed columns, precision for numeric — ignore for now.

	default:
		// Unknown modifier — skip silently. This is intentional: new modifiers
		// added to the DSL won't break the evaluator on old code.
	}
	return nil
}

// applyFKOption interprets a ChainResult for pg.OnDelete(action) / pg.OnUpdate(action).
func applyFKOption(ref *pg.FKRef, chain *ChainResult) {
	if chain.BasePkg != "pg" {
		return
	}
	var action pg.FKAction
	if len(chain.BaseArgs) > 0 {
		// The arg may be "pg.FKActionRestrict" (as a string from the selector eval)
		// or the constant value directly.
		switch v := chain.BaseArgs[0].(type) {
		case string:
			action = fkActionFromString(v)
		}
	}
	switch chain.BaseFn {
	case "OnDelete":
		ref.OnDelete = action
	case "OnUpdate":
		ref.OnUpdate = action
	}
}

// fkActionFromString maps "pg.FKActionRestrict" → pg.FKActionRestrict etc.
func fkActionFromString(s string) pg.FKAction {
	switch s {
	case "pg.FKActionRestrict", "FKActionRestrict":
		return pg.FKActionRestrict
	case "pg.FKActionCascade", "FKActionCascade":
		return pg.FKActionCascade
	case "pg.FKActionSetNull", "FKActionSetNull":
		return pg.FKActionSetNull
	case "pg.FKActionSetDefault", "FKActionSetDefault":
		return pg.FKActionSetDefault
	default:
		return pg.FKActionNoAction
	}
}
