package kit

import (
	"fmt"
	"strings"

	pg "github.com/sofired/grizzle/schema/pg"
)

// GenerateCreateSQL returns the full SQL to create all given tables from scratch,
// including separate CREATE INDEX statements. Statements are separated by ";\n".
// It accepts tables from any dialect via the TableDefiner interface.
func GenerateCreateSQL(tables ...pg.TableDefiner) string {
	var stmts []string
	for _, td := range tables {
		t := td.Def()
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
		t := snap.Tables[c.ObjectName]
		if t == nil {
			return nil
		}
		td := &pg.TableDef{Name: t.Name, Schema: t.Schema, Columns: t.Columns, Constraints: t.Constraints}
		stmts := []string{createTableSQL(td)}
		for _, con := range t.Constraints {
			if sql := indexSQL(c.ObjectName, con); sql != "" {
				stmts = append(stmts, sql)
			}
		}
		return stmts

	case ChangeDropTable:
		return []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", quoteTable(c.ObjectName))}

	case ChangeRenameTable:
		if c.RenameTarget == "" {
			return nil
		}
		// Normalize "" and "public" as equivalent: an unqualified PostgreSQL name
		// resolves to the public schema under the default search_path, so treating
		// them as the same avoids spurious cross-schema paths that would emit
		// SET SCHEMA "" (invalid SQL) when one side is qualified and the other is not.
		srcSchema := pgNormalizeSchema(schemaOf(c.ObjectName))
		dstSchema := pgNormalizeSchema(schemaOf(c.RenameTarget))
		newUnqualified := unqualifiedName(c.RenameTarget)
		if srcSchema != dstSchema {
			// Cross-schema: rename within source schema first, then move.
			// PostgreSQL requires two steps because RENAME TO cannot change the schema.
			intermediate := srcSchema + "." + newUnqualified
			return []string{
				fmt.Sprintf("ALTER TABLE %s RENAME TO %s",
					quoteTable(c.ObjectName), qi(newUnqualified)),
				fmt.Sprintf("ALTER TABLE %s SET SCHEMA %s",
					quoteTable(intermediate), qi(dstSchema)),
			}
		}
		// PostgreSQL RENAME TO accepts only the unqualified new name within the
		// same schema; a schema-qualified target like "public"."users" is invalid.
		return []string{fmt.Sprintf(
			"ALTER TABLE %s RENAME TO %s",
			quoteTable(c.ObjectName),
			quoteUnqualifiedTable(c.RenameTarget),
		)}

	case ChangeAddColumn:
		if c.NewCol == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN %s",
			quoteTable(c.ObjectName),
			columnDefSQL(*c.NewCol),
		)}

	case ChangeDropColumn:
		if c.OldCol == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s DROP COLUMN %s",
			quoteTable(c.ObjectName),
			qi(c.OldCol.Name),
		)}

	case ChangeAlterColumnType:
		if c.NewCol == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s TYPE %s",
			quoteTable(c.ObjectName),
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
				quoteTable(c.ObjectName), qi(c.NewCol.Name),
			)}
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL",
			quoteTable(c.ObjectName), qi(c.NewCol.Name),
		)}

	case ChangeAlterColumnDefault:
		if c.NewCol == nil {
			return nil
		}
		if !c.NewCol.HasDefault {
			return []string{fmt.Sprintf(
				"ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT",
				quoteTable(c.ObjectName), qi(c.NewCol.Name),
			)}
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s",
			quoteTable(c.ObjectName), qi(c.NewCol.Name), c.NewCol.DefaultExpr,
		)}

	case ChangeRenameColumn:
		if c.OldCol == nil || c.NewCol == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s RENAME COLUMN %s TO %s",
			quoteTable(c.ObjectName),
			qi(c.OldCol.Name),
			qi(c.NewCol.Name),
		)}

	case ChangeAddConstraint:
		if c.Constraint == nil {
			return nil
		}
		return addConstraintSQL(c.ObjectName, *c.Constraint)

	case ChangeDropConstraint:
		if c.Constraint == nil {
			return nil
		}
		return dropConstraintSQL(c.ObjectName, *c.Constraint)

	case ChangeCreateView:
		if c.View == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"CREATE OR REPLACE VIEW %s AS %s",
			quoteTable(c.View.QualifiedName()),
			c.View.SQL,
		)}

	case ChangeReplaceView:
		if c.View == nil {
			return nil
		}
		// CREATE OR REPLACE VIEW cannot alter column names, order, or types in PostgreSQL.
		// DROP IF EXISTS + CREATE guarantees convergence for all view changes. Without CASCADE,
		// PostgreSQL will error if dependent views exist, which surfaces the dependency clearly.
		name := quoteTable(c.View.QualifiedName())
		return []string{
			fmt.Sprintf("DROP VIEW IF EXISTS %s", name),
			fmt.Sprintf("CREATE VIEW %s AS %s", name, c.View.SQL),
		}

	case ChangeDropView:
		if c.View == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"DROP VIEW IF EXISTS %s",
			quoteTable(c.View.QualifiedName()),
		)}

	case ChangeCreateEnum:
		if c.NewEnum == nil {
			return nil
		}
		return []string{createEnumSQL(c.NewEnum)}

	case ChangeAlterEnum:
		if c.OldEnum == nil || c.NewEnum == nil {
			return nil
		}
		added := enumAddedValues(c.OldEnum, c.NewEnum)
		removedVals, reordered := enumDrift(c.OldEnum, c.NewEnum)
		if len(added) == 0 && len(removedVals) == 0 && !reordered {
			return nil
		}
		typeName := quoteTable(c.NewEnum.QualifiedName())
		stmts := make([]string, 0, len(added)+len(removedVals)+2)
		for _, v := range removedVals {
			stmts = append(stmts, fmt.Sprintf(
				"-- WARNING: enum value %q was removed from type %s; PostgreSQL cannot remove enum values — manual intervention required",
				v, typeName,
			))
		}
		if reordered {
			stmts = append(stmts, fmt.Sprintf(
				"-- WARNING: enum values for type %s have been reordered; PostgreSQL cannot reorder enum values — manual intervention required",
				typeName,
			))
		}
		if len(added) > 0 {
			stmts = append(stmts, fmt.Sprintf(
				"-- WARNING: ALTER TYPE %s ADD VALUE is not transactional in PostgreSQL < 12; run outside a transaction on PG 9.x–11.x or upgrade to PG 12+",
				typeName,
			))
		}
		for _, a := range added {
			val := strings.ReplaceAll(a.Value, "'", "''")
			if a.After != "" {
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TYPE %s ADD VALUE IF NOT EXISTS '%s' AFTER '%s'",
					typeName, val, strings.ReplaceAll(a.After, "'", "''"),
				))
			} else if a.Before != "" {
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TYPE %s ADD VALUE IF NOT EXISTS '%s' BEFORE '%s'",
					typeName, val, strings.ReplaceAll(a.Before, "'", "''"),
				))
			} else {
				stmts = append(stmts, fmt.Sprintf(
					"ALTER TYPE %s ADD VALUE IF NOT EXISTS '%s'",
					typeName, val,
				))
			}
		}
		return stmts

	case ChangeDropEnum:
		if c.OldEnum == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"DROP TYPE IF EXISTS %s",
			quoteTable(c.OldEnum.QualifiedName()),
		)}
	}
	return nil
}

// createEnumSQL generates a CREATE TYPE ... AS ENUM statement.
func createEnumSQL(e *EnumSnap) string {
	quoted := make([]string, len(e.Values))
	for i, v := range e.Values {
		quoted[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	return fmt.Sprintf(
		"CREATE TYPE %s AS ENUM (%s)",
		quoteTable(e.QualifiedName()),
		strings.Join(quoted, ", "),
	)
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
				qi(c.Name), fkCols, quoteTable(c.FKTable), refCols)
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
		fmt.Fprintf(&sb, " REFERENCES %s (%s)", quoteTable(ref.Table), qi(ref.Column))
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
			quoteColList(c.Columns), quoteTable(c.FKTable), quoteColList(c.FKColumns),
		)
		if c.FKOnDelete != "" && c.FKOnDelete != pg.FKActionNoAction {
			sql += " ON DELETE " + string(c.FKOnDelete)
		}
		if c.FKOnUpdate != "" && c.FKOnUpdate != pg.FKActionNoAction {
			sql += " ON UPDATE " + string(c.FKOnUpdate)
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

// unqualifiedName strips any schema prefix from a potentially qualified name.
// "public.users" → "users", "users" → "users".
func unqualifiedName(name string) string {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return name
}

// schemaOf returns the schema component of a potentially qualified name,
// or "" if the name has no schema prefix.
func schemaOf(name string) string {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return ""
}

// pgNormalizeSchema treats "" and "public" as equivalent for PostgreSQL
// cross-schema comparisons. An unqualified name resolves to the public schema
// under the default search_path, so the two forms are semantically identical.
func pgNormalizeSchema(schema string) string {
	if schema == "" {
		return "public"
	}
	return schema
}

// quoteUnqualifiedTable quotes only the unqualified (non-schema-prefixed) part
// of a potentially schema-qualified name. Use where PostgreSQL DDL rejects a
// schema-qualified identifier — e.g. RENAME TO.
// "public.users" → `"users"`, "users" → `"users"`.
func quoteUnqualifiedTable(name string) string {
	return qi(unqualifiedName(name))
}

// quoteColList returns a comma-separated list of quoted column names.
func quoteColList(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = qi(c)
	}
	return strings.Join(quoted, ", ")
}
