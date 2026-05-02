package pg

import "strings"

// qualifyName returns "schema.name" for non-public schemas, otherwise just "name".
func qualifyName(schema, name string) string {
	if schema != "" && schema != "public" {
		return schema + "." + name
	}
	return name
}

// ViewDef is the definition of a PostgreSQL view.
// Create one with CreateView() or SchemaView().
type ViewDef struct {
	Name   string
	Schema string // PostgreSQL schema namespace; empty = "public"
	// SQL is the SELECT statement body of the view.
	// Must be a trusted, developer-authored SQL string — never interpolate
	// runtime user input, as this is embedded verbatim in generated DDL.
	SQL string
}

// QualifiedName returns the schema-qualified view name for use in SQL.
func (v *ViewDef) QualifiedName() string {
	return qualifyName(v.Schema, v.Name)
}

// CreateView declares a PostgreSQL view with the given name and SQL body.
// Panics if name or sql is empty.
// The sql argument must be a trusted, developer-authored SELECT statement —
// never interpolate runtime user input into it, as it is embedded verbatim in DDL.
//
//	var ActiveUsers = pg.CreateView("active_users",
//	    `SELECT id, username, email FROM users WHERE enabled = true`)
func CreateView(name, sql string) *ViewDef {
	if name == "" {
		panic("pg.CreateView: name must not be empty")
	}
	if sql == "" {
		panic("pg.CreateView: sql must not be empty")
	}
	return &ViewDef{Name: name, SQL: sql}
}

// SchemaView declares a view inside a named PostgreSQL schema namespace.
// Panics if schema contains a ".", name is empty, or sql is empty.
//
//	var RecentOrders = pg.SchemaView("reporting", "recent_orders",
//	    `SELECT * FROM orders WHERE created_at > now() - interval '7 days'`)
func SchemaView(schema, name, sql string) *ViewDef {
	if strings.Contains(schema, ".") {
		panic(`pg.SchemaView: schema must not contain "."; pass the bare schema name, e.g. SchemaView("myschema", "view_name", sql)`)
	}
	if schema == "" {
		panic("pg.SchemaView: schema must not be empty; use CreateView for public-schema views")
	}
	if name == "" {
		panic("pg.SchemaView: name must not be empty")
	}
	if sql == "" {
		panic("pg.SchemaView: sql must not be empty")
	}
	return &ViewDef{Schema: schema, Name: name, SQL: sql}
}
