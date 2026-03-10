// Package expr provides the type-safe expression system for G-rizzle.
// Column types (UUIDColumn, StringColumn, etc.) expose only the operators
// that are valid for their SQL type, producing compile-time errors when
// mismatched types are compared.
package expr

import "github.com/sofired/grizzle/dialect"

// BuildContext accumulates bound parameters and carries the active dialect
// during SQL generation. It is threaded through every ToSQL call.
type BuildContext struct {
	args []any
	d    dialect.Dialect
}

// NewBuildContext creates a fresh context for a single query.
func NewBuildContext(d dialect.Dialect) *BuildContext {
	return &BuildContext{d: d}
}

// Add appends a bound value and returns its placeholder string ("$1", "?", etc.).
func (c *BuildContext) Add(val any) string {
	c.args = append(c.args, val)
	return c.d.Placeholder(len(c.args))
}

// Quote wraps an identifier in dialect-appropriate quote characters.
func (c *BuildContext) Quote(name string) string {
	return c.d.QuoteIdent(name)
}

// ColRef returns the fully-qualified "table"."column" reference,
// or just "column" if table is empty.
func (c *BuildContext) ColRef(table, name string) string {
	if table != "" {
		return c.d.QuoteIdent(table) + "." + c.d.QuoteIdent(name)
	}
	return c.d.QuoteIdent(name)
}

// Args returns the ordered slice of bound parameter values.
func (c *BuildContext) Args() []any { return c.args }

// Dialect returns the active dialect.
func (c *BuildContext) Dialect() dialect.Dialect { return c.d }
