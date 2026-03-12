package sqlite

import pg "github.com/sofired/grizzle/schema/pg"

// ---------------------------------------------------------------------------
// Type aliases for table construction
// ---------------------------------------------------------------------------

type (
	// TableDef is the complete, immutable definition of a table.
	TableDef = pg.TableDef

	// NamedColumn pairs a column name with its builder (produced by C()).
	NamedColumn = pg.NamedColumn

	// TableRef is passed into the WithConstraints callback for column name resolution.
	TableRef = pg.TableRef

	// Constraint describes a table-level constraint or index.
	Constraint = pg.Constraint

	// ConstraintKind identifies the SQL construct a Constraint represents.
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
// Table construction helpers — identical to schema/pg
// ---------------------------------------------------------------------------

// C binds a column name to a column builder.
//
//	sqlite.C("id",    sqlite.Integer().PrimaryKey()),
//	sqlite.C("title", sqlite.Text().NotNull()),
var C = pg.C

// Table declares a table with the given name and columns.
// Returns a builder; chain .WithConstraints() or .Build() to finalise.
//
//	var Notes = sqlite.Table("notes",
//	    sqlite.C("id",    sqlite.Integer().PrimaryKey()),
//	    sqlite.C("title", sqlite.Text().NotNull()),
//	).Build()
var Table = pg.Table

// SchemaTable declares a table inside a named schema namespace.
// In SQLite, schema names correspond to attached database aliases.
var SchemaTable = pg.SchemaTable

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
