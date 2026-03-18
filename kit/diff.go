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
	ChangeRenameTable        ChangeKind = "rename_table"
	ChangeAddConstraint      ChangeKind = "add_constraint"
	ChangeDropConstraint     ChangeKind = "drop_constraint"
)

// Change represents a single schema mutation — the unit that SQL generation works from.
type Change struct {
	Kind      ChangeKind
	TableName string // qualified name; for ChangeRenameTable this is the old (source) name

	// RenameTarget is set only for ChangeRenameTable: it holds the new table name.
	// For all other change kinds this field is empty.
	RenameTarget string

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
// Rename detection: if a new table carries PreviousName matching a table that
// was removed, Diff emits ChangeRenameTable instead of ChangeDropTable +
// ChangeCreateTable. The same applies to columns within a table: a column with
// PreviousName matching a removed column emits ChangeRenameColumn instead of
// ChangeDropColumn + ChangeAddColumn. Each removed entity can be claimed by at
// most one rename; first match (by sorted new-name order) wins.
//
// Ordering is deterministic:
//  1. Rename or create new tables (renames before creates, both sorted by new name).
//  2. Alter existing tables (columns first, then constraints).
//  3. Drop removed tables.
//
// Output is sorted for determinism: within each phase, changes are sorted by
// table name. Within a single table, changes are emitted in slice order
// (renamed/added/modified columns first, then dropped columns, then
// added/dropped constraints). No secondary sort by change kind or column name
// is applied.
func Diff(old, new Snapshot) []Change {
	var changes []Change

	// Collect sorted table names to ensure deterministic output.
	newNames := sortedTableNames(new.Tables)
	oldNames := sortedTableNames(old.Tables)

	// Build sets of added and dropped tables for rename detection.
	// A table is "added" if it appears in new but not in old.
	// A table is "dropped" if it appears in old but not in new.
	droppedTables := make(map[string]struct{})
	for _, name := range oldNames {
		if _, exists := new.Tables[name]; !exists {
			droppedTables[name] = struct{}{}
		}
	}

	// Build a lookup: old table name → new table name, for tables renamed via PreviousName.
	// This prevents the same dropped-table from being matched by multiple new tables.
	renamedFrom := make(map[string]string) // old name → new name
	for _, name := range newNames {
		newT := new.Tables[name]
		if _, existsInOld := old.Tables[name]; !existsInOld && newT.PreviousName != "" {
			if _, wasDropped := droppedTables[newT.PreviousName]; wasDropped {
				if _, alreadyClaimed := renamedFrom[newT.PreviousName]; !alreadyClaimed {
					renamedFrom[newT.PreviousName] = name
				}
			}
		}
	}

	// Build reverse lookup: new table name → old table name.
	renamedTo := make(map[string]string) // new name → old name
	for oldName, newName := range renamedFrom {
		renamedTo[newName] = oldName
	}

	// Phase 1: new tables not in old.
	// Renames are collected and appended before creates so that FK references
	// from a newly created table to the renamed table's new name are safe,
	// regardless of alphabetical ordering.
	var renames []Change
	var creates []Change
	for _, name := range newNames {
		if _, exists := old.Tables[name]; !exists {
			if oldName, isRename := renamedTo[name]; isRename {
				renames = append(renames, Change{
					Kind:         ChangeRenameTable,
					TableName:    oldName,
					RenameTarget: name,
				})
			} else {
				creates = append(creates, Change{
					Kind:      ChangeCreateTable,
					TableName: name,
				})
			}
		}
	}
	changes = append(changes, renames...)
	changes = append(changes, creates...)

	// Phase 2: tables present in both → diff columns and constraints.
	// Also handle tables that were renamed: diff the renamed table's contents.
	for _, name := range newNames {
		if _, exists := old.Tables[name]; exists {
			changes = append(changes, diffTable(name, old.Tables[name], new.Tables[name])...)
		} else if oldName, isRename := renamedTo[name]; isRename {
			// Renamed table: diff columns and constraints under the new name.
			changes = append(changes, diffTable(name, old.Tables[oldName], new.Tables[name])...)
		}
	}

	// Phase 3: tables in old but not new → DROP TABLE (unless renamed away).
	for _, name := range oldNames {
		if _, exists := new.Tables[name]; !exists {
			if _, wasRenamed := renamedFrom[name]; !wasRenamed {
				changes = append(changes, Change{
					Kind:      ChangeDropTable,
					TableName: name,
				})
			}
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

	// Build a set of old column names that are not present in new (candidates for rename).
	droppedCols := make(map[string]struct{})
	for _, oc := range old.Columns {
		if _, exists := newCols[oc.Name]; !exists {
			droppedCols[oc.Name] = struct{}{}
		}
	}

	// Build a mapping: old col name → new col name, for columns renamed via PreviousName.
	// Each dropped-column can only be the source of one rename.
	colRenamedFrom := make(map[string]string) // old name → new name
	for _, nc := range new.Columns {
		if _, existsInOld := oldCols[nc.Name]; !existsInOld && nc.PreviousName != "" {
			if _, wasDropped := droppedCols[nc.PreviousName]; wasDropped {
				if _, alreadyClaimed := colRenamedFrom[nc.PreviousName]; !alreadyClaimed {
					colRenamedFrom[nc.PreviousName] = nc.Name
				}
			}
		}
	}
	// Reverse lookup: new col name → old col name.
	colRenamedTo := make(map[string]string) // new name → old name
	for oldName, newName := range colRenamedFrom {
		colRenamedTo[newName] = oldName
	}

	// Added/renamed columns (preserve new.Columns order).
	for _, nc := range new.Columns {
		oc, exists := oldCols[nc.Name]
		if !exists {
			nc := nc // copy
			if oldColName, isRename := colRenamedTo[nc.Name]; isRename {
				// Emit rename: OldCol carries the old name, NewCol carries the new name.
				oldColDef := oldCols[oldColName]
				o, n := oldColDef, nc
				changes = append(changes, Change{
					Kind:      ChangeRenameColumn,
					TableName: tableName,
					OldCol:    &o,
					NewCol:    &n,
				})
				// Also diff the old vs new column definitions (targeting the new
				// column name) so that type/nullability/default changes that
				// co-occur with the rename are not silently dropped.
				// We use a copy of the old column with its name updated to the new
				// name so that the ALTER COLUMN statements reference the post-rename
				// column name.
				oldColRenamed := oldColDef
				oldColRenamed.Name = nc.Name
				changes = append(changes, diffColumn(tableName, oldColRenamed, nc)...)
			} else {
				changes = append(changes, Change{
					Kind:      ChangeAddColumn,
					TableName: tableName,
					NewCol:    &nc,
				})
			}
			continue
		}
		// Modified columns.
		changes = append(changes, diffColumn(tableName, oc, nc)...)
	}

	// Dropped columns (skip columns that were renamed away).
	for _, oc := range old.Columns {
		if _, exists := newCols[oc.Name]; !exists {
			if _, wasRenamed := colRenamedFrom[oc.Name]; !wasRenamed {
				oc := oc // copy
				changes = append(changes, Change{
					Kind:      ChangeDropColumn,
					TableName: tableName,
					OldCol:    &oc,
				})
			}
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
// or constraints sharing a name do not collide in the map.
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
// including FK-specific fields (FKTable, FKColumns, FKOnDelete, FKOnUpdate).
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
	// Compare FK-specific fields when present.
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
