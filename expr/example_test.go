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

// ExampleRawArgs demonstrates a parameterised raw SQL fragment using $? placeholders.
// Each $? is replaced in order with the dialect's native placeholder ($1, $2, … for
// PostgreSQL; ? for MySQL/SQLite) and the corresponding value is bound safely.
func ExampleRawArgs() {
	e := expr.RawArgs("tsv @@ websearch_to_tsquery($?)", "grizzle ORM")
	ctx := expr.NewBuildContext(dialect.Postgres)
	fmt.Println(e.ToSQL(ctx))
	fmt.Println(ctx.Args())
	// Output:
	// tsv @@ websearch_to_tsquery($1)
	// [grizzle ORM]
}

// ExampleRawArgs_multipleParams demonstrates binding multiple parameters in a single
// RawArgs expression. Placeholders are numbered sequentially in the active context.
func ExampleRawArgs_multipleParams() {
	latDelta := 0.5
	lonDelta := 0.5
	lat := 37.7749
	lon := -122.4194
	e := expr.RawArgs(
		"lat BETWEEN $? AND $? AND lon BETWEEN $? AND $?",
		lat-latDelta, lat+latDelta, lon-lonDelta, lon+lonDelta,
	)
	ctx := expr.NewBuildContext(dialect.Postgres)
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// lat BETWEEN $1 AND $2 AND lon BETWEEN $3 AND $4
}

// ExampleRawArgs_composedWithAnd demonstrates RawArgs composing with And alongside
// ordinary column expressions. Placeholder numbering continues from the context.
func ExampleRawArgs_composedWithAnd() {
	cond := expr.And(
		ts.UsersT.Enabled.IsTrue(),
		expr.RawArgs("tsv @@ websearch_to_tsquery($?)", "grizzle ORM"),
	)
	ctx := expr.NewBuildContext(dialect.Postgres)
	fmt.Println(cond.ToSQL(ctx))
	// Output:
	// ("users"."enabled" = $1 AND tsv @@ websearch_to_tsquery($2))
}

// ExampleRawArgs_mysql demonstrates that RawArgs uses ? placeholders for MySQL.
func ExampleRawArgs_mysql() {
	e := expr.RawArgs("ST_Distance($?, $?) < $?", 37.7749, -122.4194, 10.0)
	ctx := expr.NewBuildContext(dialect.MySQL)
	fmt.Println(e.ToSQL(ctx))
	// Output:
	// ST_Distance(?, ?) < ?
}
