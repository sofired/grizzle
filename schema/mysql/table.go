package mysql

import pg "github.com/sofired/grizzle/schema/pg"

// ---------------------------------------------------------------------------
// Dialect-specific TableDef
// ---------------------------------------------------------------------------

// TableDef is the complete, immutable definition of a MySQL/MariaDB table.
// It embeds *pg.TableDef so that column, constraint, and schema fields are
// directly accessible while carrying its own Dialect() identity.
//
// *TableDef implements pg.TableDefiner so it can be passed to any kit
// function that accepts dialect-agnostic table definitions (e.g. kit.FromDefs,
// kit.GenerateCreateSQLMySQL, kit.MigrateMySQL).
type TableDef struct {
	*pg.TableDef
}

// Def returns the embedded *pg.TableDef, giving kit and migration code
// access to column and constraint data.
func (t *TableDef) Def() *pg.TableDef { return t.TableDef }

// Dialect returns "mysql" to identify this table definition's originating
// schema package.
func (t *TableDef) Dialect() string { return "mysql" }

// ---------------------------------------------------------------------------
// Type aliases shared with schema/pg
// ---------------------------------------------------------------------------

type (
	// TableBuilder accumulates columns and constraints during table construction.
	// It is returned by mysql.Table() and mysql.SchemaTable(). Consumers that
	// need to store or pass the builder type explicitly may use *mysql.TableBuilder.
	TableBuilder = pg.TableBuilder

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
// mysqlTableBuilder — wraps pg.TableBuilder to produce *mysql.TableDef
// ---------------------------------------------------------------------------

// mysqlTableBuilder accumulates columns and constraints for a MySQL table.
type mysqlTableBuilder struct {
	pgBuilder *pg.TableBuilder
}

// RenamedFrom declares that this table was renamed from oldName.
// Diff() will emit ChangeRenameTable instead of drop+create when oldName
// matches a dropped table in the old snapshot.
// Remove this call from your schema definition once the migration has been applied.
func (b *mysqlTableBuilder) RenamedFrom(oldName string) *mysqlTableBuilder {
	b.pgBuilder.RenamedFrom(oldName)
	return b
}

// WithConstraints adds table-level constraints.
// The callback receives a TableRef for column name resolution.
func (b *mysqlTableBuilder) WithConstraints(fn func(t TableRef) []Constraint) *TableDef {
	return &TableDef{TableDef: b.pgBuilder.WithConstraints(fn)}
}

// Build finalises the table definition without additional constraints.
func (b *mysqlTableBuilder) Build() *TableDef {
	return &TableDef{TableDef: b.pgBuilder.Build()}
}

// ---------------------------------------------------------------------------
// Table construction helpers
// ---------------------------------------------------------------------------

// C binds a column name to a column builder.
//
//	mysql.C("id",       mysql.UUID().PrimaryKey().DefaultRandom()),
//	mysql.C("username", mysql.Varchar(255).NotNull()),
var C = pg.C

// Table declares a MySQL table with the given name and columns.
// Returns a builder; chain .WithConstraints() or .Build() to finalise.
//
//	var Users = mysql.Table("users",
//	    mysql.C("id",   mysql.UUID().PrimaryKey().DefaultRandom()),
//	    mysql.C("name", mysql.Varchar(255).NotNull()),
//	).Build()
func Table(name string, cols ...NamedColumn) *mysqlTableBuilder {
	return &mysqlTableBuilder{pgBuilder: pg.NewTableBuilder(name, cols...)}
}

// SchemaTable declares a table inside a named schema namespace.
// In MySQL, schema names correspond to database names.
func SchemaTable(schema, name string, cols ...NamedColumn) *mysqlTableBuilder {
	return &mysqlTableBuilder{pgBuilder: pg.NewSchemaTableBuilder(schema, name, cols...)}
}

// ---------------------------------------------------------------------------
// Constraint constructors — identical to schema/pg
// ---------------------------------------------------------------------------

// Index starts a non-unique index with the given name.
var Index = pg.Index

// UniqueIndex starts a unique index with the given name.
// Note: MySQL does not support partial indexes (WHERE clause) before 8.0.13.
// The .Where() expression is accepted but silently dropped in MySQL DDL output.
var UniqueIndex = pg.UniqueIndex

// Check creates a CHECK constraint.
// MySQL 8.0+ supports CHECK constraints; earlier versions parse but ignore them.
var Check = pg.Check

// ForeignKey starts a composite (multi-column) foreign key constraint.
var ForeignKey = pg.ForeignKey

// CompositePrimaryKey creates a composite primary key constraint.
var CompositePrimaryKey = pg.CompositePrimaryKey

// UniqueConstraint creates a named UNIQUE constraint (not a separate index).
var UniqueConstraint = pg.UniqueConstraint

// IsNull produces a raw "col IS NULL" SQL fragment for use in WHERE expressions.
var IsNull = pg.IsNull

// IsNotNull produces a raw "col IS NOT NULL" SQL fragment.
var IsNotNull = pg.IsNotNull
