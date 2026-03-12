package expr_test

import (
	"testing"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	ts "github.com/sofired/grizzle/internal/testschema"
)

func newPGCtx() *expr.BuildContext {
	return expr.NewBuildContext(dialect.Postgres)
}

// -------------------------------------------------------------------
// FrameBound rendering
// -------------------------------------------------------------------

func TestFrameBoundSQL(t *testing.T) {
	tests := []struct {
		name  string
		bound expr.FrameBound
		want  string
	}{
		{"UnboundedPreceding", expr.UnboundedPreceding, "UNBOUNDED PRECEDING"},
		{"CurrentRow", expr.CurrentRow, "CURRENT ROW"},
		{"UnboundedFollowing", expr.UnboundedFollowing, "UNBOUNDED FOLLOWING"},
		{"Preceding(3)", expr.Preceding(3), "3 PRECEDING"},
		{"Following(5)", expr.Following(5), "5 FOLLOWING"},
		{"Preceding(0)", expr.Preceding(0), "0 PRECEDING"},
		{"Following(1)", expr.Following(1), "1 FOLLOWING"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// We test frame bounds indirectly by rendering a simple window with the bound.
			w := expr.WinSum(ts.UsersT.CreatedAt).Rows(tc.bound, tc.bound)
			ctx := newPGCtx()
			sql := w.ToSQL(ctx)
			want := `SUM("users"."created_at") OVER (ROWS BETWEEN ` + tc.want + ` AND ` + tc.want + `)`
			if sql != want {
				t.Errorf("got  %q\nwant %q", sql, want)
			}
		})
	}
}

// -------------------------------------------------------------------
// ROWS BETWEEN
// -------------------------------------------------------------------

func TestRowsFrame_UnboundedPrecedingToCurrentRow(t *testing.T) {
	w := expr.WinSum(ts.UsersT.CreatedAt).
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		Rows(expr.UnboundedPreceding, expr.CurrentRow).
		As("running_total")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `SUM("users"."created_at") OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS "running_total"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestRowsFrame_UnboundedPrecedingToUnboundedFollowing(t *testing.T) {
	w := expr.LastValue(ts.UsersT.Username).
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		Rows(expr.UnboundedPreceding, expr.UnboundedFollowing).
		As("last_username")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `LAST_VALUE("users"."username") OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING) AS "last_username"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestRowsFrame_OffsetBounds(t *testing.T) {
	w := expr.WinAvg(ts.UsersT.CreatedAt).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		Rows(expr.Preceding(2), expr.Following(2)).
		As("moving_avg")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `AVG("users"."created_at") OVER (ORDER BY "users"."created_at" ASC ROWS BETWEEN 2 PRECEDING AND 2 FOLLOWING) AS "moving_avg"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// -------------------------------------------------------------------
// RANGE BETWEEN
// -------------------------------------------------------------------

func TestRangeFrame_UnboundedPrecedingToCurrentRow(t *testing.T) {
	w := expr.WinSum(ts.UsersT.CreatedAt).
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		Range(expr.UnboundedPreceding, expr.CurrentRow).
		As("range_sum")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `SUM("users"."created_at") OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC RANGE BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS "range_sum"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestRangeFrame_CurrentRowToUnboundedFollowing(t *testing.T) {
	w := expr.WinSum(ts.UsersT.CreatedAt).
		OrderBy(ts.UsersT.CreatedAt.Desc()).
		Range(expr.CurrentRow, expr.UnboundedFollowing).
		As("future_sum")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `SUM("users"."created_at") OVER (ORDER BY "users"."created_at" DESC RANGE BETWEEN CURRENT ROW AND UNBOUNDED FOLLOWING) AS "future_sum"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// -------------------------------------------------------------------
// GROUPS BETWEEN
// -------------------------------------------------------------------

func TestGroupsFrame(t *testing.T) {
	w := expr.WinSum(ts.UsersT.CreatedAt).
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		Groups(expr.UnboundedPreceding, expr.CurrentRow).
		As("groups_sum")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `SUM("users"."created_at") OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC GROUPS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS "groups_sum"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestGroupsFrame_OffsetBounds(t *testing.T) {
	w := expr.WinAvg(ts.UsersT.CreatedAt).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		Groups(expr.Preceding(1), expr.Following(1))

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `AVG("users"."created_at") OVER (ORDER BY "users"."created_at" ASC GROUPS BETWEEN 1 PRECEDING AND 1 FOLLOWING)`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// -------------------------------------------------------------------
// NthValue with offset argument
// -------------------------------------------------------------------

func TestNthValue(t *testing.T) {
	w := expr.NthValue(ts.UsersT.Username, 3).
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		As("third_user")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `NTH_VALUE("users"."username", 3) OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC) AS "third_user"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestNthValue_WithFrame(t *testing.T) {
	w := expr.NthValue(ts.UsersT.Username, 2).
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		Rows(expr.UnboundedPreceding, expr.UnboundedFollowing).
		As("second_user")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `NTH_VALUE("users"."username", 2) OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING) AS "second_user"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// -------------------------------------------------------------------
// Lead / Lag with offset and default
// -------------------------------------------------------------------

func TestLeadWithOffset(t *testing.T) {
	w := expr.LeadWithOffset(ts.UsersT.Username, 2).
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		As("next2_user")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `LEAD("users"."username", 2) OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC) AS "next2_user"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestLeadWithDefault(t *testing.T) {
	w := expr.LeadWithDefault(ts.UsersT.Username, 1, "unknown").
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		As("next_user")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `LEAD("users"."username", 1, unknown) OVER (ORDER BY "users"."created_at" ASC) AS "next_user"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestLagWithOffset(t *testing.T) {
	w := expr.LagWithOffset(ts.UsersT.Username, 3).
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		As("prev3_user")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `LAG("users"."username", 3) OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC) AS "prev3_user"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestLagWithDefault(t *testing.T) {
	w := expr.LagWithDefault(ts.UsersT.Username, 1, "none").
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		As("prev_user")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `LAG("users"."username", 1, none) OVER (ORDER BY "users"."created_at" ASC) AS "prev_user"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// -------------------------------------------------------------------
// Backward-compatibility: existing functions without frame still work
// -------------------------------------------------------------------

func TestWindowExpr_NoFrame(t *testing.T) {
	w := expr.RowNumber().
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		As("rn")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `ROW_NUMBER() OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC) AS "rn"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestWindowExpr_Lead_NoOffset(t *testing.T) {
	w := expr.Lead(ts.UsersT.Email).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		As("next_email")

	ctx := newPGCtx()
	got := w.ToSQL(ctx)
	want := `LEAD("users"."email") OVER (ORDER BY "users"."created_at" ASC) AS "next_email"`
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// -------------------------------------------------------------------
// Example-style tests (run with go test -v to see output)
// -------------------------------------------------------------------

func ExampleWindowExpr_Rows() {
	w := expr.WinSum(ts.UsersT.CreatedAt).
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		Rows(expr.UnboundedPreceding, expr.CurrentRow).
		As("running_total")

	ctx := expr.NewBuildContext(dialect.Postgres)
	_ = w.ToSQL(ctx)
	// Output would be:
	// SUM("users"."created_at") OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS "running_total"
}

func ExampleNthValue() {
	w := expr.NthValue(ts.UsersT.Username, 3).
		PartitionBy(ts.UsersT.RealmID).
		OrderBy(ts.UsersT.CreatedAt.Asc()).
		Rows(expr.UnboundedPreceding, expr.UnboundedFollowing).
		As("third_user")

	ctx := expr.NewBuildContext(dialect.Postgres)
	_ = w.ToSQL(ctx)
	// Output would be:
	// NTH_VALUE("users"."username", 3) OVER (PARTITION BY "users"."realm_id" ORDER BY "users"."created_at" ASC ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING) AS "third_user"
}
