package filemigrate

import "context"

// SourceFile holds the content of a single schema source file.
// Content is a caller-owned defensive copy.
type SourceFile struct {
	RelPath string
	Content []byte
}

// SourceStore is the interface through which filemigrate reads schema source
// files (the input side of generate/check). The production implementation uses
// the real filesystem; tests may use the in-memory implementation returned by
// NewMemSourceStore.
//
// All implementations must:
//   - reject paths that escape the configured root (containment)
//   - return caller-owned defensive byte copies
//   - enforce ResourceLimits carried by each operation's options
//
// Filesystem-backed implementations must additionally use handle-relative
// traversal and Lstat/open identity checks, and reject symlinked roots,
// directories, and files.
type SourceStore interface {
	// ResolveSourceRoot resolves dir as the schema source root. It must securely
	// open the path, reject symlinked roots, and return a SourceRoot describing
	// the resolved real path.
	ResolveSourceRoot(ctx context.Context, dir string) (SourceRoot, error)

	// ListSourceFiles returns the relative paths of schema source files under
	// root, subject to opts.Limits. Implementations must skip generated output
	// files where detectable by the managed header, and reject discovered
	// symlinked or non-regular files.
	ListSourceFiles(ctx context.Context, root SourceRoot, opts ListSourceFilesOptions) ([]string, error)

	// ReadSourceFile reads the named source file within root. It must Lstat the
	// file, reject non-regular/symlinked files without a validate/open race,
	// enforce byte caps, and return a caller-owned copy.
	ReadSourceFile(ctx context.Context, root SourceRoot, relpath string, opts ReadSourceFileOptions) (*SourceFile, error)
}

// ListSourceFilesOptions carries options for SourceStore.ListSourceFiles.
type ListSourceFilesOptions struct {
	Limits ResourceLimits
}

// ReadSourceFileOptions carries options for SourceStore.ReadSourceFile.
type ReadSourceFileOptions struct {
	Limits ResourceLimits
}
