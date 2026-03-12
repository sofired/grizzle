package kit

import (
	pg "github.com/sofired/grizzle/schema/pg"
)

// ChangeKind identifies the type of schema change.
type ChangeKind string

const (
	ChangeCreateTable        ChangeKind = "create_table"
	ChangeDropTable          ChangeKind = "drop_table"
	ChangeRenameTable        ChangeKind = "rename_table"
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
	TableName string // qualified name (new name for rename, otherwise current name)

	// Set for ChangeRenameTable: the previous qualified table name.
	OldTableName string

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
// Rename detection: if a table in the new snapshot carries a PreviousName
// that matches a table in the old snapshot (and that old table is absent from
// the new snapshot), a ChangeRenameTable is emitted instead of a DROP+CREATE
// pair. Column/constraint diffs for the renamed table are appended as usual.
//
// Ordering is deterministic:
//  1. Rename tables (must happen before column diffs on the renamed table).
//  2. Create new tables (so FK references resolve).
//  3. Alter existing and renamed tables (columns first, then constraints).
//  4. Drop removed tables (in reverse to respect FKs — caller may need to reorder).
func Diff(old, new Snapshot) []Change {
	var changes []Change

	// Build a set of old table names that are consumed by a rename, so that
	// they are not also emitted as DROP TABLE.
	renamedOldNames := make(map[string]bool)

	// Phase 1: detect renames — new table with PreviousName matching a dropped table.
	for newName, newT := range new.Tables {
		if newT.PreviousName == "" {
			continue
		}
		oldName := newT.PreviousName
		// Qualify the old name if the new table has a schema.
		if newT.Schema != "" && !containsDot(oldName) {
			oldName = newT.Schema + "." + oldName
		}
		oldT, oldExists := old.Tables[oldName]
		if !oldExists {
			continue // previous name not found in old snapshot — treat as create
		}
		if _, newExists := new.Tables[oldName]; newExists {
			continue // old name still exists in new schema — not a rename
		}
		// Emit rename.
		renamedOldNames[oldName] = true
		changes = append(changes, Change{
			Kind:         ChangeRenameTable,
			TableName:    newName,
			OldTableName: oldName,
		})
		// Diff columns/constraints of the renamed table using the new name.
		changes = append(changes, diffTable(newName, oldT, newT)...)
	}

	// Phase 2: new tables not in old (and not a rename target) → CREATE TABLE.
	for name, newT := range new.Tables {
		if _, exists := old.Tables[name]; exists {
			continue // present in old — handled in phase 3
		}
		if newT.PreviousName != "" {
			// Check if we already emitted a rename for this table.
			oldName := newT.PreviousName
			if newT.Schema != "" && !containsDot(oldName) {
				oldName = newT.Schema + "." + oldName
			}
			if renamedOldNames[oldName] {
				continue // already handled as a rename
			}
		}
		changes = append(changes, Change{
			Kind:      ChangeCreateTable,
			TableName: name,
		})
		// Individual column and constraint adds are implied by CREATE TABLE;
		// we don't emit separate ADD COLUMN / ADD INDEX changes for new tables.
	}

	// Phase 3: tables present in both (unchanged name) → diff columns and constraints.
	for name, newT := range new.Tables {
		oldT, exists := old.Tables[name]
		if !exists {
			continue // handled above
		}
		// Skip if this table was already diffed as part of a rename.
		if newT.PreviousName != "" {
			oldName := newT.PreviousName
			if newT.Schema != "" && !containsDot(oldName) {
				oldName = newT.Schema + "." + oldName
			}
			if renamedOldNames[oldName] {
				continue
			}
		}
		changes = append(changes, diffTable(name, oldT, newT)...)
	}

	// Phase 4: tables in old but not new → DROP TABLE (unless consumed by rename).
	for name := range old.Tables {
		if _, exists := new.Tables[name]; exists {
			continue
		}
		if renamedOldNames[name] {
			continue // consumed by a rename — do not drop
		}
		changes = append(changes, Change{
			Kind:      ChangeDropTable,
			TableName: name,
		})
	}

	return changes
}

// containsDot reports whether s contains a "." character.
func containsDot(s string) bool {
	for _, r := range s {
		if r == '.' {
			return true
		}
	}
	return false
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

	// Added constraints.
	for _, nc := range new.Constraints {
		if _, exists := oldCons[nc.Name]; !exists {
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
		nc, exists := newCons[oc.Name]
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

func constraintMap(cons []pg.Constraint) map[string]pg.Constraint {
	m := make(map[string]pg.Constraint, len(cons))
	for _, c := range cons {
		m[c.Name] = c
	}
	return m
}

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
	return true
}
