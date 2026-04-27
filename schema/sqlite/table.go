package sqlite

import pg "github.com/sofired/grizzle/schema/pg"

// ---------------------------------------------------------------------------
// Dialect-specific TableDef
// ---------------------------------------------------------------------------

// TableDef is the complete, immutable definition of a SQLite table.
// It embeds *pg.TableDef so that column, constraint, and schema fields are
// directly accessible while carrying its own Dialect() identity.
//
// *TableDef implements pg.TableDefiner so it can be passed to any kit
// function that accepts dialect-agnostic table definitions (e.g. kit.FromDefs,
// kit.GenerateCreateSQLSQLite, kit.MigrateSQLite).
type TableDef struct {
	*pg.TableDef
}

// Def returns the embedded *pg.TableDef, giving kit and migration code
// access to column and constraint data.
func (t *TableDef) Def() *pg.TableDef { return t.TableDef }

// Dialect returns "sqlite" to identify this table definition's originating
// schema package.
func (t *TableDef) Dialect() string { return "sqlite" }

// ---------------------------------------------------------------------------
// Type aliases shared with schema/pg
// ---------------------------------------------------------------------------

type (
	// TableBuilder accumulates columns and constraints during table construction.
	// It is returned by sqlite.Table() and sqlite.SchemaTable(). Consumers that
	// need to store or pass the builder type explicitly may use *sqlite.TableBuilder.
	TableBuilder = pg.TableBuilder

	// NamedColumn pairs a column name with its builder (produced by C()).
	NamedColumn = pg.NamedColumn

	// TableRef is passed into the WithConstraints callback for column name resolution.
	TableRef = pg.TableRef

	// Constraint describes a table-level constraint or index.
	Constraint = pg.Constraint

	// ConstraintKind identifies the SQL construct a Constraint describes.
	ConstraintKind = pg.ConstraintKind
)

// Constraint kind constants.
const (
	KindIndex       = pg.KindIndex
	KindUniqueIndex = pg.KindUniqueIndex
	KindCheck       = pg.KindCheck
	KindForeignKey  = pg.KindForeignKey
	KindPrimaryKey  = pg.KindPrimaryKey
	KindUnique      = pg.KindUnique
)

// ---------------------------------------------------------------------------
// sqliteTableBuilder — wraps pg.TableBuilder to produce *sqlite.TableDef
// ---------------------------------------------------------------------------

// sqliteTableBuilder accumulates columns and constraints for a SQLite table.
type sqliteTableBuilder struct {
	pgBuilder *pg.TableBuilder
}

// WithConstraints adds table-level constraints.
// The callback receives a TableRef for column name resolution.
func (b *sqliteTableBuilder) WithConstraints(fn func(t TableRef) []Constraint) *TableDef {
	return &TableDef{TableDef: b.pgBuilder.WithConstraints(fn)}
}

// Build finalises the table definition without additional constraints.
func (b *sqliteTableBuilder) Build() *TableDef {
	return &TableDef{TableDef: b.pgBuilder.Build()}
}

// ---------------------------------------------------------------------------
// Table construction helpers
// ---------------------------------------------------------------------------

// C binds a column name to a column builder.
//
//	sqlite.C("id",    sqlite.Integer().PrimaryKey()),
//	sqlite.C("title", sqlite.Text().NotNull()),
var C = pg.C

// Table declares a SQLite table with the given name and columns.
// Returns a builder; chain .WithConstraints() or .Build() to finalise.
//
//	var Notes = sqlite.Table("notes",
//	    sqlite.C("id",    sqlite.Integer().PrimaryKey()),
//	    sqlite.C("title", sqlite.Text().NotNull()),
//	).Build()
func Table(name string, cols ...NamedColumn) *sqliteTableBuilder {
	return &sqliteTableBuilder{pgBuilder: pg.NewTableBuilder(name, cols...)}
}

// SchemaTable declares a table inside a named schema namespace.
// In SQLite, schema names correspond to attached database aliases.
func SchemaTable(schema, name string, cols ...NamedColumn) *sqliteTableBuilder {
	return &sqliteTableBuilder{pgBuilder: pg.NewSchemaTableBuilder(schema, name, cols...)}
}

// ---------------------------------------------------------------------------
// Constraint constructors — identical to schema/pg
// ---------------------------------------------------------------------------

// Index starts a non-unique index with the given name.
var Index = pg.Index

// UniqueIndex starts a unique index with the given name.
var UniqueIndex = pg.UniqueIndex

// Check creates a CHECK constraint.
var Check = pg.Check

// ForeignKey starts a composite (multi-column) foreign key constraint.
// Note: SQLite enforces foreign keys only when PRAGMA foreign_keys = ON.
var ForeignKey = pg.ForeignKey

// CompositePrimaryKey creates a composite primary key constraint.
var CompositePrimaryKey = pg.CompositePrimaryKey

// UniqueConstraint creates a named UNIQUE constraint (not a separate index).
var UniqueConstraint = pg.UniqueConstraint

// IsNull produces a raw "col IS NULL" SQL fragment for use in WHERE expressions.
var IsNull = pg.IsNull

// IsNotNull produces a raw "col IS NOT NULL" SQL fragment.
var IsNotNull = pg.IsNotNull
