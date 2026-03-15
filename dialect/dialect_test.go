package dialect_test

import (
	"testing"

	"github.com/sofired/grizzle/dialect"
)

// TestDialectFeatureMatrix verifies that each dialect reports the correct
// capabilities for all feature-detection methods added in #155.
func TestDialectFeatureMatrix(t *testing.T) {
	type row struct {
		name               string
		d                  dialect.Dialect
		supportsCTE        bool
		supportsWindow     bool
		supportsDistinctOn bool
		supportsForUpdate  bool
		supportsForNoKey   bool
		supportsFullJoin   bool
		forShareClause     string
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
			forShareClause:     "FOR SHARE",
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
			forShareClause:     "LOCK IN SHARE MODE",
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
			forShareClause:     "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkBool := func(method string, got, want bool) {
				t.Helper()
				if got != want {
					t.Errorf("%s.%s() = %v, want %v", c.name, method, got, want)
				}
			}
			checkStr := func(method, got, want string) {
				t.Helper()
				if got != want {
					t.Errorf("%s.%s() = %q, want %q", c.name, method, got, want)
				}
			}
			checkBool("SupportsCTE", c.d.SupportsCTE(), c.supportsCTE)
			checkBool("SupportsWindowFunctions", c.d.SupportsWindowFunctions(), c.supportsWindow)
			checkBool("SupportsDistinctOn", c.d.SupportsDistinctOn(), c.supportsDistinctOn)
			checkBool("SupportsForUpdate", c.d.SupportsForUpdate(), c.supportsForUpdate)
			checkBool("SupportsForNoKeyUpdate", c.d.SupportsForNoKeyUpdate(), c.supportsForNoKey)
			checkBool("SupportsFullJoin", c.d.SupportsFullJoin(), c.supportsFullJoin)
			checkStr("ForShareClause", c.d.ForShareClause(), c.forShareClause)
		})
	}
}
