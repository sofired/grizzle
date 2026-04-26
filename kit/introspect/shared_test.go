package introspect

import (
	"testing"

	pg "github.com/sofired/grizzle/schema/pg"
)

func TestNormalizeFKAction(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"CASCADE", string(pg.FKActionCascade)},
		{"cascade", string(pg.FKActionCascade)},
		{"NO ACTION", string(pg.FKActionNoAction)},
		{"no action", string(pg.FKActionNoAction)},
		{"RESTRICT", string(pg.FKActionRestrict)},
		{"SET NULL", string(pg.FKActionSetNull)},
		{"SET DEFAULT", string(pg.FKActionSetDefault)},
		{"", string(pg.FKActionNoAction)},
		// Defensive: unknown values fall back to NO ACTION rather than
		// propagating an invalid action string into the diff engine.
		{"UNKNOWN_VALUE", string(pg.FKActionNoAction)},
	}
	for _, tt := range tests {
		got := normalizeFKAction(tt.input)
		if got != tt.want {
			t.Errorf("normalizeFKAction(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
