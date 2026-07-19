// Package filemigrate implements Grizzle's RC.1-style file-based migration
// workflow. It is the Go equivalent of Drizzle Kit's generate/migrate/check/pull
// command surface.
//
// # Current foundation
//
// This package currently contains the foundation for the implementation
// sequence defined in docs/spec/file-migrations-implementation-sequence.md:
//
//   - Stable [ErrorCode] constants and [Error] / [ExecutionError] / [PartialApplicationError] types
//   - [CodeSentinel] values for programmatic errors.Is classification
//   - [Diagnostic] carrier with redaction rules
//   - [ResourceLimits] struct and production defaults
//   - [ArtifactStore] and [ManagedSourceStore] interfaces
//   - Filesystem-backed and in-memory store implementations
//
// Artifact discovery, snapshot validation, check, generate, migrate, and pull
// implementations build on this foundation.
//
// # Isolation constraint
//
// This package must not import or reuse the legacy kit.Snapshot, kit.SaveJSON,
// kit.LoadJSON, kit.MigrationsTable, or kit.ChecksumSQL symbols. Those are
// quarantined pending a separate cutover. The two code paths coexist in the
// repository until the CLI cutover is complete.
//
// # Error contract
//
// All errors returned by this package are *[Error], *[ExecutionError], or
// *[PartialApplicationError]. Callers may use errors.Is with any [CodeSentinel]
// variable (e.g. [ErrInvalidPath]) to classify errors by code, or errors.As to
// recover the typed struct for structured fields.
//
// Diagnostic.Message and Diagnostic.Path are safe-rendered fields. They must
// not contain credentials, DSNs, raw SQL text, bind values, matched secret
// literals, or full database object names from broad introspection scans.
//
// # Filesystem safety
//
// Filesystem-backed stores use [os.Root] handles and relative operations for
// traversal, reads, staging writes, cleanup, and publication. Each entry opened
// for use is checked with Lstat, opened relative to its already-open parent,
// and identity-checked after open. This prevents a symlink or rename swap
// between metadata validation and use from redirecting an operation outside
// its configured root.
//
// This is a path-containment guarantee, not a general filesystem sandbox. It
// assumes a trusted mount namespace and does not prevent crossing filesystem
// boundaries or bind mounts, or separately authorized access through special
// trees such as /proc. Non-regular migration inputs and discovered .go source
// files, including Unix devices, are rejected, but callers must not treat the
// configured root as an operating-system isolation boundary.
//
// Go documents os.Root as TOCTOU-vulnerable on js and as path-based across
// directory renames on plan9. Filesystem-backed stores therefore fail closed
// with [CodeInvalidPath] before filesystem access on those platforms. The
// in-memory stores remain contract-compatible and available there.
package filemigrate
