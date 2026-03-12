package kit

import (
	"sort"

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

// RenameResolver is called by DiffWithResolver when it detects a probable
// table rename — a table that disappeared from the old snapshot and a new
// table that appeared in the new snapshot with overlapping columns.
//
// Return true to emit ChangeRenameTable (preserves data); return false to
// emit ChangeDropTable + ChangeCreateTable instead.
//
// This mirrors Drizzle Kit's interactive prompt behaviour: at migration
// generation time, the developer is asked "Did you rename X to Y?" and their
// answer determines which SQL is generated. The resolver callback is the
// library-level equivalent; the CLI wires it to an actual terminal prompt.
type RenameResolver func(oldName, newName string) bool

// Diff computes the ordered list of Changes needed to transition from
// the old snapshot to the new snapshot. Pass EmptySnapshot() as old
// when targeting a fresh database.
//
// Renames are not detected automatically — tables that disappear become
// ChangeDropTable and new tables become ChangeCreateTable. Use
// DiffWithResolver to enable interactive rename detection, matching
// Drizzle Kit's behaviour.
//
// Ordering is deterministic:
//  1. Rename tables (must happen before column diffs on the renamed table).
//  2. Create new tables (so FK references resolve).
//  3. Alter existing and renamed tables (columns first, then constraints).
//  4. Drop removed tables (in reverse to respect FKs — caller may need to reorder).
func Diff(old, new Snapshot) []Change {
	return DiffWithResolver(old, new, nil)
}

// DiffWithResolver computes the ordered list of Changes needed to transition
// from the old snapshot to the new snapshot, with interactive rename detection.
//
// When a table disappears from old and a new table appears in new, and the two
// tables share at least one column name in common, resolver is called to
// determine whether the change is a rename or a drop+create. If resolver is
// nil, all such cases are treated as drop+create (equivalent to Diff).
//
// The resolver is called at most once per (oldName, newName) pair. Once a
// rename is confirmed, both tables are marked as handled and no further
// rename prompt is issued for them.
func DiffWithResolver(old, new Snapshot, resolver RenameResolver) []Change {
	var changes []Change

	// Find all dropped tables (in old, not in new) and created tables (in new, not in old).
	var dropped, created []string
	for name := range old.Tables {
		if _, exists := new.Tables[name]; !exists {
			dropped = append(dropped, name)
		}
	}
	for name := range new.Tables {
		if _, exists := old.Tables[name]; !exists {
			created = append(created, name)
		}
	}
	sort.Strings(dropped)
	sort.Strings(created)

	// Detect renames: for each dropped table, find created tables that share
	// column name overlap (the same heuristic Drizzle Kit uses to identify
	// probable renames before prompting the developer).
	renamedOld := make(map[string]bool) // old names consumed by a confirmed rename
	renamedNew := make(map[string]bool) // new names consumed by a confirmed rename

	if resolver != nil && len(dropped) > 0 && len(created) > 0 {
		for _, oldName := range dropped {
			oldT := old.Tables[oldName]
			for _, newName := range created {
				if renamedNew[newName] {
					continue // already claimed by a prior rename
				}
				newT := new.Tables[newName]
				if columnOverlap(oldT, newT) == 0 {
					continue // no shared column names — unrelated tables, skip
				}
				if resolver(oldName, newName) {
					renamedOld[oldName] = true
					renamedNew[newName] = true
					changes = append(changes, Change{
						Kind:         ChangeRenameTable,
						TableName:    newName,
						OldTableName: oldName,
					})
					// Diff columns/constraints of the renamed table.
					changes = append(changes, diffTable(newName, oldT, newT)...)
					break
				}
			}
		}
	}

	// Phase 2: new tables not claimed by a rename → CREATE TABLE.
	for _, name := range created {
		if renamedNew[name] {
			continue
		}
		changes = append(changes, Change{
			Kind:      ChangeCreateTable,
			TableName: name,
		})
	}

	// Phase 3: tables present in both snapshots → diff columns and constraints.
	// Process in sorted order for determinism.
	existing := make([]string, 0, len(new.Tables))
	for name := range new.Tables {
		if _, inOld := old.Tables[name]; inOld {
			existing = append(existing, name)
		}
	}
	sort.Strings(existing)
	for _, name := range existing {
		changes = append(changes, diffTable(name, old.Tables[name], new.Tables[name])...)
	}

	// Phase 4: dropped tables not consumed by a rename → DROP TABLE.
	for _, name := range dropped {
		if renamedOld[name] {
			continue
		}
		changes = append(changes, Change{
			Kind:      ChangeDropTable,
			TableName: name,
		})
	}

	return changes
}

// columnOverlap returns the Jaccard similarity of column name sets between
// two table snapshots. Returns 0 if either table has no columns.
func columnOverlap(a, b *TableSnap) float64 {
	if len(a.Columns) == 0 || len(b.Columns) == 0 {
		return 0
	}
	aNames := make(map[string]bool, len(a.Columns))
	for _, c := range a.Columns {
		aNames[c.Name] = true
	}
	shared := 0
	for _, c := range b.Columns {
		if aNames[c.Name] {
			shared++
		}
	}
	if shared == 0 {
		return 0
	}
	union := len(aNames) + len(b.Columns) - shared
	return float64(shared) / float64(union)
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
