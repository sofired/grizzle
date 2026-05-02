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
