package kit

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	pg "github.com/grizzle-orm/grizzle/schema/pg"
	"github.com/grizzle-orm/grizzle/kit/introspect"
)

// MigrationsTable is the name of the history table G-rizzle creates to track
// applied migrations.
const MigrationsTable = "_grizzle_migrations"

// MigrationRecord is a row in the history table.
type MigrationRecord struct {
	ID          int64     `db:"id"`
	AppliedAt   time.Time `db:"applied_at"`
	Checksum    string    `db:"checksum"`    // SHA-256 hex of the SQL batch
	SQLBatch    string    `db:"sql_batch"`   // full SQL that was applied
	Description string    `db:"description"` // human-readable summary of changes
}

// MigrateResult contains the outcome of a Migrate call.
type MigrateResult struct {
	AlreadyCurrent bool   // true when no changes were needed
	Changes        []Change
	SQL            []string
	Checksum       string
}

// StatusResult is returned by Status — it shows what is recorded vs. what
// the live schema looks like.
type StatusResult struct {
	Applied []MigrationRecord // rows in _grizzle_migrations (oldest first)
	Pending []Change          // changes not yet applied
	SQL     []string          // SQL that would apply the pending changes
}

// -------------------------------------------------------------------
// Public API
// -------------------------------------------------------------------

// Migrate is like Push but records the applied SQL in the _grizzle_migrations
// history table. Calling Migrate twice with an unchanged schema is a no-op.
//
//	result, err := kit.Migrate(ctx, pool, schema.Users, schema.Realms)
func Migrate(ctx context.Context, pool *pgxpool.Pool, tables ...*pg.TableDef) (MigrateResult, error) {
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return MigrateResult{}, err
	}

	live, err := introspect.IntrospectPostgres(ctx, pool)
	if err != nil {
		return MigrateResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := Diff(current, target)

	if len(changes) == 0 {
		return MigrateResult{AlreadyCurrent: true}, nil
	}

	stmts := AllChangeSQL(target, changes)
	checksum := ChecksumSQL(stmts)
	desc := DescribeChanges(changes)

	if err := applyWithHistory(ctx, pool, stmts, checksum, desc); err != nil {
		return MigrateResult{Changes: changes, SQL: stmts, Checksum: checksum},
			fmt.Errorf("apply: %w", err)
	}
	return MigrateResult{Changes: changes, SQL: stmts, Checksum: checksum}, nil
}

// Status reports the applied migration history and any pending changes without
// modifying the database.
//
//	status, err := kit.Status(ctx, pool, schema.Users, schema.Realms)
//	for _, r := range status.Applied {
//	    fmt.Printf("[%s] %s\n", r.AppliedAt.Format(time.RFC3339), r.Description)
//	}
//	if len(status.Pending) > 0 {
//	    fmt.Println("Pending changes:", len(status.Pending))
//	}
func Status(ctx context.Context, pool *pgxpool.Pool, tables ...*pg.TableDef) (StatusResult, error) {
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return StatusResult{}, err
	}

	applied, err := loadHistory(ctx, pool)
	if err != nil {
		return StatusResult{}, err
	}

	live, err := introspect.IntrospectPostgres(ctx, pool)
	if err != nil {
		return StatusResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := Diff(current, target)
	stmts := AllChangeSQL(target, changes)

	return StatusResult{Applied: applied, Pending: changes, SQL: stmts}, nil
}

// LoadHistory returns all rows from _grizzle_migrations in chronological order.
// Returns an empty slice (not an error) if the table does not exist yet.
func LoadHistory(ctx context.Context, pool *pgxpool.Pool) ([]MigrationRecord, error) {
	if err := ensureMigrationsTable(ctx, pool); err != nil {
		return nil, err
	}
	return loadHistory(ctx, pool)
}

// -------------------------------------------------------------------
// Internal helpers
// -------------------------------------------------------------------

const createMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS ` + MigrationsTable + ` (
    id          BIGSERIAL    PRIMARY KEY,
    applied_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    checksum    TEXT         NOT NULL,
    sql_batch   TEXT         NOT NULL,
    description TEXT         NOT NULL DEFAULT ''
)`

func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, createMigrationsTableSQL); err != nil {
		return fmt.Errorf("ensure migrations table: %w", err)
	}
	return nil
}

// applyWithHistory runs the DDL statements and inserts a history record, all
// in a single transaction so they succeed or fail together.
func applyWithHistory(ctx context.Context, pool *pgxpool.Pool, stmts []string, checksum, desc string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}

	const insertSQL = `INSERT INTO ` + MigrationsTable +
		` (checksum, sql_batch, description) VALUES ($1, $2, $3)`
	if _, err := tx.Exec(ctx, insertSQL, checksum, strings.Join(stmts, "\n"), desc); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("record migration: %w", err)
	}

	return tx.Commit(ctx)
}

func loadHistory(ctx context.Context, pool *pgxpool.Pool) ([]MigrationRecord, error) {
	const q = `SELECT id, applied_at, checksum, sql_batch, description
	           FROM ` + MigrationsTable + ` ORDER BY id ASC`

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

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
				col = c.TableName + "." + c.NewCol.Name
			} else if c.OldCol != nil {
				col = c.TableName + "." + c.OldCol.Name
			}
			counts[c.Kind] = append(counts[c.Kind], col)
		default:
			counts[c.Kind] = append(counts[c.Kind], c.TableName)
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
