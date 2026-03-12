package pg

// -------------------------------------------------------------------
// NamedColumn — a column builder with its name bound
// -------------------------------------------------------------------

// NamedColumn pairs a column name with its builder. Create one with C().
type NamedColumn struct {
	name    string
	builder ColumnBuilder
}

// C binds a column name to a column builder. This is the primary way to
// add columns to a table definition, preserving declaration order.
//
//	pg.C("id",       pg.UUID().PrimaryKey().DefaultRandom()),
//	pg.C("username", pg.Varchar(255).NotNull()),
func C(name string, builder ColumnBuilder) NamedColumn {
	return NamedColumn{name: name, builder: builder}
}

// -------------------------------------------------------------------
// TableDef — the fully-resolved table definition
// -------------------------------------------------------------------

// TableDef is the complete, immutable definition of a PostgreSQL table.
// It carries everything needed for migration snapshot generation and
// Go code generation.
type TableDef struct {
	Name        string
	Schema      string // PostgreSQL schema namespace; empty = "public"
	Columns     []ColumnDef
	Constraints []Constraint
}

// ColMap returns a map of column name → ColumnDef for quick lookups.
func (t TableDef) ColMap() map[string]ColumnDef {
	m := make(map[string]ColumnDef, len(t.Columns))
	for _, c := range t.Columns {
		m[c.Name] = c
	}
	return m
}

// tableBuilder accumulates columns and constraints during construction.
type tableBuilder struct {
	def TableDef
}

// WithConstraints adds table-level constraints (indexes, checks, FKs).
// The callback receives a TableRef for column name resolution and must
// return a slice of Constraint values.
//
//	pg.Table("users", ...).WithConstraints(func(t pg.TableRef) []pg.Constraint {
//	    return []pg.Constraint{
//	        pg.UniqueIndex("users_email_idx").On(t.Col("email")).Where(pg.IsNull(t.Col("deleted_at"))).Build(),
//	        pg.Check("age_check", "age >= 0"),
//	    }
//	})
func (b *tableBuilder) WithConstraints(fn func(t TableRef) []Constraint) *TableDef {
	ref := TableRef{
		tableName: b.def.Name,
		cols:      b.def.ColMap(),
	}
	b.def.Constraints = fn(ref)
	return &b.def
}

// Build finalises the table definition without additional constraints.
func (b *tableBuilder) Build() *TableDef { return &b.def }

// -------------------------------------------------------------------
// Table factory
// -------------------------------------------------------------------

// Table declares a PostgreSQL table with the given name and columns.
// Column order is preserved as declared.
//
// Returns a *tableBuilder so you can chain .WithConstraints() or .Build().
//
//	var Users = pg.Table("users",
//	    pg.C("id",   pg.UUID().PrimaryKey().DefaultRandom()),
//	    pg.C("name", pg.Varchar(255).NotNull()),
//	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
//	    return []pg.Constraint{
//	        pg.UniqueIndex("users_name_idx").On(t.Col("name")).Build(),
//	    }
//	})
func Table(name string, cols ...NamedColumn) *tableBuilder {
	defs := make([]ColumnDef, len(cols))
	for i, nc := range cols {
		defs[i] = nc.builder.Build(nc.name)
	}
	return &tableBuilder{
		def: TableDef{
			Name:    name,
			Columns: defs,
		},
	}
}

// SchemaTable declares a table inside a named PostgreSQL schema namespace
// (e.g. "auth", "audit"). The generated DDL will be:
//
//	CREATE TABLE <schema>.<name> (...)
func SchemaTable(schema, name string, cols ...NamedColumn) *tableBuilder {
	b := Table(name, cols...)
	b.def.Schema = schema
	return b
}

// QualifiedName returns the schema-qualified table name for use in SQL.
// Returns just the table name if no schema is set.
func (t *TableDef) QualifiedName() string {
	if t.Schema != "" {
		return t.Schema + "." + t.Name
	}
	return t.Name
}
