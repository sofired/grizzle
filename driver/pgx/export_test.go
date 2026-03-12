// export_test.go exposes internal helpers exclusively for use in tests.
// This file is only compiled during `go test`.
package pgx

import "github.com/jackc/pgx/v5"

// NewTxForTest constructs a *Tx wrapping the supplied pgx.Tx. Intended for
// unit tests that need to exercise Tx methods without a live database.
func NewTxForTest(inner pgx.Tx) *Tx {
	return &Tx{tx: inner}
}
