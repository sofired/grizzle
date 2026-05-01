package kit

import (
	"testing"
)

func TestEnumAddedValues_PrependBeforeExisting(t *testing.T) {
	// new values "a","b" inserted before existing "z"; "a" should get Before:"z", "b" should get After:"a"
	old := &EnumSnap{Name: "s", Values: []string{"z"}}
	nw := &EnumSnap{Name: "s", Values: []string{"a", "b", "z"}}
	got := enumAddedValues(old, nw)
	if len(got) != 2 {
		t.Fatalf("expected 2 additions, got %d", len(got))
	}
	if got[0].Value != "a" || got[0].Before != "z" || got[0].After != "" {
		t.Errorf("got[0] = %+v, want {Value:a Before:z}", got[0])
	}
	if got[1].Value != "b" || got[1].After != "a" || got[1].Before != "" {
		t.Errorf("got[1] = %+v, want {Value:b After:a}", got[1])
	}
}

func TestEnumAddedValues_AllNewNoFollowingAnchor(t *testing.T) {
	// new values "a","b" added to enum that had only "x" (not retained in new);
	// "a" has no preceding or following existing anchor → plain append (Before:"", After:"");
	// "b" anchors After:"a" (the previously added value)
	old := &EnumSnap{Name: "s", Values: []string{"x"}}
	nw := &EnumSnap{Name: "s", Values: []string{"a", "b"}}
	got := enumAddedValues(old, nw)
	if len(got) != 2 {
		t.Fatalf("expected 2 additions, got %d", len(got))
	}
	if got[0].Value != "a" || got[0].Before != "" || got[0].After != "" {
		t.Errorf("got[0] = %+v, want {Value:a Before:'' After:''}", got[0])
	}
	if got[1].Value != "b" || got[1].After != "a" || got[1].Before != "" {
		t.Errorf("got[1] = %+v, want {Value:b After:a}", got[1])
	}
}

func TestEnumAddedValues_AppendToEnd(t *testing.T) {
	// old=[a,b], new=[a,b,c]: "c" should have After:"b", Before:""
	old := &EnumSnap{Name: "s", Values: []string{"a", "b"}}
	nw := &EnumSnap{Name: "s", Values: []string{"a", "b", "c"}}
	got := enumAddedValues(old, nw)
	if len(got) != 1 {
		t.Fatalf("expected 1 addition, got %d", len(got))
	}
	if got[0].Value != "c" || got[0].After != "b" || got[0].Before != "" {
		t.Errorf("got %+v, want {Value:c After:b}", got[0])
	}
}

func TestEnumAddedValues_InterleavedMultiPosition(t *testing.T) {
	// old=[a,c,e], new=[a,b,c,d,e]: "b" inserted between "a" and "c",
	// "d" inserted between "c" and "e" — non-contiguous interleaved insertions.
	old := &EnumSnap{Name: "s", Values: []string{"a", "c", "e"}}
	nw := &EnumSnap{Name: "s", Values: []string{"a", "b", "c", "d", "e"}}
	got := enumAddedValues(old, nw)
	if len(got) != 2 {
		t.Fatalf("expected 2 additions, got %d", len(got))
	}
	if got[0].Value != "b" || got[0].After != "a" || got[0].Before != "" {
		t.Errorf("got[0] = %+v, want {Value:b After:a}", got[0])
	}
	if got[1].Value != "d" || got[1].After != "c" || got[1].Before != "" {
		t.Errorf("got[1] = %+v, want {Value:d After:c}", got[1])
	}
}

// -------------------------------------------------------------------
// enumDrift white-box unit tests
// -------------------------------------------------------------------

func TestEnumDrift_ValuesRemoved_NoReorder(t *testing.T) {
	old := &EnumSnap{Name: "s", Values: []string{"a", "b", "c"}}
	nw := &EnumSnap{Name: "s", Values: []string{"a", "c"}}
	removed, reordered := enumDrift(old, nw)
	if len(removed) != 1 || removed[0] != "b" {
		t.Errorf("expected removed=[b], got %v", removed)
	}
	if reordered {
		t.Error("expected reordered=false when only values are removed")
	}
}

func TestEnumDrift_ValuesReordered_NoneRemoved(t *testing.T) {
	old := &EnumSnap{Name: "s", Values: []string{"a", "b", "c"}}
	nw := &EnumSnap{Name: "s", Values: []string{"a", "c", "b"}}
	removed, reordered := enumDrift(old, nw)
	if len(removed) != 0 {
		t.Errorf("expected no removed values, got %v", removed)
	}
	if !reordered {
		t.Error("expected reordered=true when retained values change order")
	}
}

func TestEnumDrift_BothRemovedAndReordered(t *testing.T) {
	old := &EnumSnap{Name: "s", Values: []string{"a", "b", "c", "d"}}
	// "b" is removed and "c","d" are swapped relative to old
	nw := &EnumSnap{Name: "s", Values: []string{"a", "d", "c"}}
	removed, reordered := enumDrift(old, nw)
	if len(removed) != 1 || removed[0] != "b" {
		t.Errorf("expected removed=[b], got %v", removed)
	}
	if !reordered {
		t.Error("expected reordered=true when retained values change order")
	}
}

func TestEnumDrift_NoChanges(t *testing.T) {
	old := &EnumSnap{Name: "s", Values: []string{"a", "b", "c"}}
	nw := &EnumSnap{Name: "s", Values: []string{"a", "b", "c"}}
	removed, reordered := enumDrift(old, nw)
	if len(removed) != 0 {
		t.Errorf("expected no removed values, got %v", removed)
	}
	if reordered {
		t.Error("expected reordered=false for identical enum values")
	}
}

// -------------------------------------------------------------------
// normalizeViewSQL unit tests
// -------------------------------------------------------------------

func TestNormalizeViewSQL_TrailingSemicolon(t *testing.T) {
	got := normalizeViewSQL("SELECT 1;")
	want := "SELECT 1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeViewSQL_LeadingTrailingWhitespace(t *testing.T) {
	got := normalizeViewSQL("  SELECT 1  ")
	want := "SELECT 1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeViewSQL_WhitespaceAndSemicolon(t *testing.T) {
	got := normalizeViewSQL("  SELECT 1;  ")
	want := "SELECT 1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeViewSQL_MultipleTrailingSemicolons(t *testing.T) {
	got := normalizeViewSQL("SELECT 1;;;")
	want := "SELECT 1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNormalizeViewSQL_EmptyString(t *testing.T) {
	got := normalizeViewSQL("")
	want := ""
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
