package pgx_test

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
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

// ExampleDB_TransactionWithOptions shows how to start a transaction with a
// specific isolation level using pgx.TxOptions.
func ExampleDB_TransactionWithOptions() {
	ctx := context.Background()

	err := db.TransactionWithOptions(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable}, func(tx *pgxdb.Tx) error {
		_, err := tx.Exec(ctx, query.Update(ts.UsersT).
			Set("enabled", false).
			Where(ts.UsersT.DeletedAt.IsNotNull()))
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
}

// ExampleTx_Savepoint shows manual savepoint management inside a transaction.
// Use this when you need fine-grained control over which partial state to roll
// back to before continuing.
func ExampleTx_Savepoint() {
	ctx := context.Background()

	err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
		if err := tx.Savepoint(ctx, "before_child"); err != nil {
			return err
		}

		_, err := tx.Exec(ctx, query.Update(ts.UsersT).
			Set("enabled", false).
			Where(ts.UsersT.DeletedAt.IsNotNull()))
		if err != nil {
			// Roll back only to the savepoint, not the whole transaction.
			if rbErr := tx.RollbackToSavepoint(ctx, "before_child"); rbErr != nil {
				return rbErr
			}
			// continue without this update
			return nil
		}

		return tx.ReleaseSavepoint(ctx, "before_child")
	})
	if err != nil {
		log.Fatal(err)
	}
}

// ExampleTx_NestedTransaction shows the automatic savepoint helper. The
// savepoint is named and managed automatically; on success it is released, on
// failure it is rolled back.
func ExampleTx_NestedTransaction() {
	ctx := context.Background()

	err := db.Transaction(ctx, func(tx *pgxdb.Tx) error {
		nestedErr := tx.NestedTransaction(ctx, func(tx *pgxdb.Tx) error {
			_, err := tx.Exec(ctx, query.Update(ts.UsersT).
				Set("enabled", false).
				Where(ts.UsersT.DeletedAt.IsNotNull()))
			return err
		})
		if nestedErr != nil {
			// Nested transaction was rolled back; outer transaction is intact.
			fmt.Println("nested op failed, continuing:", nestedErr)
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
