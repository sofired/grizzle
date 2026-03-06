package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	pg "github.com/grizzle-orm/grizzle/schema/pg"
)

// IntrospectMySQL reads the live schema from a MySQL / MariaDB database using
// the standard database/sql interface. The caller is responsible for providing
// an open *sql.DB connected to the target database.
//
// The schemas parameter restricts introspection to the given database names
// (MySQL uses "database" rather than "schema"). Pass no arguments to use the
// current database (selected at connection time).
//
//	import _ "github.com/go-sql-driver/mysql"
//	db, _ := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/mydb")
//	snap, err := introspect.IntrospectMySQL(ctx, db)
func IntrospectMySQL(ctx context.Context, db *sql.DB, databases ...string) (LiveSnapshot, error) {
	currentDB, err := currentMySQLDB(ctx, db)
	if err != nil {
		return LiveSnapshot{}, err
	}

	dbs := databases
	if len(dbs) == 0 {
		dbs = []string{currentDB}
	}

	snap := LiveSnapshot{Tables: make(map[string]*LiveTable)}

	for _, dbName := range dbs {
		if err := introspectMySQLDB(ctx, db, dbName, snap); err != nil {
			return LiveSnapshot{}, fmt.Errorf("introspect database %q: %w", dbName, err)
		}
	}
	return snap, nil
}

func currentMySQLDB(ctx context.Context, db *sql.DB) (string, error) {
	var name string
	if err := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name); err != nil {
		return "", fmt.Errorf("get current database: %w", err)
	}
	return name, nil
}

func introspectMySQLDB(ctx context.Context, db *sql.DB, dbName string, snap LiveSnapshot) error {
	// Tables
	tables, err := mySQLTables(ctx, db, dbName)
	if err != nil {
		return err
	}
	for _, t := range tables {
		key := dbName + "." + t
		snap.Tables[key] = &LiveTable{Name: t, Schema: dbName}
	}

	// Columns
	if err := mySQLColumns(ctx, db, dbName, snap); err != nil {
		return err
	}

	// Indexes (including UNIQUE)
	if err := mySQLIndexes(ctx, db, dbName, snap); err != nil {
		return err
	}

	// Foreign keys
	if err := mySQLForeignKeys(ctx, db, dbName, snap); err != nil {
		return err
	}

	return nil
}

func mySQLTables(ctx context.Context, db *sql.DB, dbName string) ([]string, error) {
	const q = `SELECT TABLE_NAME FROM information_schema.TABLES
	           WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
	           ORDER BY TABLE_NAME`
	rows, err := db.QueryContext(ctx, q, dbName)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func mySQLColumns(ctx context.Context, db *sql.DB, dbName string, snap LiveSnapshot) error {
	const q = `
		SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE, COLUMN_TYPE,
		       IS_NULLABLE, COLUMN_DEFAULT, COLUMN_KEY, EXTRA
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME, ORDINAL_POSITION`

	rows, err := db.QueryContext(ctx, q, dbName)
	if err != nil {
		return fmt.Errorf("query columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			tableName, colName, dataType, colType string
			isNullable                             string
			colDefault                             sql.NullString
			colKey, extra                          string
		)
		if err := rows.Scan(&tableName, &colName, &dataType, &colType,
			&isNullable, &colDefault, &colKey, &extra); err != nil {
			return err
		}

		key := dbName + "." + tableName
		ts, ok := snap.Tables[key]
		if !ok {
			continue
		}

		col := pg.ColumnDef{
			Name:       colName,
			SQLType:    normalizeMySQLType(dataType, colType),
			NotNull:    isNullable == "NO",
			PrimaryKey: colKey == "PRI",
			Unique:     colKey == "UNI",
		}
		if colDefault.Valid {
			col.HasDefault = true
			col.DefaultExpr = colDefault.String
		}
		if strings.Contains(extra, "auto_increment") {
			col.HasDefault = true
			col.DefaultExpr = "AUTO_INCREMENT"
		}
		ts.Columns = append(ts.Columns, col)
	}
	return rows.Err()
}

func mySQLIndexes(ctx context.Context, db *sql.DB, dbName string, snap LiveSnapshot) error {
	const q = `
		SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, COLUMN_NAME, SEQ_IN_INDEX
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ?
		  AND INDEX_NAME != 'PRIMARY'
		ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`

	rows, err := db.QueryContext(ctx, q, dbName)
	if err != nil {
		return fmt.Errorf("query indexes: %w", err)
	}
	defer rows.Close()

	type idxKey struct{ table, name string }
	type idxInfo struct {
		nonUnique bool
		cols      []string
	}
	indexes := make(map[idxKey]*idxInfo)
	var order []idxKey

	for rows.Next() {
		var tableName, indexName, colName string
		var nonUnique int
		var seqInIndex int
		if err := rows.Scan(&tableName, &indexName, &nonUnique, &colName, &seqInIndex); err != nil {
			return err
		}
		k := idxKey{tableName, indexName}
		if _, ok := indexes[k]; !ok {
			indexes[k] = &idxInfo{nonUnique: nonUnique == 1}
			order = append(order, k)
		}
		indexes[k].cols = append(indexes[k].cols, colName)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, k := range order {
		info := indexes[k]
		key := dbName + "." + k.table
		ts, ok := snap.Tables[key]
		if !ok {
			continue
		}
		kind := pg.KindUniqueIndex
		if info.nonUnique {
			kind = pg.KindIndex
		}
		ts.Constraints = append(ts.Constraints, pg.Constraint{
			Kind:    kind,
			Name:    k.name,
			Columns: info.cols,
		})
	}
	return nil
}

func mySQLForeignKeys(ctx context.Context, db *sql.DB, dbName string, snap LiveSnapshot) error {
	const q = `
		SELECT kcu.CONSTRAINT_NAME, kcu.TABLE_NAME, kcu.COLUMN_NAME,
		       kcu.REFERENCED_TABLE_NAME, kcu.REFERENCED_COLUMN_NAME,
		       rc.DELETE_RULE, rc.UPDATE_RULE
		FROM information_schema.KEY_COLUMN_USAGE kcu
		JOIN information_schema.REFERENTIAL_CONSTRAINTS rc
		  ON rc.CONSTRAINT_SCHEMA = kcu.CONSTRAINT_SCHEMA
		 AND rc.CONSTRAINT_NAME   = kcu.CONSTRAINT_NAME
		WHERE kcu.CONSTRAINT_SCHEMA = ?
		  AND kcu.REFERENCED_TABLE_NAME IS NOT NULL
		ORDER BY kcu.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`

	rows, err := db.QueryContext(ctx, q, dbName)
	if err != nil {
		return fmt.Errorf("query foreign keys: %w", err)
	}
	defer rows.Close()

	type fkKey struct{ name, table string }
	type fkInfo struct {
		cols, refCols []string
		refTable      string
		onDelete      string
		onUpdate      string
	}
	fks := make(map[fkKey]*fkInfo)
	var order []fkKey

	for rows.Next() {
		var constraintName, tableName, colName, refTable, refCol, delRule, updRule string
		if err := rows.Scan(&constraintName, &tableName, &colName, &refTable, &refCol, &delRule, &updRule); err != nil {
			return err
		}
		k := fkKey{constraintName, tableName}
		if _, ok := fks[k]; !ok {
			fks[k] = &fkInfo{refTable: refTable, onDelete: delRule, onUpdate: updRule}
			order = append(order, k)
		}
		fks[k].cols = append(fks[k].cols, colName)
		fks[k].refCols = append(fks[k].refCols, refCol)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, k := range order {
		info := fks[k]
		key := dbName + "." + k.table
		ts, ok := snap.Tables[key]
		if !ok {
			continue
		}
		ts.Constraints = append(ts.Constraints, pg.Constraint{
			Kind:       pg.KindForeignKey,
			Name:       k.name,
			Columns:    info.cols,
			FKTable:    info.refTable,
			FKColumns:  info.refCols,
			FKOnDelete: pg.FKAction(normalizeFKAction(info.onDelete)),
			FKOnUpdate: pg.FKAction(normalizeFKAction(info.onUpdate)),
		})
	}
	return nil
}

// normalizeMySQLType maps MySQL DATA_TYPE + COLUMN_TYPE to a canonical type string.
func normalizeMySQLType(dataType, colType string) string {
	lower := strings.ToLower(dataType)
	switch lower {
	case "tinyint":
		if strings.Contains(colType, "(1)") {
			return "boolean"
		}
		return "smallint"
	case "char":
		if strings.Contains(colType, "(36)") {
			return "uuid" // treat CHAR(36) as UUID
		}
		return colType
	case "datetime":
		return "timestamp"
	case "json":
		return "jsonb"
	case "int", "integer":
		return "integer"
	case "bigint":
		return "bigint"
	case "smallint":
		return "smallint"
	case "float":
		return "real"
	case "double":
		return "double precision"
	case "text", "mediumtext", "longtext":
		return "text"
	case "varchar":
		// colType is e.g. "varchar(255)" — normalise to "varchar(255)"
		return strings.ToLower(colType)
	default:
		return strings.ToLower(dataType)
	}
}

// normalizeFKAction converts MySQL referential action strings to pg.FKAction constants.
func normalizeFKAction(action string) string {
	switch strings.ToUpper(action) {
	case "CASCADE":
		return string(pg.FKActionCascade)
	case "SET NULL":
		return string(pg.FKActionSetNull)
	case "SET DEFAULT":
		return string(pg.FKActionSetDefault)
	case "RESTRICT":
		return string(pg.FKActionRestrict)
	default:
		return string(pg.FKActionNoAction)
	}
}
