// Package introspect reads a live PostgreSQL database and returns a kit.Snapshot
// representing its current schema. This is used by kit.Push to compute diffs.
package introspect

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	pg "github.com/sofired/grizzle/schema/pg"
)

// LiveSnapshot holds a read-back of the live DB schema.
// Returned by IntrospectPostgres so it can be diffed against the target.
type LiveSnapshot struct {
	Tables map[string]*LiveTable // keyed by qualified name
	Views  map[string]*LiveView  // keyed by qualified name
	Enums  map[string]*LiveEnum  // keyed by qualified name
}

// LiveTable mirrors kit.TableSnap but is built from information_schema queries.
type LiveTable struct {
	Name        string
	Schema      string
	Columns     []pg.ColumnDef
	Constraints []pg.Constraint
}

func (t *LiveTable) QualifiedName() string {
	if t.Schema != "" && t.Schema != "public" {
		return t.Schema + "." + t.Name
	}
	return t.Name
}

// LiveView mirrors kit.ViewSnap but is built from information_schema queries.
type LiveView struct {
	Name   string
	Schema string
	SQL    string
}

func (v *LiveView) QualifiedName() string {
	if v.Schema != "" && v.Schema != "public" {
		return v.Schema + "." + v.Name
	}
	return v.Name
}

// LiveEnum mirrors kit.EnumSnap but is built from pg_catalog queries.
type LiveEnum struct {
	Name   string
	Schema string
	Values []string
}

func (e *LiveEnum) QualifiedName() string {
	if e.Schema != "" && e.Schema != "public" {
		return e.Schema + "." + e.Name
	}
	return e.Name
}

// IntrospectPostgres queries information_schema and pg_catalog to build a
// live picture of the database schema, restricted to the given schema names.
// Pass no schemas to default to {"public"}.
func IntrospectPostgres(ctx context.Context, pool *pgxpool.Pool, schemas ...string) (LiveSnapshot, error) {
	if len(schemas) == 0 {
		schemas = []string{"public"}
	}

	live := LiveSnapshot{
		Tables: make(map[string]*LiveTable),
		Views:  make(map[string]*LiveView),
		Enums:  make(map[string]*LiveEnum),
	}

	// --- 1. Enumerate tables ---
	tables, err := queryTables(ctx, pool, schemas)
	if err != nil {
		return live, fmt.Errorf("introspect tables: %w", err)
	}
	for _, t := range tables {
		live.Tables[t.QualifiedName()] = t
	}

	// --- 2. Columns for each table ---
	for _, t := range live.Tables {
		cols, err := queryColumns(ctx, pool, t.Schema, t.Name)
		if err != nil {
			return live, fmt.Errorf("introspect columns for %s: %w", t.Name, err)
		}
		t.Columns = cols
	}

	// --- 3. Indexes (pg_indexes) ---
	for _, t := range live.Tables {
		idxs, err := queryIndexes(ctx, pool, t.Schema, t.Name)
		if err != nil {
			return live, fmt.Errorf("introspect indexes for %s: %w", t.Name, err)
		}
		t.Constraints = append(t.Constraints, idxs...)
	}

	// --- 4. Check constraints ---
	for _, t := range live.Tables {
		checks, err := queryCheckConstraints(ctx, pool, t.Schema, t.Name)
		if err != nil {
			return live, fmt.Errorf("introspect checks for %s: %w", t.Name, err)
		}
		t.Constraints = append(t.Constraints, checks...)
	}

	// --- 5. Views (information_schema.views) ---
	views, err := queryViews(ctx, pool, schemas)
	if err != nil {
		return live, fmt.Errorf("introspect views: %w", err)
	}
	for _, v := range views {
		live.Views[v.QualifiedName()] = v
	}

	// --- 6. Enum types (pg_type / pg_enum) ---
	enums, err := queryEnums(ctx, pool, schemas)
	if err != nil {
		return live, fmt.Errorf("introspect enums: %w", err)
	}
	for _, e := range enums {
		live.Enums[e.QualifiedName()] = e
	}

	return live, nil
}

func queryTables(ctx context.Context, pool *pgxpool.Pool, schemas []string) ([]*LiveTable, error) {
	placeholders := make([]string, len(schemas))
	args := make([]any, len(schemas))
	for i, s := range schemas {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = s
	}
	q := fmt.Sprintf(`
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE'
		  AND table_schema IN (%s)
		ORDER BY table_schema, table_name`,
		strings.Join(placeholders, ", "),
	)
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []*LiveTable
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			return nil, err
		}
		tables = append(tables, &LiveTable{Name: name, Schema: schema})
	}
	return tables, rows.Err()
}

func queryColumns(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]pg.ColumnDef, error) {
	q := `
		SELECT
			column_name,
			udt_name,
			data_type,
			character_maximum_length,
			numeric_precision,
			numeric_scale,
			is_nullable,
			column_default
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`

	rows, err := pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []pg.ColumnDef
	for rows.Next() {
		var (
			colName, udtName, dataType string
			charMaxLen                 *int
			numPrec, numScale          *int
			isNullable, colDefault     *string
		)
		if err := rows.Scan(&colName, &udtName, &dataType, &charMaxLen, &numPrec, &numScale, &isNullable, &colDefault); err != nil {
			return nil, err
		}

		col := pg.ColumnDef{
			Name:    colName,
			SQLType: normalizeSQLType(udtName, dataType, charMaxLen, numPrec, numScale),
			NotNull: isNullable != nil && *isNullable == "NO",
		}
		if colDefault != nil {
			col.HasDefault = true
			col.DefaultExpr = *colDefault
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

// normalizeSQLType maps information_schema type info to a canonical SQL type string.
// dataType is the information_schema data_type value, reserved as a fallback for future mappings.
func normalizeSQLType(udtName, dataType string, charMaxLen, numPrec, numScale *int) string { //nolint:unparam
	switch udtName {
	case "uuid":
		return "uuid"
	case "text":
		return "text"
	case "bool":
		return "boolean"
	case "int2":
		return "smallint"
	case "int4":
		return "integer"
	case "int8":
		return "bigint"
	case "float4":
		return "real"
	case "float8":
		return "double precision"
	case "jsonb":
		return "jsonb"
	case "json":
		return "json"
	case "timestamptz":
		return "timestamptz"
	case "timestamp":
		return "timestamp"
	case "date":
		return "date"
	case "time":
		return "time"
	case "timetz":
		return "timetz"
	case "numeric":
		if numPrec != nil && numScale != nil {
			return fmt.Sprintf("numeric(%d,%d)", *numPrec, *numScale)
		}
		return "numeric"
	case "varchar":
		if charMaxLen != nil {
			return fmt.Sprintf("varchar(%d)", *charMaxLen)
		}
		return "varchar"
	case "bpchar": // blank-padded char
		if charMaxLen != nil {
			return fmt.Sprintf("char(%d)", *charMaxLen)
		}
		return "char"
	default:
		return udtName
	}
}

func queryIndexes(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]pg.Constraint, error) {
	// pg_indexes gives us index name, whether unique, and the index def.
	// We parse column names from the index definition for simplicity.
	q := `
		SELECT indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = $1 AND tablename = $2
		ORDER BY indexname`

	rows, err := pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var constraints []pg.Constraint
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			return nil, err
		}
		// Skip primary key indexes — they're captured via column PK flag.
		if strings.HasSuffix(name, "_pkey") {
			continue
		}
		c := parseIndexDef(name, def)
		constraints = append(constraints, c)
	}
	return constraints, rows.Err()
}

// parseIndexDef extracts column names and WHERE clause from a pg_indexes indexdef.
// Example: CREATE UNIQUE INDEX users_email_idx ON public.users USING btree (email) WHERE (deleted_at IS NULL)
func parseIndexDef(name, def string) pg.Constraint {
	upper := strings.ToUpper(def)
	kind := pg.KindIndex
	if strings.Contains(upper, "UNIQUE") {
		kind = pg.KindUniqueIndex
	}

	c := pg.Constraint{Kind: kind, Name: name}

	// Extract columns: content between the first "(" and ")".
	parenOpen := strings.Index(def, "(")
	parenClose := strings.Index(def, ")")
	if parenOpen >= 0 && parenClose > parenOpen {
		colStr := def[parenOpen+1 : parenClose]
		for _, col := range strings.Split(colStr, ",") {
			col = strings.TrimSpace(col)
			col = strings.Trim(col, `"`)
			if col != "" {
				c.Columns = append(c.Columns, col)
			}
		}
	}

	// Extract WHERE clause if present.
	if whereIdx := strings.Index(upper, " WHERE "); whereIdx >= 0 {
		c.WhereExpr = strings.TrimSpace(def[whereIdx+7:])
		// Strip outer parens added by Postgres: "(deleted_at IS NULL)" → "deleted_at IS NULL"
		c.WhereExpr = strings.TrimPrefix(c.WhereExpr, "(")
		c.WhereExpr = strings.TrimSuffix(c.WhereExpr, ")")
	}

	return c
}

func queryViews(ctx context.Context, pool *pgxpool.Pool, schemas []string) ([]*LiveView, error) {
	placeholders := make([]string, len(schemas))
	args := make([]any, len(schemas))
	for i, s := range schemas {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = s
	}
	// Note: information_schema.views.view_definition returns the SQL as
	// PostgreSQL has normalised/rewritten it (e.g. fully-qualified names,
	// expression reformatting). This means the stored definition may not
	// match the original CREATE VIEW statement verbatim. Diffs against a
	// live database may produce spurious drop+recreate changes unless the
	// user stores the normalised form. This is a known limitation shared
	// by most migration toolkits.
	q := fmt.Sprintf(`
		SELECT table_schema, table_name, view_definition
		FROM information_schema.views
		WHERE table_schema IN (%s)
		ORDER BY table_schema, table_name`,
		strings.Join(placeholders, ", "),
	)
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []*LiveView
	for rows.Next() {
		var schema, name string
		var def *string
		if err := rows.Scan(&schema, &name, &def); err != nil {
			return nil, err
		}
		sql := ""
		if def != nil {
			sql = *def
		}
		views = append(views, &LiveView{Name: name, Schema: schema, SQL: sql})
	}
	return views, rows.Err()
}

func queryEnums(ctx context.Context, pool *pgxpool.Pool, schemas []string) ([]*LiveEnum, error) {
	placeholders := make([]string, len(schemas))
	args := make([]any, len(schemas))
	for i, s := range schemas {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = s
	}
	// Query enum types from pg_type joined with pg_namespace and pg_enum.
	// pg_enum.enumsortorder is used to preserve value ordering.
	q := fmt.Sprintf(`
		SELECT n.nspname AS schema, t.typname AS name,
		       array_agg(e.enumlabel ORDER BY e.enumsortorder) AS values
		FROM pg_type t
		JOIN pg_namespace n ON n.oid = t.typnamespace
		JOIN pg_enum e ON e.enumtypid = t.oid
		WHERE t.typtype = 'e'
		  AND n.nspname IN (%s)
		GROUP BY n.nspname, t.typname
		ORDER BY n.nspname, t.typname`,
		strings.Join(placeholders, ", "),
	)
	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var enums []*LiveEnum
	for rows.Next() {
		var schema, name string
		var values []string
		if err := rows.Scan(&schema, &name, &values); err != nil {
			return nil, err
		}
		enums = append(enums, &LiveEnum{Name: name, Schema: schema, Values: values})
	}
	return enums, rows.Err()
}

func queryCheckConstraints(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]pg.Constraint, error) {
	q := `
		SELECT cc.constraint_name, cc.check_clause
		FROM information_schema.check_constraints cc
		JOIN information_schema.table_constraints tc
		  ON tc.constraint_name = cc.constraint_name
		 AND tc.constraint_schema = cc.constraint_schema
		WHERE tc.table_schema = $1
		  AND tc.table_name = $2
		  AND tc.constraint_type = 'CHECK'
		ORDER BY cc.constraint_name`

	rows, err := pool.Query(ctx, q, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var constraints []pg.Constraint
	for rows.Next() {
		var name, expr string
		if err := rows.Scan(&name, &expr); err != nil {
			return nil, err
		}
		// Postgres wraps check clauses in parens; strip the outer ones.
		expr = strings.TrimPrefix(expr, "(")
		expr = strings.TrimSuffix(expr, ")")
		constraints = append(constraints, pg.Constraint{
			Kind:      pg.KindCheck,
			Name:      name,
			CheckExpr: expr,
		})
	}
	return constraints, rows.Err()
}
