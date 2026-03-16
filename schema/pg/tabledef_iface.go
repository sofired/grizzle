package pg

// TableDefiner is the interface that all dialect-specific table definitions
// must satisfy. It provides a dialect-agnostic handle to the underlying
// concrete *TableDef struct and carries the name of the originating dialect.
//
// All three built-in schema packages (schema/pg, schema/mysql, schema/sqlite)
// produce values that satisfy this interface. Kit functions accept TableDefiner
// so callers may pass tables from any dialect without a cross-dialect type leak.
//
// Example (PostgreSQL):
//
//	var Users = pg.Table("users", pg.C("id", pg.UUID().PrimaryKey())).Build()
//	// *pg.TableDef satisfies pg.TableDefiner
//
// Example (MySQL):
//
//	var Orders = mysql.Table("orders", mysql.C("id", mysql.BigSerial())).Build()
//	// *mysql.TableDef satisfies pg.TableDefiner
//
// Example (SQLite):
//
//	var Notes = sqlite.Table("notes", sqlite.C("id", sqlite.Integer().PrimaryKey())).Build()
//	// *sqlite.TableDef satisfies pg.TableDefiner
type TableDefiner interface {
	// Def returns the underlying concrete table definition. Kit and migration
	// code use this to access column and constraint data.
	Def() *TableDef

	// Dialect returns the name of the schema package that produced this
	// table definition: "postgres", "mysql", or "sqlite".
	Dialect() string
}
