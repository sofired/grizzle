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
// Ordering is deterministic and safe for direct application:
//
//  1. Create new enums — types may be referenced by table columns.
//  2. Create new tables — FK targets must exist before referencing tables.
//  3. Create new views — views SELECT FROM tables, which must already exist.
//  4. Alter existing tables (columns + constraints, including drops).
//  5. Alter existing enums (add values only; value removal is unsupported).
//  6. Replace changed views (DROP then CREATE OR REPLACE in sequence).
//  7. Drop removed views — before tables, since views depend on tables.
//  8. Drop removed tables.
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

	// Phase 2: new tables not in old → CREATE TABLE.
	for _, name := range newNames {
		if _, exists := old.Tables[name]; !exists {
			changes = append(changes, Change{
				Kind:      ChangeCreateTable,
				TableName: name,
			})
			// Individual column/constraint adds are implied by CREATE TABLE.
		}
	}

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

	// Phase 4: tables present in both → diff columns and constraints.
	for _, name := range newNames {
		oldT, exists := old.Tables[name]
		if !exists {
			continue // handled above
		}
		changes = append(changes, diffTable(name, oldT, new.Tables[name])...)
	}

	// Phase 5: enums in both → diff values (additions only; removal requires pg_catalog surgery).
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

	// Phase 8: tables in old but not new → DROP TABLE.
	for _, name := range oldNames {
		if _, exists := new.Tables[name]; !exists {
			changes = append(changes, Change{
				Kind:      ChangeDropTable,
				TableName: name,
			})
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

// enumAddedValues returns values present in newE but absent in oldE,
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

// normalizeViewSQL strips leading/trailing whitespace and trailing semicolons
// to avoid spurious diffs from PostgreSQL's view-definition reformatting.
func normalizeViewSQL(sql string) string {
	return strings.TrimRight(strings.TrimSpace(sql), ";")
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
