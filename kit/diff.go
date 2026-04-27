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
// table name. Within a single table, changes are emitted in slice order
// (added/modified columns first, then dropped columns, then added/dropped
// constraints). No secondary sort by change kind or column name is applied.
func Diff(old, new Snapshot) []Change {
	var changes []Change

	// Collect sorted table names to ensure deterministic output.
	newNames := sortedTableNames(new.Tables)
	oldNames := sortedTableNames(old.Tables)

	// Phase 1: new tables not in old → CREATE TABLE in FK-dependency order so
	// referenced tables are always created before their dependants.
	var createdNames []string
	for _, name := range newNames {
		if _, exists := old.Tables[name]; !exists {
			createdNames = append(createdNames, name)
		}
	}
	for _, name := range orderNewTables(createdNames, new.Tables) {
		changes = append(changes, Change{
			Kind:      ChangeCreateTable,
			TableName: name,
		})
		// Individual column and constraint adds are implied by CREATE TABLE;
		// we don't emit separate ADD COLUMN / ADD INDEX changes for new tables.
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

	// Build a set of inline FK signatures from the target's column References.
	// Single-column FKs defined via ColumnDef.References don't appear in
	// Constraints, so we track them here to avoid emitting spurious
	// ChangeDropConstraint when the live DB has a named FK for the same column.
	inlineRefs := make(map[string]struct{})
	for _, col := range new.Columns {
		if col.References != nil {
			inlineRefs[col.Name+":"+col.References.Table+":"+col.References.Column] = struct{}{}
		}
	}

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
			// Don't drop a single-column FK that is represented as an inline
			// ColumnDef.References in the target schema — the FK is kept alive
			// by the column definition and doesn't need to be managed here.
			if oc.Kind == pg.KindForeignKey && len(oc.Columns) == 1 && len(oc.FKColumns) == 1 {
				if _, matched := inlineRefs[oc.Columns[0]+":"+oc.FKTable+":"+oc.FKColumns[0]]; matched {
					continue
				}
			}
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
		// Treat empty string and NO ACTION as equivalent: PostgreSQL reports
		// "NO ACTION" for FKs created without an explicit action, but user
		// schemas may leave FKOnDelete/FKOnUpdate unset (zero value "").
		if normFKAction(a.FKOnDelete) != normFKAction(b.FKOnDelete) ||
			normFKAction(a.FKOnUpdate) != normFKAction(b.FKOnUpdate) {
			return false
		}
	}
	return true
}

// normFKAction normalises a foreign-key action for comparison, treating the
// zero value ("") as equivalent to pg.FKActionNoAction.
func normFKAction(a pg.FKAction) pg.FKAction {
	if a == "" {
		return pg.FKActionNoAction
	}
	return a
}

// orderNewTables returns names in dependency order so that, when creating new
// tables, a table that other new tables reference via FK is always emitted
// before the referencing table. Tables with no FK dependencies on other new
// tables come first (sorted alphabetically among themselves). Cycles fall
// back to input order and the DB will surface the constraint error.
func orderNewTables(names []string, tables map[string]*TableSnap) []string {
	if len(names) <= 1 {
		return names
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	// deps[n] = new tables that n depends on (must exist before n is created).
	deps := make(map[string][]string, len(names))
	for _, n := range names {
		t := tables[n]
		if t == nil {
			continue
		}
		seen := make(map[string]bool)
		add := func(ref string) {
			if nameSet[ref] && ref != n && !seen[ref] {
				deps[n] = append(deps[n], ref)
				seen[ref] = true
			}
		}
		for _, col := range t.Columns {
			if col.References != nil {
				add(col.References.Table)
			}
		}
		for _, c := range t.Constraints {
			if c.Kind == pg.KindForeignKey {
				add(c.FKTable)
			}
		}
	}

	// Kahn's algorithm: build in-degree and reverse adjacency list.
	inDegree := make(map[string]int, len(names))
	adj := make(map[string][]string, len(names))
	for _, n := range names {
		inDegree[n] = 0
	}
	for _, n := range names {
		for _, dep := range deps[n] {
			inDegree[n]++
			adj[dep] = append(adj[dep], n)
		}
	}
	for k := range adj {
		sort.Strings(adj[k])
	}

	// Seed with zero-degree nodes in their original (alphabetical) order.
	queue := make([]string, 0, len(names))
	for _, n := range names {
		if inDegree[n] == 0 {
			queue = append(queue, n)
		}
	}

	result := make([]string, 0, len(names))
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		result = append(result, n)
		for _, dep := range adj[n] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				i := sort.SearchStrings(queue, dep)
				queue = append(queue, "")
				copy(queue[i+1:], queue[i:])
				queue[i] = dep
			}
		}
	}

	// Cycle fallback: append remaining nodes in original input order.
	if len(result) < len(names) {
		inResult := make(map[string]bool, len(result))
		for _, r := range result {
			inResult[r] = true
		}
		for _, n := range names {
			if !inResult[n] {
				result = append(result, n)
			}
		}
	}

	return result
}
