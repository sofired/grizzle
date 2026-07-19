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
// Security-sensitive operations are rooted at open directory handles and
// identity-check entries across metadata inspection and open.
type FSArtifactStore struct{}

// NewFSArtifactStore returns the production filesystem-backed ArtifactStore.
func NewFSArtifactStore() *FSArtifactStore {
	return &FSArtifactStore{}
}

var _ ArtifactStore = (*FSArtifactStore)(nil)

const fsOp = "artifact_store"

// ResolveRoot resolves dir against the real filesystem. It walks from the
// volume root with handle-relative opens, rejecting symlinks, non-directory
// entries, and entries whose identity changes while they are opened.
func (s *FSArtifactStore) ResolveRoot(ctx context.Context, dir string, opts ResolveArtifactRootOptions) (ArtifactRoot, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactRoot{}, err
	}
	if dir == "" {
		return ArtifactRoot{}, newInvalidConfigError(fsOp + ".resolve_root")
	}

	rootHandle, real, _, err := openSecureRootPath(dir, false, 0)
	if errors.Is(err, fs.ErrNotExist) {
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
			var created bool
			rootHandle, real, created, err = openSecureRootPath(dir, true, 0o750)
			if err != nil {
				return ArtifactRoot{}, &Error{
					Code: CodeInvalidPath,
					Op:   fsOp + ".resolve_root",
					Path: safeRenderPath(dir),
					Err:  fmt.Errorf("secure mkdir: %w", err),
				}
			}
			_ = rootHandle.Close()
			state := RootExisting
			if created {
				state = RootCreated
			}
			return ArtifactRoot{Configured: dir, RealPath: real, State: state}, nil
		default:
			return ArtifactRoot{}, newInvalidConfigError(fsOp + ".resolve_root")
		}
	}
	if err != nil {
		return ArtifactRoot{}, &Error{
			Code: CodeInvalidPath,
			Op:   fsOp + ".resolve_root",
			Path: safeRenderPath(dir),
			Err:  err,
		}
	}
	_ = rootHandle.Close()
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
	rootHandle, _, _, err := openSecureRootPath(root.RealPath, false, 0)
	if err != nil {
		return nil, &Error{
			Code: CodeInvalidPath,
			Op:   fsOp + ".list_artifacts",
			Path: safeRenderPath(root.RealPath),
			Err:  err,
		}
	}
	defer func() { _ = rootHandle.Close() }()

	entries, err := readSecureDir(rootHandle)
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
		fi, statErr := rootHandle.Lstat(e.Name())
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
// directory. It securely opens and identity-checks each entry, rejects
// symlinks, enforces byte caps, and returns caller-owned copies.
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
	rootHandle, _, _, err := openSecureRootPath(root.RealPath, false, 0)
	if err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".read_artifact", Migration: name, Path: safeRenderPath(root.RealPath), Err: err}
	}
	defer func() { _ = rootHandle.Close() }()
	artifactDir, _, err := openSecureDir(rootHandle, name, nil)
	if err != nil {
		return nil, &Error{
			Code:      CodeInvalidPath,
			Op:        fsOp + ".read_artifact",
			Migration: name,
			Path:      safeRenderPath(dir),
			Err:       err,
		}
	}
	defer func() { _ = artifactDir.Close() }()

	sql, err := readArtifactFile(ctx, artifactDir, "migration.sql", lim.MaxMigrationSQLBytes)
	if err != nil {
		return nil, &Error{Code: artifactReadErrorCode(err), Op: fsOp + ".read_artifact", Migration: name, Err: err}
	}
	snap, err := readArtifactFile(ctx, artifactDir, "snapshot.json", lim.MaxSnapshotJSONBytes)
	if err != nil {
		return nil, &Error{Code: artifactReadErrorCode(err), Op: fsOp + ".read_artifact", Migration: name, Err: err}
	}

	digests := computeArtifactDigest(sql, snap)
	return &LoadedArtifact{
		Name:                 name,
		Dir:                  dir,
		MigrationSQL:         sql,
		SnapshotJSON:         snap,
		Digests:              digests,
		ManagedIntrospection: hasManagedIntrospectionHeader(sql),
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
	rootHandle, _, _, err := openSecureRootPath(root.RealPath, false, 0)
	if err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	defer func() { _ = rootHandle.Close() }()

	// Fail if a directory already exists with this name.
	if _, err := rootHandle.Lstat(artifact.Name); err == nil {
		return nil, &Error{Code: CodeDuplicateMigration, Op: fsOp + ".create_artifact", Migration: artifact.Name}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}

	// Stage writes in a temporary sibling directory, then rename atomically.
	tmpName, staging, err := createSecureTempDir(rootHandle, ".grizzle-staging-")
	if err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	// Clean up the temp dir on any failure path.
	committed := false
	published := false
	defer func() {
		_ = staging.Close()
		if !committed {
			cleanupName := tmpName
			if published {
				cleanupName = artifact.Name
			}
			_ = rootHandle.RemoveAll(cleanupName)
		}
	}()

	if err := writeSecureNewFile(staging, "migration.sql", artifact.MigrationSQL); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	if err := writeSecureNewFile(staging, "snapshot.json", artifact.SnapshotJSON); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	stagingInfo, err := staging.Stat(".")
	if err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	if err := verifySecureDirIdentity(rootHandle, tmpName, stagingInfo); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	if hooks, ok := ctx.Value(secureFSTestHooksKey{}).(secureFSTestHooks); ok && hooks.beforeArtifactPublish != nil {
		// The test hook intentionally runs in the last pathname race window:
		// after staging identity verification and before the atomic rename.
		hooks.beforeArtifactPublish(tmpName)
	}

	if err := rootHandle.Rename(tmpName, artifact.Name); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	published = true
	if err := verifySecureDirIdentity(rootHandle, artifact.Name, stagingInfo); err != nil {
		return nil, &Error{Code: CodeInvalidPath, Op: fsOp + ".create_artifact", Migration: artifact.Name, Err: err}
	}
	committed = true

	digests := computeArtifactDigest(artifact.MigrationSQL, artifact.SnapshotJSON)
	sql := bytes.Clone(artifact.MigrationSQL)
	snap := bytes.Clone(artifact.SnapshotJSON)
	return &LoadedArtifact{
		Name:                 artifact.Name,
		Dir:                  target,
		MigrationSQL:         sql,
		SnapshotJSON:         snap,
		Digests:              digests,
		ManagedIntrospection: hasManagedIntrospectionHeader(sql),
	}, nil
}

// readArtifactFile securely opens name relative to dir, validates the opened
// handle is the same regular file observed by Lstat, and reads at most
// maxBytes bytes. Returns a caller-owned copy.
func readArtifactFile(ctx context.Context, dir *os.Root, name string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var afterLstat func()
	if hooks, ok := ctx.Value(secureFSTestHooksKey{}).(secureFSTestHooks); ok && hooks.afterArtifactFileLstat != nil {
		afterLstat = func() { hooks.afterArtifactFileLstat(name) }
	}
	f, fi, err := openSecureFile(dir, name, afterLstat)
	if err != nil {
		return nil, fmt.Errorf("secure open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", name)
	}
	if hasExtraHardLinks(fi) {
		return nil, fmt.Errorf("%s: hard links are not supported", name)
	}
	if fi.Size() > maxBytes {
		return nil, fmt.Errorf("%s: file size %d exceeds limit %d: %w", name, fi.Size(), maxBytes, ErrResourceLimit)
	}

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s: content exceeds limit %d: %w", name, maxBytes, ErrResourceLimit)
	}
	return data, nil
}

// artifactReadErrorCode classifies an error returned by readArtifactFile so the
// caller can wrap it with the right ErrorCode. Size-cap failures are reported
// as resource_limit (per the artifacts spec), and everything else as
// invalid_path (lstat / open / read I/O failures, type-shape rejections).
func artifactReadErrorCode(err error) ErrorCode {
	if errors.Is(err, ErrResourceLimit) {
		return CodeResourceLimit
	}
	return CodeInvalidPath
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
//
// The check is performed via filepath.Rel rather than a string-prefix
// comparison so that relative roots (notably "." for the current directory)
// are handled correctly: filepath.Join(".", "name") cleans to "name", which
// would fail a "name has prefix ./" check even though it is contained.
func assertContained(root, name string) error {
	full := filepath.Clean(filepath.Join(root, name))
	rel, err := filepath.Rel(filepath.Clean(root), full)
	if err != nil {
		return fmt.Errorf("path escapes root")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes root")
	}
	return nil
}
