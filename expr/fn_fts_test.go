package expr_test

// Tests for full-text search functions — Fix #89.
// Verifies that:
//   - Single-argument forms bind the query string as the sole bound parameter.
//   - WithConfig forms bind config first, query second (matching SQL argument order).

import (
	"testing"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

func TestToTsquery_ArgOrder(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.ToTsquery("fat & rat")
	sql, _ := e.RenderSQL(ctx)
	args := ctx.Args()

	if sql != "to_tsquery($1)" {
		t.Errorf("ToTsquery: got SQL %q, want %q", sql, "to_tsquery($1)")
	}
	if len(args) != 1 || args[0] != "fat & rat" {
		t.Errorf("ToTsquery: got args %v, want [\"fat & rat\"]", args)
	}
}

func TestToTsqueryWithConfig_ArgOrder(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.ToTsqueryWithConfig("english", "fat & rat")
	sql, _ := e.RenderSQL(ctx)
	args := ctx.Args()

	if sql != "to_tsquery($1, $2)" {
		t.Errorf("ToTsqueryWithConfig: got SQL %q, want %q", sql, "to_tsquery($1, $2)")
	}
	if len(args) != 2 {
		t.Fatalf("ToTsqueryWithConfig: got %d args, want 2", len(args))
	}
	if args[0] != "english" {
		t.Errorf("ToTsqueryWithConfig: args[0] = %v, want \"english\" (config must come first)", args[0])
	}
	if args[1] != "fat & rat" {
		t.Errorf("ToTsqueryWithConfig: args[1] = %v, want \"fat & rat\"", args[1])
	}
}

func TestPlainToTsquery_ArgOrder(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.PlainToTsquery("fat rat")
	sql, _ := e.RenderSQL(ctx)
	args := ctx.Args()

	if sql != "plainto_tsquery($1)" {
		t.Errorf("PlainToTsquery: got SQL %q, want %q", sql, "plainto_tsquery($1)")
	}
	if len(args) != 1 || args[0] != "fat rat" {
		t.Errorf("PlainToTsquery: got args %v, want [\"fat rat\"]", args)
	}
}

func TestPlainToTsqueryWithConfig_ArgOrder(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.PlainToTsqueryWithConfig("english", "fat rat")
	sql, _ := e.RenderSQL(ctx)
	args := ctx.Args()

	if sql != "plainto_tsquery($1, $2)" {
		t.Errorf("PlainToTsqueryWithConfig: got SQL %q, want %q", sql, "plainto_tsquery($1, $2)")
	}
	if len(args) != 2 {
		t.Fatalf("PlainToTsqueryWithConfig: got %d args, want 2", len(args))
	}
	if args[0] != "english" {
		t.Errorf("PlainToTsqueryWithConfig: args[0] = %v, want \"english\" (config must come first)", args[0])
	}
	if args[1] != "fat rat" {
		t.Errorf("PlainToTsqueryWithConfig: args[1] = %v, want \"fat rat\"", args[1])
	}
}

func TestPhraseToTsquery_ArgOrder(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.PhraseToTsquery("fat cat")
	sql, _ := e.RenderSQL(ctx)
	args := ctx.Args()

	if sql != "phraseto_tsquery($1)" {
		t.Errorf("PhraseToTsquery: got SQL %q, want %q", sql, "phraseto_tsquery($1)")
	}
	if len(args) != 1 || args[0] != "fat cat" {
		t.Errorf("PhraseToTsquery: got args %v, want [\"fat cat\"]", args)
	}
}

func TestPhraseToTsqueryWithConfig_ArgOrder(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.PhraseToTsqueryWithConfig("english", "fat cat")
	sql, _ := e.RenderSQL(ctx)
	args := ctx.Args()

	if sql != "phraseto_tsquery($1, $2)" {
		t.Errorf("PhraseToTsqueryWithConfig: got SQL %q, want %q", sql, "phraseto_tsquery($1, $2)")
	}
	if len(args) != 2 {
		t.Fatalf("PhraseToTsqueryWithConfig: got %d args, want 2", len(args))
	}
	if args[0] != "english" {
		t.Errorf("PhraseToTsqueryWithConfig: args[0] = %v, want \"english\" (config must come first)", args[0])
	}
	if args[1] != "fat cat" {
		t.Errorf("PhraseToTsqueryWithConfig: args[1] = %v, want \"fat cat\"", args[1])
	}
}

func TestWebsearchToTsquery_ArgOrder(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.WebsearchToTsquery("fat cat")
	sql, _ := e.RenderSQL(ctx)
	args := ctx.Args()

	if sql != "websearch_to_tsquery($1)" {
		t.Errorf("WebsearchToTsquery: got SQL %q, want %q", sql, "websearch_to_tsquery($1)")
	}
	if len(args) != 1 || args[0] != "fat cat" {
		t.Errorf("WebsearchToTsquery: got args %v, want [\"fat cat\"]", args)
	}
}

func TestWebsearchToTsqueryWithConfig_ArgOrder(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.WebsearchToTsqueryWithConfig("english", "fat cat")
	sql, _ := e.RenderSQL(ctx)
	args := ctx.Args()

	if sql != "websearch_to_tsquery($1, $2)" {
		t.Errorf("WebsearchToTsqueryWithConfig: got SQL %q, want %q", sql, "websearch_to_tsquery($1, $2)")
	}
	if len(args) != 2 {
		t.Fatalf("WebsearchToTsqueryWithConfig: got %d args, want 2", len(args))
	}
	if args[0] != "english" {
		t.Errorf("WebsearchToTsqueryWithConfig: args[0] = %v, want \"english\" (config must come first)", args[0])
	}
	if args[1] != "fat cat" {
		t.Errorf("WebsearchToTsqueryWithConfig: args[1] = %v, want \"fat cat\"", args[1])
	}
}
