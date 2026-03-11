package kit

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sofired/grizzle/kit/introspect"
	pg "github.com/sofired/grizzle/schema/pg"
)

// ---------------------------------------------------------------------------
// MySQL variants of Push / DryRun / Migrate / Status
//
// These accept a standard *sql.DB (from database/sql) rather than a pgxpool.Pool.
// They use IntrospectMySQL for live schema discovery and AllChangeSQLMySQL for
// DDL generation.
//
// Typical usage:
//
//	import (
//	    _ "github.com/go-sql-driver/mysql"
//	    "github.com/sofired/grizzle/kit"
//	)
//	db, _ := sql.Open("mysql", "user:pass@tcp(host:3306)/mydb?parseTime=true")
//	result, err := kit.MigrateMySQL(ctx, db, schema.Users, schema.Realms)
// ---------------------------------------------------------------------------

const createMigrationsTableMySQL = `
CREATE TABLE IF NOT EXISTS ` + MigrationsTable + ` (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    applied_at  DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    checksum    VARCHAR(64)  NOT NULL,
    sql_batch   LONGTEXT     NOT NULL,
    description TEXT         NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// PushMySQL inspects the live MySQL database, diffs it against the provided
// table definitions, and applies all necessary DDL changes in a transaction.
func PushMySQL(ctx context.Context, db *sql.DB, tables ...*pg.TableDef) (PushResult, error) {
	live, err := introspect.IntrospectMySQL(ctx, db)
	if err != nil {
		return PushResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := Diff(current, target)
	if len(changes) == 0 {
		return PushResult{}, nil
	}
	stmts := AllChangeSQLMySQL(target, changes)
	if err := execTransactionMySQL(ctx, db, stmts); err != nil {
		return PushResult{Changes: changes, SQL: stmts}, fmt.Errorf("apply: %w", err)
	}
	return PushResult{Changes: changes, SQL: stmts}, nil
}

// DryRunMySQL is like PushMySQL but does not apply changes — it only computes
// and returns what would be run.
func DryRunMySQL(ctx context.Context, db *sql.DB, tables ...*pg.TableDef) (PushResult, error) {
	live, err := introspect.IntrospectMySQL(ctx, db)
	if err != nil {
		return PushResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := Diff(current, target)
	stmts := AllChangeSQLMySQL(target, changes)
	return PushResult{Changes: changes, SQL: stmts}, nil
}

// MigrateMySQL is like PushMySQL but records the applied SQL in the
// _grizzle_migrations history table. Calling MigrateMySQL twice with an
// unchanged schema is a no-op.
func MigrateMySQL(ctx context.Context, db *sql.DB, tables ...*pg.TableDef) (MigrateResult, error) {
	if err := ensureMigrationsTableMySQL(ctx, db); err != nil {
		return MigrateResult{}, err
	}

	live, err := introspect.IntrospectMySQL(ctx, db)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := Diff(current, target)

	if len(changes) == 0 {
		return MigrateResult{AlreadyCurrent: true}, nil
	}

	stmts := AllChangeSQLMySQL(target, changes)
	checksum := ChecksumSQL(stmts)
	desc := DescribeChanges(changes)

	if err := applyWithHistoryMySQL(ctx, db, stmts, checksum, desc); err != nil {
		return MigrateResult{Changes: changes, SQL: stmts, Checksum: checksum},
			fmt.Errorf("apply: %w", err)
	}
	return MigrateResult{Changes: changes, SQL: stmts, Checksum: checksum}, nil
}

// StatusMySQL reports the applied migration history and any pending changes for
// a MySQL database without modifying it.
func StatusMySQL(ctx context.Context, db *sql.DB, tables ...*pg.TableDef) (StatusResult, error) {
	if err := ensureMigrationsTableMySQL(ctx, db); err != nil {
		return StatusResult{}, err
	}

	applied, err := loadHistoryMySQL(ctx, db)
	if err != nil {
		return StatusResult{}, err
	}

	live, err := introspect.IntrospectMySQL(ctx, db)
	if err != nil {
		return StatusResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := Diff(current, target)
	stmts := AllChangeSQLMySQL(target, changes)

	return StatusResult{Applied: applied, Pending: changes, SQL: stmts}, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func ensureMigrationsTableMySQL(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, createMigrationsTableMySQL); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}
	return nil
}

// applyWithHistoryMySQL runs DDL statements and inserts a migration history
// record inside a single transaction.
func applyWithHistoryMySQL(ctx context.Context, db *sql.DB, stmts []string, checksum, desc string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	for _, stmt := range stmts {
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

// loadHistoryMySQL reads all rows from _grizzle_migrations in chronological order.
func loadHistoryMySQL(ctx context.Context, db *sql.DB) ([]MigrationRecord, error) {
	const q = `SELECT id, applied_at, checksum, sql_batch, description
	           FROM ` + MigrationsTable + ` ORDER BY id ASC`

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []MigrationRecord
	for rows.Next() {
		var r MigrationRecord
		if err := rows.Scan(&r.ID, &r.AppliedAt, &r.Checksum, &r.SQLBatch, &r.Description); err != nil {
			return nil, fmt.Errorf("scan history row: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// execTransactionMySQL runs all statements inside a single database/sql transaction.
func execTransactionMySQL(ctx context.Context, db *sql.DB, stmts []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}
	return tx.Commit()
}
