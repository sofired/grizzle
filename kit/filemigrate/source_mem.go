package filemigrate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// MemSourceStore is a thread-safe in-memory SourceStore for tests.
// Files are keyed by "root/relpath". It performs no filesystem I/O.
type MemSourceStore struct {
	mu    sync.Mutex
	files map[string][]byte // key: root + "/" + relpath
}

// NewMemSourceStore returns an initialized in-memory SourceStore.
func NewMemSourceStore() *MemSourceStore {
	return &MemSourceStore{files: make(map[string][]byte)}
}

var _ SourceStore = (*MemSourceStore)(nil)

// AddFile stores content under root/relpath. It is safe to call concurrently.
func (s *MemSourceStore) AddFile(root, relpath string, content []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[root+"/"+relpath] = bytes.Clone(content)
}

// ResolveSourceRoot always succeeds for non-empty dir values.
func (s *MemSourceStore) ResolveSourceRoot(ctx context.Context, dir string) (SourceRoot, error) {
	if err := ctx.Err(); err != nil {
		return SourceRoot{}, err
	}
	if dir == "" {
		return SourceRoot{}, newInvalidConfigError("mem_source_store.resolve_source_root")
	}
	return SourceRoot{Configured: dir, RealPath: dir}, nil
}

// ListSourceFiles returns the relative paths of .go schema source files
// stored under root, enforcing MaxSchemaFiles. Non-Go files (sidecars such
// as README.md or generated output) are skipped to match the production
// FSSourceStore, which only returns .go inputs from the schema root.
//
// Keys seeded via AddFile whose relpath escapes root (e.g.
// AddFile(root, "../outside.go", ...)) are also skipped: FSSourceStore walks
// the filesystem rooted at RealPath and physically cannot observe such paths,
// so the in-memory store must not surface them either. Counting them against
// MaxSchemaFiles or returning them would let tests pass on listings the
// production store could never produce.
//
// Build-rule filtering matches FSSourceStore: _test.go files,
// GOOS/GOARCH-suffixed files for other targets, and files with unsatisfied
// //go:build or legacy // +build constraints are skipped via
// go/build.Context.MatchFile (per docs/spec/file-migrations-api.md:1417).
// The build context uses a custom OpenFile that reads seeded bytes from the
// in-memory map, so constraints declared inside file bodies are evaluated
// the same way the production filesystem store evaluates them.
// Build-constraint parse errors fail with CodeUnsupportedSchemaConstruct
// per spec line 1420; the diagnostic uses a generic message so build-line
// text is not echoed (spec line 1415).
func (s *MemSourceStore) ListSourceFiles(ctx context.Context, root SourceRoot, opts ListSourceFilesOptions) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lim := opts.Limits.resolve()
	if err := lim.Validate("mem_source_store.list_source_files"); err != nil {
		return nil, err
	}
	prefix := root.RealPath + "/"

	// Snapshot files matching the prefix under the lock so MatchFile (which
	// invokes OpenFile during constraint evaluation) does not need to
	// re-acquire s.mu — re-entrancy on a sync.Mutex would deadlock. The
	// map values are []byte slice headers; AddFile stores bytes.Clone'd
	// slices and never mutates an existing slice in place, so reading from
	// the snapshot after the lock is released is safe.
	s.mu.Lock()
	snapshot := make(map[string][]byte, len(s.files))
	for k, v := range s.files {
		if hasPrefix(k, prefix) {
			snapshot[k] = v
		}
	}
	s.mu.Unlock()

	ctxt := schemaBuildContext
	ctxt.OpenFile = func(path string) (io.ReadCloser, error) {
		// MatchFile passes filepath.Join(dir, name) here. We always invoke
		// MatchFile with dir under root.RealPath, so resolve back to a key
		// in our prefix-bounded snapshot. Use the standard *fs.PathError /
		// fs.ErrNotExist sentinel so go/build can distinguish "file absent"
		// from a real I/O error.
		rel, err := filepath.Rel(root.RealPath, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}
		key := root.RealPath + "/" + filepath.ToSlash(rel)
		if data, ok := snapshot[key]; ok {
			return io.NopCloser(bytes.NewReader(data)), nil
		}
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}

	var out []string
	for k := range snapshot {
		rel := k[len(prefix):]
		if !strings.HasSuffix(rel, ".go") {
			continue
		}
		if assertSourceContained(root.RealPath, rel) != nil {
			continue
		}
		base := filepath.Base(rel)
		// MatchFile does not exclude _test.go files itself, so we filter
		// them out here as FSSourceStore does (per spec line 1417).
		if strings.HasSuffix(base, "_test.go") {
			continue
		}
		matchDir := filepath.Join(root.RealPath, filepath.Dir(rel))
		match, matchErr := ctxt.MatchFile(matchDir, base)
		if matchErr != nil {
			// Build-constraint parse errors (per spec line 1420) fail with
			// unsupported_schema_construct using a generic diagnostic so
			// build-line source text is not echoed (spec line 1415).
			return nil, &Error{
				Code: CodeUnsupportedSchemaConstruct,
				Op:   "mem_source_store.list_source_files",
				Path: safeRenderPath(rel),
				Err:  fmt.Errorf("invalid build constraint"),
			}
		}
		if !match {
			continue
		}
		if len(out)+1 > lim.MaxSchemaFiles {
			return nil, &Error{
				Code: CodeResourceLimit,
				Op:   "mem_source_store.list_source_files",
				Err:  fmt.Errorf("schema file count exceeds limit %d", lim.MaxSchemaFiles),
			}
		}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out, nil
}

// ReadSourceFile returns a caller-owned copy of the file at root/relpath.
//
// Validations run in this order: ctx cancellation, .go suffix, containment,
// resolved limit fields, map lookup, then per-file size cap. The order
// matters for test assertions — an invalid limit combined with a bad
// relpath surfaces the path error first, not the config error.
//
// Non-Go relpaths (those without a .go suffix) are rejected with
// CodeInvalidPath, matching the production FSSourceStore contract that
// only .go files are valid schema inputs. Relpaths that escape the
// configured root (absolute paths or paths containing `..` segments that
// resolve outside root) are rejected with CodeInvalidPath, matching the
// containment enforcement used by FSSourceStore, so test seeds cannot
// bypass containment that production filesystem reads enforce.
func (s *MemSourceStore) ReadSourceFile(ctx context.Context, root SourceRoot, relpath string, opts ReadSourceFileOptions) (*SourceFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !strings.HasSuffix(relpath, ".go") {
		return nil, &Error{
			Code: CodeInvalidPath,
			Op:   "mem_source_store.read_source_file",
			Path: safeRenderPath(relpath),
			Err:  fmt.Errorf("only .go schema files are supported"),
		}
	}
	if err := assertSourceContained(root.RealPath, relpath); err != nil {
		return nil, &Error{
			Code: CodeInvalidPath,
			Op:   "mem_source_store.read_source_file",
			Path: safeRenderPath(relpath),
			Err:  err,
		}
	}
	lim := opts.Limits.resolve()
	if err := lim.Validate("mem_source_store.read_source_file"); err != nil {
		return nil, err
	}
	key := root.RealPath + "/" + relpath
	s.mu.Lock()
	data, ok := s.files[key]
	s.mu.Unlock()
	if !ok {
		return nil, &Error{
			Code: CodeInvalidPath,
			Op:   "mem_source_store.read_source_file",
			Path: safeRenderPath(relpath),
			Err:  fmt.Errorf("file not found"),
		}
	}
	if int64(len(data)) > lim.MaxSchemaSourceFileBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: "mem_source_store.read_source_file", Path: safeRenderPath(relpath)}
	}
	return &SourceFile{RelPath: relpath, Content: bytes.Clone(data)}, nil
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
