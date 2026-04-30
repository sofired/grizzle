// Package kit provides Grizzle's migration tooling: schema snapshots, diffing,
// SQL generation, and schema push. It is the Go equivalent of Drizzle Kit.
//
// Typical usage (library mode — user writes their own migrate entrypoint):
//
//	snap := kit.FromDefs(schema.Users, schema.Realms)
//	sql  := kit.GenerateCreateSQL(schema.Users, schema.Realms)
//
//	// Push to DB (introspects current state, diffs, applies changes):
//	err := kit.Push(ctx, pool, schema.Users, schema.Realms)
package kit

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	pg "github.com/sofired/grizzle/schema/pg"
)

const snapshotVersion = "1"

// Snapshot is a serializable, point-in-time representation of a database schema.
// It is saved to disk as JSON and used to compute incremental diffs.
type Snapshot struct {
	Version   string                `json:"version"`
	CreatedAt time.Time             `json:"created_at"`
	Tables    map[string]*TableSnap `json:"tables"`          // keyed by qualified table name
	Views     map[string]*ViewSnap  `json:"views,omitempty"` // keyed by qualified view name
	Enums     map[string]*EnumSnap  `json:"enums,omitempty"` // keyed by qualified type name
}

// TableSnap is the snapshot of a single table.
type TableSnap struct {
	Name        string          `json:"name"`
	Schema      string          `json:"schema,omitempty"`
	Columns     []pg.ColumnDef  `json:"columns"`
	Constraints []pg.Constraint `json:"constraints,omitempty"`
	// PreviousName is intentionally excluded from JSON snapshots — it is only
	// meaningful as a schema definition annotation for the current migration step
	// and must not persist across snapshot saves. If it were persisted, a future
	// table that happens to share the old name would trigger a spurious RENAME
	// instead of a CREATE.
	PreviousName string `json:"-"`
}

// qualifyName returns "schema.name" for non-public schemas, otherwise just "name".
func qualifyName(schema, name string) string {
	if schema != "" && schema != "public" {
		return schema + "." + name
	}
	return name
}

// QualifiedName returns the schema-qualified name used as the map key.
func (t *TableSnap) QualifiedName() string {
	if t.Schema != "" {
		return t.Schema + "." + t.Name
	}
	return t.Name
}

// ViewSnap is the snapshot of a single view.
type ViewSnap struct {
	Name   string `json:"name"`
	Schema string `json:"schema,omitempty"`
	// SQL is the raw SELECT body as declared in the schema definition; not pre-normalized.
	// normalizeViewSQL is applied at diff time to avoid spurious diffs from PostgreSQL reformatting.
	SQL string `json:"sql"`
}

// QualifiedName returns the schema-qualified name used as the map key.
func (v *ViewSnap) QualifiedName() string {
	return qualifyName(v.Schema, v.Name)
}

// EnumSnap is the snapshot of a single PostgreSQL enum type.
type EnumSnap struct {
	Name   string `json:"name"`
	Schema string `json:"schema,omitempty"`
	// Values is the ordered list of enum labels. Must be non-empty;
	// pg.CreateEnum and pg.SchemaCreateEnum enforce this invariant.
	Values []string `json:"values"`
}

// QualifiedName returns the schema-qualified name used as the map key.
func (e *EnumSnap) QualifiedName() string {
	return qualifyName(e.Schema, e.Name)
}

// FromDefs builds a Snapshot from a set of dialect-agnostic TableDefiner values.
// This is the normal way to capture your schema definition when working with tables only.
// To include views and enums, use FromSchema instead.
// It accepts tables from any dialect (pg, mysql, sqlite).
func FromDefs(tables ...pg.TableDefiner) Snapshot {
	snap := Snapshot{
		Version:   snapshotVersion,
		CreatedAt: time.Now().UTC(),
		Tables:    make(map[string]*TableSnap, len(tables)),
		Views:     make(map[string]*ViewSnap),
		Enums:     make(map[string]*EnumSnap),
	}
	for _, td := range tables {
		t := td.Def()
		ts := &TableSnap{
			Name:         t.Name,
			Schema:       t.Schema,
			Columns:      t.Columns,
			Constraints:  t.Constraints,
			PreviousName: t.PreviousName,
		}
		snap.Tables[ts.QualifiedName()] = ts
	}
	return snap
}

// SchemaObjects holds all the schema object types that can be passed to FromSchema.
type SchemaObjects struct {
	// Tables holds dialect-agnostic table definitions (pg, mysql, or sqlite).
	Tables []pg.TableDefiner
	// Views holds PostgreSQL view definitions.
	// Non-PostgreSQL SQL generators emit stub comments instead of real DDL for views.
	Views []*pg.ViewDef
	// Enums holds PostgreSQL named enum type definitions.
	// Non-PostgreSQL SQL generators emit stub comments instead of real DDL for enum types.
	Enums []*pg.EnumDef
}

// FromSchema builds a Snapshot from tables, views, and enum types together.
// Use this when your schema includes PostgreSQL-specific objects beyond tables.
//
//	snap := kit.FromSchema(kit.SchemaObjects{
//	    Tables: []pg.TableDefiner{schema.Users, schema.Realms},
//	    Views:  []*pg.ViewDef{schema.ActiveUsers},
//	    Enums:  []*pg.EnumDef{schema.Status},
//	})
func FromSchema(objs SchemaObjects) Snapshot {
	snap := Snapshot{
		Version:   snapshotVersion,
		CreatedAt: time.Now().UTC(),
		Tables:    make(map[string]*TableSnap, len(objs.Tables)),
		Views:     make(map[string]*ViewSnap, len(objs.Views)),
		Enums:     make(map[string]*EnumSnap, len(objs.Enums)),
	}
	for _, td := range objs.Tables {
		t := td.Def()
		ts := &TableSnap{
			Name:         t.Name,
			Schema:       t.Schema,
			Columns:      t.Columns,
			Constraints:  t.Constraints,
			PreviousName: t.PreviousName,
		}
		snap.Tables[ts.QualifiedName()] = ts
	}
	for _, v := range objs.Views {
		vs := &ViewSnap{Name: v.Name, Schema: v.Schema, SQL: v.SQL}
		snap.Views[vs.QualifiedName()] = vs
	}
	for _, e := range objs.Enums {
		vals := make([]string, len(e.Values))
		copy(vals, e.Values)
		es := &EnumSnap{Name: e.Name, Schema: e.Schema, Values: vals}
		snap.Enums[es.QualifiedName()] = es
	}
	return snap
}

// SaveJSON writes a snapshot to a JSON file.
func SaveJSON(snap Snapshot, path string) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write snapshot %q: %w", path, err)
	}
	return nil
}

// LoadJSON reads a snapshot from a JSON file.
func LoadJSON(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read snapshot %q: %w", path, err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("unmarshal snapshot: %w", err)
	}
	if snap.Tables == nil {
		snap.Tables = make(map[string]*TableSnap)
	}
	if snap.Views == nil {
		snap.Views = make(map[string]*ViewSnap)
	}
	if snap.Enums == nil {
		snap.Enums = make(map[string]*EnumSnap)
	}
	return snap, nil
}

// EmptySnapshot returns an empty snapshot representing a blank database.
func EmptySnapshot() Snapshot {
	return Snapshot{
		Version:   snapshotVersion,
		CreatedAt: time.Now().UTC(),
		Tables:    make(map[string]*TableSnap),
		Views:     make(map[string]*ViewSnap),
		Enums:     make(map[string]*EnumSnap),
	}
}
