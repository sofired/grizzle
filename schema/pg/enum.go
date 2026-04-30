package pg

import "fmt"

// EnumDef is the definition of a PostgreSQL named enum type (CREATE TYPE ... AS ENUM).
// Create one with CreateEnum() or SchemaCreateEnum().
type EnumDef struct {
	Name   string
	Schema string   // PostgreSQL schema namespace; empty = "public"
	Values []string // Ordered list of enum label values
}

// QualifiedName returns the schema-qualified type name for use in SQL.
func (e *EnumDef) QualifiedName() string {
	return qualifyName(e.Schema, e.Name)
}

// CreateEnum declares a PostgreSQL named enum type with the given name and values.
// Values are stored in declaration order. CreateEnum panics if any value is empty.
//
//	var StatusEnum = pg.CreateEnum("status", "pending", "active", "archived")
func CreateEnum(name string, values ...string) *EnumDef {
	if len(values) == 0 {
		panic("pg.CreateEnum: at least one enum value is required")
	}
	for i, v := range values {
		if v == "" {
			panic(fmt.Sprintf("pg.CreateEnum: value at index %d is empty; all enum values must be non-empty strings", i))
		}
	}
	return &EnumDef{Name: name, Values: values}
}

// SchemaCreateEnum declares a named enum type inside a named PostgreSQL schema namespace.
// SchemaCreateEnum panics if any value is empty.
//
//	var RoleEnum = pg.SchemaCreateEnum("auth", "role", "admin", "user", "guest")
func SchemaCreateEnum(schema, name string, values ...string) *EnumDef {
	if len(values) == 0 {
		panic("pg.SchemaCreateEnum: at least one enum value is required")
	}
	for i, v := range values {
		if v == "" {
			panic(fmt.Sprintf("pg.SchemaCreateEnum: value at index %d is empty; all enum values must be non-empty strings", i))
		}
	}
	return &EnumDef{Schema: schema, Name: name, Values: values}
}

// EnumColumn starts a column whose SQL type is a named PostgreSQL enum type.
// The typeName must match the name passed to pg.CreateEnum() or pg.SchemaCreateEnum().
// No inline values are required — the type is declared separately.
//
//	pg.C("status", pg.EnumColumn("status").NotNull().Default("pending"))
//
// See also pg.Enum() for inline MySQL-style enum values on a single column.
func EnumColumn(typeName string) *EnumColumnBuilder {
	b := &EnumColumnBuilder{typeName: typeName}
	b.def.SQLType = typeName
	b.def.GoType = GoTypeString
	return b
}
