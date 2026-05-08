package filemigrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
)

// MemManagedSourceStore is a thread-safe in-memory ManagedSourceStore for tests.
// It performs no filesystem I/O and requires no cleanup.
type MemManagedSourceStore struct {
	mu    sync.Mutex
	files map[string][]byte // key: root + "/" + relpath
}

// NewMemManagedSourceStore returns an initialized in-memory ManagedSourceStore.
func NewMemManagedSourceStore() *MemManagedSourceStore {
	return &MemManagedSourceStore{files: make(map[string][]byte)}
}

var _ ManagedSourceStore = (*MemManagedSourceStore)(nil)

// AddFile stores content under root/relpath without ownership checks. It is
// intended for test setup only — use WriteManagedFile for production-like writes.
func (s *MemManagedSourceStore) AddFile(root, relpath string, content []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[root+"/"+relpath] = bytes.Clone(content)
}

// ResolveSourceRoot always succeeds for non-empty dir values.
func (s *MemManagedSourceStore) ResolveSourceRoot(ctx context.Context, dir string) (SourceRoot, error) {
	if err := ctx.Err(); err != nil {
		return SourceRoot{}, err
	}
	if dir == "" {
		return SourceRoot{}, newInvalidConfigError("mem_managed_source_store.resolve_source_root")
	}
	return SourceRoot{Configured: dir, RealPath: dir}, nil
}

// WriteManagedFile writes content to relpath under root. If a file already
// exists at that path and does not start with opts.Header, it is rejected with
// CodeManagedFileOverwrite. opts.Header must be non-empty: the
// ManagedSourceStore contract permits overwriting only files carrying the
// recognized managed header, so an empty header (which would treat every
// existing file as managed) is rejected with CodeInvalidConfig before any
// overwrite decision runs. All opts.Limits fields are validated before any
// size comparison runs; any negative value is rejected with CodeInvalidConfig.
// Limits.MaxRenderedSourceFileBytes is enforced per file. Written is true
// when the stored content differs from the incoming content.
func (s *MemManagedSourceStore) WriteManagedFile(ctx context.Context, root SourceRoot, relpath string, content []byte, opts ManagedWriteOptions) (ManagedFile, error) {
	if err := ctx.Err(); err != nil {
		return ManagedFile{}, err
	}
	if opts.Header == "" {
		return ManagedFile{}, &Error{
			Code: CodeInvalidConfig,
			Op:   "mem_managed_source_store.write_managed_file",
			Path: safeRenderPath(relpath),
			Err:  fmt.Errorf("ManagedWriteOptions.Header must not be empty"),
		}
	}
	if err := assertSourceContained(root.RealPath, relpath); err != nil {
		return ManagedFile{}, &Error{
			Code: CodeInvalidPath,
			Op:   "mem_managed_source_store.write_managed_file",
			Path: safeRenderPath(relpath),
			Err:  err,
		}
	}
	lim := opts.Limits.resolve()
	if err := lim.Validate("mem_managed_source_store.write_managed_file"); err != nil {
		return ManagedFile{}, err
	}
	if int64(len(content)) > lim.MaxRenderedSourceFileBytes {
		return ManagedFile{}, &Error{
			Code: CodeResourceLimit,
			Op:   "mem_managed_source_store.write_managed_file",
			Path: safeRenderPath(relpath),
		}
	}

	key := root.RealPath + "/" + relpath

	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.files[key]
	if exists && !strings.HasPrefix(string(existing), opts.Header) {
		return ManagedFile{}, &Error{
			Code: CodeManagedFileOverwrite,
			Op:   "mem_managed_source_store.write_managed_file",
			Path: safeRenderPath(relpath),
			Err:  fmt.Errorf("file exists and is not managed by Grizzle"),
		}
	}

	written := !exists || !bytes.Equal(existing, content)
	s.files[key] = bytes.Clone(content)

	digest := sha256.Sum256(content)
	return ManagedFile{
		RelPath:       relpath,
		ContentSHA256: Digest(digest),
		Written:       written,
	}, nil
}
