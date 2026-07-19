// Package expr provides the type-safe expression system for Grizzle.
// Column types (UUIDColumn, StringColumn, etc.) expose only the operators
// that are valid for their SQL type, producing compile-time errors when
// mismatched types are compared.
package expr

import (
	"strings"
	"unicode"

	"github.com/sofired/grizzle/dialect"
)

// BuildContext accumulates bound parameters and carries the active dialect
// during SQL generation. It is threaded through every RenderSQL call.
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

// Quote validates, escapes, and wraps one identifier part in
// dialect-appropriate quote characters.
func (c *BuildContext) Quote(name string) (string, error) {
	if err := validateIdentifier(name); err != nil {
		return "", err
	}
	return c.d.QuoteIdent(name), nil
}

// ColRef returns the fully-qualified "table"."column" reference,
// or just "column" if table is empty.
func (c *BuildContext) ColRef(table, name string) (string, error) {
	column, err := c.Quote(name)
	if err != nil {
		return "", err
	}
	if table != "" {
		qualified, err := c.Quote(table)
		if err != nil {
			return "", err
		}
		return qualified + "." + column, nil
	}
	return column, nil
}

// Args returns the ordered slice of bound parameter values.
func (c *BuildContext) Args() []any { return c.args }

// Dialect returns the active dialect.
func (c *BuildContext) Dialect() dialect.Dialect { return c.d }

// renderAtomically restores the argument slice when a composite expression
// fails after one of its children has already bound values.
func renderAtomically(c *BuildContext, render func() (string, error)) (string, error) {
	checkpoint := len(c.args)
	sql, err := render()
	if err == nil {
		return sql, nil
	}
	for i := checkpoint; i < len(c.args); i++ {
		c.args[i] = nil
	}
	c.args = c.args[:checkpoint]
	return "", err
}

func validateIdentifier(name string) error {
	if name == "" || strings.Contains(name, ".") {
		return NewError(CodeInvalidIdentifier, "quote_identifier", "identifier part is invalid")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return NewError(CodeInvalidIdentifier, "quote_identifier", "identifier part is invalid")
		}
	}
	return nil
}
