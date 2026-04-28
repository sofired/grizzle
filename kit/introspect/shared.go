package introspect

import (
	"strings"

	pg "github.com/sofired/grizzle/schema/pg"
)

// normalizeFKAction converts a referential action string to a pg.FKAction
// constant. PostgreSQL information_schema, MySQL, and SQLite all use the same
// uppercase action names (CASCADE, RESTRICT, SET NULL, SET DEFAULT, NO ACTION).
// Unknown values fall back to FKActionNoAction as a safe default.
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
