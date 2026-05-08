package filemigrate

import (
	"fmt"
	"strings"
)

// ErrorCode is a stable string code that classifies a filemigrate error.
// Values are stable across patch releases once this API is published.
type ErrorCode string

// DiagnosticCode classifies a non-fatal [Diagnostic] produced during a
// filemigrate operation.
type DiagnosticCode string

// DiagnosticSeverity is the severity level of a [Diagnostic].
type DiagnosticSeverity string

const (
	DiagnosticInfo    DiagnosticSeverity = "info"
	DiagnosticWarning DiagnosticSeverity = "warning"
)

const (
	DiagnosticAssumeQuiescent    DiagnosticCode = "assume_quiescent"
	DiagnosticBroadIntrospection DiagnosticCode = "broad_introspection"
	DiagnosticSecretLiteral      DiagnosticCode = "secret_literal"
)

// ErrorCode constants. Each constant has a matching Err... sentinel below.
const (
	CodeUnsupportedArtifactFormat    ErrorCode = "unsupported_artifact_format"
	CodeUnsupportedHistorySchema     ErrorCode = "unsupported_history_schema"
	CodeMalformedSnapshot            ErrorCode = "malformed_snapshot"
	CodeObsoleteSnapshotVersion      ErrorCode = "obsolete_snapshot_version"
	CodeUnsupportedSnapshotVersion   ErrorCode = "unsupported_snapshot_version"
	CodeNonCommutativeConflict       ErrorCode = "non_commutative_conflict"
	CodeMigrationIdentityDrift       ErrorCode = "migration_identity_drift"
	CodeMigrationExecution           ErrorCode = "migration_execution"
	CodeMigrationLock                ErrorCode = "migration_lock"
	CodeDuplicateMigration           ErrorCode = "duplicate_migration"
	CodeHistoryArtifactMissing       ErrorCode = "history_artifact_missing"
	CodeInvalidMigrationName         ErrorCode = "invalid_migration_name"
	CodeDialectMismatch              ErrorCode = "dialect_mismatch"
	CodeInvalidIdentifier            ErrorCode = "invalid_identifier"
	CodeInvalidPath                  ErrorCode = "invalid_path"
	CodeSecretLiteral                ErrorCode = "secret_literal"
	CodeInitPrecondition             ErrorCode = "init_precondition"
	CodeEmptyMigrationsDir           ErrorCode = "empty_migrations_dir"
	CodeBootstrapInitRequired        ErrorCode = "bootstrap_init_required"
	CodePartialApplication           ErrorCode = "partial_application"
	CodeInteractiveRequired          ErrorCode = "interactive_required"
	CodeUnsupportedObjectFamily      ErrorCode = "unsupported_object_family"
	CodeUnsupportedFeature           ErrorCode = "unsupported_feature"
	CodeUnsupportedSchemaConstruct   ErrorCode = "unsupported_schema_construct"
	CodeUnsupportedDialect           ErrorCode = "unsupported_dialect"
	CodeInvalidConfig                ErrorCode = "invalid_config"
	CodeConfigCollision              ErrorCode = "config_collision"
	CodeManagedFileOverwrite         ErrorCode = "managed_file_overwrite"
	CodeBroadIntrospectionOptIn      ErrorCode = "broad_introspection_requires_opt_in"
	CodeInvalidStatementSegmentation ErrorCode = "invalid_statement_segmentation"
	CodeIdentifierCollision          ErrorCode = "identifier_collision"
	CodeInvalidMigrationGraph        ErrorCode = "invalid_migration_graph"
	CodeBeforeWriteAborted           ErrorCode = "before_write_aborted"
	CodeResourceLimit                ErrorCode = "resource_limit"
)

// CodeSentinel is a lightweight error value used exclusively with errors.Is to
// classify filemigrate errors by their stable ErrorCode.
//
// Example:
//
//	if errors.Is(err, ErrInvalidPath) { ... }
type CodeSentinel struct {
	Code ErrorCode
}

// Error implements the error interface. The string is the raw code value.
func (s CodeSentinel) Error() string {
	return string(s.Code)
}

// Sentinel variables — one per ErrorCode constant.
var (
	ErrUnsupportedArtifactFormat    = CodeSentinel{CodeUnsupportedArtifactFormat}
	ErrUnsupportedHistorySchema     = CodeSentinel{CodeUnsupportedHistorySchema}
	ErrMalformedSnapshot            = CodeSentinel{CodeMalformedSnapshot}
	ErrObsoleteSnapshotVersion      = CodeSentinel{CodeObsoleteSnapshotVersion}
	ErrUnsupportedSnapshotVersion   = CodeSentinel{CodeUnsupportedSnapshotVersion}
	ErrNonCommutativeConflict       = CodeSentinel{CodeNonCommutativeConflict}
	ErrMigrationIdentityDrift       = CodeSentinel{CodeMigrationIdentityDrift}
	ErrMigrationExecution           = CodeSentinel{CodeMigrationExecution}
	ErrMigrationLock                = CodeSentinel{CodeMigrationLock}
	ErrDuplicateMigration           = CodeSentinel{CodeDuplicateMigration}
	ErrHistoryArtifactMissing       = CodeSentinel{CodeHistoryArtifactMissing}
	ErrInvalidMigrationName         = CodeSentinel{CodeInvalidMigrationName}
	ErrDialectMismatch              = CodeSentinel{CodeDialectMismatch}
	ErrInvalidIdentifier            = CodeSentinel{CodeInvalidIdentifier}
	ErrInvalidPath                  = CodeSentinel{CodeInvalidPath}
	ErrSecretLiteral                = CodeSentinel{CodeSecretLiteral}
	ErrInitPrecondition             = CodeSentinel{CodeInitPrecondition}
	ErrEmptyMigrationsDir           = CodeSentinel{CodeEmptyMigrationsDir}
	ErrBootstrapInitRequired        = CodeSentinel{CodeBootstrapInitRequired}
	ErrPartialApplication           = CodeSentinel{CodePartialApplication}
	ErrInteractiveRequired          = CodeSentinel{CodeInteractiveRequired}
	ErrUnsupportedObjectFamily      = CodeSentinel{CodeUnsupportedObjectFamily}
	ErrUnsupportedFeature           = CodeSentinel{CodeUnsupportedFeature}
	ErrUnsupportedSchemaConstruct   = CodeSentinel{CodeUnsupportedSchemaConstruct}
	ErrUnsupportedDialect           = CodeSentinel{CodeUnsupportedDialect}
	ErrInvalidConfig                = CodeSentinel{CodeInvalidConfig}
	ErrConfigCollision              = CodeSentinel{CodeConfigCollision}
	ErrManagedFileOverwrite         = CodeSentinel{CodeManagedFileOverwrite}
	ErrBroadIntrospectionOptIn      = CodeSentinel{CodeBroadIntrospectionOptIn}
	ErrInvalidStatementSegmentation = CodeSentinel{CodeInvalidStatementSegmentation}
	ErrIdentifierCollision          = CodeSentinel{CodeIdentifierCollision}
	ErrInvalidMigrationGraph        = CodeSentinel{CodeInvalidMigrationGraph}
	ErrBeforeWriteAborted           = CodeSentinel{CodeBeforeWriteAborted}
	ErrResourceLimit                = CodeSentinel{CodeResourceLimit}
)

// Error is the base error type for all filemigrate operations. Its exported
// fields are safe-rendered: they must not contain credentials, DSNs, raw SQL
// text, bind values, or secret literals.
//
// Use errors.Is(err, ErrXxx) to classify by code, or errors.As to recover
// structured fields.
type Error struct {
	Code      ErrorCode
	Op        string
	Path      string // safe-rendered, bounded, control chars escaped
	Migration string
	Dialect   string
	Err       error // redacted safe cause only
}

// Error implements the error interface.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(string(e.Code))
	if e.Op != "" {
		b.WriteString(" op=")
		b.WriteString(e.Op)
	}
	if e.Migration != "" {
		b.WriteString(" migration=")
		b.WriteString(e.Migration)
	}
	if e.Path != "" {
		b.WriteString(" path=")
		b.WriteString(e.Path)
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// Is satisfies errors.Is by matching any CodeSentinel with the same Code.
func (e *Error) Is(target error) bool {
	s, ok := target.(CodeSentinel)
	return ok && e.Code == s.Code
}

// Unwrap returns the redacted safe cause, if any.
func (e *Error) Unwrap() error {
	return e.Err
}

// HistoryStage identifies the stage of history-table interaction at the time
// of a migration execution error.
type HistoryStage string

const (
	HistoryStageNone   HistoryStage = "none"
	HistoryStageRead   HistoryStage = "read"
	HistoryStageCreate HistoryStage = "create"
	HistoryStageInsert HistoryStage = "insert"
)

// ExecutionError is returned when a migration statement fails during execution.
// StatementIndex is nil when the failure occurred before statement dispatch.
// Base embeds the shared Error fields.
type ExecutionError struct {
	Base             Error
	StatementIndex   *int
	HistoryStage     HistoryStage
	MayHaveCommitted bool
}

// Error implements the error interface.
func (e *ExecutionError) Error() string { return e.Base.Error() }

// Is satisfies errors.Is by delegating to the embedded Error.
func (e *ExecutionError) Is(target error) bool { return e.Base.Is(target) }

// Unwrap returns the redacted safe cause.
func (e *ExecutionError) Unwrap() error { return e.Base.Unwrap() }

// PartialApplicationError is returned when a migration may have been partially
// applied and the outcome cannot be determined safely.
type PartialApplicationError struct {
	ExecutionError
	HistoryInsertStarted   bool
	HistoryInsertSucceeded bool
}

// Error implements the error interface.
func (e *PartialApplicationError) Error() string { return e.ExecutionError.Error() }

// Is satisfies errors.Is by delegating to the embedded ExecutionError.
func (e *PartialApplicationError) Is(target error) bool { return e.ExecutionError.Is(target) }

// Unwrap returns the redacted safe cause.
func (e *PartialApplicationError) Unwrap() error { return e.ExecutionError.Unwrap() }

// Diagnostic is a non-fatal observation produced during a filemigrate operation.
// Message and Path are safe-rendered fields and must not contain credentials,
// DSNs, raw SQL text, bind values, secret literals, or full database object
// names from broad introspection scans.
type Diagnostic struct {
	Code     DiagnosticCode
	Severity DiagnosticSeverity
	Message  string // safe-rendered; no credentials or raw SQL
	Path     string // safe-rendered; root-relative or canonicalized; no DSNs
}

// newInvalidConfigError constructs an *Error with CodeInvalidConfig for the
// given op. Used by store constructors when the configured directory or other
// caller-supplied input fails the public contract.
func newInvalidConfigError(op string) *Error {
	return &Error{Code: CodeInvalidConfig, Op: op}
}

// newPathError constructs an *Error for path-related failures.
func newPathError(op, path string, cause error) *Error {
	return &Error{
		Code: CodeInvalidPath,
		Op:   op,
		Path: safeRenderPath(path),
		Err:  cause,
	}
}

// safeRenderPath returns a version of p safe for inclusion in Error.Path or
// Diagnostic.Path. It escapes control characters and truncates overly long
// values so they cannot be used as terminal-control injection vectors.
func safeRenderPath(p string) string {
	const maxLen = 512
	if len(p) > maxLen {
		p = p[:maxLen] + "…"
	}
	return fmt.Sprintf("%q", p)
}
