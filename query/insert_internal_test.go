package query

import (
	"errors"
	"testing"
)

func TestValueSlicePreservesFirstBuildError(t *testing.T) {
	first := errors.New("first build error")
	type namedRow struct {
		Name string `db:"name"`
	}
	var nilRows *[]namedRow

	cases := []struct {
		name string
		base *InsertBuilder
		rows any
	}{
		{"nil", &InsertBuilder{buildErr: first}, nil},
		{"typed nil", &InsertBuilder{buildErr: first}, nilRows},
		{"wrong kind", &InsertBuilder{buildErr: first}, 42},
		{"inconsistent columns", &InsertBuilder{colNames: []string{"other"}, buildErr: first}, []namedRow{{Name: "alice"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.base.ValueSlice(tc.rows)
			if got.buildErr != first {
				t.Fatalf("buildErr = %v, want original error %v", got.buildErr, first)
			}
		})
	}
}

func TestValueSliceCopiesExistingRowsBeforeAppend(t *testing.T) {
	type namedRow struct {
		Name string `db:"name"`
	}

	rows := make([][]any, 1, 3)
	rows[0] = []any{"base"}
	base := &InsertBuilder{colNames: []string{"name"}, rows: rows}

	left := base.ValueSlice([]namedRow{{Name: "left"}})
	right := base.ValueSlice([]namedRow{{Name: "right"}})

	if got := left.rows[1][0]; got != "left" {
		t.Fatalf("left row = %v, want left; sibling append mutated shared backing array", got)
	}
	if got := right.rows[1][0]; got != "right" {
		t.Fatalf("right row = %v, want right", got)
	}
	if len(base.rows) != 1 {
		t.Fatalf("base row count = %d, want 1", len(base.rows))
	}
}
