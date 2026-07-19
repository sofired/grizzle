package sql_test

import (
	"context"
	gosql "database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/sofired/grizzle/dialect"
	sqldb "github.com/sofired/grizzle/driver/sql"
	"github.com/sofired/grizzle/expr"
	"github.com/sofired/grizzle/query"
)

type typedNilBuilder struct{}

func (*typedNilBuilder) Build(dialect.Dialect) (string, []any, error) {
	panic("typed-nil builder must be rejected before Build")
}

func TestExecutionHelpers_PropagateBuildErrorsBeforeDatabaseUse(t *testing.T) {
	db := sqldb.New(nil, dialect.Postgres)
	b := query.Select().Where(expr.RawArgs("x = $? AND y = $?", 1))
	if rows, err := db.Query(context.Background(), b); rows != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("Query = (%v, %v), want nil and ErrBuildValidation", rows, err)
	}
	if affected, err := db.Exec(context.Background(), b); affected != 0 || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("Exec = (%d, %v), want zero and ErrBuildValidation", affected, err)
	}
}

func TestExecutionHelpers_RejectTypedNilBuildersAndReceivers(t *testing.T) {
	ctx := context.Background()
	var b *typedNilBuilder
	db := sqldb.New(nil, dialect.Postgres)
	if rows, err := db.Query(ctx, b); rows != nil || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("Query typed nil = (%v, %v), want nil and ErrBuildValidation", rows, err)
	}
	if affected, err := db.Exec(ctx, b); affected != 0 || !errors.Is(err, query.ErrBuildValidation) {
		t.Fatalf("Exec typed nil = (%d, %v), want zero and ErrBuildValidation", affected, err)
	}

	var nilDB *sqldb.DB
	if rows, err := nilDB.Query(ctx, query.Select()); rows != nil || !errors.Is(err, query.ErrInvalidReceiver) {
		t.Fatalf("nil DB Query = (%v, %v), want nil and ErrInvalidReceiver", rows, err)
	}
	var nilTx *sqldb.Tx
	if affected, err := nilTx.Exec(ctx, query.Select()); affected != 0 || !errors.Is(err, query.ErrInvalidReceiver) {
		t.Fatalf("nil Tx Exec = (%d, %v), want zero and ErrInvalidReceiver", affected, err)
	}
}

func TestTransactionExecutionHelpers_PropagateBuildErrorsBeforeDatabaseUse(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	b := query.Select().Where(expr.RawArgs("x = $? AND y = $?", 1))
	err := db.Transaction(ctx, func(tx *sqldb.Tx) error {
		if rows, err := tx.Query(ctx, b); rows != nil || !errors.Is(err, query.ErrBuildValidation) {
			t.Fatalf("Tx.Query = (%v, %v), want nil and ErrBuildValidation", rows, err)
		}
		if affected, err := tx.Exec(ctx, b); affected != 0 || !errors.Is(err, query.ErrBuildValidation) {
			t.Fatalf("Tx.Exec = (%d, %v), want zero and ErrBuildValidation", affected, err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Transaction = %v", err)
	}
}

// -------------------------------------------------------------------
// Schema helpers for tests
// -------------------------------------------------------------------

type usersTable struct{ expr.ColBase }

func (usersTable) GrizTableName() string  { return "users" }
func (usersTable) GrizTableAlias() string { return "users" }

var (
	usersT  = usersTable{}
	idCol   = expr.IntColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "id"}}
	nameCol = expr.StringColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "name"}}
)

type UserRow struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

type userInsert struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

// openTestDB opens an in-memory SQLite database with a simple users table.
func openTestDB(t *testing.T) *sqldb.DB {
	t.Helper()
	raw, err := gosql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite3: %v", err)
	}
	t.Cleanup(func() { raw.Close() })
	if _, err := raw.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO users (id, name) VALUES (1, 'Alice'), (2, 'Bob')`); err != nil {
		t.Fatalf("insert rows: %v", err)
	}
	return sqldb.New(raw, dialect.SQLite)
}

// -------------------------------------------------------------------
// Tests
// -------------------------------------------------------------------

func TestNew_Dialect(t *testing.T) {
	db := openTestDB(t)
	if db.Dialect().Name() != "sqlite" {
		t.Fatalf("want sqlite, got %s", db.Dialect().Name())
	}
}

func TestQuery_ScanAll(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	b := query.Select(idCol, nameCol).From(usersT).OrderBy(idCol.Asc())
	rows, err := db.Query(ctx, b)
	users, err := sqldb.ScanAll[UserRow](rows, err)
	if err != nil {
		t.Fatalf("ScanAll: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(users))
	}
	if users[0].Name != "Alice" || users[1].Name != "Bob" {
		t.Fatalf("unexpected names: %+v", users)
	}
}

func TestFromSelect_All(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	b := query.Select(idCol, nameCol).From(usersT).OrderBy(idCol.Asc())
	users, err := sqldb.FromSelect[UserRow](ctx, db, b)
	if err != nil {
		t.Fatalf("FromSelect: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(users))
	}
}

func TestFromSelectOne(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	b := query.Select(idCol, nameCol).From(usersT).Where(idCol.EQ(1))
	user, err := sqldb.FromSelectOne[UserRow](ctx, db, b)
	if err != nil {
		t.Fatalf("FromSelectOne: %v", err)
	}
	if user.ID != 1 || user.Name != "Alice" {
		t.Fatalf("unexpected row: %+v", user)
	}
}

func TestScanOne_ErrNoRows(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	b := query.Select(idCol, nameCol).From(usersT).Where(idCol.EQ(999))
	rows, err := db.Query(ctx, b)
	_, err = sqldb.ScanOne[UserRow](rows, err)
	if !sqldb.IsNotFound(err) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}

func TestScanOneOpt_NilOnMissing(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	b := query.Select(idCol, nameCol).From(usersT).Where(idCol.EQ(999))
	rows, err := db.Query(ctx, b)
	user, err := sqldb.ScanOneOpt[UserRow](rows, err)
	if err != nil {
		t.Fatalf("ScanOneOpt returned error: %v", err)
	}
	if user != nil {
		t.Fatalf("expected nil, got %+v", user)
	}
}

func TestFromSelectOpt_Found(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	b := query.Select(idCol, nameCol).From(usersT).Where(idCol.EQ(2))
	user, err := sqldb.FromSelectOpt[UserRow](ctx, db, b)
	if err != nil {
		t.Fatalf("FromSelectOpt: %v", err)
	}
	if user == nil || user.Name != "Bob" {
		t.Fatalf("unexpected result: %+v", user)
	}
}

func TestExec_Insert(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	ins := query.InsertInto(usersT).
		Values(userInsert{ID: 3, Name: "Carol"})
	n, err := db.Exec(ctx, ins)
	if err != nil {
		t.Fatalf("Exec insert: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row affected, got %d", n)
	}

	// Verify Carol is actually in the table.
	b := query.Select(idCol, nameCol).From(usersT).Where(idCol.EQ(3))
	user, err := sqldb.FromSelectOne[UserRow](ctx, db, b)
	if err != nil {
		t.Fatalf("verify insert: %v", err)
	}
	if user.Name != "Carol" {
		t.Fatalf("expected Carol, got %s", user.Name)
	}
}

func TestExec_Update(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	upd := query.Update(usersT).Set("name", "Alicia").Where(idCol.EQ(1))
	n, err := db.Exec(ctx, upd)
	if err != nil {
		t.Fatalf("Exec update: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row affected, got %d", n)
	}
}

func TestExec_Delete(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	del := query.DeleteFrom(usersT).Where(idCol.EQ(2))
	n, err := db.Exec(ctx, del)
	if err != nil {
		t.Fatalf("Exec delete: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row affected, got %d", n)
	}
}

func TestTransaction_Commit(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	err := db.Transaction(ctx, func(tx *sqldb.Tx) error {
		ins := query.InsertInto(usersT).Values(userInsert{ID: 10, Name: "Dave"})
		_, err := tx.Exec(ctx, ins)
		return err
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}

	// Verify Dave exists after commit.
	b := query.Select(idCol, nameCol).From(usersT).Where(idCol.EQ(10))
	user, err := sqldb.FromSelectOne[UserRow](ctx, db, b)
	if err != nil {
		t.Fatalf("verify commit: %v", err)
	}
	if user.Name != "Dave" {
		t.Fatalf("expected Dave, got %s", user.Name)
	}
}

func TestTransaction_Rollback(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	err := db.Transaction(ctx, func(tx *sqldb.Tx) error {
		ins := query.InsertInto(usersT).Values(userInsert{ID: 11, Name: "Eve"})
		if _, err := tx.Exec(ctx, ins); err != nil {
			return err
		}
		return gosql.ErrConnDone // simulate failure → rollback
	})
	if err == nil {
		t.Fatal("expected error from rollback transaction")
	}

	// Eve must NOT be in the table.
	b := query.Select(idCol, nameCol).From(usersT).Where(idCol.EQ(11))
	opt, err := sqldb.FromSelectOpt[UserRow](ctx, db, b)
	if err != nil {
		t.Fatalf("verify rollback: %v", err)
	}
	if opt != nil {
		t.Fatalf("Eve should not exist after rollback: %+v", opt)
	}
}

func TestQueryRaw_ExecRaw(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	// ExecRaw
	n, err := db.ExecRaw(ctx, `INSERT INTO users (id, name) VALUES (?, ?)`, 20, "Frank")
	if err != nil {
		t.Fatalf("ExecRaw: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row affected, got %d", n)
	}

	// QueryRaw
	rows, err := db.QueryRaw(ctx, `SELECT id, name FROM users WHERE id = ?`, 20)
	users, err := sqldb.ScanAll[UserRow](rows, err)
	if err != nil {
		t.Fatalf("ScanAll after QueryRaw: %v", err)
	}
	if len(users) != 1 || users[0].Name != "Frank" {
		t.Fatalf("unexpected result: %+v", users)
	}
}

func TestIsNotFound(t *testing.T) {
	if !sqldb.IsNotFound(gosql.ErrNoRows) {
		t.Fatal("IsNotFound should return true for sql.ErrNoRows")
	}
	if sqldb.IsNotFound(nil) {
		t.Fatal("IsNotFound should return false for nil")
	}
}
