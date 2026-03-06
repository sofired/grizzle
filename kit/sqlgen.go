package kit

import (
	"fmt"
	"strings"

	pg "github.com/grizzle-orm/g-rizzle/schema/pg"
)

// GenerateCreateSQL returns the full SQL to create all given tables from scratch,
// including separate CREATE INDEX statements. Statements are separated by ";\n".
func GenerateCreateSQL(tables ...*pg.TableDef) string {
	var stmts []string
	for _, t := range tables {
		stmts = append(stmts, createTableSQL(t))
		for _, c := range t.Constraints {
			if sql := indexSQL(t.QualifiedName(), c); sql != "" {
				stmts = append(stmts, sql)
			}
		}
	}
	return strings.Join(stmts, ";\n\n") + ";"
}

// GenerateChangeSQL translates a single Change into one or more SQL statements.
// The caller is responsible for ordering (Diff() already returns changes in
// a safe application order for common cases).
func GenerateChangeSQL(snap Snapshot, c Change) []string {
	switch c.Kind {
	case ChangeCreateTable:
		t := snap.Tables[c.TableName]
		if t == nil {
			return nil
		}
		td := &pg.TableDef{Name: t.Name, Schema: t.Schema, Columns: t.Columns, Constraints: t.Constraints}
		stmts := []string{createTableSQL(td)}
		for _, con := range t.Constraints {
			if sql := indexSQL(c.TableName, con); sql != "" {
				stmts = append(stmts, sql)
			}
		}
		return stmts

	case ChangeDropTable:
		return []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteTable(c.TableName))}

	case ChangeAddColumn:
		if c.NewCol == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN %s",
			quoteTable(c.TableName),
			columnDefSQL(*c.NewCol),
		)}

	case ChangeDropColumn:
		if c.OldCol == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s DROP COLUMN %s",
			quoteTable(c.TableName),
			qi(c.OldCol.Name),
		)}

	case ChangeAlterColumnType:
		if c.NewCol == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s TYPE %s",
			quoteTable(c.TableName),
			qi(c.NewCol.Name),
			c.NewCol.SQLType,
		)}

	case ChangeAlterColumnNull:
		if c.NewCol == nil {
			return nil
		}
		if c.NewCol.NotNull {
			return []string{fmt.Sprintf(
				"ALTER TABLE %s ALTER COLUMN %s SET NOT NULL",
				quoteTable(c.TableName), qi(c.NewCol.Name),
			)}
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL",
			quoteTable(c.TableName), qi(c.NewCol.Name),
		)}

	case ChangeAlterColumnDefault:
		if c.NewCol == nil {
			return nil
		}
		if !c.NewCol.HasDefault {
			return []string{fmt.Sprintf(
				"ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT",
				quoteTable(c.TableName), qi(c.NewCol.Name),
			)}
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s",
			quoteTable(c.TableName), qi(c.NewCol.Name), c.NewCol.DefaultExpr,
		)}

	case ChangeAddConstraint:
		if c.Constraint == nil {
			return nil
		}
		return addConstraintSQL(c.TableName, *c.Constraint)

	case ChangeDropConstraint:
		if c.Constraint == nil {
			return nil
		}
		return dropConstraintSQL(c.TableName, *c.Constraint)
	}
	return nil
}

// AllChangeSQL returns all SQL statements for all changes in order.
func AllChangeSQL(snap Snapshot, changes []Change) []string {
	var stmts []string
	for _, c := range changes {
		stmts = append(stmts, GenerateChangeSQL(snap, c)...)
	}
	return stmts
}

// -------------------------------------------------------------------
// Internal helpers
// -------------------------------------------------------------------

// createTableSQL generates a CREATE TABLE IF NOT EXISTS statement.
// Inline FKs, PKs, CHECKs, and composite UNIQUE constraints are included.
// Indexes are NOT included — emit those separately via indexSQL().
func createTableSQL(t *pg.TableDef) string {
	var sb strings.Builder
	sb.WriteString("CREATE TABLE IF NOT EXISTS ")
	sb.WriteString(quoteTable(t.QualifiedName()))
	sb.WriteString(" (\n")

	parts := make([]string, 0, len(t.Columns)+len(t.Constraints))

	// Columns.
	for _, col := range t.Columns {
		parts = append(parts, "  "+columnDefSQL(col))
	}

	// Table-level inline constraints (not indexes — those are separate).
	for _, c := range t.Constraints {
		switch c.Kind {
		case pg.KindCheck:
			parts = append(parts, fmt.Sprintf("  CONSTRAINT %s CHECK (%s)", qi(c.Name), c.CheckExpr))
		case pg.KindPrimaryKey:
			cols := quoteColList(c.Columns)
			parts = append(parts, fmt.Sprintf("  PRIMARY KEY (%s)", cols))
		case pg.KindUnique:
			cols := quoteColList(c.Columns)
			if c.Name != "" {
				parts = append(parts, fmt.Sprintf("  CONSTRAINT %s UNIQUE (%s)", qi(c.Name), cols))
			} else {
				parts = append(parts, fmt.Sprintf("  UNIQUE (%s)", cols))
			}
		case pg.KindForeignKey:
			fkCols := quoteColList(c.Columns)
			refCols := quoteColList(c.FKColumns)
			fk := fmt.Sprintf("  CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
				qi(c.Name), fkCols, qi(c.FKTable), refCols)
			if c.FKOnDelete != "" && c.FKOnDelete != pg.FKActionNoAction {
				fk += " ON DELETE " + string(c.FKOnDelete)
			}
			if c.FKOnUpdate != "" && c.FKOnUpdate != pg.FKActionNoAction {
				fk += " ON UPDATE " + string(c.FKOnUpdate)
			}
			parts = append(parts, fk)
		// KindIndex / KindUniqueIndex → separate CREATE INDEX statements.
		}
	}

	sb.WriteString(strings.Join(parts, ",\n"))
	sb.WriteString("\n)")
	return sb.String()
}

// columnDefSQL renders a single column definition for use inside CREATE TABLE
// or ALTER TABLE ADD COLUMN.
func columnDefSQL(col pg.ColumnDef) string {
	var sb strings.Builder
	sb.WriteString(qi(col.Name))
	sb.WriteString(" ")
	sb.WriteString(col.SQLType)

	if col.NotNull {
		sb.WriteString(" NOT NULL")
	}
	if col.HasDefault && col.DefaultExpr != "" {
		sb.WriteString(" DEFAULT ")
		sb.WriteString(col.DefaultExpr)
	}
	if col.PrimaryKey {
		sb.WriteString(" PRIMARY KEY")
	}
	if col.Unique && !col.PrimaryKey {
		sb.WriteString(" UNIQUE")
	}
	if col.References != nil {
		ref := col.References
		sb.WriteString(fmt.Sprintf(" REFERENCES %s (%s)", qi(ref.Table), qi(ref.Column)))
		if ref.OnDelete != "" && ref.OnDelete != pg.FKActionNoAction {
			sb.WriteString(" ON DELETE " + string(ref.OnDelete))
		}
		if ref.OnUpdate != "" && ref.OnUpdate != pg.FKActionNoAction {
			sb.WriteString(" ON UPDATE " + string(ref.OnUpdate))
		}
	}
	return sb.String()
}

// indexSQL returns the CREATE [UNIQUE] INDEX statement for KindIndex / KindUniqueIndex.
// Returns "" for other constraint kinds.
func indexSQL(tableName string, c pg.Constraint) string {
	if c.Kind != pg.KindIndex && c.Kind != pg.KindUniqueIndex {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("CREATE ")
	if c.Kind == pg.KindUniqueIndex {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX ")
	if c.Name != "" {
		sb.WriteString(qi(c.Name) + " ")
	}
	sb.WriteString("ON ")
	sb.WriteString(quoteTable(tableName))
	sb.WriteString(" (")
	sb.WriteString(quoteColList(c.Columns))
	sb.WriteString(")")
	if c.WhereExpr != "" {
		sb.WriteString(" WHERE " + c.WhereExpr)
	}
	return sb.String()
}

// addConstraintSQL generates SQL to add a new constraint to an existing table.
func addConstraintSQL(tableName string, c pg.Constraint) []string {
	switch c.Kind {
	case pg.KindIndex, pg.KindUniqueIndex:
		return []string{indexSQL(tableName, c)}
	case pg.KindCheck:
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s)",
			quoteTable(tableName), qi(c.Name), c.CheckExpr,
		)}
	case pg.KindUnique:
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s)",
			quoteTable(tableName), qi(c.Name), quoteColList(c.Columns),
		)}
	case pg.KindForeignKey:
		sql := fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			quoteTable(tableName), qi(c.Name),
			quoteColList(c.Columns), qi(c.FKTable), quoteColList(c.FKColumns),
		)
		if c.FKOnDelete != "" && c.FKOnDelete != pg.FKActionNoAction {
			sql += " ON DELETE " + string(c.FKOnDelete)
		}
		return []string{sql}
	}
	return nil
}

// dropConstraintSQL generates SQL to remove a constraint from a table.
func dropConstraintSQL(tableName string, c pg.Constraint) []string {
	switch c.Kind {
	case pg.KindIndex, pg.KindUniqueIndex:
		// Indexes are dropped with DROP INDEX, not ALTER TABLE.
		return []string{fmt.Sprintf("DROP INDEX IF EXISTS %s", qi(c.Name))}
	default:
		return []string{fmt.Sprintf(
			"ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s",
			quoteTable(tableName), qi(c.Name),
		)}
	}
}

// qi quotes a single PostgreSQL identifier: table or column name.
func qi(name string) string {
	// If the name contains a dot (schema.table), quote each part separately.
	// For simple names, just wrap in double quotes.
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// quoteTable quotes a potentially schema-qualified table name.
// "users" → `"users"`, "public.users" → `"public"."users"`
func quoteTable(name string) string {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return qi(parts[0]) + "." + qi(parts[1])
	}
	return qi(name)
}

// quoteColList returns a comma-separated list of quoted column names.
func quoteColList(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = qi(c)
	}
	return strings.Join(quoted, ", ")
}
