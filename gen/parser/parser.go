package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// ParsedColumn holds the raw parsed information extracted from a pg.C(...) call.
type ParsedColumn struct {
	Name  string
	Chain *ChainResult // e.g. BaseFn="Varchar", BaseArgs=[255], Methods=[{NotNull}, {Default, ["foo"]}]
}

// ParsedTable holds extracted information from a pg.Table(...) declaration.
type ParsedTable struct {
	VarName     string         // Go variable name, e.g. "Users"
	TableName   string         // SQL table name, e.g. "users"
	SchemaName  string         // SQL schema if pg.SchemaTable used
	Columns     []ParsedColumn
	// RawConstraintsNode is kept for future Kit/migration work but not used in codegen.
	HasConstraints bool
}

// ParseDir scans a directory for Go files and returns all pg.Table / pg.SchemaTable
// declarations found. It skips _test.go files and *_gen.go files.
func ParseDir(dir string) ([]*ParsedTable, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}

	var tables []*ParsedTable
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
			continue
		}
		path := filepath.Join(dir, name)
		fileTables, err := ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", path, err)
		}
		tables = append(tables, fileTables...)
	}
	return tables, nil
}

// ParseFile parses a single Go source file and extracts pg.Table/SchemaTable declarations.
func ParseFile(path string) ([]*ParsedTable, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse Go file: %w", err)
	}
	return extractTables(f)
}

// extractTables walks the AST and finds top-level var declarations of the form:
//
//	var X = pg.Table("name", pg.C(...), ...).WithConstraints(...)
//	var X = pg.SchemaTable("schema", "name", pg.C(...), ...)
func extractTables(f *ast.File) ([]*ParsedTable, error) {
	var tables []*ParsedTable

	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			varName := vs.Names[0].Name
			t, err := tryExtractTable(varName, vs.Values[0])
			if err != nil {
				return nil, fmt.Errorf("var %s: %w", varName, err)
			}
			if t != nil {
				tables = append(tables, t)
			}
		}
	}
	return tables, nil
}

// tryExtractTable attempts to parse an expression as a pg.Table or pg.SchemaTable call.
// Returns nil, nil if the expression is not a table declaration.
func tryExtractTable(varName string, expr ast.Expr) (*ParsedTable, error) {
	// The value may be a direct call: pg.Table(...) or pg.SchemaTable(...)
	// OR a method chain on it: pg.Table(...).WithConstraints(...)
	// We strip .WithConstraints() and similar suffixes to get the core call.
	core, hasConstraints := stripTableSuffix(expr)

	call, ok := core.(*ast.CallExpr)
	if !ok {
		return nil, nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, nil
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, nil
	}
	if pkg.Name != "pg" && pkg.Name != "mysql" {
		return nil, nil
	}

	var schemaName, tableName string
	var colArgs []ast.Expr

	switch sel.Sel.Name {
	case "Table":
		if len(call.Args) < 1 {
			return nil, fmt.Errorf("pg.Table: expected at least 1 arg")
		}
		name, err := evalStringArg(call.Args[0])
		if err != nil {
			return nil, fmt.Errorf("pg.Table name: %w", err)
		}
		tableName = name
		colArgs = call.Args[1:]

	case "SchemaTable":
		if len(call.Args) < 2 {
			return nil, fmt.Errorf("pg.SchemaTable: expected at least 2 args")
		}
		schema, err := evalStringArg(call.Args[0])
		if err != nil {
			return nil, fmt.Errorf("pg.SchemaTable schema: %w", err)
		}
		name, err := evalStringArg(call.Args[1])
		if err != nil {
			return nil, fmt.Errorf("pg.SchemaTable name: %w", err)
		}
		schemaName = schema
		tableName = name
		colArgs = call.Args[2:]

	default:
		return nil, nil
	}

	// Parse each pg.C("col_name", <chain>) argument.
	cols, err := extractColumns(colArgs)
	if err != nil {
		return nil, err
	}

	return &ParsedTable{
		VarName:        varName,
		TableName:      tableName,
		SchemaName:     schemaName,
		Columns:        cols,
		HasConstraints: hasConstraints,
	}, nil
}

// stripTableSuffix removes .WithConstraints(...) and .Build() suffixes from a
// table expression, returning the inner pg.Table(...) call.
func stripTableSuffix(expr ast.Expr) (inner ast.Expr, hasConstraints bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return expr, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return expr, false
	}
	switch sel.Sel.Name {
	case "WithConstraints":
		inner, _ = stripTableSuffix(sel.X)
		return inner, true
	case "Build":
		inner, hc := stripTableSuffix(sel.X)
		return inner, hc
	default:
		return expr, false
	}
}

// extractColumns parses a slice of pg.C("name", <chain>) AST arguments.
func extractColumns(args []ast.Expr) ([]ParsedColumn, error) {
	var cols []ParsedColumn
	for _, arg := range args {
		col, err := extractColumn(arg)
		if err != nil {
			return nil, err
		}
		if col != nil {
			cols = append(cols, *col)
		}
	}
	return cols, nil
}

// extractColumn parses: pg.C("col_name", <builder_chain>)
func extractColumn(arg ast.Expr) (*ParsedColumn, error) {
	call, ok := arg.(*ast.CallExpr)
	if !ok {
		return nil, nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, nil
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || (pkg.Name != "pg" && pkg.Name != "mysql") || sel.Sel.Name != "C" {
		return nil, nil
	}
	if len(call.Args) != 2 {
		return nil, fmt.Errorf("%s.C: expected 2 args, got %d", pkg.Name, len(call.Args))
	}
	colName, err := evalStringArg(call.Args[0])
	if err != nil {
		return nil, fmt.Errorf("pg.C name: %w", err)
	}
	chain, err := UnwrapChain(call.Args[1])
	if err != nil {
		return nil, fmt.Errorf("column %q chain: %w", colName, err)
	}
	return &ParsedColumn{Name: colName, Chain: chain}, nil
}

func evalStringArg(e ast.Expr) (string, error) {
	v, err := evalLiteral(e)
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %T", v)
	}
	return s, nil
}
