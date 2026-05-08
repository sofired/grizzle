package filemigrate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FSArtifactStore is the production ArtifactStore backed by the real filesystem.
// All metadata checks use Lstat so symlinks are never followed.
type FSArtifactStore struct{}

// NewFSArtifactStore returns the production filesystem-backed ArtifactStore.
func NewFSArtifactStore() *FSArtifactStore {
	return &FSArtifactStore{}
}

var _ ArtifactStore = (*FSArtifactStore)(nil)

const fsOp = "artifact_store"

// ResolveRoot resolves dir against the real filesystem.
// It rejects symlinks, non-directory entries, and paths with unsafe components.
// Existing parent components of dir are walked with Lstat and rejected if any
// is a symlink, so a configured path like `<base>/link/migrations` (where
// `link` is a symlink) cannot redirect resolution outside the intended tree.
// Non-existent components are tolerated for the RootEnsureForWrite case.
func (s *FSArtifactStore) ResolveRoot(ctx context.Context, dir string, opts ResolveArtifactRootOptions) (ArtifactRoot, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactRoot{}, err
	}
	if dir == "" {
		return ArtifactRoot{}, newError(CodeInvalidConfig, fsOp+".resolve_root")
	}

	// Reject any symlink in the parent chain before Lstat / EvalSymlinks /
	// MkdirAll runs. A symlinked parent would otherwise be silently followed.
	if err := assertNoSymlinkInPathChain(dir); err != nil {
		return ArtifactRoot{}, &Error{
			Code: CodeInvalidPath,
			Op:   fsOp + ".resolve_root",
			Path: safeRenderPath(dir),
			Err:  err,
		}
	}

	fi, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		switch opts.Mode {
		case RootReadForCheck:
			return ArtifactRoot{Configured: dir, RealPath: dir, State: RootAbsent}, nil
		case RootReadForMigrate:
			return ArtifactRoot{}, &Error{
				Code: CodeEmptyMigrationsDir,
				Op:   fsOp + ".resolve_root",
				Path: safeRenderPath(dir),
			}
		case RootEnsureForWrite:
			if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
				return ArtifactRoot{}, &Error{
					Code: CodeInvalidPath,
					Op:   fsOp + ".resolve_root",
					Path: safeRenderPath(dir),
					Err:  fmt.Errorf("mkdir: %w", mkErr),
				}
			}
			real, realErr := filepath.EvalSymlinks(dir)
			if realErr != nil {
				return ArtifactRoot{}, newPathError(fsOp+".resolve_root", dir, realErr)
			}
			return ArtifactRoot{Configured: dir, RealPath: real, State: RootCreated}, nil
		default:
			return ArtifactRoot{}, newError(CodeInvalidConfig, fsOp+".resolve_root")
		}
	}
	if err != nil {
		return ArtifactRoot{}, newPathError(fsOp+".resolve_root", dir, err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return ArtifactRoot{}, &Error{
			Code: CodeInvalidPath,
			Op:   fsOp + ".resolve_root",
			Path: safeRenderPath(dir),
			Err:  fmt.Errorf("symlink roots are not supported"),
		}
	}
	if !fi.IsDir() {
		return ArtifactRoot{}, &Error{
			Code: CodeInvalidPath,
			Op:   fsOp + ".resolve_root",
			Path: safeRenderPath(dir),
			Err:  fmt.Errorf("not a directory"),
		}
	}
	real, realErr := filepath.EvalSymlinks(dir)
	if realErr != nil {
		return ArtifactRoot{}, newPathError(fsOp+".resolve_root", dir, realErr)
	}
	return ArtifactRoot{Configured: dir, RealPath: real, State: RootExisting}, nil
}

// ListArtifacts returns migration directory entries directly under root,
// sorted lexicographically. It enforces MaxArtifactDirEntries and MaxArtifacts.
func (s *FSArtifactStore) ListArtifacts(ctx context.Context, root ArtifactRoot, opts ListArtifactsOptions) ([]ArtifactEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root.State == RootAbsent {
		return nil, nil
	}

	lim := opts.Limits.resolve()
	if err := lim.Validate(fsOp + ".list_artifacts"); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root.RealPath)
	if err != nil {
		return nil, &Error{
			Code: CodeInvalidPath,
			Op:   fsOp + ".list_artifacts",
			Path: safeRenderPath(root.RealPath),
			Err:  err,
		}
	}
	if len(entries) > lim.MaxArtifactDirEntries {
		return nil, &Error{
			Code:      CodeResourceLimit,
			Op:        fsOp + ".list_artifacts",
			Migration: fmt.Sprintf("dir_entries=%d limit=%d", len(entries), lim.MaxArtifactDirEntries),
		}
	}

	var out []ArtifactEntry
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entryPath := filepath.Join(root.RealPath, e.Name())
		fi, statErr := os.Lstat(entryPath)
		if statErr != nil {
			return nil, newPathError(fsOp+".list_artifacts", entryPath, statErr)
		}
		// Symlinks (whether they target a file or directory) are rejected with
		// an explicit message so the failure mode is clear to operators.
		if fi.Mode()&fs.ModeSymlink != 0 {
			return nil, &Error{
				Code: CodeInvalidPath,
				Op:   fsOp + ".list_artifacts",
				Path: safeRenderPath(entryPath),
				Err:  fmt.Errorf("symlinks are not supported"),
			}
		}
		if !fi.IsDir() {
			if err := classifyArtifactRootSidecar(e.Name(), fi); err != nil {
				err.Op = fsOp + ".list_artifacts"
				err.Path = safeRenderPath(entryPath)
				return nil, err
			}
			continue
		}
		// Reserved staging directories from interrupted CreateArtifact runs are
		// ignored so they do not pollute discovery or trigger validation errors.
		if isReservedStagingDir(e.Name()) {
			continue
		}
		out = append(out, ArtifactEntry{
			Name: e.Name(),
			Path: entryPath,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	if len(out) > lim.MaxArtifacts {
		return nil, &Error{
			Code:      CodeResourceLimit,
			Op:        fsOp + ".list_artifacts",
			Migration: fmt.Sprintf("artifacts=%d limit=%d", len(out), lim.MaxArtifacts),
		}
	}
	return out, nil
}

// ReadArtifact reads migration.sql and snapshot.json from the named migration
// directory. It uses Lstat for all metadata, rejects symlinks, enforces byte
// caps, and returns caller-owned copies.
func (s *FSArtifactStore) ReadArtifact(ctx context.Context, root ArtifactRoot, name string, opts ReadArtifactOptions) (*LoadedArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateArtifactName(name); err != nil {
		return nil, &Error{Code: CodeInvalidMigrationName, Op: fsOp + ".read_artifact", Migration: name, Err: err}
	}
	if err := assertContained(root.RealPath, name); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".read_artifact", Path: safeRenderPath(name), Err: err}
	}

	lim := opts.Limits.resolve()
	if err := lim.Validate(fsOp + ".read_artifact"); err != nil {
		return nil, err
	}
	dir := filepath.Join(root.RealPath, name)

	// Lstat the artifact directory entry itself before reading children.
	// readArtifactFile only Lstats the final file path, so a directory entry
	// that is a symlink (pointing outside the configured root) would be
	// silently followed when opening migration.sql / snapshot.json.
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".read_artifact", Migration: name, Path: safeRenderPath(dir), Err: err}
	}
	if dirInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, &Error{
			Code:      CodeInvalidPath,
			Op:        fsOp + ".read_artifact",
			Migration: name,
			Path:      safeRenderPath(dir),
			Err:       fmt.Errorf("symlink artifact directories are not supported"),
		}
	}
	if !dirInfo.IsDir() {
		return nil, &Error{
			Code:      CodeInvalidPath,
			Op:        fsOp + ".read_artifact",
			Migration: name,
			Path:      safeRenderPath(dir),
			Err:       fmt.Errorf("not a directory"),
		}
	}

	sql, err := readArtifactFile(ctx, dir, "migration.sql", lim.MaxMigrationSQLBytes)
	if err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".read_artifact", Migration: name, Err: err}
	}
	snap, err := readArtifactFile(ctx, dir, "snapshot.json", lim.MaxSnapshotJSONBytes)
	if err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".read_artifact", Migration: name, Err: err}
	}

	digests := computeArtifactDigest(sql, snap)
	return &LoadedArtifact{
		Name:         name,
		Dir:          dir,
		MigrationSQL: sql,
		SnapshotJSON: snap,
		Digests:      digests,
	}, nil
}

// CreateArtifact atomically creates a new migration artifact directory under
// root using a temporary sibling + rename approach.
func (s *FSArtifactStore) CreateArtifact(ctx context.Context, root ArtifactRoot, artifact NewArtifact, opts CreateArtifactOptions) (*LoadedArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateArtifactName(artifact.Name); err != nil {
		return nil, &Error{Code: CodeInvalidMigrationName, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	if err := assertContained(root.RealPath, artifact.Name); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Path: safeRenderPath(artifact.Name), Err: err}
	}

	lim := opts.Limits.resolve()
	if err := lim.Validate(fsOp + ".create_artifact"); err != nil {
		return nil, err
	}
	if int64(len(artifact.MigrationSQL)) > lim.MaxMigrationSQLBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: fsOp + ".create_artifact", Migration: artifact.Name}
	}
	if int64(len(artifact.SnapshotJSON)) > lim.MaxSnapshotJSONBytes {
		return nil, &Error{Code: CodeResourceLimit, Op: fsOp + ".create_artifact", Migration: artifact.Name}
	}

	target := filepath.Join(root.RealPath, artifact.Name)
	// Fail if a directory already exists with this name.
	if _, err := os.Lstat(target); err == nil {
		return nil, &Error{Code: CodeDuplicateMigration, Op: fsOp + ".create_artifact", Migration: artifact.Name}
	}

	// Stage writes in a temporary sibling directory, then rename atomically.
	tmp, err := os.MkdirTemp(root.RealPath, ".grizzle-staging-*")
	if err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	// Clean up the temp dir on any failure path.
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(tmp)
		}
	}()

	if err := writeFile(filepath.Join(tmp, "migration.sql"), artifact.MigrationSQL); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	if err := writeFile(filepath.Join(tmp, "snapshot.json"), artifact.SnapshotJSON); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}

	if err := os.Rename(tmp, target); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	committed = true

	digests := computeArtifactDigest(artifact.MigrationSQL, artifact.SnapshotJSON)
	sql := bytes.Clone(artifact.MigrationSQL)
	snap := bytes.Clone(artifact.SnapshotJSON)
	return &LoadedArtifact{
		Name:         artifact.Name,
		Dir:          target,
		MigrationSQL: sql,
		SnapshotJSON: snap,
		Digests:      digests,
	}, nil
}

// readArtifactFile reads path using Lstat (no follow), validates it is a
// regular file, and reads at most maxBytes bytes. Returns a caller-owned copy.
func readArtifactFile(ctx context.Context, dir, name string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name)
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("lstat %s: %w", name, err)
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s: symlinks are not supported", name)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", name)
	}
	if fi.Size() > maxBytes {
		return nil, fmt.Errorf("%s: file size %d exceeds limit %d", name, fi.Size(), maxBytes)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s: content exceeds limit %d", name, maxBytes)
	}
	return data, nil
}

// writeFile writes data to path, creating or truncating the file.
func writeFile(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// allowedRootSidecars lists the only root-level sidecar regular files that are
// silently ignored during artifact discovery, per the artifacts spec.
var allowedRootSidecars = map[string]struct{}{
	".gitkeep":  {},
	".DS_Store": {},
	"README":    {},
	"README.md": {},
}

// classifyArtifactRootSidecar returns nil if name is an explicitly allowed
// root-level sidecar that should be silently ignored. Otherwise it returns an
// *Error with the appropriate spec-mandated code (Op and Path are filled in by
// the caller). fi describes the entry as observed via Lstat. Callers must
// have already filtered out symlinks; the remaining non-regular cases here
// are sockets, FIFOs, and device nodes.
func classifyArtifactRootSidecar(name string, fi fs.FileInfo) *Error {
	// Non-regular non-directory entries (sockets, fifos, devices) fail with
	// invalid_path before any allowlist check.
	if !fi.Mode().IsRegular() {
		return &Error{
			Code: CodeInvalidPath,
			Err:  fmt.Errorf("not a regular file"),
		}
	}
	if _, ok := allowedRootSidecars[name]; ok {
		return nil
	}
	return &Error{
		Code: CodeUnsupportedArtifactFormat,
		Err:  fmt.Errorf("unexpected root-level file %q", name),
	}
}

// isReservedStagingDir reports whether name matches the reserved staging-dir
// pattern used by CreateArtifact (.grizzle-staging-*). Such directories may
// be left over from interrupted writes and must not be treated as artifacts.
func isReservedStagingDir(name string) bool {
	return strings.HasPrefix(name, ".grizzle-staging-")
}

// validateArtifactName returns an error if name is not a valid single-component
// migration directory name. It rejects empty names, path separators, dot
// segments, absolute paths, and names that would escape the root.
func validateArtifactName(name string) error {
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("name must not be an absolute path")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("name must not contain path separators")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("name must not be a dot segment")
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("name must not start with a dot")
	}
	return nil
}

// assertContained ensures that name, when joined with root, does not escape
// root via path traversal. name must already have passed validateArtifactName.
func assertContained(root, name string) error {
	full := filepath.Join(root, name)
	if !strings.HasPrefix(full+string(filepath.Separator), root+string(filepath.Separator)) {
		return fmt.Errorf("path escapes root")
	}
	return nil
}

// assertNoSymlinkInPathChain Lstats every existing component of dir from the
// filesystem root down. It returns an error if any existing component is a
// symlink. Components that do not exist (ENOENT) are tolerated — the walk
// stops there because only existing components can be symlinks. This is the
// configured-path equivalent of [assertNoSymlinkInChain] (which works on
// relative paths under a trusted root).
func assertNoSymlinkInPathChain(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)

	// Walk from the path itself up to the volume root, then iterate from root
	// downward. filepath.Dir terminates when parent == self (e.g., "/" or "C:\").
	var paths []string
	cur := abs
	for {
		paths = append(paths, cur)
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	// paths is now [abs, parent, ..., root]; iterate from root down, skipping
	// the volume root itself (which is conceptually outside the user-controlled
	// tree and on Unix is just "/").
	for i := len(paths) - 2; i >= 0; i-- {
		p := paths[i]
		fi, statErr := os.Lstat(p)
		if errors.Is(statErr, fs.ErrNotExist) {
			// Remaining (deeper) components cannot exist either, so they
			// cannot be symlinks. Permit creation by callers that mkdir.
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlink in path component %q is not supported", p)
		}
	}
	return nil
}
