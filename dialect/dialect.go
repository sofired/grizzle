// Package dialect defines the SQL dialect interface and built-in implementations.
// All SQL generation in G-rizzle is dialect-aware, allowing the same query builder
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
	// on INSERT / UPDATE / DELETE statements (PostgreSQL) or not (MySQL, SQLite).
	SupportsReturning() bool

	// UpsertStyle returns the dialect's INSERT conflict-resolution style.
	UpsertStyle() UpsertStyle
}

// -------------------------------------------------------------------
// PostgreSQL
// -------------------------------------------------------------------

// PostgresDialect generates ANSI-compatible SQL with PostgreSQL extensions.
var Postgres Dialect = postgresDialect{}

type postgresDialect struct{}

func (postgresDialect) Name() string              { return "postgres" }
func (postgresDialect) SupportsReturning() bool   { return true }
func (postgresDialect) UpsertStyle() UpsertStyle  { return UpsertOnConflict }

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

func (mysqlDialect) Name() string             { return "mysql" }
func (mysqlDialect) SupportsReturning() bool  { return false }
func (mysqlDialect) UpsertStyle() UpsertStyle { return UpsertDuplicateKey }

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

func (sqliteDialect) Name() string             { return "sqlite" }
func (sqliteDialect) SupportsReturning() bool  { return true } // SQLite 3.35+
func (sqliteDialect) UpsertStyle() UpsertStyle { return UpsertOnConflict }

func (sqliteDialect) Placeholder(_ int) string { return "?" }

func (sqliteDialect) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
