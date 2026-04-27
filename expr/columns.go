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
	ref   colRefer
	dir   string
	nulls string // "NULLS FIRST", "NULLS LAST", or ""
}

func (o OrderExpr) ToSQL(ctx *BuildContext) string {
	s := o.ref.colRef(ctx) + " " + o.dir
	if o.nulls != "" {
		s += " " + o.nulls
	}
	return s
}

// ToSQLUnqualified renders the ORDER BY expression using only the column name,
// without a table qualifier. Required for set operation (UNION/INTERSECT/EXCEPT)
// ORDER BY clauses, where table qualifiers are not valid SQL.
func (o OrderExpr) ToSQLUnqualified(ctx *BuildContext) string {
	name := unqualifiedColRef(o.ref, ctx)
	s := name + " " + o.dir
	if o.nulls != "" {
		s += " " + o.nulls
	}
	return s
}

// unqualifiedColRef returns only the column name portion of a colRef,
// stripping any table qualifier. Falls back to the full colRef for complex
// expressions (window functions, arithmetic, etc.) that have no table prefix.
func unqualifiedColRef(ref colRefer, ctx *BuildContext) string {
	// For ColBase (the common case), we can access the column name directly.
	type namedCol interface {
		ColumnName() string
		TableName() string
	}
	if nc, ok := ref.(namedCol); ok && nc.TableName() != "" {
		// Has a table qualifier — emit only the quoted column name.
		return ctx.Quote(nc.ColumnName())
	}
	// No table qualifier or complex expression — use full colRef.
	return ref.colRef(ctx)
}

// NullsFirst returns a copy of the ORDER BY expression with NULLS FIRST appended.
func (o OrderExpr) NullsFirst() OrderExpr {
	o.nulls = "NULLS FIRST"
	return o
}

// NullsLast returns a copy of the ORDER BY expression with NULLS LAST appended.
func (o OrderExpr) NullsLast() OrderExpr {
	o.nulls = "NULLS LAST"
	return o
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

// NotLike produces a NOT LIKE predicate.
//
//	UsersT.Username.NotLike("admin%")
//	// → "users"."username" NOT LIKE $1
func (c StringColumn) NotLike(pattern string) Expression {
	return likeExpr{ref: c.ColBase, op: "NOT LIKE", pattern: pattern}
}

// NotILike produces a case-insensitive NOT LIKE predicate (PostgreSQL-specific).
//
//	UsersT.Username.NotILike("admin%")
//	// → "users"."username" NOT ILIKE $1
func (c StringColumn) NotILike(pattern string) Expression {
	return likeExpr{ref: c.ColBase, op: "NOT ILIKE", pattern: pattern}
}

// RegexpMatch produces a case-sensitive regex match: col ~ $1 (PostgreSQL-specific).
func (c StringColumn) RegexpMatch(pattern string) Expression {
	return regexpExpr{ref: c.ColBase, op: "~", pattern: pattern}
}

// RegexpMatchI produces a case-insensitive regex match: col ~* $1 (PostgreSQL-specific).
func (c StringColumn) RegexpMatchI(pattern string) Expression {
	return regexpExpr{ref: c.ColBase, op: "~*", pattern: pattern}
}

// NotRegexpMatch produces a case-sensitive regex non-match: col !~ $1 (PostgreSQL-specific).
func (c StringColumn) NotRegexpMatch(pattern string) Expression {
	return regexpExpr{ref: c.ColBase, op: "!~", pattern: pattern}
}

// NotRegexpMatchI produces a case-insensitive regex non-match: col !~* $1 (PostgreSQL-specific).
func (c StringColumn) NotRegexpMatchI(pattern string) Expression {
	return regexpExpr{ref: c.ColBase, op: "!~*", pattern: pattern}
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

// IntColumn is a typed column handle for INTEGER / SERIAL / SMALLINT values
// (4-byte or 2-byte signed integers). Go value type: int.
// For BIGINT / BIGSERIAL columns use BigIntColumn.
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
func (c IntColumn) EQCol(other IntColumn) Expression {
	return colColExpr{left: c.ColBase, op: "=", right: other.ColBase}
}

// Arithmetic operators — return an ArithExpr usable in SELECT or WHERE.
func (c IntColumn) Add(val int) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "+", right: litRefer{val}}
}
func (c IntColumn) Sub(val int) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "-", right: litRefer{val}}
}
func (c IntColumn) Mul(val int) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "*", right: litRefer{val}}
}
func (c IntColumn) Div(val int) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "/", right: litRefer{val}}
}
func (c IntColumn) AddCol(other IntColumn) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "+", right: other.ColBase}
}
func (c IntColumn) SubCol(other IntColumn) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "-", right: other.ColBase}
}
func (c IntColumn) MulCol(other IntColumn) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "*", right: other.ColBase}
}

// -------------------------------------------------------------------
// BigIntColumn
// -------------------------------------------------------------------

// BigIntColumn is a typed column handle for BIGINT / BIGSERIAL values
// (8-byte signed integers). Go value type: int64.
// For INTEGER / SERIAL / SMALLINT columns use IntColumn.
type BigIntColumn struct{ ColBase }

func (c BigIntColumn) EQ(val int64) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c BigIntColumn) NEQ(val int64) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}
func (c BigIntColumn) GT(val int64) Expression {
	return binaryExpr{ref: c.ColBase, op: ">", val: val}
}
func (c BigIntColumn) GTE(val int64) Expression {
	return binaryExpr{ref: c.ColBase, op: ">=", val: val}
}
func (c BigIntColumn) LT(val int64) Expression {
	return binaryExpr{ref: c.ColBase, op: "<", val: val}
}
func (c BigIntColumn) LTE(val int64) Expression {
	return binaryExpr{ref: c.ColBase, op: "<=", val: val}
}
func (c BigIntColumn) Between(lo, hi int64) Expression {
	return betweenExpr{ref: c.ColBase, lo: lo, hi: hi}
}
func (c BigIntColumn) In(vals ...int64) Expression {
	if len(vals) == 0 {
		return Raw("FALSE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys}
}
func (c BigIntColumn) NotIn(vals ...int64) Expression {
	if len(vals) == 0 {
		return Raw("TRUE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys, not: true}
}
func (c BigIntColumn) EQCol(other BigIntColumn) Expression {
	return colColExpr{left: c.ColBase, op: "=", right: other.ColBase}
}

// Arithmetic operators — return an ArithExpr usable in SELECT or WHERE.
func (c BigIntColumn) Add(val int64) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "+", right: litRefer{val}}
}
func (c BigIntColumn) Sub(val int64) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "-", right: litRefer{val}}
}
func (c BigIntColumn) Mul(val int64) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "*", right: litRefer{val}}
}
func (c BigIntColumn) Div(val int64) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "/", right: litRefer{val}}
}
func (c BigIntColumn) AddCol(other BigIntColumn) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "+", right: other.ColBase}
}
func (c BigIntColumn) SubCol(other BigIntColumn) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "-", right: other.ColBase}
}
func (c BigIntColumn) MulCol(other BigIntColumn) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "*", right: other.ColBase}
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
func (c BoolColumn) EQCol(other BoolColumn) Expression {
	return colColExpr{left: c.ColBase, op: "=", right: other.ColBase}
}

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

func (c TimestampColumn) EQCol(other TimestampColumn) Expression {
	return colColExpr{left: c.ColBase, op: "=", right: other.ColBase}
}
func (c TimestampColumn) NEQCol(other TimestampColumn) Expression {
	return colColExpr{left: c.ColBase, op: "<>", right: other.ColBase}
}
func (c TimestampColumn) GTCol(other TimestampColumn) Expression {
	return colColExpr{left: c.ColBase, op: ">", right: other.ColBase}
}
func (c TimestampColumn) GTECol(other TimestampColumn) Expression {
	return colColExpr{left: c.ColBase, op: ">=", right: other.ColBase}
}
func (c TimestampColumn) LTCol(other TimestampColumn) Expression {
	return colColExpr{left: c.ColBase, op: "<", right: other.ColBase}
}
func (c TimestampColumn) LTECol(other TimestampColumn) Expression {
	return colColExpr{left: c.ColBase, op: "<=", right: other.ColBase}
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
func (c FloatColumn) EQCol(other FloatColumn) Expression {
	return colColExpr{left: c.ColBase, op: "=", right: other.ColBase}
}
func (c FloatColumn) NEQCol(other FloatColumn) Expression {
	return colColExpr{left: c.ColBase, op: "<>", right: other.ColBase}
}
func (c FloatColumn) GTCol(other FloatColumn) Expression {
	return colColExpr{left: c.ColBase, op: ">", right: other.ColBase}
}
func (c FloatColumn) GTECol(other FloatColumn) Expression {
	return colColExpr{left: c.ColBase, op: ">=", right: other.ColBase}
}
func (c FloatColumn) LTCol(other FloatColumn) Expression {
	return colColExpr{left: c.ColBase, op: "<", right: other.ColBase}
}
func (c FloatColumn) LTECol(other FloatColumn) Expression {
	return colColExpr{left: c.ColBase, op: "<=", right: other.ColBase}
}

// Arithmetic operators — return an ArithExpr usable in SELECT or WHERE.
func (c FloatColumn) Add(val float64) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "+", right: litRefer{val}}
}
func (c FloatColumn) Sub(val float64) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "-", right: litRefer{val}}
}
func (c FloatColumn) Mul(val float64) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "*", right: litRefer{val}}
}
func (c FloatColumn) Div(val float64) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "/", right: litRefer{val}}
}
func (c FloatColumn) AddCol(other FloatColumn) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "+", right: other.ColBase}
}
func (c FloatColumn) SubCol(other FloatColumn) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "-", right: other.ColBase}
}
func (c FloatColumn) MulCol(other FloatColumn) ArithExpr {
	return ArithExpr{left: c.ColBase, op: "*", right: other.ColBase}
}

// -------------------------------------------------------------------
// BytesColumn
// -------------------------------------------------------------------

// BytesColumn is a typed column handle for BLOB / binary values.
// BLOB columns are used in SQLite (and other databases) to store raw byte data.
// The corresponding Go type is []byte.
type BytesColumn struct{ ColBase }

func (c BytesColumn) EQ(val []byte) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c BytesColumn) NEQ(val []byte) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}

// -------------------------------------------------------------------
// DateColumn
// -------------------------------------------------------------------

// DateColumn is a typed column handle for SQL date (date-only) columns.
// The corresponding Go type is time.Time; only the date portion is meaningful.
type DateColumn struct{ ColBase }

func (c DateColumn) EQ(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c DateColumn) NEQ(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}
func (c DateColumn) GT(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: ">", val: val}
}
func (c DateColumn) GTE(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: ">=", val: val}
}
func (c DateColumn) LT(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: "<", val: val}
}
func (c DateColumn) LTE(val time.Time) Expression {
	return binaryExpr{ref: c.ColBase, op: "<=", val: val}
}
func (c DateColumn) EQCol(other DateColumn) Expression {
	return colColExpr{left: c.ColBase, op: "=", right: other.ColBase}
}

// -------------------------------------------------------------------
// IntervalColumn
// -------------------------------------------------------------------

// IntervalColumn is a typed column handle for PostgreSQL interval (duration) columns.
// Intervals are scanned as strings; use pgtype.Interval for richer handling.
type IntervalColumn struct{ ColBase }

func (c IntervalColumn) EQ(val string) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c IntervalColumn) NEQ(val string) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}

// -------------------------------------------------------------------
// EnumColumn
// -------------------------------------------------------------------

// EnumColumn is a typed column handle for PostgreSQL custom enum columns.
// Enum values are scanned as strings.
type EnumColumn struct{ ColBase }

func (c EnumColumn) EQ(val string) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c EnumColumn) NEQ(val string) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}
func (c EnumColumn) In(vals ...string) Expression {
	if len(vals) == 0 {
		return Raw("FALSE")
	}
	anys := make([]any, len(vals))
	for i, v := range vals {
		anys[i] = v
	}
	return inExpr{ref: c.ColBase, vals: anys}
}
func (c EnumColumn) NotIn(vals ...string) Expression {
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
// InetColumn
// -------------------------------------------------------------------

// InetColumn is a typed column handle for PostgreSQL inet, cidr, and macaddr columns.
// Network addresses are scanned as strings.
type InetColumn struct{ ColBase }

func (c InetColumn) EQ(val string) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c InetColumn) NEQ(val string) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}

// -------------------------------------------------------------------
// TsvectorColumn (PostgreSQL-specific)
// -------------------------------------------------------------------

// TsvectorColumn is a typed column handle for PostgreSQL TSVECTOR values.
// It exposes PostgreSQL full-text search operators (@@ with various tsquery constructors).
// These operators are PostgreSQL-specific and must only be used with a PostgreSQL dialect.
type TsvectorColumn struct{ ColBase }

// Matches returns col @@ to_tsquery($1) — matches a tsquery string.
//
//	ArticlesT.SearchVector.Matches("grizzle & orm")
//	// → "articles"."search_vector" @@ to_tsquery($1)
func (c TsvectorColumn) Matches(query string) Expression {
	return ftsMatchExpr{ref: c.ColBase, tsFn: "to_tsquery", query: query}
}

// MatchesWithConfig returns col @@ to_tsquery($1, $2) — uses an explicit text search
// configuration such as "english" or "simple".
// config is bound as $1 and query as $2, matching the PostgreSQL call signature.
//
//	ArticlesT.SearchVector.MatchesWithConfig("english", "grizzle & orm")
//	// → "articles"."search_vector" @@ to_tsquery($1, $2)
func (c TsvectorColumn) MatchesWithConfig(config, query string) Expression {
	return ftsMatchExpr{ref: c.ColBase, tsFn: "to_tsquery", config: config, query: query, hasConfig: true}
}

// MatchesPlain returns col @@ plainto_tsquery($1) — converts plain text to a tsquery
// by treating each word as a term connected with AND.
//
//	ArticlesT.SearchVector.MatchesPlain("grizzle orm")
//	// → "articles"."search_vector" @@ plainto_tsquery($1)
func (c TsvectorColumn) MatchesPlain(query string) Expression {
	return ftsMatchExpr{ref: c.ColBase, tsFn: "plainto_tsquery", query: query}
}

// MatchesPlainWithConfig returns col @@ plainto_tsquery($1, $2).
// config is bound as $1 and query as $2, matching the PostgreSQL call signature.
func (c TsvectorColumn) MatchesPlainWithConfig(config, query string) Expression {
	return ftsMatchExpr{ref: c.ColBase, tsFn: "plainto_tsquery", config: config, query: query, hasConfig: true}
}

// MatchesPhrase returns col @@ phraseto_tsquery($1) — matches an exact phrase.
//
//	ArticlesT.SearchVector.MatchesPhrase("fast full text")
//	// → "articles"."search_vector" @@ phraseto_tsquery($1)
func (c TsvectorColumn) MatchesPhrase(query string) Expression {
	return ftsMatchExpr{ref: c.ColBase, tsFn: "phraseto_tsquery", query: query}
}

// MatchesPhraseWithConfig returns col @@ phraseto_tsquery($1, $2).
// config is bound as $1 and query as $2, matching the PostgreSQL call signature.
func (c TsvectorColumn) MatchesPhraseWithConfig(config, query string) Expression {
	return ftsMatchExpr{ref: c.ColBase, tsFn: "phraseto_tsquery", config: config, query: query, hasConfig: true}
}

// MatchesWebSearch returns col @@ websearch_to_tsquery($1) — converts a web-search-style
// query string (quoting, minus, OR) to a tsquery.
//
//	ArticlesT.SearchVector.MatchesWebSearch("grizzle -orm")
//	// → "articles"."search_vector" @@ websearch_to_tsquery($1)
func (c TsvectorColumn) MatchesWebSearch(query string) Expression {
	return ftsMatchExpr{ref: c.ColBase, tsFn: "websearch_to_tsquery", query: query}
}

// MatchesWebSearchWithConfig returns col @@ websearch_to_tsquery($1, $2).
// config is bound as $1 and query as $2, matching the PostgreSQL call signature.
func (c TsvectorColumn) MatchesWebSearchWithConfig(config, query string) Expression {
	return ftsMatchExpr{ref: c.ColBase, tsFn: "websearch_to_tsquery", config: config, query: query, hasConfig: true}
}

// -------------------------------------------------------------------
// ArrayColumn
// -------------------------------------------------------------------

// ArrayColumn is a typed column handle for PostgreSQL array columns (e.g. text[], integer[]).
// Array values are scanned as any; the caller casts to the appropriate Go slice type.
type ArrayColumn struct{ ColBase }

func (c ArrayColumn) EQ(val any) Expression {
	return binaryExpr{ref: c.ColBase, op: "=", val: val}
}
func (c ArrayColumn) NEQ(val any) Expression {
	return binaryExpr{ref: c.ColBase, op: "<>", val: val}
}

// Contains returns a "col @> val" expression (array contains all elements of val).
func (c ArrayColumn) Contains(val any) Expression {
	return binaryExpr{ref: c.ColBase, op: "@>", val: val}
}

// ContainedBy returns a "col <@ val" expression (array is contained by val).
func (c ArrayColumn) ContainedBy(val any) Expression {
	return binaryExpr{ref: c.ColBase, op: "<@", val: val}
}

// Overlaps returns a "col && val" expression (arrays share any elements).
func (c ArrayColumn) Overlaps(val any) Expression {
	return binaryExpr{ref: c.ColBase, op: "&&", val: val}
}
