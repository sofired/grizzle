package kit

import (
	"fmt"
	"strings"

	pg "github.com/sofired/grizzle/schema/pg"
)

// -------------------------------------------------------------------
// MySQL / MariaDB DDL generation
// -------------------------------------------------------------------
//
// MySQL differs from PostgreSQL in several important ways:
//   - Identifiers use backticks instead of double-quotes
//   - UUID is stored as CHAR(36) (no native uuid type)
//   - TIMESTAMPTZ maps to DATETIME (MySQL) or DATETIME(6) for sub-second precision
//   - BOOLEAN maps to TINYINT(1)
//   - JSONB maps to JSON (MySQL 5.7.8+)
//   - BIGSERIAL maps to BIGINT AUTO_INCREMENT (handled in column def)
//   - Indexes are dropped with DROP INDEX name ON table (not DROP INDEX IF EXISTS name)
//   - ALTER COLUMN uses MODIFY COLUMN syntax
//   - Partial indexes (WHERE clause) are NOT supported in MySQL < 8.0.13
//   - gen_random_uuid() → UUID() in MySQL 8.0+ or (UUID()) in MariaDB

// GenerateCreateSQLMySQL returns CREATE TABLE statements using MySQL syntax.
func GenerateCreateSQLMySQL(tables ...*pg.TableDef) string {
	var stmts []string
	for _, t := range tables {
		stmts = append(stmts, createTableSQLMySQL(t))
		for _, c := range t.Constraints {
			if sql := indexSQLMySQL(t.QualifiedName(), c); sql != "" {
				stmts = append(stmts, sql)
			}
		}
	}
	return strings.Join(stmts, ";\n\n") + ";"
}

// GenerateChangeSQLMySQL translates a single Change into MySQL SQL statements.
func GenerateChangeSQLMySQL(snap Snapshot, c Change) []string {
	switch c.Kind {
	case ChangeCreateTable:
		t := snap.Tables[c.TableName]
		if t == nil {
			return nil
		}
		td := &pg.TableDef{Name: t.Name, Schema: t.Schema, Columns: t.Columns, Constraints: t.Constraints}
		stmts := []string{createTableSQLMySQL(td)}
		for _, con := range t.Constraints {
			if sql := indexSQLMySQL(c.TableName, con); sql != "" {
				stmts = append(stmts, sql)
			}
		}
		return stmts

	case ChangeDropTable:
		return []string{fmt.Sprintf("DROP TABLE IF EXISTS %s", qiMySQL(c.TableName))}

	case ChangeAddColumn:
		if c.NewCol == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN %s",
			qiMySQL(c.TableName),
			columnDefSQLMySQL(*c.NewCol),
		)}

	case ChangeDropColumn:
		if c.OldCol == nil {
			return nil
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s DROP COLUMN %s",
			qiMySQL(c.TableName),
			qiMySQL(c.OldCol.Name),
		)}

	case ChangeAlterColumnType:
		if c.NewCol == nil {
			return nil
		}
		// MySQL uses MODIFY COLUMN — must repeat the full column definition.
		return []string{fmt.Sprintf(
			"ALTER TABLE %s MODIFY COLUMN %s",
			qiMySQL(c.TableName),
			columnDefSQLMySQL(*c.NewCol),
		)}

	case ChangeAlterColumnNull:
		if c.NewCol == nil {
			return nil
		}
		// MODIFY COLUMN with the full definition (MySQL requires it).
		return []string{fmt.Sprintf(
			"ALTER TABLE %s MODIFY COLUMN %s",
			qiMySQL(c.TableName),
			columnDefSQLMySQL(*c.NewCol),
		)}

	case ChangeAlterColumnDefault:
		if c.NewCol == nil {
			return nil
		}
		if !c.NewCol.HasDefault {
			return []string{fmt.Sprintf(
				"ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT",
				qiMySQL(c.TableName), qiMySQL(c.NewCol.Name),
			)}
		}
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s",
			qiMySQL(c.TableName), qiMySQL(c.NewCol.Name),
			mysqlDefaultExpr(c.NewCol.DefaultExpr),
		)}

	case ChangeRenameColumn:
		if c.OldCol == nil || c.NewCol == nil {
			return nil
		}
		// MySQL 8.0+: RENAME COLUMN old TO new
		// Older MySQL requires CHANGE old new <full_def> — we use the modern syntax.
		return []string{fmt.Sprintf(
			"ALTER TABLE %s RENAME COLUMN %s TO %s",
			quoteTableMySQL(c.TableName),
			qiMySQL(c.OldCol.Name),
			qiMySQL(c.NewCol.Name),
		)}

	case ChangeAddConstraint:
		if c.Constraint == nil {
			return nil
		}
		return addConstraintSQLMySQL(c.TableName, *c.Constraint)

	case ChangeDropConstraint:
		if c.Constraint == nil {
			return nil
		}
		return dropConstraintSQLMySQL(c.TableName, *c.Constraint)
	}
	return nil
}

// AllChangeSQLMySQL returns all MySQL SQL statements for all changes in order.
func AllChangeSQLMySQL(snap Snapshot, changes []Change) []string {
	var stmts []string
	for _, c := range changes {
		stmts = append(stmts, GenerateChangeSQLMySQL(snap, c)...)
	}
	return stmts
}

// -------------------------------------------------------------------
// MySQL internal helpers
// -------------------------------------------------------------------

func createTableSQLMySQL(t *pg.TableDef) string {
	var sb strings.Builder
	sb.WriteString("CREATE TABLE IF NOT EXISTS ")
	sb.WriteString(quoteTableMySQL(t.QualifiedName()))
	sb.WriteString(" (\n")

	parts := make([]string, 0, len(t.Columns)+len(t.Constraints))

	for _, col := range t.Columns {
		parts = append(parts, "  "+columnDefSQLMySQL(col))
	}

	for _, c := range t.Constraints {
		switch c.Kind {
		case pg.KindCheck:
			// MySQL 8.0+ supports CHECK; older versions parse but ignore.
			parts = append(parts, fmt.Sprintf("  CONSTRAINT %s CHECK (%s)", qiMySQL(c.Name), c.CheckExpr))
		case pg.KindPrimaryKey:
			cols := quoteColListMySQL(c.Columns)
			parts = append(parts, fmt.Sprintf("  PRIMARY KEY (%s)", cols))
		case pg.KindUnique:
			cols := quoteColListMySQL(c.Columns)
			if c.Name != "" {
				parts = append(parts, fmt.Sprintf("  UNIQUE KEY %s (%s)", qiMySQL(c.Name), cols))
			} else {
				parts = append(parts, fmt.Sprintf("  UNIQUE (%s)", cols))
			}
		case pg.KindForeignKey:
			fkCols := quoteColListMySQL(c.Columns)
			refCols := quoteColListMySQL(c.FKColumns)
			fk := fmt.Sprintf("  CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
				qiMySQL(c.Name), fkCols, qiMySQL(c.FKTable), refCols)
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
	sb.WriteString("\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci")
	return sb.String()
}

func columnDefSQLMySQL(col pg.ColumnDef) string {
	var sb strings.Builder
	sb.WriteString(qiMySQL(col.Name))
	sb.WriteString(" ")
	sb.WriteString(mysqlType(col.SQLType))

	if col.NotNull {
		sb.WriteString(" NOT NULL")
	}
	if col.HasDefault && col.DefaultExpr != "" {
		sb.WriteString(" DEFAULT ")
		sb.WriteString(mysqlDefaultExpr(col.DefaultExpr))
	}
	if col.PrimaryKey {
		sb.WriteString(" PRIMARY KEY")
	}
	if col.Unique && !col.PrimaryKey {
		sb.WriteString(" UNIQUE")
	}
	if col.References != nil {
		ref := col.References
		fmt.Fprintf(&sb, " REFERENCES %s (%s)", qiMySQL(ref.Table), qiMySQL(ref.Column))
		if ref.OnDelete != "" && ref.OnDelete != pg.FKActionNoAction {
			sb.WriteString(" ON DELETE " + string(ref.OnDelete))
		}
		if ref.OnUpdate != "" && ref.OnUpdate != pg.FKActionNoAction {
			sb.WriteString(" ON UPDATE " + string(ref.OnUpdate))
		}
	}
	return sb.String()
}

func indexSQLMySQL(tableName string, c pg.Constraint) string {
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
		sb.WriteString(qiMySQL(c.Name) + " ")
	}
	sb.WriteString("ON ")
	sb.WriteString(quoteTableMySQL(tableName))
	sb.WriteString(" (")
	sb.WriteString(quoteColListMySQL(c.Columns))
	sb.WriteString(")")
	// MySQL 8.0.13+ supports functional index expressions but not arbitrary WHERE.
	// Partial indexes with WHERE are silently dropped for compatibility.
	return sb.String()
}

func addConstraintSQLMySQL(tableName string, c pg.Constraint) []string {
	switch c.Kind {
	case pg.KindIndex, pg.KindUniqueIndex:
		return []string{indexSQLMySQL(tableName, c)}
	case pg.KindCheck:
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s)",
			quoteTableMySQL(tableName), qiMySQL(c.Name), c.CheckExpr,
		)}
	case pg.KindUnique:
		return []string{fmt.Sprintf(
			"ALTER TABLE %s ADD UNIQUE KEY %s (%s)",
			quoteTableMySQL(tableName), qiMySQL(c.Name), quoteColListMySQL(c.Columns),
		)}
	case pg.KindForeignKey:
		sql := fmt.Sprintf(
			"ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			quoteTableMySQL(tableName), qiMySQL(c.Name),
			quoteColListMySQL(c.Columns), qiMySQL(c.FKTable), quoteColListMySQL(c.FKColumns),
		)
		if c.FKOnDelete != "" && c.FKOnDelete != pg.FKActionNoAction {
			sql += " ON DELETE " + string(c.FKOnDelete)
		}
		return []string{sql}
	}
	return nil
}

func dropConstraintSQLMySQL(tableName string, c pg.Constraint) []string {
	switch c.Kind {
	case pg.KindIndex, pg.KindUniqueIndex:
		// MySQL: DROP INDEX name ON table
		return []string{fmt.Sprintf("DROP INDEX %s ON %s", qiMySQL(c.Name), quoteTableMySQL(tableName))}
	case pg.KindForeignKey:
		return []string{fmt.Sprintf(
			"ALTER TABLE %s DROP FOREIGN KEY %s",
			quoteTableMySQL(tableName), qiMySQL(c.Name),
		)}
	default:
		return []string{fmt.Sprintf(
			"ALTER TABLE %s DROP CONSTRAINT %s",
			quoteTableMySQL(tableName), qiMySQL(c.Name),
		)}
	}
}

// mysqlType maps PostgreSQL SQL type strings to MySQL equivalents.
func mysqlType(pgType string) string {
	lower := strings.ToLower(pgType)
	switch {
	case lower == "uuid":
		return "CHAR(36)"
	case lower == "boolean" || lower == "bool":
		return "TINYINT(1)"
	case lower == "timestamptz" || lower == "timestamp with time zone":
		return "DATETIME(6)"
	case lower == "timestamp" || lower == "timestamp without time zone":
		return "DATETIME"
	case lower == "jsonb" || lower == "json":
		return "JSON"
	case lower == "bigserial":
		return "BIGINT AUTO_INCREMENT"
	case lower == "serial":
		return "INT AUTO_INCREMENT"
	case lower == "text":
		return "LONGTEXT"
	case strings.HasPrefix(lower, "numeric") || strings.HasPrefix(lower, "decimal"):
		// Pass through — MySQL supports NUMERIC(p,s) / DECIMAL(p,s) natively.
		return strings.ToUpper(pgType)
	case strings.HasPrefix(lower, "varchar"):
		return strings.ToUpper(pgType)
	case lower == "integer" || lower == "int" || lower == "int4":
		return "INT"
	case lower == "bigint" || lower == "int8":
		return "BIGINT"
	case lower == "smallint" || lower == "int2":
		return "SMALLINT"
	case lower == "real" || lower == "float4":
		return "FLOAT"
	case lower == "double precision" || lower == "float8":
		return "DOUBLE"
	default:
		// Unknown type — pass through and let MySQL decide.
		return strings.ToUpper(pgType)
	}
}

// mysqlDefaultExpr translates a PostgreSQL default expression to MySQL syntax.
func mysqlDefaultExpr(pgDefault string) string {
	switch strings.TrimSpace(pgDefault) {
	case "gen_random_uuid()", "uuid_generate_v4()":
		return "(UUID())"
	case "now()", "current_timestamp", "CURRENT_TIMESTAMP":
		return "CURRENT_TIMESTAMP(6)"
	case "true":
		return "1"
	case "false":
		return "0"
	case "'{}'::jsonb", "'{}'::json", "'{}'":
		return "('{}') " // MySQL JSON default requires expression in parens
	default:
		return pgDefault
	}
}

// qiMySQL quotes a single MySQL identifier with backticks.
func qiMySQL(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// quoteTableMySQL quotes a potentially schema-qualified table name for MySQL.
func quoteTableMySQL(name string) string {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return qiMySQL(parts[0]) + "." + qiMySQL(parts[1])
	}
	return qiMySQL(name)
}

// quoteColListMySQL returns a comma-separated list of backtick-quoted column names.
func quoteColListMySQL(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = qiMySQL(c)
	}
	return strings.Join(quoted, ", ")
}
