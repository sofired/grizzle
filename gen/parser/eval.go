package parser

import (
	"fmt"
	"math"
	"strings"

	"github.com/sofired/grizzle/schema/mysql"
	pg "github.com/sofired/grizzle/schema/pg"
	"github.com/sofired/grizzle/schema/sqlite"
)

// EvalTable converts a ParsedTable (from the AST parser) into a pg.TableDefiner
// by evaluating each column's builder chain. The concrete type returned
// depends on the dialect recorded in the ParsedTable:
//
//   - "pg"     → *pg.TableDef     (implements pg.TableDefiner, Dialect() == "postgres")
//   - "mysql"  → *mysql.TableDef  (implements pg.TableDefiner, Dialect() == "mysql")
//   - "sqlite" → *sqlite.TableDef (implements pg.TableDefiner, Dialect() == "sqlite")
//
// This ensures that parseSchemaDir in the CLI returns the correct dialect type
// for each parsed table, resolving the cross-dialect type leak (issue #156).
//
// Note: Constraint expressions that reference column values at runtime (like
// partial index WHERE clauses defined via string literals) are preserved as-is.
// The WithConstraints callback is not re-executed here; constraints parsed from
// pg.UniqueIndex(...).On(...).Where(...).Build() calls are reconstructed structurally.
func EvalTable(pt *ParsedTable) (pg.TableDefiner, error) {
	inner := &pg.TableDef{
		Name:   pt.TableName,
		Schema: pt.SchemaName,
	}

	for _, pc := range pt.Columns {
		colDef, err := evalColumn(pc)
		if err != nil {
			return nil, fmt.Errorf("column %q: %w", pc.Name, err)
		}
		inner.Columns = append(inner.Columns, colDef)
	}

	switch pt.Dialect {
	case "mysql":
		return &mysql.TableDef{TableDef: inner}, nil
	case "sqlite":
		return &sqlite.TableDef{TableDef: inner}, nil
	default:
		// "pg" or any unrecognised dialect falls back to the PostgreSQL type.
		return inner, nil
	}
}

// evalColumn evaluates a ParsedColumn's chain into a pg.ColumnDef.
func evalColumn(pc ParsedColumn) (pg.ColumnDef, error) {
	chain := pc.Chain
	def := pg.ColumnDef{Name: pc.Name}

	// Apply base type.
	if err := applyBaseType(&def, chain.BasePkg, chain.BaseFn, chain.BaseArgs); err != nil {
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
func applyBaseType(def *pg.ColumnDef, basePkg, baseFn string, args []any) error {
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

	// SQLite-specific: REAL storage class (64-bit IEEE 754 float).
	case "Real":
		def.SQLType = "real"
		def.GoType = pg.GoTypeFloat64

	// SQLite-specific: BLOB storage class (raw binary data).
	case "Blob":
		def.SQLType = "blob"
		def.GoType = pg.GoTypeByteSlice

	case "JSON":
		def.SQLType = "json"
		def.GoType = pg.GoTypeAny

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

	// MySQL-specific: MEDIUMINT (3-byte signed integer).
	case "MediumInt":
		def.SQLType = "mediumint"
		def.GoType = pg.GoTypeInt

	// MySQL-specific: YEAR (stores 1901–2155 as an integer).
	case "Year":
		def.SQLType = "year"
		def.GoType = pg.GoTypeInt

	// Enum: pg.Enum(typeName, val1, val2, ...) vs MySQL inline ENUM('v1','v2',...).
	case "Enum":
		if basePkg == "pg" {
			if len(args) < 2 {
				return fmt.Errorf("pg.Enum requires a type name and at least one value")
			}
			typeName, ok := args[0].(string)
			if !ok {
				return fmt.Errorf("pg.Enum: first argument (type name) must be a string, got %T", args[0])
			}
			for i, a := range args[1:] {
				s, ok := a.(string)
				if !ok {
					return fmt.Errorf("pg.Enum: argument %d must be a string, got %T", i+1, a)
				}
				if s == "" {
					return fmt.Errorf("pg.Enum: argument %d must not be empty", i+1)
				}
			}
			def.SQLType = typeName
			def.GoType = pg.GoTypeString
		} else {
			// MySQL-specific: ENUM('v1','v2',...) — inline enumeration.
			if len(args) == 0 {
				return fmt.Errorf("enum requires at least one value")
			}
			parts := make([]string, 0, len(args))
			for i, a := range args {
				s, ok := a.(string)
				if !ok {
					return fmt.Errorf("enum: argument %d must be a string, got %T", i, a)
				}
				parts = append(parts, "'"+strings.ReplaceAll(s, "'", "''")+"'")
			}
			def.SQLType = "enum(" + strings.Join(parts, ",") + ")"
			def.GoType = pg.GoTypeString
		}

	// MySQL-specific: SET('v1','v2',...) — multi-value set column.
	case "Set":
		if len(args) == 0 {
			return fmt.Errorf("set requires at least one value")
		}
		parts := make([]string, 0, len(args))
		for i, a := range args {
			s, ok := a.(string)
			if !ok {
				return fmt.Errorf("set: argument %d must be a string, got %T", i, a)
			}
			parts = append(parts, "'"+strings.ReplaceAll(s, "'", "''")+"'")
		}
		def.SQLType = "set(" + strings.Join(parts, ",") + ")"
		def.GoType = pg.GoTypeString

	// PostgreSQL-specific builders added in this PR.

	case "Date":
		def.SQLType = "date"
		def.GoType = pg.GoTypeTime

	case "Time":
		def.SQLType = "time"
		def.GoType = pg.GoTypeTime

	case "Interval":
		def.SQLType = "interval"
		def.GoType = pg.GoTypeString

	case "DoublePrecision":
		def.SQLType = "double precision"
		def.GoType = pg.GoTypeFloat64

	case "Char":
		n := int64(1)
		if len(args) > 0 {
			if v, ok := args[0].(int64); ok {
				n = v
			}
		}
		def.SQLType = fmt.Sprintf("char(%d)", n)
		def.GoType = pg.GoTypeString

	case "Bytea":
		def.SQLType = "bytea"
		def.GoType = pg.GoTypeByteSlice

	case "Inet":
		def.SQLType = "inet"
		def.GoType = pg.GoTypeString

	case "Cidr":
		def.SQLType = "cidr"
		def.GoType = pg.GoTypeString

	case "Macaddr":
		def.SQLType = "macaddr"
		def.GoType = pg.GoTypeString

	case "Tsvector":
		def.SQLType = "tsvector"
		def.GoType = pg.GoTypeString

	case "Tsquery":
		def.SQLType = "tsquery"
		def.GoType = pg.GoTypeString

	case "Int4Range":
		def.SQLType = "int4range"
		def.GoType = pg.GoTypeString

	case "Int8Range":
		def.SQLType = "int8range"
		def.GoType = pg.GoTypeString

	case "NumRange":
		def.SQLType = "numrange"
		def.GoType = pg.GoTypeString

	case "TsRange":
		def.SQLType = "tsrange"
		def.GoType = pg.GoTypeString

	case "TstzRange":
		def.SQLType = "tstzrange"
		def.GoType = pg.GoTypeString

	case "DateRange":
		def.SQLType = "daterange"
		def.GoType = pg.GoTypeString

	case "Array":
		if len(args) == 0 {
			return fmt.Errorf("pg.Array requires an inner column builder argument")
		}
		innerChain, ok := args[0].(*ChainResult)
		if !ok {
			return fmt.Errorf("pg.Array: argument must be a column builder chain, got %T", args[0])
		}
		var innerDef pg.ColumnDef
		if err := applyBaseType(&innerDef, innerChain.BasePkg, innerChain.BaseFn, innerChain.BaseArgs); err != nil {
			return fmt.Errorf("pg.Array inner type: %w", err)
		}
		for _, m := range innerChain.Methods {
			if err := applyMethod(&innerDef, m); err != nil {
				return fmt.Errorf("pg.Array inner method .%s: %w", m.Name, err)
			}
		}
		def.SQLType = innerDef.SQLType + "[]"
		def.GoType = pg.GoTypeAny

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
		if strings.HasSuffix(def.SQLType, "[]") {
			def.DefaultExpr = "ARRAY[]::" + def.SQLType
		} else {
			def.DefaultExpr = "'{}'::" + def.SQLType
		}

	case "DefaultEmptyArray":
		def.HasDefault = true
		def.DefaultExpr = "'[]'::" + def.SQLType

	case "Default":
		def.HasDefault = true
		if len(m.Args) > 0 {
			switch v := m.Args[0].(type) {
			case string:
				quoted := "'" + strings.ReplaceAll(v, "'", "''") + "'"
				switch {
				case def.SQLType == "interval":
					def.DefaultExpr = quoted + "::interval"
				case def.SQLType == "jsonb" || def.SQLType == "json":
					def.DefaultExpr = quoted + "::" + def.SQLType
				case strings.HasSuffix(def.SQLType, "[]"):
					def.DefaultExpr = quoted + "::" + def.SQLType
				case isCustomSQLType(def.SQLType):
					def.DefaultExpr = quoted + "::" + def.SQLType
				default:
					def.DefaultExpr = quoted
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
				switch {
				case math.IsNaN(v):
					def.DefaultExpr = fmt.Sprintf("'NaN'::%s", def.SQLType)
				case math.IsInf(v, 1):
					def.DefaultExpr = fmt.Sprintf("'Infinity'::%s", def.SQLType)
				case math.IsInf(v, -1):
					def.DefaultExpr = fmt.Sprintf("'-Infinity'::%s", def.SQLType)
				default:
					def.DefaultExpr = fmt.Sprintf("%g", v)
				}
			default:
				def.DefaultExpr = fmt.Sprintf("%v", v)
			}
		}

	case "WithTimezone":
		switch def.SQLType {
		case "timestamp":
			def.SQLType = "timestamptz"
		case "time":
			def.SQLType = "timetz"
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

// knownBuiltinSQLTypes is the complete set of parameterless built-in SQL types
// produced by this package's column builders. Any SQLType not in this set and
// containing no '(' or space is treated as a user-defined type (e.g. a pg.Enum
// type name) and receives an explicit ::cast suffix in DEFAULT expressions.
var knownBuiltinSQLTypes = map[string]bool{
	"uuid": true, "text": true, "boolean": true, "integer": true,
	"bigint": true, "serial": true, "bigserial": true, "smallint": true,
	"tinyint": true, "real": true, "blob": true, "json": true,
	"timestamp": true, "timestamptz": true, "jsonb": true, "mediumint": true,
	"year": true, "date": true, "time": true, "timetz": true,
	"interval": true, "bytea": true, "inet": true, "cidr": true,
	"macaddr": true, "tsvector": true, "tsquery": true, "int4range": true,
	"int8range": true, "numrange": true, "tsrange": true, "tstzrange": true,
	"daterange": true, "double precision": true,
}

// isCustomSQLType reports whether sqlType is a user-defined type name (e.g. a
// pg.Enum type) that needs an explicit ::cast suffix in DEFAULT expressions.
// Parameterized types (containing '(') and multi-word types (containing space)
// are never custom.
func isCustomSQLType(sqlType string) bool {
	if strings.ContainsAny(sqlType, "( ") {
		return false
	}
	return !knownBuiltinSQLTypes[sqlType]
}

// isKnownDialectPkg reports whether pkg is one of the three recognised schema
// package identifiers ("pg", "mysql", "sqlite"). It is used as a guard before
// interpreting FK option calls (OnDelete, OnUpdate), so that unrecognised
// package aliases are silently skipped rather than misinterpreted. The three
// packages are equivalent for FK options because mysql.OnDelete and
// sqlite.OnDelete are function aliases for pg.OnDelete.
func isKnownDialectPkg(pkg string) bool {
	return pkg == "pg" || pkg == "mysql" || pkg == "sqlite"
}

// applyFKOption interprets a ChainResult for OnDelete(action) / OnUpdate(action)
// from any of the three built-in schema packages (pg, mysql, sqlite).
func applyFKOption(ref *pg.FKRef, chain *ChainResult) {
	if !isKnownDialectPkg(chain.BasePkg) {
		return
	}
	var action pg.FKAction
	if len(chain.BaseArgs) > 0 {
		// The arg may be "pg.FKActionRestrict" / "mysql.FKActionCascade" (as a
		// string from the selector eval) or the unqualified constant name.
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

// fkActionFromString maps a dialect-qualified or bare FKAction constant name to
// its pg.FKAction value. Any dialect prefix (pg., mysql., sqlite.) is accepted.
func fkActionFromString(s string) pg.FKAction {
	// Strip any "pkg." prefix so that "mysql.FKActionCascade" and
	// "pg.FKActionCascade" and plain "FKActionCascade" all resolve the same way.
	if dot := strings.LastIndex(s, "."); dot >= 0 {
		s = s[dot+1:]
	}
	switch s {
	case "FKActionRestrict":
		return pg.FKActionRestrict
	case "FKActionCascade":
		return pg.FKActionCascade
	case "FKActionSetNull":
		return pg.FKActionSetNull
	case "FKActionSetDefault":
		return pg.FKActionSetDefault
	default:
		return pg.FKActionNoAction
	}
}
