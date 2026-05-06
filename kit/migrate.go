package kit

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrationsTable is the name of the history table Grizzle creates to track
// applied migrations.
const MigrationsTable = "_grizzle_migrations"

// MigrationRecord is a row in the history table.
type MigrationRecord struct {
	ID          int64     `db:"id"`
	AppliedAt   time.Time `db:"applied_at"`
	Tag         string    `db:"tag"`         // migration filename stem (e.g. "0001_initial_schema"); empty for old checksum-based rows
	Checksum    string    `db:"checksum"`    // SHA-256 hex of the file bytes (file-based) or SQL batch (legacy)
	SQLBatch    string    `db:"sql_batch"`   // full SQL that was applied; empty for baseline rows
	IsBaseline  bool      `db:"is_baseline"` // true for rows inserted by --baseline (no SQL executed)
	Description string    `db:"description"` // human-readable summary of changes
}

// MigrateResult contains the outcome of a Migrate call.
type MigrateResult struct {
	AlreadyCurrent bool // true when no changes were needed
	Applied        []MigrationFile
	Baselined      []MigrationFile
}

// StatusResult is returned by Status — it shows what is recorded vs. what
// the live schema looks like.
type StatusResult struct {
	Applied []MigrationRecord // rows in _grizzle_migrations (oldest first)
	Pending []MigrationFile   // migration files not yet applied
}

// -------------------------------------------------------------------
// Public API
// -------------------------------------------------------------------

// Migrate reads pending .sql files from opts.MigrationsDir and applies them
// in sequence order, recording each in _grizzle_migrations. Calling Migrate
// twice is idempotent — files already recorded by tag are skipped.
//
// If opts.Baseline is non-empty, migration files up to and including the named
// tag are inserted as baseline records (is_baseline=TRUE, empty sql_batch) in
// a single transaction without executing their SQL. Use this only on existing
// deployments switching to the file-based workflow; on a fresh database, omit
// Baseline and let Migrate apply all files normally.
//
// Example:
//
//	result, err := kit.Migrate(ctx, pool, kit.MigrateOptions{
//	    MigrationsDir: "./migrations",
//	})
//
// To mark existing migrations as applied without executing them:
//
//	result, err := kit.Migrate(ctx, pool, kit.MigrateOptions{
//	    MigrationsDir: "./migrations",
//	    Baseline:      "0001_initial_schema",
//	})
func Migrate(ctx context.Context, pool *pgxpool.Pool, opts MigrateOptions) (MigrateResult, error) {
	if opts.MigrationsDir == "" {
		return MigrateResult{}, fmt.Errorf("MigrateOptions.MigrationsDir must be set")
	}

	// Validate baseline tag format before any DB operation.
	if opts.Baseline != "" {
		if err := ValidateTag(opts.Baseline); err != nil {
			return MigrateResult{}, fmt.Errorf("--baseline: %w", err)
		}
	}

	// Ensure the migrations table exists and is upgraded.
	if err := ensureMigrationsTable(ctx, pool, opts.SkipSchemaUpgrade); err != nil {
		return MigrateResult{}, err
	}

	// Load migration files.
	files, err := LoadMigrationFiles(opts.MigrationsDir)
	if err != nil {
		return MigrateResult{}, err
	}

	if len(files) == 0 {
		return MigrateResult{AlreadyCurrent: true}, nil
	}

	// Load already-applied tags.
	applied, err := loadAppliedTags(ctx, pool)
	if err != nil {
		return MigrateResult{}, err
	}

	var result MigrateResult

	// Hoist baselineSeq so it is computed once and reused.
	var baselineSeq int
	if opts.Baseline != "" {
		var err error
		baselineSeq, err = parseSequenceNumber(opts.Baseline)
		if err != nil {
			return MigrateResult{}, fmt.Errorf("--baseline %q: %w", opts.Baseline, err)
		}
	}

	// Handle baseline mode.
	if opts.Baseline != "" {
		// Check the baseline tag corresponds to an actual file.
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

		// Compute checksums before inserting so we fail early and avoid re-reading
		// files after the transaction commits.
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
			if err := insertBaselinePostgres(ctx, pool, toBaseline); err != nil {
				return MigrateResult{}, fmt.Errorf("baseline: %w", err)
			}
			for _, rec := range toBaseline {
				result.Baselined = append(result.Baselined, rec.File)
				applied[rec.File.Tag] = true
				fmt.Fprintf(os.Stderr, "grizzle: baseline %s  sha256:%s\n", rec.File.FileName, rec.Checksum)
			}
		}
	}

	// Apply pending files (seqNum > baselineSeq, or all if no baseline).
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

	// Apply each file in order.
	for _, f := range toApply {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return result, fmt.Errorf("read %s: %w", f.FileName, err)
		}
		cs := checksumBytes(data)
		sql := string(data)

		if err := applyMigrationFilePostgres(ctx, pool, f.Tag, sql, cs); err != nil {
			return result, fmt.Errorf("apply %s: %w", f.FileName, err)
		}
		result.Applied = append(result.Applied, f)
	}

	return result, nil
}

// Status reports the applied migration history and any pending migration files
// without modifying the database.
//
//	status, err := kit.Status(ctx, pool, kit.MigrateOptions{MigrationsDir: "./migrations"})
func Status(ctx context.Context, pool *pgxpool.Pool, opts MigrateOptions) (StatusResult, error) {
	if opts.MigrationsDir == "" {
		return StatusResult{}, fmt.Errorf("MigrateOptions.MigrationsDir must be set")
	}

	if err := ensureMigrationsTable(ctx, pool, opts.SkipSchemaUpgrade); err != nil {
		return StatusResult{}, err
	}

	history, err := loadHistory(ctx, pool)
	if err != nil {
		return StatusResult{}, err
	}

	files, err := LoadMigrationFiles(opts.MigrationsDir)
	if err != nil {
		return StatusResult{}, err
	}

	applied := make(map[string]bool, len(history))
	for _, r := range history {
		if r.Tag != "" {
			applied[r.Tag] = true
		}
	}

	var pending []MigrationFile
	for _, f := range files {
		if !applied[f.Tag] {
			pending = append(pending, f)
		}
	}

	return StatusResult{Applied: history, Pending: pending}, nil
}

// LoadHistory returns all rows from _grizzle_migrations in chronological order.
// It creates the table (and upgrades its schema) if it does not yet exist.
func LoadHistory(ctx context.Context, pool *pgxpool.Pool) ([]MigrationRecord, error) {
	if err := ensureMigrationsTable(ctx, pool, false); err != nil {
		return nil, err
	}
	return loadHistory(ctx, pool)
}

// -------------------------------------------------------------------
// Internal helpers — PostgreSQL
// -------------------------------------------------------------------

// createMigrationsTableSQL creates the history table with the target schema
// (tag + is_baseline columns included from the start).
const createMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS ` + MigrationsTable + ` (
    id           BIGSERIAL    PRIMARY KEY,
    applied_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    tag          TEXT         NOT NULL DEFAULT '',
    checksum     TEXT         NOT NULL DEFAULT '',
    sql_batch    TEXT         NOT NULL DEFAULT '',
    description  TEXT         NOT NULL DEFAULT '',
    is_baseline  BOOLEAN      NOT NULL DEFAULT FALSE
)`

func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool, skipUpgrade bool) error {
	// Create table if it doesn't exist (uses target schema).
	if _, err := pool.Exec(ctx, createMigrationsTableSQL); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}

	if skipUpgrade {
		// Verify required columns exist; error if absent.
		if err := verifyColumnsPostgres(ctx, pool); err != nil {
			return fmt.Errorf("--skip-schema-upgrade: %w", err)
		}
		return nil
	}

	// Upgrade: add tag and is_baseline if missing.
	return upgradeSchemaPostgres(ctx, pool)
}

// upgradeSchemaPostgres adds tag and is_baseline columns if absent, and ensures
// checksum and sql_batch have DEFAULT " (idempotent).
func upgradeSchemaPostgres(ctx context.Context, pool *pgxpool.Pool) error {
	const q = `
ALTER TABLE ` + MigrationsTable + ` ADD COLUMN IF NOT EXISTS tag TEXT NOT NULL DEFAULT '';
ALTER TABLE ` + MigrationsTable + ` ADD COLUMN IF NOT EXISTS is_baseline BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE ` + MigrationsTable + ` ALTER COLUMN checksum SET DEFAULT '';
ALTER TABLE ` + MigrationsTable + ` ALTER COLUMN sql_batch SET DEFAULT '';`

	if _, err := pool.Exec(ctx, q); err != nil {
		return fmt.Errorf("upgrade migrations table schema: %w", err)
	}
	return nil
}

// verifyColumnsPostgres checks that tag and is_baseline columns exist in the
// current schema. Scoped to current_schema() to avoid false positives on
// shared servers where another schema has a table with the same name.
func verifyColumnsPostgres(ctx context.Context, pool *pgxpool.Pool) error {
	const q = `
SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = $1
  AND column_name = ANY($2)`

	var count int
	err := pool.QueryRow(ctx, q, MigrationsTable, []string{"tag", "is_baseline"}).Scan(&count)
	if err != nil {
		return fmt.Errorf("check column existence: %w", err)
	}
	if count < 2 {
		return fmt.Errorf("_grizzle_migrations is missing required columns (tag, is_baseline); run without --skip-schema-upgrade to upgrade automatically")
	}
	return nil
}

// applyMigrationFilePostgres executes the SQL from a single migration file and
// records it in the history table, all in one transaction.
func applyMigrationFilePostgres(ctx context.Context, pool *pgxpool.Pool, tag, sql, checksum string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	// Execute the migration SQL. We split on ";" to handle multi-statement files.
	stmts := splitSQLStatements(sql)
	for _, stmt := range stmts {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.Exec(ctx, stmt); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("exec statement: %w", err)
		}
	}

	const insertSQL = `INSERT INTO ` + MigrationsTable +
		` (tag, checksum, sql_batch, is_baseline) VALUES ($1, $2, $3, FALSE)`
	if _, err := tx.Exec(ctx, insertSQL, tag, checksum, sql); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("record migration: %w", err)
	}

	return tx.Commit(ctx)
}

// baselineRecord pairs a MigrationFile with its pre-computed file checksum.
type baselineRecord struct {
	File     MigrationFile
	Checksum string
}

// insertBaselinePostgres inserts baseline records in a single transaction.
// Checksums must be pre-computed by the caller to avoid TOCTOU file re-reads.
func insertBaselinePostgres(ctx context.Context, pool *pgxpool.Pool, records []baselineRecord) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	const insertSQL = `INSERT INTO ` + MigrationsTable +
		` (tag, checksum, sql_batch, is_baseline) VALUES ($1, $2, '', TRUE)`

	for _, rec := range records {
		if _, err := tx.Exec(ctx, insertSQL, rec.File.Tag, rec.Checksum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("insert baseline record for %s: %w", rec.File.Tag, err)
		}
	}

	return tx.Commit(ctx)
}

// loadAppliedTags returns a set of tag values already recorded in the history table.
// Empty-string tags (legacy checksum-based rows) are excluded.
func loadAppliedTags(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	const q = `SELECT tag FROM ` + MigrationsTable + ` WHERE tag != ''`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("load applied tags: %w", err)
	}
	defer rows.Close()

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

func loadHistory(ctx context.Context, pool *pgxpool.Pool) ([]MigrationRecord, error) {
	const q = `SELECT id, applied_at, tag, checksum, sql_batch, description, is_baseline
	           FROM ` + MigrationsTable + ` ORDER BY id ASC`

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var records []MigrationRecord
	for rows.Next() {
		var r MigrationRecord
		if err := rows.Scan(&r.ID, &r.AppliedAt, &r.Tag, &r.Checksum, &r.SQLBatch, &r.Description, &r.IsBaseline); err != nil {
			return nil, fmt.Errorf("scan history row: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// -------------------------------------------------------------------
// Utility helpers
// -------------------------------------------------------------------

// ChecksumSQL returns the SHA-256 hex digest of the concatenated SQL statements.
// The checksum is order-sensitive and is stored in the migrations history table.
func ChecksumSQL(stmts []string) string {
	h := sha256.New()
	for _, s := range stmts {
		h.Write([]byte(s))
		h.Write([]byte{0}) // separator
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// DescribeChanges produces a short human-readable summary of the change list,
// e.g. "create_table: users; add_column: posts.title; drop_column: posts.body"
func DescribeChanges(changes []Change) string {
	// Group by kind for a compact summary.
	counts := make(map[ChangeKind][]string)
	for _, c := range changes {
		switch c.Kind {
		case ChangeAddColumn, ChangeDropColumn, ChangeAlterColumnType,
			ChangeAlterColumnNull, ChangeAlterColumnDefault:
			col := ""
			if c.NewCol != nil {
				col = c.ObjectName + "." + c.NewCol.Name
			} else if c.OldCol != nil {
				col = c.ObjectName + "." + c.OldCol.Name
			}
			counts[c.Kind] = append(counts[c.Kind], col)
		case ChangeRenameColumn:
			col := c.ObjectName
			if c.OldCol != nil && c.NewCol != nil {
				col = c.ObjectName + "." + c.OldCol.Name + "→" + c.NewCol.Name
			}
			counts[c.Kind] = append(counts[c.Kind], col)
		case ChangeRenameTable:
			label := c.ObjectName
			if c.RenameTarget != "" {
				label = c.ObjectName + "→" + c.RenameTarget
			}
			counts[c.Kind] = append(counts[c.Kind], label)
		default:
			counts[c.Kind] = append(counts[c.Kind], c.ObjectName)
		}
	}

	var parts []string
	// Stable order: sort by kind string.
	var kinds []string
	for k := range counts {
		kinds = append(kinds, string(k))
	}
	sort.Strings(kinds)

	for _, k := range kinds {
		targets := counts[ChangeKind(k)]
		parts = append(parts, fmt.Sprintf("%s: %s", k, strings.Join(targets, ", ")))
	}
	return strings.Join(parts, "; ")
}

// splitSQLStatements splits a SQL text into individual statements on
// semicolons, skipping semicolons that appear inside single-quoted string
// literals, double-quoted identifiers, line comments (--), and block comments
// (/* */). Dollar-quoted blocks ($$...$$) are not handled; migration files
// containing PL/pgSQL body text should not be split this way.
func splitSQLStatements(sql string) []string {
	var result []string
	var cur strings.Builder
	i, n := 0, len(sql)

	for i < n {
		ch := sql[i]
		switch ch {
		case '\'':
			// Single-quoted literal; '' is the escape sequence for an embedded quote.
			cur.WriteByte(ch)
			i++
			for i < n {
				c := sql[i]
				cur.WriteByte(c)
				i++
				if c == '\'' {
					if i < n && sql[i] == '\'' {
						cur.WriteByte(sql[i])
						i++
					} else {
						break
					}
				}
			}
		case '"':
			// Double-quoted identifier; "" is the escape sequence.
			cur.WriteByte(ch)
			i++
			for i < n {
				c := sql[i]
				cur.WriteByte(c)
				i++
				if c == '"' {
					if i < n && sql[i] == '"' {
						cur.WriteByte(sql[i])
						i++
					} else {
						break
					}
				}
			}
		case '-':
			if i+1 < n && sql[i+1] == '-' {
				// Line comment: consume through end of line.
				for i < n && sql[i] != '\n' {
					cur.WriteByte(sql[i])
					i++
				}
			} else {
				cur.WriteByte(ch)
				i++
			}
		case '/':
			if i+1 < n && sql[i+1] == '*' {
				// Block comment: consume through closing */.
				cur.WriteByte(ch)
				i++
				cur.WriteByte(sql[i])
				i++
				for i+1 < n && (sql[i] != '*' || sql[i+1] != '/') {
					cur.WriteByte(sql[i])
					i++
				}
				if i+1 < n {
					cur.WriteByte(sql[i])
					cur.WriteByte(sql[i+1])
					i += 2
				}
			} else {
				cur.WriteByte(ch)
				i++
			}
		case ';':
			if s := strings.TrimSpace(cur.String()); s != "" {
				result = append(result, s)
			}
			cur.Reset()
			i++
		default:
			cur.WriteByte(ch)
			i++
		}
	}

	if s := strings.TrimSpace(cur.String()); s != "" {
		result = append(result, s)
	}
	return result
}
