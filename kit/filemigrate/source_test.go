package filemigrate_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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
// such as README.md, generated output, or uppercase-extension files. This
// keeps the in-memory store's listing semantics aligned with FSSourceStore
// (which is case-sensitive .go) so tests using the memory fixture do not
// feed non-Go content into schema parsing.
func TestMemSourceStore_ListFiltersNonGoFiles(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "users.go", []byte("package schema"))
	store.AddFile("/schema", "README.md", []byte("# docs"))
	store.AddFile("/schema", "generated.txt", []byte("ignore me"))
	// Uppercase extension is not Go on a case-sensitive filesystem; both
	// FSSourceStore and MemSourceStore must reject it.
	store.AddFile("/schema", "Schema.GO", []byte("not a go file"))
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
// number of .go files lists successfully under a tight limit. Multiple
// non-Go files are seeded so a regression that counted them before the
// filter would exceed the limit regardless of map-iteration order.
func TestMemSourceStore_ListNonGoFilesNotCountedAgainstLimit(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "a.go", []byte("package schema"))
	store.AddFile("/schema", "README.md", []byte("# docs"))
	store.AddFile("/schema", "generated.txt", []byte("ignore"))
	store.AddFile("/schema", ".gitkeep", []byte(""))
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
// ResolveSourceRoot returns immediately when the context is already done,
// before doing any filesystem work — matching the cancellation behavior
// of MemSourceStore.ResolveSourceRoot and the entry-time checks in
// ListSourceFiles/ReadSourceFile. Both Canceled and DeadlineExceeded
// states are exercised since ctx.Err() can return either sentinel.
func TestFSSourceStore_ResolveHonorsCancelledContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	store := filemigrate.NewFSSourceStore()

	tests := []struct {
		name    string
		makeCtx func(t *testing.T) context.Context
		want    error
	}{
		{
			name: "canceled",
			makeCtx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
			want: context.Canceled,
		},
		{
			name: "deadline_exceeded",
			makeCtx: func(t *testing.T) context.Context {
				ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
				t.Cleanup(cancel)
				return ctx
			},
			want: context.DeadlineExceeded,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.ResolveSourceRoot(tc.makeCtx(t), dir)
			if err == nil {
				t.Fatal("expected error from done context")
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("expected %v, got %v", tc.want, err)
			}
		})
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

// TestFSSourceStore_ReadRejectsNonGoFile verifies that a direct ReadSourceFile
// call for a non-.go relpath fails with CodeInvalidPath before any filesystem
// I/O. The schema-loader contract is that only .go files are valid schema
// inputs, so explicitly configured sidecars (README.md, generated.txt, etc.)
// must be rejected at the call site, not just filtered out of ListSourceFiles.
func TestFSSourceStore_ReadRejectsNonGoFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# docs"), 0o640); err != nil {
		t.Fatal(err)
	}

	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.ReadSourceFile(t.Context(), root, "README.md", filemigrate.ReadSourceFileOptions{})
	if err == nil {
		t.Fatal("expected error for non-.go file")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

// TestMemSourceStore_ReadRejectsNonGoFile mirrors the FS test on the in-memory
// store so the two backends agree on non-.go rejection semantics.
func TestMemSourceStore_ReadRejectsNonGoFile(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "README.md", []byte("# docs"))
	root, _ := store.ResolveSourceRoot(t.Context(), "/schema")

	_, err := store.ReadSourceFile(t.Context(), root, "README.md", filemigrate.ReadSourceFileOptions{})
	if err == nil {
		t.Fatal("expected error for non-.go file")
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("expected ErrInvalidPath, got %v", err)
	}
}

// TestMemSourceStore_ReadRejectsEscapingRelpath verifies that MemSourceStore
// rejects a relpath that escapes the configured root (via ".." traversal or
// an absolute path) with ErrInvalidPath, even when AddFile has been used to
// seed a key matching the escaping path. This keeps the in-memory store's
// containment contract aligned with FSSourceStore (which rejects the same
// inputs) so tests cannot pass with schema inputs that production reads
// would refuse.
func TestMemSourceStore_ReadRejectsEscapingRelpath(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	// Seed a key that AddFile's raw-concat would form for "../outside.go" so
	// the test would succeed under the pre-fix behavior.
	store.AddFile("/schema", "../outside.go", []byte("package outside"))
	root, _ := store.ResolveSourceRoot(t.Context(), "/schema")

	cases := []string{
		"../outside.go",
		"sub/../../outside.go",
		"/abs/outside.go",
	}
	for _, rel := range cases {
		t.Run(rel, func(t *testing.T) {
			_, err := store.ReadSourceFile(t.Context(), root, rel, filemigrate.ReadSourceFileOptions{})
			if err == nil {
				t.Fatal("expected error for escaping relpath")
			}
			if !errors.Is(err, filemigrate.ErrInvalidPath) {
				t.Errorf("expected ErrInvalidPath, got %v", err)
			}
		})
	}
}

// TestMemSourceStore_ListSkipsEscapingRelpath verifies that ListSourceFiles
// silently skips seeded keys whose relpath escapes the configured root, and
// does not count them against MaxSchemaFiles. FSSourceStore walks the
// filesystem rooted at RealPath and physically cannot return such entries, so
// the in-memory store must match: a caller that lists then reads must not see
// listings the filesystem store could never produce, and a tight
// MaxSchemaFiles must not trip on entries that cannot belong to the root.
func TestMemSourceStore_ListSkipsEscapingRelpath(t *testing.T) {
	store := filemigrate.NewMemSourceStore()
	store.AddFile("/schema", "a.go", []byte("package schema"))
	store.AddFile("/schema", "../outside.go", []byte("package outside"))
	store.AddFile("/schema", "sub/../../also-outside.go", []byte("package outside"))
	root, _ := store.ResolveSourceRoot(t.Context(), "/schema")

	files, err := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{
		// Tight limit: would trip if escaping keys were counted.
		Limits: filemigrate.ResourceLimits{MaxSchemaFiles: 1},
	})
	if err != nil {
		t.Fatalf("ListSourceFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "a.go" {
		t.Errorf("expected only [a.go], got %v", files)
	}
}

// TestFSSourceStore_CurrentDirRoot verifies that "." resolves and operates as
// a valid schema source root: filepath.Join(".", relpath) cleans to relpath
// (no leading "./"), so the prefix-style containment check used previously
// rejected every direct read in the current directory. The filepath.Rel-based
// check now handles this correctly.
func TestFSSourceStore_CurrentDirRoot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "schema.go"), []byte("package schema"), 0o640); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), ".")
	if err != nil {
		t.Fatalf("ResolveSourceRoot(\".\"): %v", err)
	}

	files, err := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{})
	if err != nil {
		t.Fatalf("ListSourceFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "schema.go" {
		t.Errorf("expected [schema.go], got %v", files)
	}

	sf, err := store.ReadSourceFile(t.Context(), root, "schema.go", filemigrate.ReadSourceFileOptions{})
	if err != nil {
		t.Fatalf("ReadSourceFile: %v", err)
	}
	if !bytes.Equal(sf.Content, []byte("package schema")) {
		t.Errorf("unexpected content: %q", sf.Content)
	}
}

// TestFSSourceStore_HardlinkedFileRejected verifies that both ReadSourceFile
// and ListSourceFiles fail with invalid_path when a .go file under the
// configured source root has more than one hard link. The schema source
// contract requires rejecting hard links where platform metadata exposes them.
// Two seedings are exercised: cross_root (the second link lives outside the
// configured root, the original threat model) and intra_root (both links are
// inside the root, exercising the per-inode Nlink>1 invariant). Op-string
// asserts confirm the error is reported by the right call site.
func TestFSSourceStore_HardlinkedFileRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}

	const (
		readOp = "source_store.read_source_file"
		listOp = "source_store.list_source_files"
	)

	t.Run("cross_root", func(t *testing.T) {
		dir := t.TempDir()
		outside := t.TempDir()
		realFile := filepath.Join(outside, "real.go")
		if err := os.WriteFile(realFile, []byte("package schema"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(realFile, filepath.Join(dir, "schema.go")); err != nil {
			t.Skipf("hard links not supported on this platform: %v", err)
		}
		assertSourceHardlinkRejected(t, dir, "schema.go", readOp, listOp)
	})

	t.Run("intra_root", func(t *testing.T) {
		// Both links live inside the configured source root. The store must
		// still reject the entry because the check is per-inode.
		dir := t.TempDir()
		realFile := filepath.Join(dir, "real.go")
		if err := os.WriteFile(realFile, []byte("package schema"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(realFile, filepath.Join(dir, "schema.go")); err != nil {
			t.Skipf("hard links not supported on this platform: %v", err)
		}
		assertSourceHardlinkRejected(t, dir, "schema.go", readOp, listOp)
	})
}

func assertSourceHardlinkRejected(t *testing.T, dir, relpath, readOp, listOp string) {
	t.Helper()
	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), dir)
	if err != nil {
		t.Fatalf("ResolveSourceRoot: %v", err)
	}

	_, err = store.ReadSourceFile(t.Context(), root, relpath, filemigrate.ReadSourceFileOptions{})
	assertHardlinkInvalidPathSource(t, err, readOp, "ReadSourceFile")

	// ListSourceFiles must also reject the hard-linked .go entry at discovery
	// time so a hard-linked file never reaches the caller's returned slice.
	_, listErr := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{})
	assertHardlinkInvalidPathSource(t, listErr, listOp, "ListSourceFiles")
}

// assertHardlinkInvalidPathSource is the source-store analogue of
// assertHardlinkInvalidPath in artifact_test.go. It asserts ErrInvalidPath
// and the structured Op tag so a regression that wraps the hard-link error
// under the wrong Op is caught.
func assertHardlinkInvalidPathSource(t *testing.T, err error, wantOp, label string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error", label)
	}
	if !errors.Is(err, filemigrate.ErrInvalidPath) {
		t.Errorf("%s: expected ErrInvalidPath, got %v", label, err)
	}
	var ferr *filemigrate.Error
	if errors.As(err, &ferr) {
		if ferr.Op != wantOp {
			t.Errorf("%s: Op = %q, want %q", label, ferr.Op, wantOp)
		}
	}
}

// TestFSSourceStore_HardlinkedNonGoFileIgnored verifies that hard-linked
// non-.go sidecar files (e.g. README.md, .gitkeep) under the source root do
// NOT fail listing. Non-.go files are silently skipped by ListSourceFiles, so
// the hardlink rejection must run after the suffix filter; otherwise a
// hard-linked sidecar would make the entire schema unreadable.
func TestFSSourceStore_HardlinkedNonGoFileIgnored(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping filesystem test in short mode")
	}
	dir := t.TempDir()
	outside := t.TempDir()

	// A non-.go sidecar that is hard-linked from outside the root.
	realReadme := filepath.Join(outside, "README.md")
	if err := os.WriteFile(realReadme, []byte("readme"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(realReadme, filepath.Join(dir, "README.md")); err != nil {
		t.Skipf("hard links not supported on this platform: %v", err)
	}

	// A normal, non-hard-linked .go schema file that the listing should return.
	if err := os.WriteFile(filepath.Join(dir, "schema.go"), []byte("package schema"), 0o640); err != nil {
		t.Fatal(err)
	}

	store := filemigrate.NewFSSourceStore()
	root, err := store.ResolveSourceRoot(t.Context(), dir)
	if err != nil {
		t.Fatalf("ResolveSourceRoot: %v", err)
	}
	files, err := store.ListSourceFiles(t.Context(), root, filemigrate.ListSourceFilesOptions{})
	if err != nil {
		t.Fatalf("ListSourceFiles: unexpected error for hard-linked sidecar: %v", err)
	}
	if len(files) != 1 || files[0] != "schema.go" {
		t.Errorf("expected [schema.go], got %v", files)
	}
}
