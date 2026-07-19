package pgx

// prepared_execution_test.go exercises the internal queryAllWith / queryOneWith /
// queryOptWith / execWith helpers using lightweight stubs. This is the only way
// to assert which SQL string is actually submitted to the pool without a live
// database — the public QueryAll / QueryOne / QueryOpt / Exec methods delegate
// to these helpers, so a regression where p.name is passed instead of p.sql
// will be caught here.

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	"github.com/sofired/grizzle/internal/testschema"
	"github.com/sofired/grizzle/query"
)

func invalidSelectBuilder() *query.SelectBuilder {
	return query.Select().Where(expr.RawArgs("x = $? AND y = $?", 1))
}

type invalidIdentifierTable string

func (t invalidIdentifierTable) GrizTableName() string  { return string(t) }
func (t invalidIdentifierTable) GrizTableAlias() string { return string(t) }

func invalidIdentifierSelectBuilder() *query.SelectBuilder {
	return query.Select().From(invalidIdentifierTable("unsafe\nidentifier"))
}

type typedNilBuilder struct{}

func (*typedNilBuilder) Build(dialect.Dialect) (string, []any, error) {
	panic("typed-nil builder must be rejected before Build")
}

func TestPreparedHelpers_PropagateBuildErrorsBeforeDatabaseUse(t *testing.T) {
	ctx := context.Background()

	if stmt, err := PrepareSelect[any](ctx, nil, "invalid", invalidSelectBuilder()); stmt != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("PrepareSelect = (%v, %v), want nil and ErrBuildValidation", stmt, err)
	}
	if stmt, err := PrepareExec(ctx, nil, "invalid", invalidSelectBuilder()); stmt != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("PrepareExec = (%v, %v), want nil and ErrBuildValidation", stmt, err)
	}

	reg := NewRegistry(nil)
	if stmt, err := RegisterSelect[any](reg, "invalid", invalidSelectBuilder()); stmt != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("RegisterSelect = (%v, %v), want nil and ErrBuildValidation", stmt, err)
	}
	if stmt, err := RegisterExec(reg, "invalid", invalidSelectBuilder()); stmt != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("RegisterExec = (%v, %v), want nil and ErrBuildValidation", stmt, err)
	}
	if len(reg.entries) != 0 {
		t.Fatalf("failed registrations mutated registry: %v", reg.entries)
	}
}

func TestPreparedHelpers_PreserveInvalidIdentifierCodeAndRedaction(t *testing.T) {
	ctx := context.Background()
	builder := invalidIdentifierSelectBuilder()
	_, _, normalErr := builder.Build(dialect.Postgres)
	assertInvalidIdentifierError(t, normalErr)

	if stmt, err := PrepareSelect[any](ctx, nil, "invalid_identifier", builder); stmt != nil {
		t.Fatalf("PrepareSelect returned statement: %v", stmt)
	} else {
		assertInvalidIdentifierError(t, err)
	}
	if stmt, err := PrepareExec(ctx, nil, "invalid_identifier", builder); stmt != nil {
		t.Fatalf("PrepareExec returned statement: %v", stmt)
	} else {
		assertInvalidIdentifierError(t, err)
	}

	reg := NewRegistry(nil)
	if stmt, err := RegisterSelect[any](reg, "invalid_identifier", builder); stmt != nil {
		t.Fatalf("RegisterSelect returned statement: %v", stmt)
	} else {
		assertInvalidIdentifierError(t, err)
	}
	if stmt, err := RegisterExec(reg, "invalid_identifier", builder); stmt != nil {
		t.Fatalf("RegisterExec returned statement: %v", stmt)
	} else {
		assertInvalidIdentifierError(t, err)
	}
	if len(reg.entries) != 0 {
		t.Fatalf("failed registrations mutated registry: %v", reg.entries)
	}
}

func assertInvalidIdentifierError(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, query.ErrInvalidIdentifier) {
		t.Fatalf("error = %v, want ErrInvalidIdentifier", err)
	}
	if strings.Contains(err.Error(), "unsafe") || strings.ContainsAny(err.Error(), "\n\r\x00\x1b") {
		t.Fatalf("error leaked unsafe identifier data: %q", err)
	}
}

func TestExecutionHelpers_PropagateBuildErrorsBeforePoolUse(t *testing.T) {
	ctx := context.Background()
	db := New(nil)
	if rows, err := db.Query(ctx, invalidSelectBuilder()); rows != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("Query = (%v, %v), want nil and ErrBuildValidation", rows, err)
	}
	if affected, err := db.Exec(ctx, invalidSelectBuilder()); affected != 0 || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("Exec = (%d, %v), want zero and ErrBuildValidation", affected, err)
	}

	tx := &Tx{}
	if rows, err := tx.Query(ctx, invalidSelectBuilder()); rows != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("Tx.Query = (%v, %v), want nil and ErrBuildValidation", rows, err)
	}
	if affected, err := tx.Exec(ctx, invalidSelectBuilder()); affected != 0 || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("Tx.Exec = (%d, %v), want zero and ErrBuildValidation", affected, err)
	}
}

func TestExecutionHelpers_RejectTypedNilBuildersAndReceivers(t *testing.T) {
	ctx := context.Background()
	var b *typedNilBuilder
	db := New(nil)
	if rows, err := db.Query(ctx, b); rows != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("Query typed nil = (%v, %v), want nil and ErrBuildValidation", rows, err)
	}
	if affected, err := db.Exec(ctx, b); affected != 0 || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("Exec typed nil = (%d, %v), want zero and ErrBuildValidation", affected, err)
	}
	if stmt, err := PrepareExec(ctx, nil, "typed_nil", b); stmt != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("PrepareExec typed nil = (%v, %v), want nil and ErrBuildValidation", stmt, err)
	}
	if stmt, err := RegisterExec(NewRegistry(nil), "typed_nil", b); stmt != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("RegisterExec typed nil = (%v, %v), want nil and ErrBuildValidation", stmt, err)
	}

	var nilDB *DB
	if rows, err := nilDB.Query(ctx, query.Select()); rows != nil || !errors.Is(err, query.ErrInvalidReceiver) {
		t.Fatalf("nil DB Query = (%v, %v), want nil and ErrInvalidReceiver", rows, err)
	}
	var nilTx *Tx
	if affected, err := nilTx.Exec(ctx, query.Select()); affected != 0 || !errors.Is(err, query.ErrInvalidReceiver) {
		t.Fatalf("nil Tx Exec = (%d, %v), want zero and ErrInvalidReceiver", affected, err)
	}
}

func TestPreparedValidationErrorIsRedacted(t *testing.T) {
	err := preparedValidationError("validate_prepared_statement", errors.New("secret statement and SQL"))
	if !errors.Is(err, query.ErrPreparedNotReady) {
		t.Fatalf("error = %v, want ErrPreparedNotReady", err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "SQL") {
		t.Fatalf("error leaked driver detail: %q", err)
	}
	canceled := preparedValidationError("validate_prepared_statement", context.Canceled)
	if !errors.Is(canceled, query.ErrPreparedNotReady) || !errors.Is(canceled, context.Canceled) {
		t.Fatalf("canceled error lost stable or context sentinel: %v", canceled)
	}
}

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
	stmt, _ := RegisterSelect[testschema.UserSelect](reg, "active_users", b)

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
	stmt, _ := RegisterSelect[testschema.UserSelect](reg, "active_users", b)

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
	stmt, _ := RegisterSelect[testschema.UserSelect](reg, "active_users", b)

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
	stmt, _ := RegisterExec(reg, "disable_users", b)

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
	stmt, _ := RegisterExec(reg, "disable_users", b)

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
