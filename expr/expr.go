package expr

import (
	"reflect"
	"strings"
)

// Expression is anything that can appear in a SQL WHERE, ON, or HAVING clause.
// All concrete expression types are in this package; external packages may also
// implement Expression for custom SQL fragments.
type Expression interface {
	RenderSQL(ctx *BuildContext) (string, error)
}

// -------------------------------------------------------------------
// Logical combinators
// -------------------------------------------------------------------

// And combines expressions with AND. Nil expressions are silently dropped,
// so callers can write:
//
//	And(
//	    whenPtr(p.MinAge, func(v int) Expression { return UsersT.Age.GTE(v) }),
//	    whenPtr(p.Name,   func(v string) Expression { return UsersT.Name.ILike("%"+v+"%") }),
//	)
//
// without needing explicit nil checks around each optional condition.
func And(exprs ...Expression) Expression {
	active := filterNil(exprs)
	switch len(active) {
	case 0:
		return nil
	case 1:
		return active[0]
	default:
		return andExpr{exprs: active}
	}
}

// Or combines expressions with OR. Nil expressions are silently dropped.
func Or(exprs ...Expression) Expression {
	active := filterNil(exprs)
	switch len(active) {
	case 0:
		return nil
	case 1:
		return active[0]
	default:
		return orExpr{exprs: active}
	}
}

// Not negates an expression. Returns nil if expr is nil.
func Not(expr Expression) Expression {
	if isNilExpression(expr) {
		return nil
	}
	return notExpr{expr: expr}
}

func filterNil(exprs []Expression) []Expression {
	out := exprs[:0:len(exprs)]
	for _, e := range exprs {
		if !isNilExpression(e) {
			out = append(out, e)
		}
	}
	return out
}

func isNilExpression(e Expression) bool {
	return isNilInterface(e)
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

type andExpr struct{ exprs []Expression }
type orExpr struct{ exprs []Expression }
type notExpr struct{ expr Expression }

func (e andExpr) RenderSQL(ctx *BuildContext) (string, error) {
	return renderAtomically(ctx, func() (string, error) {
		parts := make([]string, len(e.exprs))
		for i, ex := range e.exprs {
			part, err := ex.RenderSQL(ctx)
			if err != nil {
				return "", err
			}
			parts[i] = part
		}
		return "(" + strings.Join(parts, " AND ") + ")", nil
	})
}

func (e orExpr) RenderSQL(ctx *BuildContext) (string, error) {
	return renderAtomically(ctx, func() (string, error) {
		parts := make([]string, len(e.exprs))
		for i, ex := range e.exprs {
			part, err := ex.RenderSQL(ctx)
			if err != nil {
				return "", err
			}
			parts[i] = part
		}
		return "(" + strings.Join(parts, " OR ") + ")", nil
	})
}

func (e notExpr) RenderSQL(ctx *BuildContext) (string, error) {
	return renderAtomically(ctx, func() (string, error) {
		inner, err := e.expr.RenderSQL(ctx)
		if err != nil {
			return "", err
		}
		return "NOT (" + inner + ")", nil
	})
}

// -------------------------------------------------------------------
// Raw SQL escape hatch
// -------------------------------------------------------------------

// Raw wraps a literal SQL string as an Expression. Use sparingly and
// never with user-controlled input — no escaping is applied.
func Raw(sql string) Expression { return rawExpr{sql: sql} }

type rawExpr struct{ sql string }

func (e rawExpr) RenderSQL(_ *BuildContext) (string, error) { return e.sql, nil }

type invalidExpr struct{ message string }

func (e invalidExpr) RenderSQL(_ *BuildContext) (string, error) {
	return "", NewError(CodeBuildValidation, "render_expression", e.message)
}

func invalidListExpression() Expression {
	return invalidExpr{message: "list expression requires at least one value"}
}

// RawArgs wraps a SQL fragment containing $? placeholders together with the
// argument values that fill them in order. Each $? placeholder is replaced with
// the next bound-parameter placeholder ($1, ?, etc.) from the active dialect.
//
// The number of $? tokens must exactly match the number of args. Any mismatch
// returns a redacted build-validation error before binding arguments.
//
// Example:
//
//	expr.RawArgs("ST_DWithin(location, ST_MakePoint($?, $?), $?)", lon, lat, radius)
func RawArgs(sql string, args ...any) Expression {
	return rawArgsExpr{sql: sql, args: args}
}

type rawArgsExpr struct {
	sql  string
	args []any
}

func (e rawArgsExpr) RenderSQL(ctx *BuildContext) (string, error) {
	// Count $? placeholders.
	count := strings.Count(e.sql, "$?")
	if count != len(e.args) {
		return "", NewError(CodeBuildValidation, "render_raw_args", "raw expression placeholder count does not match argument count")
	}
	// Replace each $? with the next dialect placeholder, binding each arg.
	result := e.sql
	for i := 0; i < count; i++ {
		placeholder := ctx.Add(e.args[i])
		// Replace only the first occurrence of $? in each iteration.
		idx := strings.Index(result, "$?")
		result = result[:idx] + placeholder + result[idx+2:]
	}
	return result, nil
}

// -------------------------------------------------------------------
// Internal expression types (produced by column operator methods)
// -------------------------------------------------------------------

// binaryExpr holds a single column op value comparison: "table"."col" OP $n
type binaryExpr struct {
	ref colRefer
	op  string
	val any
}

func (e binaryExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	return ref + " " + e.op + " " + ctx.Add(e.val), nil
}

// colColExpr holds a column op column comparison: "t1"."c1" OP "t2"."c2"
type colColExpr struct {
	left  colRefer
	op    string
	right colRefer
}

func (e colColExpr) RenderSQL(ctx *BuildContext) (string, error) {
	left, err := e.left.colRef(ctx)
	if err != nil {
		return "", err
	}
	right, err := e.right.colRef(ctx)
	if err != nil {
		return "", err
	}
	return left + " " + e.op + " " + right, nil
}

// nullExpr holds IS NULL / IS NOT NULL
type nullExpr struct {
	ref    colRefer
	isNull bool
}

func (e nullExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	if e.isNull {
		return ref + " IS NULL", nil
	}
	return ref + " IS NOT NULL", nil
}

// inExpr holds col IN (v1, v2, ...)
type inExpr struct {
	ref  colRefer
	vals []any
	not  bool
}

func (e inExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	placeholders := make([]string, len(e.vals))
	for i, v := range e.vals {
		placeholders[i] = ctx.Add(v)
	}
	op := "IN"
	if e.not {
		op = "NOT IN"
	}
	return ref + " " + op + " (" + strings.Join(placeholders, ", ") + ")", nil
}

// betweenExpr holds col BETWEEN lo AND hi
type betweenExpr struct {
	ref colRefer
	lo  any
	hi  any
}

func (e betweenExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	return ref + " BETWEEN " + ctx.Add(e.lo) + " AND " + ctx.Add(e.hi), nil
}

// likeExpr holds col LIKE/ILIKE pattern
type likeExpr struct {
	ref     colRefer
	op      string // "LIKE" or "ILIKE"
	pattern string
}

func (e likeExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	return ref + " " + e.op + " " + ctx.Add(e.pattern), nil
}

// colRefer is the internal interface that column types implement.
// It gives expression constructors access to the quoted column reference
// without exposing the BuildContext publicly on every column method.
type colRefer interface {
	colRef(ctx *BuildContext) (string, error)
}

// -------------------------------------------------------------------
// JSONB expression types (PostgreSQL-specific operators)
// -------------------------------------------------------------------

// rawFlipExpr handles cases where the column is on the RIGHT side of an operator:
// val OP col  — used by ContainedBy (@>).
type rawFlipExpr struct {
	left any
	op   string
	ref  colRefer
}

func (e rawFlipExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	return ctx.Add(e.left) + " " + e.op + " " + ref, nil
}

// jsonbNavExpr represents col -> key  or  col ->> key (text extraction).
// op is "->" or "->>"
type jsonbNavExpr struct {
	ref colRefer
	op  string // "->" or "->>"
	key string // text key (for ->) or integer index as string (for array access)
}

func (e jsonbNavExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	return ref + " " + e.op + " " + ctx.Add(e.key), nil
}

// jsonbPathExpr represents col #> path  or  col #>> path (path extraction).
type jsonbPathExpr struct {
	ref  colRefer
	op   string   // "#>" or "#>>"
	path []string // path segments e.g. {"a","b","c"}
}

func (e jsonbPathExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	// PostgreSQL path syntax: ARRAY['a','b','c']::text[]
	quoted := make([]string, len(e.path))
	for i, seg := range e.path {
		quoted[i] = "'" + seg + "'"
	}
	return ref + " " + e.op + " ARRAY[" + strings.Join(quoted, ", ") + "]", nil
}

// jsonbContainsExpr represents col @> val::jsonb  (containment check).
type jsonbContainsExpr struct {
	ref colRefer
	val any // will be JSON-encoded via the arg mechanism
	not bool
}

func (e jsonbContainsExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	op := "@>"
	if e.not {
		return "NOT " + ref + " @> " + ctx.Add(e.val), nil
	}
	return ref + " " + op + " " + ctx.Add(e.val), nil
}

// jsonbKeyExistsExpr represents col ? key  (key existence check).
type jsonbKeyExistsExpr struct {
	ref colRefer
	key string
	not bool
}

func (e jsonbKeyExistsExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	if e.not {
		return "NOT " + ref + " ? " + ctx.Add(e.key), nil
	}
	return ref + " ? " + ctx.Add(e.key), nil
}

// jsonbAnyKeyExistsExpr represents col ?| keys  (any key exists).
type jsonbAnyKeyExistsExpr struct {
	ref  colRefer
	keys []string
}

func (e jsonbAnyKeyExistsExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	return ref + " ?| " + ctx.Add(e.keys), nil
}

// jsonbAllKeysExistExpr represents col ?& keys  (all keys exist).
type jsonbAllKeysExistExpr struct {
	ref  colRefer
	keys []string
}

func (e jsonbAllKeysExistExpr) RenderSQL(ctx *BuildContext) (string, error) {
	ref, err := e.ref.colRef(ctx)
	if err != nil {
		return "", err
	}
	return ref + " ?& " + ctx.Add(e.keys), nil
}
