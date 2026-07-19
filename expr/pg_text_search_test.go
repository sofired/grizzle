package expr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	ts "github.com/sofired/grizzle/internal/testschema"
)

// helpers

func pgCtx() *expr.BuildContext { return expr.NewBuildContext(dialect.Postgres) }

func sql(e expr.Expression) string {
	rendered, err := e.RenderSQL(pgCtx())
	if err != nil {
		panic(err)
	}
	return rendered
}

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
	got, _ := rank.RenderSQL(pgCtx())
	want := `TS_RANK("articles"."search_vector", plainto_tsquery($1))`
	if got != want {
		t.Errorf("TsRank: got %q, want %q", got, want)
	}
}

func TestTsRankCd(t *testing.T) {
	tsq := expr.PlainToTsquery("grizzle orm")
	rank := expr.TsRankCd(ts.ArticlesT.SearchVector, tsq)
	got, _ := rank.RenderSQL(pgCtx())
	want := `TS_RANK_CD("articles"."search_vector", plainto_tsquery($1))`
	if got != want {
		t.Errorf("TsRankCd: got %q, want %q", got, want)
	}
}

func TestTsRank_Desc(t *testing.T) {
	tsq := expr.PlainToTsquery("grizzle orm")
	order := expr.TsRank(ts.ArticlesT.SearchVector, tsq).Desc()
	got, _ := order.RenderSQL(pgCtx())
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
	_, _ = e.RenderSQL(ctx)
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
	_, _ = e.RenderSQL(ctx)
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
	_, _ = e.RenderSQL(ctx)
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
	_, _ = e.RenderSQL(ctx)
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
	_, _ = e.RenderSQL(ctx)
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
	fmt.Println(mustRender(e, ctx))
	// Output:
	// "users"."email" ~ $1
}

func ExampleStringColumn_RegexpMatchI() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Email.RegexpMatchI("^alice")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// "users"."email" ~* $1
}

func ExampleStringColumn_NotRegexpMatch() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Email.NotRegexpMatch("^alice")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// "users"."email" !~ $1
}

func ExampleStringColumn_NotRegexpMatchI() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Email.NotRegexpMatchI("^alice")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// "users"."email" !~* $1
}

func ExampleTsvectorColumn_Matches() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.ArticlesT.SearchVector.Matches("grizzle & orm")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// "articles"."search_vector" @@ to_tsquery($1)
}

func ExampleTsvectorColumn_MatchesPlain() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.ArticlesT.SearchVector.MatchesPlain("grizzle orm")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// "articles"."search_vector" @@ plainto_tsquery($1)
}

func ExampleTsvectorColumn_MatchesWebSearch() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.ArticlesT.SearchVector.MatchesWebSearch("grizzle -orm")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// "articles"."search_vector" @@ websearch_to_tsquery($1)
}

func ExampleToTsvector() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.ToTsvector(ts.ArticlesT.Body, "english").MatchesPlain("grizzle orm")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// to_tsvector($1, "articles"."body") @@ plainto_tsquery($2)
}

func ExampleTsRank() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	tsq := expr.PlainToTsquery("grizzle orm")
	rank := expr.TsRank(ts.ArticlesT.SearchVector, tsq)
	fmt.Println(mustRender(rank.Desc(), ctx))
	// Output:
	// TS_RANK("articles"."search_vector", plainto_tsquery($1)) DESC
}

func ExampleStringColumn_NotLike() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Username.NotLike("admin%")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// "users"."username" NOT LIKE $1
}

func ExampleStringColumn_NotILike() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.UsersT.Username.NotILike("admin%")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// "users"."username" NOT ILIKE $1
}

func ExampleTsRankCd() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	tsq := expr.PlainToTsquery("grizzle orm")
	rank := expr.TsRankCd(ts.ArticlesT.SearchVector, tsq)
	fmt.Println(mustRender(rank, ctx))
	// Output:
	// TS_RANK_CD("articles"."search_vector", plainto_tsquery($1))
}

func ExampleTsvectorColumn_MatchesWithConfig() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := ts.ArticlesT.SearchVector.MatchesWithConfig("english", "grizzle & orm")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// "articles"."search_vector" @@ to_tsquery($1, $2)
}

func ExampleToTsquery() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.ToTsquery("grizzle & orm")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// to_tsquery($1)
}

func ExampleToTsqueryWithConfig() {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.ToTsqueryWithConfig("english", "grizzle & orm")
	fmt.Println(mustRender(e, ctx))
	// Output:
	// to_tsquery($1, $2)
}

// -----------------------------------------------------------------------
// TsvectorExpr alias tests
// -----------------------------------------------------------------------

func TestTsvectorExpr_As(t *testing.T) {
	got, _ := expr.ToTsvector(ts.ArticlesT.Body).As("tsv").RenderSQL(pgCtx())
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
// Non-PostgreSQL dialect behaviour
// -----------------------------------------------------------------------

// PostgreSQL-only expression families fail closed on unsupported dialects.
// They must not emit executable fallback SQL or leave orphaned arguments.
func TestPostgresOnlyExpressions_NonPG_ReturnUnsupportedFeature(t *testing.T) {
	cases := []struct {
		name string
		expr func() expr.Expression
	}{
		{"regexp", func() expr.Expression { return ts.UsersT.Email.RegexpMatch("^alice") }},
		{"regexp insensitive", func() expr.Expression { return ts.UsersT.Email.RegexpMatchI("^alice") }},
		{"not regexp", func() expr.Expression { return ts.UsersT.Email.NotRegexpMatch("^alice") }},
		{"not regexp insensitive", func() expr.Expression { return ts.UsersT.Email.NotRegexpMatchI("^alice") }},
		{"tsvector match", func() expr.Expression { return ts.ArticlesT.SearchVector.Matches("grizzle & orm") }},
		{"plain match", func() expr.Expression { return ts.ArticlesT.SearchVector.MatchesPlain("grizzle orm") }},
		{"configured match", func() expr.Expression { return ts.ArticlesT.SearchVector.MatchesWithConfig("english", "grizzle & orm") }},
		{"phrase match", func() expr.Expression { return ts.ArticlesT.SearchVector.MatchesPhrase("fast full text") }},
		{"websearch match", func() expr.Expression { return ts.ArticlesT.SearchVector.MatchesWebSearch("grizzle -orm") }},
		{"computed tsvector match", func() expr.Expression { return expr.ToTsvector(ts.ArticlesT.Body).MatchesPlain("grizzle orm") }},
		{"computed tsvector", func() expr.Expression { return expr.ToTsvector(ts.ArticlesT.Body) }},
		{"tsquery", func() expr.Expression { return expr.ToTsquery("grizzle & orm") }},
		{"plain tsquery", func() expr.Expression { return expr.PlainToTsquery("grizzle orm") }},
		{"phrase tsquery", func() expr.Expression { return expr.PhraseToTsquery("fast full text") }},
		{"websearch tsquery", func() expr.Expression { return expr.WebsearchToTsquery("grizzle -orm") }},
		{"configured tsquery", func() expr.Expression { return expr.ToTsqueryWithConfig("english", "grizzle & orm") }},
		{"configured plain tsquery", func() expr.Expression { return expr.PlainToTsqueryWithConfig("english", "grizzle orm") }},
		{"configured phrase tsquery", func() expr.Expression { return expr.PhraseToTsqueryWithConfig("english", "fast full text") }},
		{"configured websearch tsquery", func() expr.Expression { return expr.WebsearchToTsqueryWithConfig("english", "grizzle -orm") }},
		{"rank", func() expr.Expression {
			return expr.TsRank(ts.ArticlesT.SearchVector, expr.PlainToTsquery("grizzle orm"))
		}},
		{"rank cd", func() expr.Expression {
			return expr.TsRankCd(ts.ArticlesT.SearchVector, expr.PlainToTsquery("grizzle orm"))
		}},
	}

	dialects := []struct {
		name string
		d    dialect.Dialect
	}{
		{"mysql", dialect.MySQL},
		{"sqlite", dialect.SQLite},
	}
	for _, tc := range cases {
		for _, dc := range dialects {
			t.Run(tc.name+"/"+dc.name, func(t *testing.T) {
				ctx := expr.NewBuildContext(dc.d)
				got, err := tc.expr().RenderSQL(ctx)
				if got != "" {
					t.Errorf("rendered executable SQL on failure: %q", got)
				}
				if !errors.Is(err, expr.ErrUnsupportedFeature) {
					t.Fatalf("error = %v, want ErrUnsupportedFeature", err)
				}
				if len(ctx.Args()) != 0 {
					t.Errorf("orphaned args: %v", ctx.Args())
				}
			})
		}
	}
}

func TestPostgresOnlyPredicates_NegationPropagatesUnsupportedFeature(t *testing.T) {
	predicates := []struct {
		name string
		expr expr.Expression
	}{
		{"regexp", ts.UsersT.Email.RegexpMatch("^alice")},
		{"full text", ts.ArticlesT.SearchVector.MatchesPlain("grizzle orm")},
	}
	for _, predicate := range predicates {
		for _, d := range []dialect.Dialect{dialect.MySQL, dialect.SQLite} {
			t.Run(predicate.name+"/"+d.Name(), func(t *testing.T) {
				ctx := expr.NewBuildContext(d)
				got, err := expr.Not(predicate.expr).RenderSQL(ctx)
				if got != "" || !errors.Is(err, expr.ErrUnsupportedFeature) {
					t.Fatalf("got (%q, %v), want empty SQL and ErrUnsupportedFeature", got, err)
				}
				if len(ctx.Args()) != 0 {
					t.Fatalf("negated failure retained args: %v", ctx.Args())
				}
			})
		}
	}
}
