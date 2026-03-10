package expr_test

import (
	"fmt"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	ts "github.com/sofired/grizzle/internal/testschema"
)

// ExampleAnd demonstrates nil-safe AND: nil expressions are silently dropped,
// making dynamic WHERE clauses safe to construct without explicit nil checks.
func ExampleAnd() {
	var emailFilter expr.Expression // nil — not filtering by email this time

	cond := expr.And(
		ts.UsersT.DeletedAt.IsNull(),
		ts.UsersT.Enabled.IsTrue(),
		emailFilter, // nil — silently dropped
	)
	ctx := expr.NewBuildContext(dialect.Postgres)
	fmt.Println(cond.ToSQL(ctx))
	// Output:
	// ("users"."deleted_at" IS NULL AND "users"."enabled" = $1)
}

// ExampleOr demonstrates combining conditions with OR.
func ExampleOr() {
	cond := expr.Or(
		ts.UsersT.Email.IsNull(),
		ts.UsersT.Email.EQ("alice@example.com"),
	)
	ctx := expr.NewBuildContext(dialect.Postgres)
	fmt.Println(cond.ToSQL(ctx))
	// Output:
	// ("users"."email" IS NULL OR "users"."email" = $1)
}

// ExampleNot demonstrates negating an expression.
func ExampleNot() {
	cond := expr.Not(ts.UsersT.DeletedAt.IsNull())
	ctx := expr.NewBuildContext(dialect.Postgres)
	fmt.Println(cond.ToSQL(ctx))
	// Output:
	// NOT ("users"."deleted_at" IS NULL)
}

// ExampleCase demonstrates a searched CASE expression with WHEN/THEN/ELSE branches.
// Use Lit to wrap Go values as bound parameters in THEN and ELSE clauses.
func ExampleCase() {
	status := expr.Case().
		When(ts.UsersT.DeletedAt.IsNotNull(), expr.Lit("deleted")).
		When(ts.UsersT.Enabled.IsTrue(), expr.Lit("active")).
		Else(expr.Lit("inactive")).
		As("status")
	ctx := expr.NewBuildContext(dialect.Postgres)
	fmt.Println(status.ToSQL(ctx))
	// Output:
	// CASE WHEN "users"."deleted_at" IS NOT NULL THEN $1 WHEN "users"."enabled" = $2 THEN $3 ELSE $4 END AS "status"
}

// ExampleLit demonstrates wrapping a Go value as a bound-parameter expression.
// Use Lit in THEN/ELSE clauses of a CASE expression, or anywhere a literal value
// needs to participate as an Expression rather than as a typed column argument.
func ExampleLit() {
	v := expr.Lit(42)
	ctx := expr.NewBuildContext(dialect.Postgres)
	fmt.Println(v.ToSQL(ctx))
	// Output:
	// $1
}

// ExampleRaw demonstrates embedding a raw SQL fragment without parameter binding.
// Use sparingly and never with user-controlled input — no escaping is applied.
func ExampleRaw() {
	e := expr.Raw("now()")
	ctx := expr.NewBuildContext(dialect.Postgres)
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// now()
}
