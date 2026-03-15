package kit

import (
	"fmt"
	"sort"
	"strings"

	pg "github.com/sofired/grizzle/schema/pg"
)

// ChangeKind identifies the type of schema change.
type ChangeKind string

const (
	ChangeCreateTable        ChangeKind = "create_table"
	ChangeDropTable          ChangeKind = "drop_table"
	ChangeAddColumn          ChangeKind = "add_column"
	ChangeDropColumn         ChangeKind = "drop_column"
	ChangeAlterColumnType    ChangeKind = "alter_column_type"
	ChangeAlterColumnNull    ChangeKind = "alter_column_nullable"
	ChangeAlterColumnDefault ChangeKind = "alter_column_default"
	ChangeRenameColumn       ChangeKind = "rename_column"
	ChangeAddConstraint      ChangeKind = "add_constraint"
	ChangeDropConstraint     ChangeKind = "drop_constraint"
)

// Change represents a single schema mutation — the unit that SQL generation works from.
type Change struct {
	Kind      ChangeKind
	TableName string // qualified name

	// Set for column-level changes.
	OldCol *pg.ColumnDef
	NewCol *pg.ColumnDef

	// Set for constraint-level changes.
	Constraint *pg.Constraint
}

// Diff computes the ordered list of Changes needed to transition from
// the old snapshot to the new snapshot. Pass EmptySnapshot() as old
// when targeting a fresh database.
//
// Ordering is deterministic:
//  1. Create new tables (so FK references resolve).
//  2. Alter existing tables (columns first, then constraints).
//  3. Drop removed constraints.
//  4. Drop removed tables.
//
// Output is sorted for determinism: within each phase, changes are sorted by
// table name, then by change kind, then by column name where applicable.
func Diff(old, new Snapshot) []Change {
	var changes []Change

	// Collect sorted table names to ensure deterministic output.
	newNames := sortedTableNames(new.Tables)
	oldNames := sortedTableNames(old.Tables)

	// Phase 1: new tables not in old → CREATE TABLE.
	for _, name := range newNames {
		if _, exists := old.Tables[name]; !exists {
			changes = append(changes, Change{
				Kind:      ChangeCreateTable,
				TableName: name,
			})
			// Individual column and constraint adds are implied by CREATE TABLE;
			// we don't emit separate ADD COLUMN / ADD INDEX changes for new tables.
		}
	}

	// Phase 2: tables present in both → diff columns and constraints.
	for _, name := range newNames {
		oldT, exists := old.Tables[name]
		if !exists {
			continue // handled above
		}
		changes = append(changes, diffTable(name, oldT, new.Tables[name])...)
	}

	// Phase 3: tables in old but not new → DROP TABLE.
	for _, name := range oldNames {
		if _, exists := new.Tables[name]; !exists {
			changes = append(changes, Change{
				Kind:      ChangeDropTable,
				TableName: name,
			})
		}
	}

	return changes
}

// sortedTableNames returns the keys of the given map, sorted alphabetically.
func sortedTableNames(tables map[string]*TableSnap) []string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// diffTable computes column- and constraint-level changes for one table.
func diffTable(tableName string, old, new *TableSnap) []Change {
	var changes []Change

	// --- Columns ---
	oldCols := colMap(old.Columns)
	newCols := colMap(new.Columns)

	// Added columns (preserve new.Columns order).
	for _, nc := range new.Columns {
		oc, exists := oldCols[nc.Name]
		if !exists {
			nc := nc // copy
			changes = append(changes, Change{
				Kind:      ChangeAddColumn,
				TableName: tableName,
				NewCol:    &nc,
			})
			continue
		}
		// Modified columns.
		changes = append(changes, diffColumn(tableName, oc, nc)...)
	}

	// Dropped columns.
	for _, oc := range old.Columns {
		if _, exists := newCols[oc.Name]; !exists {
			oc := oc // copy
			changes = append(changes, Change{
				Kind:      ChangeDropColumn,
				TableName: tableName,
				OldCol:    &oc,
			})
		}
	}

	// --- Constraints ---
	oldCons := constraintMap(old.Constraints)
	newCons := constraintMap(new.Constraints)

	// Added constraints (preserve new.Constraints order for stability).
	for _, nc := range new.Constraints {
		key := constraintKey(nc)
		if _, exists := oldCons[key]; !exists {
			nc := nc
			changes = append(changes, Change{
				Kind:       ChangeAddConstraint,
				TableName:  tableName,
				Constraint: &nc,
			})
		}
	}

	// Dropped constraints (or changed — drop+re-add).
	for _, oc := range old.Constraints {
		key := constraintKey(oc)
		nc, exists := newCons[key]
		if !exists {
			oc := oc
			changes = append(changes, Change{
				Kind:       ChangeDropConstraint,
				TableName:  tableName,
				Constraint: &oc,
			})
		} else if !constraintsEqual(oc, nc) {
			// Changed: drop then recreate.
			oc, nc := oc, nc
			changes = append(changes,
				Change{Kind: ChangeDropConstraint, TableName: tableName, Constraint: &oc},
				Change{Kind: ChangeAddConstraint, TableName: tableName, Constraint: &nc},
			)
		}
	}

	return changes
}

// diffColumn emits ALTER COLUMN changes when type, nullability, or default differs.
func diffColumn(tableName string, old, new pg.ColumnDef) []Change {
	var changes []Change
	if old.SQLType != new.SQLType {
		o, n := old, new
		changes = append(changes, Change{
			Kind:      ChangeAlterColumnType,
			TableName: tableName,
			OldCol:    &o,
			NewCol:    &n,
		})
	}
	if old.NotNull != new.NotNull {
		o, n := old, new
		changes = append(changes, Change{
			Kind:      ChangeAlterColumnNull,
			TableName: tableName,
			OldCol:    &o,
			NewCol:    &n,
		})
	}
	if old.DefaultExpr != new.DefaultExpr || old.HasDefault != new.HasDefault {
		o, n := old, new
		changes = append(changes, Change{
			Kind:      ChangeAlterColumnDefault,
			TableName: tableName,
			OldCol:    &o,
			NewCol:    &n,
		})
	}
	return changes
}

func colMap(cols []pg.ColumnDef) map[string]pg.ColumnDef {
	m := make(map[string]pg.ColumnDef, len(cols))
	for _, c := range cols {
		m[c.Name] = c
	}
	return m
}

// constraintKey returns a stable, collision-free key for a constraint.
// It incorporates the kind and sorted column list so that unnamed constraints
// or constraints sharing a name do not collide in the map (Fix #6).
func constraintKey(c pg.Constraint) string {
	cols := make([]string, len(c.Columns))
	copy(cols, c.Columns)
	sort.Strings(cols)
	return fmt.Sprintf("%s:%s:%s", c.Kind, c.Name, strings.Join(cols, ","))
}

// constraintMap returns constraints keyed by their stable synthetic key.
func constraintMap(cons []pg.Constraint) map[string]pg.Constraint {
	m := make(map[string]pg.Constraint, len(cons))
	for _, c := range cons {
		m[constraintKey(c)] = c
	}
	return m
}

// constraintsEqual compares two constraints for logical equality,
// including FK-specific fields (Fix #9).
func constraintsEqual(a, b pg.Constraint) bool {
	if a.Kind != b.Kind || a.Name != b.Name || a.WhereExpr != b.WhereExpr || a.CheckExpr != b.CheckExpr {
		return false
	}
	if len(a.Columns) != len(b.Columns) {
		return false
	}
	for i := range a.Columns {
		if a.Columns[i] != b.Columns[i] {
			return false
		}
	}
	// Compare FK-specific fields (Fix #9).
	if a.Kind == pg.KindForeignKey {
		if a.FKTable != b.FKTable {
			return false
		}
		if len(a.FKColumns) != len(b.FKColumns) {
			return false
		}
		for i := range a.FKColumns {
			if a.FKColumns[i] != b.FKColumns[i] {
				return false
			}
		}
		if a.FKOnDelete != b.FKOnDelete || a.FKOnUpdate != b.FKOnUpdate {
			return false
		}
	}
	return true
}
