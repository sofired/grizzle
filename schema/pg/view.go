package pg

// ViewDef is the definition of a PostgreSQL view.
// Create one with CreateView() or SchemaView().
type ViewDef struct {
	Name   string
	Schema string // PostgreSQL schema namespace; empty = "public"
	SQL    string // The SELECT statement body of the view
}

// QualifiedName returns the schema-qualified view name for use in SQL.
func (v *ViewDef) QualifiedName() string {
	if v.Schema != "" && v.Schema != "public" {
		return v.Schema + "." + v.Name
	}
	return v.Name
}

// CreateView declares a PostgreSQL view with the given name and SQL body.
// The sql argument is the SELECT statement that defines the view.
//
//	var ActiveUsers = pg.CreateView("active_users",
//	    `SELECT id, username, email FROM users WHERE enabled = true`)
func CreateView(name, sql string) *ViewDef {
	return &ViewDef{Name: name, SQL: sql}
}

// SchemaView declares a view inside a named PostgreSQL schema namespace.
//
//	var RecentOrders = pg.SchemaView("reporting", "recent_orders",
//	    `SELECT * FROM orders WHERE created_at > now() - interval '7 days'`)
func SchemaView(schema, name, sql string) *ViewDef {
	return &ViewDef{Schema: schema, Name: name, SQL: sql}
}
