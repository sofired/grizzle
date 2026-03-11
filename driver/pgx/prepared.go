package pgx

import (
	"context"
	"fmt"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/query"
)

// PreparedSelect is a SELECT query whose SQL has been pre-built and validated
// against the database at initialization time. Use it for static queries —
// those where the table, columns, and WHERE conditions do not change between
// calls. Dynamic per-call conditions should still use the regular query builder.
//
// Pre-validation means any SQL error (wrong column name, type mismatch, bad
// syntax) is surfaced at server startup rather than during a live request.
//
// The statement name is visible in pg_stat_statements and pg_prepared_statements,
// making it easy to track query performance in production.
//
// Example (static active-users list):
//
//	var activeUsers *pgxdb.PreparedSelect[UserSelect]
//
//	func initQueries(ctx context.Context, db *pgxdb.DB) error {
//	    var err error
//	    activeUsers, err = pgxdb.PrepareSelect[UserSelect](ctx, db, "active_users",
//	        query.Select(UsersT.ID, UsersT.Username, UsersT.Email).
//	            From(UsersT).
//	            Where(expr.And(UsersT.Enabled.IsTrue(), UsersT.DeletedAt.IsNull())).
//	            OrderBy(UsersT.CreatedAt.Desc()),
//	    )
//	    return err
//	}
//
//	users, err := activeUsers.QueryAll(ctx, db)
//
// PreparedSelect holds the pre-built SQL and its bound args.
// pgx v5 automatically maintains a per-connection prepared statement cache
// keyed by the statement name — so queries using the same name benefit from
// the server-side parse cache on every subsequent execution.
type PreparedSelect[T any] struct {
	name string
	sql  string
	args []any
}

// PrepareSelect validates the query SQL against the live database and returns
// a PreparedSelect for repeated execution. Returns an error if the SQL is
// syntactically invalid or references unknown columns or tables.
func PrepareSelect[T any](ctx context.Context, db *DB, name string, b *query.SelectBuilder) (*PreparedSelect[T], error) {
	sql, args := b.Build(dialect.Postgres)
	if err := validateStatement(ctx, db, name, sql); err != nil {
		return nil, err
	}
	return &PreparedSelect[T]{name: name, sql: sql, args: args}, nil
}

// Name returns the statement name (visible in pg_stat_statements).
func (p *PreparedSelect[T]) Name() string { return p.name }

// SQL returns the pre-built SQL string.
func (p *PreparedSelect[T]) SQL() string { return p.sql }

// QueryAll executes the prepared query and returns all matching rows.
func (p *PreparedSelect[T]) QueryAll(ctx context.Context, db *DB) ([]T, error) {
	rows, err := db.Pool().Query(ctx, p.name, p.args...)
	return ScanAll[T](rows, err)
}

// QueryOne executes the prepared query and expects exactly one row.
// Returns an error if zero or more than one row is returned.
func (p *PreparedSelect[T]) QueryOne(ctx context.Context, db *DB) (T, error) {
	rows, err := db.Pool().Query(ctx, p.name, p.args...)
	return ScanOne[T](rows, err)
}

// QueryOpt executes the prepared query and returns nil if no rows are found.
func (p *PreparedSelect[T]) QueryOpt(ctx context.Context, db *DB) (*T, error) {
	rows, err := db.Pool().Query(ctx, p.name, p.args...)
	return ScanOneOpt[T](rows, err)
}

// -------------------------------------------------------------------
// PreparedExec — for INSERT / UPDATE / DELETE
// -------------------------------------------------------------------

// PreparedExec is a mutation query (INSERT/UPDATE/DELETE) whose SQL has been
// pre-built and validated. Use it for hot-path mutations that always operate
// on the same shape of data.
//
// Example (soft-delete by ID):
//
//	type deleteArgs struct{ ID uuid.UUID }
//
//	var softDelete *pgxdb.PreparedExec
//
//	func initQueries(ctx context.Context, db *pgxdb.DB) (err error) {
//	    softDelete, err = pgxdb.PrepareExec(ctx, db, "soft_delete_user",
//	        query.Update(UsersT).
//	            Set("deleted_at", time.Now()).  // placeholder — real value passed at Exec time
//	            Where(UsersT.ID.EQ(uuid.Nil)),
//	    )
//	    return
//	}
type PreparedExec struct {
	name string
	sql  string
	args []any
}

// PrepareExec validates and caches a mutation query. The builder interface
// accepts SelectBuilder, InsertBuilder, UpdateBuilder, or DeleteBuilder —
// anything with a Build method.
func PrepareExec(ctx context.Context, db *DB, name string, b interface {
	Build(dialect.Dialect) (string, []any)
}) (*PreparedExec, error) {
	sql, args := b.Build(dialect.Postgres)
	if err := validateStatement(ctx, db, name, sql); err != nil {
		return nil, err
	}
	return &PreparedExec{name: name, sql: sql, args: args}, nil
}

// Name returns the statement name.
func (p *PreparedExec) Name() string { return p.name }

// SQL returns the pre-built SQL string.
func (p *PreparedExec) SQL() string { return p.sql }

// Exec runs the prepared mutation and returns the number of rows affected.
func (p *PreparedExec) Exec(ctx context.Context, db *DB) (int64, error) {
	tag, err := db.Pool().Exec(ctx, p.name, p.args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ExecTx runs the prepared mutation inside an existing transaction.
func (p *PreparedExec) ExecTx(ctx context.Context, tx *Tx) (int64, error) {
	tag, err := tx.tx.Exec(ctx, p.name, p.args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// -------------------------------------------------------------------
// Registry — manage a named set of prepared statements
// -------------------------------------------------------------------

// Registry holds a set of named prepared statements and validates them all
// against the database in one shot at startup.
//
// Example:
//
//	reg := pgxdb.NewRegistry(db)
//	getUser   := pgxdb.Register[UserSelect](reg, "get_active_users", activeUsersQuery)
//	updateUser := pgxdb.RegisterExec(reg, "soft_delete",  softDeleteQuery)
//
//	if err := reg.PrepareAll(ctx); err != nil {
//	    log.Fatal("query validation failed:", err)
//	}
type Registry struct {
	db      *DB
	entries []registryEntry
}

type registryEntry struct {
	name string
	sql  string
	args []any
}

// NewRegistry creates a Registry bound to the given DB.
func NewRegistry(db *DB) *Registry {
	return &Registry{db: db}
}

// PrepareAll validates every registered statement against the live database.
// Call this once during server startup; if it returns an error, at least one
// query has a SQL problem.
func (r *Registry) PrepareAll(ctx context.Context) error {
	for _, e := range r.entries {
		if err := validateStatement(ctx, r.db, e.name, e.sql); err != nil {
			return fmt.Errorf("prepare %q: %w", e.name, err)
		}
	}
	return nil
}

// register adds a built query to the registry and returns the cached sql+args.
func (r *Registry) register(name string, sql string, args []any) {
	r.entries = append(r.entries, registryEntry{name: name, sql: sql, args: args})
}

// RegisterSelect adds a SELECT query to a Registry. Call reg.PrepareAll(ctx)
// to validate all registered statements in one shot.
//
//	reg := pgxdb.NewRegistry(db)
//	stmt := pgxdb.RegisterSelect[UserSelect](reg, "active_users", activeUsersBuilder)
//	if err := reg.PrepareAll(ctx); err != nil { ... }
//	users, err := stmt.QueryAll(ctx, db)
func RegisterSelect[T any](reg *Registry, name string, b *query.SelectBuilder) *PreparedSelect[T] {
	sql, args := b.Build(dialect.Postgres)
	reg.register(name, sql, args)
	return &PreparedSelect[T]{name: name, sql: sql, args: args}
}

// RegisterExec adds a mutation query to a Registry.
func RegisterExec(reg *Registry, name string, b interface {
	Build(dialect.Dialect) (string, []any)
}) *PreparedExec {
	sql, args := b.Build(dialect.Postgres)
	reg.register(name, sql, args)
	return &PreparedExec{name: name, sql: sql, args: args}
}

// -------------------------------------------------------------------
// Internal helpers
// -------------------------------------------------------------------

// validateStatement prepares a statement on a single pooled connection to
// check its SQL validity. The connection is immediately released back to the pool.
//
// pgx v5's Conn.Prepare sends a Parse message to PostgreSQL, which validates
// the SQL without executing it — wrong column names, type errors, and syntax
// problems are all caught here.
func validateStatement(ctx context.Context, db *DB, name, sql string) error {
	conn, err := db.Pool().Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	// Conn() returns the underlying *pgx.Conn.
	// Prepare(ctx, name, sql) sends a Parse + Describe to the backend.
	if _, err := conn.Conn().Prepare(ctx, name, sql); err != nil {
		return fmt.Errorf("SQL validation failed for %q: %w", name, err)
	}
	return nil
}
