package expr_test

import (
	"fmt"
	"testing"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	ts "github.com/sofired/grizzle/internal/testschema"
)

// helpers

func pgCtx() *expr.BuildContext     { return expr.NewBuildContext(dialect.Postgres) }
func myCtx() *expr.BuildContext     { return expr.NewBuildContext(dialect.MySQL) }
func sqliteCtx() *expr.BuildContext { return expr.NewBuildContext(dialect.SQLite) }

func sql(e expr.Expression) string { return e.ToSQL(pgCtx()) }

// -----------------------------------------------------------------------
// StringColumn regex operators
// -----------------------------------------------------------------------

func TestStringColumn_RegexpMatch(t *testing.T) {
	got := sql(ts.UsersT.Email.RegexpMatch("^alice"))
	want := `"users"."email" ~ $1`
	if got != want {
		t.Errorf("RegexpMatch: got %q, want %q", got, want)
	}
}

func TestStringColumn_RegexpMatchI(t *testing.T) {
	got := sql(ts.UsersT.Email.RegexpMatchI("^alice"))
	want := `"users"."email" ~* $1`
	if got != want {
		t.Errorf("RegexpMatchI: got %q, want %q", got, want)
	}
}

func TestStringColumn_NotRegexpMatch(t *testing.T) {
	got := sql(ts.UsersT.Email.NotRegexpMatch("^alice"))
	want := `"users"."email" !~ $1`
	if got != want {
		t.Errorf("NotRegexpMatch: got %q, want %q", got, want)
	}
}

func TestStringColumn_NotRegexpMatchI(t *testing.T) {
	got := sql(ts.UsersT.Email.NotRegexpMatchI("^alice"))
	want := `"users"."email" !~* $1`
	if got != want {
		t.Errorf("NotRegexpMatchI: got %q, want %q", got, want)
	}
}

// -----------------------------------------------------------------------
// StringColumn LIKE / NOT LIKE / NOT ILIKE
// -----------------------------------------------------------------------

func TestStringColumn_NotLike(t *testing.T) {
	got := sql(ts.UsersT.Username.NotLike("admin%"))
	want := `"users"."username" NOT LIKE $1`
	if got != want {
		t.Errorf("NotLike: got %q, want %q", got, want)
	}
}

func TestStringColumn_NotILike(t *testing.T) {
	got := sql(ts.UsersT.Username.NotILike("admin%"))
	want := `"users"."username" NOT ILIKE $1`
	if got != want {
		t.Errorf("NotILike: got %q, want %q", got, want)
	}
}

// -----------------------------------------------------------------------
// TsvectorColumn FTS operators
// -----------------------------------------------------------------------

func TestTsvectorColumn_Matches(t *testing.T) {
	got := sql(ts.ArticlesT.SearchVector.Matches("grizzle & orm"))
	want := `"articles"."search_vector" @@ to_tsquery($1)`
	if got != want {
		t.Errorf("Matches: got %q, want %q", got, want)
	}
}

func TestTsvectorColumn_MatchesWithConfig(t *testing.T) {
	got := sql(ts.ArticlesT.SearchVector.MatchesWithConfig("english", "grizzle & orm"))
	want := `"articles"."search_vector" @@ to_tsquery($1, $2)`
	if got != want {
		t.Errorf("MatchesWithConfig: got %q, want %q", got, want)
	}
}

func TestTsvectorColumn_MatchesPlain(t *testing.T) {
	got := sql(ts.ArticlesT.SearchVector.MatchesPlain("grizzle orm"))
	want := `"articles"."search_vector" @@ plainto_tsquery($1)`
	if got != want {
		t.Errorf("MatchesPlain: got %q, want %q", got, want)
	}
}

func TestTsvectorColumn_MatchesPlainWithConfig(t *testing.T) {
	got := sql(ts.ArticlesT.SearchVector.MatchesPlainWithConfig("english", "grizzle orm"))
	want := `"articles"."search_vector" @@ plainto_tsquery($1, $2)`
	if got != want {
		t.Errorf("MatchesPlainWithConfig: got %q, want %q", got, want)
	}
}

func TestTsvectorColumn_MatchesPhrase(t *testing.T) {
	got := sql(ts.ArticlesT.SearchVector.MatchesPhrase("fast full text"))
	want := `"articles"."search_vector" @@ phraseto_tsquery($1)`
	if got != want {
		t.Errorf("MatchesPhrase: got %q, want %q", got, want)
	}
}

func TestTsvectorColumn_MatchesPhraseWithConfig(t *testing.T) {
	got := sql(ts.ArticlesT.SearchVector.MatchesPhraseWithConfig("english", "fast full text"))
	want := `"articles"."search_vector" @@ phraseto_tsquery($1, $2)`
	if got != want {
		t.Errorf("MatchesPhraseWithConfig: got %q, want %q", got, want)
	}
}

func TestTsvectorColumn_MatchesWebSearch(t *testing.T) {
	got := sql(ts.ArticlesT.SearchVector.MatchesWebSearch("grizzle -orm"))
	want := `"articles"."search_vector" @@ websearch_to_tsquery($1)`
	if got != want {
		t.Errorf("MatchesWebSearch: got %q, want %q", got, want)
	}
}

func TestTsvectorColumn_MatchesWebSearchWithConfig(t *testing.T) {
	got := sql(ts.ArticlesT.SearchVector.MatchesWebSearchWithConfig("english", "grizzle -orm"))
	want := `"articles"."search_vector" @@ websearch_to_tsquery($1, $2)`
	if got != want {
		t.Errorf("MatchesWebSearchWithConfig: got %q, want %q", got, want)
	}
}

// -----------------------------------------------------------------------
// Standalone tsquery helpers
// -----------------------------------------------------------------------

func TestToTsquery(t *testing.T) {
	got := sql(expr.ToTsquery("grizzle & orm"))
	want := `to_tsquery($1)`
	if got != want {
		t.Errorf("ToTsquery: got %q, want %q", got, want)
	}
}

func TestToTsqueryWithConfig(t *testing.T) {
	got := sql(expr.ToTsqueryWithConfig("english", "grizzle & orm"))
	want := `to_tsquery($1, $2)`
	if got != want {
		t.Errorf("ToTsqueryWithConfig: got %q, want %q", got, want)
	}
}

func TestPlainToTsquery(t *testing.T) {
	got := sql(expr.PlainToTsquery("grizzle orm"))
	want := `plainto_tsquery($1)`
	if got != want {
		t.Errorf("PlainToTsquery: got %q, want %q", got, want)
	}
}

func TestPlainToTsqueryWithConfig(t *testing.T) {
	got := sql(expr.PlainToTsqueryWithConfig("english", "grizzle orm"))
	want := `plainto_tsquery($1, $2)`
	if got != want {
		t.Errorf("PlainToTsqueryWithConfig: got %q, want %q", got, want)
	}
}

func TestPhraseToTsquery(t *testing.T) {
	got := sql(expr.PhraseToTsquery("fast full text search"))
	want := `phraseto_tsquery($1)`
	if got != want {
		t.Errorf("PhraseToTsquery: got %q, want %q", got, want)
	}
}

func TestPhraseToTsqueryWithConfig(t *testing.T) {
	got := sql(expr.PhraseToTsqueryWithConfig("english", "fast full text search"))
	want := `phraseto_tsquery($1, $2)`
	if got != want {
		t.Errorf("PhraseToTsqueryWithConfig: got %q, want %q", got, want)
	}
}

func TestWebsearchToTsquery(t *testing.T) {
	got := sql(expr.WebsearchToTsquery("grizzle -orm"))
	want := `websearch_to_tsquery($1)`
	if got != want {
		t.Errorf("WebsearchToTsquery: got %q, want %q", got, want)
	}
}

func TestWebsearchToTsqueryWithConfig(t *testing.T) {
	got := sql(expr.WebsearchToTsqueryWithConfig("english", "grizzle -orm"))
	want := `websearch_to_tsquery($1, $2)`
	if got != want {
		t.Errorf("WebsearchToTsqueryWithConfig: got %q, want %q", got, want)
	}
}

// -----------------------------------------------------------------------
// ToTsvector helper
// -----------------------------------------------------------------------

func TestToTsvector_NoConfig(t *testing.T) {
	got := sql(expr.ToTsvector(ts.ArticlesT.Body).MatchesPlain("grizzle orm"))
	want := `to_tsvector("articles"."body") @@ plainto_tsquery($1)`
	if got != want {
		t.Errorf("ToTsvector(no config): got %q, want %q", got, want)
	}
}

func TestToTsvector_WithConfig(t *testing.T) {
	got := sql(expr.ToTsvector(ts.ArticlesT.Body, "english").MatchesPlain("grizzle orm"))
	want := `to_tsvector($1, "articles"."body") @@ plainto_tsquery($2)`
	if got != want {
		t.Errorf("ToTsvector(with config): got %q, want %q", got, want)
	}
}

func TestToTsvector_MatchesWithConfig(t *testing.T) {
	got := sql(expr.ToTsvector(ts.ArticlesT.Body, "english").MatchesWithConfig("english", "grizzle & orm"))
	want := `to_tsvector($1, "articles"."body") @@ to_tsquery($2, $3)`
	if got != want {
		t.Errorf("ToTsvector.MatchesWithConfig: got %q, want %q", got, want)
	}
}

// -----------------------------------------------------------------------
// TsRank / TsRankCd
// -----------------------------------------------------------------------

func TestTsRank(t *testing.T) {
	tsq := expr.PlainToTsquery("grizzle orm")
	rank := expr.TsRank(ts.ArticlesT.SearchVector, tsq)
	got := rank.ToSQL(pgCtx())
	want := `TS_RANK("articles"."search_vector", plainto_tsquery($1))`
	if got != want {
		t.Errorf("TsRank: got %q, want %q", got, want)
	}
}

func TestTsRankCd(t *testing.T) {
	tsq := expr.PlainToTsquery("grizzle orm")
	rank := expr.TsRankCd(ts.ArticlesT.SearchVector, tsq)
	got := rank.ToSQL(pgCtx())
	want := `TS_RANK_CD("articles"."search_vector", plainto_tsquery($1))`
	if got != want {
		t.Errorf("TsRankCd: got %q, want %q", got, want)
	}
}

func TestTsRank_Desc(t *testing.T) {
	tsq := expr.PlainToTsquery("grizzle orm")
	order := expr.TsRank(ts.ArticlesT.SearchVector, tsq).Desc()
	got := order.ToSQL(pgCtx())
	want := `TS_RANK("articles"."search_vector", plainto_tsquery($1)) DESC`
	if got != want {
		t.Errorf("TsRank.Desc: got %q, want %q", got, want)
	}
}

// -----------------------------------------------------------------------
// *WithConfig stability: empty config must still emit the 2-arg SQL form
// -----------------------------------------------------------------------

// Copilot review: *WithConfig variants previously fell back to the 1-arg form
// when config == "", which shifted placeholder numbering and contradicted the
// method contract. The hasConfig bool field now gates the SQL shape so that
// the 2-arg form is always emitted regardless of config's value.

func TestMatchesWithConfig_EmptyConfigStillEmits2ArgForm(t *testing.T) {
	got := sql(ts.ArticlesT.SearchVector.MatchesWithConfig("", "grizzle & orm"))
	want := `"articles"."search_vector" @@ to_tsquery($1, $2)`
	if got != want {
		t.Errorf("MatchesWithConfig(empty config): got %q, want %q", got, want)
	}
}

func TestToTsqueryWithConfig_EmptyConfigStillEmits2ArgForm(t *testing.T) {
	got := sql(expr.ToTsqueryWithConfig("", "grizzle & orm"))
	want := `to_tsquery($1, $2)`
	if got != want {
		t.Errorf("ToTsqueryWithConfig(empty config): got %q, want %q", got, want)
	}
}

func TestPlainToTsqueryWithConfig_EmptyConfigStillEmits2ArgForm(t *testing.T) {
	got := sql(expr.PlainToTsqueryWithConfig("", "grizzle orm"))
	want := `plainto_tsquery($1, $2)`
	if got != want {
		t.Errorf("PlainToTsqueryWithConfig(empty config): got %q, want %q", got, want)
	}
}

func TestPhraseToTsqueryWithConfig_EmptyConfigStillEmits2ArgForm(t *testing.T) {
	got := sql(expr.PhraseToTsqueryWithConfig("", "fast full text search"))
	want := `phraseto_tsquery($1, $2)`
	if got != want {
		t.Errorf("PhraseToTsqueryWithConfig(empty config): got %q, want %q", got, want)
	}
}

func TestWebsearchToTsqueryWithConfig_EmptyConfigStillEmits2ArgForm(t *testing.T) {
	got := sql(expr.WebsearchToTsqueryWithConfig("", "grizzle -orm"))
	want := `websearch_to_tsquery($1, $2)`
	if got != want {
		t.Errorf("WebsearchToTsqueryWithConfig(empty config): got %q, want %q", got, want)
	}
}

// -----------------------------------------------------------------------
// Argument accumulation: ensure params are bound in correct order
// -----------------------------------------------------------------------

func TestFTSArgOrder_TsvectorColumn(t *testing.T) {
	ctx := pgCtx()
	e := ts.ArticlesT.SearchVector.MatchesWithConfig("english", "grizzle & orm")
	_ = e.ToSQL(ctx)
	args := ctx.Args()
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != "english" {
		t.Errorf("arg[0]: got %v, want %q", args[0], "english")
	}
	if args[1] != "grizzle & orm" {
		t.Errorf("arg[1]: got %v, want %q", args[1], "grizzle & orm")
	}
}

func TestFTSArgOrder_ToTsvector(t *testing.T) {
	ctx := pgCtx()
	e := expr.ToTsvector(ts.ArticlesT.Body, "english").MatchesPlain("grizzle orm")
	_ = e.ToSQL(ctx)
	args := ctx.Args()
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[0] != "english" {
		t.Errorf("arg[0]: got %v, want %q", args[0], "english")
	}
	if args[1] != "grizzle orm" {
		t.Errorf("arg[1]: got %v, want %q", args[1], "grizzle orm")
	}
}

func TestFTSArgOrder_ToTsqueryWithConfig(t *testing.T) {
	ctx := pgCtx()
	e := expr.ToTsqueryWithConfig("english", "grizzle & orm")
	_ = e.ToSQL(ctx)
	args := ctx.Args()
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "english" {
		t.Errorf("arg[0]: got %v, want %q", args[0], "english")
	}
	if args[1] != "grizzle & orm" {
		t.Errorf("arg[1]: got %v, want %q", args[1], "grizzle & orm")
	}
}

func TestFTSArgOrder_PlainToTsqueryWithConfig(t *testing.T) {
	ctx := pgCtx()
	e := expr.PlainToTsqueryWithConfig("english", "grizzle orm")
	_ = e.ToSQL(ctx)
	args := ctx.Args()
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "english" {
		t.Errorf("arg[0]: got %v, want %q", args[0], "english")
	}
	if args[1] != "grizzle orm" {
		t.Errorf("arg[1]: got %v, want %q", args[1], "grizzle orm")
	}
}

func TestRegexpArgOrder(t *testing.T) {
	ctx := pgCtx()
	e := ts.UsersT.Email.RegexpMatch("^alice")
	_ = e.ToSQL(ctx)
	args := ctx.Args()
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}
	if args[0] != "^alice" {
		t.Errorf("arg[0]: got %v, want %q", args[0], "^alice")
	}
}

// -----------------------------------------------------------------------
// Example functions (appear in godoc)
// -----------------------------------------------------------------------

func ExampleStringColumn_RegexpMatch() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Email.RegexpMatch("^alice")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// "users"."email" ~ $1
}

func ExampleStringColumn_RegexpMatchI() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Email.RegexpMatchI("^alice")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// "users"."email" ~* $1
}

func ExampleStringColumn_NotRegexpMatch() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Email.NotRegexpMatch("^alice")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// "users"."email" !~ $1
}

func ExampleStringColumn_NotRegexpMatchI() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Email.NotRegexpMatchI("^alice")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// "users"."email" !~* $1
}

func ExampleTsvectorColumn_Matches() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.ArticlesT.SearchVector.Matches("grizzle & orm")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// "articles"."search_vector" @@ to_tsquery($1)
}

func ExampleTsvectorColumn_MatchesPlain() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.ArticlesT.SearchVector.MatchesPlain("grizzle orm")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// "articles"."search_vector" @@ plainto_tsquery($1)
}

func ExampleTsvectorColumn_MatchesWebSearch() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.ArticlesT.SearchVector.MatchesWebSearch("grizzle -orm")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// "articles"."search_vector" @@ websearch_to_tsquery($1)
}

func ExampleToTsvector() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.ToTsvector(ts.ArticlesT.Body, "english").MatchesPlain("grizzle orm")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// to_tsvector($1, "articles"."body") @@ plainto_tsquery($2)
}

func ExampleTsRank() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	tsq := expr.PlainToTsquery("grizzle orm")
	rank := expr.TsRank(ts.ArticlesT.SearchVector, tsq)
	fmt.Println(rank.Desc().ToSQL(ctx))
	// Output:
	// TS_RANK("articles"."search_vector", plainto_tsquery($1)) DESC
}

func ExampleStringColumn_NotLike() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Username.NotLike("admin%")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// "users"."username" NOT LIKE $1
}

func ExampleStringColumn_NotILike() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Username.NotILike("admin%")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// "users"."username" NOT ILIKE $1
}

func ExampleTsRankCd() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	tsq := expr.PlainToTsquery("grizzle orm")
	rank := expr.TsRankCd(ts.ArticlesT.SearchVector, tsq)
	fmt.Println(rank.ToSQL(ctx))
	// Output:
	// TS_RANK_CD("articles"."search_vector", plainto_tsquery($1))
}

func ExampleTsvectorColumn_MatchesWithConfig() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.ArticlesT.SearchVector.MatchesWithConfig("english", "grizzle & orm")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// "articles"."search_vector" @@ to_tsquery($1, $2)
}

func ExampleToTsquery() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.ToTsquery("grizzle & orm")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// to_tsquery($1)
}

func ExampleToTsqueryWithConfig() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.ToTsqueryWithConfig("english", "grizzle & orm")
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// to_tsquery($1, $2)
}

// -----------------------------------------------------------------------
// TsvectorExpr alias tests
// -----------------------------------------------------------------------

func TestTsvectorExpr_As(t *testing.T) {
	got := expr.ToTsvector(ts.ArticlesT.Body).As("tsv").ToSQL(pgCtx())
	want := `to_tsvector("articles"."body") AS "tsv"`
	if got != want {
		t.Errorf("TsvectorExpr.As: got %q, want %q", got, want)
	}
}

func TestTsvectorExpr_ColumnName(t *testing.T) {
	// Without alias: falls back to "to_tsvector".
	noAlias := expr.ToTsvector(ts.ArticlesT.Body)
	if got, want := noAlias.ColumnName(), "to_tsvector"; got != want {
		t.Errorf("ColumnName (no alias): got %q, want %q", got, want)
	}

	// With alias: returns the alias.
	withAlias := expr.ToTsvector(ts.ArticlesT.Body).As("tsv")
	if got, want := withAlias.ColumnName(), "tsv"; got != want {
		t.Errorf("ColumnName (with alias): got %q, want %q", got, want)
	}
}

// -----------------------------------------------------------------------
// Non-PostgreSQL dialect behaviour: unsupported operators emit safe fallbacks
// -----------------------------------------------------------------------
// Per issue #230: pg-only regex and FTS operators must not emit unconditionally.
// On non-PG dialects:
//   - Predicate expressions (regexpExpr, ftsMatchExpr, ftsMatchExprOnExpr) → "FALSE"
//   - Scalar expressions (TsvectorExpr, tsQueryFnExpr) → "NULL"
//
// No args are bound when the fallback is emitted, so ctx.Args() remains empty.

func TestRegexpMatch_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		e := ts.UsersT.Email.RegexpMatch("^alice")
		got := e.ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("RegexpMatch on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("RegexpMatch on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestRegexpMatchI_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := ts.UsersT.Email.RegexpMatchI("^alice").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("RegexpMatchI on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("RegexpMatchI on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestNotRegexpMatch_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := ts.UsersT.Email.NotRegexpMatch("^alice").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("NotRegexpMatch on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("NotRegexpMatch on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestNotRegexpMatchI_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := ts.UsersT.Email.NotRegexpMatchI("^alice").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("NotRegexpMatchI on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("NotRegexpMatchI on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestTsvectorColumn_Matches_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		e := ts.ArticlesT.SearchVector.Matches("grizzle & orm")
		got := e.ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("TsvectorColumn.Matches on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("TsvectorColumn.Matches on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestTsvectorColumn_MatchesPlain_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := ts.ArticlesT.SearchVector.MatchesPlain("grizzle orm").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("TsvectorColumn.MatchesPlain on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("TsvectorColumn.MatchesPlain on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestTsvectorColumn_MatchesWithConfig_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := ts.ArticlesT.SearchVector.MatchesWithConfig("english", "grizzle & orm").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("TsvectorColumn.MatchesWithConfig on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("TsvectorColumn.MatchesWithConfig on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestTsvectorColumn_MatchesPhrase_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := ts.ArticlesT.SearchVector.MatchesPhrase("fast full text").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("TsvectorColumn.MatchesPhrase on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("TsvectorColumn.MatchesPhrase on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestTsvectorColumn_MatchesWebSearch_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := ts.ArticlesT.SearchVector.MatchesWebSearch("grizzle -orm").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("TsvectorColumn.MatchesWebSearch on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("TsvectorColumn.MatchesWebSearch on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestToTsvector_NonPG_MatchesPlain_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := expr.ToTsvector(ts.ArticlesT.Body).MatchesPlain("grizzle orm").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("ToTsvector.MatchesPlain on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("ToTsvector.MatchesPlain on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestToTsvector_NonPG_Scalar_EmitsNULL(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := expr.ToTsvector(ts.ArticlesT.Body).ToSQL(ctx)
		if got != "NULL" {
			t.Errorf("ToTsvector scalar on %s: got %q, want \"NULL\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("ToTsvector scalar on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestToTsquery_NonPG_EmitsNULL(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := expr.ToTsquery("grizzle & orm").ToSQL(ctx)
		if got != "NULL" {
			t.Errorf("ToTsquery on %s: got %q, want \"NULL\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("ToTsquery on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestPlainToTsquery_NonPG_EmitsNULL(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := expr.PlainToTsquery("grizzle orm").ToSQL(ctx)
		if got != "NULL" {
			t.Errorf("PlainToTsquery on %s: got %q, want \"NULL\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("PlainToTsquery on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestPhraseToTsquery_NonPG_EmitsNULL(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := expr.PhraseToTsquery("fast full text").ToSQL(ctx)
		if got != "NULL" {
			t.Errorf("PhraseToTsquery on %s: got %q, want \"NULL\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("PhraseToTsquery on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestWebsearchToTsquery_NonPG_EmitsNULL(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := expr.WebsearchToTsquery("grizzle -orm").ToSQL(ctx)
		if got != "NULL" {
			t.Errorf("WebsearchToTsquery on %s: got %q, want \"NULL\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("WebsearchToTsquery on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestToTsqueryWithConfig_NonPG_EmitsNULL(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := expr.ToTsqueryWithConfig("english", "grizzle & orm").ToSQL(ctx)
		if got != "NULL" {
			t.Errorf("ToTsqueryWithConfig on %s: got %q, want \"NULL\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("ToTsqueryWithConfig on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestPlainToTsqueryWithConfig_NonPG_EmitsNULL(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := expr.PlainToTsqueryWithConfig("english", "grizzle orm").ToSQL(ctx)
		if got != "NULL" {
			t.Errorf("PlainToTsqueryWithConfig on %s: got %q, want \"NULL\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("PlainToTsqueryWithConfig on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestPhraseToTsqueryWithConfig_NonPG_EmitsNULL(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := expr.PhraseToTsqueryWithConfig("english", "fast full text").ToSQL(ctx)
		if got != "NULL" {
			t.Errorf("PhraseToTsqueryWithConfig on %s: got %q, want \"NULL\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("PhraseToTsqueryWithConfig on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestWebsearchToTsqueryWithConfig_NonPG_EmitsNULL(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := expr.WebsearchToTsqueryWithConfig("english", "grizzle -orm").ToSQL(ctx)
		if got != "NULL" {
			t.Errorf("WebsearchToTsqueryWithConfig on %s: got %q, want \"NULL\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("WebsearchToTsqueryWithConfig on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestTsvectorColumn_MatchesPlainWithConfig_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := ts.ArticlesT.SearchVector.MatchesPlainWithConfig("english", "grizzle orm").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("TsvectorColumn.MatchesPlainWithConfig on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("TsvectorColumn.MatchesPlainWithConfig on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestTsvectorColumn_MatchesPhraseWithConfig_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := ts.ArticlesT.SearchVector.MatchesPhraseWithConfig("english", "fast full text").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("TsvectorColumn.MatchesPhraseWithConfig on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("TsvectorColumn.MatchesPhraseWithConfig on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

func TestTsvectorColumn_MatchesWebSearchWithConfig_NonPG_EmitsFALSE(t *testing.T) {
	for _, name := range []string{"mysql", "sqlite"} {
		ctx := myCtx()
		if name == "sqlite" {
			ctx = sqliteCtx()
		}
		got := ts.ArticlesT.SearchVector.MatchesWebSearchWithConfig("english", "grizzle -orm").ToSQL(ctx)
		if got != "FALSE" {
			t.Errorf("TsvectorColumn.MatchesWebSearchWithConfig on %s: got %q, want \"FALSE\"", name, got)
		}
		if len(ctx.Args()) != 0 {
			t.Errorf("TsvectorColumn.MatchesWebSearchWithConfig on %s: expected no args bound, got %v", name, ctx.Args())
		}
	}
}

// TsRank and TsRankCd are built on the generic FuncExpr and do not have their
// own dialect gate. On non-PG dialects the tsquery argument emits NULL (since
// tsQueryFnExpr is gated), producing TS_RANK(col, NULL) — syntactically valid
// but semantically meaningless. Callers should check SupportsFullTextSearch()
// before using TsRank/TsRankCd with non-PG dialects.
func TestTsRank_NonPG_EmitsWithNullArg(t *testing.T) {
	cases := []struct {
		name string
		ctx  *expr.BuildContext
		col  string // expected quoted column reference
	}{
		{"mysql", myCtx(), "`articles`.`search_vector`"},
		{"sqlite", sqliteCtx(), `"articles"."search_vector"`},
	}
	for _, tc := range cases {
		tsq := expr.PlainToTsquery("grizzle orm")
		rank := expr.TsRank(ts.ArticlesT.SearchVector, tsq)
		got := rank.ToSQL(tc.ctx)
		// The tsquery arg emits NULL on non-PG; no args should be bound.
		want := "TS_RANK(" + tc.col + ", NULL)"
		if got != want {
			t.Errorf("TsRank on %s: got %q, want %q", tc.name, got, want)
		}
		if len(tc.ctx.Args()) != 0 {
			t.Errorf("TsRank on %s: expected no args bound, got %v", tc.name, tc.ctx.Args())
		}
	}
}

func TestTsRankCd_NonPG_EmitsWithNullArg(t *testing.T) {
	cases := []struct {
		name string
		ctx  *expr.BuildContext
		col  string
	}{
		{"mysql", myCtx(), "`articles`.`search_vector`"},
		{"sqlite", sqliteCtx(), `"articles"."search_vector"`},
	}
	for _, tc := range cases {
		tsq := expr.PlainToTsquery("grizzle orm")
		rank := expr.TsRankCd(ts.ArticlesT.SearchVector, tsq)
		got := rank.ToSQL(tc.ctx)
		want := "TS_RANK_CD(" + tc.col + ", NULL)"
		if got != want {
			t.Errorf("TsRankCd on %s: got %q, want %q", tc.name, got, want)
		}
		if len(tc.ctx.Args()) != 0 {
			t.Errorf("TsRankCd on %s: expected no args bound, got %v", tc.name, tc.ctx.Args())
		}
	}
}
