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

	// View change kinds.
	ChangeCreateView ChangeKind = "create_view"
	ChangeDropView   ChangeKind = "drop_view"

	// Enum change kinds (PostgreSQL only).
	ChangeCreateEnum ChangeKind = "create_enum"
	ChangeAlterEnum  ChangeKind = "alter_enum" // adds values to an existing enum
	ChangeDropEnum   ChangeKind = "drop_enum"
)

// Change represents a single schema mutation — the unit that SQL generation works from.
type Change struct {
	Kind      ChangeKind
	TableName string // qualified name (also used as object name for views/enums); for ChangeRenameTable this is the old (source) name

	// RenameTarget is set only for ChangeRenameTable: it holds the new table name.
	// For all other change kinds this field is empty.
	RenameTarget string

	// Set for column-level changes.
	OldCol *pg.ColumnDef
	NewCol *pg.ColumnDef

	// Set for constraint-level changes.
	Constraint *pg.Constraint

	// Set for view-level changes.
	View *ViewSnap

	// Set for enum-level changes.
	OldEnum *EnumSnap
	NewEnum *EnumSnap
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
// most one rename; for tables, the first match in sorted new-table-name order
// wins, and for columns, the first match in new.Columns slice order wins.
//
// Ordering is deterministic and safe for direct application:
//
//  1. Create new enums — types may be referenced by table columns.
//  2. Rename or create new tables (renames before creates, both sorted by new name).
//  3. Create new views — views SELECT FROM tables, which must already exist.
//  4. Alter existing tables (columns + constraints, including drops).
//  5. Alter existing enums (add values only; value removal is unsupported).
//  6. Replace changed views (DROP then CREATE OR REPLACE in sequence).
//  7. Drop removed views — before tables, since views depend on tables.
//  8. Drop removed tables (unless renamed away).
//  9. Drop removed enums — after tables that may reference them are gone.
//
// Within each phase, output is sorted by object name for determinism.
func Diff(old, new Snapshot) []Change {
	var changes []Change

	// Normalise nil maps so range loops are safe.
	if old.Views == nil {
		old.Views = make(map[string]*ViewSnap)
	}
	if old.Enums == nil {
		old.Enums = make(map[string]*EnumSnap)
	}
	if new.Views == nil {
		new.Views = make(map[string]*ViewSnap)
	}
	if new.Enums == nil {
		new.Enums = make(map[string]*EnumSnap)
	}

	newNames := sortedTableNames(new.Tables)
	oldNames := sortedTableNames(old.Tables)

	// Build sets of added and dropped tables for rename detection.
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

	// Phase 1: new enums not in old → CREATE TYPE ... AS ENUM.
	for _, name := range sortedKeys(new.Enums) {
		if _, exists := old.Enums[name]; !exists {
			e := *new.Enums[name]
			changes = append(changes, Change{
				Kind:      ChangeCreateEnum,
				TableName: name,
				NewEnum:   &e,
			})
		}
	}

	// Phase 2: new tables not in old.
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

	// Phase 3: new views not in old → CREATE VIEW (after tables exist).
	for _, name := range sortedKeys(new.Views) {
		if _, exists := old.Views[name]; !exists {
			v := *new.Views[name]
			changes = append(changes, Change{
				Kind:      ChangeCreateView,
				TableName: name,
				View:      &v,
			})
		}
	}

	// Phase 4: enums in both → diff values (additions only; removal requires pg_catalog surgery).
	// Emitted before table diffs so that newly-added enum labels are available if a table
	// ALTER (e.g. ALTER COLUMN SET DEFAULT) references them in the same migration.
	for _, name := range sortedKeys(new.Enums) {
		oldE, exists := old.Enums[name]
		if !exists {
			continue // handled above
		}
		newE := new.Enums[name]
		if addedVals := enumAddedValues(oldE, newE); len(addedVals) > 0 {
			o, n := *oldE, *newE
			changes = append(changes, Change{
				Kind:      ChangeAlterEnum,
				TableName: name,
				OldEnum:   &o,
				NewEnum:   &n,
			})
		}
	}

	// Phase 5: tables present in both → diff columns and constraints.
	// Also handle tables that were renamed: diff the renamed table's contents.
	for _, name := range newNames {
		if _, exists := old.Tables[name]; exists {
			changes = append(changes, diffTable(name, old.Tables[name], new.Tables[name], renamedFrom)...)
		} else if oldName, isRename := renamedTo[name]; isRename {
			// Renamed table: diff columns and constraints under the new name.
			changes = append(changes, diffTable(name, old.Tables[oldName], new.Tables[name], renamedFrom)...)
		}
	}

	// Phase 6: views present in both — DROP + recreate if SQL differs.
	for _, name := range sortedKeys(new.Views) {
		oldV, exists := old.Views[name]
		if !exists {
			continue // handled above
		}
		newV := new.Views[name]
		if normalizeViewSQL(oldV.SQL) != normalizeViewSQL(newV.SQL) {
			n := *newV
			changes = append(changes,
				Change{Kind: ChangeDropView, TableName: name, View: &ViewSnap{Name: oldV.Name, Schema: oldV.Schema, SQL: oldV.SQL}},
				Change{Kind: ChangeCreateView, TableName: name, View: &n},
			)
		}
	}

	// Phase 7: views in old but not new → DROP VIEW (before tables, views depend on tables).
	for _, name := range sortedKeys(old.Views) {
		if _, exists := new.Views[name]; !exists {
			v := *old.Views[name]
			changes = append(changes, Change{
				Kind:      ChangeDropView,
				TableName: name,
				View:      &v,
			})
		}
	}

	// Phase 8: tables in old but not new → DROP TABLE (unless renamed away).
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

	// Phase 9: enums in old but not new → DROP TYPE (after referencing tables are gone).
	for _, name := range sortedKeys(old.Enums) {
		if _, exists := new.Enums[name]; !exists {
			e := *old.Enums[name]
			changes = append(changes, Change{
				Kind:      ChangeDropEnum,
				TableName: name,
				OldEnum:   &e,
			})
		}
	}

	return changes
}

// sortedKeys returns the keys of the given map sorted alphabetically.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

// enumValueAddition describes a single new label to add to an existing PostgreSQL enum.
type enumValueAddition struct {
	Value  string
	After  string // non-empty: ADD VALUE ... AFTER 'After'
	Before string // non-empty and After=="": ADD VALUE ... BEFORE 'Before' (prepend case)
	// both empty: plain ADD VALUE (append to end)
}

// enumAddedValues returns the labels present in newE but absent in oldE, in the order
// they appear in newE, each paired with an AFTER/BEFORE anchor so that PostgreSQL places
// them at the correct position even when values are inserted in the middle of the ordering.
// It scans newE left-to-right, tracking the nearest preceding existing value as the AFTER
// anchor; for a label that must be inserted before all existing labels it uses the first
// following existing label as a BEFORE anchor instead.
func enumAddedValues(oldE, newE *EnumSnap) []enumValueAddition {
	existing := make(map[string]bool, len(oldE.Values))
	for _, v := range oldE.Values {
		existing[v] = true
	}
	var added []enumValueAddition
	var lastExisting string // last existing label seen while scanning left-to-right
	for i, v := range newE.Values {
		if existing[v] {
			lastExisting = v
			continue
		}
		if lastExisting != "" {
			added = append(added, enumValueAddition{Value: v, After: lastExisting})
		} else {
			// No existing label precedes this position; find the first one that follows.
			var before string
			for _, next := range newE.Values[i+1:] {
				if existing[next] {
					before = next
					break
				}
			}
			added = append(added, enumValueAddition{Value: v, Before: before})
		}
	}
	return added
}

// normalizeViewSQL strips leading/trailing whitespace and trailing semicolons
// to avoid spurious diffs from PostgreSQL's view-definition reformatting.
func normalizeViewSQL(sql string) string {
	return strings.TrimRight(strings.TrimSpace(sql), ";")
}

// diffTable computes column- and constraint-level changes for one table.
// tableRenames maps old table name → new table name (from the outer Diff call)
// and is used to normalize FK target references in old constraints so that a
// rename of a referenced table is not misread as a constraint drop+add.
func diffTable(tableName string, old, new *TableSnap, tableRenames map[string]string) []Change {
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
	// Normalize old constraints so that column/table renames don't produce
	// spurious drop+add cycles. After RENAME COLUMN or RENAME TABLE the database
	// automatically updates constraint column references, so a constraint that
	// only changed due to a rename compares equal to its post-rename form and
	// generates no changes. Without normalization the add-before-drop ordering
	// can fail at runtime because the existing constraint (now referencing the
	// new column name) still holds the same constraint name.
	normalizedOldCons := make([]pg.Constraint, len(old.Constraints))
	for i, oc := range old.Constraints {
		normalizedOldCons[i] = normalizeConstraintRefs(oc, tableName, colRenamedFrom, tableRenames)
	}
	oldCons := constraintMap(normalizedOldCons)
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
	for _, oc := range normalizedOldCons {
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

// normalizeConstraintRefs rewrites constraint column/table references using
// rename maps so that a post-rename old constraint compares equal to the new
// constraint definition. colRenames maps old column name → new column name
// within the current table only; tableRenames maps old table name → new table
// name (used for FK target normalization). currentTable is the new (post-rename)
// name of the table being diffed and is used to scope colRenames to FKColumns
// only for self-referential FKs — applying local column renames to a foreign
// table's referenced columns is incorrect.
func normalizeConstraintRefs(c pg.Constraint, currentTable string, colRenames, tableRenames map[string]string) pg.Constraint {
	if len(colRenames) > 0 {
		c.Columns = applyRenames(c.Columns, colRenames)
		// Apply colRenames to FKColumns only for self-referential FKs. colRenames
		// captures column renames within currentTable; the FKTable in the old
		// constraint uses the pre-rename table name, so check both the current
		// name and any old name that maps to it.
		if c.FKTable == currentTable || tableRenames[c.FKTable] == currentTable {
			c.FKColumns = applyRenames(c.FKColumns, colRenames)
		}
	}
	if len(tableRenames) > 0 {
		if newTable, ok := tableRenames[c.FKTable]; ok {
			c.FKTable = newTable
		}
	}
	return c
}

// applyRenames returns a new slice with any element found in renames replaced
// by its mapped value. Returns the original slice unchanged if no renames apply.
func applyRenames(cols []string, renames map[string]string) []string {
	if len(cols) == 0 || len(renames) == 0 {
		return cols
	}
	needsCopy := false
	for _, col := range cols {
		if _, ok := renames[col]; ok {
			needsCopy = true
			break
		}
	}
	if !needsCopy {
		return cols
	}
	out := make([]string, len(cols))
	for i, col := range cols {
		if newName, ok := renames[col]; ok {
			out[i] = newName
		} else {
			out[i] = col
		}
	}
	return out
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
