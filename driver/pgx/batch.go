package pgx

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sofired/grizzle/dialect"
)

// builder is the minimal interface required to build SQL from a Grizzle query builder.
type builder interface {
	Build(dialect.Dialect) (string, []any)
}

// batchEntry records a single queued item: its pre-built SQL, args, and
// whether the caller intends to read rows (isQuery=true) or just a command
// tag (isQuery=false).
type batchEntry struct {
	sql     string
	args    []any
	isQuery bool
}

// Batch accumulates multiple SQL statements and sends them to PostgreSQL in a
// single round-trip using pgx's native batch API.
//
// Obtain a Batch from DB.NewBatch or Tx.NewBatch, queue statements with Queue,
// QueueQuery, or QueueRaw, then call Send to execute all of them at once.
//
//	batch := db.NewBatch()
//	batch.Queue(query.InsertInto(UsersT).Values(user1))
//	batch.Queue(query.InsertInto(UsersT).Values(user2))
//	results, err := batch.Send(ctx)
//	if err != nil { ... }
//	defer results.Close()
//
//	n1, err := results.Exec()   // rows affected by first INSERT
//	n2, err := results.Exec()   // rows affected by second INSERT
type Batch struct {
	entries []batchEntry
	pool    interface {
		sendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	}
}

// Queue adds a builder (INSERT, UPDATE, or DELETE) to the batch as an
// exec-style entry. The number of rows affected is readable via
// BatchResults.Exec after Send. For SELECT statements whose rows you want
// to read, use QueueQuery instead.
func (b *Batch) Queue(bl builder) {
	sql, args := bl.Build(dialect.Postgres)
	b.entries = append(b.entries, batchEntry{sql: sql, args: args, isQuery: false})
}

// QueueQuery adds a SELECT builder to the batch as a query entry. Rows are
// readable via BatchResults.Query after Send.
func (b *Batch) QueueQuery(bl builder) {
	sql, args := bl.Build(dialect.Postgres)
	b.entries = append(b.entries, batchEntry{sql: sql, args: args, isQuery: true})
}

// QueueRaw adds a raw SQL statement to the batch. The statement is treated
// as an exec entry (rows affected). Use QueueRawQuery for SELECT statements
// whose rows you want to read.
func (b *Batch) QueueRaw(sql string, args ...any) {
	argsCopy := make([]any, len(args))
	copy(argsCopy, args)
	b.entries = append(b.entries, batchEntry{sql: sql, args: argsCopy, isQuery: false})
}

// QueueRawQuery adds a raw SQL SELECT to the batch as a query entry. Rows are
// readable via BatchResults.Query after Send.
func (b *Batch) QueueRawQuery(sql string, args ...any) {
	argsCopy := make([]any, len(args))
	copy(argsCopy, args)
	b.entries = append(b.entries, batchEntry{sql: sql, args: argsCopy, isQuery: true})
}

// Len returns the number of statements queued so far.
func (b *Batch) Len() int { return len(b.entries) }

// Send executes all queued statements in a single round-trip and returns a
// BatchResults for consuming results in order.
//
// When Send returns a non-nil error, the returned *BatchResults is nil;
// there is nothing to close. When Send succeeds, the caller must call
// BatchResults.Close when done reading results.
func (b *Batch) Send(ctx context.Context) (*BatchResults, error) {
	if len(b.entries) == 0 {
		return nil, fmt.Errorf("grizzle: Batch.Send called with no queued statements")
	}
	if b.pool == nil {
		return nil, fmt.Errorf("grizzle: Batch.Send called on a test batch with no pool (use DB.NewBatch or Tx.NewBatch)")
	}

	pgxBatch := &pgx.Batch{}
	for _, e := range b.entries {
		pgxBatch.Queue(e.sql, e.args...)
	}

	br := b.pool.sendBatch(ctx, pgxBatch)
	snapshot := make([]batchEntry, len(b.entries))
	copy(snapshot, b.entries)
	return &BatchResults{br: br, entries: snapshot}, nil
}

// -------------------------------------------------------------------
// BatchResults — iterates over the results of a sent batch
// -------------------------------------------------------------------

// BatchResults holds the server response to a sent Batch. Consume results
// in the same order statements were queued: call Exec for each Queue/QueueRaw
// entry and Query (or ScanAll / ScanOne) for each QueueQuery/QueueRawQuery entry.
//
// Always call Close when done, even if an earlier method returned an error.
type BatchResults struct {
	br      pgx.BatchResults
	entries []batchEntry
	idx     int
}

// Exec reads the result of the next queued exec statement and returns the
// number of rows affected. It returns an error if the next entry is a query
// entry (IsQuery=true); call Query instead.
func (r *BatchResults) Exec() (int64, error) {
	if r.idx >= len(r.entries) {
		return 0, fmt.Errorf("grizzle: BatchResults.Exec: no more results")
	}
	if r.entries[r.idx].isQuery {
		return 0, fmt.Errorf("grizzle: BatchResults.Exec: next entry is a query entry; call Query instead")
	}
	ct, err := r.br.Exec()
	if err != nil {
		return 0, err
	}
	r.idx++
	return ct.RowsAffected(), nil
}

// ExecCommandTag reads the result of the next queued exec statement and returns
// the raw pgconn.CommandTag. It returns an error if the next entry is a query
// entry (IsQuery=true); call Query instead.
func (r *BatchResults) ExecCommandTag() (pgconn.CommandTag, error) {
	if r.idx >= len(r.entries) {
		return pgconn.CommandTag{}, fmt.Errorf("grizzle: BatchResults.ExecCommandTag: no more results")
	}
	if r.entries[r.idx].isQuery {
		return pgconn.CommandTag{}, fmt.Errorf("grizzle: BatchResults.ExecCommandTag: next entry is a query entry; call Query instead")
	}
	ct, err := r.br.Exec()
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	r.idx++
	return ct, nil
}

// Query reads the rows from the next queued query statement. The caller must
// close the returned pgx.Rows before calling Query or Exec again. It returns
// an error if the next entry is an exec entry (IsQuery=false); call Exec instead.
//
//	rows, err := results.Query()
//	users, err := pgxdb.ScanAll[UserSelect](rows, err)
func (r *BatchResults) Query() (pgx.Rows, error) {
	if r.idx >= len(r.entries) {
		return nil, fmt.Errorf("grizzle: BatchResults.Query: no more results")
	}
	if !r.entries[r.idx].isQuery {
		return nil, fmt.Errorf("grizzle: BatchResults.Query: next entry is an exec entry; call Exec instead")
	}
	rows, err := r.br.Query()
	if err != nil {
		return nil, err
	}
	r.idx++
	return rows, nil
}

// Close closes the underlying pgx.BatchResults. Must be called after all
// results have been consumed (or to abort early). Safe to call multiple times.
func (r *BatchResults) Close() error {
	return r.br.Close()
}

// -------------------------------------------------------------------
// batchSender is satisfied by both *pgxpool.Pool and a pgx.Tx wrapper.
// -------------------------------------------------------------------

// poolSender wraps *pgxpool.Pool to satisfy the private pool interface on Batch.
type poolSender struct {
	pool interface {
		SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	}
}

func (s poolSender) sendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return s.pool.SendBatch(ctx, b)
}

// txSender wraps pgx.Tx to satisfy the private pool interface on Batch.
type txSender struct {
	tx pgx.Tx
}

func (s txSender) sendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return s.tx.SendBatch(ctx, b)
}

// -------------------------------------------------------------------
// BatchEntry is the exported view of a single queued item, used in tests.
// -------------------------------------------------------------------

// BatchEntry is the exported, read-only view of a single entry in a Batch.
// It is used by tests to inspect the pre-built SQL and metadata without
// requiring a live database connection.
type BatchEntry struct {
	SQL     string
	Args    []any
	IsQuery bool
}

// Entries returns a snapshot of all queued entries. It is intended for use in
// unit tests that need to verify generated SQL without a live database.
// Each returned BatchEntry has its own copy of the Args slice so that callers
// cannot accidentally mutate the Batch's internal state.
func (b *Batch) Entries() []BatchEntry {
	out := make([]BatchEntry, len(b.entries))
	for i, e := range b.entries {
		var argsCopy []any
		if e.args != nil {
			argsCopy = make([]any, len(e.args))
			copy(argsCopy, e.args)
		}
		out[i] = BatchEntry{SQL: e.sql, Args: argsCopy, IsQuery: e.isQuery}
	}
	return out
}

// NewBatchForTest returns a Batch with no pool attached. It is safe to call
// Queue, QueueQuery, QueueRaw, QueueRawQuery, Len, and Entries, but Send will
// return an error. Use this in unit tests that only need to inspect generated SQL.
func NewBatchForTest() *Batch {
	return &Batch{}
}

// NewBatchResultsForTest returns a BatchResults backed by the provided
// pgx.BatchResults mock and the given entries. It is intended for unit tests
// that need to verify entry-type guards and index-advancement behaviour
// without a live database connection.
func NewBatchResultsForTest(br pgx.BatchResults, entries []BatchEntry) *BatchResults {
	internal := make([]batchEntry, len(entries))
	for i, e := range entries {
		internal[i] = batchEntry{sql: e.SQL, args: e.Args, isQuery: e.IsQuery}
	}
	return &BatchResults{br: br, entries: internal}
}

// -------------------------------------------------------------------
// DB.NewBatch and Tx.NewBatch
// -------------------------------------------------------------------

// NewBatch returns an empty Batch bound to this DB. Queue statements with
// Batch.Queue, Batch.QueueQuery, or Batch.QueueRaw, then call Batch.Send.
func (db *DB) NewBatch() *Batch {
	return &Batch{pool: poolSender{pool: db.pool}}
}

// NewBatch returns an empty Batch bound to this transaction. Statements are
// executed within the transaction when Batch.Send is called.
func (tx *Tx) NewBatch() *Batch {
	return &Batch{pool: txSender{tx: tx.tx}}
}
