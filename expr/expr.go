package expr

import (
	"fmt"
	"strings"
)

// Expression is anything that can appear in a SQL WHERE, ON, or HAVING clause.
// All concrete expression types are in this package; external packages may also
// implement Expression for custom SQL fragments.
type Expression interface {
	ToSQL(ctx *BuildContext) string
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
	if expr == nil {
		return nil
	}
	return notExpr{expr: expr}
}

func filterNil(exprs []Expression) []Expression {
	out := exprs[:0:len(exprs)]
	for _, e := range exprs {
		if e != nil {
			out = append(out, e)
		}
	}
	return out
}

type andExpr struct{ exprs []Expression }
type orExpr struct{ exprs []Expression }
type notExpr struct{ expr Expression }

func (e andExpr) ToSQL(ctx *BuildContext) string {
	parts := make([]string, len(e.exprs))
	for i, ex := range e.exprs {
		parts[i] = ex.ToSQL(ctx)
	}
	return "(" + strings.Join(parts, " AND ") + ")"
}

func (e orExpr) ToSQL(ctx *BuildContext) string {
	parts := make([]string, len(e.exprs))
	for i, ex := range e.exprs {
		parts[i] = ex.ToSQL(ctx)
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func (e notExpr) ToSQL(ctx *BuildContext) string {
	return "NOT (" + e.expr.ToSQL(ctx) + ")"
}

// -------------------------------------------------------------------
// Raw SQL escape hatch
// -------------------------------------------------------------------

// Raw wraps a literal SQL string as an Expression. Use sparingly and
// never with user-controlled input — no escaping is applied.
func Raw(sql string) Expression { return rawExpr{sql: sql} }

type rawExpr struct{ sql string }

func (e rawExpr) ToSQL(_ *BuildContext) string { return e.sql }

// -------------------------------------------------------------------
// Internal expression types (produced by column operator methods)
// -------------------------------------------------------------------

// binaryExpr holds a single column op value comparison: "table"."col" OP $n
type binaryExpr struct {
	ref colRefer
	op  string
	val any
}

func (e binaryExpr) ToSQL(ctx *BuildContext) string {
	return e.ref.colRef(ctx) + " " + e.op + " " + ctx.Add(e.val)
}

// colColExpr holds a column op column comparison: "t1"."c1" OP "t2"."c2"
type colColExpr struct {
	left  colRefer
	op    string
	right colRefer
}

func (e colColExpr) ToSQL(ctx *BuildContext) string {
	return e.left.colRef(ctx) + " " + e.op + " " + e.right.colRef(ctx)
}

// nullExpr holds IS NULL / IS NOT NULL
type nullExpr struct {
	ref    colRefer
	isNull bool
}

func (e nullExpr) ToSQL(ctx *BuildContext) string {
	if e.isNull {
		return e.ref.colRef(ctx) + " IS NULL"
	}
	return e.ref.colRef(ctx) + " IS NOT NULL"
}

// inExpr holds col IN (v1, v2, ...)
type inExpr struct {
	ref  colRefer
	vals []any
	not  bool
}

func (e inExpr) ToSQL(ctx *BuildContext) string {
	placeholders := make([]string, len(e.vals))
	for i, v := range e.vals {
		placeholders[i] = ctx.Add(v)
	}
	op := "IN"
	if e.not {
		op = "NOT IN"
	}
	return e.ref.colRef(ctx) + " " + op + " (" + strings.Join(placeholders, ", ") + ")"
}

// betweenExpr holds col BETWEEN lo AND hi
type betweenExpr struct {
	ref colRefer
	lo  any
	hi  any
}

func (e betweenExpr) ToSQL(ctx *BuildContext) string {
	return fmt.Sprintf("%s BETWEEN %s AND %s",
		e.ref.colRef(ctx), ctx.Add(e.lo), ctx.Add(e.hi))
}

// likeExpr holds col LIKE/ILIKE pattern
type likeExpr struct {
	ref     colRefer
	op      string // "LIKE" or "ILIKE"
	pattern string
}

func (e likeExpr) ToSQL(ctx *BuildContext) string {
	return e.ref.colRef(ctx) + " " + e.op + " " + ctx.Add(e.pattern)
}

// colRefer is the internal interface that column types implement.
// It gives expression constructors access to the quoted column reference
// without exposing the BuildContext publicly on every column method.
type colRefer interface {
	colRef(ctx *BuildContext) string
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

func (e rawFlipExpr) ToSQL(ctx *BuildContext) string {
	return ctx.Add(e.left) + " " + e.op + " " + e.ref.colRef(ctx)
}

// jsonbNavExpr represents col -> key  or  col ->> key (text extraction).
// op is "->" or "->>"
type jsonbNavExpr struct {
	ref colRefer
	op  string // "->" or "->>"
	key string // text key (for ->) or integer index as string (for array access)
}

func (e jsonbNavExpr) ToSQL(ctx *BuildContext) string {
	return e.ref.colRef(ctx) + " " + e.op + " " + ctx.Add(e.key)
}

// jsonbPathExpr represents col #> path  or  col #>> path (path extraction).
type jsonbPathExpr struct {
	ref  colRefer
	op   string   // "#>" or "#>>"
	path []string // path segments e.g. {"a","b","c"}
}

func (e jsonbPathExpr) ToSQL(ctx *BuildContext) string {
	// PostgreSQL path syntax: ARRAY['a','b','c']::text[]
	quoted := make([]string, len(e.path))
	for i, seg := range e.path {
		quoted[i] = "'" + seg + "'"
	}
	return e.ref.colRef(ctx) + " " + e.op + " ARRAY[" + strings.Join(quoted, ", ") + "]"
}

// jsonbContainsExpr represents col @> val::jsonb  (containment check).
type jsonbContainsExpr struct {
	ref colRefer
	val any // will be JSON-encoded via the arg mechanism
	not bool
}

func (e jsonbContainsExpr) ToSQL(ctx *BuildContext) string {
	op := "@>"
	if e.not {
		return "NOT " + e.ref.colRef(ctx) + " @> " + ctx.Add(e.val)
	}
	return e.ref.colRef(ctx) + " " + op + " " + ctx.Add(e.val)
}

// jsonbKeyExistsExpr represents col ? key  (key existence check).
type jsonbKeyExistsExpr struct {
	ref colRefer
	key string
	not bool
}

func (e jsonbKeyExistsExpr) ToSQL(ctx *BuildContext) string {
	if e.not {
		return "NOT " + e.ref.colRef(ctx) + " ? " + ctx.Add(e.key)
	}
	return e.ref.colRef(ctx) + " ? " + ctx.Add(e.key)
}

// jsonbAnyKeyExistsExpr represents col ?| keys  (any key exists).
type jsonbAnyKeyExistsExpr struct {
	ref  colRefer
	keys []string
}

func (e jsonbAnyKeyExistsExpr) ToSQL(ctx *BuildContext) string {
	return e.ref.colRef(ctx) + " ?| " + ctx.Add(e.keys)
}

// jsonbAllKeysExistExpr represents col ?& keys  (all keys exist).
type jsonbAllKeysExistExpr struct {
	ref  colRefer
	keys []string
}

func (e jsonbAllKeysExistExpr) ToSQL(ctx *BuildContext) string {
	return e.ref.colRef(ctx) + " ?& " + ctx.Add(e.keys)
}
