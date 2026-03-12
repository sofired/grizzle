package pgx_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxdb "github.com/sofired/grizzle/driver/pgx"
	ts "github.com/sofired/grizzle/internal/testschema"
	"github.com/sofired/grizzle/query"
)

// stubBatchResults is a minimal pgx.BatchResults mock for unit tests.
type stubBatchResults struct{}

func (s *stubBatchResults) Exec() (pgconn.CommandTag, error)  { return pgconn.CommandTag{}, nil }
func (s *stubBatchResults) Query() (pgx.Rows, error)          { return nil, nil }
func (s *stubBatchResults) QueryRow() pgx.Row                 { return nil }
func (s *stubBatchResults) Close() error                      { return nil }

// TestBatch_QueueBuildsSQL verifies that Queue / QueueQuery / QueueRaw all
// accept valid input and that the batch length is tracked correctly.
func TestBatch_QueueBuildsSQL(t *testing.T) {
	// NewBatch with a nil DB is safe as long as Send is never called.
	b := pgxdb.NewBatchForTest()

	b.Queue(query.Update(ts.UsersT).Set("enabled", false).Where(ts.UsersT.DeletedAt.IsNotNull()))
	if b.Len() != 1 {
		t.Fatalf("Len() = %d, want 1 after Queue", b.Len())
	}

	b.QueueQuery(query.Select().From(ts.UsersT).Where(ts.UsersT.DeletedAt.IsNull()))
	if b.Len() != 2 {
		t.Fatalf("Len() = %d, want 2 after QueueQuery", b.Len())
	}

	b.QueueRaw("SELECT 1")
	if b.Len() != 3 {
		t.Fatalf("Len() = %d, want 3 after QueueRaw", b.Len())
	}

	b.QueueRawQuery("SELECT id FROM users WHERE enabled = $1", true)
	if b.Len() != 4 {
		t.Fatalf("Len() = %d, want 4 after QueueRawQuery", b.Len())
	}
}

// TestBatch_SQLContent verifies that Queue correctly builds SQL from builders
// and that the generated SQL contains the expected fragments.
func TestBatch_SQLContent(t *testing.T) {
	b := pgxdb.NewBatchForTest()

	b.Queue(query.Update(ts.UsersT).
		Set("enabled", false).
		Where(ts.UsersT.ID.EQ(uuid.MustParse("00000000-0000-0000-0000-000000000001"))))

	b.QueueQuery(query.Select(ts.UsersT.ID, ts.UsersT.Username).
		From(ts.UsersT).
		Where(ts.UsersT.DeletedAt.IsNull()).
		OrderBy(ts.UsersT.CreatedAt.Desc()))

	entries := b.Entries()

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	updateSQL := entries[0].SQL
	for _, want := range []string{`UPDATE "users"`, `SET "enabled"`, `WHERE`} {
		if !strings.Contains(updateSQL, want) {
			t.Errorf("update SQL missing %q\ngot: %s", want, updateSQL)
		}
	}
	if entries[0].IsQuery {
		t.Error("Queue entry should have IsQuery=false")
	}

	selectSQL := entries[1].SQL
	for _, want := range []string{
		`"users"."id"`,
		`"users"."username"`,
		`FROM "users"`,
		`"users"."deleted_at" IS NULL`,
		`ORDER BY "users"."created_at" DESC`,
	} {
		if !strings.Contains(selectSQL, want) {
			t.Errorf("select SQL missing %q\ngot: %s", want, selectSQL)
		}
	}
	if !entries[1].IsQuery {
		t.Error("QueueQuery entry should have IsQuery=true")
	}
}

// TestBatch_SendEmptyReturnsError ensures that calling Send on an empty Batch
// returns a descriptive error rather than sending a no-op to the database.
func TestBatch_SendEmptyReturnsError(t *testing.T) {
	b := pgxdb.NewBatchForTest()

	_, err := b.Send(nil) //nolint:staticcheck // intentional nil ctx for unit test
	if err == nil {
		t.Fatal("expected error when sending empty batch, got nil")
	}
	if !strings.Contains(err.Error(), "no queued statements") {
		t.Errorf("expected 'no queued statements' in error, got: %v", err)
	}
}

// TestBatch_SendWithNoPoolReturnsError ensures that calling Send on a
// NewBatchForTest batch (which has no pool) returns a descriptive error
// rather than panicking.
func TestBatch_SendWithNoPoolReturnsError(t *testing.T) {
	b := pgxdb.NewBatchForTest()
	b.QueueRaw("SELECT 1")

	_, err := b.Send(nil) //nolint:staticcheck // intentional nil ctx for unit test
	if err == nil {
		t.Fatal("expected error when sending batch with no pool, got nil")
	}
	if !strings.Contains(err.Error(), "no pool") {
		t.Errorf("expected 'no pool' in error, got: %v", err)
	}
}

// TestBatch_QueueRawArgs verifies that args passed to QueueRaw are stored.
func TestBatch_QueueRawArgs(t *testing.T) {
	b := pgxdb.NewBatchForTest()
	b.QueueRaw("UPDATE users SET enabled = $1 WHERE id = $2", false, "some-id")

	entries := b.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].SQL != "UPDATE users SET enabled = $1 WHERE id = $2" {
		t.Errorf("unexpected SQL: %s", entries[0].SQL)
	}
	if len(entries[0].Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(entries[0].Args))
	}
}

// TestBatch_MixedQueueTypes verifies that exec and query entries can be
// freely interleaved.
func TestBatch_MixedQueueTypes(t *testing.T) {
	b := pgxdb.NewBatchForTest()
	b.Queue(query.Update(ts.UsersT).Set("enabled", false).Where(ts.UsersT.DeletedAt.IsNotNull()))
	b.QueueQuery(query.Select().From(ts.RealmsT))
	b.QueueRaw("DELETE FROM users WHERE purged_at IS NOT NULL")
	b.QueueRawQuery("SELECT count(*) FROM realms")

	entries := b.Entries()
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	wantIsQuery := []bool{false, true, false, true}
	for i, e := range entries {
		if e.IsQuery != wantIsQuery[i] {
			t.Errorf("entries[%d].IsQuery = %v, want %v", i, e.IsQuery, wantIsQuery[i])
		}
	}
}

// TestBatchResults_ExecRejectsQueryEntry verifies that calling Exec when the
// next entry is a query entry returns an error and does not advance idx.
func TestBatchResults_ExecRejectsQueryEntry(t *testing.T) {
	entries := []pgxdb.BatchEntry{
		{SQL: "SELECT 1", Args: nil, IsQuery: true},
	}
	r := pgxdb.NewBatchResultsForTest(&stubBatchResults{}, entries)

	_, err := r.Exec()
	if err == nil {
		t.Fatal("expected error when calling Exec on a query entry, got nil")
	}
	if !strings.Contains(err.Error(), "query entry") {
		t.Errorf("expected error to mention 'query entry', got: %v", err)
	}
}

// TestBatchResults_QueryRejectsExecEntry verifies that calling Query when the
// next entry is an exec entry returns an error and does not advance idx.
func TestBatchResults_QueryRejectsExecEntry(t *testing.T) {
	entries := []pgxdb.BatchEntry{
		{SQL: "UPDATE users SET enabled = false", Args: nil, IsQuery: false},
	}
	r := pgxdb.NewBatchResultsForTest(&stubBatchResults{}, entries)

	_, err := r.Query()
	if err == nil {
		t.Fatal("expected error when calling Query on an exec entry, got nil")
	}
	if !strings.Contains(err.Error(), "exec entry") {
		t.Errorf("expected error to mention 'exec entry', got: %v", err)
	}
}

// TestBatchResults_ExecCommandTagRejectsQueryEntry verifies that calling
// ExecCommandTag when the next entry is a query entry returns an error.
func TestBatchResults_ExecCommandTagRejectsQueryEntry(t *testing.T) {
	entries := []pgxdb.BatchEntry{
		{SQL: "SELECT id FROM users", Args: nil, IsQuery: true},
	}
	r := pgxdb.NewBatchResultsForTest(&stubBatchResults{}, entries)

	_, err := r.ExecCommandTag()
	if err == nil {
		t.Fatal("expected error when calling ExecCommandTag on a query entry, got nil")
	}
	if !strings.Contains(err.Error(), "query entry") {
		t.Errorf("expected error to mention 'query entry', got: %v", err)
	}
}

// TestBatchResults_IdxNotAdvancedOnTypeError verifies that idx is not advanced
// when an entry-type mismatch error is returned, so the caller can call the
// correct method on the same entry.
func TestBatchResults_IdxNotAdvancedOnTypeError(t *testing.T) {
	entries := []pgxdb.BatchEntry{
		{SQL: "SELECT 1", Args: nil, IsQuery: true},
		{SQL: "SELECT 2", Args: nil, IsQuery: true},
	}
	r := pgxdb.NewBatchResultsForTest(&stubBatchResults{}, entries)

	// Calling Exec on a query entry should fail.
	_, err := r.Exec()
	if err == nil {
		t.Fatal("expected error on first Exec (query entry), got nil")
	}

	// idx must not have advanced; calling Query for the same entry must succeed.
	_, err = r.Query()
	if err != nil {
		t.Errorf("expected Query to succeed after failed Exec, got: %v", err)
	}
}

// TestBatch_EntriesArgsAreCopied verifies that mutating the Args slice returned
// by Entries does not affect the Batch's internal state.
func TestBatch_EntriesArgsAreCopied(t *testing.T) {
	b := pgxdb.NewBatchForTest()
	b.QueueRaw("UPDATE users SET enabled = $1 WHERE id = $2", false, "some-id")

	entries := b.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Mutate the returned Args slice.
	entries[0].Args[0] = "mutated"

	// Re-fetch entries; internal state must be unchanged.
	fresh := b.Entries()
	if fresh[0].Args[0] != false {
		t.Errorf("expected internal arg to remain false, got: %v", fresh[0].Args[0])
	}
}
