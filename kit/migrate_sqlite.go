package kit

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sofired/grizzle/kit/introspect"
	pg "github.com/sofired/grizzle/schema/pg"
)

// ---------------------------------------------------------------------------
// SQLite variants of Push / DryRun / Migrate / Status
//
// These accept a standard *sql.DB connected to a SQLite database.
//
// Typical usage:
//
//	import _ "github.com/mattn/go-sqlite3"
//	db, _ := sql.Open("sqlite3", "./mydb.sqlite?_foreign_keys=on")
//	result, err := kit.MigrateSQLite(ctx, db, schema.Users, schema.Realms)
// ---------------------------------------------------------------------------

const createMigrationsTableSQLite = `
CREATE TABLE IF NOT EXISTS ` + MigrationsTable + ` (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    applied_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    checksum    TEXT    NOT NULL,
    sql_batch   TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT ''
)`

// PushSQLite inspects the live SQLite database, diffs it against the provided
// table definitions, and applies all necessary DDL changes.
//
// Note: SQLite does not support ALTER COLUMN (type, nullability, or default
// changes). Such changes will produce SQL comment stubs; apply them by
// rebuilding the affected table manually.
func PushSQLite(ctx context.Context, db *sql.DB, tables ...*pg.TableDef) (PushResult, error) {
	live, err := introspect.IntrospectSQLite(ctx, db)
	if err != nil {
		return PushResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := SQLiteApplyableChanges(Diff(current, target))
	if len(changes) == 0 {
		return PushResult{}, nil
	}
	stmts := AllChangeSQLSQLite(target, changes)
	if err := execTransactionSQLite(ctx, db, stmts); err != nil {
		return PushResult{Changes: changes, SQL: stmts}, fmt.Errorf("apply: %w", err)
	}
	return PushResult{Changes: changes, SQL: stmts}, nil
}

// DryRunSQLite is like PushSQLite but does not apply changes.
func DryRunSQLite(ctx context.Context, db *sql.DB, tables ...*pg.TableDef) (PushResult, error) {
	live, err := introspect.IntrospectSQLite(ctx, db)
	if err != nil {
		return PushResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := Diff(current, target)
	stmts := AllChangeSQLSQLite(target, changes)
	return PushResult{Changes: changes, SQL: stmts}, nil
}

// MigrateSQLite is like PushSQLite but records the applied SQL in the
// _grizzle_migrations history table.
func MigrateSQLite(ctx context.Context, db *sql.DB, tables ...*pg.TableDef) (MigrateResult, error) {
	if err := ensureMigrationsTableSQLite(ctx, db); err != nil {
		return MigrateResult{}, err
	}

	live, err := introspect.IntrospectSQLite(ctx, db)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := SQLiteApplyableChanges(Diff(current, target))

	if len(changes) == 0 {
		return MigrateResult{AlreadyCurrent: true}, nil
	}

	stmts := AllChangeSQLSQLite(target, changes)
	checksum := ChecksumSQL(stmts)
	desc := DescribeChanges(changes)

	if err := applyWithHistorySQLite(ctx, db, stmts, checksum, desc); err != nil {
		return MigrateResult{Changes: changes, SQL: stmts, Checksum: checksum},
			fmt.Errorf("apply: %w", err)
	}
	return MigrateResult{Changes: changes, SQL: stmts, Checksum: checksum}, nil
}

// StatusSQLite reports the applied migration history and any pending changes
// for a SQLite database without modifying it.
func StatusSQLite(ctx context.Context, db *sql.DB, tables ...*pg.TableDef) (StatusResult, error) {
	if err := ensureMigrationsTableSQLite(ctx, db); err != nil {
		return StatusResult{}, err
	}

	applied, err := loadHistorySQLite(ctx, db)
	if err != nil {
		return StatusResult{}, err
	}

	live, err := introspect.IntrospectSQLite(ctx, db)
	if err != nil {
		return StatusResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := SQLiteApplyableChanges(Diff(current, target))
	stmts := AllChangeSQLSQLite(target, changes)

	return StatusResult{Applied: applied, Pending: changes, SQL: stmts}, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func ensureMigrationsTableSQLite(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, createMigrationsTableSQLite); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}
	return nil
}

func applyWithHistorySQLite(ctx context.Context, db *sql.DB, stmts []string, checksum, desc string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	for _, stmt := range stmts {
		// Skip comment-only statements (ALTER COLUMN stubs).
		if strings.HasPrefix(strings.TrimSpace(stmt), "--") {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}

	const insertSQL = `INSERT INTO ` + MigrationsTable +
		` (checksum, sql_batch, description) VALUES (?, ?, ?)`
	if _, err := tx.ExecContext(ctx, insertSQL, checksum, strings.Join(stmts, "\n"), desc); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration: %w", err)
	}

	return tx.Commit()
}

func loadHistorySQLite(ctx context.Context, db *sql.DB) ([]MigrationRecord, error) {
	const q = `SELECT id, applied_at, checksum, sql_batch, description
	           FROM ` + MigrationsTable + ` ORDER BY id ASC`

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var records []MigrationRecord
	for rows.Next() {
		var r MigrationRecord
		// applied_at is stored as ISO8601 text; scan into string, parse manually.
		var appliedAt string
		if err := rows.Scan(&r.ID, &appliedAt, &r.Checksum, &r.SQLBatch, &r.Description); err != nil {
			return nil, fmt.Errorf("scan history row: %w", err)
		}
		// Parse flexible SQLite timestamp formats.
		ts, err := parseSQLiteTime(appliedAt)
		if err != nil {
			return nil, fmt.Errorf("parse applied_at %q: %w", appliedAt, err)
		}
		r.AppliedAt = ts
		records = append(records, r)
	}
	return records, rows.Err()
}

func execTransactionSQLite(ctx context.Context, db *sql.DB, stmts []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	for _, stmt := range stmts {
		if strings.HasPrefix(strings.TrimSpace(stmt), "--") {
			continue // skip comment stubs
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}
	return tx.Commit()
}

// parseSQLiteTime parses common SQLite timestamp string formats into time.Time.
func parseSQLiteTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05.999Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.999999Z",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999",
		"2006-01-02 15:04:05.999999",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised SQLite timestamp format: %q", s)
}
