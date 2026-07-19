package query_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	"github.com/sofired/grizzle/query"
)

type unsafeTable string

func (t unsafeTable) GrizTableName() string  { return string(t) }
func (t unsafeTable) GrizTableAlias() string { return string(t) }

type nilTestDialect struct{ dialect.Dialect }

type leakingExpression struct{}

func (leakingExpression) RenderSQL(ctx *expr.BuildContext) (string, error) {
	_ = ctx.Add("sensitive-value")
	return "partial unsafe SQL", fmt.Errorf("driver detail: secret-table")
}

func TestBuild_RejectsNilAndTypedNilDialect(t *testing.T) {
	var typedNil *nilTestDialect
	for _, d := range []dialect.Dialect{nil, typedNil} {
		sql, args, err := query.Select().Build(d)
		if sql != "" || args != nil {
			t.Fatalf("got SQL %q args %v on invalid dialect", sql, args)
		}
		if !errors.Is(err, query.ErrUnsupportedDialect) {
			t.Fatalf("error = %v, want ErrUnsupportedDialect", err)
		}
	}
}

func TestBuild_InvalidIdentifierFailsClosedAndRedacted(t *testing.T) {
	const unsafe = "users\nsecret-table"
	sql, args, err := query.Select().From(unsafeTable(unsafe)).Build(dialect.Postgres)
	if sql != "" || args != nil {
		t.Fatalf("got SQL %q args %v on invalid identifier", sql, args)
	}
	if !errors.Is(err, query.ErrInvalidIdentifier) {
		t.Fatalf("error = %v, want ErrInvalidIdentifier", err)
	}
	if strings.Contains(err.Error(), unsafe) || strings.Contains(err.Error(), "secret-table") {
		t.Fatalf("error leaked unsafe identifier: %q", err)
	}
}

func TestBuild_ExpressionErrorFailsClosedWithoutArguments(t *testing.T) {
	col := expr.StringColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "name"}}
	b := query.Select().From(unsafeTable("users")).Where(expr.And(
		col.EQ("sensitive-value"),
		expr.RawArgs("x = $? AND y = $?", 1),
	))
	sql, args, err := b.Build(dialect.Postgres)
	if sql != "" || args != nil {
		t.Fatalf("got SQL %q args %v on failed render", sql, args)
	}
	if !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("error = %v, want ErrBuildValidation", err)
	}
	if strings.Contains(err.Error(), "sensitive-value") || strings.Contains(err.Error(), "x =") {
		t.Fatalf("error leaked SQL or value: %q", err)
	}
}

func TestBuild_ExternalExpressionErrorIsNormalizedAndRedacted(t *testing.T) {
	sql, args, err := query.Select().Where(leakingExpression{}).Build(dialect.Postgres)
	if sql != "" || args != nil {
		t.Fatalf("got SQL %q args %v on failed external render", sql, args)
	}
	if !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("error = %v, want ErrBuildValidation", err)
	}
	if strings.Contains(err.Error(), "secret-table") || strings.Contains(err.Error(), "partial unsafe SQL") {
		t.Fatalf("error leaked external details: %q", err)
	}
}

func TestBuild_UnsupportedExpressionFailsClosedWithoutArguments(t *testing.T) {
	col := expr.StringColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "name"}}
	b := query.Select().From(unsafeTable("users")).Where(expr.Not(col.RegexpMatch("secret-pattern")))
	sql, args, err := b.Build(dialect.MySQL)
	if sql != "" || args != nil {
		t.Fatalf("got SQL %q args %v on unsupported feature", sql, args)
	}
	if !errors.Is(err, query.ErrUnsupportedFeature) {
		t.Fatalf("error = %v, want ErrUnsupportedFeature", err)
	}
}

func TestBuild_RejectsTypedNilWherePredicates(t *testing.T) {
	var typedNil *leakingExpression
	table := unsafeTable("users")
	builders := []query.Builder{
		query.Select().From(table).Where(typedNil),
		query.Update(table).Set("name", "alice").Where(typedNil),
		query.DeleteFrom(table).Where(typedNil),
	}
	for _, b := range builders {
		assertBuildError(t, b, dialect.Postgres, query.ErrBuildValidation)
	}
}

func TestBuild_LogicalCombinatorsPreserveTypedNilPredicates(t *testing.T) {
	var typedNil *leakingExpression
	table := unsafeTable("users")
	builders := []query.Builder{
		query.Select().From(table).Where(expr.And(typedNil, nil)),
		query.Select().From(table).Where(expr.Or(nil, typedNil)),
		query.Select().From(table).Where(expr.Not(typedNil)),
		query.Update(table).Set("name", "alice").Where(typedNil).And(nil),
		query.DeleteFrom(table).Where(typedNil).And(nil),
	}
	for _, b := range builders {
		assertBuildError(t, b, dialect.Postgres, query.ErrBuildValidation)
	}
}

func TestBuild_CaseExpressionsRejectTypedNilFallbacks(t *testing.T) {
	var typedNil *leakingExpression
	username := expr.StringColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "username"}}
	expressions := []expr.Expression{
		expr.Case().When(expr.Raw("TRUE"), expr.Lit("matched")).Else(typedNil),
		expr.SimpleCase(username).WhenVal("alice", expr.Lit("matched")).Else(typedNil),
	}
	for _, expression := range expressions {
		ctx := expr.NewBuildContext(dialect.Postgres)
		sql, err := expression.RenderSQL(ctx)
		if sql != "" || !errors.Is(err, expr.ErrBuildValidation) {
			t.Fatalf("RenderSQL = (%q, %v), want empty SQL and ErrBuildValidation", sql, err)
		}
		if len(ctx.Args()) != 0 {
			t.Fatalf("Args = %v, want no orphaned arguments", ctx.Args())
		}
	}
}

func TestBuild_AllowsIntentionalNilWherePredicates(t *testing.T) {
	table := unsafeTable("users")
	builders := []query.Builder{
		query.Select().From(table).Where(nil),
		query.Update(table).Set("name", "alice").Where(nil),
		query.DeleteFrom(table).Where(nil),
	}
	for _, b := range builders {
		sql, _, err := b.Build(dialect.Postgres)
		if err != nil || sql == "" {
			t.Fatalf("intentional nil WHERE build = (%q, %v), want non-empty SQL and nil error", sql, err)
		}
	}
}

func TestBuild_RejectsNilJoinPredicates(t *testing.T) {
	var typedNil *leakingExpression
	for _, predicate := range []expr.Expression{nil, typedNil} {
		builders := []query.Builder{
			query.Select().From(unsafeTable("users")).InnerJoin(unsafeTable("realms"), predicate),
			query.Select().From(unsafeTable("users")).LeftJoin(unsafeTable("realms"), predicate),
			query.Select().From(unsafeTable("users")).RightJoin(unsafeTable("realms"), predicate),
			query.Select().From(unsafeTable("users")).FullJoin(unsafeTable("realms"), predicate),
		}
		for _, b := range builders {
			assertBuildError(t, b, dialect.Postgres, query.ErrBuildValidation)
		}
	}
}

func TestBuild_InvalidLockMetadataReturnsBuildValidation(t *testing.T) {
	base := query.Select().From(unsafeTable("users"))
	cases := []query.Builder{
		base.For(query.LockStrength("FOR UPDATE; DROP TABLE users")),
		base.For(query.LockForUpdate, query.LockOption("INVALID")),
		base.For(query.LockForUpdate, query.NoWait, query.NoWait),
		base.Of(),
		base.ForUpdate().Of(unsafeTable("other")),
	}
	for _, b := range cases {
		sql, args, err := b.Build(dialect.Postgres)
		if sql != "" || args != nil || !errors.Is(err, query.ErrBuildValidation) {
			t.Fatalf("got (%q, %v, %v), want closed build-validation failure", sql, args, err)
		}
	}
}

func TestBuild_ConflictValidationFailsClosed(t *testing.T) {
	table := unsafeTable("users")

	postgresMissingTarget := query.InsertInto(table).
		Values(struct {
			Name string `db:"name"`
		}{Name: "alice"}).
		DoUpdateSet("name", "bob")
	assertBuildError(t, postgresMissingTarget, dialect.Postgres, query.ErrBuildValidation)

	mysqlTarget := query.InsertInto(table).
		Values(struct {
			Name string `db:"name"`
		}{Name: "alice"}).
		OnConflict("name").
		DoUpdateSet("name", "bob")
	assertBuildError(t, mysqlTarget, dialect.MySQL, query.ErrUnsupportedFeature)
}

func TestBuild_NegativePaginationReturnsBuildValidation(t *testing.T) {
	table := unsafeTable("users")
	cases := []query.Builder{
		query.Select().From(table).Limit(-1),
		query.Select().From(table).Offset(-1),
		query.Select().Union(query.Select()).Limit(-1),
		query.Select().Union(query.Select()).Offset(-1),
	}
	for _, b := range cases {
		assertBuildError(t, b, dialect.Postgres, query.ErrBuildValidation)
	}
}
