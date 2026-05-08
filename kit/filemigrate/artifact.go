package filemigrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
)

// managedIntrospectionHeader is the exact line that marks an artifact as
// managed-introspection per the file-migrations spec. The detection rule
// requires this to be the first non-empty physical line of migration.sql,
// after an optional UTF-8 BOM. See:
//   - docs/spec/file-migrations-artifacts.md:128
//   - docs/spec/pull.md:504
const managedIntrospectionHeader = "-- grizzle:managed-introspection v1"

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// hasManagedIntrospectionHeader reports whether sql is a managed-introspection
// migration: its first non-empty physical line, after an optional UTF-8 BOM,
// is exactly managedIntrospectionHeader. Physical lines are split on LF; a
// trailing CR (CRLF input) is tolerated for the matched header line.
func hasManagedIntrospectionHeader(sql []byte) bool {
	b := bytes.TrimPrefix(sql, utf8BOM)
	for {
		nl := bytes.IndexByte(b, '\n')
		var line []byte
		if nl < 0 {
			line = b
		} else {
			line = b[:nl]
		}
		// Tolerate CRLF: drop a trailing CR before comparison.
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		if len(line) > 0 {
			return string(line) == managedIntrospectionHeader
		}
		if nl < 0 {
			return false
		}
		b = b[nl+1:]
	}
}

// Digest is a raw SHA-256 digest value.
type Digest [32]byte

// HashHex is a hex-encoded digest string.
type HashHex string

// ArtifactDigest holds the individual and combined digests for a migration artifact.
// CombinedSHA256 is computed as:
//
//	SHA256(
//	  "grizzle-artifact-v1" || 0x00 ||
//	  "migration.sql" || 0x00 || uint64be(len(migration.sql)) || raw migration.sql bytes ||
//	  "snapshot.json" || 0x00 || uint64be(len(snapshot.json)) || raw snapshot.json bytes
//	)
type ArtifactDigest struct {
	MigrationSQLSHA256 Digest
	SnapshotJSONSHA256 Digest
	CombinedSHA256     Digest
}

// computeArtifactDigest computes the three digests for a migration artifact.
func computeArtifactDigest(migrationSQL, snapshotJSON []byte) ArtifactDigest {
	var d ArtifactDigest
	d.MigrationSQLSHA256 = sha256.Sum256(migrationSQL)
	d.SnapshotJSONSHA256 = sha256.Sum256(snapshotJSON)

	h := sha256.New()
	h.Write([]byte("grizzle-artifact-v1"))
	h.Write([]byte{0x00})
	h.Write([]byte("migration.sql"))
	h.Write([]byte{0x00})
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(len(migrationSQL)))
	h.Write(buf[:])
	h.Write(migrationSQL)
	h.Write([]byte("snapshot.json"))
	h.Write([]byte{0x00})
	binary.BigEndian.PutUint64(buf[:], uint64(len(snapshotJSON)))
	h.Write(buf[:])
	h.Write(snapshotJSON)
	d.CombinedSHA256 = Digest(h.Sum(nil))
	return d
}

// LoadedArtifact holds the validated, caller-owned bytes of a migration artifact.
// Byte slices are defensive copies — the store does not retain them after return.
//
// ManagedIntrospection is true when MigrationSQL carries the managed
// introspection header — i.e., its first non-empty physical line, after an
// optional UTF-8 BOM, is exactly "-- grizzle:managed-introspection v1". See
// docs/spec/file-migrations-artifacts.md:128 and docs/spec/pull.md:504. The
// downstream `migrate` flow uses this flag to reject pending/unrecorded
// bootstrap artifacts with bootstrap_init_required.
type LoadedArtifact struct {
	Name                 string
	Dir                  string
	MigrationSQL         []byte
	SnapshotJSON         []byte
	Digests              ArtifactDigest
	ManagedIntrospection bool
}

// ArtifactEntry is a discovered migration directory name and its path.
type ArtifactEntry struct {
	Name string
	Path string
}

// NewArtifact is the payload for creating a new migration artifact.
type NewArtifact struct {
	Name         string
	MigrationSQL []byte
	SnapshotJSON []byte
}

// ArtifactRootMode controls what the store is expected to do when the
// configured migrations directory does not exist.
type ArtifactRootMode string

const (
	// RootReadForCheck allows an absent migrations directory; the returned
	// ArtifactRoot carries State: RootAbsent so check can model an empty graph.
	RootReadForCheck ArtifactRootMode = "read_for_check"

	// RootReadForMigrate fails for an absent migrations directory unless the
	// caller has selected the controlled AllowEmpty path.
	RootReadForMigrate ArtifactRootMode = "read_for_migrate"

	// RootEnsureForWrite creates an absent root with write-safety rules and
	// returns State: RootCreated.
	RootEnsureForWrite ArtifactRootMode = "ensure_for_write"
)

// ArtifactRootState reports whether the configured migrations directory
// was absent, already existed, or was created by the store.
type ArtifactRootState string

const (
	RootAbsent   ArtifactRootState = "absent"
	RootExisting ArtifactRootState = "existing"
	RootCreated  ArtifactRootState = "created"
)

// ArtifactRoot is the resolved migrations root returned by ArtifactStore.ResolveRoot.
type ArtifactRoot struct {
	Configured string
	RealPath   string
	State      ArtifactRootState
}

// ResolveArtifactRootOptions carries options for ArtifactStore.ResolveRoot.
type ResolveArtifactRootOptions struct {
	Mode   ArtifactRootMode
	Limits ResourceLimits
}

// ListArtifactsOptions carries options for ArtifactStore.ListArtifacts.
type ListArtifactsOptions struct {
	Limits ResourceLimits
}

// ReadArtifactOptions carries options for ArtifactStore.ReadArtifact.
type ReadArtifactOptions struct {
	Limits ResourceLimits
}

// CreateArtifactOptions carries options for ArtifactStore.CreateArtifact.
type CreateArtifactOptions struct {
	Limits ResourceLimits
}

// ArtifactStore is the interface through which filemigrate reads and writes
// migration artifacts. The production implementation uses a real filesystem;
// tests may use the in-memory implementation returned by NewMemArtifactStore.
//
// All implementations must:
//   - use Lstat (no symlink follow) when checking file/directory metadata
//   - reject symlinked roots, artifact directories, and artifact files
//   - reject paths that escape the configured root (containment)
//   - return caller-owned defensive byte copies from ReadArtifact
//   - enforce the ResourceLimits carried by each operation's Options
type ArtifactStore interface {
	// ResolveRoot resolves the configured migrations directory.
	// It must Lstat the path and reject symlinked roots.
	ResolveRoot(ctx context.Context, dir string, opts ResolveArtifactRootOptions) (ArtifactRoot, error)

	// ListArtifacts returns the names of the immediate-child migration
	// directories under root, sorted lexicographically. It must enforce
	// opts.Limits.MaxArtifacts and MaxArtifactDirEntries.
	ListArtifacts(ctx context.Context, root ArtifactRoot, opts ListArtifactsOptions) ([]ArtifactEntry, error)

	// ReadArtifact reads and validates the migration.sql and snapshot.json
	// files inside the named migration directory. It must Lstat both files,
	// reject non-regular/symlinked files, and enforce opts.Limits byte caps.
	ReadArtifact(ctx context.Context, root ArtifactRoot, name string, opts ReadArtifactOptions) (*LoadedArtifact, error)

	// CreateArtifact writes a new migration artifact directory using atomic
	// publish semantics. It must validate the name before constructing any
	// path, and must enforce opts.Limits byte caps before staging writes.
	CreateArtifact(ctx context.Context, root ArtifactRoot, artifact NewArtifact, opts CreateArtifactOptions) (*LoadedArtifact, error)
}

// ManagedFile is the post-write result returned after WriteManagedFile succeeds.
type ManagedFile struct {
	RelPath       string
	ContentSHA256 Digest
	Written       bool
}

// ManagedWriteOptions carries options for ManagedSourceStore.WriteManagedFile.
type ManagedWriteOptions struct {
	Header string
	Limits ResourceLimits
}

// SourceRoot is the resolved schema-output root returned by
// ManagedSourceStore.ResolveSourceRoot.
type SourceRoot struct {
	Configured string
	RealPath   string
}

// ManagedSourceStore writes managed Go source files to the schema-output
// directory. It is separate from ArtifactStore because schema-out and
// migrations-out are independent roots with different file-ownership rules.
type ManagedSourceStore interface {
	// ResolveSourceRoot resolves and optionally creates the schema output
	// directory, rejecting symlinked roots.
	ResolveSourceRoot(ctx context.Context, dir string) (SourceRoot, error)

	// WriteManagedFile writes content to relpath under root. It must reject
	// unowned files (not carrying the recognized managed header) with
	// CodeManagedFileOverwrite, enforce opts.Limits, and return the post-write
	// ManagedFile digest and status.
	WriteManagedFile(ctx context.Context, root SourceRoot, relpath string, content []byte, opts ManagedWriteOptions) (ManagedFile, error)
}
