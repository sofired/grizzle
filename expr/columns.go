package expr

import (
	"time"

	"github.com/google/uuid"
)

// -------------------------------------------------------------------
// colBase — shared infrastructure embedded by all typed column types
// -------------------------------------------------------------------

// ColBase holds the table and column name. It is embedded by every typed
// column type and provides the common IsNull/IsNotNull/Asc/Desc methods.
// Exported so generated code can initialise column structs with field literals.
type ColBase struct {
	TableAlias string // the alias used in the query (usually == table name)
	ColName    string // the SQL column name
}

func (c ColBase) colRef(ctx *BuildContext) string {
	return ctx.ColRef(c.TableAlias, c.ColName)
}

// ColumnName returns the raw SQL column name (without table qualification).
func (c ColBase) ColumnName() string { return c.ColName }

// TableName returns the table alias used in the query.
func (c ColBase) TableName() string { return c.TableAlias }

// IsNull returns a "col IS NULL" expression.
func (c ColBase) IsNull() Expression { return nullExpr{ref: c, isNull: true} }

// IsNotNull returns a "col IS NOT NULL" expression.
func (c ColBase) IsNotNull() Expression { return nullExpr{ref: c, isNull: false} }

// Asc returns an ascending ORDER BY expression for this column.
func (c ColBase) Asc() OrderExpr { return OrderExpr{ref: c, dir: "ASC"} }

// Desc returns a descending ORDER BY expression for this column.
func (c ColBase) Desc() OrderExpr { return OrderExpr{ref: c, dir: "DESC"} }

// -------------------------------------------------------------------
// OrderExpr
// -------------------------------------------------------------------

// OrderExpr represents a single ORDER BY clause entry.
type OrderExpr struct {
	ref colRefer
	dir string
}

func (o OrderExpr) ToSQL(ctx *BuildContext) string {
	return o.ref.colRef(ctx) + " " + o.dir
}

// -------------------------------------------------------------------
// SelectableColumn — implemented by all column types
// -------------------------------------------------------------------

// SelectableColumn can appear in a SELECT clause. Generated table types
// expose their columns as SelectableColumn values.
type SelectableColumn interface {
	colRef(ctx *BuildContext) string
	ColumnName() string
	TableName() string
}

// -------------------------------------------------------------------
// UUIDColumn
// -------------------------------------------------------------------

// UUIDColumn is a typed column handle for UUID values.
// Only UUID-compatible operators are exposed, preventing type mismatches
// at compile time.
type UUIDColumn struct{ ColBase }

func (c UUIDColumn) EQ(val uuid.UUID) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c UUIDColumn) NEQ(val uuid.UUID) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}
func (c UUIDColumn) In(vals ...uuid.UUID) Expression {
	if len(vals) == 0 {
		return Raw("FALSE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys}
}
func (c UUIDColumn) NotIn(vals ...uuid.UUID) Expression {
	if len(vals) == 0 {
		return Raw("TRUE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys, not: true}
}

// EQCol compares this column to another UUID column: useful for JOIN conditions.
func (c UUIDColumn) EQCol(other UUIDColumn) Expression {
	return colColExpr{left: c.ColBase, op: "=", right: other.ColBase}
}

// -------------------------------------------------------------------
// StringColumn
// -------------------------------------------------------------------

// StringColumn is a typed column handle for TEXT / VARCHAR values.
type StringColumn struct{ ColBase }

func (c StringColumn) EQ(val string) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c StringColumn) NEQ(val string) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}
func (c StringColumn) Like(pattern string) Expression {
	return likeExpr{ref: c.ColBase, op: "LIKE", pattern: pattern}
}

// ILike produces a case-insensitive LIKE (PostgreSQL-specific).
func (c StringColumn) ILike(pattern string) Expression {
	return likeExpr{ref: c.ColBase, op: "ILIKE", pattern: pattern}
}
func (c StringColumn) In(vals ...string) Expression {
	if len(vals) == 0 {
		return Raw("FALSE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys}
}
func (c StringColumn) NotIn(vals ...string) Expression {
	if len(vals) == 0 {
		return Raw("TRUE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys, not: true}
}
func (c StringColumn) EQCol(other StringColumn) Expression {
	return colColExpr{left: c.ColBase, op: "=", right: other.ColBase}
}

// -------------------------------------------------------------------
// IntColumn
// -------------------------------------------------------------------

// IntColumn is a typed column handle for INTEGER / BIGINT values.
type IntColumn struct{ ColBase }

func (c IntColumn) EQ(val int) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c IntColumn) NEQ(val int) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}
func (c IntColumn) GT(val int) Expression {
	return binaryExpr{ref: c.ColBase, op: ">", val: val}
}
func (c IntColumn) GTE(val int) Expression {
	return binaryExpr{ref: c.ColBase, op: ">=", val: val}
}
func (c IntColumn) LT(val int) Expression {
	return binaryExpr{ref: c.ColBase, op: "<", val: val}
}
func (c IntColumn) LTE(val int) Expression {
	return binaryExpr{ref: c.ColBase, op: "<=", val: val}
}
func (c IntColumn) Between(lo, hi int) Expression {
	return betweenExpr{ref: c.ColBase, lo: lo, hi: hi}
}
func (c IntColumn) In(vals ...int) Expression {
	if len(vals) == 0 {
		return Raw("FALSE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys}
}
func (c IntColumn) NotIn(vals ...int) Expression {
	if len(vals) == 0 {
		return Raw("TRUE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys, not: true}
}

// -------------------------------------------------------------------
// BoolColumn
// -------------------------------------------------------------------

// BoolColumn is a typed column handle for BOOLEAN values.
type BoolColumn struct{ ColBase }

func (c BoolColumn) EQ(val bool) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c BoolColumn) IsTrue() Expression  { return binaryExpr{ref: c.ColBase, op: "=", val: true} }
func (c BoolColumn) IsFalse() Expression { return binaryExpr{ref: c.ColBase, op: "=", val: false} }

// -------------------------------------------------------------------
// TimestampColumn
// -------------------------------------------------------------------

// TimestampColumn is a typed column handle for TIMESTAMP / TIMESTAMPTZ values.
type TimestampColumn struct{ ColBase }

func (c TimestampColumn) EQ(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c TimestampColumn) GT(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: ">", val: val}
}
func (c TimestampColumn) GTE(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: ">=", val: val}
}
func (c TimestampColumn) LT(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: "<", val: val}
}
func (c TimestampColumn) LTE(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: "<=", val: val}
}
func (c TimestampColumn) Between(lo, hi time.Time) Expression {
	return betweenExpr{ref: c.ColBase, lo: lo, hi: hi}
}

// GTCol compares two timestamp columns: useful for check-style expressions.
func (c TimestampColumn) GTCol(other TimestampColumn) Expression {
	return colColExpr{left: c.ColBase, op: ">", right: other.ColBase}
}
func (c TimestampColumn) GTECol(other TimestampColumn) Expression {
	return colColExpr{left: c.ColBase, op: ">=", right: other.ColBase}
}

// -------------------------------------------------------------------
// JSONBColumn
// -------------------------------------------------------------------

// JSONBColumn is a typed column handle for JSONB / JSON values.
// T is the Go type the JSON will be scanned into (e.g. map[string]any,
// []string, or a custom struct). The type parameter makes it easy for
// generated code to carry the correct scan type without runtime casts.
type JSONBColumn[T any] struct{ ColBase }

// Arrow returns col -> key — navigate to the JSONB object field `key`,
// returning a JSONB value. Useful in SELECT lists; produces a new
// expression (not chainable for further operators).
//
//	UsersT.Attributes.Arrow("role")  →  "users"."attributes" -> $1
func (c JSONBColumn[T]) Arrow(key string) Expression {
	return jsonbNavExpr{ref: c.ColBase, op: "->", key: key}
}

// ArrowText returns col ->> key — extract the JSONB field `key` as text.
//
//	UsersT.Attributes.ArrowText("role")  →  "users"."attributes" ->> $1
func (c JSONBColumn[T]) ArrowText(key string) Expression {
	return jsonbNavExpr{ref: c.ColBase, op: "->>", key: key}
}

// Path returns col #> path — navigate to a nested JSONB value via a path.
//
//	UsersT.Attributes.Path("address", "city")
//	  →  "users"."attributes" #> ARRAY['address', 'city']
func (c JSONBColumn[T]) Path(segments ...string) Expression {
	return jsonbPathExpr{ref: c.ColBase, op: "#>", path: segments}
}

// PathText returns col #>> path — navigate to a nested JSONB value and return as text.
func (c JSONBColumn[T]) PathText(segments ...string) Expression {
	return jsonbPathExpr{ref: c.ColBase, op: "#>>", path: segments}
}

// Contains returns col @> val::jsonb — true when this column contains val.
// val should be a JSON-serialisable Go value (map, struct, slice, scalar).
//
//	UsersT.Attributes.Contains(map[string]any{"role": "admin"})
//	  →  "users"."attributes" @> $1
func (c JSONBColumn[T]) Contains(val any) Expression {
	return jsonbContainsExpr{ref: c.ColBase, val: val}
}

// ContainedBy returns val @> col — true when val contains this column.
// (The operands are flipped relative to Contains.)
func (c JSONBColumn[T]) ContainedBy(val any) Expression {
	// val @> col  is  NOT (col @> val) is wrong — we need a raw flip.
	// Use a raw expr because the standard binaryExpr puts the column on the left.
	return rawFlipExpr{left: val, op: "@>", ref: c.ColBase}
}

// HasKey returns col ? key — true when the top-level JSONB object has key.
//
//	UsersT.Attributes.HasKey("role")  →  "users"."attributes" ? $1
func (c JSONBColumn[T]) HasKey(key string) Expression {
	return jsonbKeyExistsExpr{ref: c.ColBase, key: key}
}

// HasKeyNot returns NOT col ? key.
func (c JSONBColumn[T]) HasKeyNot(key string) Expression {
	return jsonbKeyExistsExpr{ref: c.ColBase, key: key, not: true}
}

// HasAnyKey returns col ?| keys — true when the object has any of the given keys.
//
//	UsersT.Attributes.HasAnyKey("role", "admin")
//	  →  "users"."attributes" ?| $1
func (c JSONBColumn[T]) HasAnyKey(keys ...string) Expression {
	return jsonbAnyKeyExistsExpr{ref: c.ColBase, keys: keys}
}

// HasAllKeys returns col ?& keys — true when the object has all of the given keys.
func (c JSONBColumn[T]) HasAllKeys(keys ...string) Expression {
	return jsonbAllKeysExistExpr{ref: c.ColBase, keys: keys}
}

// -------------------------------------------------------------------
// FloatColumn
// -------------------------------------------------------------------

// FloatColumn is a typed column handle for NUMERIC / REAL / DOUBLE PRECISION values.
type FloatColumn struct{ ColBase }

func (c FloatColumn) EQ(val float64) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c FloatColumn) NEQ(val float64) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}
func (c FloatColumn) GT(val float64) Expression {
	return binaryExpr{ref: c.ColBase, op: ">", val: val}
}
func (c FloatColumn) GTE(val float64) Expression {
	return binaryExpr{ref: c.ColBase, op: ">=", val: val}
}
func (c FloatColumn) LT(val float64) Expression {
	return binaryExpr{ref: c.ColBase, op: "<", val: val}
}
func (c FloatColumn) LTE(val float64) Expression {
	return binaryExpr{ref: c.ColBase, op: "<=", val: val}
}
func (c FloatColumn) Between(lo, hi float64) Expression {
	return betweenExpr{ref: c.ColBase, lo: lo, hi: hi}
}
func (c FloatColumn) In(vals ...float64) Expression {
	if len(vals) == 0 {
		return Raw("FALSE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys}
}
func (c FloatColumn) NotIn(vals ...float64) Expression {
	if len(vals) == 0 {
		return Raw("TRUE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys, not: true}
}
