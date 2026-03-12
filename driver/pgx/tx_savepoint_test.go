package pgx_test

import (
	"context"
	"errors"
	"testing"

	pgxdb "github.com/sofired/grizzle/driver/pgx"
)

// TestValidateSavepointName exercises the exported behaviour through the
// Savepoint method: valid names must be accepted and invalid ones rejected
// without touching the database.
func TestSavepoint_InvalidName(t *testing.T) {
	tests := []struct {
		name      string
		spName    string
		wantError bool
	}{
		// Valid names
		{"simple", "mysp", false},
		{"with underscore prefix", "_sp", false},
		{"with digits", "sp_123", false},
		{"max length 63", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"[:63], false},
		// Invalid names
		{"starts with digit", "1sp", true},
		{"contains space", "my sp", true},
		{"contains hyphen", "my-sp", true},
		{"contains semicolon", "sp;drop", true},
		{"contains quote", "sp'name", true},
		{"empty string", "", true},
		{"too long (64 chars)", "a" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"[:63], true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx := newFakeTx(nil)
			err := tx.Savepoint(context.Background(), tc.spName)
			if tc.wantError && err == nil {
				t.Errorf("Savepoint(%q): expected error, got nil", tc.spName)
			}
			if !tc.wantError && err != nil {
				t.Errorf("Savepoint(%q): unexpected error: %v", tc.spName, err)
			}
		})
	}
}

func TestRollbackToSavepoint_InvalidName(t *testing.T) {
	tx := newFakeTx(nil)
	err := tx.RollbackToSavepoint(context.Background(), "bad name")
	if err == nil {
		t.Error("RollbackToSavepoint with invalid name: expected error, got nil")
	}
}

func TestReleaseSavepoint_InvalidName(t *testing.T) {
	tx := newFakeTx(nil)
	err := tx.ReleaseSavepoint(context.Background(), "bad name")
	if err == nil {
		t.Error("ReleaseSavepoint with invalid name: expected error, got nil")
	}
}

// TestSavepointSQL verifies the SQL strings issued by Savepoint,
// RollbackToSavepoint, and ReleaseSavepoint.
func TestSavepointSQL(t *testing.T) {
	var execCalls []string
	fake := newFakeTxWithExec(func(sql string) error {
		execCalls = append(execCalls, sql)
		return nil
	})

	ctx := context.Background()
	if err := fake.Savepoint(ctx, "sp_a"); err != nil {
		t.Fatalf("Savepoint: %v", err)
	}
	if err := fake.RollbackToSavepoint(ctx, "sp_a"); err != nil {
		t.Fatalf("RollbackToSavepoint: %v", err)
	}
	if err := fake.ReleaseSavepoint(ctx, "sp_a"); err != nil {
		t.Fatalf("ReleaseSavepoint: %v", err)
	}

	want := []string{
		"SAVEPOINT sp_a",
		"ROLLBACK TO SAVEPOINT sp_a",
		"RELEASE SAVEPOINT sp_a",
	}
	if len(execCalls) != len(want) {
		t.Fatalf("got %d exec calls, want %d: %v", len(execCalls), len(want), execCalls)
	}
	for i, w := range want {
		if execCalls[i] != w {
			t.Errorf("exec[%d] = %q, want %q", i, execCalls[i], w)
		}
	}
}

// TestNestedTransaction_Success verifies that NestedTransaction issues
// SAVEPOINT … RELEASE SAVEPOINT when fn succeeds.
func TestNestedTransaction_Success(t *testing.T) {
	var execCalls []string
	fake := newFakeTxWithExec(func(sql string) error {
		execCalls = append(execCalls, sql)
		return nil
	})

	err := fake.NestedTransaction(context.Background(), func(_ *pgxdb.Tx) error {
		return nil
	})
	if err != nil {
		t.Fatalf("NestedTransaction: unexpected error: %v", err)
	}

	if len(execCalls) != 2 {
		t.Fatalf("expected 2 exec calls, got %d: %v", len(execCalls), execCalls)
	}
	if execCalls[0] != "SAVEPOINT sp_1" {
		t.Errorf("first call = %q, want %q", execCalls[0], "SAVEPOINT sp_1")
	}
	if execCalls[1] != "RELEASE SAVEPOINT sp_1" {
		t.Errorf("second call = %q, want %q", execCalls[1], "RELEASE SAVEPOINT sp_1")
	}
}

// TestNestedTransaction_Rollback verifies that NestedTransaction rolls back
// to the savepoint when fn returns an error, and propagates that error.
func TestNestedTransaction_Rollback(t *testing.T) {
	var execCalls []string
	fake := newFakeTxWithExec(func(sql string) error {
		execCalls = append(execCalls, sql)
		return nil
	})

	sentinel := errors.New("intentional failure")
	err := fake.NestedTransaction(context.Background(), func(_ *pgxdb.Tx) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("NestedTransaction: want sentinel error, got %v", err)
	}

	if len(execCalls) != 2 {
		t.Fatalf("expected 2 exec calls, got %d: %v", len(execCalls), execCalls)
	}
	if execCalls[0] != "SAVEPOINT sp_1" {
		t.Errorf("first call = %q, want %q", execCalls[0], "SAVEPOINT sp_1")
	}
	if execCalls[1] != "ROLLBACK TO SAVEPOINT sp_1" {
		t.Errorf("second call = %q, want %q", execCalls[1], "ROLLBACK TO SAVEPOINT sp_1")
	}
}

// TestNestedTransaction_AutoIncrementingNames verifies that successive
// NestedTransaction calls use unique, incrementing savepoint names.
func TestNestedTransaction_AutoIncrementingNames(t *testing.T) {
	var execCalls []string
	fake := newFakeTxWithExec(func(sql string) error {
		execCalls = append(execCalls, sql)
		return nil
	})

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := fake.NestedTransaction(ctx, func(_ *pgxdb.Tx) error { return nil }); err != nil {
			t.Fatalf("NestedTransaction call %d: %v", i+1, err)
		}
	}

	// Each call: SAVEPOINT sp_N + RELEASE SAVEPOINT sp_N → 6 calls total.
	if len(execCalls) != 6 {
		t.Fatalf("expected 6 exec calls, got %d: %v", len(execCalls), execCalls)
	}
	wantSavepoints := []string{"SAVEPOINT sp_1", "SAVEPOINT sp_2", "SAVEPOINT sp_3"}
	for i, want := range wantSavepoints {
		if execCalls[i*2] != want {
			t.Errorf("exec[%d] = %q, want %q", i*2, execCalls[i*2], want)
		}
	}
}

// TestNestedTransaction_PartialRollback exercises the documented use case:
// a child operation fails but the outer "transaction" (represented by
// additional fake exec calls) continues unaffected.
func TestNestedTransaction_PartialRollback(t *testing.T) {
	var execCalls []string
	fake := newFakeTxWithExec(func(sql string) error {
		execCalls = append(execCalls, sql)
		return nil
	})

	ctx := context.Background()

	// Simulate outer operation (tracked separately via the call log).
	execCalls = append(execCalls, "INSERT parent")

	dupErr := errors.New("duplicate key")
	nestedErr := fake.NestedTransaction(ctx, func(_ *pgxdb.Tx) error {
		execCalls = append(execCalls, "INSERT child")
		return dupErr
	})
	if !errors.Is(nestedErr, dupErr) {
		t.Fatalf("expected dupErr, got %v", nestedErr)
	}

	// After rollback to savepoint, outer transaction is still usable; simulate
	// another outer operation.
	execCalls = append(execCalls, "INSERT other")

	wantSeq := []string{
		"INSERT parent",
		"SAVEPOINT sp_1",
		"INSERT child",
		"ROLLBACK TO SAVEPOINT sp_1",
		"INSERT other",
	}
	if len(execCalls) != len(wantSeq) {
		t.Fatalf("exec call count = %d, want %d: %v", len(execCalls), len(wantSeq), execCalls)
	}
	for i, want := range wantSeq {
		if execCalls[i] != want {
			t.Errorf("exec[%d] = %q, want %q", i, execCalls[i], want)
		}
	}
}
