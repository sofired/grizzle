package codegen_test

import (
	"testing"

	"github.com/sofired/grizzle/gen/codegen"
	"github.com/sofired/grizzle/gen/parser"
)

func TestResolveColumn_DefaultEmpty_SetsHasDefault(t *testing.T) {
	col := parser.ParsedColumn{
		Name: "tags",
		Chain: &parser.ChainResult{
			BasePkg: "pg",
			BaseFn:  "JSONB",
			Methods: []parser.MethodCall{{Name: "DefaultEmpty"}},
		},
	}
	info, err := codegen.ResolveColumn(col)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.HasDefault {
		t.Error("HasDefault should be true for DefaultEmpty column")
	}
	if !info.IsOmitEmpty {
		t.Error("IsOmitEmpty should be true for DefaultEmpty column")
	}
}

func TestResolveColumn_DefaultEmptyArray_SetsHasDefault(t *testing.T) {
	col := parser.ParsedColumn{
		Name: "items",
		Chain: &parser.ChainResult{
			BasePkg: "pg",
			BaseFn:  "JSONB",
			Methods: []parser.MethodCall{{Name: "DefaultEmptyArray"}},
		},
	}
	info, err := codegen.ResolveColumn(col)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.HasDefault {
		t.Error("HasDefault should be true for DefaultEmptyArray column")
	}
	if !info.IsOmitEmpty {
		t.Error("IsOmitEmpty should be true for DefaultEmptyArray column")
	}
}

// --- Issue #236 regression: Numeric maps to string, not float64 ---

func TestResolveColumn_Numeric_MapsToString(t *testing.T) {
	col := parser.ParsedColumn{
		Name: "price",
		Chain: &parser.ChainResult{
			BasePkg: "pg",
			BaseFn:  "Numeric",
			Methods: []parser.MethodCall{{Name: "NotNull"}},
		},
	}
	info, err := codegen.ResolveColumn(col)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.GoType != "string" {
		t.Errorf("GoType = %q, want %q (numeric must not use float64 to avoid precision loss)", info.GoType, "string")
	}
	if info.GoTypePtr != "*string" {
		t.Errorf("GoTypePtr = %q, want %q", info.GoTypePtr, "*string")
	}
	if info.ColType != "expr.StringColumn" {
		t.Errorf("ColType = %q, want %q", info.ColType, "expr.StringColumn")
	}
}

func TestResolveColumn_Numeric_NullableIsPointerToString(t *testing.T) {
	col := parser.ParsedColumn{
		Name: "amount",
		Chain: &parser.ChainResult{
			BasePkg: "pg",
			BaseFn:  "Numeric",
			Methods: []parser.MethodCall{},
		},
	}
	info, err := codegen.ResolveColumn(col)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.IsNullable {
		t.Error("column without NotNull should be nullable")
	}
	if info.SelectGoType() != "*string" {
		t.Errorf("SelectGoType() = %q, want %q", info.SelectGoType(), "*string")
	}
}

// Verify Real and DoublePrecision still map to float64 (not affected by #236 split).
func TestResolveColumn_Real_StillFloat64(t *testing.T) {
	for _, baseFn := range []string{"Real", "DoublePrecision"} {
		t.Run(baseFn, func(t *testing.T) {
			col := parser.ParsedColumn{
				Name: "score",
				Chain: &parser.ChainResult{
					BasePkg: "pg",
					BaseFn:  baseFn,
					Methods: []parser.MethodCall{},
				},
			}
			info, err := codegen.ResolveColumn(col)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.GoType != "float64" {
				t.Errorf("%s GoType = %q, want float64", baseFn, info.GoType)
			}
		})
	}
}
