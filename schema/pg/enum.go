package pg

import "fmt"

// EnumDef is the definition of a PostgreSQL enum type (CREATE TYPE ... AS ENUM).
// Create one with Enum().
type EnumDef struct {
	Name   string
	Schema string   // PostgreSQL schema namespace; empty = "public"
	Values []string // Ordered list of enum label values
}

// QualifiedName returns the schema-qualified type name for use in SQL.
func (e *EnumDef) QualifiedName() string {
	if e.Schema != "" {
		return e.Schema + "." + e.Name
	}
	return e.Name
}

// Enum declares a PostgreSQL enum type with the given name and values.
// Values are stored in declaration order and must be non-empty strings.
//
//	var StatusEnum = pg.Enum("status", "pending", "active", "archived")
func Enum(name string, values ...string) *EnumDef {
	return &EnumDef{Name: name, Values: values}
}

// SchemaEnum declares an enum type inside a named PostgreSQL schema namespace.
//
//	var RoleEnum = pg.SchemaEnum("auth", "role", "admin", "user", "guest")
func SchemaEnum(schema, name string, values ...string) *EnumDef {
	return &EnumDef{Schema: schema, Name: name, Values: values}
}

// -------------------------------------------------------------------
// EnumColumn — a column builder that references a named enum type
// -------------------------------------------------------------------

// EnumColumnBuilder builds a column definition whose SQL type is a user-defined
// PostgreSQL enum. The typeName must match the name used in Enum().
type EnumColumnBuilder struct {
	colBuilder
}

// EnumColumn starts a column whose SQL type is the named enum type.
// The typeName should be the unqualified or qualified name of the enum,
// matching what was passed to pg.Enum() or pg.SchemaEnum().
//
//	pg.C("status", pg.EnumColumn("status").NotNull().Default("pending"))
func EnumColumn(typeName string) *EnumColumnBuilder {
	b := &EnumColumnBuilder{}
	b.def.SQLType = typeName
	b.def.GoType = GoTypeString
	return b
}

func (b *EnumColumnBuilder) NotNull() *EnumColumnBuilder { b.setNotNull(); return b }
func (b *EnumColumnBuilder) Default(val string) *EnumColumnBuilder {
	b.setDefault(fmt.Sprintf("'%s'", val))
	return b
}
func (b *EnumColumnBuilder) Build(name string) ColumnDef { return b.build(name) }
