// Package pgx provides the G-rizzle database adapter for jackc/pgx v5.
// It wraps pgxpool.Pool and exposes a transaction helper, keeping the
// query builder and execution layer cleanly separated.
//
// Usage:
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
package pgx

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/grizzle-orm/grizzle/dialect"
	"github.com/grizzle-orm/grizzle/query"
)

// DB wraps a pgxpool.Pool and provides G-rizzle integration helpers.
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
func (db *DB) Query(ctx context.Context, b interface{ Build(dialect.Dialect) (string, []any) }) (pgx.Rows, error) {
	sql, args := b.Build(dialect.Postgres)
	return db.pool.Query(ctx, sql, args...)
}

// Exec executes an INSERT, UPDATE, or DELETE builder and returns the
// number of rows affected.
func (db *DB) Exec(ctx context.Context, b interface{ Build(dialect.Dialect) (string, []any) }) (int64, error) {
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
		return zero, fmt.Errorf("g-rizzle: ScanOne: expected 1 row, got %d", len(results))
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

// Tx wraps a pgx.Tx with G-rizzle helpers.
type Tx struct {
	tx pgx.Tx
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
		return fmt.Errorf("g-rizzle: begin transaction: %w", err)
	}

	tx := &Tx{tx: pgxTx}
	if err := fn(tx); err != nil {
		_ = pgxTx.Rollback(ctx)
		return err
	}
	return pgxTx.Commit(ctx)
}

// Query executes a SELECT builder within the transaction.
func (tx *Tx) Query(ctx context.Context, b interface{ Build(dialect.Dialect) (string, []any) }) (pgx.Rows, error) {
	sql, args := b.Build(dialect.Postgres)
	return tx.tx.Query(ctx, sql, args...)
}

// Exec executes an INSERT/UPDATE/DELETE builder within the transaction.
func (tx *Tx) Exec(ctx context.Context, b interface{ Build(dialect.Dialect) (string, []any) }) (int64, error) {
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
