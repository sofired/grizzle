package expr_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

// helpers for window function tests
var (
	testUsernameCol = expr.StringColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "username"}}
	testScoreCol    = expr.IntColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "score"}}
)

type noWindowDialect struct{ dialect.Dialect }

func (noWindowDialect) SupportsWindowFunctions() bool { return false }

// -------------------------------------------------------------------
// Fix #93 — LeadWithDefault / LagWithDefault bind default as param
// -------------------------------------------------------------------

func TestLeadWithDefault_DefaultBoundAsParam(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	w := expr.LeadWithDefault(testUsernameCol, 1, "N/A").OrderBy(testScoreCol.Asc())
	sql, _ := w.RenderSQL(ctx)
	args := ctx.Args()

	// The default value must appear as a bound parameter, not literal text.
	if strings.Contains(sql, "'N/A'") || strings.Contains(sql, `"N/A"`) {
		t.Errorf("default must not be interpolated directly into SQL, got: %s", sql)
	}
	// Must have 2 bound args: offset (1) and default ("N/A").
	if len(args) != 2 {
		t.Errorf("expected 2 bound args (offset + default), got %d: %v", len(args), args)
	}
	if args[0] != 1 {
		t.Errorf("expected args[0] = 1 (offset), got %v", args[0])
	}
	if args[1] != "N/A" {
		t.Errorf("expected args[1] = \"N/A\" (default), got %v", args[1])
	}
	// Both should be referenced as placeholders in the SQL.
	if !strings.Contains(sql, "$1") || !strings.Contains(sql, "$2") {
		t.Errorf("expected $1 and $2 placeholders in SQL, got: %s", sql)
	}
}

func TestLeadWithDefault_NumericDefaultBoundAsParams(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	w := expr.LeadWithDefault(testScoreCol, 1, 0).OrderBy(testScoreCol.Asc())
	sql, err := w.RenderSQL(ctx)
	if err != nil {
		t.Fatalf("RenderSQL() error = %v", err)
	}

	wantSQL := `LEAD("users"."score", $1, $2) OVER (ORDER BY "users"."score" ASC)`
	if sql != wantSQL {
		t.Errorf("RenderSQL() = %q, want %q", sql, wantSQL)
	}
	if placeholderCount := strings.Count(sql, "$"); placeholderCount != 2 {
		t.Errorf("placeholder count = %d, want 2 in SQL %q", placeholderCount, sql)
	}

	args := ctx.Args()
	wantArgs := []any{1, 0}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("Args() = %#v, want %#v", args, wantArgs)
	}
}

func TestLagWithDefault_DefaultBoundAsParam(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	w := expr.LagWithDefault(testScoreCol, 1, 0).OrderBy(testScoreCol.Asc())
	sql, _ := w.RenderSQL(ctx)
	args := ctx.Args()

	if !strings.Contains(sql, "LAG(") {
		t.Errorf("expected LAG function, got: %s", sql)
	}
	// offset + default should both be bound params
	if len(args) != 2 {
		t.Errorf("expected 2 bound args (offset + default), got %d: %v", len(args), args)
	}
	if args[0] != 1 {
		t.Errorf("expected args[0] = 1 (offset), got %v", args[0])
	}
	if args[1] != 0 {
		t.Errorf("expected args[1] = 0 (default), got %v", args[1])
	}
}

func TestLagWithDefault_StringDefault_NotInterpolated(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	// A string with single quotes must be handled safely.
	w := expr.LagWithDefault(testUsernameCol, 1, "it's fine").OrderBy(testScoreCol.Asc())
	sql, _ := w.RenderSQL(ctx)
	if strings.Contains(sql, "it's fine") {
		t.Errorf("default string must not be interpolated into SQL, got: %s", sql)
	}
	args := ctx.Args()
	found := false
	for _, a := range args {
		if a == "it's fine" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected default string as bound arg, args: %v", args)
	}
}

// -------------------------------------------------------------------
// Fix #99 — NthValue validates n >= 1
// -------------------------------------------------------------------

func TestNthValue_ValidN(t *testing.T) {
	// Should not panic.
	ctx := expr.NewBuildContext(dialect.Postgres)
	w := expr.NthValue(testUsernameCol, 1)
	sql, _ := w.RenderSQL(ctx)
	if !strings.Contains(sql, "NTH_VALUE(") {
		t.Errorf("expected NTH_VALUE in SQL, got: %s", sql)
	}
}

func TestNthValue_InvalidN_ReturnsBuildError(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got, err := expr.NthValue(testUsernameCol, 0).RenderSQL(ctx)
	if got != "" {
		t.Errorf("SQL = %q, want empty", got)
	}
	if !errors.Is(err, expr.ErrBuildValidation) {
		t.Fatalf("error = %v, want ErrBuildValidation", err)
	}
}

func TestNthValue_NegativeN_ReturnsBuildError(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got, err := expr.NthValue(testUsernameCol, -1).RenderSQL(ctx)
	if got != "" {
		t.Errorf("SQL = %q, want empty", got)
	}
	if !errors.Is(err, expr.ErrBuildValidation) {
		t.Fatalf("error = %v, want ErrBuildValidation", err)
	}
}

func TestWindowExpr_UnsupportedDialectReturnsError(t *testing.T) {
	ctx := expr.NewBuildContext(noWindowDialect{Dialect: dialect.Postgres})
	got, err := expr.RowNumber().RenderSQL(ctx)
	if got != "" || !errors.Is(err, expr.ErrUnsupportedFeature) {
		t.Fatalf("RenderSQL = (%q, %v), want empty SQL and ErrUnsupportedFeature", got, err)
	}
	if len(ctx.Args()) != 0 {
		t.Fatalf("Args = %v, want no orphaned arguments", ctx.Args())
	}
}

func TestWindowExpr_RequiredColumnsRejectNil(t *testing.T) {
	var typedNil *expr.StringColumn
	columns := []struct {
		name string
		col  expr.SelectableColumn
	}{
		{"plain nil", nil},
		{"typed nil", typedNil},
	}
	factories := []struct {
		name string
		new  func(expr.SelectableColumn) expr.WindowExpr
	}{
		{"lead", expr.Lead},
		{"lead with default", func(col expr.SelectableColumn) expr.WindowExpr { return expr.LeadWithDefault(col, 1, "default") }},
		{"lag", expr.Lag},
		{"lag with default", func(col expr.SelectableColumn) expr.WindowExpr { return expr.LagWithDefault(col, 1, "default") }},
		{"first value", expr.FirstValue},
		{"last value", expr.LastValue},
		{"nth value", func(col expr.SelectableColumn) expr.WindowExpr { return expr.NthValue(col, 1) }},
		{"sum", expr.WinSum},
		{"avg", expr.WinAvg},
	}

	for _, column := range columns {
		for _, factory := range factories {
			t.Run(column.name+"/"+factory.name, func(t *testing.T) {
				ctx := expr.NewBuildContext(dialect.Postgres)
				got, err := factory.new(column.col).RenderSQL(ctx)
				if got != "" || !errors.Is(err, expr.ErrBuildValidation) {
					t.Fatalf("RenderSQL = (%q, %v), want empty SQL and ErrBuildValidation", got, err)
				}
				if len(ctx.Args()) != 0 {
					t.Fatalf("Args = %v, want no orphaned arguments", ctx.Args())
				}
			})
		}
	}
}

// -------------------------------------------------------------------
// Fix #104 — window frame sentinels are immutable
// -------------------------------------------------------------------

func TestWindowFrameBound_Immutable(t *testing.T) {
	// Verify we get distinct values for each sentinel.
	a := expr.UnboundedPreceding()
	b := expr.CurrentRow()
	c := expr.UnboundedFollowing()

	if a.SQL() != "UNBOUNDED PRECEDING" {
		t.Errorf("UnboundedPreceding: got %q", a.SQL())
	}
	if b.SQL() != "CURRENT ROW" {
		t.Errorf("CurrentRow: got %q", b.SQL())
	}
	if c.SQL() != "UNBOUNDED FOLLOWING" {
		t.Errorf("UnboundedFollowing: got %q", c.SQL())
	}

	// Each call returns the same value (they are functionally constant).
	// Compare a second call against the known constant, not against itself.
	if expr.UnboundedPreceding().SQL() != "UNBOUNDED PRECEDING" {
		t.Error("UnboundedPreceding() must return same value on each call")
	}
	if expr.CurrentRow().SQL() != "CURRENT ROW" {
		t.Error("CurrentRow() must return same value on each call")
	}
	if expr.UnboundedFollowing().SQL() != "UNBOUNDED FOLLOWING" {
		t.Error("UnboundedFollowing() must return same value on each call")
	}
}

// -------------------------------------------------------------------
// Fix #131 — AliasedCol does not emit AS in GROUP BY / ORDER BY
// -------------------------------------------------------------------

func TestAliasedCol_RenderSQL_EmitsAlias(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	col := expr.ColAs(testUsernameCol, "uname")
	got, _ := col.RenderSQL(ctx)
	want := `"users"."username" AS "uname"`
	if got != want {
		t.Errorf("RenderSQL: got %q, want %q", got, want)
	}
}

func TestAliasedCol_colRef_NoAlias(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	col := expr.ColAs(testUsernameCol, "uname")
	// OrderExpr.RenderSQL calls colRef internally — should not include the alias.
	orderExpr := col.Asc()
	got, _ := orderExpr.RenderSQL(ctx)
	// Check for " AS " with surrounding spaces, not just "AS" (which appears in "ASC").
	if strings.Contains(got, " AS ") {
		t.Errorf("ORDER BY must not include AS alias clause, got: %s", got)
	}
	if !strings.Contains(got, `"users"."username"`) {
		t.Errorf("ORDER BY must include column reference, got: %s", got)
	}
}

// -------------------------------------------------------------------
// Fix #129 — RawArgs excess placeholders
// -------------------------------------------------------------------

func TestRawArgs_MatchingPlaceholders(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.RawArgs("col = $? AND other = $?", 42, "hello")
	got, _ := e.RenderSQL(ctx)
	want := "col = $1 AND other = $2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	args := ctx.Args()
	if len(args) != 2 || args[0] != 42 || args[1] != "hello" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestRawArgs_TooFewArgs_ReturnsBuildError(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got, err := expr.RawArgs("col = $? AND other = $?", 42).RenderSQL(ctx)
	if got != "" || !errors.Is(err, expr.ErrBuildValidation) {
		t.Fatalf("got (%q, %v), want empty SQL and ErrBuildValidation", got, err)
	}
	if len(ctx.Args()) != 0 {
		t.Fatalf("orphaned args: %v", ctx.Args())
	}
}

func TestRawArgs_TooManyArgs_ReturnsBuildError(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	got, err := expr.RawArgs("col = $?", 42, "extra").RenderSQL(ctx)
	if got != "" || !errors.Is(err, expr.ErrBuildValidation) {
		t.Fatalf("got (%q, %v), want empty SQL and ErrBuildValidation", got, err)
	}
	if len(ctx.Args()) != 0 {
		t.Fatalf("orphaned args: %v", ctx.Args())
	}
}

func TestRawArgs_NoPlaceholders_NoArgs(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.RawArgs("TRUE")
	got, _ := e.RenderSQL(ctx)
	if got != "TRUE" {
		t.Errorf("got %q, want %q", got, "TRUE")
	}
}
