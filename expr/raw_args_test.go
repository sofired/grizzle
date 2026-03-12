package expr_test

import (
	"testing"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	ts "github.com/sofired/grizzle/internal/testschema"
)

func TestRawArgs_noPlaceholders(t *testing.T) {
	e := expr.RawArgs("now()")
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := e.ToSQL(ctx)
	if got != "now()" {
		t.Errorf("got %q, want %q", got, "now()")
	}
	if len(ctx.Args()) != 0 {
		t.Errorf("expected no args, got %v", ctx.Args())
	}
}

func TestRawArgs_singleParam_postgres(t *testing.T) {
	e := expr.RawArgs("tsv @@ websearch_to_tsquery($?)", "hello world")
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := e.ToSQL(ctx)
	want := "tsv @@ websearch_to_tsquery($1)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(ctx.Args()) != 1 || ctx.Args()[0] != "hello world" {
		t.Errorf("unexpected args: %v", ctx.Args())
	}
}

func TestRawArgs_multipleParams_postgres(t *testing.T) {
	e := expr.RawArgs("col BETWEEN $? AND $?", 10, 20)
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := e.ToSQL(ctx)
	want := "col BETWEEN $1 AND $2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	args := ctx.Args()
	if len(args) != 2 || args[0] != 10 || args[1] != 20 {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestRawArgs_renumbersAfterExistingParams(t *testing.T) {
	// Simulate a context that already has one bound parameter (from another expression).
	ctx := expr.NewBuildContext(dialect.Postgres)
	existing := ts.UsersT.Enabled.IsTrue()
	existing.ToSQL(ctx) // binds $1 = true

	e := expr.RawArgs("tsv @@ websearch_to_tsquery($?)", "grizzle ORM")
	got := e.ToSQL(ctx)
	want := "tsv @@ websearch_to_tsquery($2)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(ctx.Args()) != 2 {
		t.Errorf("expected 2 total args, got %v", ctx.Args())
	}
}

func TestRawArgs_mysql_questionMarkPlaceholder(t *testing.T) {
	e := expr.RawArgs("ST_Distance($?, $?) < $?", 37.7749, -122.4194, 10.0)
	ctx := expr.NewBuildContext(dialect.MySQL)
	got := e.ToSQL(ctx)
	want := "ST_Distance(?, ?) < ?"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(ctx.Args()) != 3 {
		t.Errorf("expected 3 args, got %v", ctx.Args())
	}
}

func TestRawArgs_sqlite_questionMarkPlaceholder(t *testing.T) {
	e := expr.RawArgs("json_extract(data, $?) IS NOT NULL", "$.name")
	ctx := expr.NewBuildContext(dialect.SQLite)
	got := e.ToSQL(ctx)
	want := "json_extract(data, ?) IS NOT NULL"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRawArgs_composedWithAnd(t *testing.T) {
	cond := expr.And(
		ts.UsersT.Enabled.IsTrue(),
		expr.RawArgs("tsv @@ websearch_to_tsquery($?)", "grizzle ORM"),
	)
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := cond.ToSQL(ctx)
	want := `("users"."enabled" = $1 AND tsv @@ websearch_to_tsquery($2))`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(ctx.Args()) != 2 {
		t.Errorf("expected 2 args, got %v", ctx.Args())
	}
}

func TestRawArgs_composedWithOr(t *testing.T) {
	cond := expr.Or(
		ts.UsersT.Email.EQ("admin@example.com"),
		expr.RawArgs("role = $?", "superuser"),
	)
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := cond.ToSQL(ctx)
	want := `("users"."email" = $1 OR role = $2)`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRawArgs_morePlaceholdersThanArgs(t *testing.T) {
	// When there are more $? tokens than args, the extra tokens are emitted
	// literally so the mismatch is visible in the SQL.
	e := expr.RawArgs("a = $? AND b = $?", 1) // only one arg for two placeholders
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := e.ToSQL(ctx)
	want := "a = $1 AND b = $?"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if len(ctx.Args()) != 1 {
		t.Errorf("expected 1 arg, got %v", ctx.Args())
	}
}

func TestRawArgs_noArgs(t *testing.T) {
	// RawArgs with zero args and no placeholders behaves like Raw.
	e := expr.RawArgs("current_timestamp")
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := e.ToSQL(ctx)
	if got != "current_timestamp" {
		t.Errorf("got %q, want %q", got, "current_timestamp")
	}
}

func TestRawArgs_moreArgsThanPlaceholders(t *testing.T) {
	// When there are more args than $? tokens, the extra args are not bound.
	// The generated SQL only contains references to the args that matched a
	// placeholder — unused trailing args are silently ignored.
	e := expr.RawArgs("a = $?", 1, 2, 3) // three args but only one placeholder
	ctx := expr.NewBuildContext(dialect.Postgres)
	got := e.ToSQL(ctx)
	want := "a = $1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// Only one arg is actually bound; the other two are not added to ctx.
	if len(ctx.Args()) != 1 {
		t.Errorf("expected 1 arg, got %v", ctx.Args())
	}
}

func TestRawArgs_implementsExpression(t *testing.T) {
	// Compile-time check: RawArgs must satisfy expr.Expression.
	var _ expr.Expression = expr.RawArgs("$?", 1)
}
