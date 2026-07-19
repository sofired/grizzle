package filemigrate

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
)

// MemArtifactStore is a thread-safe in-memory ArtifactStore for tests.
// It does not perform filesystem I/O and requires no cleanup.
type MemArtifactStore struct {
	mu        sync.Mutex
	artifacts map[string]memArtifact // keyed by root+"/"+name
}

type memArtifact struct {
	root         string
	name         string
	migrationSQL []byte
	snapshotJSON []byte
}

// NewMemArtifactStore returns an initialized in-memory ArtifactStore.
func NewMemArtifactStore() *MemArtifactStore {
	return &MemArtifactStore{
		artifacts: make(map[string]memArtifact),
	}
}

var _ ArtifactStore = (*MemArtifactStore)(nil)

// ResolveRoot always succeeds for non-empty dir values.
// RootReadForCheck returns RootAbsent if the root has no artifacts yet.
func (s *MemArtifactStore) ResolveRoot(ctx context.Context, dir string, opts ResolveArtifactRootOptions) (ArtifactRoot, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactRoot{}, err
	}
	if dir == "" {
		return ArtifactRoot{}, newInvalidConfigError("mem_artifact_store.resolve_root")
	}
	s.mu.Lock()
	hasAny := false
	for k := range s.artifacts {
		if len(k) > len(dir)+1 && k[:len(dir)+1] == dir+"/" {
			hasAny = true
			break
		}
	}
	s.mu.Unlock()

	state := RootExisting
	if !hasAny && opts.Mode == RootReadForCheck {
		state = RootAbsent
	}
	if !hasAny && opts.Mode == RootReadForMigrate {
		return ArtifactRoot{}, &Error{
			Code: CodeEmptyMigrationsDir,
			Op:   "mem_artifact_store.resolve_root",
			Path: safeRenderPath(dir),
		}
	}
	if opts.Mode == RootEnsureForWrite {
		state = RootExisting // in-memory store treats ensure as a no-op
	}
	return ArtifactRoot{Configured: dir, RealPath: dir, State: state}, nil
}

// ListArtifacts returns all artifact entries for the given root, sorted by name.
// It enforces both MaxArtifactDirEntries (the raw directory-entry budget that
// FSArtifactStore checks against os.ReadDir output) and MaxArtifacts. The
// in-memory store has no concept of sidecars or staging dirs, so the entry
// count equals the artifact count, but the dir-entry limit is still checked
// to keep mem and FS behavior aligned for tests using either backend.
//
// Check order: MaxArtifactDirEntries fires on the unsorted slice (matching
// the FS store, which checks before sorting), then the slice is sorted, then
// MaxArtifacts fires on the sorted slice so its diagnostics are deterministic.
func (s *MemArtifactStore) ListArtifacts(ctx context.Context, root ArtifactRoot, opts ListArtifactsOptions) ([]ArtifactEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root.State == RootAbsent {
		return nil, nil
	}
	lim := opts.Limits.resolve()
	if err := lim.Validate("mem_artifact_store.list_artifacts"); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []ArtifactEntry
	for _, a := range s.artifacts {
		if a.root != root.RealPath {
			continue
		}
		out = append(out, ArtifactEntry{
			Name: a.name,
			Path: root.RealPath + "/" + a.name,
		})
	}
	if len(out) > lim.MaxArtifactDirEntries {
		return nil, &Error{
			Code:      CodeResourceLimit,
			Op:        "mem_artifact_store.list_artifacts",
			Migration: fmt.Sprintf("dir_entries=%d limit=%d", len(out), lim.MaxArtifactDirEntries),
		}
	}
	// Sort before enforcing the MaxArtifacts limit so the diagnostic count
	// fires on a deterministic ordered set, not map-iteration order.
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > lim.MaxArtifacts {
		return nil, &Error{
			Code:      CodeResourceLimit,
			Op:        "mem_artifact_store.list_artifacts",
			Migration: fmt.Sprintf("artifacts=%d limit=%d", len(out), lim.MaxArtifacts),
		}
	}
	return out, nil
}

// ReadArtifact returns a copy of the stored artifact bytes for the given name.
func (s *MemArtifactStore) ReadArtifact(ctx context.Context, root ArtifactRoot, name string, opts ReadArtifactOptions) (*LoadedArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateArtifactName(name); err != nil {
		return nil, &Error{Code: CodeInvalidMigrationName, Op: "mem_artifact_store.read_artifact", Migration: name, Err: err}
	}
	lim := opts.Limits.resolve()
	if err := lim.Validate("mem_artifact_store.read_artifact"); err != nil {
		return nil, err
	}
	key := root.RealPath + "/" + name
	s.mu.Lock()
	a, ok := s.artifacts[key]
	s.mu.Unlock()
	if !ok {
		return nil, &Error{
			Code:      CodeInvalidPath,
			Op:        "mem_artifact_store.read_artifact",
			Migration: name,
			Err:       fmt.Errorf("artifact not found"),
		}
	}
	if int64(len(a.migrationSQL)) > lim.MaxMigrationSQLBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: "mem_artifact_store.read_artifact", Migration: name}
	}
	if int64(len(a.snapshotJSON)) > lim.MaxSnapshotJSONBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: "mem_artifact_store.read_artifact", Migration: name}
	}
	sql := bytes.Clone(a.migrationSQL)
	if err := validateMigrationSQL(name, sql); err != nil {
		return nil, err
	}
	snap := bytes.Clone(a.snapshotJSON)
	digests := computeArtifactDigest(sql, snap)
	return &LoadedArtifact{
		Name:                 name,
		Dir:                  root.RealPath + "/" + name,
		MigrationSQL:         sql,
		SnapshotJSON:         snap,
		Digests:              digests,
		ManagedIntrospection: hasManagedIntrospectionHeader(sql),
	}, nil
}

// CreateArtifact stores a new artifact in memory. Returns CodeDuplicateMigration
// if an artifact with the same name already exists under root.
func (s *MemArtifactStore) CreateArtifact(ctx context.Context, root ArtifactRoot, artifact NewArtifact, opts CreateArtifactOptions) (*LoadedArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateArtifactName(artifact.Name); err != nil {
		return nil, &Error{Code: CodeInvalidMigrationName, Op: "mem_artifact_store.create_artifact", Migration: artifact.Name, Err: err}
	}
	lim := opts.Limits.resolve()
	if err := lim.Validate("mem_artifact_store.create_artifact"); err != nil {
		return nil, err
	}
	if int64(len(artifact.MigrationSQL)) > lim.MaxMigrationSQLBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: "mem_artifact_store.create_artifact", Migration: artifact.Name}
	}
	if int64(len(artifact.SnapshotJSON)) > lim.MaxSnapshotJSONBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: "mem_artifact_store.create_artifact", Migration: artifact.Name}
	}

	key := root.RealPath + "/" + artifact.Name
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.artifacts[key]; exists {
		return nil, &Error{Code: CodeDuplicateMigration, Op: "mem_artifact_store.create_artifact", Migration: artifact.Name}
	}
	sql := bytes.Clone(artifact.MigrationSQL)
	snap := bytes.Clone(artifact.SnapshotJSON)
	s.artifacts[key] = memArtifact{
		root:         root.RealPath,
		name:         artifact.Name,
		migrationSQL: sql,
		snapshotJSON: snap,
	}
	digests := computeArtifactDigest(sql, snap)
	return &LoadedArtifact{
		Name:                 artifact.Name,
		Dir:                  root.RealPath + "/" + artifact.Name,
		MigrationSQL:         bytes.Clone(sql),
		SnapshotJSON:         bytes.Clone(snap),
		Digests:              digests,
		ManagedIntrospection: hasManagedIntrospectionHeader(sql),
	}, nil
}
