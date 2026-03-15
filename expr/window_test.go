package expr_test

import (
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

// -------------------------------------------------------------------
// Fix #93 — LeadWithDefault / LagWithDefault bind default as param
// -------------------------------------------------------------------

func TestLeadWithDefault_DefaultBoundAsParam(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	w := expr.LeadWithDefault(testUsernameCol, 1, "N/A").OrderBy(testScoreCol.Asc())
	sql := w.ToSQL(ctx)
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

func TestLagWithDefault_DefaultBoundAsParam(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	w := expr.LagWithDefault(testScoreCol, 1, 0).OrderBy(testScoreCol.Asc())
	sql := w.ToSQL(ctx)
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
	sql := w.ToSQL(ctx)

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
	sql := w.ToSQL(ctx)
	if !strings.Contains(sql, "NTH_VALUE(") {
		t.Errorf("expected NTH_VALUE in SQL, got: %s", sql)
	}
}

func TestNthValue_InvalidN_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for NthValue(col, 0)")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "n must be >= 1") {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()
	expr.NthValue(testUsernameCol, 0)
}

func TestNthValue_NegativeN_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for NthValue(col, -1)")
		}
	}()
	expr.NthValue(testUsernameCol, -1)
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
	if expr.UnboundedPreceding().SQL() != expr.UnboundedPreceding().SQL() {
		t.Error("UnboundedPreceding() must return same value on each call")
	}
}

// -------------------------------------------------------------------
// Fix #131 — AliasedCol does not emit AS in GROUP BY / ORDER BY
// -------------------------------------------------------------------

func TestAliasedCol_ToSQL_EmitsAlias(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	col := expr.ColAs(testUsernameCol, "uname")
	got := col.ToSQL(ctx)
	want := `"users"."username" AS "uname"`
	if got != want {
		t.Errorf("ToSQL: got %q, want %q", got, want)
	}
}

func TestAliasedCol_colRef_NoAlias(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	col := expr.ColAs(testUsernameCol, "uname")
	// OrderExpr.ToSQL calls colRef internally — should not include the alias.
	orderExpr := col.Asc()
	got := orderExpr.ToSQL(ctx)
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
	got := e.ToSQL(ctx)
	want := "col = $1 AND other = $2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	args := ctx.Args()
	if len(args) != 2 || args[0] != 42 || args[1] != "hello" {
		t.Errorf("unexpected args: %v", args)
	}
}

func TestRawArgs_MismatchPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for mismatched placeholder count")
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, "placeholder count") {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()
	ctx := expr.NewBuildContext(dialect.Postgres)
	expr.RawArgs("col = $? AND other = $?", 42).ToSQL(ctx) // 2 placeholders, 1 arg
}

func TestRawArgs_NoPlaceholders_NoArgs(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.RawArgs("TRUE")
	got := e.ToSQL(ctx)
	if got != "TRUE" {
		t.Errorf("got %q, want %q", got, "TRUE")
	}
}
