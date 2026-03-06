// Package kit provides G-rizzle's migration tooling: schema snapshots, diffing,
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

	pg "github.com/grizzle-orm/grizzle/schema/pg"
)

const snapshotVersion = "1"

// Snapshot is a serializable, point-in-time representation of a database schema.
// It is saved to disk as JSON and used to compute incremental diffs.
type Snapshot struct {
	Version   string                `json:"version"`
	CreatedAt time.Time             `json:"created_at"`
	Tables    map[string]*TableSnap `json:"tables"` // keyed by qualified table name
}

// TableSnap is the snapshot of a single table.
type TableSnap struct {
	Name        string           `json:"name"`
	Schema      string           `json:"schema,omitempty"`
	Columns     []pg.ColumnDef   `json:"columns"`
	Constraints []pg.Constraint  `json:"constraints,omitempty"`
}

// QualifiedName returns the schema-qualified name used as the map key.
func (t *TableSnap) QualifiedName() string {
	if t.Schema != "" {
		return t.Schema + "." + t.Name
	}
	return t.Name
}

// FromDefs builds a Snapshot from a set of *pg.TableDef values.
// This is the normal way to capture your schema definition.
func FromDefs(tables ...*pg.TableDef) Snapshot {
	snap := Snapshot{
		Version:   snapshotVersion,
		CreatedAt: time.Now().UTC(),
		Tables:    make(map[string]*TableSnap, len(tables)),
	}
	for _, t := range tables {
		ts := &TableSnap{
			Name:        t.Name,
			Schema:      t.Schema,
			Columns:     t.Columns,
			Constraints: t.Constraints,
		}
		snap.Tables[ts.QualifiedName()] = ts
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
	return snap, nil
}

// EmptySnapshot returns an empty snapshot representing a blank database.
func EmptySnapshot() Snapshot {
	return Snapshot{
		Version:   snapshotVersion,
		CreatedAt: time.Now().UTC(),
		Tables:    make(map[string]*TableSnap),
	}
}
