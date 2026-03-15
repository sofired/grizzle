package expr_test

import (
	"fmt"
	"testing"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	ts "github.com/sofired/grizzle/internal/testschema"
)

// helpers

func pgCtx() *expr.BuildContext { return expr.NewBuildContext(dialect.Postgres) }

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
	got := sql(ts.ArticlesT.SearchVector.MatchesPhraseWithConfig("simple", "fast full text"))
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
// ToTsvector helper function
// -----------------------------------------------------------------------

func TestToTsvector_NoConfig(t *testing.T) {
	e := expr.ToTsvector(ts.ArticlesT.Body)
	got := e.ToSQL(pgCtx())
	want := `to_tsvector("articles"."body")`
	if got != want {
		t.Errorf("ToTsvector (no config): got %q, want %q", got, want)
	}
}

func TestToTsvector_WithConfig(t *testing.T) {
	e := expr.ToTsvector(ts.ArticlesT.Body, "english")
	got := e.ToSQL(pgCtx())
	want := `to_tsvector($1, "articles"."body")`
	if got != want {
		t.Errorf("ToTsvector (with config): got %q, want %q", got, want)
	}
}

func TestToTsvector_Matches(t *testing.T) {
	e := expr.ToTsvector(ts.ArticlesT.Body).Matches("grizzle & orm")
	got := e.ToSQL(pgCtx())
	want := `to_tsvector("articles"."body") @@ to_tsquery($1)`
	if got != want {
		t.Errorf("ToTsvector.Matches: got %q, want %q", got, want)
	}
}

func TestToTsvector_MatchesPlain(t *testing.T) {
	e := expr.ToTsvector(ts.ArticlesT.Body, "english").MatchesPlain("grizzle orm")
	got := e.ToSQL(pgCtx())
	want := `to_tsvector($1, "articles"."body") @@ plainto_tsquery($2)`
	if got != want {
		t.Errorf("ToTsvector(config).MatchesPlain: got %q, want %q", got, want)
	}
}

func TestToTsvector_MatchesWebSearch(t *testing.T) {
	e := expr.ToTsvector(ts.ArticlesT.Title).MatchesWebSearch("grizzle -orm")
	got := e.ToSQL(pgCtx())
	want := `to_tsvector("articles"."title") @@ websearch_to_tsquery($1)`
	if got != want {
		t.Errorf("ToTsvector.MatchesWebSearch: got %q, want %q", got, want)
	}
}

func TestToTsvector_MatchesPhrase(t *testing.T) {
	e := expr.ToTsvector(ts.ArticlesT.Body).MatchesPhrase("fast full text")
	got := e.ToSQL(pgCtx())
	want := `to_tsvector("articles"."body") @@ phraseto_tsquery($1)`
	if got != want {
		t.Errorf("ToTsvector.MatchesPhrase: got %q, want %q", got, want)
	}
}

func TestToTsvector_AsAlias(t *testing.T) {
	// ToTsvector satisfies SelectableColumn so it can appear in SELECT lists.
	e := expr.ToTsvector(ts.ArticlesT.Body, "english").As("body_tsv")
	got := e.ToSQL(pgCtx())
	want := `to_tsvector($1, "articles"."body") AS "body_tsv"`
	if got != want {
		t.Errorf("ToTsvector.As: got %q, want %q", got, want)
	}
	if e.ColumnName() != "body_tsv" {
		t.Errorf("ColumnName: got %q, want %q", e.ColumnName(), "body_tsv")
	}
}

// -----------------------------------------------------------------------
// Standalone tsquery function helpers
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

func TestToTsqueryWithConfig_ArgOrder(t *testing.T) {
	ctx := pgCtx()
	_ = expr.ToTsqueryWithConfig("english", "grizzle & orm").ToSQL(ctx)
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
// TsQueryExpr — SELECT-list / alias support
// -----------------------------------------------------------------------

func TestToTsquery_AsAlias(t *testing.T) {
	// TsQueryExpr satisfies SelectableColumn so it can appear in SELECT lists.
	e := expr.ToTsquery("grizzle & orm").As("q")
	got := e.ToSQL(pgCtx())
	want := `to_tsquery($1) AS "q"`
	if got != want {
		t.Errorf("ToTsquery.As: got %q, want %q", got, want)
	}
	if e.ColumnName() != "q" {
		t.Errorf("ColumnName: got %q, want %q", e.ColumnName(), "q")
	}
	if e.TableName() != "" {
		t.Errorf("TableName: got %q, want %q", e.TableName(), "")
	}
}

func TestToTsqueryWithConfig_AsAlias(t *testing.T) {
	e := expr.ToTsqueryWithConfig("english", "grizzle & orm").As("q")
	got := e.ToSQL(pgCtx())
	want := `to_tsquery($1, $2) AS "q"`
	if got != want {
		t.Errorf("ToTsqueryWithConfig.As: got %q, want %q", got, want)
	}
}

func TestPlainToTsquery_AsAlias(t *testing.T) {
	e := expr.PlainToTsquery("grizzle orm").As("q")
	got := e.ToSQL(pgCtx())
	want := `plainto_tsquery($1) AS "q"`
	if got != want {
		t.Errorf("PlainToTsquery.As: got %q, want %q", got, want)
	}
}

func TestTsQueryExpr_ColumnName_NoAlias(t *testing.T) {
	// Without alias, ColumnName returns the function name.
	e := expr.ToTsquery("grizzle & orm")
	if e.ColumnName() != "to_tsquery" {
		t.Errorf("ColumnName (no alias): got %q, want %q", e.ColumnName(), "to_tsquery")
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
