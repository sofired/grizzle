// Package parser provides AST-based parsing of G-rizzle schema definitions.
// It reads Go source files containing pg.Table(...) declarations and extracts
// structured TableDef information without executing the schema code.
package parser

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
)

// MethodCall represents one call in a builder chain: .MethodName(arg1, arg2)
type MethodCall struct {
	Name string
	Args []any // string, int64, float64, bool, or nil for no args
}

// ChainResult is the parsed representation of a column builder expression.
// For: pg.UUID().PrimaryKey().DefaultRandom()
//
//	BasePkg  = "pg"
//	BaseFn   = "UUID"
//	BaseArgs = []
//	Methods  = [{Name:"PrimaryKey"}, {Name:"DefaultRandom"}]
type ChainResult struct {
	BasePkg  string
	BaseFn   string
	BaseArgs []any
	Methods  []MethodCall
}

// UnwrapChain decomposes a chained call expression like:
//
//	pg.Varchar(255).NotNull().Default("foo")
//
// into a ChainResult. Returns an error if the expression is not a
// recognizable chain starting with a qualified call (pkg.Fn(...)).
func UnwrapChain(expr ast.Expr) (*ChainResult, error) {
	var methods []MethodCall

	// Walk the chain from outermost (rightmost) call inward.
	cur := expr
	for {
		call, ok := cur.(*ast.CallExpr)
		if !ok {
			return nil, fmt.Errorf("expected call expression, got %T", cur)
		}

		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			args, err := evalArgs(call.Args)
			if err != nil {
				return nil, fmt.Errorf("in .%s args: %w", fn.Sel.Name, err)
			}

			switch recv := fn.X.(type) {
			case *ast.Ident:
				// This is pkg.Fn(...) — the base of the chain.
				baseArgs, err := evalArgs(call.Args)
				if err != nil {
					return nil, fmt.Errorf("in %s.%s args: %w", recv.Name, fn.Sel.Name, err)
				}
				// Reverse collected methods (we walked right-to-left).
				reverseMethodCalls(methods)
				return &ChainResult{
					BasePkg:  recv.Name,
					BaseFn:   fn.Sel.Name,
					BaseArgs: baseArgs,
					Methods:  methods,
				}, nil

			default:
				// This is a method call on a nested expression: <expr>.Method(...)
				methods = append(methods, MethodCall{Name: fn.Sel.Name, Args: args})
				cur = fn.X
			}

		default:
			return nil, fmt.Errorf("unexpected function expression type %T in chain", call.Fun)
		}
	}
}

// evalArgs converts a slice of AST argument expressions into Go values.
// Supported literals: string, int, float, bool, nil, negative numbers.
func evalArgs(exprs []ast.Expr) ([]any, error) {
	if len(exprs) == 0 {
		return nil, nil
	}
	out := make([]any, 0, len(exprs))
	for _, e := range exprs {
		v, err := evalLiteral(e)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func evalLiteral(e ast.Expr) (any, error) {
	switch x := e.(type) {
	case *ast.BasicLit:
		switch x.Kind {
		case token.STRING:
			s, err := strconv.Unquote(x.Value)
			if err != nil {
				return nil, fmt.Errorf("unquote string %q: %w", x.Value, err)
			}
			return s, nil
		case token.INT:
			n, err := strconv.ParseInt(x.Value, 0, 64)
			if err != nil {
				return nil, fmt.Errorf("parse int %q: %w", x.Value, err)
			}
			return n, nil
		case token.FLOAT:
			f, err := strconv.ParseFloat(x.Value, 64)
			if err != nil {
				return nil, fmt.Errorf("parse float %q: %w", x.Value, err)
			}
			return f, nil
		default:
			return nil, fmt.Errorf("unsupported literal kind %v", x.Kind)
		}

	case *ast.Ident:
		switch x.Name {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "nil":
			return nil, nil
		default:
			// Unresolved identifier — pass through as string for enum-like values.
			return x.Name, nil
		}

	case *ast.UnaryExpr:
		// Handle negative numbers: -1, -3.14
		if x.Op == token.SUB {
			inner, err := evalLiteral(x.X)
			if err != nil {
				return nil, err
			}
			switch v := inner.(type) {
			case int64:
				return -v, nil
			case float64:
				return -v, nil
			}
		}
		return nil, fmt.Errorf("unsupported unary operator %v", x.Op)

	case *ast.SelectorExpr:
		// pkg.Constant — return as "pkg.Name" string for caller to interpret.
		if pkg, ok := x.X.(*ast.Ident); ok {
			return pkg.Name + "." + x.Sel.Name, nil
		}
		return nil, fmt.Errorf("complex selector in arg")

	case *ast.CallExpr:
		// Nested call like pg.OnDelete(pg.FKActionRestrict) — record as a
		// special nested ChainResult so the consumer can inspect it.
		chain, err := UnwrapChain(e)
		if err != nil {
			return nil, fmt.Errorf("nested call in arg: %w", err)
		}
		return chain, nil

	default:
		return nil, fmt.Errorf("unsupported arg expression type %T", e)
	}
}

func reverseMethodCalls(s []MethodCall) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
