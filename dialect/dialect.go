// Package dialect defines the SQL dialect interface and built-in implementations.
// All SQL generation in Grizzle is dialect-aware, allowing the same query builder
// to produce correct SQL for PostgreSQL, MySQL, and SQLite.
package dialect

import (
	"fmt"
	"strings"
)

// UpsertStyle describes how a dialect handles INSERT … ON CONFLICT.
type UpsertStyle string

const (
	// UpsertOnConflict is PostgreSQL style: ON CONFLICT (cols) DO UPDATE SET …
	UpsertOnConflict UpsertStyle = "on_conflict"
	// UpsertDuplicateKey is MySQL/MariaDB style: ON DUPLICATE KEY UPDATE …
	UpsertDuplicateKey UpsertStyle = "duplicate_key"
	// UpsertNone means upserts are not supported (syntax error).
	UpsertNone UpsertStyle = "none"
)

// Dialect handles the differences in SQL syntax between database engines.
type Dialect interface {
	// Placeholder returns the parameterized placeholder for the nth argument (1-indexed).
	// PostgreSQL: "$1", "$2", ...  MySQL/SQLite: "?", "?", ...
	Placeholder(n int) string

	// QuoteIdent wraps an identifier (table or column name) in the appropriate
	// quote characters, escaping any embedded quote characters.
	QuoteIdent(name string) string

	// Name returns a short identifier for the dialect ("postgres", "mysql", "sqlite").
	Name() string

	// SupportsReturning reports whether the dialect supports the RETURNING clause
	// on INSERT / UPDATE / DELETE statements (e.g. PostgreSQL, SQLite 3.35+) or
	// not (e.g. MySQL).
	SupportsReturning() bool

	// UpsertStyle returns the dialect's INSERT conflict-resolution style.
	UpsertStyle() UpsertStyle

	// InsertIgnoreClause returns the SQL keyword phrase that replaces "INSERT"
	// for an ignore-on-conflict insert, e.g. "INSERT IGNORE" (MySQL) or
	// "INSERT OR IGNORE" (SQLite). Returns "" for dialects that have no native
	// equivalent (PostgreSQL — use OnConflict…DoNothing instead).
	InsertIgnoreClause() string

	// SupportsCTE reports whether the dialect supports Common Table Expressions
	// (WITH clauses). True for PostgreSQL, MySQL 8.0+, and SQLite 3.8.3+.
	//
	// When false, the query builder silently drops all WITH clauses at build time.
	// Custom dialects targeting engines older than these versions should return false.
	SupportsCTE() bool

	// SupportsWindowFunctions reports whether the dialect supports window
	// functions (OVER clause). True for PostgreSQL, MySQL 8.0+, and SQLite 3.25+.
	//
	// When false, the query builder silently drops only the window function columns
	// from the SELECT list at build time; non-window columns are preserved as-is.
	// If every column in the SELECT list is a window function (i.e. no non-window
	// columns remain after dropping), the query falls back to SELECT *.
	SupportsWindowFunctions() bool

	// SupportsDistinctOn reports whether the dialect supports SELECT DISTINCT ON
	// (expr, ...). This is a PostgreSQL extension; MySQL and SQLite do not support it.
	//
	// When false, DistinctOn() degrades to regular SELECT DISTINCT at build time.
	// This is a semantic change: DISTINCT ON returns one row per distinct-on group
	// (using ORDER BY to pick which row), whereas DISTINCT deduplicates across all
	// selected columns. Query results will differ in most cases.
	SupportsDistinctOn() bool

	// SupportsForUpdate reports whether the dialect supports row-level locking.
	// True for PostgreSQL and MySQL; false for SQLite, which uses file-level
	// locking only.
	//
	// Note: the shared-lock syntax differs by dialect — PostgreSQL uses FOR SHARE
	// while MySQL uses LOCK IN SHARE MODE (see ForShareClause). The query builder
	// gates all row-level locking clauses on this flag; when false, locking clauses
	// are silently dropped from the output SQL.
	SupportsForUpdate() bool

	// SupportsForNoKeyUpdate reports whether the dialect supports the
	// PostgreSQL-specific FOR NO KEY UPDATE and FOR KEY SHARE locking modes.
	// False for MySQL and SQLite.
	//
	// The query builder gates ForNoKeyUpdate() and ForKeyShare() on this flag.
	SupportsForNoKeyUpdate() bool

	// SupportsFullJoin reports whether the dialect supports FULL [OUTER] JOIN.
	// True for PostgreSQL; false for MySQL and SQLite.
	//
	// When false, the query builder silently drops FULL JOIN clauses at build time.
	// This is a semantic change: rows that would have been included via the outer
	// side of the join are omitted entirely from the result set.
	SupportsFullJoin() bool

	// ForShareClause returns the SQL keyword phrase for a shared row lock.
	// PostgreSQL: "FOR SHARE". MySQL: "LOCK IN SHARE MODE".
	// Returns "" for dialects that do not support row-level locking (e.g. SQLite).
	// A non-empty value is only returned when SupportsForUpdate() is true.
	ForShareClause() string

	// SupportsForShareOf reports whether the dialect supports an OF table list
	// on the shared-lock clause (FOR SHARE … OF / LOCK IN SHARE MODE … OF).
	// True for PostgreSQL (FOR SHARE OF …); false for MySQL (LOCK IN SHARE MODE
	// does not accept an OF clause) and SQLite (no row-level locking at all).
	SupportsForShareOf() bool
}

// -------------------------------------------------------------------
// PostgreSQL
// -------------------------------------------------------------------

// PostgresDialect generates ANSI-compatible SQL with PostgreSQL extensions.
var Postgres Dialect = postgresDialect{}

type postgresDialect struct{}

func (postgresDialect) Name() string                  { return "postgres" }
func (postgresDialect) SupportsReturning() bool       { return true }
func (postgresDialect) UpsertStyle() UpsertStyle      { return UpsertOnConflict }
func (postgresDialect) InsertIgnoreClause() string    { return "" } // use ON CONFLICT … DO NOTHING
func (postgresDialect) SupportsCTE() bool             { return true }
func (postgresDialect) SupportsWindowFunctions() bool { return true }
func (postgresDialect) SupportsDistinctOn() bool      { return true }
func (postgresDialect) SupportsForUpdate() bool       { return true }
func (postgresDialect) SupportsForNoKeyUpdate() bool  { return true }
func (postgresDialect) SupportsFullJoin() bool        { return true }
func (postgresDialect) ForShareClause() string        { return "FOR SHARE" }
func (postgresDialect) SupportsForShareOf() bool      { return true }

func (postgresDialect) Placeholder(n int) string {
	return fmt.Sprintf("$%d", n)
}

func (postgresDialect) QuoteIdent(name string) string {
	// Escape embedded double-quotes by doubling them.
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// -------------------------------------------------------------------
// MySQL / MariaDB
// -------------------------------------------------------------------

// MySQLDialect generates MySQL-compatible SQL.
var MySQL Dialect = mysqlDialect{}

type mysqlDialect struct{}

func (mysqlDialect) Name() string                  { return "mysql" }
func (mysqlDialect) SupportsReturning() bool       { return false }
func (mysqlDialect) UpsertStyle() UpsertStyle      { return UpsertDuplicateKey }
func (mysqlDialect) InsertIgnoreClause() string    { return "INSERT IGNORE" }
func (mysqlDialect) SupportsCTE() bool             { return true } // MySQL 8.0+
func (mysqlDialect) SupportsWindowFunctions() bool { return true } // MySQL 8.0+
func (mysqlDialect) SupportsDistinctOn() bool      { return false }
func (mysqlDialect) SupportsForUpdate() bool       { return true }
func (mysqlDialect) SupportsForNoKeyUpdate() bool  { return false }
func (mysqlDialect) SupportsFullJoin() bool        { return false }
func (mysqlDialect) ForShareClause() string        { return "LOCK IN SHARE MODE" }
func (mysqlDialect) SupportsForShareOf() bool      { return false }

func (mysqlDialect) Placeholder(_ int) string { return "?" }

func (mysqlDialect) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// -------------------------------------------------------------------
// SQLite
// -------------------------------------------------------------------

// SQLiteDialect generates SQLite-compatible SQL.
var SQLite Dialect = sqliteDialect{}

type sqliteDialect struct{}

func (sqliteDialect) Name() string                  { return "sqlite" }
func (sqliteDialect) SupportsReturning() bool       { return true } // SQLite 3.35+
func (sqliteDialect) UpsertStyle() UpsertStyle      { return UpsertOnConflict }
func (sqliteDialect) InsertIgnoreClause() string    { return "INSERT OR IGNORE" }
func (sqliteDialect) SupportsCTE() bool             { return true } // SQLite 3.8.3+
func (sqliteDialect) SupportsWindowFunctions() bool { return true } // SQLite 3.25+
func (sqliteDialect) SupportsDistinctOn() bool      { return false }
func (sqliteDialect) SupportsForUpdate() bool       { return false }
func (sqliteDialect) SupportsForNoKeyUpdate() bool  { return false }
func (sqliteDialect) SupportsFullJoin() bool        { return false }
func (sqliteDialect) ForShareClause() string        { return "" }
func (sqliteDialect) SupportsForShareOf() bool      { return false }

func (sqliteDialect) Placeholder(_ int) string { return "?" }

func (sqliteDialect) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
