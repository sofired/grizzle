package filemigrate

import "fmt"

// ResourceLimitName is the stable string key used in [ResourceLimitStatus].
// Each constant corresponds to one field of [ResourceLimits].
type ResourceLimitName string

// ResourceLimitName constants. Every public ResourceLimits field has a matching
// constant here except MaxSecretValueBytes, which is enforced internally and
// intentionally omitted from public LimitStatus output.
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
	// LimitMaxSecretValueBytes is intentionally absent: secret-value usage must
	// not be exposed in public LimitStatus output.
)

// ResourceLimits bounds the resources that filemigrate operations may consume.
// Zero-valued fields use the production defaults returned by [DefaultLimits].
// Negative values are rejected with [CodeInvalidConfig].
//
// Aggregate limits (MaxTotal*, MaxRenderedSource*, MaxPlannedWrite*,
// MaxSchemaSource*, MaxIntrospectionPayload*) are measured across an entire
// command invocation. Per-item limits (MaxMigrationSQL*, MaxSnapshotJSON*,
// MaxRenderedSourceFile*, MaxSchemaSourceFile*, MaxObjectName*,
// MaxIntrospectionSQL*) are measured per individual artifact/file/payload.
type ResourceLimits struct {
	// Artifact counts and sizes.
	MaxArtifacts          int
	MaxArtifactDirEntries int
	MaxTotalArtifactBytes int64
	MaxMigrationSQLBytes  int64
	MaxSnapshotJSONBytes  int64
	MaxSnapshotJSONDepth  int
	MaxSnapshotEntities   int

	// Temp and cleanup.
	MaxTempCleanupEntries int

	// Introspection.
	MaxIntrospectionObjects      int
	MaxObjectNameBytes           int
	MaxIntrospectionSQLBytes     int64
	MaxIntrospectionPayloadBytes int64

	// Rendered source output.
	MaxRenderedSourceFileBytes int64
	MaxRenderedSourceBytes     int64

	// Bootstrap artifact sizes.
	MaxBootstrapMigrationSQLBytes int64
	MaxBootstrapSnapshotJSONBytes int64

	// Planned write aggregate.
	MaxPlannedWriteBytes int64

	// Schema source input.
	MaxSchemaFiles           int
	MaxSchemaSourceFileBytes int64
	MaxSchemaSourceBytes     int64
	MaxSchemaASTNodes        int
	MaxSchemaASTDepth        int
	MaxSchemaDeclarations    int
	MaxSchemaLiteralBytes    int64

	// Secret enforcement — not published in ResourceLimitStatus.
	MaxSecretValueBytes int64
}

// ResourceLimitStatus is a single usage/limit pair included in operation results.
// Used and Limit carry counts for Max*Objects/Entries/Files/Entities/Nodes/Depth/
// Declarations fields, and byte counts for public Max*Bytes fields.
//
// ResourceLimitStatus must not carry raw SQL, source bytes, snapshot JSON,
// object payloads, credentials, bind values, or secret-value usage.
type ResourceLimitStatus struct {
	Name  ResourceLimitName
	Used  int64
	Limit int64
}

// DefaultLimits returns the production resource-limit profile. Zero-valued
// [ResourceLimits] fields resolve to these values at enforcement time.
func DefaultLimits() ResourceLimits {
	return ResourceLimits{
		MaxArtifacts:                  10_000,
		MaxArtifactDirEntries:         50_000,
		MaxTotalArtifactBytes:         536_870_912, // 512 MiB
		MaxMigrationSQLBytes:          16_777_216,  // 16 MiB
		MaxSnapshotJSONBytes:          16_777_216,  // 16 MiB
		MaxSnapshotJSONDepth:          128,
		MaxSnapshotEntities:           100_000,
		MaxTempCleanupEntries:         10_000,
		MaxIntrospectionObjects:       50_000,
		MaxObjectNameBytes:            512,
		MaxIntrospectionSQLBytes:      1_048_576,   // 1 MiB
		MaxIntrospectionPayloadBytes:  67_108_864,  // 64 MiB
		MaxRenderedSourceFileBytes:    8_388_608,   // 8 MiB
		MaxRenderedSourceBytes:        134_217_728, // 128 MiB
		MaxBootstrapMigrationSQLBytes: 16_777_216,  // 16 MiB
		MaxBootstrapSnapshotJSONBytes: 16_777_216,  // 16 MiB
		MaxPlannedWriteBytes:          268_435_456, // 256 MiB
		MaxSchemaFiles:                1_000,
		MaxSchemaSourceFileBytes:      8_388_608,   // 8 MiB
		MaxSchemaSourceBytes:          134_217_728, // 128 MiB
		MaxSchemaASTNodes:             1_000_000,
		MaxSchemaASTDepth:             128,
		MaxSchemaDeclarations:         100_000,
		MaxSchemaLiteralBytes:         1_048_576, // 1 MiB
		MaxSecretValueBytes:           65_536,    // 64 KiB
	}
}

// resolve returns a ResourceLimits where every zero field is replaced by the
// corresponding value from DefaultLimits. Negative fields are not validated
// here; callers should call validate before use.
func (l ResourceLimits) resolve() ResourceLimits {
	d := DefaultLimits()
	if l.MaxArtifacts == 0 {
		l.MaxArtifacts = d.MaxArtifacts
	}
	if l.MaxArtifactDirEntries == 0 {
		l.MaxArtifactDirEntries = d.MaxArtifactDirEntries
	}
	if l.MaxTotalArtifactBytes == 0 {
		l.MaxTotalArtifactBytes = d.MaxTotalArtifactBytes
	}
	if l.MaxMigrationSQLBytes == 0 {
		l.MaxMigrationSQLBytes = d.MaxMigrationSQLBytes
	}
	if l.MaxSnapshotJSONBytes == 0 {
		l.MaxSnapshotJSONBytes = d.MaxSnapshotJSONBytes
	}
	if l.MaxSnapshotJSONDepth == 0 {
		l.MaxSnapshotJSONDepth = d.MaxSnapshotJSONDepth
	}
	if l.MaxSnapshotEntities == 0 {
		l.MaxSnapshotEntities = d.MaxSnapshotEntities
	}
	if l.MaxTempCleanupEntries == 0 {
		l.MaxTempCleanupEntries = d.MaxTempCleanupEntries
	}
	if l.MaxIntrospectionObjects == 0 {
		l.MaxIntrospectionObjects = d.MaxIntrospectionObjects
	}
	if l.MaxObjectNameBytes == 0 {
		l.MaxObjectNameBytes = d.MaxObjectNameBytes
	}
	if l.MaxIntrospectionSQLBytes == 0 {
		l.MaxIntrospectionSQLBytes = d.MaxIntrospectionSQLBytes
	}
	if l.MaxIntrospectionPayloadBytes == 0 {
		l.MaxIntrospectionPayloadBytes = d.MaxIntrospectionPayloadBytes
	}
	if l.MaxRenderedSourceFileBytes == 0 {
		l.MaxRenderedSourceFileBytes = d.MaxRenderedSourceFileBytes
	}
	if l.MaxRenderedSourceBytes == 0 {
		l.MaxRenderedSourceBytes = d.MaxRenderedSourceBytes
	}
	if l.MaxBootstrapMigrationSQLBytes == 0 {
		l.MaxBootstrapMigrationSQLBytes = d.MaxBootstrapMigrationSQLBytes
	}
	if l.MaxBootstrapSnapshotJSONBytes == 0 {
		l.MaxBootstrapSnapshotJSONBytes = d.MaxBootstrapSnapshotJSONBytes
	}
	if l.MaxPlannedWriteBytes == 0 {
		l.MaxPlannedWriteBytes = d.MaxPlannedWriteBytes
	}
	if l.MaxSchemaFiles == 0 {
		l.MaxSchemaFiles = d.MaxSchemaFiles
	}
	if l.MaxSchemaSourceFileBytes == 0 {
		l.MaxSchemaSourceFileBytes = d.MaxSchemaSourceFileBytes
	}
	if l.MaxSchemaSourceBytes == 0 {
		l.MaxSchemaSourceBytes = d.MaxSchemaSourceBytes
	}
	if l.MaxSchemaASTNodes == 0 {
		l.MaxSchemaASTNodes = d.MaxSchemaASTNodes
	}
	if l.MaxSchemaASTDepth == 0 {
		l.MaxSchemaASTDepth = d.MaxSchemaASTDepth
	}
	if l.MaxSchemaDeclarations == 0 {
		l.MaxSchemaDeclarations = d.MaxSchemaDeclarations
	}
	if l.MaxSchemaLiteralBytes == 0 {
		l.MaxSchemaLiteralBytes = d.MaxSchemaLiteralBytes
	}
	if l.MaxSecretValueBytes == 0 {
		l.MaxSecretValueBytes = d.MaxSecretValueBytes
	}
	return l
}

// Validate returns an error if any field has a negative value.
// Callers should invoke this after resolving zero fields to defaults.
func (l ResourceLimits) Validate(op string) error {
	return l.validate(op)
}

// validate is the unexported implementation used internally.
func (l ResourceLimits) validate(op string) error {
	// Enumerates all public fields except MaxSecretValueBytes (intentionally
	// excluded — secret-value usage must not appear in public LimitStatus output).
	checks := []struct {
		name ResourceLimitName
		v    int64
	}{
		{LimitMaxTotalArtifactBytes, l.MaxTotalArtifactBytes},
		{LimitMaxMigrationSQLBytes, l.MaxMigrationSQLBytes},
		{LimitMaxSnapshotJSONBytes, l.MaxSnapshotJSONBytes},
		{LimitMaxIntrospectionSQLBytes, l.MaxIntrospectionSQLBytes},
		{LimitMaxIntrospectionPayloadBytes, l.MaxIntrospectionPayloadBytes},
		{LimitMaxRenderedSourceFileBytes, l.MaxRenderedSourceFileBytes},
		{LimitMaxRenderedSourceBytes, l.MaxRenderedSourceBytes},
		{LimitMaxBootstrapMigrationSQLBytes, l.MaxBootstrapMigrationSQLBytes},
		{LimitMaxBootstrapSnapshotJSONBytes, l.MaxBootstrapSnapshotJSONBytes},
		{LimitMaxPlannedWriteBytes, l.MaxPlannedWriteBytes},
		{LimitMaxSchemaSourceFileBytes, l.MaxSchemaSourceFileBytes},
		{LimitMaxSchemaSourceBytes, l.MaxSchemaSourceBytes},
		{LimitMaxSchemaLiteralBytes, l.MaxSchemaLiteralBytes},
	}
	intChecks := []struct {
		name ResourceLimitName
		v    int
	}{
		{LimitMaxArtifacts, l.MaxArtifacts},
		{LimitMaxArtifactDirEntries, l.MaxArtifactDirEntries},
		{LimitMaxSnapshotJSONDepth, l.MaxSnapshotJSONDepth},
		{LimitMaxSnapshotEntities, l.MaxSnapshotEntities},
		{LimitMaxTempCleanupEntries, l.MaxTempCleanupEntries},
		{LimitMaxIntrospectionObjects, l.MaxIntrospectionObjects},
		{LimitMaxObjectNameBytes, l.MaxObjectNameBytes},
		{LimitMaxSchemaFiles, l.MaxSchemaFiles},
		{LimitMaxSchemaASTNodes, l.MaxSchemaASTNodes},
		{LimitMaxSchemaASTDepth, l.MaxSchemaASTDepth},
		{LimitMaxSchemaDeclarations, l.MaxSchemaDeclarations},
	}
	for _, c := range checks {
		if c.v < 0 {
			return &Error{
				Code: CodeInvalidConfig,
				Op:   op,
				Err:  fmt.Errorf("resource limit %s must not be negative", c.name),
			}
		}
	}
	for _, c := range intChecks {
		if c.v < 0 {
			return &Error{
				Code: CodeInvalidConfig,
				Op:   op,
				Err:  fmt.Errorf("resource limit %s must not be negative", c.name),
			}
		}
	}
	return nil
}
