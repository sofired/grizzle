package filemigrate_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sofired/grizzle/kit/filemigrate"
)

// --- MemSourceStore ---

func TestMemSourceStore_EmptyRoot(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), "/schema")
	if err != nil {
		t.Fatalf("ResolveSourceRoot: %v", err)
	}
	files, err := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{})
	if err != nil {
		t.Fatalf("ListSourceFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestMemSourceStore_AddAndRead(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "users.go", []byte("package schema"))
	root, err := store.ResolveSourceRoot(t.Context(), "/schema")
	if err != nil {
		t.Fatal(err)
	}

	files, err := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{})
	if err != nil {
		t.Fatalf("ListSourceFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "users.go" {
		t.Errorf("unexpected files: %v", files)
	}

	sf, err := store.ReadSourceFile(t.Context(), root, "users.go", filemigrate.ReadSourceFileOptions{})
	if err != nil {
		t.Fatalf("ReadSourceFile: %v", err)
	}
	if !bytes.Equal(sf.Content, []byte("package schema")) {
		t.Errorf("unexpected content: %q", sf.Content)
	}
}

func TestMemSourceStore_ReturnedSliceIsIndependent(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "a.go", []byte("package schema"))
	root, _ := store.ResolveSourceRoot(t.Context(), "/schema")

	sf, err := store.ReadSourceFile(t.Context(), root, "a.go", filemigrate.ReadSourceFileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	sf.Content[0] = 'X'

	sf2, err := store.ReadSourceFile(t.Context(), root, "a.go", filemigrate.ReadSourceFileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if sf2.Content[0] == 'X' {
		t.Error("store returned a reference to its internal slice (not a copy)")
	}
}

func TestMemSourceStore_MissingFileError(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	root, _ := store.ResolveSourceRoot(t.Context(), "/schema")
	_, err := store.ReadSourceFile(t.Context(), root, "missing.go", filemigrate.ReadSourceFileOptions{})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

func TestMemSourceStore_ResourceLimitOnList(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "a.go", []byte("package schema"))
	store.AddFile("/schema", "b.go", []byte("package schema"))
	root, _ := store.ResolveSourceRoot(t.Context(), "/schema")

	_, err := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{
		Limits: filemigrate.ResourceLimits{MaxSchemaFiles: 1},
	})
	if err == nil {
		t.Fatal("expected resource limit error")
	}
	if !errors.Is(err, filemigrate.ErrResourceLimit) {
		t.Errorf("expected ErrResourceLimit, got %v", err)
	}
}

func TestMemSourceStore_ResourceLimitOnRead(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "big.go", []byte("package schema // lots of content here"))
	root, _ := store.ResolveSourceRoot(t.Context(), "/schema")

	_, err := store.ReadSourceFile(t.Context(), root, "big.go", filemigrate.ReadSourceFileOptions{
		Limits: filemigrate.ResourceLimits{MaxSchemaSourceFileBytes: 4},
	})
	if err == nil {
		t.Fatal("expected resource limit error")
	}
	if !errors.Is(err, filemigrate.ErrResourceLimit) {
		t.Errorf("expected ErrResourceLimit, got %v", err)
	}
}

func TestMemSourceStore_ListSortedLexicographically(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "z.go", []byte("z"))
	store.AddFile("/schema", "a.go", []byte("a"))
	store.AddFile("/schema", "m.go", []byte("m"))
	root, _ := store.ResolveSourceRoot(t.Context(), "/schema")

	files, err := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.go", "m.go", "z.go"}
	for i, f := range files {
		if f != want[i] {
			t.Errorf("files[%d] = %q, want %q", i, f, want[i])
		}
	}
}

func TestMemSourceStore_EmptyDirRejected(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	_, err := store.ResolveSourceRoot(t.Context(), "")
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

// TestMemSourceStore_ListFiltersNonGoFiles verifies that ListSourceFiles
// returns only .go files even when callers seed the store with sidecars
// such as README.md or generated output. This keeps the in-memory store's
// listing semantics aligned with FSSourceStore so tests using the memory
// fixture do not feed non-Go content into schema parsing.
func TestMemSourceStore_ListFiltersNonGoFiles(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "users.go", []byte("package schema"))
	store.AddFile("/schema", "README.md", []byte("# docs"))
	store.AddFile("/schema", "generated.txt", []byte("ignore me"))
	store.AddFile("/schema", "posts.go", []byte("package schema"))
	root, _ := store.ResolveSourceRoot(t.Context(), "/schema")

	files, err := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{})
	if err != nil {
		t.Fatalf("ListSourceFiles: %v", err)
	}
	want := []string{"posts.go", "users.go"}
	if len(files) != len(want) {
		t.Fatalf("expected %v, got %v", want, files)
	}
	for i, f := range files {
		if f != want[i] {
			t.Errorf("files[%d] = %q, want %q", i, f, want[i])
		}
	}
}

// TestMemSourceStore_ListNonGoFilesNotCountedAgainstLimit verifies that
// non-Go sidecars do not consume MaxSchemaFiles budget — they are skipped
// before the limit check, so a store seeded with sidecars plus a small
// number of .go files lists successfully under a tight limit.
func TestMemSourceStore_ListNonGoFilesNotCountedAgainstLimit(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "a.go", []byte("package schema"))
	store.AddFile("/schema", "README.md", []byte("# docs"))
	store.AddFile("/schema", "b.go", []byte("package schema"))
	root, _ := store.ResolveSourceRoot(t.Context(), "/schema")

	files, err := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{
		Limits: filemigrate.ResourceLimits{MaxSchemaFiles: 2},
	})
	if err != nil {
		t.Fatalf("ListSourceFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 .go files, got %v", files)
	}
}

// --- FSSourceStore ---

func TestFSSourceStore_ResolveAndList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.go"), []byte("package schema"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# docs"), 0o640); err != nil {
		t.Fatal(err)
	}

	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), dir)
	if err != nil {
		t.Fatalf("ResolveSourceRoot: %v", err)
	}
	files, err := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{})
	if err != nil {
		t.Fatalf("ListSourceFiles: %v", err)
	}
	// Only .go files should appear.
	if len(files) != 1 || files[0] != "users.go" {
		t.Errorf("expected [users.go], got %v", files)
	}
}

func TestFSSourceStore_SymlinkRootRejected(t *testing.T) {
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

	store := filemigrate.NewFSSourceStore()
	_, err := store.ResolveSourceRoot(t.Context(), link)
	if err == nil {
		t.Fatal("expected error for symlinked root")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

func TestFSSourceStore_SymlinkFileRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.go")
	linkFile := filepath.Join(dir, "linked.go")
	if err := os.WriteFile(realFile, []byte("package schema"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realFile, linkFile); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), dir)
	if err != nil {
		t.Fatalf("ResolveSourceRoot: %v", err)
	}
	_, err = store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{})
	if err == nil {
		t.Fatal("expected error for symlinked source file")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

func TestFSSourceStore_PathTraversalRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.ReadSourceFile(t.Context(), root, "../../etc/passwd", filemigrate.ReadSourceFileOptions{})
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

func TestFSSourceStore_SymlinkedSubdirRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	realSub := filepath.Join(dir, "realsub")
	if err := os.Mkdir(realSub, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realSub, "schema.go"), []byte("package schema"), 0o640); err != nil {
		t.Fatal(err)
	}
	// Create a symlink to the subdirectory under the source root.
	linkSub := filepath.Join(dir, "linked")
	if err := os.Symlink(realSub, linkSub); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), dir)
	if err != nil {
		t.Fatalf("ResolveSourceRoot: %v", err)
	}
	// WalkDir will visit the symlinked subdirectory; ListSourceFiles must reject it.
	_, err = store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{})
	if err == nil {
		t.Fatal("expected error for symlinked subdirectory under source root")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

func TestFSSourceStore_ReadRejectsSymlinkedParent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	// Create a real subdirectory containing schema.go OUTSIDE the source root.
	outside := t.TempDir()
	realSub := filepath.Join(outside, "real")
	if err := os.Mkdir(realSub, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realSub, "schema.go"), []byte("package schema"), 0o640); err != nil {
		t.Fatal(err)
	}
	// Symlink "linked" under the source root to the outside subdirectory.
	linked := filepath.Join(dir, "linked")
	if err := os.Symlink(realSub, linked); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), dir)
	if err != nil {
		t.Fatalf("ResolveSourceRoot: %v", err)
	}
	// ReadSourceFile must reject the parent symlink even though the final
	// component (schema.go) is a regular file that Lstat alone would accept.
	_, err = store.ReadSourceFile(t.Context(), root, "linked/schema.go", filemigrate.ReadSourceFileOptions{})
	if err == nil {
		t.Fatal("expected error for symlinked parent component")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

// TestFSSourceStore_SymlinkedParentRootRejected covers the case where the
// configured root path itself contains a symlinked parent component (e.g.
// `<base>/link/schema` where `link -> /outside`). A bare Lstat on the final
// component would silently follow that parent symlink; ResolveSourceRoot must
// reject it before EvalSymlinks redirects scanning outside the intended tree.
func TestFSSourceStore_SymlinkedParentRootRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	base := t.TempDir()
	// Create a real "schema" directory outside the base.
	outside := t.TempDir()
	realParent := filepath.Join(outside, "real")
	if err := os.MkdirAll(filepath.Join(realParent, "schema"), 0o750); err != nil {
		t.Fatal(err)
	}
	// Place a symlink "link" under base pointing at the outside parent.
	if err := os.Symlink(realParent, filepath.Join(base, "link")); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	store := filemigrate.NewFSSourceStore()
	_, err := store.ResolveSourceRoot(t.Context(), filepath.Join(base, "link", "schema"))
	if err == nil {
		t.Fatal("expected error for symlinked parent component in source root")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

// TestFSSourceStore_NegativeLimitRejected verifies that a negative field in
// ResourceLimits is rejected with ErrInvalidConfig before any filesystem work
// runs, so that callers cannot silently get unexpected behavior from the
// resolve()-then-compare flow.
func TestFSSourceStore_NegativeLimitRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "schema.go"), []byte("package x"), 0o640); err != nil {
		t.Fatal(err)
	}
	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}

	// ListSourceFiles
	_, err = store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{
		Limits: filemigrate.ResourceLimits{MaxSchemaFiles: -1},
	})
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("ListSourceFiles: expected ErrInvalidConfig, got %v", err)
	}

	// ReadSourceFile
	_, err = store.ReadSourceFile(t.Context(), root, "schema.go", filemigrate.ReadSourceFileOptions{
		Limits: filemigrate.ResourceLimits{MaxSchemaSourceFileBytes: -1},
	})
	if !errors.Is(err, filemigrate.ErrInvalidConfig) {
		t.Errorf("ReadSourceFile: expected ErrInvalidConfig, got %v", err)
	}
}

// TestFSSourceStore_ResolveHonorsCancelledContext verifies that
// ResolveSourceRoot returns immediately when given an already-cancelled
// context, before doing any filesystem work — matching the cancellation
// behavior of MemSourceStore.ResolveSourceRoot and the entry-time checks
// in ListSourceFiles/ReadSourceFile.
func TestFSSourceStore_ResolveHonorsCancelledContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	store := filemigrate.NewFSSourceStore()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := store.ResolveSourceRoot(ctx, dir)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestFSSourceStore_ReadHonorsCancelledContext verifies that ReadSourceFile
// returns immediately when given an already-cancelled context, before doing
// any filesystem work — matching the cancellation contract documented in the
// schema-loader spec and matching the existing context check in ListSourceFiles.
func TestFSSourceStore_ReadHonorsCancelledContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "schema.go"), []byte("package x"), 0o640); err != nil {
		t.Fatal(err)
	}
	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = store.ReadSourceFile(ctx, root, "schema.go", filemigrate.ReadSourceFileOptions{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestFSSourceStore_ReadByteCap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	content := []byte("package schema // this content is intentionally long")
	if err := os.WriteFile(filepath.Join(dir, "schema.go"), content, 0o640); err != nil {
		t.Fatal(err)
	}

	store := filemigrate.NewFSSourceStore()
	root, _ := store.ResolveSourceRoot(t.Context(), dir)
	_, err := store.ReadSourceFile(t.Context(), root, "schema.go", filemigrate.ReadSourceFileOptions{
		Limits: filemigrate.ResourceLimits{MaxSchemaSourceFileBytes: 5},
	})
	if err == nil {
		t.Fatal("expected resource limit error")
	}
	if !errors.Is(err, filemigrate.ErrResourceLimit) {
		t.Errorf("expected ErrResourceLimit, got %v", err)
	}
}
