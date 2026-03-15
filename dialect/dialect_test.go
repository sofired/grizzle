package dialect_test

import (
	"testing"

	"github.com/sofired/grizzle/dialect"
)

// TestDialectFeatureMatrix verifies that each dialect reports the correct
// capabilities for all feature-detection methods added in #155.
func TestDialectFeatureMatrix(t *testing.T) {
	type row struct {
		name                string
		d                   dialect.Dialect
		supportsCTE         bool
		supportsWindow      bool
		supportsDistinctOn  bool
		supportsForUpdate   bool
		supportsForNoKey    bool
		supportsFullJoin    bool
	}

	cases := []row{
		{
			name:               "postgres",
			d:                  dialect.Postgres,
			supportsCTE:        true,
			supportsWindow:     true,
			supportsDistinctOn: true,
			supportsForUpdate:  true,
			supportsForNoKey:   true,
			supportsFullJoin:   true,
		},
		{
			name:               "mysql",
			d:                  dialect.MySQL,
			supportsCTE:        true,
			supportsWindow:     true,
			supportsDistinctOn: false,
			supportsForUpdate:  true,
			supportsForNoKey:   false,
			supportsFullJoin:   false,
		},
		{
			name:               "sqlite",
			d:                  dialect.SQLite,
			supportsCTE:        true,
			supportsWindow:     true,
			supportsDistinctOn: false,
			supportsForUpdate:  false,
			supportsForNoKey:   false,
			supportsFullJoin:   false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check := func(method string, got, want bool) {
				t.Helper()
				if got != want {
					t.Errorf("%s.%s() = %v, want %v", c.name, method, got, want)
				}
			}
			check("SupportsCTE", c.d.SupportsCTE(), c.supportsCTE)
			check("SupportsWindowFunctions", c.d.SupportsWindowFunctions(), c.supportsWindow)
			check("SupportsDistinctOn", c.d.SupportsDistinctOn(), c.supportsDistinctOn)
			check("SupportsForUpdate", c.d.SupportsForUpdate(), c.supportsForUpdate)
			check("SupportsForNoKeyUpdate", c.d.SupportsForNoKeyUpdate(), c.supportsForNoKey)
			check("SupportsFullJoin", c.d.SupportsFullJoin(), c.supportsFullJoin)
		})
	}
}
