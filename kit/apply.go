package kit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sofired/grizzle/kit/introspect"
	pg "github.com/sofired/grizzle/schema/pg"
)

// PushResult contains the outcome of a Push operation.
type PushResult struct {
	Changes []Change
	SQL     []string
}

// Push inspects the live database, diffs it against the provided table
// definitions, and applies all necessary DDL changes in a single transaction.
// It accepts tables from any dialect via the TableDefiner interface.
//
// Example:
//
//	result, err := kit.Push(ctx, pool, schema.Users, schema.Realms)
//	for _, stmt := range result.SQL {
//	    fmt.Println(stmt)
//	}
func Push(ctx context.Context, pool *pgxpool.Pool, tables ...pg.TableDefiner) (PushResult, error) {
	// Introspect the current live schema.
	live, err := introspect.IntrospectPostgres(ctx, pool)
	if err != nil {
		return PushResult{}, fmt.Errorf("introspect: %w", err)
	}

	// Convert live snapshot to kit.Snapshot for diffing.
	current := liveToSnapshot(live)

	// Build the target snapshot from the provided TableDef values.
	target := FromDefs(tables...)

	// Compute changes.
	changes := Diff(current, target)
	if len(changes) == 0 {
		return PushResult{}, nil
	}

	// Generate SQL.
	stmts := AllChangeSQL(target, changes)

	// Apply in a transaction.
	if err := execTransaction(ctx, pool, stmts); err != nil {
		return PushResult{Changes: changes, SQL: stmts}, fmt.Errorf("apply: %w", err)
	}

	return PushResult{Changes: changes, SQL: stmts}, nil
}

// DryRun is like Push but does not apply changes — it only computes and
// returns what would be run.
// It accepts tables from any dialect via the TableDefiner interface.
func DryRun(ctx context.Context, pool *pgxpool.Pool, tables ...pg.TableDefiner) (PushResult, error) {
	live, err := introspect.IntrospectPostgres(ctx, pool)
	if err != nil {
		return PushResult{}, fmt.Errorf("introspect: %w", err)
	}
	current := liveToSnapshot(live)
	target := FromDefs(tables...)
	changes := Diff(current, target)
	stmts := AllChangeSQL(target, changes)
	return PushResult{Changes: changes, SQL: stmts}, nil
}

// liveToSnapshot converts an introspect.LiveSnapshot into a kit.Snapshot
// so the differ can compare apples to apples.
//
// Views and enums are intentionally NOT populated here. Push() and DryRun()
// accept only table definitions as targets, so populating live views/enums
// would cause Diff() to emit DROP VIEW / DROP TYPE for every unmanaged live
// object not listed in the caller's table set. A dedicated PushSchema() API
// (tracked in #136) is the correct place to manage views and enums via Push.
func liveToSnapshot(live introspect.LiveSnapshot) Snapshot {
	snap := Snapshot{
		Tables: make(map[string]*TableSnap, len(live.Tables)),
		Views:  make(map[string]*ViewSnap),
		Enums:  make(map[string]*EnumSnap),
	}
	for key, t := range live.Tables {
		if t.Name == MigrationsTable {
			continue // exclude internal grizzle tracking table from diffs
		}
		snap.Tables[key] = &TableSnap{
			Name:        t.Name,
			Schema:      t.Schema,
			Columns:     t.Columns,
			Constraints: t.Constraints,
		}
	}
	return snap
}

// execTransaction runs all statements inside a single PostgreSQL transaction.
// If any statement fails the transaction is rolled back and the error is returned.
func execTransaction(ctx context.Context, pool *pgxpool.Pool, stmts []string) error {
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
	return tx.Commit(ctx)
}
