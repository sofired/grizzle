package filemigrate

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMigrationSQLRejectsInvalidText(t *testing.T) {
	secret := "github_pat_must_not_appear"
	prefix := "SELECT '" + secret + "';"
	tests := []struct {
		name      string
		sql       []byte
		wantCause string
	}{
		{
			name:      "invalid_utf8",
			sql:       append([]byte(prefix+" -- "), 0xff),
			wantCause: "migration.sql is not valid UTF-8",
		},
		{
			name:      "nul",
			sql:       []byte(prefix + "\x00"),
			wantCause: fmt.Sprintf("migration.sql contains NUL at byte offset %d", len(prefix)),
		},
		{
			name:      "c0_control",
			sql:       []byte(prefix + "\x01"),
			wantCause: fmt.Sprintf("migration.sql contains an unsupported control character at byte offset %d", len(prefix)),
		},
		{
			name:      "delete_control",
			sql:       []byte(prefix + "\x7f"),
			wantCause: fmt.Sprintf("migration.sql contains an unsupported control character at byte offset %d", len(prefix)),
		},
		{
			name:      "c1_control",
			sql:       []byte(prefix + "\u0085"),
			wantCause: fmt.Sprintf("migration.sql contains an unsupported control character at byte offset %d", len(prefix)),
		},
		{
			name:      "non_leading_bom",
			sql:       []byte(prefix + "\ufeff"),
			wantCause: fmt.Sprintf("migration.sql contains a non-leading UTF-8 BOM at byte offset %d", len(prefix)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			migration := "20260719000000_invalid"
			err := validateMigrationSQL(migration, tc.sql)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !errors.Is(err, ErrInvalidStatementSegmentation) {
				t.Fatalf("errors.Is(err, ErrInvalidStatementSegmentation) = false: %v", err)
			}

			var ferr *Error
			if !errors.As(err, &ferr) {
				t.Fatalf("errors.As(err, *Error) = false: %T %v", err, err)
			}
			if ferr.Code != CodeInvalidStatementSegmentation {
				t.Errorf("Code = %q, want %q", ferr.Code, CodeInvalidStatementSegmentation)
			}
			if ferr.Op != validateMigrationSQLOp {
				t.Errorf("Op = %q, want %q", ferr.Op, validateMigrationSQLOp)
			}
			if ferr.Migration != migration {
				t.Errorf("Migration = %q, want %q", ferr.Migration, migration)
			}
			if ferr.Err == nil || ferr.Err.Error() != tc.wantCause {
				t.Errorf("safe cause = %q, want %q", ferr.Err, tc.wantCause)
			}
			if !strings.Contains(err.Error(), tc.wantCause) {
				t.Errorf("error %q does not contain complete safe cause %q", err, tc.wantCause)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error leaked SQL content: %q", err)
			}
		})
	}
}

func TestValidateMigrationSQLAcceptsSupportedText(t *testing.T) {
	tests := []struct {
		name string
		sql  []byte
	}{
		{name: "empty"},
		{name: "ascii", sql: []byte("SELECT 1;")},
		{name: "tab_lf_cr", sql: []byte("\tSELECT 1;\nSELECT 2;\rSELECT 3;\r\n")},
		{name: "leading_bom", sql: []byte("\ufeffSELECT 1;")},
		{name: "unicode", sql: []byte("-- café 雪\nSELECT 'naïve';\n")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateMigrationSQL("20260719000000_valid", tc.sql); err != nil {
				t.Fatalf("validateMigrationSQL: %v", err)
			}
		})
	}
}

func TestFindMigrationSQLDelimiterCandidatesRecognizesLineEndingsAndPreservesOriginalByteSpans(t *testing.T) {
	tests := []struct {
		name string
		eol  string
	}{
		{name: "lf", eol: "\n"},
		{name: "crlf", eol: "\r\n"},
		{name: "cr", eol: "\r"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sql := []byte("SELECT 1;" + tc.eol + " \t" + statementBreakpointLine + "\t " + tc.eol + "SELECT 2;" + tc.eol)
			before := bytes.Clone(sql)
			beforeDigest := sha256.Sum256(sql)

			candidates := requireValidMigrationSQLAndFindCandidates(t, "20260719000000_lines", sql)
			if len(candidates) != 1 {
				t.Fatalf("candidate count = %d, want 1", len(candidates))
			}

			d := candidates[0]
			wantDelimiter := []byte(" \t" + statementBreakpointLine + "\t " + tc.eol)
			if got := sql[d.start:d.end]; !bytes.Equal(got, wantDelimiter) {
				t.Errorf("delimiter bytes = %q, want %q", got, wantDelimiter)
			}
			segments := rawSegmentsFromCandidates(sql, candidates)
			wantSegments := [][]byte{
				[]byte("SELECT 1;" + tc.eol),
				[]byte("SELECT 2;" + tc.eol),
			}
			if !equalByteSlices(segments, wantSegments) {
				t.Errorf("segments = %q, want %q", segments, wantSegments)
			}
			if !bytes.Equal(sql, before) {
				t.Errorf("input bytes changed: got %q, want %q", sql, before)
			}
			if got := sha256.Sum256(sql); got != beforeDigest {
				t.Errorf("input digest changed: got %x, want %x", got, beforeDigest)
			}
		})
	}
}

func TestFindMigrationSQLDelimiterCandidatesMixedLineEndingsAndTrailingNewline(t *testing.T) {
	sql := []byte(
		"SELECT 'first';\r\n" +
			statementBreakpointLine + "\r" +
			"SELECT 'second';\n" +
			statementBreakpointLine + "\r\n" +
			"SELECT 'third';\r",
	)

	candidates := requireValidMigrationSQLAndFindCandidates(t, "20260719000000_mixed", sql)
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}

	want := [][]byte{
		[]byte("SELECT 'first';\r\n"),
		[]byte("SELECT 'second';\n"),
		[]byte("SELECT 'third';\r"),
	}
	if got := rawSegmentsFromCandidates(sql, candidates); !equalByteSlices(got, want) {
		t.Errorf("segments = %q, want %q", got, want)
	}
}

func TestFindMigrationSQLDelimiterCandidatesOnlyMatchesFullTrimmedPhysicalLines(t *testing.T) {
	sql := []byte(strings.Join([]string{
		"SELECT '--> statement-breakpoint';",
		"-- --> statement-breakpoint",
		"prefix --> statement-breakpoint",
		"--> statement-breakpoint suffix",
		statementBreakpointLine,
	}, "\n"))

	candidates := requireValidMigrationSQLAndFindCandidates(t, "20260719000000_full_line", sql)
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}
	if got := sql[candidates[0].start:candidates[0].end]; !bytes.Equal(got, []byte(statementBreakpointLine)) {
		t.Errorf("delimiter bytes = %q, want %q", got, statementBreakpointLine)
	}
}

func TestFindMigrationSQLDelimiterCandidatesLeavesInactiveTextFilteringToExecutor(t *testing.T) {
	sql := []byte(strings.Join([]string{
		"/*",
		statementBreakpointLine,
		"*/",
		"SELECT 1;",
		statementBreakpointLine,
		"SELECT 2;",
	}, "\n"))

	candidates := requireValidMigrationSQLAndFindCandidates(t, "20260719000000_candidates", sql)
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2", len(candidates))
	}
	for i, candidate := range candidates {
		if got := bytes.TrimSpace(sql[candidate.start:candidate.end]); !bytes.Equal(got, []byte(statementBreakpointLine)) {
			t.Errorf("candidate[%d] = %q, want %q", i, got, statementBreakpointLine)
		}
	}
}

func TestArtifactStoresReadRejectInvalidMigrationSQL(t *testing.T) {
	migration := "20260719000000_invalid"
	secret := "github_pat_must_not_appear"
	sql := []byte("SELECT '" + secret + "';\x00")
	snapshot := []byte(`{}`)

	for _, fixture := range seededArtifactReadFixtures(t, migration, sql, snapshot) {
		t.Run(fixture.name, func(t *testing.T) {
			loaded, err := fixture.store.ReadArtifact(
				t.Context(),
				fixture.root,
				migration,
				ReadArtifactOptions{},
			)
			if err == nil {
				t.Fatal("ReadArtifact succeeded for invalid migration.sql")
			}
			if loaded != nil {
				t.Errorf("ReadArtifact returned an artifact on validation failure: %#v", loaded)
			}
			if !errors.Is(err, ErrInvalidStatementSegmentation) {
				t.Errorf("errors.Is(err, ErrInvalidStatementSegmentation) = false: %v", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error leaked SQL content: %q", err)
			}
		})
	}
}

func TestArtifactStoresReadApplyByteLimitsBeforeSQLValidation(t *testing.T) {
	migration := "20260719000000_multi_invalid"
	sql := []byte("SELECT 1;\x00")
	snapshot := []byte(`{}`)

	for _, fixture := range seededArtifactReadFixtures(t, migration, sql, snapshot) {
		t.Run(fixture.name, func(t *testing.T) {
			loaded, err := fixture.store.ReadArtifact(
				t.Context(),
				fixture.root,
				migration,
				ReadArtifactOptions{Limits: ResourceLimits{MaxSnapshotJSONBytes: 1}},
			)
			if err == nil {
				t.Fatal("ReadArtifact succeeded for an oversized snapshot")
			}
			if loaded != nil {
				t.Errorf("ReadArtifact returned an artifact on limit failure: %#v", loaded)
			}
			if !errors.Is(err, ErrResourceLimit) {
				t.Errorf("errors.Is(err, ErrResourceLimit) = false: %v", err)
			}
			if errors.Is(err, ErrInvalidStatementSegmentation) {
				t.Errorf("ReadArtifact validated SQL before enforcing both file limits: %v", err)
			}
		})
	}
}

func TestArtifactStoresReadPreserveValidatedMigrationSQLBytesAndDigest(t *testing.T) {
	migration := "20260719000000_mixed"
	sql := []byte(
		"SELECT 'first';\r\n" +
			statementBreakpointLine + "\r" +
			"SELECT 'second';\n" +
			statementBreakpointLine + "\r\n" +
			"SELECT 'third';\r",
	)
	snapshot := []byte(`{}`)
	wantDigest := Digest(sha256.Sum256(sql))

	for _, fixture := range seededArtifactReadFixtures(t, migration, sql, snapshot) {
		t.Run(fixture.name, func(t *testing.T) {
			loaded, err := fixture.store.ReadArtifact(
				t.Context(),
				fixture.root,
				migration,
				ReadArtifactOptions{},
			)
			if err != nil {
				t.Fatalf("ReadArtifact: %v", err)
			}
			if !bytes.Equal(loaded.MigrationSQL, sql) {
				t.Errorf("MigrationSQL = %q, want exact bytes %q", loaded.MigrationSQL, sql)
			}
			if loaded.Digests.MigrationSQLSHA256 != wantDigest {
				t.Errorf("MigrationSQLSHA256 = %x, want %x", loaded.Digests.MigrationSQLSHA256, wantDigest)
			}
		})
	}
}

type artifactReadFixture struct {
	name  string
	store ArtifactStore
	root  ArtifactRoot
}

func seededArtifactReadFixtures(t *testing.T, migration string, sql, snapshot []byte) []artifactReadFixture {
	t.Helper()

	memRoot := ArtifactRoot{Configured: "/migrations", RealPath: "/migrations", State: RootExisting}
	memStore := NewMemArtifactStore()
	memStore.artifacts[memRoot.RealPath+"/"+migration] = memArtifact{
		root:         memRoot.RealPath,
		name:         migration,
		migrationSQL: bytes.Clone(sql),
		snapshotJSON: bytes.Clone(snapshot),
	}

	fsRootPath := t.TempDir()
	artifactPath := filepath.Join(fsRootPath, migration)
	if err := os.Mkdir(artifactPath, 0o700); err != nil {
		t.Fatalf("Mkdir artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactPath, "migration.sql"), sql, 0o600); err != nil {
		t.Fatalf("WriteFile migration.sql: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactPath, "snapshot.json"), snapshot, 0o600); err != nil {
		t.Fatalf("WriteFile snapshot.json: %v", err)
	}
	fsStore := NewFSArtifactStore()
	fsRoot, err := fsStore.ResolveRoot(t.Context(), fsRootPath, ResolveArtifactRootOptions{Mode: RootReadForCheck})
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}

	return []artifactReadFixture{
		{name: "memory", store: memStore, root: memRoot},
		{name: "filesystem", store: fsStore, root: fsRoot},
	}
}

func requireValidMigrationSQLAndFindCandidates(t *testing.T, migration string, sql []byte) []migrationSQLDelimiterCandidate {
	t.Helper()
	if err := validateMigrationSQL(migration, sql); err != nil {
		t.Fatalf("validateMigrationSQL: %v", err)
	}
	return findMigrationSQLDelimiterCandidates(sql)
}

func rawSegmentsFromCandidates(sql []byte, candidates []migrationSQLDelimiterCandidate) [][]byte {
	segments := make([][]byte, 0, len(candidates)+1)
	start := 0
	for _, candidate := range candidates {
		segments = append(segments, sql[start:candidate.start])
		start = candidate.end
	}
	return append(segments, sql[start:])
}

func equalByteSlices(got, want [][]byte) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !bytes.Equal(got[i], want[i]) {
			return false
		}
	}
	return true
}
