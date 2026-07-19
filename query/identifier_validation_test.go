package query_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/sofired/grizzle/dialect"
	"github.com/sofired/grizzle/expr"
	"github.com/sofired/grizzle/query"
)

type identifierTable struct {
	name  string
	alias string
}

func (t identifierTable) GrizTableName() string  { return t.name }
func (t identifierTable) GrizTableAlias() string { return t.alias }

type identifierRow struct {
	Name string `db:"name"`
}

type dottedIdentifierRow struct {
	Value string `db:"unsafe.identifier"`
}

func TestBuild_InvalidIdentifierClassesFailClosedAndRedacted(t *testing.T) {
	invalid := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "dotted", value: "audit.users"},
		{name: "nul", value: "nul\x00byte"},
		{name: "line_feed", value: "line\nfeed"},
		{name: "carriage_return", value: "carriage\rreturn"},
		{name: "delete", value: "delete\x7fchar"},
		{name: "c0_control", value: "control\x01char"},
		{name: "c1_control", value: "control\u0085char"},
	}

	for _, invalidPart := range invalid {
		t.Run(invalidPart.name, func(t *testing.T) {
			table := identifierTable{name: invalidPart.value, alias: invalidPart.value}
			aliasedTable := identifierTable{name: "users", alias: invalidPart.value}
			builders := map[string]query.Builder{
				"select_name":  query.Select().From(table),
				"insert_name":  query.InsertInto(table).Values(identifierRow{Name: "sensitive-value"}),
				"update_name":  query.Update(table).Set("name", "sensitive-value"),
				"delete_name":  query.DeleteFrom(table),
				"select_alias": query.Select().From(aliasedTable),
				"insert_alias": query.InsertInto(aliasedTable).Values(identifierRow{Name: "sensitive-value"}),
				"update_alias": query.Update(aliasedTable).Set("name", "sensitive-value"),
				"delete_alias": query.DeleteFrom(aliasedTable),
				"set_operation": query.Select().From(table).
					Union(query.Select().From(identifierTable{name: "users", alias: "users"})),
			}
			for name, builder := range builders {
				t.Run(name, func(t *testing.T) {
					assertInvalidIdentifierBuild(t, builder, dialect.Postgres, invalidPart.value)
				})
			}
		})
	}
}

func TestBuild_AllIdentifierBearingSurfacesUseValidation(t *testing.T) {
	const unsafe = "unsafe\nidentifier"
	validTable := identifierTable{name: "users", alias: "users"}
	unsafeAliasTable := identifierTable{name: "users", alias: unsafe}
	validColumn := expr.StringColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "name"}}
	unsafeTableColumn := expr.StringColumn{ColBase: expr.ColBase{TableAlias: unsafe, ColName: "name"}}
	unsafeNameColumn := expr.StringColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: unsafe}}
	validIntColumn := expr.IntColumn{ColBase: expr.ColBase{TableAlias: "users", ColName: "age"}}
	validRow := identifierRow{Name: "sensitive-value"}

	cases := []struct {
		name      string
		builder   query.Builder
		untrusted string
	}{
		{name: "select_source_alias", builder: query.Select().From(unsafeAliasTable), untrusted: unsafe},
		{name: "join_source_alias", builder: query.Select().From(validTable).InnerJoin(unsafeAliasTable, expr.Raw("TRUE")), untrusted: unsafe},
		{name: "cte_name", builder: query.Select().With(unsafe, query.Select().From(validTable)).From(validTable), untrusted: unsafe},
		{name: "cte_reference", builder: query.Select().From(query.CTERef(unsafe)), untrusted: unsafe},
		{name: "subquery_alias", builder: query.Select().From(query.FromSubquery(query.Select().From(validTable), unsafe)), untrusted: unsafe},
		{name: "column_table", builder: query.Select(unsafeTableColumn).From(validTable), untrusted: unsafe},
		{name: "column_name", builder: query.Select(unsafeNameColumn).From(validTable), untrusted: unsafe},
		{name: "column_alias", builder: query.Select(expr.ColAs(validColumn, unsafe)).From(validTable), untrusted: unsafe},
		{name: "aggregate_alias", builder: query.Select(expr.Count().As(unsafe)).From(validTable), untrusted: unsafe},
		{name: "window_alias", builder: query.Select(expr.RowNumber().As(unsafe)).From(validTable), untrusted: unsafe},
		{name: "function_alias", builder: query.Select(expr.TsRank(validColumn, expr.ToTsquery("sensitive-value")).As(unsafe)).From(validTable), untrusted: unsafe},
		{name: "arithmetic_alias", builder: query.Select(validIntColumn.Add(1).As(unsafe)).From(validTable), untrusted: unsafe},
		{name: "case_alias", builder: query.Select(expr.Case().When(expr.Raw("TRUE"), expr.Lit("sensitive-value")).As(unsafe)).From(validTable), untrusted: unsafe},
		{name: "simple_case_alias", builder: query.Select(expr.SimpleCase(validColumn).WhenVal("sensitive-value", expr.Lit("matched")).As(unsafe)).From(validTable), untrusted: unsafe},
		{name: "text_search_alias", builder: query.Select(expr.ToTsvector(validColumn).As(unsafe)).From(validTable), untrusted: unsafe},
		{name: "distinct_on", builder: query.Select(validColumn).DistinctOn(unsafeNameColumn).From(validTable), untrusted: unsafe},
		{name: "group_by", builder: query.Select(validColumn).From(validTable).GroupBy(unsafeNameColumn), untrusted: unsafe},
		{name: "order_by", builder: query.Select(validColumn).From(validTable).OrderBy(unsafeNameColumn.Asc()), untrusted: unsafe},
		{name: "insert_source_alias", builder: query.InsertInto(unsafeAliasTable).Values(validRow), untrusted: unsafe},
		{name: "insert_column", builder: query.InsertInto(validTable).Values(dottedIdentifierRow{}), untrusted: "unsafe.identifier"},
		{name: "conflict_column", builder: query.InsertInto(validTable).Values(validRow).OnConflict(unsafe).DoNothing(), untrusted: unsafe},
		{name: "conflict_constraint", builder: query.InsertInto(validTable).Values(validRow).OnConflictConstraint(unsafe).DoNothing(), untrusted: unsafe},
		{name: "upsert_set", builder: query.InsertInto(validTable).Values(validRow).OnConflict("name").DoUpdateSet(unsafe, "sensitive-value"), untrusted: unsafe},
		{name: "upsert_excluded", builder: query.InsertInto(validTable).Values(validRow).OnConflict("name").DoUpdateSetExcluded(unsafe), untrusted: unsafe},
		{name: "insert_returning", builder: query.InsertInto(validTable).Values(validRow).Returning(unsafeNameColumn), untrusted: unsafe},
		{name: "update_source_alias", builder: query.Update(unsafeAliasTable).Set("name", "sensitive-value"), untrusted: unsafe},
		{name: "update_set", builder: query.Update(validTable).Set(unsafe, "sensitive-value"), untrusted: unsafe},
		{name: "update_struct", builder: query.Update(validTable).SetStruct(dottedIdentifierRow{}), untrusted: "unsafe.identifier"},
		{name: "update_returning", builder: query.Update(validTable).Set("name", "sensitive-value").Returning(unsafeNameColumn), untrusted: unsafe},
		{name: "delete_source_alias", builder: query.DeleteFrom(unsafeAliasTable), untrusted: unsafe},
		{name: "delete_returning", builder: query.DeleteFrom(validTable).Returning(unsafeNameColumn), untrusted: unsafe},
		{name: "set_operation_order", builder: query.Select(validColumn).From(validTable).
			Union(query.Select(validColumn).From(validTable)).OrderBy(unsafeNameColumn.Asc()), untrusted: unsafe},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assertInvalidIdentifierBuild(t, test.builder, dialect.Postgres, test.untrusted)
		})
	}
}

func TestBuild_ValidEmbeddedIdentifierQuotesAreEscapedByDialect(t *testing.T) {
	tests := []struct {
		name   string
		d      dialect.Dialect
		table  string
		alias  string
		column string
		want   string
	}{
		{name: "postgres", d: dialect.Postgres, table: `ta"ble`, alias: `al"ias`, column: `co"l`, want: `SELECT "al""ias"."co""l" FROM "ta""ble" AS "al""ias"`},
		{name: "sqlite", d: dialect.SQLite, table: `ta"ble`, alias: `al"ias`, column: `co"l`, want: `SELECT "al""ias"."co""l" FROM "ta""ble" AS "al""ias"`},
		{name: "mysql", d: dialect.MySQL, table: "ta`ble", alias: "al`ias", column: "co`l", want: "SELECT `al``ias`.`co``l` FROM `ta``ble` AS `al``ias`"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			table := identifierTable{name: test.table, alias: test.alias}
			column := expr.StringColumn{ColBase: expr.ColBase{TableAlias: test.alias, ColName: test.column}}
			sql, args, err := query.Select(column).From(table).Build(test.d)
			if err != nil {
				t.Fatal(err)
			}
			if sql != test.want || args != nil {
				t.Fatalf("Build() = (%q, %v), want (%q, nil)", sql, args, test.want)
			}
		})
	}
}

func assertInvalidIdentifierBuild(t *testing.T, builder query.Builder, d dialect.Dialect, untrusted string) {
	t.Helper()
	sql, args, err := builder.Build(d)
	if sql != "" || args != nil {
		t.Fatalf("Build() = (%q, %v, %v), want empty SQL and nil args", sql, args, err)
	}
	if !errors.Is(err, query.ErrInvalidIdentifier) {
		t.Fatalf("error = %v, want ErrInvalidIdentifier", err)
	}
	rendered := err.Error()
	if untrusted != "" && strings.Contains(rendered, untrusted) {
		t.Fatalf("error leaked untrusted identifier %q: %q", untrusted, err)
	}
	if strings.ContainsAny(rendered, "\x00\x01\n\r\x7f\u0085\x1b") {
		t.Fatalf("error contains unsafe control characters: %q", err)
	}
}
