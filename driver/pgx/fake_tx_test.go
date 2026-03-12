package pgx_test

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	pgxdb "github.com/sofired/grizzle/driver/pgx"
)

// fakeTx is a minimal pgx.Tx implementation for unit tests. Only Exec is
// wired up; all other methods panic or return zero values.
type fakeTx struct {
	onExec func(sql string) error
}

var _ pgx.Tx = (*fakeTx)(nil)

func (f *fakeTx) Begin(ctx context.Context) (pgx.Tx, error) { panic("fakeTx.Begin not implemented") }
func (f *fakeTx) Commit(ctx context.Context) error          { return nil }
func (f *fakeTx) Rollback(ctx context.Context) error        { return nil }
func (f *fakeTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	panic("fakeTx.CopyFrom not implemented")
}
func (f *fakeTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults {
	panic("fakeTx.SendBatch not implemented")
}
func (f *fakeTx) LargeObjects() pgx.LargeObjects { panic("fakeTx.LargeObjects not implemented") }
func (f *fakeTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	panic("fakeTx.Prepare not implemented")
}
func (f *fakeTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if f.onExec != nil {
		return pgconn.CommandTag{}, f.onExec(sql)
	}
	return pgconn.CommandTag{}, nil
}
func (f *fakeTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	panic("fakeTx.Query not implemented")
}
func (f *fakeTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	panic("fakeTx.QueryRow not implemented")
}
func (f *fakeTx) Conn() *pgx.Conn { return nil }

// newFakeTx returns a *pgxdb.Tx wrapping a fakeTx with a no-op Exec.
func newFakeTx(onExec func(sql string) error) *pgxdb.Tx {
	return pgxdb.NewTxForTest(&fakeTx{onExec: onExec})
}

// newFakeTxWithExec is an alias for readability.
func newFakeTxWithExec(onExec func(sql string) error) *pgxdb.Tx {
	return newFakeTx(onExec)
}
