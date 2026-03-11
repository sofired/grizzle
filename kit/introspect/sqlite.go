package introspect

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	pg "github.com/sofired/grizzle/schema/pg"
)

// IntrospectSQLite reads the live schema from a SQLite database using the
// standard database/sql interface.
//
// Because SQLite uses type affinity (any type name is valid), this function
// reads type names verbatim and normalises only the common aliases (INTEGER,
// TEXT, REAL, NUMERIC, BLOB → their lowercase equivalents). Canonical type
// names stored by GenerateCreateSQLSQLite (uuid, boolean, timestamptz, …)
// round-trip cleanly.
//
// SQLite has no schema namespace; all tables are returned without a Schema field.
//
//	import _ "github.com/mattn/go-sqlite3"
//	db, _ := sql.Open("sqlite3", "./mydb.sqlite")
//	snap, err := introspect.IntrospectSQLite(ctx, db)
func IntrospectSQLite(ctx context.Context, db *sql.DB) (LiveSnapshot, error) {
	snap := LiveSnapshot{Tables: make(map[string]*LiveTable)}

	// Enumerate all user tables.
	tables, err := sqliteTables(ctx, db)
	if err != nil {
		return snap, fmt.Errorf("introspect tables: %w", err)
	}
	for _, name := range tables {
		snap.Tables[name] = &LiveTable{Name: name}
	}

	// Columns, PKs, defaults via PRAGMA table_info.
	for name, tbl := range snap.Tables {
		cols, err := sqliteColumns(ctx, db, name)
		if err != nil {
			return snap, fmt.Errorf("introspect columns for %s: %w", name, err)
		}
		tbl.Columns = cols
	}

	// Indexes via PRAGMA index_list + index_info + sqlite_master (for WHERE).
	for name, tbl := range snap.Tables {
		cons, err := sqliteIndexes(ctx, db, name)
		if err != nil {
			return snap, fmt.Errorf("introspect indexes for %s: %w", name, err)
		}
		tbl.Constraints = append(tbl.Constraints, cons...)
	}

	// Foreign keys via PRAGMA foreign_key_list.
	for name, tbl := range snap.Tables {
		cons, err := sqliteForeignKeys(ctx, db, name)
		if err != nil {
			return snap, fmt.Errorf("introspect foreign keys for %s: %w", name, err)
		}
		tbl.Constraints = append(tbl.Constraints, cons...)
	}

	return snap, nil
}

// sqliteTables returns all non-system table names in the database.
func sqliteTables(ctx context.Context, db *sql.DB) ([]string, error) {
	const q = `SELECT name FROM sqlite_master
	           WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
	           ORDER BY name`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// sqliteColumns introspects column definitions using PRAGMA table_info.
// The pragma returns: cid, name, type, notnull, dflt_value, pk.
func sqliteColumns(ctx context.Context, db *sql.DB, table string) ([]pg.ColumnDef, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, fmt.Errorf("pragma table_info: %w", err)
	}
	defer rows.Close()

	var cols []pg.ColumnDef
	for rows.Next() {
		var (
			cid      int
			name     string
			typeName string
			notNull  int
			dfltVal  sql.NullString
			pk       int
		)
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &dfltVal, &pk); err != nil {
			return nil, err
		}

		col := pg.ColumnDef{
			Name:       name,
			SQLType:    normalizeSQLiteType(typeName),
			NotNull:    notNull == 1 || pk > 0, // PK columns are implicitly NOT NULL in SQLite
			PrimaryKey: pk > 0,
		}
		if dfltVal.Valid && dfltVal.String != "" {
			col.HasDefault = true
			col.DefaultExpr = dfltVal.String
		}
		// SQLite serial columns (INTEGER PRIMARY KEY) always have an implicit default.
		if pk > 0 && strings.EqualFold(typeName, "integer") {
			col.HasDefault = true
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

// sqliteIndexes introspects indexes for one table using PRAGMA index_list and
// PRAGMA index_info. It also fetches WHERE clauses from sqlite_master for
// partial indexes.
func sqliteIndexes(ctx context.Context, db *sql.DB, table string) ([]pg.Constraint, error) {
	// index_list: seq, name, unique, origin, partial
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%q)", table))
	if err != nil {
		return nil, fmt.Errorf("pragma index_list: %w", err)
	}
	defer rows.Close()

	type idxMeta struct {
		name    string
		unique  bool
		partial bool
	}
	var indexes []idxMeta
	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return nil, err
		}
		// Skip auto-created indexes (origin='pk' for PK, 'u' for inline UNIQUE).
		if origin == "pk" {
			continue
		}
		indexes = append(indexes, idxMeta{name: name, unique: unique == 1, partial: partial == 1})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var constraints []pg.Constraint
	for _, idx := range indexes {
		// Fetch columns.
		cols, err := sqliteIndexCols(ctx, db, idx.name)
		if err != nil {
			return nil, err
		}

		kind := pg.KindIndex
		if idx.unique {
			kind = pg.KindUniqueIndex
		}
		c := pg.Constraint{Kind: kind, Name: idx.name, Columns: cols}

		// Fetch WHERE clause for partial indexes.
		if idx.partial {
			where, err := sqliteIndexWhere(ctx, db, idx.name)
			if err != nil {
				return nil, err
			}
			c.WhereExpr = where
		}

		constraints = append(constraints, c)
	}
	return constraints, nil
}

func sqliteIndexCols(ctx context.Context, db *sql.DB, idxName string) ([]string, error) {
	// index_info: seqno, cid, name
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_info(%q)", idxName))
	if err != nil {
		return nil, fmt.Errorf("pragma index_info(%q): %w", idxName, err)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var seqno, cid int
		var name string
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

// sqliteIndexWhere extracts the WHERE clause from a partial index definition
// stored in sqlite_master.
func sqliteIndexWhere(ctx context.Context, db *sql.DB, idxName string) (string, error) {
	var ddl string
	err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='index' AND name=?`, idxName,
	).Scan(&ddl)
	if err != nil {
		return "", fmt.Errorf("read index DDL for %q: %w", idxName, err)
	}
	// Extract WHERE clause: everything after " WHERE ".
	upper := strings.ToUpper(ddl)
	idx := strings.LastIndex(upper, " WHERE ")
	if idx < 0 {
		return "", nil
	}
	return strings.TrimSpace(ddl[idx+7:]), nil
}

// sqliteForeignKeys introspects FK constraints using PRAGMA foreign_key_list.
// The pragma returns: id, seq, table, from, to, on_update, on_delete, match.
func sqliteForeignKeys(ctx context.Context, db *sql.DB, table string) ([]pg.Constraint, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%q)", table))
	if err != nil {
		return nil, fmt.Errorf("pragma foreign_key_list: %w", err)
	}
	defer rows.Close()

	type fkRow struct {
		id       int
		seq      int
		fkTable  string
		fromCol  string
		toCol    string
		onUpdate string
		onDelete string
	}

	fkMap := make(map[int]*pg.Constraint)
	var fkOrder []int

	for rows.Next() {
		var r fkRow
		var match string
		if err := rows.Scan(&r.id, &r.seq, &r.fkTable, &r.fromCol, &r.toCol,
			&r.onUpdate, &r.onDelete, &match); err != nil {
			return nil, err
		}
		if _, exists := fkMap[r.id]; !exists {
			fkMap[r.id] = &pg.Constraint{
				Kind:       pg.KindForeignKey,
				Name:       fmt.Sprintf("fk_%s_%d", table, r.id),
				FKTable:    r.fkTable,
				FKOnDelete: pg.FKAction(normalizeFKAction(r.onDelete)),
				FKOnUpdate: pg.FKAction(normalizeFKAction(r.onUpdate)),
			}
			fkOrder = append(fkOrder, r.id)
		}
		fk := fkMap[r.id]
		fk.Columns = append(fk.Columns, r.fromCol)
		fk.FKColumns = append(fk.FKColumns, r.toCol)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	constraints := make([]pg.Constraint, 0, len(fkOrder))
	for _, id := range fkOrder {
		constraints = append(constraints, *fkMap[id])
	}
	return constraints, nil
}

// normalizeSQLiteType normalises a SQLite type name to lowercase.
// Canonical grizzle type names (uuid, boolean, timestamptz, …) pass through
// unchanged; only bare SQLite affinity names are lowercased.
func normalizeSQLiteType(t string) string {
	if t == "" {
		return "text" // SQLite BLOB affinity with no type → treat as text
	}
	// Preserve canonical type names written by GenerateCreateSQLSQLite.
	// These already match what the schema DSL produces, so no mapping needed.
	return strings.ToLower(t)
}
