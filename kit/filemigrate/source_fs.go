package filemigrate

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FSSourceStore is the production SourceStore backed by the real filesystem.
// All metadata checks use Lstat so symlinks are never followed.
type FSSourceStore struct{}

// NewFSSourceStore returns the production filesystem-backed SourceStore.
func NewFSSourceStore() *FSSourceStore {
	return &FSSourceStore{}
}

var _ SourceStore = (*FSSourceStore)(nil)

const sourceOp = "source_store"

// ResolveSourceRoot resolves dir as a schema source root. Honors ctx
// cancellation before any filesystem work runs. It rejects symlinked
// paths and non-directory entries. Existing parent components of dir
// are walked with Lstat and rejected if any is a symlink, so a
// configured path like `<base>/link/schema` (where `link` is a symlink)
// cannot redirect resolution outside the intended tree.
func (s *FSSourceStore) ResolveSourceRoot(ctx context.Context, dir string) (SourceRoot, error) {
	if err := ctx.Err(); err != nil {
		return SourceRoot{}, err
	}
	if dir == "" {
		return SourceRoot{}, newInvalidConfigError(sourceOp + ".resolve_source_root")
	}
	if err := assertNoSymlinkInPathChain(dir); err != nil {
		return SourceRoot{}, &Error{
			Code: CodeInvalidPath,
			Op:   sourceOp + ".resolve_source_root",
			Path: safeRenderPath(dir),
			Err:  err,
		}
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return SourceRoot{}, newPathError(sourceOp+".resolve_source_root", dir, err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return SourceRoot{}, &Error{
			Code: CodeInvalidPath,
			Op:   sourceOp + ".resolve_source_root",
			Path: safeRenderPath(dir),
			Err:  fmt.Errorf("symlink roots are not supported"),
		}
	}
	if !fi.IsDir() {
		return SourceRoot{}, &Error{
			Code: CodeInvalidPath,
			Op:   sourceOp + ".resolve_source_root",
			Path: safeRenderPath(dir),
			Err:  fmt.Errorf("not a directory"),
		}
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return SourceRoot{}, newPathError(sourceOp+".resolve_source_root", dir, err)
	}
	return SourceRoot{Configured: dir, RealPath: real}, nil
}

// ListSourceFiles returns relative paths of .go source files under root.
// It rejects symlinked directories and files, and enforces MaxSchemaFiles.
func (s *FSSourceStore) ListSourceFiles(ctx context.Context, root SourceRoot, opts ListSourceFilesOptions) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lim := opts.Limits.resolve()
	if err := lim.Validate(sourceOp + ".list_source_files"); err != nil {
		return nil, err
	}
	var out []string

	walkErr := filepath.WalkDir(root.RealPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return &Error{Code: CodeInvalidPath, Op: sourceOp + ".list_source_files", Path: safeRenderPath(path), Err: err}
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		// Use Lstat to detect symlinks WalkDir might not report.
		fi, statErr := os.Lstat(path)
		if statErr != nil {
			return newPathError(sourceOp+".list_source_files", path, statErr)
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return &Error{
				Code: CodeInvalidPath,
				Op:   sourceOp + ".list_source_files",
				Path: safeRenderPath(path),
				Err:  fmt.Errorf("symlinks are not supported"),
			}
		}
		if d.IsDir() {
			// Skip root itself.
			if path == root.RealPath {
				return nil
			}
			return nil
		}
		if !fi.Mode().IsRegular() {
			return &Error{
				Code: CodeInvalidPath,
				Op:   sourceOp + ".list_source_files",
				Path: safeRenderPath(path),
				Err:  fmt.Errorf("not a regular file"),
			}
		}
		if hasExtraHardLinks(fi) {
			return &Error{
				Code: CodeInvalidPath,
				Op:   sourceOp + ".list_source_files",
				Path: safeRenderPath(path),
				Err:  fmt.Errorf("hardlinks are not supported"),
			}
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, relErr := filepath.Rel(root.RealPath, path)
		if relErr != nil {
			return newPathError(sourceOp+".list_source_files", path, relErr)
		}
		if len(out)+1 > lim.MaxSchemaFiles {
			return &Error{
				Code: CodeResourceLimit,
				Op:   sourceOp + ".list_source_files",
				Err:  fmt.Errorf("schema file count exceeds limit %d", lim.MaxSchemaFiles),
			}
		}
		out = append(out, rel)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(out)
	return out, nil
}

// ReadSourceFile reads the file at relpath under root using Lstat (no follow),
// validates it is a regular .go file, enforces byte caps, and returns a
// caller-owned copy. Honors ctx cancellation before any filesystem work runs.
//
// Non-Go relpaths (those without a .go suffix) are rejected with CodeInvalidPath
// before any filesystem work runs, matching the schema-loader contract that
// only .go files are valid schema inputs. The .go check fires before the
// containment check so the rejection reason is the most specific (and the
// ordering matches MemSourceStore).
func (s *FSSourceStore) ReadSourceFile(ctx context.Context, root SourceRoot, relpath string, opts ReadSourceFileOptions) (*SourceFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(relpath, ".go") {
		return nil, &Error{
			Code: CodeInvalidPath,
			Op:   sourceOp + ".read_source_file",
			Path: safeRenderPath(relpath),
			Err:  fmt.Errorf("only .go schema files are supported"),
		}
	}
	if err := assertSourceContained(root.RealPath, relpath); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: sourceOp + ".read_source_file", Path: safeRenderPath(relpath), Err: err}
	}
	lim := opts.Limits.resolve()
	if err := lim.Validate(sourceOp + ".read_source_file"); err != nil {
		return nil, err
	}
	path := filepath.Join(root.RealPath, relpath)

	// Lstat each path component under root so we reject parent-directory
	// symlinks. A bare Lstat of the final path follows parent symlinks and
	// would accept files outside the configured source root.
	if err := assertNoSymlinkInChain(root.RealPath, relpath); err != nil {
		return nil, &Error{
			Code: CodeInvalidPath,
			Op:   sourceOp + ".read_source_file",
			Path: safeRenderPath(relpath),
			Err:  err,
		}
	}

	fi, err := os.Lstat(path)
	if err != nil {
		return nil, newPathError(sourceOp+".read_source_file", relpath, err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return nil, &Error{
			Code: CodeInvalidPath,
			Op:   sourceOp + ".read_source_file",
			Path: safeRenderPath(relpath),
			Err:  fmt.Errorf("symlinks are not supported"),
		}
	}
	if !fi.Mode().IsRegular() {
		return nil, &Error{
			Code: CodeInvalidPath,
			Op:   sourceOp + ".read_source_file",
			Path: safeRenderPath(relpath),
			Err:  fmt.Errorf("not a regular file"),
		}
	}
	if hasExtraHardLinks(fi) {
		return nil, &Error{
			Code: CodeInvalidPath,
			Op:   sourceOp + ".read_source_file",
			Path: safeRenderPath(relpath),
			Err:  fmt.Errorf("hardlinks are not supported"),
		}
	}
	if fi.Size() > lim.MaxSchemaSourceFileBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: sourceOp + ".read_source_file", Path: safeRenderPath(relpath)}
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, newPathError(sourceOp+".read_source_file", relpath, err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, lim.MaxSchemaSourceFileBytes+1))
	if err != nil {
		return nil, newPathError(sourceOp+".read_source_file", relpath, err)
	}
	if int64(len(data)) > lim.MaxSchemaSourceFileBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: sourceOp + ".read_source_file", Path: safeRenderPath(relpath)}
	}
	return &SourceFile{RelPath: relpath, Content: data}, nil
}

// assertNoSymlinkInChain Lstats every component of relpath under root except
// the final one and returns an error if any is a symlink. The caller is
// responsible for Lstating the final component (and rejecting it if it is
// itself a symlink).
func assertNoSymlinkInChain(root, relpath string) error {
	clean := filepath.Clean(relpath)
	if clean == "." {
		return nil
	}
	parts := strings.Split(clean, string(filepath.Separator))
	current := root
	for i, part := range parts {
		// filepath.Clean must not produce empty components from a non-absolute
		// relpath; if one appears we fail closed rather than skip silently.
		if part == "" {
			return fmt.Errorf("invalid empty path component")
		}
		current = filepath.Join(current, part)
		// Only check parent components here; the final component is checked by
		// the caller (which also enforces regular-file semantics).
		if i == len(parts)-1 {
			return nil
		}
		fi, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported")
		}
	}
	return nil
}

// assertSourceContained verifies that relpath, when joined with root, stays
// within root (no path-traversal escape) and names a file under root rather
// than root itself.
//
// The containment check is performed via filepath.Rel rather than a
// string-prefix comparison so that relative roots (notably "." for the
// current directory) are handled correctly: filepath.Join(".", "schema.go")
// cleans to "schema.go", which would fail a "schema.go has prefix ./" check
// even though it is contained.
//
// An empty relpath, ".", or any path that cleans to root itself is also
// rejected: those forms are not valid file names and would otherwise produce
// degenerate map keys in the in-memory stores or refer to the root directory
// in the filesystem store.
func assertSourceContained(root, relpath string) error {
	if filepath.IsAbs(relpath) {
		return fmt.Errorf("absolute paths are not allowed")
	}
	full := filepath.Clean(filepath.Join(root, relpath))
	rel, err := filepath.Rel(filepath.Clean(root), full)
	if err != nil {
		return fmt.Errorf("path escapes root")
	}
	if rel == "." {
		return fmt.Errorf("relpath must name a file under root")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes root")
	}
	return nil
}
