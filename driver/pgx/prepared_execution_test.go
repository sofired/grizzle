package pgx

// prepared_execution_test.go exercises the internal queryAllWith / queryOneWith /
// queryOptWith / execWith helpers using lightweight stubs. This is the only way
// to assert which SQL string is actually submitted to the pool without a live
// database — the public QueryAll / QueryOne / QueryOpt / Exec methods delegate
// to these helpers, so a regression where p.name is passed instead of p.sql
// will be caught here.

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sofired/grizzle/internal/testschema"
	"github.com/sofired/grizzle/query"
)

// stubQuerier is a poolQuerier stub that records the SQL string and args
// passed to Query and returns pgx.ErrNoRows. Tests verify both the SQL string
// and that args pass through unchanged.
type stubQuerier struct {
	gotSQL  string
	gotArgs []any
}

func (s *stubQuerier) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	s.gotSQL = sql
	s.gotArgs = args
	return nil, pgx.ErrNoRows
}

// stubExecer is a poolExecer stub that records the SQL string and args passed to Exec
// and returns a successful CommandTag. Tests verify both the SQL string and that args pass through unchanged.
type stubExecer struct {
	gotSQL  string
	gotArgs []any
}

func (s *stubExecer) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	s.gotSQL = sql
	s.gotArgs = args
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

// TestPreparedSelect_QueryAllUsesSQLNotName calls queryAllWith with a stub
// and asserts that the SQL string — not the statement name — is submitted,
// and that query args pass through unchanged.
func TestPreparedSelect_QueryAllUsesSQLNotName(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := query.Select(testschema.UsersT.ID).
		From(testschema.UsersT).
		Where(testschema.UsersT.RealmID.EQ(realmID))
	reg := NewRegistry(nil)
	stmt := RegisterSelect[testschema.UserSelect](reg, "active_users", b)

	stub := &stubQuerier{}
	// queryAllWith returns pgx.ErrNoRows from the stub — ignore the scan error.
	_, _ = stmt.queryAllWith(context.Background(), stub)

	if stub.gotSQL != stmt.sql {
		t.Errorf("QueryAll submitted %q to pool, want SQL %q", stub.gotSQL, stmt.sql)
	}
	if stub.gotSQL == stmt.name {
		t.Errorf("QueryAll submitted the statement name %q instead of the SQL string", stub.gotSQL)
	}
	if !reflect.DeepEqual(stub.gotArgs, stmt.args) {
		t.Errorf("QueryAll submitted args %v, want %v", stub.gotArgs, stmt.args)
	}
}

// TestPreparedSelect_QueryOneUsesSQLNotName is the same guard for queryOneWith:
// asserts the SQL string — not the statement name — is submitted, and that args pass through unchanged.
func TestPreparedSelect_QueryOneUsesSQLNotName(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := query.Select(testschema.UsersT.ID).
		From(testschema.UsersT).
		Where(testschema.UsersT.RealmID.EQ(realmID))
	reg := NewRegistry(nil)
	stmt := RegisterSelect[testschema.UserSelect](reg, "active_users", b)

	stub := &stubQuerier{}
	_, _ = stmt.queryOneWith(context.Background(), stub)

	if stub.gotSQL != stmt.sql {
		t.Errorf("QueryOne submitted %q to pool, want SQL %q", stub.gotSQL, stmt.sql)
	}
	if stub.gotSQL == stmt.name {
		t.Errorf("QueryOne submitted the statement name %q instead of the SQL string", stub.gotSQL)
	}
	if !reflect.DeepEqual(stub.gotArgs, stmt.args) {
		t.Errorf("QueryOne submitted args %v, want %v", stub.gotArgs, stmt.args)
	}
}

// TestPreparedSelect_QueryOptUsesSQLNotName is the same guard for queryOptWith:
// asserts the SQL string — not the statement name — is submitted, and that args pass through unchanged.
func TestPreparedSelect_QueryOptUsesSQLNotName(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := query.Select(testschema.UsersT.ID).
		From(testschema.UsersT).
		Where(testschema.UsersT.RealmID.EQ(realmID))
	reg := NewRegistry(nil)
	stmt := RegisterSelect[testschema.UserSelect](reg, "active_users", b)

	stub := &stubQuerier{}
	_, _ = stmt.queryOptWith(context.Background(), stub)

	if stub.gotSQL != stmt.sql {
		t.Errorf("QueryOpt submitted %q to pool, want SQL %q", stub.gotSQL, stmt.sql)
	}
	if stub.gotSQL == stmt.name {
		t.Errorf("QueryOpt submitted the statement name %q instead of the SQL string", stub.gotSQL)
	}
	if !reflect.DeepEqual(stub.gotArgs, stmt.args) {
		t.Errorf("QueryOpt submitted args %v, want %v", stub.gotArgs, stmt.args)
	}
}

// TestPreparedExec_ExecUsesSQLNotName calls execWith with a stub and asserts
// that the SQL string — not the statement name — is submitted, and that args
// pass through unchanged.
func TestPreparedExec_ExecUsesSQLNotName(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := query.Update(testschema.UsersT).
		Set("enabled", false).
		Where(testschema.UsersT.RealmID.EQ(realmID))

	reg := NewRegistry(nil)
	stmt := RegisterExec(reg, "disable_users", b)

	stub := &stubExecer{}
	_, err := stmt.execWith(context.Background(), stub)
	if err != nil {
		t.Fatalf("execWith returned unexpected error: %v", err)
	}

	if stub.gotSQL != stmt.sql {
		t.Errorf("Exec submitted %q to pool, want SQL %q", stub.gotSQL, stmt.sql)
	}
	if stub.gotSQL == stmt.name {
		t.Errorf("Exec submitted the statement name %q instead of the SQL string", stub.gotSQL)
	}
	if !reflect.DeepEqual(stub.gotArgs, stmt.args) {
		t.Errorf("Exec submitted args %v, want %v", stub.gotArgs, stmt.args)
	}
}

// fakePgxTx is a minimal pgx.Tx implementation used only in tests. Only Exec
// is implemented; all other methods panic so any unexpected call is obvious.
type fakePgxTx struct {
	gotSQL  string
	gotArgs []any
}

func (f *fakePgxTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.gotSQL = sql
	f.gotArgs = args
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (f *fakePgxTx) Begin(_ context.Context) (pgx.Tx, error) { panic("unexpected") }
func (f *fakePgxTx) Commit(_ context.Context) error          { panic("unexpected") }
func (f *fakePgxTx) Rollback(_ context.Context) error        { panic("unexpected") }
func (f *fakePgxTx) CopyFrom(_ context.Context, _ pgx.Identifier, _ []string, _ pgx.CopyFromSource) (int64, error) {
	panic("unexpected")
}
func (f *fakePgxTx) SendBatch(_ context.Context, _ *pgx.Batch) pgx.BatchResults { panic("unexpected") }
func (f *fakePgxTx) LargeObjects() pgx.LargeObjects                             { panic("unexpected") }
func (f *fakePgxTx) Prepare(_ context.Context, _, _ string) (*pgconn.StatementDescription, error) {
	panic("unexpected")
}
func (f *fakePgxTx) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	panic("unexpected")
}
func (f *fakePgxTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row { panic("unexpected") }
func (f *fakePgxTx) Conn() *pgx.Conn                                        { panic("unexpected") }

// TestPreparedExec_ExecTxUsesSQLNotName guards the ExecTx public entry point.
// ExecTx calls execWith(ctx, tx.tx) — this test constructs a Tx with a fake
// pgx.Tx to confirm the delegation cannot accidentally be reverted to pass p.name.
func TestPreparedExec_ExecTxUsesSQLNotName(t *testing.T) {
	realmID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := query.Update(testschema.UsersT).
		Set("enabled", false).
		Where(testschema.UsersT.RealmID.EQ(realmID))

	reg := NewRegistry(nil)
	stmt := RegisterExec(reg, "disable_users", b)

	fake := &fakePgxTx{}
	tx := &Tx{tx: fake}
	_, err := stmt.ExecTx(context.Background(), tx)
	if err != nil {
		t.Fatalf("ExecTx returned unexpected error: %v", err)
	}

	if fake.gotSQL != stmt.sql {
		t.Errorf("ExecTx submitted %q to pool, want SQL %q", fake.gotSQL, stmt.sql)
	}
	if fake.gotSQL == stmt.name {
		t.Errorf("ExecTx submitted the statement name %q instead of the SQL string", fake.gotSQL)
	}
	if !reflect.DeepEqual(fake.gotArgs, stmt.args) {
		t.Errorf("ExecTx submitted args %v, want %v", fake.gotArgs, stmt.args)
	}
}
