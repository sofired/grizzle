package kit

import (
	"context"
	"database/sql"
	"fmt"
	"os"
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
//	result, err := kit.MigrateMySQL(ctx, db, kit.MigrateOptions{MigrationsDir: "./migrations"})
// ---------------------------------------------------------------------------

const createMigrationsTableMySQL = `
CREATE TABLE IF NOT EXISTS ` + MigrationsTable + ` (
    id           INT AUTO_INCREMENT PRIMARY KEY,
    applied_at   DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    tag          VARCHAR(255)    NOT NULL DEFAULT '',
    checksum     VARCHAR(64)     NOT NULL DEFAULT '',
    sql_batch    LONGTEXT        NOT NULL DEFAULT '',
    description  TEXT            NOT NULL DEFAULT '',
    is_baseline  TINYINT(1)      NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`

// PushMySQL inspects the live MySQL database, diffs it against the provided
// table definitions, and applies all necessary DDL changes in a transaction.
// It accepts tables from any dialect via the TableDefiner interface.
func PushMySQL(ctx context.Context, db *sql.DB, tables ...pg.TableDefiner) (PushResult, error) {
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
// It accepts tables from any dialect via the TableDefiner interface.
func DryRunMySQL(ctx context.Context, db *sql.DB, tables ...pg.TableDefiner) (PushResult, error) {
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

// MigrateMySQL reads pending .sql files from opts.MigrationsDir and applies
// them in sequence order, recording each in _grizzle_migrations.
// It accepts a *sql.DB connected to a MySQL database.
func MigrateMySQL(ctx context.Context, db *sql.DB, opts MigrateOptions) (MigrateResult, error) {
	if opts.MigrationsDir == "" {
		return MigrateResult{}, fmt.Errorf("MigrateOptions.MigrationsDir must be set")
	}

	// Validate baseline tag before any DB operation.
	if opts.Baseline != "" {
		if err := ValidateTag(opts.Baseline); err != nil {
			return MigrateResult{}, fmt.Errorf("--baseline: %w", err)
		}
	}

	if err := ensureMigrationsTableMySQL(ctx, db, opts.SkipSchemaUpgrade); err != nil {
		return MigrateResult{}, err
	}

	files, err := LoadMigrationFiles(opts.MigrationsDir)
	if err != nil {
		return MigrateResult{}, err
	}

	if len(files) == 0 {
		return MigrateResult{AlreadyCurrent: true}, nil
	}

	applied, err := loadAppliedTagsMySQL(ctx, db)
	if err != nil {
		return MigrateResult{}, err
	}

	var result MigrateResult

	var baselineSeq int
	if opts.Baseline != "" {
		var err error
		baselineSeq, err = parseSequenceNumber(opts.Baseline)
		if err != nil {
			return MigrateResult{}, fmt.Errorf("--baseline %q: %w", opts.Baseline, err)
		}
	}

	if opts.Baseline != "" {
		found := false
		for _, f := range files {
			if f.Tag == opts.Baseline {
				found = true
				break
			}
		}
		if !found {
			return MigrateResult{}, fmt.Errorf("--baseline %q does not correspond to any file in %s", opts.Baseline, opts.MigrationsDir)
		}

		var toBaseline []baselineRecord
		for _, f := range files {
			if f.SeqNum <= baselineSeq && !applied[f.Tag] {
				cs, err := ChecksumFile(f.Path)
				if err != nil {
					return MigrateResult{}, fmt.Errorf("checksum %s: %w", f.FileName, err)
				}
				toBaseline = append(toBaseline, baselineRecord{File: f, Checksum: cs})
			}
		}

		if len(toBaseline) > 0 {
			if err := insertBaselineMySQL(ctx, db, toBaseline); err != nil {
				return MigrateResult{}, fmt.Errorf("baseline: %w", err)
			}
			for _, rec := range toBaseline {
				result.Baselined = append(result.Baselined, rec.File)
				applied[rec.File.Tag] = true
				fmt.Fprintf(os.Stderr, "grizzle: baseline %s  sha256:%s\n", rec.File.FileName, rec.Checksum)
			}
		}
	}

	var toApply []MigrationFile
	for _, f := range files {
		if !applied[f.Tag] {
			if opts.Baseline != "" && f.SeqNum <= baselineSeq {
				continue
			}
			toApply = append(toApply, f)
		}
	}

	if len(toApply) == 0 {
		if len(result.Baselined) == 0 {
			result.AlreadyCurrent = true
		}
		return result, nil
	}

	for _, f := range toApply {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return result, fmt.Errorf("read %s: %w", f.FileName, err)
		}
		cs := checksumBytes(data)
		sqlText := string(data)

		if err := applyMigrationFileMySQL(ctx, db, f.Tag, sqlText, cs); err != nil {
			return result, fmt.Errorf("apply %s: %w", f.FileName, err)
		}
		result.Applied = append(result.Applied, f)
	}

	return result, nil
}

// StatusMySQL reports the applied migration history and any pending migration
// files for a MySQL database without modifying it.
func StatusMySQL(ctx context.Context, db *sql.DB, opts MigrateOptions) (StatusResult, error) {
	if opts.MigrationsDir == "" {
		return StatusResult{}, fmt.Errorf("MigrateOptions.MigrationsDir must be set")
	}

	if err := ensureMigrationsTableMySQL(ctx, db, opts.SkipSchemaUpgrade); err != nil {
		return StatusResult{}, err
	}

	history, err := loadHistoryMySQL(ctx, db)
	if err != nil {
		return StatusResult{}, err
	}

	files, err := LoadMigrationFiles(opts.MigrationsDir)
	if err != nil {
		return StatusResult{}, err
	}

	appliedSet := make(map[string]bool, len(history))
	for _, r := range history {
		if r.Tag != "" {
			appliedSet[r.Tag] = true
		}
	}

	var pending []MigrationFile
	for _, f := range files {
		if !appliedSet[f.Tag] {
			pending = append(pending, f)
		}
	}

	return StatusResult{Applied: history, Pending: pending}, nil
}

// LoadHistoryMySQL returns all rows from _grizzle_migrations for MySQL.
func LoadHistoryMySQL(ctx context.Context, db *sql.DB) ([]MigrationRecord, error) {
	if err := ensureMigrationsTableMySQL(ctx, db, false); err != nil {
		return nil, err
	}
	return loadHistoryMySQL(ctx, db)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func ensureMigrationsTableMySQL(ctx context.Context, db *sql.DB, skipUpgrade bool) error {
	if _, err := db.ExecContext(ctx, createMigrationsTableMySQL); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	if skipUpgrade {
		return verifyColumnsMySQLCtx(ctx, db)
	}
	return upgradeSchemaMySQL(ctx, db)
}

// upgradeSchemaMySQL adds tag and is_baseline columns if absent (MySQL 8.0+
// does not support ADD COLUMN IF NOT EXISTS, so we check via INFORMATION_SCHEMA).
func upgradeSchemaMySQL(ctx context.Context, db *sql.DB) error {
	existing, err := columnExistenceMySQL(ctx, db, []string{"tag", "is_baseline"})
	if err != nil {
		return err
	}

	if !existing["tag"] {
		const q = `ALTER TABLE ` + MigrationsTable + ` ADD COLUMN tag VARCHAR(255) NOT NULL DEFAULT ''`
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("add tag column: %w", err)
		}
	}
	if !existing["is_baseline"] {
		const q = `ALTER TABLE ` + MigrationsTable + ` ADD COLUMN is_baseline TINYINT(1) NOT NULL DEFAULT 0`
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("add is_baseline column: %w", err)
		}
	}
	return nil
}

func verifyColumnsMySQLCtx(ctx context.Context, db *sql.DB) error {
	existing, err := columnExistenceMySQL(ctx, db, []string{"tag", "is_baseline"})
	if err != nil {
		return err
	}
	if !existing["tag"] || !existing["is_baseline"] {
		return fmt.Errorf("_grizzle_migrations is missing required columns (tag, is_baseline); run without --skip-schema-upgrade to upgrade automatically")
	}
	return nil
}

// columnExistenceMySQL checks which of the given columns exist in the
// _grizzle_migrations table using INFORMATION_SCHEMA.
func columnExistenceMySQL(ctx context.Context, db *sql.DB, cols []string) (map[string]bool, error) {
	// Build a parameterized query for the column list.
	placeholders := make([]string, len(cols))
	args := make([]interface{}, len(cols)+1)
	args[0] = MigrationsTable
	for i, c := range cols {
		placeholders[i] = "?"
		args[i+1] = c
	}
	q := `SELECT column_name FROM information_schema.columns
	      WHERE table_schema = DATABASE()
	        AND table_name = ?
	        AND column_name IN (` + strings.Join(placeholders, ", ") + `)`

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("check column existence: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result[name] = true
	}
	return result, rows.Err()
}

// applyMigrationFileMySQL executes a migration file and records it in MySQL.
func applyMigrationFileMySQL(ctx context.Context, db *sql.DB, tag, sqlText, checksum string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	stmts := splitSQLStatements(sqlText)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec statement: %w", err)
		}
	}

	const insertSQL = `INSERT INTO ` + MigrationsTable +
		` (tag, checksum, sql_batch, is_baseline) VALUES (?, ?, ?, 0)`
	if _, err := tx.ExecContext(ctx, insertSQL, tag, checksum, sqlText); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration: %w", err)
	}

	return tx.Commit()
}

// insertBaselineMySQL inserts baseline records in a single transaction.
func insertBaselineMySQL(ctx context.Context, db *sql.DB, records []baselineRecord) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	const insertSQL = `INSERT INTO ` + MigrationsTable +
		` (tag, checksum, sql_batch, is_baseline) VALUES (?, ?, '', 1)`

	for _, rec := range records {
		if _, err := tx.ExecContext(ctx, insertSQL, rec.File.Tag, rec.Checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert baseline record for %s: %w", rec.File.Tag, err)
		}
	}

	return tx.Commit()
}

func loadAppliedTagsMySQL(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	const q = `SELECT tag FROM ` + MigrationsTable + ` WHERE tag != ''`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("load applied tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tags := make(map[string]bool)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags[tag] = true
	}
	return tags, rows.Err()
}

// loadHistoryMySQL reads all rows from _grizzle_migrations in chronological order.
func loadHistoryMySQL(ctx context.Context, db *sql.DB) ([]MigrationRecord, error) {
	const q = `SELECT id, applied_at, tag, checksum, sql_batch, description, is_baseline
	           FROM ` + MigrationsTable + ` ORDER BY id ASC`

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []MigrationRecord
	for rows.Next() {
		var r MigrationRecord
		var isBaselineInt int
		if err := rows.Scan(&r.ID, &r.AppliedAt, &r.Tag, &r.Checksum, &r.SQLBatch, &r.Description, &isBaselineInt); err != nil {
			return nil, fmt.Errorf("scan history row: %w", err)
		}
		r.IsBaseline = isBaselineInt != 0
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
