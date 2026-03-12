package kit

import (
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

	// View change kinds.
	ChangeCreateView ChangeKind = "create_view"
	ChangeDropView   ChangeKind = "drop_view"

	// Enum change kinds.
	ChangeCreateEnum ChangeKind = "create_enum"
	ChangeAlterEnum  ChangeKind = "alter_enum" // add values to an existing enum
	ChangeDropEnum   ChangeKind = "drop_enum"
)

// Change represents a single schema mutation — the unit that SQL generation works from.
type Change struct {
	Kind      ChangeKind
	TableName string // qualified name (also used as object name for views/enums)

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
// Ordering is deterministic:
//  1. Create new enums (types needed by tables/columns).
//  2. Create new views.
//  3. Create new tables (so FK references resolve).
//  4. Alter existing tables (columns first, then constraints).
//  5. Alter existing enums (add values only).
//  6. Replace changed views (drop + recreate).
//  7. Drop removed constraints.
//  8. Drop removed tables (in reverse to respect FKs — caller may need to reorder).
//  9. Drop removed views.
// 10. Drop removed enums.
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

	// Phase 1: new enums not in old → CREATE TYPE ... AS ENUM.
	for name, newE := range new.Enums {
		if _, exists := old.Enums[name]; !exists {
			e := *newE
			changes = append(changes, Change{
				Kind:      ChangeCreateEnum,
				TableName: name,
				NewEnum:   &e,
			})
		}
	}

	// Phase 2: new views not in old → CREATE VIEW.
	for name, newV := range new.Views {
		if _, exists := old.Views[name]; !exists {
			v := *newV
			changes = append(changes, Change{
				Kind:      ChangeCreateView,
				TableName: name,
				View:      &v,
			})
		}
	}

	// Phase 3: new tables not in old → CREATE TABLE.
	for name := range new.Tables {
		if _, exists := old.Tables[name]; !exists {
			changes = append(changes, Change{
				Kind:      ChangeCreateTable,
				TableName: name,
			})
			// Individual column and constraint adds are implied by CREATE TABLE;
			// we don't emit separate ADD COLUMN / ADD INDEX changes for new tables.
		}
	}

	// Phase 4: tables present in both → diff columns and constraints.
	for name, newT := range new.Tables {
		oldT, exists := old.Tables[name]
		if !exists {
			continue // handled above
		}
		changes = append(changes, diffTable(name, oldT, newT)...)
	}

	// Phase 5: enums in both → diff values (only additions are safe without USING).
	for name, newE := range new.Enums {
		oldE, exists := old.Enums[name]
		if !exists {
			continue // handled above
		}
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

	// Phase 6: views present in both — drop and recreate if SQL differs.
	for name, newV := range new.Views {
		oldV, exists := old.Views[name]
		if !exists {
			continue // handled above
		}
		if normalizeViewSQL(oldV.SQL) != normalizeViewSQL(newV.SQL) {
			v := *newV
			changes = append(changes,
				Change{Kind: ChangeDropView, TableName: name, View: &ViewSnap{Name: oldV.Name, Schema: oldV.Schema, SQL: oldV.SQL}},
				Change{Kind: ChangeCreateView, TableName: name, View: &v},
			)
		}
	}

	// Phase 7: tables in old but not new → DROP TABLE.
	for name := range old.Tables {
		if _, exists := new.Tables[name]; !exists {
			changes = append(changes, Change{
				Kind:      ChangeDropTable,
				TableName: name,
			})
		}
	}

	// Phase 8: views in old but not new → DROP VIEW.
	for name, oldV := range old.Views {
		if _, exists := new.Views[name]; !exists {
			v := *oldV
			changes = append(changes, Change{
				Kind:      ChangeDropView,
				TableName: name,
				View:      &v,
			})
		}
	}

	// Phase 9: enums in old but not new → DROP TYPE.
	for name, oldE := range old.Enums {
		if _, exists := new.Enums[name]; !exists {
			e := *oldE
			changes = append(changes, Change{
				Kind:      ChangeDropEnum,
				TableName: name,
				OldEnum:   &e,
			})
		}
	}

	return changes
}

// enumAddedValues returns the values that are in newE but not in oldE,
// in the order they appear in newE.
func enumAddedValues(oldE, newE *EnumSnap) []string {
	existing := make(map[string]bool, len(oldE.Values))
	for _, v := range oldE.Values {
		existing[v] = true
	}
	var added []string
	for _, v := range newE.Values {
		if !existing[v] {
			added = append(added, v)
		}
	}
	return added
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

// normalizeViewSQL trims whitespace and trailing semicolons for comparison
// purposes. This avoids spurious diffs caused by formatting differences
// between the stored definition and the live introspected definition.
func normalizeViewSQL(sql string) string {
	return strings.TrimRight(strings.TrimSpace(sql), ";")
}
