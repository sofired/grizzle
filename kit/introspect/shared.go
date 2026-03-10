package introspect

import (
	"strings"

	pg "github.com/sofired/grizzle/schema/pg"
)

// normalizeFKAction converts a referential action string (as reported by MySQL,
// SQLite, and most databases) to a pg.FKAction constant.
func normalizeFKAction(action string) string {
	switch strings.ToUpper(action) {
	case "CASCADE":
		return string(pg.FKActionCascade)
	case "SET NULL":
		return string(pg.FKActionSetNull)
	case "SET DEFAULT":
		return string(pg.FKActionSetDefault)
	case "RESTRICT":
		return string(pg.FKActionRestrict)
	default:
		return string(pg.FKActionNoAction)
	}
}
