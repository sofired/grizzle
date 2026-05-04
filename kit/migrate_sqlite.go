package kit

import (
	"context"
	"database/sql"
	"fmt"
	"os"
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
//	result, err := kit.MigrateSQLite(ctx, db, kit.MigrateOptions{MigrationsDir: "./migrations"})
// ---------------------------------------------------------------------------

const createMigrationsTableSQLite = `
CREATE TABLE IF NOT EXISTS ` + MigrationsTable + ` (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    applied_at   TEXT     NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    tag          TEXT     NOT NULL DEFAULT '',
    checksum     TEXT     NOT NULL DEFAULT '',
    sql_batch    TEXT     NOT NULL DEFAULT '',
    description  TEXT     NOT NULL DEFAULT '',
    is_baseline  INTEGER  NOT NULL DEFAULT 0
)`

// PushSQLite inspects the live SQLite database, diffs it against the provided
// table definitions, and applies all necessary DDL changes.
// It accepts tables from any dialect via the TableDefiner interface.
//
// Note: SQLite does not support ALTER COLUMN (type, nullability, or default
// changes). Such changes will produce SQL comment stubs; apply them by
// rebuilding the affected table manually.
func PushSQLite(ctx context.Context, db *sql.DB, tables ...pg.TableDefiner) (PushResult, error) {
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
// It accepts tables from any dialect via the TableDefiner interface.
func DryRunSQLite(ctx context.Context, db *sql.DB, tables ...pg.TableDefiner) (PushResult, error) {
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

// MigrateSQLite reads pending .sql files from opts.MigrationsDir and applies
// them in sequence order, recording each in _grizzle_migrations.
// It accepts a *sql.DB connected to a SQLite database.
func MigrateSQLite(ctx context.Context, db *sql.DB, opts MigrateOptions) (MigrateResult, error) {
	if opts.MigrationsDir == "" {
		return MigrateResult{}, fmt.Errorf("MigrateOptions.MigrationsDir must be set")
	}

	if opts.Baseline != "" {
		if err := ValidateTag(opts.Baseline); err != nil {
			return MigrateResult{}, fmt.Errorf("--baseline: %w", err)
		}
	}

	if err := ensureMigrationsTableSQLite(ctx, db, opts.SkipSchemaUpgrade); err != nil {
		return MigrateResult{}, err
	}

	files, err := LoadMigrationFiles(opts.MigrationsDir)
	if err != nil {
		return MigrateResult{}, err
	}

	if len(files) == 0 {
		return MigrateResult{AlreadyCurrent: true}, nil
	}

	applied, err := loadAppliedTagsSQLite(ctx, db)
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
			if err := insertBaselineSQLite(ctx, db, toBaseline); err != nil {
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

		if err := applyMigrationFileSQLite(ctx, db, f.Tag, sqlText, cs); err != nil {
			return result, fmt.Errorf("apply %s: %w", f.FileName, err)
		}
		result.Applied = append(result.Applied, f)
	}

	return result, nil
}

// StatusSQLite reports the applied migration history and any pending migration
// files for a SQLite database without modifying it.
func StatusSQLite(ctx context.Context, db *sql.DB, opts MigrateOptions) (StatusResult, error) {
	if opts.MigrationsDir == "" {
		return StatusResult{}, fmt.Errorf("MigrateOptions.MigrationsDir must be set")
	}

	if err := ensureMigrationsTableSQLite(ctx, db, opts.SkipSchemaUpgrade); err != nil {
		return StatusResult{}, err
	}

	history, err := loadHistorySQLite(ctx, db)
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

// LoadHistorySQLite returns all rows from _grizzle_migrations for SQLite.
func LoadHistorySQLite(ctx context.Context, db *sql.DB) ([]MigrationRecord, error) {
	if err := ensureMigrationsTableSQLite(ctx, db, false); err != nil {
		return nil, err
	}
	return loadHistorySQLite(ctx, db)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func ensureMigrationsTableSQLite(ctx context.Context, db *sql.DB, skipUpgrade bool) error {
	if _, err := db.ExecContext(ctx, createMigrationsTableSQLite); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	if skipUpgrade {
		return verifyColumnsSQLite(ctx, db)
	}
	return upgradeSchemaSQLite(ctx, db)
}

// upgradeSchemaSQLite adds tag and is_baseline columns if absent.
// SQLite does not support ADD COLUMN IF NOT EXISTS, so we check via PRAGMA.
func upgradeSchemaSQLite(ctx context.Context, db *sql.DB) error {
	existing, err := columnExistenceSQLite(ctx, db)
	if err != nil {
		return err
	}

	if !existing["tag"] {
		const q = `ALTER TABLE ` + MigrationsTable + ` ADD COLUMN tag TEXT NOT NULL DEFAULT ''`
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("add tag column: %w", err)
		}
	}
	if !existing["is_baseline"] {
		const q = `ALTER TABLE ` + MigrationsTable + ` ADD COLUMN is_baseline INTEGER NOT NULL DEFAULT 0`
		if _, err := db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("add is_baseline column: %w", err)
		}
	}
	return nil
}

func verifyColumnsSQLite(ctx context.Context, db *sql.DB) error {
	existing, err := columnExistenceSQLite(ctx, db)
	if err != nil {
		return err
	}
	if !existing["tag"] || !existing["is_baseline"] {
		return fmt.Errorf("_grizzle_migrations is missing required columns (tag, is_baseline); run without --skip-schema-upgrade to upgrade automatically")
	}
	return nil
}

// columnExistenceSQLite checks which columns exist in _grizzle_migrations
// using PRAGMA table_info.
func columnExistenceSQLite(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info("`+MigrationsTable+`")`)
	if err != nil {
		return nil, fmt.Errorf("pragma table_info: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		result[name] = true
	}
	return result, rows.Err()
}

// applyMigrationFileSQLite executes a migration file and records it in SQLite.
func applyMigrationFileSQLite(ctx context.Context, db *sql.DB, tag, sqlText, checksum string) error {
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
		// Strip leading comment lines so migration files that open with a
		// descriptive comment block are not silently skipped. Fragments that
		// contain only comment lines (e.g. ALTER COLUMN stubs from the push
		// workflow) are still discarded after stripping.
		stmt = stripLeadingCommentLines(stmt)
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

// insertBaselineSQLite inserts baseline records in a single transaction.
// Checksums must be pre-computed by the caller.
func insertBaselineSQLite(ctx context.Context, db *sql.DB, records []baselineRecord) error {
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

func loadAppliedTagsSQLite(ctx context.Context, db *sql.DB) (map[string]bool, error) {
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

func loadHistorySQLite(ctx context.Context, db *sql.DB) ([]MigrationRecord, error) {
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
		var appliedAt string
		var isBaselineInt int
		if err := rows.Scan(&r.ID, &appliedAt, &r.Tag, &r.Checksum, &r.SQLBatch, &r.Description, &isBaselineInt); err != nil {
			return nil, fmt.Errorf("scan history row: %w", err)
		}
		ts, err := parseSQLiteTime(appliedAt)
		if err != nil {
			return nil, fmt.Errorf("parse applied_at %q: %w", appliedAt, err)
		}
		r.AppliedAt = ts
		r.IsBaseline = isBaselineInt != 0
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
		// DDL generated by AllChangeSQLSQLite emits pure comment strings
		// (e.g. "-- ALTER COLUMN ... not supported") as stubs. Unlike user
		// migration files, these are never "comment header + real SQL", so
		// a simple HasPrefix check is correct and sufficient here.
		if strings.HasPrefix(strings.TrimSpace(stmt), "--") {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}
	return tx.Commit()
}

// stripLeadingCommentLines removes leading blank lines and SQL line-comment
// lines (those whose first non-whitespace characters are "--") from s.
// Only "--" line comments are stripped; "/* ... */" block comments are left
// intact because they are valid SQL and do not cause the HasPrefix skip.
// The primary use case is user-authored migration files that open with a
// descriptive "-- ..." header; fragments containing only line comments
// (e.g. ALTER COLUMN stubs emitted by the push workflow) resolve to "" and
// are skipped by the caller.
func stripLeadingCommentLines(s string) string {
	lines := strings.Split(s, "\n")
	for len(lines) > 0 {
		t := strings.TrimSpace(lines[0])
		if t == "" || strings.HasPrefix(t, "--") {
			lines = lines[1:]
		} else {
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
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
