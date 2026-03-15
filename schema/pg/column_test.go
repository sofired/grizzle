package pg_test

import (
	"testing"

	pg "github.com/sofired/grizzle/schema/pg"
)

// -------------------------------------------------------------------
// Fix #111 — EnumColumnBuilder.Default escapes single quotes
// -------------------------------------------------------------------

func TestEnumColumn_Default_EscapesSingleQuote(t *testing.T) {
	col := pg.Enum("mood", "happy", "it's fine", "sad").
		Default("it's fine").
		Build("status")

	if !col.HasDefault {
		t.Error("expected HasDefault = true")
	}
	// The expected output is 'it''s fine'::mood — single quote doubled.
	want := "'it''s fine'::mood"
	if col.DefaultExpr != want {
		t.Errorf("expected default expr %q, got: %q", want, col.DefaultExpr)
	}
}

func TestEnumColumn_Default_NoQuoteInValue(t *testing.T) {
	col := pg.Enum("status", "active", "inactive").
		Default("active").
		Build("status")

	if col.DefaultExpr != "'active'::status" {
		t.Errorf("expected \"'active'::status\", got: %s", col.DefaultExpr)
	}
}

// -------------------------------------------------------------------
// Fix #118 — Enum() validates non-empty values
// -------------------------------------------------------------------

func TestEnum_EmptyValues_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for empty values slice")
		}
	}()
	pg.Enum("mood")
}

func TestEnum_EmptyStringValue_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic for empty-string value")
		}
	}()
	pg.Enum("mood", "happy", "", "sad")
}

func TestEnum_ValidValues_NoPanic(t *testing.T) {
	// Should not panic.
	col := pg.Enum("mood", "happy", "neutral", "sad").NotNull().Build("mood")
	if col.SQLType != "mood" {
		t.Errorf("expected SQLType = %q, got %q", "mood", col.SQLType)
	}
	if !col.NotNull {
		t.Error("expected NotNull = true")
	}
}

// -------------------------------------------------------------------
// Fix #13 — CheckBuilder removed; Check() still works
// -------------------------------------------------------------------

func TestCheck_StillWorks(t *testing.T) {
	c := pg.Check("price_positive", "price > 0")
	if c.Kind != pg.KindCheck {
		t.Errorf("expected KindCheck, got %s", c.Kind)
	}
	if c.Name != "price_positive" {
		t.Errorf("expected name %q, got %q", "price_positive", c.Name)
	}
	if c.CheckExpr != "price > 0" {
		t.Errorf("expected expr %q, got %q", "price > 0", c.CheckExpr)
	}
}
