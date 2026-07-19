package kit_test

import (
	"testing"

	"github.com/sofired/grizzle/kit"
	"github.com/sofired/grizzle/schema/pg"
)

func TestDiff_DefaultPresenceControlsExpressionComparison(t *testing.T) {
	cases := []struct {
		name                   string
		oldHasDefault          bool
		oldExpr                string
		newHasDefault          bool
		newExpr                string
		wantAlterColumnDefault bool
	}{
		{
			name:    "both_has_default_false_ignore_residual_expression_text",
			oldExpr: "",
			newExpr: "ignored residual text",
		},
		{
			name:                   "add_non_null_default",
			oldExpr:                "ignored residual text",
			newHasDefault:          true,
			newExpr:                "'pending'",
			wantAlterColumnDefault: true,
		},
		{
			name:                   "remove_non_null_default",
			oldHasDefault:          true,
			oldExpr:                "'pending'",
			newExpr:                "ignored residual text",
			wantAlterColumnDefault: true,
		},
		{
			name:                   "change_non_null_default_expression",
			oldHasDefault:          true,
			oldExpr:                "'pending'",
			newHasDefault:          true,
			newExpr:                "'active'",
			wantAlterColumnDefault: true,
		},
		{
			name:          "same_non_null_default_is_unchanged",
			oldHasDefault: true,
			oldExpr:       "'pending'",
			newHasDefault: true,
			newExpr:       "'pending'",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changes := diffSingleColumnDefault(
				tc.oldHasDefault,
				tc.oldExpr,
				tc.newHasDefault,
				tc.newExpr,
			)
			requireAlterColumnDefault(
				t,
				changes,
				tc.wantAlterColumnDefault,
				tc.oldHasDefault,
				tc.oldExpr,
				tc.newHasDefault,
				tc.newExpr,
			)
		})
	}
}

// TestDiff_ExplicitNullDefaultPresence uses representative legacy snapshot
// expressions for PostgreSQL, MySQL, and SQLite. It does not exercise live
// introspection, database connections, or dialect SQL generation.
func TestDiff_ExplicitNullDefaultPresence(t *testing.T) {
	expressionStyles := []struct {
		name         string
		explicitNull string
	}{
		{name: "postgresql_style_expression", explicitNull: "NULL::text"},
		{name: "mysql_style_expression", explicitNull: "NULL"},
		{name: "sqlite_style_expression", explicitNull: "NULL"},
	}

	for _, style := range expressionStyles {
		t.Run(style.name, func(t *testing.T) {
			cases := []struct {
				name                   string
				oldHasDefault          bool
				oldExpr                string
				newHasDefault          bool
				newExpr                string
				wantAlterColumnDefault bool
			}{
				{
					name:    "both_has_default_false_ignore_null_expression_text",
					newExpr: style.explicitNull,
				},
				{
					name:                   "add_explicit_null_when_presence_becomes_true",
					oldExpr:                style.explicitNull,
					newHasDefault:          true,
					newExpr:                style.explicitNull,
					wantAlterColumnDefault: true,
				},
				{
					name:                   "remove_explicit_null_when_presence_becomes_false",
					oldHasDefault:          true,
					oldExpr:                style.explicitNull,
					newExpr:                style.explicitNull,
					wantAlterColumnDefault: true,
				},
				{
					name:          "same_explicit_null_default_is_unchanged",
					oldHasDefault: true,
					oldExpr:       style.explicitNull,
					newHasDefault: true,
					newExpr:       style.explicitNull,
				},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					changes := diffSingleColumnDefault(
						tc.oldHasDefault,
						tc.oldExpr,
						tc.newHasDefault,
						tc.newExpr,
					)
					requireAlterColumnDefault(
						t,
						changes,
						tc.wantAlterColumnDefault,
						tc.oldHasDefault,
						tc.oldExpr,
						tc.newHasDefault,
						tc.newExpr,
					)
				})
			}
		})
	}
}

func diffSingleColumnDefault(oldHasDefault bool, oldExpr string, newHasDefault bool, newExpr string) []kit.Change {
	snapshot := func(hasDefault bool, defaultExpr string) kit.Snapshot {
		return kit.Snapshot{
			Version: "1",
			Tables: map[string]*kit.TableSnap{
				"items": {
					Name: "items",
					Columns: []pg.ColumnDef{
						{
							Name:        "status",
							SQLType:     "text",
							HasDefault:  hasDefault,
							DefaultExpr: defaultExpr,
						},
					},
				},
			},
		}
	}

	return kit.Diff(snapshot(oldHasDefault, oldExpr), snapshot(newHasDefault, newExpr))
}

func requireAlterColumnDefault(
	t *testing.T,
	changes []kit.Change,
	want bool,
	oldHasDefault bool,
	oldExpr string,
	newHasDefault bool,
	newExpr string,
) {
	t.Helper()
	if !want {
		if len(changes) != 0 {
			t.Fatalf(
				"expected no changes for HasDefault=%t DefaultExpr=%q -> HasDefault=%t DefaultExpr=%q; got %v",
				oldHasDefault,
				oldExpr,
				newHasDefault,
				newExpr,
				changes,
			)
		}
		return
	}
	if len(changes) != 1 || changes[0].Kind != kit.ChangeAlterColumnDefault {
		t.Fatalf(
			"expected one AlterColumnDefault for HasDefault=%t DefaultExpr=%q -> HasDefault=%t DefaultExpr=%q; got %v",
			oldHasDefault,
			oldExpr,
			newHasDefault,
			newExpr,
			changes,
		)
	}
}
