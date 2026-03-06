package pg

import "strings"

// -------------------------------------------------------------------
// Constraint — the result type for all table-level constraints
// -------------------------------------------------------------------

// ConstraintKind identifies the SQL construct a Constraint represents.
type ConstraintKind string

const (
	KindIndex         ConstraintKind = "index"
	KindUniqueIndex   ConstraintKind = "unique_index"
	KindCheck         ConstraintKind = "check"
	KindForeignKey    ConstraintKind = "foreign_key"
	KindPrimaryKey    ConstraintKind = "primary_key"
	KindUnique        ConstraintKind = "unique"
)

// Constraint describes a table-level constraint or index.
type Constraint struct {
	Kind       ConstraintKind
	Name       string
	Columns    []string  // column names this constraint applies to
	WhereExpr  string    // partial index WHERE clause (raw SQL)
	CheckExpr  string    // CHECK constraint expression (raw SQL)
	// Foreign key fields
	FKTable    string
	FKColumns  []string
	FKOnDelete FKAction
	FKOnUpdate FKAction
}

// -------------------------------------------------------------------
// TableRef — allows the constraint callback to reference columns by name
// -------------------------------------------------------------------

// TableRef is passed into the constraint callback and lets you reference
// columns by name to build partial index WHERE clauses, etc.
type TableRef struct {
	tableName string
	cols      map[string]ColumnDef
}

// Col returns the column name for use in raw SQL expressions.
func (t TableRef) Col(name string) string { return name }

// -------------------------------------------------------------------
// IsNull / IsNotNull helpers for partial index WHERE clauses
// -------------------------------------------------------------------

// IsNull produces a raw "col IS NULL" SQL fragment for use in
// partial index WHERE expressions.
func IsNull(col string) string { return col + " IS NULL" }

// IsNotNull produces a raw "col IS NOT NULL" SQL fragment.
func IsNotNull(col string) string { return col + " IS NOT NULL" }

// -------------------------------------------------------------------
// Index builder
// -------------------------------------------------------------------

// IndexBuilder builds an index or unique index constraint.
type IndexBuilder struct {
	c Constraint
}

// Index starts a non-unique index with the given name.
func Index(name string) *IndexBuilder {
	return &IndexBuilder{c: Constraint{Kind: KindIndex, Name: name}}
}

// UniqueIndex starts a unique index with the given name.
func UniqueIndex(name string) *IndexBuilder {
	return &IndexBuilder{c: Constraint{Kind: KindUniqueIndex, Name: name}}
}

// On specifies the columns covered by the index.
func (b *IndexBuilder) On(cols ...string) *IndexBuilder {
	b.c.Columns = cols
	return b
}

// Where adds a partial index predicate (raw SQL expression).
// Example: pg.UniqueIndex("users_email_idx").On("email").Where(pg.IsNull("deleted_at"))
func (b *IndexBuilder) Where(expr string) *IndexBuilder {
	b.c.WhereExpr = expr
	return b
}

func (b *IndexBuilder) Build() Constraint { return b.c }

// -------------------------------------------------------------------
// Check builder
// -------------------------------------------------------------------

// CheckBuilder builds a CHECK constraint.
type CheckBuilder struct{ c Constraint }

// Check creates a CHECK constraint with the given name and raw SQL expression.
func Check(name, expr string) Constraint {
	return Constraint{Kind: KindCheck, Name: name, CheckExpr: expr}
}

// -------------------------------------------------------------------
// ForeignKey builder
// -------------------------------------------------------------------

// ForeignKeyBuilder builds a composite (multi-column) foreign key constraint.
type ForeignKeyBuilder struct{ c Constraint }

// ForeignKey starts a composite FK constraint.
func ForeignKey(name string) *ForeignKeyBuilder {
	return &ForeignKeyBuilder{c: Constraint{Kind: KindForeignKey, Name: name}}
}

// From specifies the local columns.
func (b *ForeignKeyBuilder) From(cols ...string) *ForeignKeyBuilder {
	b.c.Columns = cols
	return b
}

// References specifies the target table and columns.
func (b *ForeignKeyBuilder) References(table string, cols ...string) *ForeignKeyBuilder {
	b.c.FKTable = table
	b.c.FKColumns = cols
	return b
}

// OnDelete sets the ON DELETE action.
func (b *ForeignKeyBuilder) OnDelete(action FKAction) *ForeignKeyBuilder {
	b.c.FKOnDelete = action
	return b
}

// OnUpdate sets the ON UPDATE action.
func (b *ForeignKeyBuilder) OnUpdate(action FKAction) *ForeignKeyBuilder {
	b.c.FKOnUpdate = action
	return b
}

func (b *ForeignKeyBuilder) Build() Constraint { return b.c }

// -------------------------------------------------------------------
// PrimaryKey builder (composite)
// -------------------------------------------------------------------

// CompositePrimaryKey creates a composite primary key constraint.
func CompositePrimaryKey(cols ...string) Constraint {
	return Constraint{Kind: KindPrimaryKey, Columns: cols}
}

// UniqueConstraint creates a named UNIQUE constraint (not an index).
func UniqueConstraint(name string, cols ...string) Constraint {
	return Constraint{Kind: KindUnique, Name: name, Columns: cols}
}

// -------------------------------------------------------------------
// Convenience: convert IndexBuilder to Constraint directly
// -------------------------------------------------------------------

// Ensure IndexBuilder satisfies a common interface so it can be
// returned from the constraint callback alongside bare Constraint values.
type ConstraintProvider interface {
	Build() Constraint
}

// ToConstraint converts any ConstraintProvider to a Constraint.
func ToConstraint(p ConstraintProvider) Constraint { return p.Build() }

// -------------------------------------------------------------------
// SQL generation helpers (used by Kit)
// -------------------------------------------------------------------

// ToCreateIndexSQL generates the CREATE INDEX statement for this constraint.
func (c Constraint) ToCreateIndexSQL(tableName string) string {
	if c.Kind != KindIndex && c.Kind != KindUniqueIndex {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("CREATE ")
	if c.Kind == KindUniqueIndex {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX ")
	if c.Name != "" {
		sb.WriteString(c.Name + " ")
	}
	sb.WriteString("ON ")
	sb.WriteString(tableName)
	sb.WriteString(" (")
	sb.WriteString(strings.Join(c.Columns, ", "))
	sb.WriteString(")")
	if c.WhereExpr != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(c.WhereExpr)
	}
	return sb.String()
}
