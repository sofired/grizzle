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
//
// TableDef implements TableDefiner (via the Def/Dialect methods below) so
// that *TableDef can be passed wherever a dialect-agnostic TableDefiner is
// expected — e.g. kit.FromDefs, kit.Push, kit.Migrate.
type TableDef struct {
	Name        string
	Schema      string // PostgreSQL schema namespace; empty = "public"
	Columns     []ColumnDef
	Constraints []Constraint
	// PreviousName is intentionally excluded from JSON snapshots — it is only
	// meaningful as a schema definition annotation for the current migration step
	// and must not persist across snapshot saves. If it were persisted, a future
	// table that happens to share the old name would trigger a spurious RENAME
	// instead of a CREATE.
	PreviousName string `json:"-"`
}

// Def returns a pointer to the receiver so that *TableDef satisfies
// the TableDefiner interface.
func (t *TableDef) Def() *TableDef { return t }

// Dialect returns "postgres" for a *TableDef produced by schema/pg.
func (t *TableDef) Dialect() string { return "postgres" }

// ColMap returns a map of column name → ColumnDef for quick lookups.
func (t TableDef) ColMap() map[string]ColumnDef {
	m := make(map[string]ColumnDef, len(t.Columns))
	for _, c := range t.Columns {
		m[c.Name] = c
	}
	return m
}

// QualifiedName returns the schema-qualified table name for use in SQL.
// Returns just the table name if no schema is set.
func (t *TableDef) QualifiedName() string {
	if t.Schema != "" {
		return t.Schema + "." + t.Name
	}
	return t.Name
}

// -------------------------------------------------------------------
// TableBuilder — internal builder type (exported for dialect packages)
// -------------------------------------------------------------------

// TableBuilder accumulates columns and constraints during construction.
// It is exported so that dialect packages (schema/mysql, schema/sqlite)
// can wrap it to produce their own dialect-specific table definition types.
type TableBuilder struct {
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
func (b *TableBuilder) WithConstraints(fn func(t TableRef) []Constraint) *TableDef {
	ref := TableRef{
		tableName: b.def.Name,
		cols:      b.def.ColMap(),
	}
	b.def.Constraints = fn(ref)
	return &b.def
}

// RenamedFrom declares that this table was renamed from oldName.
// Diff() will emit ChangeRenameTable instead of drop+create when oldName
// matches a dropped table in the old snapshot. Leave empty for new tables
// or tables whose name has not changed.
//
// oldName must match the qualified map key used in the old snapshot:
//   - For tables without a schema (or schema "public"): pass the bare table
//     name, e.g. "accounts".
//   - For tables inside a named schema: pass the dot-separated qualified name,
//     e.g. "auth.accounts" — matching the key that FromDefs() would have stored.
//
// Passing an unqualified name when the old table had a schema (or vice-versa)
// will result in no rename being detected; Diff() will fall back to drop+create.
// Remove this call from your schema definition once the migration has been applied.
func (b *TableBuilder) RenamedFrom(oldName string) *TableBuilder {
	b.def.PreviousName = oldName
	return b
}

// Build finalises the table definition without additional constraints.
func (b *TableBuilder) Build() *TableDef { return &b.def }

// -------------------------------------------------------------------
// Table factories
// -------------------------------------------------------------------

// Table declares a PostgreSQL table with the given name and columns.
// Column order is preserved as declared.
//
// Returns a *TableBuilder so you can chain .WithConstraints() or .Build().
//
//	var Users = pg.Table("users",
//	    pg.C("id",   pg.UUID().PrimaryKey().DefaultRandom()),
//	    pg.C("name", pg.Varchar(255).NotNull()),
//	).WithConstraints(func(t pg.TableRef) []pg.Constraint {
//	    return []pg.Constraint{
//	        pg.UniqueIndex("users_name_idx").On(t.Col("name")).Build(),
//	    }
//	})
func Table(name string, cols ...NamedColumn) *TableBuilder {
	return NewTableBuilder(name, cols...)
}

// SchemaTable declares a table inside a named PostgreSQL schema namespace
// (e.g. "auth", "audit"). The generated DDL will be:
//
//	CREATE TABLE <schema>.<name> (...)
func SchemaTable(schema, name string, cols ...NamedColumn) *TableBuilder {
	return NewSchemaTableBuilder(schema, name, cols...)
}

// NewTableBuilder constructs a TableBuilder for the given table name and columns.
// This lower-level constructor is used by dialect packages (schema/mysql,
// schema/sqlite) to create their own dialect-typed wrappers.
func NewTableBuilder(name string, cols ...NamedColumn) *TableBuilder {
	defs := make([]ColumnDef, len(cols))
	for i, nc := range cols {
		defs[i] = nc.builder.Build(nc.name)
	}
	return &TableBuilder{
		def: TableDef{
			Name:    name,
			Columns: defs,
		},
	}
}

// NewSchemaTableBuilder constructs a TableBuilder for a schema-namespaced table.
// This lower-level constructor is used by dialect packages.
func NewSchemaTableBuilder(schema, name string, cols ...NamedColumn) *TableBuilder {
	b := NewTableBuilder(name, cols...)
	b.def.Schema = schema
	return b
}
