package filemigrate

import (
	"bytes"
	"context"
	"fmt"
	"sort"
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

// ListSourceFiles returns the relative paths of files stored under root,
// enforcing MaxSchemaFiles.
func (s *MemSourceStore) ListSourceFiles(ctx context.Context, root SourceRoot, opts ListSourceFilesOptions) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lim := opts.Limits.resolve()
	if err := lim.Validate("mem_source_store.list_source_files"); err != nil {
		return nil, err
	}
	prefix := root.RealPath + "/"
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []string
	for k := range s.files {
		if !hasPrefix(k, prefix) {
			continue
		}
		rel := k[len(prefix):]
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
func (s *MemSourceStore) ReadSourceFile(ctx context.Context, root SourceRoot, relpath string, opts ReadSourceFileOptions) (*SourceFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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
