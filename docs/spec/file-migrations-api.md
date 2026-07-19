# File Migrations CLI and API Specification

## Status

Draft

## Purpose

Define the public CLI and library API contract for Grizzle's file-based migration workflow.

Pinned upstream target:

- Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`

## Scope

This document must define:

- CLI commands
- CLI flags
- library APIs
- mutating vs read-only behavior
- error contracts
- replacement strategy for current APIs

## Upstream References

- Drizzle Kit overview
- Drizzle Kit migrate
- Drizzle Kit check
- Drizzle config docs
- [file-migrations-upstream-mapping.md](./file-migrations-upstream-mapping.md)

## CLI Surface

Upstream Drizzle CLI surface relevant to file migrations:

- `generate`
- `migrate`
- `check`
- `push`
- `pull`
- `up`
- `export` exists upstream but is not file-migration-relevant in the initial Grizzle target; it is deferred to a dedicated future export spec

Current scope note:

- `up` exists in Drizzle because Drizzle had older folder formats to upgrade
- Grizzle intentionally omits an equivalent command from the initial RC.1-scoped implementation because Grizzle has no released old file-migration folder format to upgrade
- To remain aligned with Drizzle's public surface, Grizzle must not expose `snapshot` or `diff` as part of the target file-migrations CLI workflow
- Grizzle must use `./grizzle` as the default output directory instead of Drizzle's `./drizzle`; this is DEVIATION:INTENTIONAL Grizzle-branded default naming
- Grizzle must not expose a conflict-bypass flag analogous to Drizzle's `--ignore-conflicts`; this is DEVIATION:INTENTIONAL safety hardening relative to Drizzle RC.1

Target Grizzle file-migrations CLI surface:

- `grizzle generate`
- `grizzle migrate`
- `grizzle check`
- `grizzle push`
- `grizzle pull`
- `grizzle introspect` as an alias of `grizzle pull`

Initial omission:

- no `grizzle up` (**DEVIATION:INTENTIONAL**; excluded from the initial public workflow)
- no `grizzle studio` (**DEVIATION:INTENTIONAL**; excluded from the initial public workflow)
- no `grizzle export` (**DEVIATION:INTENTIONAL**; deferred until a dedicated export spec exists)
- no public `grizzle snapshot`
- no public `grizzle diff`

## Library Surface

The library API must mirror the workflow roles, not expose a different conceptual model.

Target public package:

- `github.com/sofired/grizzle/kit/filemigrate`

Target logical operations:

- `Generate`
- `Migrate`
- `Check`
- `Pull`

The public file-migration API must preserve the file-based command boundaries. `push` remains a CLI surface command, but its library API belongs to a dedicated direct-sync spec because it is not a file-migration operation.

Current file-migration decisions affecting API shape:

- migration table name and PostgreSQL schema must be configurable
- custom migrations are in scope
- `meta/` is not part of the target artifact model
- `check` is a normal prerequisite in the file-migrations workflow
- `check` gates both `generate` and `migrate`

Initial Go shape:

```go
package filemigrate

func Check(ctx context.Context, opts CheckOptions) (*CheckResult, error)
func Generate(ctx context.Context, opts GenerateOptions) (*GenerateResult, error)
func Migrate(ctx context.Context, opts MigrateOptions) (*MigrateResult, error)
func Pull(ctx context.Context, opts PullOptions) (*PullResult, error)
```

Option structs, not variadic functional options, are the initial public contract. This keeps CLI resolution, library use, and tests aligned around one explicit data shape.

Initial result shapes:

```go
type Snapshot struct {
	Version string
	Dialect string
	ID      string
	PrevIDs []string
	DDL     []DDLEntity
	Renames []string
}

type DDLEntity interface {
	DDLKind() string
}

type CheckResult struct {
	BaseSnapshot            *Snapshot
	BaseID                  string
	EffectiveSnapshot       *Snapshot
	EffectiveParentIDs      []string
	BranchStatements        []BranchStatement
	ArtifactDigests         map[string]ArtifactDigest
	LoadedArtifacts         []LoadedArtifact
}

type GenerateResult struct {
	Changed  bool
	Artifact *LoadedArtifact
}

type MigrateResult struct {
	Applied []AppliedMigration
	Skipped []AppliedMigration
}

type PullResult struct {
	SchemaFiles          []ManagedFile
	BootstrapArtifact   *LoadedArtifact
	InitRecorded         bool
	IntrospectionSummary *IntrospectionSummary
	LimitStatus         []ResourceLimitStatus
	Diagnostics          []Diagnostic
}

type PullBeforeWriteFunc func(ctx context.Context, plan PullWritePlan) error

type PullWritePlan struct {
	IntrospectionSummary *IntrospectionSummary
	Diagnostics          []Diagnostic
	SchemaFiles          []PlannedSourceFile
	BootstrapArtifact   *PlannedBootstrapArtifact
	LimitStatus         []ResourceLimitStatus
	Init                 bool
}

type ResourceLimitName string

const (
	LimitMaxArtifacts                  ResourceLimitName = "max_artifacts"
	LimitMaxArtifactDirEntries         ResourceLimitName = "max_artifact_dir_entries"
	LimitMaxTotalArtifactBytes         ResourceLimitName = "max_total_artifact_bytes"
	LimitMaxMigrationSQLBytes          ResourceLimitName = "max_migration_sql_bytes"
	LimitMaxSnapshotJSONBytes          ResourceLimitName = "max_snapshot_json_bytes"
	LimitMaxSnapshotJSONDepth          ResourceLimitName = "max_snapshot_json_depth"
	LimitMaxSnapshotEntities           ResourceLimitName = "max_snapshot_entities"
	LimitMaxTempCleanupEntries         ResourceLimitName = "max_temp_cleanup_entries"
	LimitMaxIntrospectionObjects       ResourceLimitName = "max_introspection_objects"
	LimitMaxObjectNameBytes            ResourceLimitName = "max_object_name_bytes"
	LimitMaxIntrospectionSQLBytes      ResourceLimitName = "max_introspection_sql_bytes"
	LimitMaxIntrospectionPayloadBytes  ResourceLimitName = "max_introspection_payload_bytes"
	LimitMaxRenderedSourceFileBytes    ResourceLimitName = "max_rendered_source_file_bytes"
	LimitMaxRenderedSourceBytes        ResourceLimitName = "max_rendered_source_bytes"
	LimitMaxBootstrapMigrationSQLBytes ResourceLimitName = "max_bootstrap_migration_sql_bytes"
	LimitMaxBootstrapSnapshotJSONBytes ResourceLimitName = "max_bootstrap_snapshot_json_bytes"
	LimitMaxPlannedWriteBytes          ResourceLimitName = "max_planned_write_bytes"
	LimitMaxSchemaFiles                ResourceLimitName = "max_schema_files"
	LimitMaxSchemaSourceFileBytes      ResourceLimitName = "max_schema_source_file_bytes"
	LimitMaxSchemaSourceBytes          ResourceLimitName = "max_schema_source_bytes"
	LimitMaxSchemaASTNodes             ResourceLimitName = "max_schema_ast_nodes"
	LimitMaxSchemaASTDepth             ResourceLimitName = "max_schema_ast_depth"
	LimitMaxSchemaDeclarations         ResourceLimitName = "max_schema_declarations"
	LimitMaxSchemaLiteralBytes         ResourceLimitName = "max_schema_literal_bytes"
	LimitMaxSecretValueBytes           ResourceLimitName = "max_secret_value_bytes" // internal enforcement only; omit from public LimitStatus
)

type ResourceLimitStatus struct {
	Name  ResourceLimitName
	Used  int64
	Limit int64
}

type PlannedSourceFile struct {
	RelPath string
	Size    int64
	Digest  string // library-only; omit from built-in CLI/broad-scan summaries
}

type PlannedBootstrapArtifact struct {
	Name               string
	MigrationSQLSize   int64
	MigrationSQLDigest string // library-only; omit from built-in CLI/broad-scan summaries
	SnapshotJSONSize   int64
	SnapshotJSONDigest string // library-only; omit from built-in CLI/broad-scan summaries
	ArtifactDigest     string // library-only; omit from built-in CLI/broad-scan summaries
}
```

Minimal carrier types:

```go
type BranchStatement struct {
	Dialect string
	Kind    string
	RawJSON []byte
}

type SQLSegment struct {
	Migration string
	Index     int
	SQL       []byte
}

type Digest [32]byte

type HashHex string

type Diagnostic struct {
	Code     DiagnosticCode
	Severity DiagnosticSeverity
	Message  string
	Path     string
}

type DiagnosticCode string

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

type Literal struct {
	Kind     LiteralKind
	String   string
	Strings  []string
	Int64    *int64
	Float64  *float64
	Bool     *bool
	Bytes    []byte
}

type LiteralKind string

const (
	LiteralNull       LiteralKind = "null"
	LiteralString     LiteralKind = "string"
	LiteralStringList LiteralKind = "string_list"
	LiteralInt64      LiteralKind = "int64"
	LiteralFloat64    LiteralKind = "float64"
	LiteralBool       LiteralKind = "bool"
	LiteralBytes      LiteralKind = "bytes"
)

type ArtifactDigest struct {
	MigrationSQLSHA256 Digest
	SnapshotJSONSHA256 Digest
	CombinedSHA256     Digest
}

type LoadedArtifact struct {
	Name                 string
	Dir                  string
	MigrationSQL         []byte
	SnapshotJSON         []byte
	Snapshot             *Snapshot
	Digests              ArtifactDigest
	ManagedIntrospection bool
}

type AppliedMigration struct {
	Name      string
	HashHex   HashHex
	CreatedAt int64
	AppliedAt *time.Time
}

type ManagedFile struct {
	RelPath       string
	ContentSHA256 Digest
	Written       bool
}

type Result interface {
	RowsAffected() (int64, error)
}

type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

// A successful MigrationSession.Query transfers Rows ownership to the caller.
// Callers must close returned rows exactly once on every path.

type Clock interface {
	Now() time.Time
}

type UUIDSource interface {
	New() string
}

type NameGenerator interface {
	Generate(now time.Time, suffix string) (string, error)
}

type HashFunc func([]byte) Digest

type LockIdentity struct {
	Dialect       string
	TargetID      string
	HistorySchema string
	HistoryTable  string
}

type TransactionMode string

const (
	TransactionNone      TransactionMode = "none"
	TransactionBatch     TransactionMode = "batch"
	TransactionImmediate TransactionMode = "immediate"
)
```

Diagnostic redaction rules:

- `Diagnostic.Message` and `Diagnostic.Path` are safe-rendered fields; they must not contain credentials, DSNs, raw SQL text, bind values, matched secret literals, surrounding SQL context, or full database object names from broad introspection
- all public `DiagnosticBroadIntrospection` values exposed through CLI output, `PullWritePlan.Diagnostics`, `PullResult.Diagnostics`, or other exported result/callback structs must contain counts/categories only and must not include full object names in `Message`, `Path`, or any other safe-rendered field during broad scans
- `DiagnosticSecretLiteral` messages must identify only the object family, artifact/file path when safe, and a redacted location or stable internal error code; they must never include hashes/fingerprints of secret-bearing payloads, the matched literal, or neighboring SQL text
- CLI rendering must use only explicitly safe diagnostic fields; any richer internal diagnostic payloads require separate typed fields with redaction rules before publication
- redaction tests must cover broad-scan diagnostics in CLI output, `BeforeWrite` plans, and `PullResult`, proving that full object names are absent from `Message`, `Path`, formatted errors, and structured diagnostics. Any future unsafe library opt-in for broad-scan object refs is scoped to explicit object-reference fields such as `IntrospectionSummary.Objects`; it must not weaken diagnostic redaction.

`LoadedArtifact` owns immutable byte snapshots of the validated files, or an implementation-equivalent stable no-follow file handle with a clear close lifecycle. If exported `[]byte` fields are used, returned slices are caller-owned defensive copies and are never reused internally after publication. The initial public API prefers owned byte slices so callers do not need to manage artifact handles.

### Artifact digest formulas and canonical vectors

The binary per-file digest values carried by `ArtifactDigest` are the SHA-256
of each file's exact raw byte sequence:

```text
MigrationSQLSHA256 = SHA256(raw migration.sql bytes)
SnapshotJSONSHA256 = SHA256(raw snapshot.json bytes)
```

Each `ArtifactDigest` field carries the 32 raw digest bytes. In the canonical
fixture and any textual serialization, those bytes are encoded as exactly 64
lowercase hexadecimal characters:

```text
MigrationSQLSHA256Hex = lowercase_hex(MigrationSQLSHA256)
SnapshotJSONSHA256Hex = lowercase_hex(SnapshotJSONSHA256)
```

These per-file digests apply no framing, text decoding, Unicode normalization,
line-ending conversion, SQL comment removal, whitespace trimming, or
trailing-newline normalization. Every byte, including embedded NUL and bytes
that are not valid UTF-8, participates unchanged in SHA-256.

`ArtifactDigest.CombinedSHA256` is required in all successful local artifact
validation results. Its 32 raw digest bytes are the SHA-256 of this framed byte
sequence:

```text
SHA256(
  "grizzle-artifact-v1" || 0x00 ||
  "migration.sql" || 0x00 || uint64be(len(migration.sql)) || raw migration.sql bytes ||
  "snapshot.json" || 0x00 || uint64be(len(snapshot.json)) || raw snapshot.json bytes
)
```

Quoted domain and file-name strings in this formula denote their exact ASCII
bytes. `len(payload)` is the raw payload byte count, and `uint64be` encodes that
count as exactly eight unsigned big-endian bytes. The fixture's
`combined_sha256` field is the 64-character lowercase hexadecimal encoding of
the resulting 32 digest bytes.

The combined digest is a local artifact/TOCTOU diagnostic value only. It is not written to the initial database history table.

The canonical cross-implementation examples are
[`artifact_digest_vectors.json`](../../kit/filemigrate/testdata/artifact_digest_vectors.json).
Each vector has a stable name, hexadecimal encodings of the exact two input byte
sequences, and the expected per-file and combined digests. This fixture is the
single vector source for the Go conformance tests and the independent
[`digest_reference.py`](../../kit/filemigrate/testdata/digest_reference.py)
checker; neither implementation may maintain a separate copy of the vector
inventory.

Verify the published vectors without modifying them:

```sh
python3 kit/filemigrate/testdata/digest_reference.py --check
go test ./kit/filemigrate -run TestArtifactDigest
```

An intentional vector update requires an approved specification change. Update
the normative formula and independent Python implementation as needed, edit only
the fixture's names or hexadecimal input fields when changing cases, then
regenerate expected values with:

```sh
python3 kit/filemigrate/testdata/digest_reference.py --write
```

Review the complete fixture diff, rerun `--check` and the Go conformance tests,
and commit the specification, checker, and fixture changes together. Never
regenerate expected values from the Go production implementation.

Literal validation table:

| Kind | Required populated fields | Fields that must be zero/nil | Rendering expectation |
| --- | --- | --- | --- |
| `LiteralNull` | none beyond `Kind` | `String`, `Strings`, `Int64`, `Float64`, `Bool`, `Bytes` | renders dialect `NULL` only where null is valid |
| `LiteralString` | `String` | `Strings`, `Int64`, `Float64`, `Bool`, `Bytes` | renders as a dialect-escaped SQL string literal |
| `LiteralStringList` | non-empty `Strings` | `String`, `Int64`, `Float64`, `Bool`, `Bytes` | renders only in allowlisted list/enum contexts, with each string escaped |
| `LiteralInt64` | `Int64` | `String`, `Strings`, `Float64`, `Bool`, `Bytes` | renders deterministic base-10 integer text |
| `LiteralFloat64` | `Float64` | `String`, `Strings`, `Int64`, `Bool`, `Bytes` | renders deterministic finite numeric text; NaN/Inf are invalid |
| `LiteralBool` | `Bool` | `String`, `Strings`, `Int64`, `Float64`, `Bytes` | renders dialect boolean literal only where booleans are valid |
| `LiteralBytes` | `Bytes` | `String`, `Strings`, `Int64`, `Float64`, `Bool` | renders only in dialects/contexts with a specified byte-literal encoding |

Invalid literal combinations, multiple populated value fields, empty required fields, and values outside the dialect-specific allowlist must fail validation before DDL rendering.

All public result maps, slices, and nested byte slices returned by `Check`, `Generate`, `Migrate`, and `Pull` are caller-owned defensive copies. Internal execution must not depend on callers preserving or not mutating returned containers after publication.

These are required logical result shapes. Exact Go field names may change before publication, but the information carried by each result is required for implementation and tests.

`DDLEntity` is the dialect-specific, validated entity union carried by the RC.1-style snapshot validator. It must not be represented internally as an unchecked `map[string]any`; unsupported or unvalidated entity families must fail with `unsupported_object_family`, and unsupported fields on recognized families must fail with `unsupported_feature`, before they participate in diffing, checking, or migration generation.

`Snapshot` in this package is the RC.1-style artifact snapshot envelope defined in [file-migrations-artifacts.md](./file-migrations-artifacts.md). It must not reuse the current or legacy `kit.Snapshot` type, whose shape is not compatible with the RC.1 artifact model.

## CLI Flags

Initial command-level expectations:

### `generate`

- supports selecting schema input
- requires dialect selection, either directly or through a resolved config file
- supports configuring output directory
- supports custom migration naming
- supports custom/empty migration generation
- accepts breakpoint configuration only when omitted or explicitly enabled; rejecting `Breakpoints=false` is **DEVIATION:INTENTIONAL** safety hardening until disabled-breakpoint execution is explicitly designed
- defaults breakpoints to enabled
- does not support `--ignore-conflicts` as DEVIATION:INTENTIONAL safety hardening

### `migrate`

- follows RC.1's config-file-driven command model for dialect, output directory, database credential references, and migration table/schema resolution
- may expose direct Grizzle CLI flags for dialect, output directory, non-secret database connection fields, or migration table/schema as DEVIATION:INTENTIONAL Go CLI conveniences, but those flags are not Drizzle RC.1 parity
- supports explicit `--allow-empty` as a DEVIATION:INTENTIONAL Grizzle extension for controlled no-op deployments only when the artifact root is absent or empty and database history is absent or empty
- does not support `--ignore-conflicts` as DEVIATION:INTENTIONAL safety hardening

### `check`

- supports configuring migrations output directory
- supports dialect selection as required by snapshot validation rules
- does not require machine-readable CLI output in the initial design
- does not support `--ignore-conflicts` as DEVIATION:INTENTIONAL safety hardening
- does not require database connection credentials in the initial design
- if a config file contains database credentials for other commands, `check` must ignore them rather than opening a database connection

### `push`

- remains a separate direct-apply workflow
- is documented here only as a command boundary
- must not be reshaped into file-based migrate semantics
- must not receive implementation-ready CLI/API semantics in this file
- dialect inputs, credentials, filters, dry-run/verbose output, force/destructive handling, non-interactive behavior, and locking belong to a dedicated push/direct-sync spec

### `pull`

- remains a separate introspection workflow
- connects to a live database and introspects its schema
- writes Go schema definition files to the configured schema output directory
- may write a bootstrap introspection migration artifact to the configured migrations output directory when no snapshots exist there
- requires explicit `--all-schemas` / `AllowBroadScan` when no schema/table filters are configured and the command would write files or history metadata
- accepts breakpoint configuration for bootstrap introspection migration SQL only when omitted or explicitly enabled; rejecting `Breakpoints=false` is **DEVIATION:INTENTIONAL** safety hardening until disabled-breakpoint execution is explicitly designed
- defaults breakpoints to enabled
- must remain separate from file-based migration generation and application

### Flag / Config Resolution

The implementation must not infer CLI behavior from prose-only “supports” lists.

Initial command matrix:

| Command | Required resolved inputs | Primary CLI/config names | Defaults | Database credential inputs |
| --- | --- | --- | --- | --- |
| `generate` | dialect, schema input, migrations output directory | `--dialect`, `--schema`, `--out`, `--name`, `--custom`, `--breakpoints` when omitted or true only, `--allow-secret-literals` as explicit risk acceptance | `out = ./grizzle`, `breakpoints = true`, secret-literal blocking enabled | ignored |
| `check` | dialect, migrations output directory | `--dialect`, `--out`, `--allow-secret-literals` as explicit risk acceptance | `out = ./grizzle`, secret-literal blocking enabled | ignored |
| `migrate` | dialect, migrations output directory, database connection, history table/schema | RC.1-style config values for `dialect`, `out`, credential references/config, `migrations.table`, `migrations.schema`; optional direct Grizzle flags for non-secret connection fields are DEVIATION:INTENTIONAL extensions; `--allow-empty` is a DEVIATION:INTENTIONAL extension; `--allow-secret-literals` is explicit risk acceptance | `out = ./grizzle`, branded history defaults table `__grizzle_migrations` / PostgreSQL schema `grizzle` as **DEVIATION:INTENTIONAL**, secret-literal blocking enabled | required as credential references/config; literal secret CLI args are unsupported |
| `pull` | dialect, database connection, schema output directory, migrations output directory, history table/schema for exclusion; `--init` additionally uses history table/schema for metadata writes | `--dialect`, credential references/config plus non-secret direct connection flags, `--schema-out`, `--migrations-out`, `--breakpoints` when omitted or true only, `--introspect-casing`, filters, `--all-schemas` for broad scans, `--allow-secret-literals` as explicit risk acceptance, `--init`, `--assume-quiescent` for init only; config `out` maps to migrations output; `migrations.table` and `migrations.schema` resolve through the same `HistoryOptions` rules as `migrate`; split outputs are DEVIATION:INTENTIONAL Go workflow hardening, defined in [pull.md](./pull.md) | `schema-out = ./schema`, `migrations-out = ./grizzle`, `breakpoints = true`, secret-literal blocking enabled | required as credential references/config; literal secret CLI args are unsupported |
| `push` | deferred to dedicated push/direct-sync spec | deferred to dedicated push/direct-sync spec | deferred | deferred to dedicated push/direct-sync spec |

Resolution and collision rules:

- Grizzle must follow RC.1's config-vs-CLI collision posture rather than a generic "flags override config" model.
- initial config loading is DEVIATION:INTENTIONAL safety hardening from Drizzle RC.1's executable `drizzle.config.ts/js` model: Grizzle config loading must be declarative and non-executing
- all initial CLI commands accept a common optional `--config <path>` selector flag. This flag only selects the config file; it is not a command overlay and is exempt from `config_collision`. Lower-level library APIs receive already-resolved options and do not expose `ConfigPath`.
- initial supported config filenames are `grizzle.config.json`, `grizzle.config.yaml`, `grizzle.config.yml`, and `grizzle.config.toml`; discovery starts at the working directory unless an explicit config path is supplied
- if more than one default config file exists, config resolution fails with `config_collision`; an explicit config path disables default discovery
- TypeScript, JavaScript, and executable config files such as `drizzle.config.ts`, `drizzle.config.js`, `grizzle.config.ts`, or `grizzle.config.js` fail with `unsupported_feature` unless a future trusted executable config mode is specified
- static config formats must not execute project code, import Go packages, run shell commands, evaluate plugins, or expand arbitrary templates
- config-file paths must resolve through the same canonical no-follow path model as schema inputs: symlinks, FIFOs, devices, sockets, directories, hardlinks where detectable, and non-regular config files fail with `invalid_path`
- config loading happens before user-provided `ResourceLimits` are trusted, so implementations must use fixed bootstrap limits: config file bytes <= 1048576, parser nesting depth <= 64, key count <= 4096, string length <= 65536 bytes, and total scalar count <= 16384; exceeding them fails with `resource_limit`
- config parsers must use bounded readers/parsers and redact parse diagnostics so raw credentials, DSNs, and large literals are not printed
- literal password, token, private-key, URL userinfo, or DSN-with-secret CLI arguments are not part of the initial Grizzle CLI; direct flags may carry non-secret fields such as host, port, user name, database name, and credential reference names only
- secret credential values must be supplied through documented references such as environment-variable names, protected secret-file references, or a future prompt/fd mechanism; CLI docs must warn that literal secrets in command arguments leak through process lists and shell history
- any future executable config mode must be explicit, trusted, opt-in, and covered by the credential-redaction rules before it is added
- If a config file is used, command inputs come from that config plus only the command-specific overlay flags listed below.
- If no config file is used, CLI flags may supply the required inputs listed in the command matrix. For DB-backed commands, no-config CLI may supply dialect/output plus non-secret connection fields and credential references, but not literal secret values.
- Environment variables may supply credential values only through documented credential/config fields.
- `check` must ignore database credentials even when present in the resolved config.
- `push` is listed here only as a public command boundary; it is not implementation-ready until the dedicated push/direct-sync spec defines its full config matrix.

Credential config wire shape:

```yaml
dialect: postgresql
out: ./grizzle
dbCredentials:
  url:
    env: DATABASE_URL
```

Split credential example:

```yaml
dialect: postgresql
out: ./grizzle
dbCredentials:
  host: localhost
  port: 5432
  user: app
  database: appdb
  password:
    file: ./secrets/db-password
```

- secret fields use exactly one mapping key: `env`, `file`, `prompt`, or `fd`; plain strings are invalid for secret-bearing fields
- non-secret fields such as `host`, `port`, `user`, and `database` may be plain scalars
- `url` is a secret-bearing field because DSNs commonly contain userinfo or password query parameters; it must use a secret reference
- `dbCredentials` matches Drizzle RC.1's config key. A `credentials` alias is not accepted in the initial target unless a future compatibility layer explicitly labels it as `DEVIATION:LANGUAGE`.
- config containing both `dbCredentials.url` and split credential fields fails with `config_collision`

Allowed config-mode overlays:

| Command | Allowed overlays when config is used |
| --- | --- |
| `generate` | `--name`, `--custom`, `--allow-secret-literals` |
| `check` | `--allow-secret-literals`; `--ignore-conflicts` is intentionally omitted |
| `migrate` | `--allow-empty` as a DEVIATION:INTENTIONAL Grizzle extension; `--allow-secret-literals` |
| `pull` | `--schema-out` / `--migrations-out` only for targets not supplied by config, plus `--all-schemas`, `--allow-secret-literals`, and `--init`; `--assume-quiescent` is allowed only when `--init` is also supplied; split-output fill-ins are DEVIATION:INTENTIONAL from RC.1 introspect collision rules |

Any other CLI/config mixture must fail with `config_collision` unless a future spec explicitly adds that overlay.

## Library Input Contracts

Common local options:

```go
type CommonOptions struct {
	Dialect       dialect.Dialect
	MigrationsDir string
	History       HistoryOptions
	LockTimeout   time.Duration
	Limits        ResourceLimits
	AllowSecretLiterals bool

	Clock         Clock
	UUIDSource    UUIDSource
	NameGenerator NameGenerator
	ArtifactStore ArtifactStore
	Hash          HashFunc
}

type ResourceLimits struct {
	MaxArtifacts                 int
	MaxArtifactDirEntries        int
	MaxTotalArtifactBytes        int64
	MaxMigrationSQLBytes         int64
	MaxSnapshotJSONBytes         int64
	MaxSnapshotJSONDepth         int
	MaxSnapshotEntities          int
	MaxTempCleanupEntries        int
	MaxIntrospectionObjects      int
	MaxObjectNameBytes           int
	MaxIntrospectionSQLBytes     int64
	MaxIntrospectionPayloadBytes int64
	MaxRenderedSourceFileBytes   int64
	MaxRenderedSourceBytes       int64
	MaxBootstrapMigrationSQLBytes int64
	MaxBootstrapSnapshotJSONBytes int64
	MaxPlannedWriteBytes         int64
	MaxSchemaFiles               int
	MaxSchemaSourceFileBytes     int64
	MaxSchemaSourceBytes         int64
	MaxSchemaASTNodes            int
	MaxSchemaASTDepth            int
	MaxSchemaDeclarations        int
	MaxSchemaLiteralBytes        int64
	MaxSecretValueBytes          int64
}
```

Production default resource-limit profile:

| Field | Default |
| --- | ---: |
| `MaxArtifacts` | 10000 |
| `MaxArtifactDirEntries` | 50000 |
| `MaxTotalArtifactBytes` | 536870912 |
| `MaxMigrationSQLBytes` | 16777216 |
| `MaxSnapshotJSONBytes` | 16777216 |
| `MaxSnapshotJSONDepth` | 128 |
| `MaxSnapshotEntities` | 100000 |
| `MaxTempCleanupEntries` | 10000 |
| `MaxIntrospectionObjects` | 50000 |
| `MaxObjectNameBytes` | 512 |
| `MaxIntrospectionSQLBytes` | 1048576 |
| `MaxIntrospectionPayloadBytes` | 67108864 |
| `MaxRenderedSourceFileBytes` | 8388608 |
| `MaxRenderedSourceBytes` | 134217728 |
| `MaxBootstrapMigrationSQLBytes` | 16777216 |
| `MaxBootstrapSnapshotJSONBytes` | 16777216 |
| `MaxPlannedWriteBytes` | 268435456 |
| `MaxSchemaFiles` | 1000 |
| `MaxSchemaSourceFileBytes` | 8388608 |
| `MaxSchemaSourceBytes` | 134217728 |
| `MaxSchemaASTNodes` | 1000000 |
| `MaxSchemaASTDepth` | 128 |
| `MaxSchemaDeclarations` | 100000 |
| `MaxSchemaLiteralBytes` | 1048576 |
| `MaxSecretValueBytes` | 65536 |

Zero-valued `ResourceLimits` fields use this profile. Implementations may allow stricter configured values, but conformance tests must assert these production defaults or stricter caps for config size, artifact bytes/counts, JSON depth/entities, per-payload introspection SQL bytes, aggregate introspection payload bytes, introspection object counts, rendered source bytes, planned write bytes, and secret value bytes.

```go
type HistoryOptions struct {
	Table  string
	Schema string
}

type SecretRefKind string

const (
	SecretFromEnv    SecretRefKind = "env"
	SecretFromFile   SecretRefKind = "file"
	SecretFromPrompt SecretRefKind = "prompt" // future CLI mode unless explicitly enabled
	SecretFromFD     SecretRefKind = "fd"     // future CLI mode unless explicitly enabled
)

type SecretRef struct {
	Kind SecretRefKind
	Name string // env name, protected file path, prompt label, or fd number string
}

type DatabaseCredentials struct {
	URL      *SecretRef
	Host     string
	Port     int
	User     string
	Password *SecretRef
	Database string
}
```

`SecretFromEnv` names must match `^[A-Za-z_][A-Za-z0-9_]{0,127}$` before lookup. Empty names, overly long names, controls, whitespace, shell metacharacters, and non-ASCII env names fail with `invalid_config` before lookup or diagnostic rendering. Other `SecretRef.Name` forms must use their kind-specific validation before any log or error output.

Normative option rules:

- `Dialect` is required for every command that validates snapshots or produces SQL
- `MigrationsDir` defaults to `./grizzle` when omitted by CLI/config resolution
- `History.Table` defaults to `__grizzle_migrations`, a **DEVIATION:INTENTIONAL** Grizzle namespace/branding divergence from RC.1's `__drizzle_migrations`
- `History.Schema` defaults to `grizzle` for PostgreSQL, a **DEVIATION:INTENTIONAL** Grizzle namespace/branding divergence from RC.1's `drizzle`; it must be empty for dialects that do not support a separate schema/namespace
- `LockTimeout` defaults to a finite implementation-defined value when the caller's context has no deadline; zero means use the default, not wait forever
- `Clock`, `UUIDSource`, `NameGenerator`, `ArtifactStore`, and `Hash` are deterministic test hooks; nil values use production defaults
- the production `NameGenerator` must produce `<UTC YYYYMMDDHHmmss>_<suffix>` when `suffix` is explicit, and `<UTC YYYYMMDDHHmmss>_<adjective>_<hero>` when `suffix` is empty, using RC.1-derived adjective and hero word lists for the generated suffix
- `Hash` is a library test hook only and must not be accepted from CLI/config; any injected hash must preserve SHA-256 semantics for history and artifact digests or be limited to tests that do not write production-compatible history rows
- breakpoint options must be represented as nullable/pointer booleans or an equivalent tri-state so nil means "use default true"
- `Breakpoints=false` is rejected in the initial implementation for `generate` and `pull` as **DEVIATION:INTENTIONAL** safety hardening; disabled-breakpoint artifacts require a future explicit execution design
- `Limits` fields default to the production profile above when zero; negative values fail with `invalid_config`
- `AllowSecretLiterals` defaults to false; setting it true is an explicit unsafe opt-out from high-confidence secret-literal blocking and must be unavailable unless a command documents the corresponding CLI/config flag
- artifact loading, snapshot parsing, hashing, source rendering, introspection, temporary cleanup scans, and planned writes must check the active context and fail with `resource_limit` before unbounded memory, file, JSON, or object growth
- `MaxIntrospectionSQLBytes` is a per-payload cap for raw default, check, view, generated expression, or catalog SQL text values; `MaxIntrospectionPayloadBytes` is the aggregate cap across all such raw introspection payloads collected during one `pull`. Adapters must enforce both while streaming metadata, before rendering source or bootstrap artifacts.
- each public `ResourceLimits` field maps to the same snake-case `ResourceLimitName` constant, except secret-value size limits whose usage is intentionally not exposed; `Used` and `Limit` are counts for `Max*Objects`, `Max*Entries`, `Max*Files`, `Max*Entities`, `Max*Nodes`, `Max*Depth`, and `Max*Declarations`, and bytes for public `Max*Bytes` fields
- aggregate limits such as `MaxTotalArtifactBytes`, `MaxRenderedSourceBytes`, `MaxPlannedWriteBytes`, `MaxSchemaSourceBytes`, and `MaxIntrospectionPayloadBytes` are measured across the whole command invocation; per-item limits such as `MaxMigrationSQLBytes`, `MaxSnapshotJSONBytes`, `MaxRenderedSourceFileBytes`, `MaxSchemaSourceFileBytes`, `MaxObjectNameBytes`, and `MaxIntrospectionSQLBytes` are measured for each individual artifact/file/name/SQL payload
- `ResourceLimitStatus` contains only counts/byte sizes and stable limit names; it must not contain raw SQL, source bytes, snapshot JSON, object payloads, credentials, bind values, or secret-value usage. `MaxSecretValueBytes` is enforced internally but must be omitted from public `LimitStatus` because `Used` would reveal secret length.

Nil and typed-nil option rules:

- optional interface or function options that are nil or typed-nil are treated as absent and use the documented production default
- required interface options that are nil or typed-nil fail before use with `invalid_config`
- public commands must detect typed-nil interfaces before invoking option methods where Go reflection can do so safely
- adapter methods must not return nil or typed-nil result interfaces/pointers with a nil error; if they do, the command must fail with `invalid_config` and a redacted adapter-contract diagnostic

Credential reference rules:

- `DatabaseCredentials.URL` is mutually exclusive with split host/port/user/password/database fields
- secret-bearing fields such as URL, password, tokens, and private keys must be represented as `SecretRef`; non-secret fields may be literal config/flag values
- `SecretFromEnv` reads the named environment variable at command execution time; missing or empty required variables fail with `invalid_config`
- secret values from environment variables or files must be capped by `MaxSecretValueBytes`; oversized values fail with `resource_limit` before connector construction
- `SecretFromEnv` must reject control characters other than tab/CR/LF according to the connector's accepted credential format and must never log the value length with enough context to identify the secret
- `SecretFromFile` reads a protected regular file through no-follow path handling, using a bounded reader capped by `MaxSecretValueBytes`; symlinks, hardlinks where detectable, non-regular files, directories, invalid bytes, and unsafe permissions fail with `invalid_path` or `invalid_config` before connecting
- on POSIX, protected secret files must be owned by the current user or root and must not grant group/other permissions; on Windows, implementations must reject directories/reparse points and require the file ACL to be restricted to the current user, Administrators, and SYSTEM, or fail closed when that cannot be determined
- `SecretFromFile` relative paths resolve against the containing config file directory when read from config, or the current working directory when supplied by an explicit future CLI reference; a single trailing LF or CRLF may be trimmed, but other bytes are preserved
- `SecretFromPrompt` and `SecretFromFD` are reserved future reference kinds and must fail with `unsupported_feature` until their CLI behavior is specified
- resolved secret values are passed only to the connector/driver layer; they must not appear in `PullResult`, diagnostics, logs, error strings, `Unwrap()`, or `%+v`
- dialect-specific credential structs may add non-secret fields and additional `SecretRef` fields, but they must preserve these redaction and validation rules before constructing a `Connector`

Local-only operation inputs:

- migrations output directory
- dialect

Database-backed operation inputs:

- database connection or session handle
- migrations output directory
- dialect where needed by the operation
- migration table name override
- PostgreSQL migration schema override

Credential safety:

- CLI examples should prefer environment variables or config references over password-bearing DSNs
- errors, logs, verbose output, and config diagnostics must redact URL userinfo, password query params, and split secret fields
- public errors must not wrap or render raw DSNs or password values

SQL and artifact-output safety:

- generated migration files are committed artifacts and should not contain secrets, seed passwords, API tokens, or personal data
- Grizzle cannot prove absence of secrets in arbitrary SQL or introspected defaults, but high-confidence secret-literal findings must fail closed by default before `generate`, `pull`, `check`, or `migrate` publishes files or executes SQL
- verbose/explain output must redact bound values and show placeholders plus value types; no unsafe value-printing debug mode is part of the initial scope
- custom migration docs must warn that SQL literals are committed to source control and should not be used for secrets
- `--allow-secret-literals` / `AllowSecretLiterals=true` is a **DEVIATION:INTENTIONAL** explicit risk-acceptance escape hatch; it may downgrade high-confidence findings to warnings, but diagnostics remain redacted and absence of a warning must not be treated as proof that output is secret-free

Minimum high-confidence secret-literal detector:

- password-bearing or token-bearing DSNs/URLs, including URL userinfo and query parameters such as `password=`, `token=`, `access_token=`, `refresh_token=`, `api_key=`, `apikey=`, `secret=`, `client_secret=`, `private_key=`
- PEM private-key blocks and SSH/OpenSSH private-key blocks
- SQL string literals assigned to or inserted into columns/keys whose names case-insensitively contain `password`, `passwd`, `pwd`, `secret`, `token`, `api_key`, `apikey`, `private_key`, `client_secret`, or `credential`
- SQL DDL credential clauses with string literals, including PostgreSQL `CREATE ROLE` / `ALTER ROLE` / `CREATE USER` / `ALTER USER ... PASSWORD '...'`, MySQL `CREATE USER` / `ALTER USER ... IDENTIFIED BY '...'` and `IDENTIFIED WITH ... BY '...'`, and dialect-equivalent password/authentication clauses
- obvious bearer/API token literals with common prefixes such as `Bearer `, `sk_`, `ghp_`, `github_pat_`, and JWT-shaped three-segment base64url tokens when they appear in SQL/default/view/check text
- allowlist behavior must be explicit and local to the finding site; broad global allowlists are not part of the initial scope
- detector tests must include true positives for each category above, false-positive controls for ordinary identifiers/comments that only mention the words, redacted diagnostics, and `AllowSecretLiterals` downgrade behavior

Database abstraction:

```go
type Connector interface {
	OpenMigrationSession(ctx context.Context) (MigrationSession, error)
}

type MigrationSession interface {
	Dialect() dialect.Dialect
	Capabilities() MigrationCapabilities
	LockIdentity(ctx context.Context, history HistoryOptions) (LockIdentity, error)
	AcquireLock(ctx context.Context, identity LockIdentity) error
	BeginStableSchemaSnapshot(ctx context.Context) (StableSchemaSnapshot, error)
	EndStableSchemaSnapshot(ctx context.Context, snapshot StableSchemaSnapshot) error
	Begin(ctx context.Context, mode TransactionMode) error
	Exec(ctx context.Context, sql string, args ...any) (Result, error)
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
	ReleaseLock(ctx context.Context) error
	Close(ctx context.Context) error
}

type MigrationCapabilities struct {
	RequiresPinnedSession             bool
	SupportsMigrationLock             bool
	SupportsBatchTransaction           bool
	SupportsImmediateTransaction       bool
	SupportsWholeFileMultiStatement    bool
	SupportsStableSchemaSnapshot       bool
	DDLMayImplicitCommit               bool
	PartialApplicationCapable          bool
	SQLiteImmediateBeginActsAsLock     bool
}

type StableSchemaSnapshot struct {
	OwnsTransaction bool
}

type IntrospectionFilter struct {
	SchemaFilters    []string
	TableFilters     []string
	ExtensionFilters []string
}

type IntrospectionOptions struct {
	Filter IntrospectionFilter
	Limits ResourceLimits
}

type IntrospectionSnapshot struct {
	Dialect     string
	DDL         []DDLEntity
	ViewColumns []IntrospectionViewColumn
	Relations   []IntrospectionRelation
}

type IntrospectionViewColumn struct {
	Schema         string
	View           string
	Name           string
	PropertyKey    string
	Type           TypeRef
	TypeDimensions int
	NotNull        bool
}

type IntrospectionRelation struct {
	Schema         string
	Table          string
	Columns        []string
	ForeignSchema  string
	ForeignTable   string
	ForeignColumns []string
}

type IntrospectionSummary struct {
	BroadScan bool
	Counts    map[string]int
	Objects   []IntrospectionObjectRef
}

type IntrospectionObjectRef struct {
	Family string
	Schema string
	Name   string
}

type SchemaRenderOptions struct {
	PackageName string
	Casing      IntrospectCasing
	Limits      ResourceLimits
}

type RenderedSchema struct {
	Files []RenderedSourceFile
}

type RenderedSourceFile struct {
	RelPath string
	Content []byte
}

type IntrospectCasing string

const (
	IntrospectCasingPreserve IntrospectCasing = "preserve"
	IntrospectCasingCamel    IntrospectCasing = "camel"
)

type Introspector interface {
	Introspect(ctx context.Context, session MigrationSession, opts IntrospectionOptions) (*IntrospectionSnapshot, error)
	RenderSchema(ctx context.Context, snapshot *IntrospectionSnapshot, opts SchemaRenderOptions) (*RenderedSchema, error)
}
```

No-error adapter accessors such as `MigrationSession.Dialect()` and `MigrationSession.Capabilities()` must return valid non-nil, non-typed-nil values. Nil or typed-nil dialects fail before comparison/rendering with `unsupported_dialect`; invalid capability values fail with `invalid_config`.

Capability validation matrix:

- `RequiresPinnedSession=true` requires the connector/session implementation to guarantee the same physical session for lock, transaction, introspection, execution, and history operations that depend on that lifecycle; otherwise opening the session fails with `invalid_config`
- `SQLiteImmediateBeginActsAsLock=true` requires `SupportsImmediateTransaction=true` and a session implementation where `AcquireLock` starts or verifies the active immediate transaction; inconsistent combinations fail with `invalid_config`
- if `SupportsMigrationLock=false`, `migrate` and `pull --init` fail before SQL execution unless a dialect-specific no-lock exception is explicitly documented
- if `SupportsBatchTransaction=false`, batch execution must report partial-application risk through `PartialApplicationCapable=true` and `ExecutionError.MayHaveCommitted`; otherwise migration execution fails with `unsupported_feature`
- if `DDLMayImplicitCommit=true`, the adapter must either set `PartialApplicationCapable=true` or reject transactional batch execution before running SQL
- if `RequiresPinnedSession=true` and the connector cannot keep lock/session/history operations on the pinned session, DB-backed commands fail before acquiring locks or executing migration SQL
- invalid combinations fail during session/capability validation, before acquiring locks or executing migration SQL.

`LockIdentity` is the logical migration target identity defined in [file-migrations-execution.md](./file-migrations-execution.md#concurrency-model). The session derives it because only the connected adapter can reliably know the current database/catalog/file target identity. `TransactionMode` is a driver capability request, not a promise that every dialect can roll back every DDL statement.

`MigrationSession.Query` row ownership rules:

- a successful `Query` result transfers ownership of `Rows` to the caller, and the caller must close it exactly once on every success, scan error, early-exit, or cancellation path
- `Query` must not return nil or typed-nil `Rows` with a nil error; doing so is an adapter contract violation surfaced as `invalid_config`
- when row iteration fails, the iteration/scan error is primary; a later `Close` error is only a redacted secondary diagnostic unless no earlier error exists
- context cancellation/deadline observed during iteration or close must preserve the standard context sentinel through `errors.Is` while retaining the outer stable operation code

`RenderedSourceFile` is pre-write output from `Introspector.RenderSchema`. `ManagedFile` is a post-write result carrier returned by `Pull` after `ManagedSourceStore.WriteManagedFile` succeeds or reports an owned managed file. These types must not be conflated because render output must carry source bytes while write results carry digest and write status.

`IntrospectionSnapshot` is a pull/source-generation model, not just a migration `snapshot.json` wrapper. RC.1 keeps some source-generation inputs outside DDL entities, including view-column metadata and relation-generation data derived from foreign keys. Grizzle's pull renderer must receive that metadata explicitly through typed fields or an equivalent dialect-specific pull model; `RenderSchema` must not try to recover it from raw JSON maps or by reparsing rendered SQL.

`IntrospectionViewColumn.PropertyKey` is the resolved generated view property key after applying configured introspection casing. If an adapter leaves it empty, `RenderSchema` must derive it deterministically from `Name` using the same casing rules before rendering; duplicate or empty derived keys fail before file writes. Generated view columns use this key as `sqlmeta.ColumnMeta.SelectionKey`, while SQL scan structs continue to use `db` tags with the physical SQL column name.

Introspection SQL safety rules:

- metadata queries must use parameterized catalog predicates where the database supports them
- catalog-derived identifiers used in SQL, such as names passed to dialect commands like `SHOW CREATE VIEW`, must be quoted with the dialect identifier helper and must never be concatenated as raw SQL fragments
- introspection adapters must include tests for quotes, backticks, newlines, control characters, comment delimiters, and delimiter-like text in database object names
- rendered Go source must preserve database names only through safe Go string quoting/escaping and fixed allowlisted imports

`IntrospectionFilter.SchemaFilters`, `TableFilters`, and `ExtensionFilters` map to RC.1's functional config `schemaFilter`, `tablesFilter`, and `extensionsFilters` semantics. The plural Go field represents the resolved list of schema filters; it is not a claim that RC.1's CLI `schemaFilters` flag is wired into effective filtering. Table filters use glob-style matching against table names only. Schema filters select eligible schemas before table matching. Empty filter slices mean no explicit filtering, subject to dialect visibility and the history-table exclusion rules in [pull.md](./pull.md). Entity filters for roles or other unsupported object families are not initial API fields and must fail during config resolution with `unsupported_feature` if supplied.

`IntrospectionSummary.Counts` is keyed by object family and is safe for CLI summaries. `IntrospectionSummary.Objects` may contain full database object names only when the pull is not a broad scan, or when a future explicit unsafe library opt-in is designed for broad-scan object refs. For broad scans in the initial scope, `Objects` must be nil or empty in both `PullWritePlan.IntrospectionSummary` and `PullResult.IntrospectionSummary`; CLI output must not print full object names because no CLI verbose/explain object-list mode is specified.

Pull/introspection resource limits:

- `Introspector.Introspect` receives `ResourceLimits` through `IntrospectionOptions` so adapters can enforce object count, object-name byte length, per-payload raw default/check/view SQL byte length, and aggregate raw introspection payload bytes while querying/streaming metadata rather than after building an unbounded snapshot
- `RenderSchema` receives `ResourceLimits` through `SchemaRenderOptions` so renderers can enforce rendered source-file byte length and total rendered source bytes while generating output
- bootstrap artifact planning must enforce bootstrap `migration.sql` bytes, bootstrap `snapshot.json` bytes, and total planned write bytes before returning or publishing planned outputs
- limits must be checked before `PullOptions.BeforeWrite` and again before publishing any managed source file, bootstrap artifact, or `pull --init` history metadata
- `PullWritePlan.LimitStatus` must report resolved usage/limit pairs to library callbacks and CLI summaries without exposing raw payloads
- exceeding a limit fails closed with `resource_limit`; the command must not publish partial source files, partial artifacts, or history metadata after a limit failure

The migration session must pin one physical connection or equivalent driver session for lock acquisition, history reads, SQL execution, history inserts, and lock release. This is required for MySQL `GET_LOCK`, PostgreSQL advisory-lock variants, and any driver whose lock semantics are connection-scoped.

The same pinned session must be usable by `Introspector.Introspect` for `pull --init` live-state validation. Pool-level introspection helpers are not sufficient for init validation because they can query a different physical connection than the one holding the migration lock.

Stable schema serialization contract:

- `SupportsStableSchemaSnapshot` means the adapter can establish a schema-stable observation window on the pinned session and prevent relevant external DDL from interleaving through the init history insert, or can prove equivalent DDL serialization/conflict behavior
- `BeginStableSchemaSnapshot` must acquire the adapter's documented schema lock, metadata lock, DDL-serializing transaction mode, or equivalent mechanism and keep it active until `EndStableSchemaSnapshot`
- `StableSchemaSnapshot.OwnsTransaction` is true when stable-schema acquisition already opened the transaction that must contain history validation and insertion; when true, the caller must skip `Begin` but must still finalize the active transaction with the normal `Commit` or `Rollback` methods
- when `StableSchemaSnapshot.OwnsTransaction` is false, the caller must use the normal `Begin` / `Commit` / `Rollback` lifecycle for history metadata writes
- pure MVCC/read snapshot isolation is insufficient by itself unless the database guarantees external DDL cannot commit outside the observed schema window or will conflict before Grizzle inserts bootstrap history
- the stable window must cover fresh live introspection, live-vs-artifact equality validation, history schema/table creation or validation, the empty-history check, and the init history insert
- `EndStableSchemaSnapshot` must release any adapter-owned schema lock or non-transaction state using the same cleanup guarantees as rollback/lock release; it must not be the only place where an owned metadata transaction can commit or roll back
- stable-schema acquisition must use `CommonOptions.LockTimeout` or a caller-controlled bounded context and must map acquisition timeout/cancellation to `migration_lock`
- if `SupportsStableSchemaSnapshot` is false, `pull --init` must fail unless `PullOptions.AssumeQuiescent` is true
- if `PullOptions.AssumeQuiescent` is true, the stable-snapshot methods are not required, but the result diagnostics must include `DiagnosticAssumeQuiescent`
- every adapter claiming `SupportsStableSchemaSnapshot` must include concurrent-DDL tests proving that relevant schema changes cannot be committed between fresh introspection and bootstrap history insertion without being blocked or causing `pull --init` to fail

The concrete implementation may provide adapters for `database/sql`, `pgx`, or dialect-specific drivers, but the file migration package must not require one driver family as its public API.

Dialect authority rule:

- `CommonOptions.Dialect` is the dialect used for artifact validation and SQL generation
- `MigrationSession.Dialect()` is the dialect reported by the opened database session
- DB-backed commands must canonicalize CLI/config, internal, session, and snapshot dialect IDs using the matrix below before comparing values
- DB-backed commands must compare canonical values before reading history, introspecting, executing SQL, or writing metadata
- a mismatch must fail with `dialect_mismatch`
- this rule applies to `migrate`, plain `pull`, and `pull --init`; plain `pull` validates dialect before introspection but does not enter the migration lock/history lifecycle unless `Init` is true

Initial dialect ID matrix:

| CLI/config value | Internal dialect name | Snapshot dialect | Initial DB-backed status |
| --- | --- | --- | --- |
| `postgresql` | `postgres` | `postgres` | supported |
| `mysql` | `mysql` | `mysql` | supported |
| `sqlite` | `sqlite` | `sqlite` | supported |
| `turso` / `libsql` | `sqlite` | `sqlite` | unsupported until driver behavior is designed |
| `cockroach` | `cockroach` | `cockroach` | unsupported initial file-migration target |
| `singlestore` | `singlestore` | `singlestore` | unsupported initial file-migration target |
| `mssql` | `mssql` | `mssql` | unsupported initial file-migration target |

Migration session lifecycle for `migrate` and `pull --init`:

1. `OpenMigrationSession`
2. validate `CommonOptions.Dialect` against `MigrationSession.Dialect()` for DB-backed commands
3. inspect `MigrationSession.Capabilities()` and fail before SQL execution if required lock or transaction support is absent; whole-file execution capability is only relevant to a future disabled-breakpoint design
4. derive `LockIdentity` from `MigrationSession.LockIdentity(ctx, HistoryOptions)`
5. acquire the migration lock using the dialect-specific lifecycle below
6. `Begin` with the requested `TransactionMode` when the lock primitive is not itself the transaction
7. create or validate the history schema/table on the pinned locked session when the command will apply or record migrations
8. history reads, pending computation, SQL execution, and history inserts
9. `Commit` on success or `Rollback` on failure when a transaction is active
10. `ReleaseLock`
11. `Close`

Plain `pull` session lifecycle:

1. open the database/introspection session
2. validate `CommonOptions.Dialect` against the connected session dialect before introspection
3. introspect and render source/bootstrap outputs subject to the pull resource limits
4. call `BeforeWrite` when configured
5. publish managed source files and any managed introspection artifact through the filesystem stores
6. close the session

Plain `pull` must not derive a migration `LockIdentity`, acquire the migration lock, begin a migration/history transaction, create/read/write the history schema/table, or call the runtime migrator. Only `pull --init` may use the migration lock/history lifecycle to record the pulled state as applied.

Dialect-specific lifecycle:

- PostgreSQL: `AcquireLock` uses the session advisory-lock primitive before `Begin(Batch)`
- MySQL: `AcquireLock` uses `GET_LOCK` before `Begin(Batch)` when a transaction is requested
- SQLite: `AcquireLock` must perform `BEGIN IMMEDIATE` and mark the transaction active; a later `Begin(Immediate)` call verifies the active immediate transaction and must not issue a second `BEGIN`
- drivers with non-transactional execution use `Begin(None)` or skip `Begin` according to the adapter contract, but must still acquire any required lock first

Logical cleanup order:

1. `Rollback` on failure when a transaction is active
2. `Commit` on success when a transaction is active
3. `EndStableSchemaSnapshot` when a stable schema lifecycle is active
4. `ReleaseLock`
5. `Close`

Lifecycle rules:

- `Rollback`, `EndStableSchemaSnapshot`, `ReleaseLock`, and `Close` must be safe to call during cleanup after earlier failures
- cleanup after cancellation must not reuse an already-canceled operation context; the implementation must derive a bounded cleanup context or the adapter must use an owned finite release deadline for rollback, stable-schema release, lock release, and close
- if `Commit` fails on a partial-application-capable driver, the command must return `partial_application`
- `Close` must not hide prior execution errors
- lock or stable-schema acquisition timeout/cancellation, and lock or stable-schema release failure, must map to stable `migration_lock` errors
- context cancellation during SQL execution, schema loading, artifact IO, source generation, or `BeforeWrite` must map through the active operation's error code, usually `migration_execution` for SQL execution, while preserving only a redacted safe cause under the shared error contract
- `TransactionMode` must include at least `None`, `Batch`, and `Immediate` so non-transactional drivers and SQLite write-lock behavior are explicit

Additional generate-specific inputs:

- schema definitions or schema loading configuration
- dialect
- optional custom migration name
- optional custom-migration mode
- breakpoints setting, initially required to resolve to enabled

```go
type GenerateOptions struct {
	Common         CommonOptions
	Schema         SchemaInput
	Name           string
	Custom         bool
	Breakpoints    *bool
	RenameResolver RenameResolver
}
```

Generate/check handoff rule:

- `generate` must have all inputs required to run the same local artifact check that `check` would run directly
- in particular, `generate` must pass its resolved migrations output directory and dialect into the internal check step before writing a new artifact
- `GenerateOptions.Schema.Source` must be non-empty for both normal and custom generation; custom generation validates schema input for RC.1 parity but writes snapshot DDL from the checked effective parent
- an intentionally empty target schema is represented by a `SchemaInput` with a valid `Source` and no schema objects
- the dialect used for schema loading, snapshot validation, and migration generation must be the same resolved dialect for one command invocation
- if the dialect cannot be resolved, `generate` must fail before running `check` or writing artifacts

Additional check-specific inputs:

- none beyond local artifact/snapshot inputs in the initial design

```go
type CheckOptions struct {
	Common CommonOptions
}
```

### Direct `push` API Boundary

`push` is part of the Grizzle Kit CLI surface, but it is not part of the `kit/filemigrate` library package because it does not operate on migration artifacts, snapshots, or migration history.

Initial API rule:

- `kit/filemigrate` must not expose `Push`
- direct schema synchronization must be specified in a dedicated push/direct-sync spec before new push CLI work continues
- that future API must use a direct-sync database adapter and a direct-sync lock identity, not the file-migration `MigrationSession` history lock, unless the push spec explicitly chooses to share history lock inputs
- the future push spec must define finite lock timeouts, non-interactive destructive-operation behavior, lock identity, and failure semantics before any DB-mutating push work resumes
- `CommonOptions.MigrationsDir`, history table/schema options, `CheckResult`, and artifact validation do not apply to `push`

### Pull Options

Additional pull-specific inputs:

- resolved `Connector`; CLI/config may construct it from `dbCredentials` and secret references before calling the lower-level API
- dialect
- schema-code output directory
- migrations output directory
- history table/schema options, resolved for every pull so history metadata objects can be excluded from generated schema output
- breakpoints setting, initially required to resolve to enabled
- optional schema/table filters if introspection supports them
- `AllowBroadScan` / CLI `--all-schemas` for filesystem-mutating broad scans without schema/table filters
- `AllowSecretLiterals` / CLI `--allow-secret-literals` as explicit risk acceptance for high-confidence secret-literal findings
- optional library-only `BeforeWrite` callback for pre-write summaries and abort decisions

```go
type PullOptions struct {
	Common          CommonOptions
	DB              Connector
	Introspector    Introspector
	SourceStore     ManagedSourceStore
	BeforeWrite     PullBeforeWriteFunc
	SchemaOut       string
	Init            bool
	AssumeQuiescent bool
	AllowBroadScan  bool
	Breakpoints     *bool
	Casing          IntrospectCasing
	Filter          IntrospectionFilter
}
```

`PullOptions.DB` is required and nil/typed-nil values fail with `invalid_config`. `PullOptions.Introspector` is optional; nil uses the production introspector selected by the resolved dialect/connector, and typed-nil values are treated as absent. `SourceStore` and `BeforeWrite` remain optional as documented.

`PullOptions.AllowBroadScan` is required when no schema or table filters are configured and the command would mutate the filesystem or `pull --init` history metadata. CLI `pull` must expose this as an explicit `--all-schemas` or equivalently named broad-scan opt-in. Non-interactive CLI runs without filters and without this opt-in fail with `broad_introspection_requires_opt_in` before broad introspection, filesystem writes, or history changes. This is **DEVIATION:INTENTIONAL** safety hardening from RC.1's broad default introspection.

`SourceStore` defaults to the production filesystem managed-source store when nil. It is separate from `Common.ArtifactStore` because `schema-out` and `migrations-out` are independent roots with different file ownership rules.

`BeforeWrite`, when non-nil, must be called after introspection, source rendering, bootstrap-artifact planning, and resource-limit evaluation, but before `ManagedSourceStore.WriteManagedFile`, `ArtifactStore.CreateArtifact`, or any `pull --init` history metadata write. The callback receives the same payload-redacted pre-write plan shape used by the CLI broad-scan summary when broad-scan output is required, including `LimitStatus`. Payload-redacted means rendered Go source bytes, SQL bytes, snapshot JSON bytes, raw default expressions, raw view definitions, credentials, and bind values are excluded; structured metadata such as counts, relative output paths, resource-limit status, and summary categories may still be present. Content-derived digests are library-only unsafe-to-log fields; the built-in CLI and broad-scan summaries must omit them. If high-confidence secret-literal findings are downgraded through `AllowSecretLiterals`, implementations must clear all content-derived digest fields in the `BeforeWrite` plan to empty strings before invoking the callback. Full object refs are omitted from broad-scan callback payloads by default: `PullWritePlan.IntrospectionSummary.Objects` must be nil or empty for broad scans unless a future explicit unsafe library opt-in exposes them. If that opt-in is ever added, it must be documented as unsafe to log. If it returns an error, `Pull` must abort without publishing schema files, migration artifacts, or history metadata and return `CodeBeforeWriteAborted` with `Op: "pull.before_write"`. Cancellation/deadline errors returned by the callback must preserve `errors.Is(err, context.Canceled)` or `errors.Is(err, context.DeadlineExceeded)`.

The callback is a library extension point, but callers must not log the whole plan by default. The built-in CLI callback must use only `BroadScan`, `Counts`, diagnostics, planned relative paths, and sizes; it must not print full object names or content-derived digests during broad scans.

`BeforeWrite` must receive deep defensive copies of all maps, slices, pointers, diagnostics, summary objects, and planned-write metadata. Callback mutations must not affect internal write plans, staged outputs, later validation, cleanup behavior, or the returned `PullResult`.

`PullOptions.Casing` resolves to `IntrospectCasingCamel` when empty, matching the RC.1 default. Any non-empty value outside the `IntrospectCasing` enum fails with `invalid_config`.

`PullResult.BootstrapArtifact` is nil when no bootstrap artifact is generated or reused. When `pull` creates a new bootstrap artifact, it is the checked loaded artifact returned after publish. When `pull --init` reuses an existing managed introspection artifact, it is the exact loaded artifact that was validated and recorded in history.

`PullOptions.AssumeQuiescent` is an explicit risk-acceptance switch for `pull --init` only. If `Init` is true and the selected adapter does not report `SupportsStableSchemaSnapshot`, `Pull` must fail with `init_precondition` unless `AssumeQuiescent` is true. When the switch is true, diagnostics must record that live schema stability was assumed rather than proven. The switch must not weaken credential redaction, artifact validation, or history-lock requirements.

`schema-out` and `migrations-out` must not overlap in either direction after canonicalization. `pull` must reject overlapping roots with `invalid_path` unless a future spec defines a discovery-safe nested layout.

### Migrate Options

Additional migrate-specific inputs:

```go
type MigrateOptions struct {
	Common     CommonOptions
	DB         Connector
	AllowEmpty bool
}
```

`MigrateOptions.DB` is required and nil/typed-nil values fail with `invalid_config`; there is no production default connector for `migrate`.

## Check Result Contract

The initial Grizzle library API must surface the same conceptual result shape that Drizzle RC.1 `checkHandler()` uses internally.

Initial library result fields:

- `baseSnapshot`
  - root/common-ancestor snapshot used for branch analysis; never nil on successful `Check`
- `baseID`
  - snapshot ID corresponding to `baseSnapshot`, or the sentinel origin UUID for an empty graph
- `effectiveSnapshot`
  - materialized prior schema state that `generate` must diff against; never nil on successful `Check`
- `effectiveParentIDs`
  - deterministically ordered snapshot IDs that the next generated snapshot must write to `prevIds`; multiple leaf IDs are sorted lexicographically by snapshot `id`
- `branchStatements`
  - ordered dialect JSON diff/commutativity statements used to materialize `effectiveSnapshot` from `baseSnapshot`; these are not executable SQL segments
- `artifactDigests`
  - migration name to exact raw-byte SHA-256 digest metadata for local `migration.sql`, `snapshot.json`, and the required combined artifact digest
- `loadedArtifacts`
  - internal or public carrier for the checked artifact graph, including bytes or stable handles plus digests

Normative API rule:

- the library `Check` operation must return a structured result sufficient for `Generate` to consume directly
- successful `Check` returns `(*CheckResult, nil)`
- on successful `Check`, `BaseSnapshot` and `EffectiveSnapshot` are never nil; for an empty graph both are the dialect root/dry snapshot and `BaseID` / `EffectiveParentIDs` use the sentinel origin UUID
- failed `Check` returns `(nil, error)` unless a future diagnostic carrier is explicitly added
- non-commutative and indeterminate-graph diagnostics belong in typed errors, not in a successful `CheckResult`
- the `Migrate` implementation must execute the loaded artifact graph returned by `Check`, including the exact validated bytes or stable no-follow file handles
- a digest revalidation followed by a fresh path-based read is not sufficient TOCTOU protection
- the initial CLI `grizzle check` command does not need to expose machine-readable output as long as the library API carries the structured result

## Migrate Handoff Contract

`Migrate` must run the same local artifact check before any database mutation.

Normative `Migrate` algorithm:

1. resolve `CommonOptions` and database connection inputs
2. run `Check(ctx, CheckOptions{Common: opts.Common})`
3. fail before opening or mutating database state if `Check` fails
4. carry `CheckResult.LoadedArtifacts` and `CheckResult.ArtifactDigests` into database pending detection and execution
5. open the pinned migration session, acquire the lock, and validate history using that checked artifact set
6. execute pending migrations from the exact checked artifact bytes or stable no-follow handles
7. insert history rows using the checked artifact metadata

`Migrate` must not perform a later best-effort filesystem rescan or fresh path read as the source of executable SQL. Any implementation that revalidates digests for defense in depth must still execute the original checked bytes or stable handles.

## Schema Input Contract

`Generate` must receive schema definitions through an explicit, non-magical `SchemaInput` contract.

Initial public shape:

```go
type SchemaInput struct {
	Source      SchemaSource
	Schemas     []SchemaDef
	Tables      []TableDef
	Enums       []EnumDef
	Views       []ViewDef
	Unsupported []UnsupportedSchemaObject
}

type SchemaSource string

const (
	SchemaSourceRegistry SchemaSource = "registry"
	SchemaSourceParsed   SchemaSource = "parsed"
	SchemaSourceMemory   SchemaSource = "memory"
)

type SchemaDef struct {
	Name string
}

type TableDef struct {
	Dialect      string
	Schema       string
	Name         string
	IsRlsEnabled bool
	Columns      []ColumnDef
	Indexes      []IndexDef
	Constraints  []ConstraintDef
}

type ColumnDef struct {
	Name          string
	Type          TypeRef
	NotNull       bool
	PrimaryKey    bool
	Unique        bool
	AutoIncrement bool
	Default       *DefaultDef
	Generated     *GeneratedDef
	Identity      *IdentityDef
	References    []ForeignKeyDef
	DialectOptions []DialectOption
	Unsupported   []UnsupportedFeature
}

type TypeRef struct {
	Dialect    string
	Name       string
	TypeSchema string
	Args       []Literal
	Mode       string
	Dimensions int
}

type DefaultKind string

const (
	DefaultLiteral DefaultKind = "literal"
	DefaultSQL     DefaultKind = "sql"
)

type DefaultDef struct {
	Kind  DefaultKind
	Value *Literal
	Expr  ddl.Expression
}

type GeneratedDef struct {
	Expression ddl.Expression
	Mode       GeneratedMode
}

type GeneratedMode string

const (
	GeneratedStored  GeneratedMode = "stored"
	GeneratedVirtual GeneratedMode = "virtual"
)

type IdentityDef struct {
	Type     IdentityType
	Sequence SequenceDef
}

type IdentityType string

const (
	IdentityAlways    IdentityType = "always"
	IdentityByDefault IdentityType = "byDefault"
)

type SequenceDef struct {
	Name      string
	Increment string
	MinValue  string
	MaxValue  string
	StartWith string
	Cache     *int64
	Cycle     bool
}

type EnumDef struct {
	Schema string
	Name   string
	Values []string
}

type ViewDef struct {
	Dialect      string
	Schema       string
	Name         string
	Columns      []ViewColumnDef
	Definition   ddl.Expression
	Materialized bool
	IsExisting   bool
	With         []DialectOption
	WithNoData   bool
	Using        string
	Tablespace   string
	Unsupported  []UnsupportedFeature
}

type ViewColumnDef struct {
	Name        string
	PropertyKey string
	Type        TypeRef
	NotNull     bool
	Default     *DefaultDef
}

type IndexDef struct {
	Name         string
	NameExplicit bool
	Columns      []IndexColumnDef
	Unique       bool
	Where        ddl.Expression
	Method       IndexMethod
	Options      []DialectOption
	Unsupported  []UnsupportedFeature
}

type IndexMethod string

type IndexColumnDef struct {
	Name      string
	Desc      bool
	Nulls     NullsOrder
	OpClass   string
	Expr      ddl.Expression
}

type NullsOrder string

const (
	NullsUnspecified NullsOrder = ""
	NullsFirst       NullsOrder = "first"
	NullsLast        NullsOrder = "last"
)

type ConstraintDef struct {
	Name         string
	NameExplicit bool
	Kind         ConstraintKind
	Columns      []string
	Expression   ddl.Expression
	ForeignKey   *ForeignKeyDef
	Unsupported  []UnsupportedFeature
}

type ConstraintKind string

const (
	ConstraintPrimaryKey ConstraintKind = "primary_key"
	ConstraintUnique     ConstraintKind = "unique"
	ConstraintCheck      ConstraintKind = "check"
	ConstraintForeignKey ConstraintKind = "foreign_key"
)

type ForeignKeyDef struct {
	Name             string
	NameExplicit     bool
	Columns          []string
	ForeignSchema    string
	ForeignTable     string
	ForeignColumns   []string
	OnUpdate         ReferentialAction
	OnDelete         ReferentialAction
}

type ReferentialAction string

const (
	ActionUnspecified ReferentialAction = ""
	ActionNoAction    ReferentialAction = "NO ACTION"
	ActionRestrict    ReferentialAction = "RESTRICT"
	ActionCascade     ReferentialAction = "CASCADE"
	ActionSetNull     ReferentialAction = "SET NULL"
	ActionSetDefault  ReferentialAction = "SET DEFAULT"
)

type DialectOption struct {
	Dialect string
	Key     string
	Value   Literal
}

type UnsupportedFeature struct {
	Family string
	Feature string
	Path    string
	Reason  string
}

type UnsupportedSchemaObject struct {
	Family string
	Name   string
	Path   string
	Reason string
}
```

Initial rules:

- schema input must expose all object families in the `generate` column of the object-family matrix in [file-migrations-artifacts.md](./file-migrations-artifacts.md#snapshot-format)
- table definitions must include columns, indexes, primary keys, foreign keys, unique constraints, and supported check constraints
- `TypeRef`, `DefaultDef`, `GeneratedDef`, `IdentityDef`, `ViewDef`, `IndexDef`, `ConstraintDef`, `ForeignKeyDef`, and `DialectOption` values must be validated against explicit dialect-specific allowlists before snapshot serialization; unknown enum values, option keys, modes, methods, null-order values, FK actions, and constraint kinds must fail with `unsupported_feature`
- `TypeRef.TypeSchema` and `TypeRef.Dimensions` carry PostgreSQL RC.1 `typeSchema` and array/vector dimension metadata; if a schema input cannot represent them losslessly, cross-schema named types and dimensioned types must fail with `unsupported_feature`
- `DialectOption` is a typed escape hatch for RC.1 properties that are not cross-dialect; it must not become an unchecked map accepted by the public API
- initial index support is the dialect-specific allowlist in [file-migrations-snapshot-fields.md](./file-migrations-snapshot-fields.md): PostgreSQL accepts btree, non-concurrent, empty-`with`, non-expression columns with default/null opclasses and optional typed partial predicates; MySQL accepts only the documented default/null `using`, `algorithm`, and `lock` values; SQLite accepts only manual, non-expression-column indexes with optional typed partial predicates
- initial MySQL `DialectOption` support is intentionally narrow: column `charSet` and `collation` must be null/absent, index `using` may be null or `btree`, index `algorithm` may be null or `default`, and index `lock` may be null or `default`; all other non-null values fail with `unsupported_feature`
- `Literal` rendering for DDL must be dialect-specific and must safely inline values for snapshot fields: strings and bytes must be escaped, unsafe control characters must be rejected where the target dialect cannot represent them safely, numeric and boolean rendering must be deterministic, and null/list/enum handling must be explicit
- `ddl.Expression` values in public schema input must come from Grizzle's typed SQL-expression renderer and must render to a dialect-specific SQL string suitable for RC.1 snapshot fields
- `ddl.Expression` is a sealed interface owned by `github.com/sofired/grizzle/schema/ddl`, not by dialect-specific schema packages; external schema DSL packages must construct DDL expressions through exported constructors in that owning package
- the minimum public `schema/ddl` constructor surface is `Lit(value)`, `Ident(part)`, `Table(name)`, `SchemaTable(schema, name)`, `Column(name)` / `TableRef.Col(name)`, comparison/logical operators such as `EQ`, `GT`, `And`, `Or`, null predicates such as `IsNull`, function calls such as `Call(name, args...)`, `Select(...)` for view definitions, and `RawTrusted(sql)` for explicit trusted-input DDL escape hatches
- dialect packages may add supported DDL-expression nodes only in the shared package or through exported constructors there, avoiding import cycles while preserving the sealed set of renderable DDL nodes
- DDL expression rendering follows Drizzle RC.1's `inlineParams()` intent: typed literal parameters may be safely literalized through `ddl.LiteralRenderer`, but no residual driver bind arguments may remain after rendering
- identifier inputs to DDL constructors are trusted identifier parts, not SQL fragments. `Ident(part)`, `Table(name)`, `Column(name)`, and `TableRef.Col(name)` accept one identifier part; schema-qualified constructors such as `SchemaTable(schema, name)` accept parts separately. Empty names, dotted names, NUL/control characters, or pre-quoted fragments must fail with `invalid_identifier`; dialect renderers quote each accepted part.
- `Call(name, args...)` accepts only a validated function identifier or an explicit structured function reference. It must not accept arbitrary SQL syntax such as `now() filter (...)` as a name. Function syntax that cannot be expressed as validated identifier parts requires `ddl.RawTrusted(sql)`.
- literal values in DDL expressions must flow through `ddl.Lit(value)` and the dialect literalizer. Public callers must not interpolate values into raw strings to build defaults, checks, indexes, or views.
- `ddl.Expression` is intentionally sealed by an unexported method; public callers must obtain values through Grizzle's typed constructors, not by implementing the interface with raw strings
- query `Expression.RenderSQL(ctx *BuildContext)` is not DDL-safe by default because it binds values through `BuildContext.Add`; DDL-safe expressions must render through `ddl.BuildContext` and `ddl.LiteralRenderer` without producing driver placeholders
- shared query/DDL AST nodes are allowed only when they implement the sealed DDL render path explicitly; an adapter from query expressions to DDL expressions must validate that every value is literalizable and that no raw SQL or residual placeholders remain
- runtime-only binds, untyped raw strings, unsafe raw fragments, or expressions whose parameters cannot be safely literalized must fail with `unsupported_feature` or a more specific validation error before snapshot serialization
- `ddl.BuildContext` is a DDL-only rendering context; if shared query expressions are reused, their render path must route values through the DDL literalizer rather than query placeholders because RC.1 snapshot fields store rendered SQL strings, not parameter arrays
- untyped raw SQL strings are internal-only carriers for introspection output and tests; they are not public file-migration `SchemaInput`. Public raw DDL is allowed only through an explicit `ddl.RawTrusted(sql)` constructor, which is a trusted-input escape hatch and must follow the same redaction and literalization rules as other DDL expressions.
- `GeneratedDef` models generated column storage mode (`stored`/`virtual`) and `IdentityDef` models PostgreSQL identity columns (`always`/`byDefault`), but both are recognized-yet-unsupported in the initial public schema input until corresponding schema DSL, diff, and SQL generation behavior is fully specified
- `TableDef.IsRlsEnabled=true` is recognized-yet-unsupported in the initial public schema input and must fail with `unsupported_feature`
- PostgreSQL policy records are a recognized RC.1 object family but are not an initial public schema-input family; if encountered through parsing, introspection, or snapshot validation, they fail with `unsupported_object_family`
- `ViewDef.IsExisting=true`, `ViewDef.Materialized=true`, PostgreSQL materialized-view-only fields (`WithNoData`, `Using`, `Tablespace`), PostgreSQL view storage/security options outside the basic regular-view subset, MySQL non-default view algorithm/security/check options, and SQLite existing/error view forms are recognized-yet-unsupported in the initial public schema input unless the view support matrix in [file-migrations-snapshot-fields.md](./file-migrations-snapshot-fields.md) marks the exact combination as supported
- schema, enum, and view definitions are dialect-gated and are emitted only for dialects that support them in the initial object-family matrix
- `Unsupported` records recognized object families or features that cannot be emitted by the initial generator; non-empty unsupported entries must fail with `unsupported_object_family` or `unsupported_feature` as appropriate
- the implementation may use a Go schema registry, parser output, or in-memory schema objects, but it must convert them into the RC.1-style `ddl` entity model before writing `snapshot.json`
- default schema loading must not execute arbitrary target-project package initialization code; trusted executable loading is future scope and must be explicitly designed before it is added

Strict schema loading rule:

- the loader that produces `SchemaInput` for file migrations must be lossless for the object families it claims to support
- parser or registry paths must not silently drop unsupported column modifiers, indexes, constraints, views, enums, or dialect-specific schema features
- any recognized but unsupported construct must be represented in `Unsupported` and cause `generate` to fail before diffing
- current code-generation parser behavior is not sufficient by itself unless it is hardened to meet this strict-loader contract

### Schema Loader Boundary

The initial library API may accept already-materialized `SchemaInput`. CLI `generate --schema` is implementation-ready only after the loader below is implemented or replaced by an equally explicit loader spec.

Static Go schema loader v1:

- resolves every explicitly configured file or directory through a canonical input root, rejects paths outside the root, rejects symlinked roots, and uses no-follow file opening for parsed source files
- explicitly configured schema paths that are missing, symlinks, hardlinks where detectable, non-regular files, device files, FIFOs, sockets, or generated output child directories fail with `invalid_path`; they must not be silently skipped
- discovered child `.go` files with unsafe metadata (symlink, hardlink where detectable, non-regular file) fail with `invalid_path` so schema cannot be omitted silently
- directory traversal skips only documented ignored child directories: `.git`, `.hg`, `.svn`, `vendor`, `node_modules`, generated-code output child directories, the configured migrations output directory, and private Grizzle temporary directories whose names use the reserved prefix documented by the artifact/source store. If the schema root is the same directory as the generated-code output root, traversal must not skip the root; it skips only managed generated files by header/pattern.
- non-Go files discovered under an accepted directory are ignored; explicitly configuring a non-Go file fails with `invalid_path`
- enforces `ResourceLimits` for source file count, per-file source bytes, total source bytes, AST node count, AST depth, declaration count, and literal byte length; exceeding those limits fails with `resource_limit`
- invalid schema input paths, containment failures, symlink traversal, or non-regular files fail with `invalid_path`
- cancellation must be checked during traversal, reading, parsing, and AST evaluation; cancellation/deadline errors preserve standard context sentinels with redacted diagnostics
- diagnostics must include source file and line when available, but must not print arbitrary source literals, comments, credentials, raw SQL payloads, or large expression text
- parses explicitly configured Go files or directories with `go/parser` / `go/ast`; it must not compile, run tests, invoke `go run`, load plugins, or execute package `init()` functions
- file selection must delegate to `go/build.Context.MatchFile` or an equivalent implementation for `//go:build`, legacy `// +build`, `_test.go`, and GOOS/GOARCH filename suffix handling
- the initial context uses process `GOOS` / `GOARCH` environment values when set, otherwise `runtime.GOOS` / `runtime.GOARCH`; `Compiler` is the current toolchain compiler, `CgoEnabled` is false unless a future option explicitly enables cgo-aware schema files, and `ReleaseTags` use the current Go toolchain release tags
- initial CLI/config has no custom build-tag field; files requiring unsatisfied custom tags are skipped as inactive, and custom-tag activation is future scope
- build-constraint parse errors, multiple active packages in one input directory, or duplicate active top-level schema declarations that make the effective schema ambiguous fail with `unsupported_schema_construct`
- recognizes imports of the Grizzle schema DSL packages by import path, not by local alias name
- accepts top-level schema declarations that are direct calls to supported schema builder functions or simple variables/constants referenced by those calls
- evaluates only literal string, integer, boolean, and string-slice values plus constants composed from those literals
- rejects helper functions, loops, reflection, side-effectful package variables, values imported from arbitrary non-Grizzle packages, and dynamic expressions as `unsupported_schema_construct`
- preserves source positions for unsupported constructs so diagnostics can point to the exact file and line
- converts accepted declarations into the same `SchemaInput` model used by the library API before diffing

Recognized schema constructs that are valid Go but unsupported by the file-migration serializer must be recorded as `Unsupported` and fail with `unsupported_feature` or `unsupported_object_family` before snapshot generation.

Constraint-callback loader rule:

- CLI `generate --schema` is not implementation-ready until the loader can either statically evaluate the supported `WithConstraints(func(TableRef) []Constraint { ... })` subset or require an equivalent generated registry input
- the supported AST subset must include direct `TableRef.Col("name")` references, supported index/unique/foreign-key/check builder calls, and typed DDL-expression calls with literalizable values
- helper functions, captured variables outside the allowed literal/registry subset, loops, reflection, and arbitrary expression construction remain `unsupported_schema_construct`
- if this subset is not implemented, the CLI must fail before diffing rather than silently dropping table-level constraints

View loader rule:

- the static loader accepts regular view declarations such as `pg.CreateView(name).As(ddl.Select(...))`, `pg.SchemaView(schema, name).As(...)`, `mysql.CreateView(name).As(...)`, and `sqlite.CreateView(name).As(...)` when the `.As(...)` argument is a statically loadable `ddl.Expression`; schema-qualified MySQL views are not initial file-migration input because RC.1 Kit snapshots do not carry a MySQL view schema field
- `ddl.RawTrusted(sql)` is accepted for view definitions only when `sql` is a literal string or constant composed from literal strings; legacy raw string overloads such as `pg.CreateView(name, sql string)` are not accepted by the strict loader and fail with `unsupported_schema_construct`
- helper functions, callbacks that build views dynamically, non-literal raw SQL, query-builder handles from generated runtime query packages, or unsupported view options fail with `unsupported_schema_construct` or `unsupported_feature` before diffing
- schema-time references inside view expressions use `ddl.Table(name).Col(col)` or `ddl.SchemaTable(schema, name).Col(col)`, not generated query handles such as `db.UsersT`

## Rename Resolver Contract

`Generate` must not read directly from standard input.

Initial public shape:

```go
type RenameResolver interface {
	ResolveRenames(ctx context.Context, prompt RenamePrompt) ([]RenameDecision, error)
}

type RenamePrompt struct {
	Dialect  string
	Created  []EntityRef
	Deleted  []EntityRef
}

type EntityRef struct {
	Family string
	Schema string
	Table  string
	Name   string
	Path   string
}

type RenameKind string

const (
	RenameSchema     RenameKind = "schema"
	RenameTable      RenameKind = "table"
	RenameColumn     RenameKind = "column"
	RenameEnum       RenameKind = "enum"
	RenameView       RenameKind = "view"
	RenameIndex      RenameKind = "index"
	RenameConstraint RenameKind = "constraint"
)

type RenameDecision struct {
	From EntityRef
	To   EntityRef
	Kind RenameKind
}
```

Initial rules:

- `RenameResolver` is the library boundary for Drizzle RC.1-style rename prompts
- `Generate` must pass a deep defensive copy of `RenamePrompt`, including copied `Created` / `Deleted` slices and `EntityRef` values; resolver mutations must not affect diff inputs, diagnostics, or generated artifact metadata
- returned decisions must be validated and copied before use; later resolver-side mutation of the returned slice or nested values must not affect generation
- rename decisions use canonical `EntityRef` values, not display strings; `Path` is a stable slash-separated entity path such as `schema/table/column`
- `RenameDecision.Kind` must be one of the `RenameKind` constants and must match the entity family represented by `From` and `To`
- `From.Family` and `To.Family` must match; cross-family rename decisions fail before diff statement generation
- dialects may accept only the rename kinds supported by their RC.1 diff/statement path; unsupported rename kinds fail with `unsupported_feature`
- artifact rename metadata must be serialized with the RC.1-compatible encoder defined in [file-migrations-artifacts.md](./file-migrations-artifacts.md#snapshot-format)
- the CLI resolver may prompt interactively when a TTY is available
- when ambiguous create/drop pairs require a prompt and no resolver or TTY is available, `Generate` must fail with a stable `interactive_required` error
- non-interactive answer files or config-based rename mappings remain future scope tracked in `sofired/grizzle#279`

## Artifact Store Contract

`ArtifactStore` must support the strict artifact rules in [file-migrations-artifacts.md](./file-migrations-artifacts.md).

Initial public shape:

```go
type ArtifactStore interface {
	ResolveRoot(ctx context.Context, dir string, opts ResolveArtifactRootOptions) (ArtifactRoot, error)
	ListArtifacts(ctx context.Context, root ArtifactRoot, opts ListArtifactsOptions) ([]ArtifactEntry, error)
	ReadArtifact(ctx context.Context, root ArtifactRoot, name string, opts ReadArtifactOptions) (*LoadedArtifact, error)
	CreateArtifact(ctx context.Context, root ArtifactRoot, artifact NewArtifact, opts CreateArtifactOptions) (*LoadedArtifact, error)
}

type ResolveArtifactRootOptions struct {
	Mode   ArtifactRootMode
	Limits ResourceLimits
}

type ListArtifactsOptions struct {
	Limits ResourceLimits
}

type ReadArtifactOptions struct {
	Limits ResourceLimits
}

type CreateArtifactOptions struct {
	Limits ResourceLimits
}

type ArtifactRootMode string

const (
	RootReadForCheck    ArtifactRootMode = "read_for_check"
	RootReadForMigrate  ArtifactRootMode = "read_for_migrate"
	RootEnsureForWrite  ArtifactRootMode = "ensure_for_write"
)

type ArtifactRoot struct {
	Configured string
	RealPath   string
	State      ArtifactRootState
}

type ArtifactRootState string

const (
	RootAbsent   ArtifactRootState = "absent"
	RootExisting ArtifactRootState = "existing"
	RootCreated  ArtifactRootState = "created"
)

type ArtifactEntry struct {
	Name string
	Path string
}

type NewArtifact struct {
	Name         string
	MigrationSQL []byte
	SnapshotJSON []byte
}

type ManagedWriteOptions struct {
	Header        string
	Limits        ResourceLimits
}

type ManagedSourceStore interface {
	ResolveSourceRoot(ctx context.Context, dir string) (SourceRoot, error)
	WriteManagedFile(ctx context.Context, root SourceRoot, relpath string, content []byte, opts ManagedWriteOptions) (ManagedFile, error)
}

type SourceRoot struct {
	Configured string
	RealPath   string
}
```

Contract requirements:

- `ResolveRoot` must `Lstat` the configured migrations root, reject any symlinked root in the initial implementation, and carry `ResolveArtifactRootOptions.Limits` into the returned root or later store operations as needed for custom stores
- `RootReadForCheck` may return `ArtifactRoot{State: RootAbsent}` for an absent migrations directory so `check` can model an empty graph without inferring absence from empty paths
- `RootReadForMigrate` must fail for an absent migrations directory unless `migrate` has already selected the controlled `AllowEmpty` path
- `RootEnsureForWrite` may create an absent root using the write-safety rules in [file-migrations-artifacts.md](./file-migrations-artifacts.md#output-write-safety) and must return `State: RootCreated` when it creates the root
- `ListArtifacts` lists immediate child migration directories, applies the root-file, temp-directory, and legacy-marker rules from the artifact spec, and enforces `ListArtifactsOptions.Limits` before returning unbounded entries
- `ReadArtifact` uses `Lstat`-style metadata checks, rejects symlinks, rejects hardlinks where platform metadata allows it, validates regular files, preserves exact raw bytes, and enforces `ReadArtifactOptions.Limits` before buffering or parsing artifact files
- `CreateArtifact` validates `NewArtifact.Name` as a supported migration identity before any path construction; invalid names, path separators, dot segments, nested names, absolute paths, and names that would not resolve to exactly one immediate child directory under the canonical root fail with `invalid_migration_name`. It then creates a new real migration directory and writes `migration.sql` plus `snapshot.json` using observable publish invariants: failed writes must not leave a discoverable valid artifact, publish uses a private same-filesystem temporary sibling directory plus atomic rename where supported, final bytes/digests are verified after publish, and platform-specific file/parent-directory fsync behavior must be documented in adapter tests. It must enforce `CreateArtifactOptions.Limits` before staging or publishing oversized artifact bytes. If atomic publication is unavailable, it must fail closed unless a future fallback spec preserves the same invariants. Conformance tests must cover traversal, nested-name, absolute-path, duplicate-name, and overlong-name failures.

Managed source store requirements:

- `ResolveSourceRoot` applies the same root containment posture to `schema-out`: `Lstat` existing roots, reject symlinked roots unless a future opt-in is designed, canonicalize absent-root parents before creation, and canonicalize the created root before writing
- `WriteManagedFile` is used for `schema.go` and `relations.go`, must not follow symlinks, must reject hardlinks/non-regular files where detectable, must enforce `ManagedWriteOptions.Limits` before writing content, must overwrite only files carrying the recognized managed header for that command, must reject unowned files with `managed_file_overwrite`, and must return the post-write `ManagedFile` digest/status used in `PullResult.SchemaFiles`
- migration artifact roots and managed source roots are independent; resolving one must not authorize writes to the other

## Pull Contract

`pull` is the Go equivalent of Drizzle's database-to-schema introspection workflow.

Normative responsibilities:

- inspect a live database schema
- translate the introspected schema into Go schema builder definitions
- write Go source files to the configured schema output directory
- avoid mutating application schema objects
- optionally write a bootstrap introspection migration artifact to the migrations output directory when no snapshots exist there
- support `init` metadata bootstrap exactly as defined in [pull.md](./pull.md)

Target output contract:

- generated files are Go schema-definition source files, not SQL migrations
- generated files must carry the exact managed `grizzle pull` header defined in [pull.md](./pull.md)
- generated schema files are managed outputs; later `pull` runs may overwrite only files with the recognized managed header and must reject unowned files
- all filesystem-mutating pull paths must run the `BeforeWrite` callback, when configured, before the first managed source write, bootstrap artifact publish, or init history metadata write; CLI broad-scan summaries are implemented through this pre-write plan path, not by waiting for `PullResult`

## Direct `push` Contract Boundary

`push` is the direct-apply shortcut equivalent to Drizzle's schema-to-database sync workflow.

This file records the boundary only. A dedicated push/direct-sync spec must define the full safety contract before substantial new `push` CLI work continues.

Boundary rules:

- `push` does not create migration artifacts
- `push` is a development-oriented shortcut, not the primary deployment workflow
- `push` must remain operationally distinct from `migrate`
- no implementation-ready `push` flags, config fields, API types, lock identity, destructive-operation policy, dry-run behavior, or CI semantics are specified here
- those details belong only in the dedicated push/direct-sync spec

## Error Contracts

Errors must be explicit and categorized by workflow stage.

Required error classes:

- unsupported artifact format
- unsupported history-table schema
- invalid config
- config collision
- malformed snapshot
- obsolete snapshot version
- unsupported snapshot version
- non-commutative migration conflict
- migration identity drift detected
- migration execution failure
- migration lock acquisition or release failure
- concurrency / duplicate migration identity failure
- history row without matching local artifact
- invalid migration name
- dialect mismatch between config and database session
- invalid identifier
- path containment failure
- init precondition failure
- empty migrations directory
- bootstrap init required
- partial application
- interactive input required
- before-write callback abort
- unsupported object family
- unsupported feature inside a recognized object family
- unsupported static schema construct
- unsupported dialect
- managed file overwrite rejected
- invalid statement segmentation
- identifier collision in generated Go output
- invalid migration graph
- resource limit exceeded

Normative behavior:

- mutating commands must fail fast on precondition errors
- `check` failures must be surfaced distinctly from SQL execution failures
- unsupported legacy formats must produce direct unsupported-format errors, not fallback behavior
- errors must support stable programmatic classification through `errors.Is` sentinels and stable `ErrorCode` values

Initial typed error shape:

```go
type ErrorCode string

type Error struct {
	Code      ErrorCode
	Op        string
	Path      string
	Migration string
	Dialect   string
	Err       error // redacted safe cause only
}

type HistoryStage string

const (
	HistoryStageNone   HistoryStage = "none"
	HistoryStageRead   HistoryStage = "read"
	HistoryStageCreate HistoryStage = "create"
	HistoryStageInsert HistoryStage = "insert"
)

type ExecutionError struct {
	Error
	StatementIndex   *int
	HistoryStage     HistoryStage
	MayHaveCommitted bool
}

type PartialApplicationError struct {
	ExecutionError
	HistoryInsertStarted bool
	HistoryInsertSucceeded bool
}
```

`Error.Path` must use the same safe-rendered path contract as `Diagnostic.Path`: canonicalized or root-relative where possible, control characters escaped, length bounded, and never a raw config/artifact path that could contain credentials, DSNs, or terminal-control text. `errors.As` callers must not be able to recover an unsafe raw path through exported error fields.

`ErrorCode` values must be stable across patch releases once this API is published.

`Code...` constants are stable string codes. `Err...` identifiers are error sentinels. `errors.Is(err, ErrUnsupportedArtifactFormat)` must match an `*Error` whose `Code` is `CodeUnsupportedArtifactFormat`.

Initial `ErrorCode` constants:

```go
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
	CodeInvalidIdentifier           ErrorCode = "invalid_identifier"
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
```

Initial sentinel shape:

```go
type CodeSentinel struct {
	Code ErrorCode
}

func (s CodeSentinel) Error() string {
	return string(s.Code)
}

func (e *Error) Is(target error) bool {
	s, ok := target.(CodeSentinel)
	return ok && e.Code == s.Code
}

func (e *Error) Unwrap() error {
	return e.Err
}

var (
	ErrUnsupportedArtifactFormat = CodeSentinel{CodeUnsupportedArtifactFormat}
	ErrUnsupportedHistorySchema  = CodeSentinel{CodeUnsupportedHistorySchema}
	// One sentinel must exist for every stable ErrorCode.
)
```

`Error` must implement `Unwrap() error`, but `Err` and `Unwrap()` may expose only redacted safe causes, stable sentinels, or the standard context sentinels `context.Canceled` and `context.DeadlineExceeded`. Raw driver errors, raw config parse errors, DSNs, SQL text, bind values, and password-bearing values must remain internal and must not be recoverable through `Error()`, `Unwrap()`, logs, verbose diagnostics, or formatted `%+v` output. For cancellation and deadline failures, `errors.Is(err, context.Canceled)` or `errors.Is(err, context.DeadlineExceeded)` must work while the outer Grizzle error still exposes the stable operation-specific `ErrorCode`. Tests must prove credential, SQL text, bind-value redaction, and context sentinel preservation across rendered errors and unwrapped causes.

`PartialApplicationError` must include migration name, statement index when known, whether SQL may have committed, whether history insertion started, and whether the history insert succeeded. A nil `StatementIndex` means the failing statement is unknown; non-nil indexes are zero-based executable segment indexes. Error rendering must apply the credential and SQL-output redaction rules before returning text to CLI output, logs, or verbose diagnostics.

Failure-to-code mapping:

| Failure | Required code |
| --- | --- |
| invalid or empty statement segment | `invalid_statement_segmentation` |
| user-supplied `Breakpoints=false` / `--breakpoints=false` in the initial target | `unsupported_feature` |
| forbidden CLI/config overlay when config mode is active | `config_collision` |
| missing required config/CLI input or invalid resolved config value | `invalid_config` |
| filesystem-mutating `pull` has no schema/table filters and no `AllowBroadScan` / `--all-schemas` opt-in | `broad_introspection_requires_opt_in` |
| high-confidence secret-literal finding without `AllowSecretLiterals` / `--allow-secret-literals` | `secret_literal` |
| generated Go identifier collision during `pull` | `identifier_collision` |
| `PullOptions.BeforeWrite` returns a non-context error | `before_write_aborted` |
| `PullOptions.BeforeWrite` returns or observes context cancellation/deadline | `before_write_aborted`, preserving `context.Canceled` or `context.DeadlineExceeded` through `errors.Is` |
| artifact/source/introspection/resource usage exceeds resolved limits | `resource_limit` |
| invalid SQL identifier value in schema, history, artifact, or generated source input | `invalid_identifier` |
| unresolved parent snapshot, cycle, or invalid branch graph | `invalid_migration_graph` |
| recognized object family with unsupported field/property | `unsupported_feature` |
| static Go schema loader cannot safely evaluate a construct | `unsupported_schema_construct` |
| snapshot version older than supported RC.1 target | `obsolete_snapshot_version` |
| snapshot version newer or outside supported family | `unsupported_snapshot_version` |

## Read / Write Boundaries

These categories are about database access and filesystem mutation and are not mutually exclusive.

No-database-access commands:

- `check`
- `generate`

Database-read-only commands:

- `pull` without `--init`, relative to application schema

Database-mutating commands:

- `migrate`
- `push`
- `pull --init`, limited to migration metadata bootstrap

Filesystem-mutating commands:

- `generate`
- `pull`

Normative rules:

- `check` must not mutate database schema or migration history
- `generate` mutates the local artifact set only
- `migrate` mutates the database only by applying existing artifacts and recording history; artifact SQL may perform schema, data, or operational effects, especially for custom migrations
- `push` mutates database schema directly without creating migration artifacts, but this line is only a side-effect classification; implementation-ready `push` behavior belongs to the dedicated direct-sync spec
- `pull` must not apply migrations
- `pull` must not mutate live application schema
- `pull` mutates local generated schema source files
- `pull` may also write local bootstrap migration artifacts to the configured migrations output directory when no snapshots exist there
- `pull --init` may write migration metadata to the database as defined in [pull.md](./pull.md), including creating the history schema/table if absent, but it must not alter application schema objects

## Replacement / Deprecation Strategy

The upstream-inspired rule for Grizzle is:

- add the complete file-based workflow surface first
- only then repurpose or remove older behavior

The initial API contract must also make clear:

- file-based commands operate only on the supported RC.1-style artifact layout
- file-based commands expect the supported RC.1-style history-table shape
- commands do not auto-upgrade old artifact layouts or old table schemas
- this is an intentional initial-scope exclusion, not an unimplemented required parity feature
- commands do not provide a validation-bypass flag for migration-history conflicts in the initial design

Migration of existing behavior:

- existing pre-file-based `migrate` semantics must not survive under the same name once the new workflow is launched
- if transitional compatibility is needed during implementation, it must be temporary and explicitly documented, not left as ambiguous dual behavior

## Driver Adapter Scope

Initial scope decision:

- `filemigrate` exposes the driver-neutral `Connector` / `MigrationSession` interfaces above as its public contract
- concrete driver adapters may live in separate packages
- at least one production adapter must exist before the CLI can call DB-backed operations
- adding adapters for `database/sql`, `pgx`, or dialect-specific drivers must not change the `filemigrate` API shape
