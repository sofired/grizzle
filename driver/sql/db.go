// Package sql provides G-rizzle integration for database/sql-based drivers
// (MySQL via go-sql-driver/mysql, SQLite via mattn/go-sqlite3, and any other
// driver that uses the standard database/sql interface).
//
// This package mirrors the API surface of driver/pgx so that application code
// can switch databases with minimal changes.
//
// Usage (MySQL):
//
//	import (
//	    sqldb "github.com/sofired/grizzle/driver/sql"
//	    "github.com/sofired/grizzle/dialect"
//	    _ "github.com/go-sql-driver/mysql"
//	)
//
//	raw, err := sql.Open("mysql", dsn)
//	db := sqldb.New(raw, dialect.MySQL)
//
//	sql, args := query.Select(UsersT.ID, UsersT.Name).
//	    From(UsersT).
//	    Where(UsersT.DeletedAt.IsNull()).
//	    Build(db.Dialect())
//
//	rows, err := db.DB().QueryContext(ctx, sql, args...)
//	users, err := sqldb.ScanAll[UserSelect](rows, err)
//
// Usage (SQLite):
//
//	import (
//	    sqldb "github.com/sofired/grizzle/driver/sql"
//	    "github.com/sofired/grizzle/dialect"
//	    _ "github.com/mattn/go-sqlite3"
//	)
//
//	raw, err := sql.Open("sqlite3", "./app.db")
//	db := sqldb.New(raw, dialect.SQLite)
package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/query"
)

// ErrNoRows is returned by ScanOne when the query returns zero rows.
// It mirrors the sentinel error from database/sql.
var ErrNoRows = sql.ErrNoRows

// DB wraps a *sql.DB together with its dialect and provides G-rizzle helpers.
type DB struct {
	db  *sql.DB
	d   dialect.Dialect
}

// New creates a DB from an existing *sql.DB and the dialect that matches the
// underlying driver (dialect.MySQL, dialect.SQLite, etc.).
func New(db *sql.DB, d dialect.Dialect) *DB {
	return &DB{db: db, d: d}
}

// DB returns the underlying *sql.DB for direct use when needed.
func (w *DB) DB() *sql.DB { return w.db }

// Dialect returns the dialect this DB was configured with.
func (w *DB) Dialect() dialect.Dialect { return w.d }

// -------------------------------------------------------------------
// Query execution conveniences
// -------------------------------------------------------------------

// Query executes a SELECT builder and returns the raw *sql.Rows.
// Use ScanAll or ScanOne to collect results into typed structs.
func (w *DB) Query(ctx context.Context, b interface{ Build(dialect.Dialect) (string, []any) }) (*sql.Rows, error) {
	q, args := b.Build(w.d)
	return w.db.QueryContext(ctx, q, args...)
}

// Exec executes an INSERT, UPDATE, or DELETE builder and returns the number
// of rows affected.
func (w *DB) Exec(ctx context.Context, b interface{ Build(dialect.Dialect) (string, []any) }) (int64, error) {
	q, args := b.Build(w.d)
	res, err := w.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// QueryRaw executes raw SQL with bound args and returns *sql.Rows.
func (w *DB) QueryRaw(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return w.db.QueryContext(ctx, q, args...)
}

// ExecRaw executes raw SQL with bound args and returns the number of rows
// affected.
func (w *DB) ExecRaw(ctx context.Context, q string, args ...any) (int64, error) {
	res, err := w.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// -------------------------------------------------------------------
// Generic scan helpers
// -------------------------------------------------------------------

// ScanAll collects all rows into a []T. T must be a struct with
// db:"col_name" tags; column names from the query are matched to struct fields
// case-insensitively.
//
//	rows, err := db.Query(ctx, selectQuery)
//	users, err := sqldb.ScanAll[UserSelect](rows, err)
func ScanAll[T any](rows *sql.Rows, err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("grizzle/sql: columns: %w", err)
	}

	var results []T
	for rows.Next() {
		var zero T
		if err := scanRow(rows, cols, &zero); err != nil {
			return nil, err
		}
		results = append(results, zero)
	}
	return results, rows.Err()
}

// ScanOne collects exactly one row into T. Returns ErrNoRows if no rows are
// returned, or an error if more than one row is returned.
//
//	rows, err := db.Query(ctx, selectQuery.Limit(1))
//	user, err := sqldb.ScanOne[UserSelect](rows, err)
func ScanOne[T any](rows *sql.Rows, err error) (T, error) {
	var zero T
	if err != nil {
		return zero, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return zero, fmt.Errorf("grizzle/sql: columns: %w", err)
	}

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return zero, err
		}
		return zero, ErrNoRows
	}
	if err := scanRow(rows, cols, &zero); err != nil {
		return zero, err
	}
	if rows.Next() {
		return zero, fmt.Errorf("grizzle/sql: ScanOne: expected 1 row, got more")
	}
	return zero, rows.Err()
}

// ScanOneOpt collects zero or one rows into *T. Returns (nil, nil) if no
// rows are found. Useful for lookups that may find nothing.
func ScanOneOpt[T any](rows *sql.Rows, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("grizzle/sql: columns: %w", err)
	}

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	var zero T
	if err := scanRow(rows, cols, &zero); err != nil {
		return nil, err
	}
	return &zero, rows.Err()
}

// -------------------------------------------------------------------
// Transactions
// -------------------------------------------------------------------

// Tx wraps a *sql.Tx with G-rizzle helpers.
type Tx struct {
	tx *sql.Tx
	d  dialect.Dialect
}

// Transaction runs fn inside a database transaction. If fn returns an error
// the transaction is rolled back; otherwise it is committed.
//
//	err := db.Transaction(ctx, func(tx *sqldb.Tx) error {
//	    _, err := tx.Exec(ctx, updateQuery)
//	    if err != nil { return err }
//	    _, err = tx.Exec(ctx, deleteQuery)
//	    return err
//	})
func (w *DB) Transaction(ctx context.Context, fn func(tx *Tx) error) error {
	sqlTx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("grizzle/sql: begin transaction: %w", err)
	}

	tx := &Tx{tx: sqlTx, d: w.d}
	if err := fn(tx); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	return sqlTx.Commit()
}

// Dialect returns the dialect this Tx was configured with.
func (tx *Tx) Dialect() dialect.Dialect { return tx.d }

// Query executes a SELECT builder within the transaction.
func (tx *Tx) Query(ctx context.Context, b interface{ Build(dialect.Dialect) (string, []any) }) (*sql.Rows, error) {
	q, args := b.Build(tx.d)
	return tx.tx.QueryContext(ctx, q, args...)
}

// Exec executes an INSERT/UPDATE/DELETE builder within the transaction.
func (tx *Tx) Exec(ctx context.Context, b interface{ Build(dialect.Dialect) (string, []any) }) (int64, error) {
	q, args := b.Build(tx.d)
	res, err := tx.tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// QueryRaw executes raw SQL within the transaction.
func (tx *Tx) QueryRaw(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return tx.tx.QueryContext(ctx, q, args...)
}

// ExecRaw executes raw SQL within the transaction.
func (tx *Tx) ExecRaw(ctx context.Context, q string, args ...any) (int64, error) {
	res, err := tx.tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// -------------------------------------------------------------------
// Fluent execution helpers
// -------------------------------------------------------------------

// FromSelect executes a SelectBuilder against db and collects all results.
//
//	users, err := sqldb.FromSelect[UserSelect](ctx, db, query.Select(...).From(...))
func FromSelect[T any](ctx context.Context, db *DB, b *query.SelectBuilder) ([]T, error) {
	rows, err := db.Query(ctx, b)
	return ScanAll[T](rows, err)
}

// FromSelectOne executes a SelectBuilder against db and expects exactly one row.
func FromSelectOne[T any](ctx context.Context, db *DB, b *query.SelectBuilder) (T, error) {
	rows, err := db.Query(ctx, b)
	return ScanOne[T](rows, err)
}

// FromSelectOpt executes a SelectBuilder against db and returns nil when no
// rows are found.
func FromSelectOpt[T any](ctx context.Context, db *DB, b *query.SelectBuilder) (*T, error) {
	rows, err := db.Query(ctx, b)
	return ScanOneOpt[T](rows, err)
}

// -------------------------------------------------------------------
// Internal reflection-based row scanner
// -------------------------------------------------------------------

// scanRow scans a single *sql.Rows row into dest (must be a pointer to struct)
// by matching column names to struct fields with db:"name" tags.
func scanRow(rows *sql.Rows, cols []string, dest any) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("grizzle/sql: ScanAll/ScanOne destination must be a pointer to struct")
	}
	rv = rv.Elem()
	rt := rv.Type()

	// Build a map from column name → field index for quick lookup.
	// We do this per-scan to keep things simple; for hot paths a cached
	// mapping could be added later.
	fieldIdx := make(map[string]int, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.SplitN(tag, ",", 2)[0]
		fieldIdx[strings.ToLower(name)] = i
	}

	// Build the slice of destination pointers for sql.Rows.Scan.
	dests := make([]any, len(cols))
	for i, col := range cols {
		key := strings.ToLower(col)
		if idx, ok := fieldIdx[key]; ok {
			dests[i] = rv.Field(idx).Addr().Interface()
		} else {
			// Discard columns that don't map to a field.
			dests[i] = new(any)
		}
	}

	if err := rows.Scan(dests...); err != nil {
		return fmt.Errorf("grizzle/sql: scan: %w", err)
	}
	return nil
}

// -------------------------------------------------------------------
// Sentinel error helpers
// -------------------------------------------------------------------

// IsNotFound reports whether err indicates that a query returned no rows
// (equivalent to sql.ErrNoRows).
func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
