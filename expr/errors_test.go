package expr_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
)

func TestError_StableClassificationAndRedaction(t *testing.T) {
	const secret = "raw SQL: SELECT password FROM users"
	err := expr.NewError(expr.CodeBuildValidation, "render_test", "expression is invalid")
	err.Err = errors.New(secret)

	if !errors.Is(err, expr.ErrBuildValidation) {
		t.Fatal("errors.Is did not match ErrBuildValidation")
	}
	var target *expr.Error
	if !errors.As(err, &target) {
		t.Fatal("errors.As did not expose *expr.Error")
	}
	if target.Code != expr.CodeBuildValidation || target.Op != "render_test" {
		t.Fatalf("unexpected error shape: %#v", target)
	}
	for _, rendered := range []string{err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%+v", err), fmt.Sprintf("%q", err)} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("diagnostic leaked unsafe cause: %q", rendered)
		}
	}
}

func TestError_PreservesCodeAndSafeContextSentinel(t *testing.T) {
	err := expr.NewError(expr.CodeBuildValidation, "render_test", "expression is invalid")
	err.Err = context.Canceled
	if !errors.Is(err, expr.ErrBuildValidation) {
		t.Fatal("context error lost stable build classification")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("context error lost cancellation sentinel")
	}
}

func TestBuildContext_QuoteIdentifierContract(t *testing.T) {
	valid := []struct {
		name string
		d    dialect.Dialect
		in   string
		want string
	}{
		{"postgres quote", dialect.Postgres, `a"b`, `"a""b"`},
		{"sqlite quote", dialect.SQLite, `a"b`, `"a""b"`},
		{"mysql quote", dialect.MySQL, "a`b", "`a``b`"},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expr.NewBuildContext(tc.d).Quote(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("Quote() = %q, want %q", got, tc.want)
			}
		})
	}

	invalid := []string{"", "audit.users", "nul\x00byte", "line\nfeed", "carriage\rreturn", "delete\x7fchar", "control\x01char"}
	for _, name := range invalid {
		t.Run(fmt.Sprintf("invalid_%x", name), func(t *testing.T) {
			got, err := expr.NewBuildContext(dialect.Postgres).Quote(name)
			if got != "" {
				t.Fatalf("Quote() rendered unsafe identifier: %q", got)
			}
			if !errors.Is(err, expr.ErrInvalidIdentifier) {
				t.Fatalf("error = %v, want ErrInvalidIdentifier", err)
			}
			if name != "" && strings.Contains(err.Error(), name) {
				t.Fatalf("error leaked identifier: %q", err)
			}
		})
	}
}

func TestCompositeRender_RollsBackArgumentsOnError(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	e := expr.And(
		expr.Lit("sensitive-value"),
		expr.RawArgs("x = $? AND y = $?", 1),
	)
	got, err := e.RenderSQL(ctx)
	if got != "" || !errors.Is(err, expr.ErrBuildValidation) {
		t.Fatalf("got (%q, %v), want empty SQL and ErrBuildValidation", got, err)
	}
	if len(ctx.Args()) != 0 {
		t.Fatalf("failed render retained arguments: %v", ctx.Args())
	}
}

func TestZeroValueExpressions_ReturnBuildValidation(t *testing.T) {
	ctx := expr.NewBuildContext(dialect.Postgres)
	expressions := []expr.Expression{
		expr.AggExpr{},
		expr.FuncExpr{},
		expr.WindowExpr{},
		expr.TsvectorExpr{},
	}
	for _, expression := range expressions {
		got, err := expression.RenderSQL(ctx)
		if got != "" || !errors.Is(err, expr.ErrBuildValidation) {
			t.Fatalf("%T rendered (%q, %v), want empty SQL and ErrBuildValidation", expression, got, err)
		}
	}
	if got, err := (expr.OrderExpr{}).RenderSQL(ctx); got != "" || !errors.Is(err, expr.ErrBuildValidation) {
		t.Fatalf("zero OrderExpr rendered (%q, %v), want empty SQL and ErrBuildValidation", got, err)
	}
}
