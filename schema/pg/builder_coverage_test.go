package pg_test

import (
	"reflect"
	"testing"

	pg "github.com/sofired/grizzle/schema/pg"
)

func TestCompositeForeignKeyBuild(t *testing.T) {
	got := pg.ForeignKey("order_items_product_fk").
		From("tenant_id", "product_id").
		References("products", "tenant_id", "id").
		OnDelete(pg.FKActionCascade).
		OnUpdate(pg.FKActionRestrict).
		Build()
	want := pg.Constraint{
		Kind:       pg.KindForeignKey,
		Name:       "order_items_product_fk",
		Columns:    []string{"tenant_id", "product_id"},
		FKTable:    "products",
		FKColumns:  []string{"tenant_id", "id"},
		FKOnDelete: pg.FKActionCascade,
		FKOnUpdate: pg.FKActionRestrict,
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Build() = %#v, want %#v", got, want)
	}
}

func TestCompositePrimaryKeyBuild(t *testing.T) {
	got := pg.CompositePrimaryKey("tenant_id", "product_id")
	want := pg.Constraint{
		Kind:    pg.KindPrimaryKey,
		Columns: []string{"tenant_id", "product_id"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("CompositePrimaryKey() = %#v, want %#v", got, want)
	}
}

func TestUniqueConstraintBuild(t *testing.T) {
	got := pg.UniqueConstraint("products_tenant_sku_unique", "tenant_id", "sku")
	want := pg.Constraint{
		Kind:    pg.KindUnique,
		Name:    "products_tenant_sku_unique",
		Columns: []string{"tenant_id", "sku"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("UniqueConstraint() = %#v, want %#v", got, want)
	}
}

func TestPlainIndexBuild(t *testing.T) {
	got := pg.Index("products_tenant_sku_idx").
		On("tenant_id", "sku").
		Build()
	want := pg.Constraint{
		Kind:    pg.KindIndex,
		Name:    "products_tenant_sku_idx",
		Columns: []string{"tenant_id", "sku"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Build() = %#v, want %#v", got, want)
	}
	if sql := got.ToCreateIndexSQL("products"); sql != "CREATE INDEX products_tenant_sku_idx ON products (tenant_id, sku)" {
		t.Errorf("ToCreateIndexSQL() = %q, want %q", sql, "CREATE INDEX products_tenant_sku_idx ON products (tenant_id, sku)")
	}
}

func TestJSONBTypeMetadata(t *testing.T) {
	got := pg.JSONB().Type("ProductMetadata").Build("metadata")
	want := pg.ColumnDef{
		Name:        "metadata",
		SQLType:     "jsonb",
		GoType:      pg.GoTypeAny,
		JsonbGoType: "ProductMetadata",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Build() = %#v, want %#v", got, want)
	}
}

func TestNamedColumnBuilderDefaultsAndTypes(t *testing.T) {
	tests := []struct {
		name string
		got  pg.ColumnDef
		want pg.ColumnDef
	}{
		{
			name: "boolean",
			got:  pg.Boolean().Default(true).Build("active"),
			want: pg.ColumnDef{Name: "active", SQLType: "boolean", GoType: pg.GoTypeBool, HasDefault: true, DefaultExpr: "true"},
		},
		{
			name: "integer",
			got:  pg.Integer().Default(3).Build("retry_count"),
			want: pg.ColumnDef{Name: "retry_count", SQLType: "integer", GoType: pg.GoTypeInt, HasDefault: true, DefaultExpr: "3"},
		},
		{
			name: "numeric",
			got:  pg.Numeric(12, 4).Default("0.0000").Build("amount"),
			want: pg.ColumnDef{Name: "amount", SQLType: "numeric(12,4)", GoType: pg.GoTypeString, HasDefault: true, DefaultExpr: "0.0000"},
		},
		{
			name: "json",
			got:  pg.JSON().Default(`{"enabled":true}`).Build("settings"),
			want: pg.ColumnDef{Name: "settings", SQLType: "json", GoType: pg.GoTypeAny, HasDefault: true, DefaultExpr: `'{"enabled":true}'::json`},
		},
		{
			name: "text",
			got:  pg.Text().Default("pending").Build("status"),
			want: pg.ColumnDef{Name: "status", SQLType: "text", GoType: pg.GoTypeString, HasDefault: true, DefaultExpr: "'pending'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Errorf("Build() = %#v, want %#v", tt.got, tt.want)
			}
		})
	}
}
