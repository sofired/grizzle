package pgx_test

import (
	"context"
	"fmt"
	"log"

	pgxdb "github.com/sofired/grizzle/driver/pgx"
	ts "github.com/sofired/grizzle/internal/testschema"
	"github.com/sofired/grizzle/query"
)

// db is a package-level variable used by examples.
// In production, initialise it with pgxdb.New(pool).
var db *pgxdb.DB

// ExampleScanAll shows the two-step pattern: build a query with the query
// package, execute it with db.Query, then collect results with ScanAll.
func ExampleScanAll() {
	ctx := context.Background()

	rows, err := db.Query(ctx,
		query.Select().
			From(ts.UsersT).
			Where(ts.UsersT.DeletedAt.IsNull()),
	)
	users, err := pgxdb.ScanAll[ts.UserSelect](rows, err)
	if err != nil {
		log.Fatal(err)
	}
	for _, u := range users {
		fmt.Println(u.Username)
	}
}

// ExampleFromSelect shows the one-call helper that combines db.Query and
// ScanAll into a single expression.
func ExampleFromSelect() {
	ctx := context.Background()

	users, err := pgxdb.FromSelect[ts.UserSelect](ctx, db,
		query.Select(ts.UsersT.ID, ts.UsersT.Username).
			From(ts.UsersT).
			Where(ts.UsersT.DeletedAt.IsNull()).
			OrderBy(ts.UsersT.Username.Asc()),
	)
	if err != nil {
		log.Fatal(err)
	}
	for _, u := range users {
		fmt.Println(u.Username)
	}
}

// ExampleScanOneOpt shows a nullable lookup: returns nil when no row is found
// rather than returning pgx.ErrNoRows.
func ExampleScanOneOpt() {
	ctx := context.Background()

	rows, err := db.Query(ctx,
		query.Select().
			From(ts.UsersT).
			Where(ts.UsersT.Username.EQ("alice")).
			Limit(1),
	)
	user, err := pgxdb.ScanOneOpt[ts.UserSelect](rows, err)
	if err != nil {
		log.Fatal(err)
	}
	if user == nil {
		fmt.Println("not found")
		return
	}
	fmt.Println(user.Username)
}

// ExampleBatch_exec demonstrates sending multiple mutation statements in a
// single round-trip. This is more efficient than issuing each statement
// separately because all SQL is pipelined to PostgreSQL at once.
//
// Use Queue for INSERT / UPDATE / DELETE and read rows-affected via
// BatchResults.Exec in the same order.
func ExampleBatch_exec() {
	ctx := context.Background()

	batch := db.NewBatch()
	batch.Queue(query.Update(ts.UsersT).
		Set("enabled", false).
		Where(ts.UsersT.DeletedAt.IsNotNull()))
	batch.Queue(query.Update(ts.UsersT).
		Set("purged_at", nil).
		Where(ts.UsersT.DeletedAt.IsNull()))

	results, err := batch.Send(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer results.Close()

	n1, err := results.Exec()
	if err != nil {
		log.Fatal(err)
	}
	n2, err := results.Exec()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("disabled %d users, cleared purge flag on %d users\n", n1, n2)
}

// ExampleBatch_query demonstrates queuing multiple SELECT statements and
// collecting typed results for each in order using ScanAll.
func ExampleBatch_query() {
	ctx := context.Background()

	batch := db.NewBatch()
	batch.QueueQuery(query.Select().From(ts.UsersT).Where(ts.UsersT.Enabled.IsTrue()))
	batch.QueueQuery(query.Select().From(ts.UsersT).Where(ts.UsersT.DeletedAt.IsNull()))

	results, err := batch.Send(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer results.Close()

	activeUsers, err := pgxdb.ScanAll[ts.UserSelect](results.Query())
	if err != nil {
		log.Fatal(err)
	}
	nonDeletedUsers, err := pgxdb.ScanAll[ts.UserSelect](results.Query())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("active: %d, non-deleted: %d\n", len(activeUsers), len(nonDeletedUsers))
}

// ExampleBatch_transaction shows batching inside a transaction — all
// statements are sent in one round-trip and participate in the same
// transaction.
func ExampleBatch_transaction() {
	ctx := context.Background()

	err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
		batch := tx.NewBatch()
		batch.Queue(query.Update(ts.UsersT).
			Set("enabled", false).
			Where(ts.UsersT.DeletedAt.IsNotNull()))
		batch.Queue(query.Update(ts.RealmsT).
			Set("enabled", false).
			Where(ts.RealmsT.Enabled.IsTrue()))

		results, err := batch.Send(ctx)
		if err != nil {
			return err
		}
		defer results.Close()

		if _, err := results.Exec(); err != nil {
			return err
		}
		if _, err := results.Exec(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

// ExampleDB_Transaction shows the transaction callback pattern.
// Returning a non-nil error from fn automatically rolls back the transaction.
func ExampleDB_Transaction() {
	ctx := context.Background()

	err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
		_, err := tx.Exec(ctx, query.Update(ts.UsersT).
			Set("enabled", false).
			Where(ts.UsersT.DeletedAt.IsNotNull()))
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
}
