package kit_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sofired/grizzle/kit"
	pg "github.com/sofired/grizzle/schema/pg"
)

// fkIntegrationDSN returns the DSN for the integration test database.
// Set GRIZZLE_TEST_PG_DSN to override.
func fkIntegrationDSN() string {
	if v := os.Getenv("GRIZZLE_TEST_PG_DSN"); v != "" {
		return v
	}
	return "postgres://grizzle:grizzle@localhost:5444/grizzle_test"
}

var intRealmsTable = pg.Table("realms",
	pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("name", pg.Varchar(255).NotNull()),
).Build()

var intUsersWithFK = pg.Table("users",
	pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("realm_id", pg.UUID().NotNull()),
	pg.C("username", pg.Varchar(255).NotNull()),
).WithConstraints(func(t pg.TableRef) []pg.Constraint {
	return []pg.Constraint{
		pg.ForeignKey("fk_users_realm_id").
			From("realm_id").
			References("realms", "id").
			OnDelete("CASCADE").
			Build(),
	}
})

var intUsersWithoutFK = pg.Table("users",
	pg.C("id", pg.UUID().PrimaryKey().DefaultRandom()),
	pg.C("realm_id", pg.UUID().NotNull()),
	pg.C("username", pg.Varchar(255).NotNull()),
).Build()

func newIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := fkIntegrationDSN()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("cannot connect to postgres (%v) — set GRIZZLE_TEST_PG_DSN to enable", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("postgres not reachable: %v", err)
	}
	t.Cleanup(func() {
		dropTables(t, pool, "users", "realms")
		pool.Close()
	})
	return pool
}

// dropTables cleans up test tables in dependency order.
func dropTables(t *testing.T, pool *pgxpool.Pool, tables ...string) {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range tables {
		if _, err := pool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %q CASCADE`, tbl)); err != nil {
			t.Logf("cleanup: failed to drop %q: %v", tbl, err)
		}
	}
}

// TestFKIntegration_Push_NoDuplicateOnRepeat verifies:
// Push with FK → inspect pg_constraint → Push again → no duplicate error, no changes.
func TestFKIntegration_Push_NoDuplicateOnRepeat(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()

	// Step 1: Push schema with FK.
	t.Log("Step 1: Push schema (realms + users with FK)...")
	r1, err := kit.Push(ctx, pool, intRealmsTable, intUsersWithFK)
	if err != nil {
		t.Fatalf("Push #1 failed: %v", err)
	}
	t.Logf("Push #1: %d change(s)", len(r1.Changes))
	for _, s := range r1.SQL {
		t.Logf("  SQL: %s", s)
	}
	if len(r1.Changes) == 0 {
		t.Fatal("expected changes on first Push but got none")
	}

	// Step 2: Confirm FK exists in pg_constraint.
	t.Log("Step 2: Inspecting pg_constraint for FK...")
	rows, err := pool.Query(ctx,
		`SELECT conname FROM pg_constraint WHERE conrelid = 'users'::regclass AND contype = 'f'`)
	if err != nil {
		t.Fatalf("pg_constraint query: %v", err)
	}
	var fkNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		fkNames = append(fkNames, name)
	}
	rows.Close()
	t.Logf("FK constraints in pg_constraint: %v", fkNames)
	if len(fkNames) == 0 {
		t.Fatal("expected FK constraint in pg_constraint after Push #1")
	}

	// Step 3: Push again — must be a no-op.
	t.Log("Step 3: Push again (must be no-op, no duplicate constraint error)...")
	r2, err := kit.Push(ctx, pool, intRealmsTable, intUsersWithFK)
	if err != nil {
		t.Fatalf("Push #2 failed (duplicate constraint error?): %v", err)
	}
	if len(r2.Changes) != 0 {
		t.Errorf("Push #2 produced %d unexpected change(s):", len(r2.Changes))
		for _, s := range r2.SQL {
			t.Logf("  SQL: %s", s)
		}
	} else {
		t.Log("Push #2: no changes — PASS")
	}
}

// TestFKIntegration_DropFK_EmitsDrop verifies that removing an FK from the
// schema definition causes DryRun to emit DROP CONSTRAINT.
func TestFKIntegration_DropFK_EmitsDrop(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()

	// Push with FK first.
	t.Log("Step 1: Push schema with FK...")
	if _, err := kit.Push(ctx, pool, intRealmsTable, intUsersWithFK); err != nil {
		t.Fatalf("initial Push failed: %v", err)
	}

	// Remove FK from schema definition, DryRun.
	t.Log("Step 2: DryRun with FK removed — expect DROP CONSTRAINT...")
	result, err := kit.DryRun(ctx, pool, intRealmsTable, intUsersWithoutFK)
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	t.Logf("DryRun: %d change(s)", len(result.Changes))
	for _, s := range result.SQL {
		t.Logf("  SQL: %s", s)
	}

	found := false
	for _, s := range result.SQL {
		if strings.Contains(s, "DROP CONSTRAINT") && strings.Contains(s, "fk_users_realm_id") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected DROP CONSTRAINT fk_users_realm_id in DryRun SQL\ngot: %v", result.SQL)
	} else {
		t.Log("DROP CONSTRAINT emitted correctly — PASS")
	}
}

// TestFKIntegration_AddFK_NoReAttempt verifies that a FK that already exists
// in the live database is not re-added on a second Push.
func TestFKIntegration_AddFK_NoReAttempt(t *testing.T) {
	pool := newIntegrationPool(t)
	ctx := context.Background()

	// Push with FK.
	t.Log("Step 1: Push schema with FK...")
	r1, err := kit.Push(ctx, pool, intRealmsTable, intUsersWithFK)
	if err != nil {
		t.Fatalf("Push #1 failed: %v", err)
	}
	t.Logf("Push #1: %d change(s)", len(r1.Changes))

	// Push again — FK already live, must not emit ADD CONSTRAINT.
	t.Log("Step 2: Push again (FK already exists — must not re-add)...")
	r2, err := kit.Push(ctx, pool, intRealmsTable, intUsersWithFK)
	if err != nil {
		t.Fatalf("Push #2 failed (should not re-attempt): %v", err)
	}
	if len(r2.Changes) != 0 {
		t.Errorf("Push #2 produced %d unexpected change(s):", len(r2.Changes))
		for _, s := range r2.SQL {
			t.Logf("  SQL: %s", s)
		}
	} else {
		t.Log("Push #2: no changes — FK not re-attempted — PASS")
	}
}
