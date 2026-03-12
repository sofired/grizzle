package introspect

import (
	"testing"

	pg "github.com/sofired/grizzle/schema/pg"
)

// TestNormalizeFKAction verifies that information_schema referential action
// strings are correctly mapped to pg.FKAction constants.
func TestNormalizeFKAction(t *testing.T) {
	cases := []struct {
		input string
		want  pg.FKAction
	}{
		{"CASCADE", pg.FKActionCascade},
		{"cascade", pg.FKActionCascade}, // case-insensitive
		{"SET NULL", pg.FKActionSetNull},
		{"SET DEFAULT", pg.FKActionSetDefault},
		{"RESTRICT", pg.FKActionRestrict},
		{"NO ACTION", pg.FKActionNoAction},
		{"", pg.FKActionNoAction},        // empty → no action
		{"UNKNOWN", pg.FKActionNoAction}, // unmapped → no action
	}
	for _, tc := range cases {
		got := pg.FKAction(normalizeFKAction(tc.input))
		if got != tc.want {
			t.Errorf("normalizeFKAction(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestFKConstraintFields verifies that the FK constraint struct is populated
// with the expected fields when constructed as queryForeignKeys would produce.
func TestFKConstraintFields(t *testing.T) {
	// Simulate what queryForeignKeys assembles from query rows.
	c := pg.Constraint{
		Kind:       pg.KindForeignKey,
		Name:       "orders_customer_fk",
		Columns:    []string{"customer_id"},
		FKTable:    "customers",
		FKColumns:  []string{"id"},
		FKOnDelete: pg.FKAction(normalizeFKAction("CASCADE")),
		FKOnUpdate: pg.FKAction(normalizeFKAction("NO ACTION")),
	}

	if c.Kind != pg.KindForeignKey {
		t.Errorf("expected KindForeignKey, got %q", c.Kind)
	}
	if c.Name != "orders_customer_fk" {
		t.Errorf("unexpected constraint name: %q", c.Name)
	}
	if len(c.Columns) != 1 || c.Columns[0] != "customer_id" {
		t.Errorf("unexpected local columns: %v", c.Columns)
	}
	if c.FKTable != "customers" {
		t.Errorf("unexpected FK table: %q", c.FKTable)
	}
	if len(c.FKColumns) != 1 || c.FKColumns[0] != "id" {
		t.Errorf("unexpected FK columns: %v", c.FKColumns)
	}
	if c.FKOnDelete != pg.FKActionCascade {
		t.Errorf("unexpected FKOnDelete: %q", c.FKOnDelete)
	}
	if c.FKOnUpdate != pg.FKActionNoAction {
		t.Errorf("unexpected FKOnUpdate: %q", c.FKOnUpdate)
	}
}

// TestFKConstraintFields_MultiColumn verifies composite FK columns are handled.
func TestFKConstraintFields_MultiColumn(t *testing.T) {
	c := pg.Constraint{
		Kind:       pg.KindForeignKey,
		Name:       "orders_product_fk",
		Columns:    []string{"product_id", "variant_id"},
		FKTable:    "products",
		FKColumns:  []string{"id", "variant_id"},
		FKOnDelete: pg.FKAction(normalizeFKAction("RESTRICT")),
		FKOnUpdate: pg.FKAction(normalizeFKAction("CASCADE")),
	}

	if len(c.Columns) != 2 {
		t.Fatalf("expected 2 local columns, got %d", len(c.Columns))
	}
	if c.Columns[0] != "product_id" || c.Columns[1] != "variant_id" {
		t.Errorf("unexpected local columns: %v", c.Columns)
	}
	if len(c.FKColumns) != 2 {
		t.Fatalf("expected 2 FK columns, got %d", len(c.FKColumns))
	}
	if c.FKColumns[0] != "id" || c.FKColumns[1] != "variant_id" {
		t.Errorf("unexpected FK columns: %v", c.FKColumns)
	}
	if c.FKOnDelete != pg.FKActionRestrict {
		t.Errorf("unexpected FKOnDelete: %q", c.FKOnDelete)
	}
	if c.FKOnUpdate != pg.FKActionCascade {
		t.Errorf("unexpected FKOnUpdate: %q", c.FKOnUpdate)
	}
}
