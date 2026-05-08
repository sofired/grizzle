// Package filemigrate implements Grizzle's RC.1-style file-based migration
// workflow. It is the Go equivalent of Drizzle Kit's generate/migrate/check/pull
// command surface.
//
// # Slice 0 scope
//
// This package currently contains only the foundation required by Slice 0 of
// the implementation sequence defined in docs/spec/file-migrations-implementation-sequence.md:
//
//   - Stable [ErrorCode] constants and [Error] / [ExecutionError] / [PartialApplicationError] types
//   - [CodeSentinel] values for programmatic errors.Is classification
//   - [Diagnostic] carrier with redaction rules
//   - [ResourceLimits] struct and production defaults
//   - [ArtifactStore] and [ManagedSourceStore] interfaces
//   - Filesystem-backed and in-memory store implementations
//
// Later slices will add artifact discovery, snapshot validation, check, generate,
// migrate, and pull implementations on top of this foundation.
//
// # Isolation constraint
//
// This package must not import or reuse the legacy kit.Snapshot, kit.SaveJSON,
// kit.LoadJSON, kit.MigrationsTable, or kit.ChecksumSQL symbols. Those are
// quarantined pending a separate cutover. The two code paths coexist in the
// repository until Slice 8 wires the CLI to this package.
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
package filemigrate
