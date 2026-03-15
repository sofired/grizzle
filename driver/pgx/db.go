// Package pgx provides the Grizzle database adapter for jackc/pgx v5.
// It wraps pgxpool.Pool and exposes a transaction helper, keeping the
// query builder and execution layer cleanly separated.
//
// # Basic usage
//
//	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
//	db := pgxdb.New(pool)
//
//	// Build a query with the query package, execute with pgx.
//	sql, args := query.Select(UsersT.ID, UsersT.Name).
//	    From(UsersT).
//	    Where(UsersT.DeletedAt.IsNull()).
//	    Build(dialect.Postgres)
//
//	rows, err := db.Pool().Query(ctx, sql, args...)
//	users, err := pgxdb.ScanAll[UserSelect](rows, err)
//
// # Transactions
//
// Use [DB.Transaction] to run a function inside a database transaction that
// automatically commits on success and rolls back on error.
// [DB.TransactionWithOptions] accepts [pgx.TxOptions] to set an isolation
// level, access mode, or deferrable behaviour.
//
// # Savepoints and nested transactions
//
// [Tx.Savepoint], [Tx.RollbackToSavepoint], and [Tx.ReleaseSavepoint] expose
// raw PostgreSQL savepoint commands for fine-grained partial rollback control.
// Savepoint names are validated against PostgreSQL identifier rules before
// being interpolated into SQL, preventing injection attacks.
//
// [Tx.NestedTransaction] is a higher-level helper: it creates an auto-named
// savepoint, runs the supplied function, and releases the savepoint on success
// or rolls it back on error — leaving the outer transaction intact either way.
package pgx

import (
	"context"
	"fmt"
	"regexp"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/query"
)

// savepointNameRE matches valid savepoint identifiers: start with a letter or
// underscore, followed by letters, digits, or underscores, max 63 characters
// (PostgreSQL's identifier length limit).
var savepointNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)

// validateSavepointName returns an error when name contains characters that
// could be used for SQL injection or otherwise violates PostgreSQL identifier
// rules. Savepoint names are interpolated directly into SQL statements so they
// must be strictly validated.
func validateSavepointName(name string) error {
	if !savepointNameRE.MatchString(name) {
		return fmt.Errorf("grizzle: invalid savepoint name %q: must match [A-Za-z_][A-Za-z0-9_]* and be at most 63 characters", name)
	}
	return nil
}

// DB wraps a pgxpool.Pool and provides Grizzle integration helpers.
type DB struct {
	pool *pgxpool.Pool
}

// New creates a DB from an existing pgxpool.Pool.
func New(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// Pool returns the underlying pgxpool.Pool for direct use when needed.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }

// Dialect returns the PostgreSQL dialect, suitable for passing to query.Build().
func (db *DB) Dialect() dialect.Dialect { return dialect.Postgres }

// -------------------------------------------------------------------
// Query execution conveniences
// -------------------------------------------------------------------

// Query executes a SELECT builder and returns the raw pgx.Rows.
// Use ScanAll or ScanOne to collect results into typed structs.
func (db *DB) Query(ctx context.Context, b interface {
	Build(dialect.Dialect) (string, []any)
}) (pgx.Rows, error) {
	sql, args := b.Build(dialect.Postgres)
	return db.pool.Query(ctx, sql, args...)
}

// Exec executes an INSERT, UPDATE, or DELETE builder and returns the
// number of rows affected.
func (db *DB) Exec(ctx context.Context, b interface {
	Build(dialect.Dialect) (string, []any)
}) (int64, error) {
	sql, args := b.Build(dialect.Postgres)
	tag, err := db.pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// QueryRaw executes raw SQL with bound args and returns pgx.Rows.
func (db *DB) QueryRaw(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return db.pool.Query(ctx, sql, args...)
}

// ExecRaw executes raw SQL with bound args.
func (db *DB) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := db.pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// -------------------------------------------------------------------
// Generic scan helpers
// -------------------------------------------------------------------

// ScanAll collects all rows into a []T using pgx's struct-by-name scanner.
// T must be a struct with db:"col_name" tags (or field names matching column
// names after snake_case conversion).
//
//	rows, err := db.Query(ctx, selectQuery)
//	users, err := pgxdb.ScanAll[UserSelect](rows, err)
func ScanAll[T any](rows pgx.Rows, err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[T])
}

// ScanOne collects exactly one row into T. Returns an error if no rows
// are returned or if more than one row is returned.
//
//	rows, err := db.Query(ctx, selectQuery.Limit(1))
//	user, err := pgxdb.ScanOne[UserSelect](rows, err)
func ScanOne[T any](rows pgx.Rows, err error) (T, error) {
	var zero T
	if err != nil {
		return zero, err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return zero, err
	}
	switch len(results) {
	case 0:
		return zero, pgx.ErrNoRows
	case 1:
		return results[0], nil
	default:
		return zero, fmt.Errorf("grizzle: ScanOne: expected 1 row, got %d", len(results))
	}
}

// ScanOneOpt collects zero or one rows into *T. Returns (nil, nil) if no
// rows are returned. Useful for lookups that may find nothing.
func ScanOneOpt[T any](rows pgx.Rows, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

// -------------------------------------------------------------------
// Transactions
// -------------------------------------------------------------------

// Tx wraps a pgx.Tx with Grizzle helpers.
type Tx struct {
	tx      pgx.Tx
	nestedN atomic.Uint64 // counter for auto-naming nested transactions
}

// Transaction runs fn inside a database transaction. If fn returns an
// error the transaction is rolled back; otherwise it is committed.
//
//	err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
//	    _, err := tx.Exec(ctx, updateQuery)
//	    if err != nil { return err }
//	    _, err = tx.Exec(ctx, deleteQuery)
//	    return err
//	})
func (db *DB) Transaction(ctx context.Context, fn func(tx *Tx) error) error {
	pgxTx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("grizzle: begin transaction: %w", err)
	}

	tx := &Tx{tx: pgxTx}
	if err := fn(tx); err != nil {
		_ = pgxTx.Rollback(ctx)
		return err
	}
	return pgxTx.Commit(ctx)
}

// TransactionWithOptions is like Transaction but accepts pgx.TxOptions to
// control isolation level, access mode, and deferrable behaviour.
//
//	err := db.TransactionWithOptions(ctx, pgx.TxOptions{
//	    IsoLevel:   pgx.Serializable,
//	    AccessMode: pgx.ReadWrite,
//	}, func(tx *pgxdb.Tx) error {
//	    _, err := tx.Exec(ctx, updateQuery)
//	    return err
//	})
func (db *DB) TransactionWithOptions(ctx context.Context, opts pgx.TxOptions, fn func(tx *Tx) error) error {
	pgxTx, err := db.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("grizzle: begin transaction: %w", err)
	}

	tx := &Tx{tx: pgxTx}
	if err := fn(tx); err != nil {
		_ = pgxTx.Rollback(ctx)
		return err
	}
	return pgxTx.Commit(ctx)
}

// Query executes a SELECT builder within the transaction.
func (tx *Tx) Query(ctx context.Context, b interface {
	Build(dialect.Dialect) (string, []any)
}) (pgx.Rows, error) {
	sql, args := b.Build(dialect.Postgres)
	return tx.tx.Query(ctx, sql, args...)
}

// Exec executes an INSERT/UPDATE/DELETE builder within the transaction.
func (tx *Tx) Exec(ctx context.Context, b interface {
	Build(dialect.Dialect) (string, []any)
}) (int64, error) {
	sql, args := b.Build(dialect.Postgres)
	tag, err := tx.tx.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// QueryRaw executes raw SQL within the transaction.
func (tx *Tx) QueryRaw(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return tx.tx.Query(ctx, sql, args...)
}

// ExecRaw executes raw SQL within the transaction.
func (tx *Tx) ExecRaw(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := tx.tx.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// -------------------------------------------------------------------
// Savepoints
// -------------------------------------------------------------------

// Savepoint issues a SAVEPOINT command with the given name. The name must
// consist of ASCII letters, digits, and underscores, starting with a letter
// or underscore (PostgreSQL identifier rules). This constraint prevents SQL
// injection, since the name is interpolated directly into the statement.
//
//	if err := tx.Savepoint(ctx, "before_child_insert"); err != nil {
//	    return err
//	}
func (tx *Tx) Savepoint(ctx context.Context, name string) error {
	if err := validateSavepointName(name); err != nil {
		return err
	}
	_, err := tx.tx.Exec(ctx, "SAVEPOINT "+name)
	return err
}

// RollbackToSavepoint issues ROLLBACK TO SAVEPOINT, restoring the transaction
// state to the point at which the named savepoint was set. The savepoint
// itself is preserved and may be rolled back to again.
//
//	if err := tx.RollbackToSavepoint(ctx, "before_child_insert"); err != nil {
//	    return err
//	}
func (tx *Tx) RollbackToSavepoint(ctx context.Context, name string) error {
	if err := validateSavepointName(name); err != nil {
		return err
	}
	_, err := tx.tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+name)
	return err
}

// ReleaseSavepoint issues RELEASE SAVEPOINT, destroying the named savepoint
// (and all savepoints set after it). This does not commit the transaction.
//
//	if err := tx.ReleaseSavepoint(ctx, "before_child_insert"); err != nil {
//	    return err
//	}
func (tx *Tx) ReleaseSavepoint(ctx context.Context, name string) error {
	if err := validateSavepointName(name); err != nil {
		return err
	}
	_, err := tx.tx.Exec(ctx, "RELEASE SAVEPOINT "+name)
	return err
}

// NestedTransaction runs fn within an auto-named savepoint. On success the
// savepoint is released; on failure it is rolled back, leaving the outer
// transaction intact. Each call to NestedTransaction on the same Tx uses a
// unique savepoint name so concurrent or sequential calls do not interfere.
//
//	err := tx.NestedTransaction(ctx, func(tx *pgxdb.Tx) error {
//	    _, err := tx.Exec(ctx, insertChildQuery)
//	    return err
//	})
//	if err != nil && !isDuplicateError(err) {
//	    return err
//	}
func (tx *Tx) NestedTransaction(ctx context.Context, fn func(tx *Tx) error) error {
	n := tx.nestedN.Add(1)
	name := fmt.Sprintf("sp_%d", n)

	if _, err := tx.tx.Exec(ctx, "SAVEPOINT "+name); err != nil {
		return fmt.Errorf("grizzle: nested transaction savepoint: %w", err)
	}

	if err := fn(tx); err != nil {
		if _, rbErr := tx.tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+name); rbErr != nil {
			return fmt.Errorf("grizzle: nested transaction rollback: %w (original error: %w)", rbErr, err)
		}
		return err
	}

	if _, err := tx.tx.Exec(ctx, "RELEASE SAVEPOINT "+name); err != nil {
		return fmt.Errorf("grizzle: nested transaction release: %w", err)
	}
	return nil
}

// -------------------------------------------------------------------
// QueryResultBuilder — fluent execution chaining
// -------------------------------------------------------------------

// SelectResult chains a SelectBuilder to produce a typed result without
// intermediate variable assignment.
//
//	users, err := pgxdb.FromSelect[UserSelect](
//	    ctx, db,
//	    query.Select(UsersT.ID, UsersT.Name).From(UsersT).Where(cond),
//	)
func FromSelect[T any](ctx context.Context, db *DB, b *query.SelectBuilder) ([]T, error) {
	rows, err := db.Query(ctx, b)
	return ScanAll[T](rows, err)
}

// FromSelectOne is like FromSelect but expects exactly one row.
func FromSelectOne[T any](ctx context.Context, db *DB, b *query.SelectBuilder) (T, error) {
	rows, err := db.Query(ctx, b)
	return ScanOne[T](rows, err)
}

// FromSelectOpt is like FromSelect but returns nil when no rows are found.
func FromSelectOpt[T any](ctx context.Context, db *DB, b *query.SelectBuilder) (*T, error) {
	rows, err := db.Query(ctx, b)
	return ScanOneOpt[T](rows, err)
}
