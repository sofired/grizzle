package filemigrate

import (
	"context"
	"fmt"
	"go/build"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// schemaBuildContext is the go/build.Context used to filter discovered .go
// schema source files. Per docs/spec/file-migrations-api.md:1417 file
// selection delegates to go/build.Context.MatchFile for //go:build, legacy
// // +build, and GOOS/GOARCH suffix handling. The context uses GOOS/GOARCH
// from environment when set, otherwise runtime.GOOS/runtime.GOARCH (via
// build.Default), the current toolchain Compiler and ReleaseTags, no custom
// build tags, and CgoEnabled=false — matching spec line 1418 until
// cgo-aware schema files are an explicit option.
var schemaBuildContext = func() build.Context {
	ctxt := build.Default
	ctxt.CgoEnabled = false
	ctxt.BuildTags = nil
	return ctxt
}()

// FSSourceStore is the production SourceStore backed by the real filesystem.
// Security-sensitive operations are rooted at open directory handles and
// identity-check entries across metadata inspection and open.
type FSSourceStore struct{}

// NewFSSourceStore returns the production filesystem-backed SourceStore.
func NewFSSourceStore() *FSSourceStore {
	return &FSSourceStore{}
}

var _ SourceStore = (*FSSourceStore)(nil)

const sourceOp = "source_store"

// ResolveSourceRoot resolves dir as a schema source root. It walks from the
// volume root with handle-relative opens, rejecting symlinks, non-directory
// entries, and entries whose identity changes while they are opened.
func (s *FSSourceStore) ResolveSourceRoot(ctx context.Context, dir string) (SourceRoot, error) {
	if err := ctx.Err(); err != nil {
		return SourceRoot{}, err
	}
	if dir == "" {
		return SourceRoot{}, newInvalidConfigError(sourceOp + ".resolve_source_root")
	}
	rootHandle, real, _, err := openSecureRootPath(dir, false, 0)
	if err != nil {
		return SourceRoot{}, &Error{
			Code: CodeInvalidPath,
			Op:   sourceOp + ".resolve_source_root",
			Path: safeRenderPath(dir),
			Err:  err,
		}
	}
	_ = rootHandle.Close()
	return SourceRoot{Configured: dir, RealPath: real}, nil
}

// ListSourceFiles returns relative paths of .go source files under root.
// It rejects symlinked directories and files, and enforces MaxSchemaFiles.
//
// Discovered .go files are passed through go/build.Context.MatchFile per
// docs/spec/file-migrations-api.md:1417 so that _test.go files, files for
// other GOOS/GOARCH targets, and files with unsatisfied //go:build or legacy
// // +build constraints are skipped instead of fed to the schema parser.
// Build-constraint parse errors fail with CodeUnsupportedSchemaConstruct;
// the diagnostic uses a generic message so build-line text is not echoed
// (spec line 1415).
func (s *FSSourceStore) ListSourceFiles(ctx context.Context, root SourceRoot, opts ListSourceFilesOptions) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lim := opts.Limits.resolve()
	if err := lim.Validate(sourceOp + ".list_source_files"); err != nil {
		return nil, err
	}
	rootHandle, _, _, err := openSecureRootPath(root.RealPath, false, 0)
	if err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: sourceOp + ".list_source_files", Path: safeRenderPath(root.RealPath), Err: err}
	}
	defer func() { _ = rootHandle.Close() }()

	var out []string
	if err := walkSecureSourceDir(ctx, rootHandle, root.RealPath, "", lim, &out); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func walkSecureSourceDir(ctx context.Context, dir *os.Root, displayRoot, relDir string, lim ResourceLimits, out *[]string) error {
	entries, err := readSecureDir(dir)
	if err != nil {
		return &Error{Code: CodeInvalidPath, Op: sourceOp + ".list_source_files", Path: safeRenderPath(filepath.Join(displayRoot, relDir)), Err: err}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		relpath := name
		if relDir != "" {
			relpath = filepath.Join(relDir, name)
		}
		displayPath := filepath.Join(displayRoot, relpath)
		info, err := dir.Lstat(name)
		if err != nil {
			return newPathError(sourceOp+".list_source_files", displayPath, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return &Error{Code: CodeInvalidPath, Op: sourceOp + ".list_source_files", Path: safeRenderPath(displayPath), Err: fmt.Errorf("symlinks are not supported")}
		}
		if info.IsDir() {
			child, _, err := openSecureDirFromInfo(dir, name, info, nil)
			if err != nil {
				return &Error{Code: CodeInvalidPath, Op: sourceOp + ".list_source_files", Path: safeRenderPath(displayPath), Err: err}
			}
			walkErr := walkSecureSourceDir(ctx, child, displayRoot, relpath, lim, out)
			_ = child.Close()
			if walkErr != nil {
				return walkErr
			}
			continue
		}
		// Non-.go files are ignored regardless of their shape, matching the
		// schema-loader contract and the in-memory store.
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if !info.Mode().IsRegular() {
			return &Error{Code: CodeInvalidPath, Op: sourceOp + ".list_source_files", Path: safeRenderPath(displayPath), Err: fmt.Errorf("not a regular file")}
		}
		if hasExtraHardLinks(info) {
			return &Error{Code: CodeInvalidPath, Op: sourceOp + ".list_source_files", Path: safeRenderPath(displayPath), Err: fmt.Errorf("hard links are not supported")}
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		buildContext := schemaBuildContext
		var secureOpenErr error
		buildContext.OpenFile = func(path string) (io.ReadCloser, error) {
			f, openedInfo, openErr := openSecureFile(dir, filepath.Base(path), nil)
			if openErr == nil && (!openedInfo.Mode().IsRegular() || hasExtraHardLinks(openedInfo)) {
				_ = f.Close()
				openErr = fmt.Errorf("unsafe source file shape")
			}
			if openErr != nil {
				secureOpenErr = openErr
			}
			return f, openErr
		}
		match, matchErr := buildContext.MatchFile(".", name)
		if secureOpenErr != nil {
			return &Error{Code: CodeInvalidPath, Op: sourceOp + ".list_source_files", Path: safeRenderPath(displayPath), Err: secureOpenErr}
		}
		if matchErr != nil {
			// Build-line source text is intentionally redacted.
			return &Error{Code: CodeUnsupportedSchemaConstruct, Op: sourceOp + ".list_source_files", Path: safeRenderPath(displayPath), Err: fmt.Errorf("invalid build constraint")}
		}
		if !match {
			continue
		}
		if len(*out)+1 > lim.MaxSchemaFiles {
			return &Error{Code: CodeResourceLimit, Op: sourceOp + ".list_source_files", Err: fmt.Errorf("schema file count exceeds limit %d", lim.MaxSchemaFiles)}
		}
		*out = append(*out, relpath)
	}
	return nil
}

// ReadSourceFile walks to relpath through open directory handles, validates the
// opened entry is the same regular .go file observed by Lstat, enforces byte
// caps, and returns a caller-owned copy. Honors ctx cancellation before any
// filesystem work runs.
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
	rootHandle, _, _, err := openSecureRootPath(root.RealPath, false, 0)
	if err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: sourceOp + ".read_source_file", Path: safeRenderPath(relpath), Err: err}
	}
	defer func() { _ = rootHandle.Close() }()

	f, fi, err := openSecureFilePath(rootHandle, relpath)
	if err != nil {
		return nil, newPathError(sourceOp+".read_source_file", relpath, err)
	}
	defer func() { _ = f.Close() }()
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
			Err:  fmt.Errorf("hard links are not supported"),
		}
	}
	if fi.Size() > lim.MaxSchemaSourceFileBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: sourceOp + ".read_source_file", Path: safeRenderPath(relpath)}
	}

	data, err := io.ReadAll(io.LimitReader(f, lim.MaxSchemaSourceFileBytes+1))
	if err != nil {
		return nil, newPathError(sourceOp+".read_source_file", relpath, err)
	}
	if int64(len(data)) > lim.MaxSchemaSourceFileBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: sourceOp + ".read_source_file", Path: safeRenderPath(relpath)}
	}
	return &SourceFile{RelPath: relpath, Content: data}, nil
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
