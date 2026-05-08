package filemigrate_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/sofired/grizzle/kit/filemigrate"
)

// --- shared test helpers ---

func mustCreateArtifact(t *testing.T, store filemigrate.ArtifactStore, root filemigrate.ArtifactRoot, name string, sql, snap []byte) *filemigrate.LoadedArtifact {
	t.Helper()
	a, err := store.CreateArtifact(t.Context(), root, filemigrate.NewArtifact{
		Name:         name,
		MigrationSQL: sql,
		SnapshotJSON: snap,
	}, filemigrate.CreateArtifactOptions{})
	if err != nil {
		t.Fatalf("CreateArtifact(%q): %v", name, err)
	}
	return a
}

func mustResolveRoot(t *testing.T, store filemigrate.ArtifactStore, dir string, mode filemigrate.ArtifactRootMode) filemigrate.ArtifactRoot {
	t.Helper()
	root, err := store.ResolveRoot(t.Context(), dir, filemigrate.ResolveArtifactRootOptions{Mode: mode})
	if err != nil {
		t.Fatalf("ResolveRoot(%q): %v", dir, err)
	}
	return root
}

// --- ArtifactDigest ---

func TestArtifactDigestKnownValue(t *testing.T) {
	// Spot-check that individual digests are correct SHA-256 values.
	sql := []byte("CREATE TABLE t (id INT);")
	snap := []byte(`{"version":"1"}`)
	a := filemigrate.NewArtifact{Name: "20240101000000_init", MigrationSQL: sql, SnapshotJSON: snap}
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/r", filemigrate.RootEnsureForWrite)
	loaded := mustCreateArtifact(t, store, root, a.Name, sql, snap)

	wantSQL := sha256.Sum256(sql)
	if loaded.Digests.MigrationSQLSHA256 != filemigrate.Digest(wantSQL) {
		t.Errorf("MigrationSQLSHA256 mismatch: got %s want %s",
			hex.EncodeToString(loaded.Digests.MigrationSQLSHA256[:]),
			hex.EncodeToString(wantSQL[:]))
	}
	wantSnap := sha256.Sum256(snap)
	if loaded.Digests.SnapshotJSONSHA256 != filemigrate.Digest(wantSnap) {
		t.Errorf("SnapshotJSONSHA256 mismatch: got %s want %s",
			hex.EncodeToString(loaded.Digests.SnapshotJSONSHA256[:]),
			hex.EncodeToString(wantSnap[:]))
	}
	// CombinedSHA256 must be non-zero.
	var zero filemigrate.Digest
	if loaded.Digests.CombinedSHA256 == zero {
		t.Error("CombinedSHA256 must not be zero")
	}
}

// --- MemArtifactStore ---

func TestMemArtifactStore_EmptyRoot(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/migrations", filemigrate.RootReadForCheck)
	if root.State != filemigrate.RootAbsent {
		t.Errorf("expected RootAbsent for empty store, got %q", root.State)
	}
	entries, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{})
	if err != nil {
		t.Fatalf("ListArtifacts on absent root: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for absent root, got %d", len(entries))
	}
}

func TestMemArtifactStore_CreateAndRead(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/migrations", filemigrate.RootEnsureForWrite)

	sql := []byte("CREATE TABLE users (id INT);")
	snap := []byte(`{"version":"1","dialect":"postgresql"}`)

	loaded := mustCreateArtifact(t, store, root, "20240101000000_users", sql, snap)

	if loaded.Name != "20240101000000_users" {
		t.Errorf("unexpected Name: %q", loaded.Name)
	}
	if !bytes.Equal(loaded.MigrationSQL, sql) {
		t.Error("MigrationSQL mismatch")
	}
	if !bytes.Equal(loaded.SnapshotJSON, snap) {
		t.Error("SnapshotJSON mismatch")
	}

	// ReadArtifact should return a copy with matching content.
	got, err := store.ReadArtifact(t.Context(), root, "20240101000000_users", filemigrate.ReadArtifactOptions{})
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if !bytes.Equal(got.MigrationSQL, sql) {
		t.Error("ReadArtifact MigrationSQL mismatch")
	}
}

func TestMemArtifactStore_ReturnedSlicesAreIndependent(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/migrations", filemigrate.RootEnsureForWrite)

	sql := []byte("SELECT 1;")
	snap := []byte(`{}`)
	loaded := mustCreateArtifact(t, store, root, "20240102000000_a", sql, snap)

	// Mutate the returned slice.
	loaded.MigrationSQL[0] = 'X'

	// The store should still return the original bytes.
	got, err := store.ReadArtifact(t.Context(), root, "20240102000000_a", filemigrate.ReadArtifactOptions{})
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if got.MigrationSQL[0] == 'X' {
		t.Error("store returned a reference to its internal slice (not a copy)")
	}
}

func TestMemArtifactStore_DuplicateRejected(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/migrations", filemigrate.RootEnsureForWrite)
	mustCreateArtifact(t, store, root, "20240101000000_init", []byte("SQL"), []byte("{}"))

	_, err := store.CreateArtifact(t.Context(), root, filemigrate.NewArtifact{
		Name:         "20240101000000_init",
		MigrationSQL: []byte("SQL"),
		SnapshotJSON: []byte("{}"),
	}, filemigrate.CreateArtifactOptions{})
	if err == nil {
		t.Fatal("expected error for duplicate migration name")
	}
	if !errors.Is(err, filemigrate.ErrDuplicateMigration) {
		t.Errorf("expected ErrDuplicateMigration, got %v", err)
	}
}

func TestMemArtifactStore_ListSortedLexicographically(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)
	mustCreateArtifact(t, store, root, "20240103_c", []byte("c"), []byte("{}"))
	mustCreateArtifact(t, store, root, "20240101_a", []byte("a"), []byte("{}"))
	mustCreateArtifact(t, store, root, "20240102_b", []byte("b"), []byte("{}"))

	entries, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{})
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	want := []string{"20240101_a", "20240102_b", "20240103_c"}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("entry[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestMemArtifactStore_InvalidNameRejected(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)

	badNames := []string{
		"",
		"../../etc/passwd",
		"/absolute",
		"a/b",
		".",
		"..",
		".hidden",
	}
	for _, name := range badNames {
		_, err := store.CreateArtifact(t.Context(), root, filemigrate.NewArtifact{
			Name:         name,
			MigrationSQL: []byte("SQL"),
			SnapshotJSON: []byte("{}"),
		}, filemigrate.CreateArtifactOptions{})
		if err == nil {
			t.Errorf("CreateArtifact(%q): expected error, got nil", name)
		}
		if !errors.Is(err, filemigrate.ErrInvalidMigrationName) {
			t.Errorf("CreateArtifact(%q): expected ErrInvalidMigrationName, got %v", name, err)
		}
	}
}

func TestMemArtifactStore_ResourceLimitOnList(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)
	mustCreateArtifact(t, store, root, "20240101_a", []byte("a"), []byte("{}"))
	mustCreateArtifact(t, store, root, "20240102_b", []byte("b"), []byte("{}"))

	_, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{
		Limits: filemigrate.ResourceLimits{MaxArtifacts: 1},
	})
	if err == nil {
		t.Fatal("expected resource limit error")
	}
	if !errors.Is(err, filemigrate.ErrResourceLimit) {
		t.Errorf("expected ErrResourceLimit, got %v", err)
	}
}

func TestMemArtifactStore_ReadMissingArtifact(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootReadForCheck)
	_, err := store.ReadArtifact(t.Context(), root, "nonexistent", filemigrate.ReadArtifactOptions{})
	if err == nil {
		t.Fatal("expected error for nonexistent artifact")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

func TestMemArtifactStore_ReadMigrateFailsOnAbsent(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	_, err := store.ResolveRoot(t.Context(), "/missing", filemigrate.ResolveArtifactRootOptions{
		Mode: filemigrate.RootReadForMigrate,
	})
	if err == nil {
		t.Fatal("expected error for absent root with RootReadForMigrate")
	}
	if !errors.Is(err, filemigrate.ErrEmptyMigrationsDir) {
		t.Errorf("expected ErrEmptyMigrationsDir, got %v", err)
	}
}

// --- FSArtifactStore ---

func TestFSArtifactStore_CreateReadRoundtrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootEnsureForWrite)

	sql := []byte("CREATE TABLE items (id BIGINT PRIMARY KEY);")
	snap := []byte(`{"version":"1"}`)
	loaded := mustCreateArtifact(t, store, root, "20240101000000_items", sql, snap)

	if !bytes.Equal(loaded.MigrationSQL, sql) {
		t.Error("MigrationSQL mismatch after create")
	}

	got, err := store.ReadArtifact(t.Context(), root, "20240101000000_items", filemigrate.ReadArtifactOptions{})
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if !bytes.Equal(got.MigrationSQL, sql) {
		t.Error("MigrationSQL mismatch after read")
	}
	if !bytes.Equal(got.SnapshotJSON, snap) {
		t.Error("SnapshotJSON mismatch after read")
	}
}

func TestFSArtifactStore_SymlinkRootRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	if err := os.Mkdir(real, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	store := filemigrate.NewFSArtifactStore()
	_, err := store.ResolveRoot(t.Context(), link, filemigrate.ResolveArtifactRootOptions{
		Mode: filemigrate.RootReadForCheck,
	})
	if err == nil {
		t.Fatal("expected error for symlinked root")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

func TestFSArtifactStore_SymlinkArtifactFileRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootEnsureForWrite)

	// Manually create the artifact directory and make migration.sql a symlink.
	artDir := filepath.Join(dir, "20240101000000_sym")
	if err := os.Mkdir(artDir, 0o750); err != nil {
		t.Fatal(err)
	}
	realSQL := filepath.Join(dir, "real.sql")
	if err := os.WriteFile(realSQL, []byte("SELECT 1;"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realSQL, filepath.Join(artDir, "migration.sql")); err != nil {
		t.Skip("symlinks not supported on this platform")
	}
	if err := os.WriteFile(filepath.Join(artDir, "snapshot.json"), []byte(`{}`), 0o640); err != nil {
		t.Fatal(err)
	}

	_, err := store.ReadArtifact(context.Background(), root, "20240101000000_sym", filemigrate.ReadArtifactOptions{})
	if err == nil {
		t.Fatal("expected error for symlinked artifact file")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

func TestFSArtifactStore_SymlinkedArtifactDirRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	// Build a fully-formed migration directory outside the configured root.
	outside := t.TempDir()
	realArt := filepath.Join(outside, "realart")
	if err := os.Mkdir(realArt, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realArt, "migration.sql"), []byte("SELECT 1;"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realArt, "snapshot.json"), []byte(`{}`), 0o640); err != nil {
		t.Fatal(err)
	}
	// Symlink the artifact dir name under the configured root to the outside dir.
	linkArt := filepath.Join(dir, "20240101000000_link")
	if err := os.Symlink(realArt, linkArt); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootReadForCheck)

	_, err := store.ReadArtifact(t.Context(), root, "20240101000000_link", filemigrate.ReadArtifactOptions{})
	if err == nil {
		t.Fatal("expected error for symlinked artifact directory")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}

	// ListArtifacts must also reject the symlinked directory at discovery time.
	_, listErr := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{})
	if listErr == nil {
		t.Fatal("expected ListArtifacts error for symlinked artifact directory")
	}
	if !errors.Is(listErr, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath from ListArtifacts, got %v", listErr)
	}
}

func TestFSArtifactStore_ReadRejectsRegularFileAtArtifactPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	// Create a regular file (not a directory) at the would-be artifact path.
	if err := os.WriteFile(filepath.Join(dir, "20240101000000_file"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootReadForCheck)
	_, err := store.ReadArtifact(t.Context(), root, "20240101000000_file", filemigrate.ReadArtifactOptions{})
	if err == nil {
		t.Fatal("expected error when artifact path is a regular file")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

func TestFSArtifactStore_ListRejectsStrayFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	store := filemigrate.NewFSArtifactStore()

	cases := []struct {
		name     string
		filename string
	}{
		{"sql file", "001.sql"},
		{"migrations bundle js", "migrations.js"},
		{"migrations bundle ts", "migrations.ts"},
		{"migrations bundle mjs", "migrations.mjs"},
		{"migrations bundle cjs", "migrations.cjs"},
		{"unknown regular file", "extra.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.filename), []byte("x"), 0o640); err != nil {
				t.Fatal(err)
			}
			root := mustResolveRoot(t, store, dir, filemigrate.RootReadForCheck)
			_, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{})
			if err == nil {
				t.Fatalf("expected error for stray %s", tc.filename)
			}
			if !errors.Is(err, filemigrate.ErrUnsupportedArtifactFormat) {
				t.Errorf("expected ErrUnsupportedArtifactFormat for %s, got %v", tc.filename, err)
			}
		})
	}
}

func TestFSArtifactStore_ListRejectsNonRegularSidecar(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifoPath, 0o640); err != nil {
		// Mkfifo is not portable to Windows; skip if unsupported.
		t.Skipf("mkfifo not supported on this platform: %v", err)
	}
	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootReadForCheck)
	_, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{})
	if err == nil {
		t.Fatal("expected error for non-regular sidecar (FIFO)")
	}
	// Spec: non-regular non-directory entries fail with invalid_path,
	// distinct from the unsupported_artifact_format used for unknown regular files.
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
	if errors.Is(err, filemigrate.ErrUnsupportedArtifactFormat) {
		t.Errorf("FIFO must not be classified as ErrUnsupportedArtifactFormat: %v", err)
	}
}

func TestFSArtifactStore_ListIgnoresAllowedSidecars(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	for _, name := range []string{".gitkeep", ".DS_Store", "README", "README.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootEnsureForWrite)
	mustCreateArtifact(t, store, root, "20240101000000_x", []byte("SELECT 1;"), []byte(`{}`))

	entries, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{})
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "20240101000000_x" {
		t.Errorf("unexpected entries with allowed sidecars: %+v", entries)
	}
}

func TestFSArtifactStore_ListIgnoresReservedStagingDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".grizzle-staging-abc"), 0o750); err != nil {
		t.Fatal(err)
	}
	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootReadForCheck)
	entries, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{})
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected reserved staging dir to be skipped, got %+v", entries)
	}
}

// TestFSArtifactStore_SymlinkedParentRootRejected covers the case where the
// configured migrations root contains a symlinked parent component (e.g.
// `<base>/link/migrations` where `link -> /outside`). A bare Lstat on the
// final component would silently follow that parent symlink and let
// EvalSymlinks accept the outside path as RealPath; ResolveRoot must reject
// the configured root before any file work runs. Verified for all three modes
// (read-for-check, read-for-migrate, ensure-for-write).
func TestFSArtifactStore_SymlinkedParentRootRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}

	store := filemigrate.NewFSArtifactStore()
	modes := []filemigrate.ArtifactRootMode{
		filemigrate.RootReadForCheck,
		filemigrate.RootReadForMigrate,
		filemigrate.RootEnsureForWrite,
	}
	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			base := t.TempDir()
			outside := t.TempDir()
			// Create a parent directory and a real "migrations" inside it,
			// outside the configured base.
			realParent := filepath.Join(outside, "real")
			if err := os.MkdirAll(filepath.Join(realParent, "migrations"), 0o750); err != nil {
				t.Fatal(err)
			}
			// Symlink "link" under base to the outside parent.
			if err := os.Symlink(realParent, filepath.Join(base, "link")); err != nil {
				t.Skip("symlinks not supported on this platform")
			}
			_, err := store.ResolveRoot(t.Context(), filepath.Join(base, "link", "migrations"),
				filemigrate.ResolveArtifactRootOptions{Mode: mode})
			if err == nil {
				t.Fatalf("mode=%s: expected error for symlinked parent", mode)
			}
			if !errors.Is(err, filemigrate.ErrInvalidPath) {
				t.Errorf("mode=%s: expected ErrInvalidPath, got %v", mode, err)
			}
		})
	}
}

// TestFSArtifactStore_NegativeLimitRejected verifies that public store methods
// fail closed with ErrInvalidConfig when a negative ResourceLimits field is
// supplied, before any filesystem work runs.
func TestFSArtifactStore_NegativeLimitRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootEnsureForWrite)
	mustCreateArtifact(t, store, root, "20240101000000_x", []byte("SELECT 1;"), []byte("{}"))

	// ListArtifacts
	_, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{
		Limits: filemigrate.ResourceLimits{MaxArtifactDirEntries: -1},
	})
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("ListArtifacts: expected ErrInvalidConfig, got %v", err)
	}

	// ReadArtifact
	_, err = store.ReadArtifact(t.Context(), root, "20240101000000_x", filemigrate.ReadArtifactOptions{
		Limits: filemigrate.ResourceLimits{MaxMigrationSQLBytes: -1},
	})
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("ReadArtifact: expected ErrInvalidConfig, got %v", err)
	}

	// CreateArtifact
	_, err = store.CreateArtifact(t.Context(), root, filemigrate.NewArtifact{
		Name:         "20240102000000_y",
		MigrationSQL: []byte("SELECT 1;"),
		SnapshotJSON: []byte("{}"),
	}, filemigrate.CreateArtifactOptions{
		Limits: filemigrate.ResourceLimits{MaxArtifacts: -1},
	})
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("CreateArtifact: expected ErrInvalidConfig, got %v", err)
	}
}

// TestMemArtifactStore_NegativeLimitRejected mirrors the FS test for the
// in-memory store so the validation contract is consistent across backends.
func TestMemArtifactStore_NegativeLimitRejected(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)
	mustCreateArtifact(t, store, root, "20240101000000_x", []byte("a"), []byte("{}"))

	_, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{
		Limits: filemigrate.ResourceLimits{MaxArtifacts: -1},
	})
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("ListArtifacts: expected ErrInvalidConfig, got %v", err)
	}
	_, err = store.ReadArtifact(t.Context(), root, "20240101000000_x", filemigrate.ReadArtifactOptions{
		Limits: filemigrate.ResourceLimits{MaxMigrationSQLBytes: -1},
	})
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("ReadArtifact: expected ErrInvalidConfig, got %v", err)
	}
	_, err = store.CreateArtifact(t.Context(), root, filemigrate.NewArtifact{
		Name:         "20240102000000_y",
		MigrationSQL: []byte("a"),
		SnapshotJSON: []byte("{}"),
	}, filemigrate.CreateArtifactOptions{
		Limits: filemigrate.ResourceLimits{MaxSnapshotJSONBytes: -1},
	})
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("CreateArtifact: expected ErrInvalidConfig, got %v", err)
	}
}

// TestMemSourceStore_NegativeLimitRejected mirrors the FS test for the
// in-memory source store.
func TestMemSourceStore_NegativeLimitRejected(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "a.go", []byte("package x"))
	root, err := store.ResolveSourceRoot(t.Context(), "/schema")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{
		Limits: filemigrate.ResourceLimits{MaxSchemaFiles: -1},
	})
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("ListSourceFiles: expected ErrInvalidConfig, got %v", err)
	}
	_, err = store.ReadSourceFile(t.Context(), root, "a.go", filemigrate.ReadSourceFileOptions{
		Limits: filemigrate.ResourceLimits{MaxSchemaSourceFileBytes: -1},
	})
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("ReadSourceFile: expected ErrInvalidConfig, got %v", err)
	}
}

func TestFSArtifactStore_ByteCapEnforced(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootEnsureForWrite)
	mustCreateArtifact(t, store, root, "20240101000000_big", []byte("SELECT 1;"), []byte(`{}`))

	_, err := store.ReadArtifact(t.Context(), root, "20240101000000_big", filemigrate.ReadArtifactOptions{
		Limits: filemigrate.ResourceLimits{MaxMigrationSQLBytes: 3}, // smaller than "SELECT 1;"
	})
	if err == nil {
		t.Fatal("expected resource limit error for oversized migration.sql")
	}
	if !errors.Is(err, filemigrate.ErrResourceLimit) {
		t.Errorf("expected ErrResourceLimit, got %v", err)
	}
	if errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("oversized files must not be reported as ErrInvalidPath: %v", err)
	}
}

// TestFSArtifactStore_OversizedSnapshotReturnsResourceLimit verifies that an
// oversized snapshot.json is reported as resource_limit (not invalid_path),
// matching the spec contract that user-tunable size limit breaches use a
// distinct, classifiable error code from corrupt-path failures.
func TestFSArtifactStore_OversizedSnapshotReturnsResourceLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootEnsureForWrite)
	mustCreateArtifact(t, store, root, "20240101000000_snap", []byte("a;"), []byte(`{"version":"1"}`))

	_, err := store.ReadArtifact(t.Context(), root, "20240101000000_snap", filemigrate.ReadArtifactOptions{
		Limits: filemigrate.ResourceLimits{MaxSnapshotJSONBytes: 3}, // smaller than the snapshot
	})
	if err == nil {
		t.Fatal("expected resource limit error for oversized snapshot.json")
	}
	if !errors.Is(err, filemigrate.ErrResourceLimit) {
		t.Errorf("expected ErrResourceLimit, got %v", err)
	}
}

// TestFSArtifactStore_NonRegularArtifactFileReturnsInvalidPath verifies the
// non-size readArtifactFile failure paths still report invalid_path, so the
// caller can tell user-tunable size breaches apart from path/shape failures.
// Regression guard: prior to the codex-round-4 fix, both size-cap failures
// and non-regular-file failures were wrapped as invalid_path, making them
// indistinguishable; this test ensures the split is preserved.
func TestFSArtifactStore_NonRegularArtifactFileReturnsInvalidPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	store := filemigrate.NewFSArtifactStore()
	root := mustResolveRoot(t, store, dir, filemigrate.RootEnsureForWrite)
	// Build a directory that contains a non-regular migration.sql (use a FIFO
	// so the type-shape check fires inside readArtifactFile).
	migDir := filepath.Join(dir, "20240101000000_bad")
	if err := os.Mkdir(migDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(migDir, "migration.sql"), 0o600); err != nil {
		t.Skipf("Mkfifo not supported on this platform: %v", err)
	}
	if err := os.WriteFile(filepath.Join(migDir, "snapshot.json"), []byte("{}"), 0o640); err != nil {
		t.Fatal(err)
	}

	_, err := store.ReadArtifact(t.Context(), root, "20240101000000_bad", filemigrate.ReadArtifactOptions{})
	if err == nil {
		t.Fatal("expected error for non-regular migration.sql")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
	if errors.Is(err, filemigrate.ErrResourceLimit) {
		t.Errorf("non-regular file must not be reported as ErrResourceLimit: %v", err)
	}
}

// TestFSArtifactStore_CurrentDirRoot verifies that "." resolves and operates
// as a valid artifact root: filepath.Join(".", name) cleans to name (no leading
// "./"), so the prefix-style containment check used previously rejected every
// artifact name in the current directory. The filepath.Rel-based check now
// handles this correctly.
func TestFSArtifactStore_CurrentDirRoot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	t.Chdir(t.TempDir())
	store := filemigrate.NewFSArtifactStore()
	root, err := store.ResolveRoot(t.Context(), ".", filemigrate.ResolveArtifactRootOptions{Mode: filemigrate.RootEnsureForWrite})
	if err != nil {
		t.Fatalf("ResolveRoot(\".\"): %v", err)
	}

	loaded := mustCreateArtifact(t, store, root, "20240101000000_init", []byte("CREATE TABLE t (id INT);"), []byte(`{"version":"1"}`))
	if loaded.Name != "20240101000000_init" {
		t.Errorf("unexpected loaded.Name: %q", loaded.Name)
	}

	read, err := store.ReadArtifact(t.Context(), root, "20240101000000_init", filemigrate.ReadArtifactOptions{})
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if !bytes.Equal(read.MigrationSQL, []byte("CREATE TABLE t (id INT);")) {
		t.Errorf("unexpected SQL: %q", read.MigrationSQL)
	}

	entries, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{})
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "20240101000000_init" {
		t.Errorf("unexpected entries: %v", entries)
	}
}

// TestMemArtifactStore_ListEnforcesDirEntryLimit verifies that the in-memory
// artifact store enforces MaxArtifactDirEntries (the raw directory-entry cap
// FSArtifactStore checks against os.ReadDir output) and not just MaxArtifacts.
// Without this, tests using the memory backend can miss configurations that
// would fail under the production FS store.
func TestMemArtifactStore_ListEnforcesDirEntryLimit(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/migrations", filemigrate.RootEnsureForWrite)
	for _, name := range []string{"20240101000000_a", "20240102000000_b", "20240103000000_c"} {
		mustCreateArtifact(t, store, root, name, []byte("a;"), []byte("{}"))
	}

	_, err := store.ListArtifacts(t.Context(), root, filemigrate.ListArtifactsOptions{
		Limits: filemigrate.ResourceLimits{MaxArtifactDirEntries: 2, MaxArtifacts: 100},
	})
	if err == nil {
		t.Fatal("expected resource limit error for dir-entry overflow")
	}
	if !errors.Is(err, filemigrate.ErrResourceLimit) {
		t.Errorf("expected ErrResourceLimit, got %v", err)
	}
}

// TestFSArtifactStore_ReadRejectsHardlinkedArtifactFile verifies that
// ReadArtifact fails with invalid_path when migration.sql or snapshot.json is
// a hard link to another file. The artifact store contract requires rejecting
// hardlinks where platform metadata exposes them so a migration directory
// cannot alias bytes outside the configured root without using a symlink.
func TestFSArtifactStore_ReadRejectsHardlinkedArtifactFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}

	cases := []string{"migration.sql", "snapshot.json"}
	for _, target := range cases {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			outside := t.TempDir()

			// Build the real artifact bytes outside the configured root.
			realSrc := filepath.Join(outside, target)
			if err := os.WriteFile(realSrc, []byte("hardlinked"), 0o640); err != nil {
				t.Fatal(err)
			}

			// Build the artifact directory under the root, with one of the
			// expected files hardlinked from outside and the other a normal
			// regular file.
			artDir := filepath.Join(dir, "20240101000000_hl")
			if err := os.Mkdir(artDir, 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(realSrc, filepath.Join(artDir, target)); err != nil {
				t.Skipf("hard links not supported on this platform: %v", err)
			}
			other := "snapshot.json"
			if target == "snapshot.json" {
				other = "migration.sql"
			}
			if err := os.WriteFile(filepath.Join(artDir, other), []byte("{}"), 0o640); err != nil {
				t.Fatal(err)
			}

			store := filemigrate.NewFSArtifactStore()
			root := mustResolveRoot(t, store, dir, filemigrate.RootReadForCheck)

			_, err := store.ReadArtifact(t.Context(), root, "20240101000000_hl", filemigrate.ReadArtifactOptions{})
			if err == nil {
				t.Fatalf("expected error for hardlinked %s", target)
			}
			if !errors.Is(err, filemigrate.ErrInvalidPath) {
				t.Errorf("expected ErrInvalidPath, got %v", err)
			}
		})
	}
}
