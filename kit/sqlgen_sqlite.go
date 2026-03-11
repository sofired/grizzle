package kit

import (
	"fmt"
	"strings"

	pg "github.com/sofired/grizzle/schema/pg"
)

// -------------------------------------------------------------------
// SQLite DDL generation
// -------------------------------------------------------------------
//
// SQLite notes:
//   - Type affinity: any type name is accepted; SQLite maps to one of five
//     affinities. We use canonical names (uuid, boolean, timestamptz, …) so
//     that introspected schemas round-trip cleanly through the diff engine.
//   - serial/bigserial → INTEGER (required for rowid-alias autoincrement).
//   - ALTER TABLE is limited: only ADD COLUMN and DROP COLUMN (3.35+) are
//     supported. Type/nullability/default changes require a table rebuild —
//     those change kinds emit a SQL comment explaining the limitation.
//   - Partial indexes (WHERE clause) are supported since SQLite 3.8.9.
//   - RETURNING is supported since SQLite 3.35.
//   - Identifiers are quoted with double-quotes.

// GenerateCreateSQLSQLite returns CREATE TABLE + CREATE INDEX statements for
// SQLite. Unlike the PostgreSQL version, schemas are ignored (SQLite has no
// schema namespace; ATTACH DATABASE is the SQLite equivalent).
func GenerateCreateSQLSQLite(tables ...*pg.TableDef) string {
	var stmts []string
	for _, t := range tables {
		stmts = append(stmts, createTableSQLSQLite(t))
		for _, c := range t.Constraints {
			if sql := indexSQLSQLite(t.Name, c); sql != "" {
				stmts = append(stmts, sql)
			}
		}
	}
	return strings.Join(stmts, ";\n\n") + ";"
}

// GenerateChangeSQLSQLite translates a single Change into SQLite SQL statements.
func GenerateChangeSQLSQLite(snap Snapshot, c Change) []string {
	switch c.Kind {
	case ChangeCreateTable:
		t := snap.Tables[c.TableName]
		if t == nil {
			return nil
		}
		td := &pg.TableDef{Name: t.Name, Columns: t.Columns, Constraints: t.Constraints}
		stmts := []string{createTableSQLSQLite(td)}
		for _, con := range t.Constraints {
			if sql := indexSQLSQLite(c.TableName, con); sql != "" {
				stmts = append(stmts, sql)
			}
		}
		return stmts

	case ChangeDropTable:
		return []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", qiSQLite(c.TableName))}

	case ChangeAddColumn:
		if c.NewCol == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN %s",
			qiSQLite(c.TableName),
			columnDefSQLSQLite(*c.NewCol),
		)}

	case ChangeDropColumn:
		if c.OldCol == nil {
			return nil
		}
		// Supported since SQLite 3.35.0.
		return []string{fmt.Sprintf(
			"ALTER TABLE %s DROP COLUMN %s",
			qiSQLite(c.TableName),
			qiSQLite(c.OldCol.Name),
		)}

	case ChangeRenameColumn:
		if c.OldCol == nil || c.NewCol == nil {
			return nil
		}
		// Supported since SQLite 3.25.0.
		return []string{fmt.Sprintf(
			"ALTER TABLE %s RENAME COLUMN %s TO %s",
			qiSQLite(c.TableName),
			qiSQLite(c.OldCol.Name),
			qiSQLite(c.NewCol.Name),
		)}

	case ChangeAlterColumnType, ChangeAlterColumnNull, ChangeAlterColumnDefault:
		// SQLite does not support ALTER COLUMN. A full table rebuild is required.
		// Return a comment so callers can see what needs manual intervention.
		col := ""
		if c.NewCol != nil {
			col = c.NewCol.Name
		} else if c.OldCol != nil {
			col = c.OldCol.Name
		}
		return []string{fmt.Sprintf(
			"-- SQLite does not support ALTER COLUMN: manual table rebuild required for %s.%s (%s)",
			c.TableName, col, string(c.Kind),
		)}

	case ChangeAddConstraint:
		if c.Constraint == nil {
			return nil
		}
		return addConstraintSQLSQLite(c.TableName, *c.Constraint)

	case ChangeDropConstraint:
		if c.Constraint == nil {
			return nil
		}
		return dropConstraintSQLSQLite(c.TableName, *c.Constraint)
	}
	return nil
}

// AllChangeSQLSQLite returns all SQLite SQL statements for all changes in order.
// Includes SQL comment stubs for changes that SQLite cannot apply directly
// (ALTER COLUMN type/nullability/untranslatable-default changes).
func AllChangeSQLSQLite(snap Snapshot, changes []Change) []string {
	var stmts []string
	for _, c := range changes {
		stmts = append(stmts, GenerateChangeSQLSQLite(snap, c)...)
	}
	return stmts
}

// SQLiteApplyableChanges filters a change list to only those that SQLite can
// actually execute. Changes that require a full table rebuild (ALTER COLUMN type
// or nullability) and default changes where the translated expression is empty
// (e.g. gen_random_uuid()) are removed.
//
// Use this when deciding whether a migration has real work to do, so that
// untranslatable schema differences do not cause infinite spurious migrations.
func SQLiteApplyableChanges(changes []Change) []Change {
	out := changes[:0:len(changes)]
	for _, c := range changes {
		switch c.Kind {
		case ChangeAlterColumnType, ChangeAlterColumnNull:
			// Requires table rebuild — not auto-applicable.
			continue
		case ChangeAlterColumnDefault:
			if c.NewCol != nil && sqliteDefaultExpr(c.NewCol.DefaultExpr) == "" {
				continue // translated default is empty — no-op in SQLite
			}
			if c.NewCol != nil && !c.NewCol.HasDefault {
				continue // dropping a default is also unsupported
			}
		}
		out = append(out, c)
	}
	return out
}

// -------------------------------------------------------------------
// SQLite internal helpers
// -------------------------------------------------------------------

func createTableSQLSQLite(t *pg.TableDef) string {
	var sb strings.Builder
	sb.WriteString("CREATE TABLE IF NOT EXISTS ")
	sb.WriteString(qiSQLite(t.Name)) // SQLite has no schema namespace
	sb.WriteString(" (\n")

	parts := make([]string, 0, len(t.Columns)+len(t.Constraints))

	for _, col := range t.Columns {
		parts = append(parts, "  "+columnDefSQLSQLite(col))
	}

	for _, c := range t.Constraints {
		switch c.Kind {
		case pg.KindCheck:
			parts = append(parts, fmt.Sprintf("  CONSTRAINT %s CHECK (%s)", qiSQLite(c.Name), c.CheckExpr))
		case pg.KindPrimaryKey:
			cols := quoteColListSQLite(c.Columns)
			parts = append(parts, fmt.Sprintf("  PRIMARY KEY (%s)", cols))
		case pg.KindUnique:
			cols := quoteColListSQLite(c.Columns)
			if c.Name != "" {
				parts = append(parts, fmt.Sprintf("  CONSTRAINT %s UNIQUE (%s)", qiSQLite(c.Name), cols))
			} else {
				parts = append(parts, fmt.Sprintf("  UNIQUE (%s)", cols))
			}
		case pg.KindForeignKey:
			fkCols := quoteColListSQLite(c.Columns)
			refCols := quoteColListSQLite(c.FKColumns)
			fk := fmt.Sprintf("  FOREIGN KEY (%s) REFERENCES %s (%s)",
				fkCols, qiSQLite(c.FKTable), refCols)
			if c.FKOnDelete != "" && c.FKOnDelete != pg.FKActionNoAction {
				fk += " ON DELETE " + string(c.FKOnDelete)
			}
			if c.FKOnUpdate != "" && c.FKOnUpdate != pg.FKActionNoAction {
				fk += " ON UPDATE " + string(c.FKOnUpdate)
			}
			parts = append(parts, fk)
		}
	}

	sb.WriteString(strings.Join(parts, ",\n"))
	sb.WriteString("\n)")
	return sb.String()
}

func columnDefSQLSQLite(col pg.ColumnDef) string {
	var sb strings.Builder
	sb.WriteString(qiSQLite(col.Name))
	sb.WriteString(" ")
	sb.WriteString(sqliteType(col.SQLType, col.PrimaryKey))

	if col.NotNull && !col.PrimaryKey {
		sb.WriteString(" NOT NULL")
	}
	if col.HasDefault && col.DefaultExpr != "" && !isSerialType(col.SQLType) {
		if translated := sqliteDefaultExpr(col.DefaultExpr); translated != "" {
			sb.WriteString(" DEFAULT ")
			sb.WriteString(translated)
		}
	}
	if col.PrimaryKey && !isSerialType(col.SQLType) {
		sb.WriteString(" PRIMARY KEY")
	}
	if col.Unique && !col.PrimaryKey {
		sb.WriteString(" UNIQUE")
	}
	if col.References != nil {
		ref := col.References
		fmt.Fprintf(&sb, " REFERENCES %s (%s)", qiSQLite(ref.Table), qiSQLite(ref.Column))
		if ref.OnDelete != "" && ref.OnDelete != pg.FKActionNoAction {
			sb.WriteString(" ON DELETE " + string(ref.OnDelete))
		}
		if ref.OnUpdate != "" && ref.OnUpdate != pg.FKActionNoAction {
			sb.WriteString(" ON UPDATE " + string(ref.OnUpdate))
		}
	}
	return sb.String()
}

func indexSQLSQLite(tableName string, c pg.Constraint) string {
	if c.Kind != pg.KindIndex && c.Kind != pg.KindUniqueIndex {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("CREATE ")
	if c.Kind == pg.KindUniqueIndex {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX IF NOT EXISTS ")
	if c.Name != "" {
		sb.WriteString(qiSQLite(c.Name) + " ")
	}
	sb.WriteString("ON ")
	sb.WriteString(qiSQLite(tableName))
	sb.WriteString(" (")
	sb.WriteString(quoteColListSQLite(c.Columns))
	sb.WriteString(")")
	if c.WhereExpr != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(c.WhereExpr)
	}
	return sb.String()
}

func addConstraintSQLSQLite(tableName string, c pg.Constraint) []string {
	switch c.Kind {
	case pg.KindIndex, pg.KindUniqueIndex:
		return []string{indexSQLSQLite(tableName, c)}
	default:
		// SQLite does not support ALTER TABLE ADD CONSTRAINT for checks/FKs.
		return []string{fmt.Sprintf(
			"-- SQLite does not support ADD CONSTRAINT for %s on %s — rebuild table manually",
			c.Name, tableName,
		)}
	}
}

func dropConstraintSQLSQLite(tableName string, c pg.Constraint) []string {
	switch c.Kind {
	case pg.KindIndex, pg.KindUniqueIndex:
		return []string{fmt.Sprintf("DROP INDEX IF EXISTS %s", qiSQLite(c.Name))}
	default:
		return []string{fmt.Sprintf(
			"-- SQLite does not support DROP CONSTRAINT for %s on %s — rebuild table manually",
			c.Name, tableName,
		)}
	}
}

// sqliteType maps canonical SQL type strings to SQLite type names.
// SQLite's type affinity means arbitrary names work, but we translate the
// common serial types which need "INTEGER" for rowid-alias behaviour.
// isPK is reserved for future PK-specific type handling (e.g. AUTOINCREMENT).
func sqliteType(sqlType string, isPK bool) string { //nolint:unparam
	lower := strings.ToLower(sqlType)
	// serial/bigserial → INTEGER PRIMARY KEY (handled inline in columnDefSQLSQLite)
	if lower == "serial" || lower == "bigserial" {
		return "INTEGER PRIMARY KEY AUTOINCREMENT"
	}
	// Pass all other canonical types through — SQLite accepts them via affinity.
	return sqlType
}

// isSerialType returns true for types that embed PRIMARY KEY in their type name.
func isSerialType(sqlType string) bool {
	lower := strings.ToLower(sqlType)
	return lower == "serial" || lower == "bigserial"
}

// sqliteDefaultExpr translates a PostgreSQL/canonical default expression to
// SQLite syntax.
func sqliteDefaultExpr(pgDefault string) string {
	switch strings.TrimSpace(pgDefault) {
	case "now()", "current_timestamp", "CURRENT_TIMESTAMP":
		return "CURRENT_TIMESTAMP"
	case "gen_random_uuid()", "uuid_generate_v4()":
		// SQLite has no built-in UUID generator; omit the default.
		// Applications are expected to supply UUID values.
		return ""
	case "true":
		return "1"
	case "false":
		return "0"
	default:
		// Strip PostgreSQL-specific casts (::jsonb, ::json, ::text).
		d := strings.TrimSpace(pgDefault)
		if idx := strings.Index(d, "::"); idx >= 0 {
			d = d[:idx]
		}
		return d
	}
}

// qiSQLite quotes a single SQLite identifier with double-quotes.
func qiSQLite(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// quoteColListSQLite returns a comma-separated list of double-quoted column names.
func quoteColListSQLite(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = qiSQLite(c)
	}
	return strings.Join(quoted, ", ")
}
