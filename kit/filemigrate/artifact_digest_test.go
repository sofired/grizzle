package filemigrate_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/sofired/grizzle/kit/filemigrate"
)

// Artifact digest specification and canonical vectors:
//
//   - docs/spec/file-migrations-api.md
//   - docs/spec/file-migrations-artifacts.md
//   - testdata/artifact_digest_vectors.json
//
// The formula is:
//
//	SHA256(
//	  "grizzle-artifact-v1" || 0x00 ||
//	  "migration.sql" || 0x00 || uint64be(len(migration.sql)) || raw migration.sql bytes ||
//	  "snapshot.json" || 0x00 || uint64be(len(snapshot.json)) || raw snapshot.json bytes
//	)
//
// The tests in this file pin that exact byte layout. Two independent layers of
// conformance are exercised:
//
//  1. Canonical golden hex vectors — shared with and independently checked by
//     Python's hashlib. These catch any change to the formula, including
//     endianness flips, separator swaps, payload reordering, or label edits.
//  2. Byte-assembly — re-derive the byte stream inside the test using stdlib
//     primitives that do not share code with the implementation, then hash it
//     and compare. This catches behavioral drift even if the goldens get
//     updated without independent recomputation.

// artifactDigestVectorFixture is the published, shared vector contract used
// by both these tests and the independent Python checker.
type artifactDigestVectorFixture struct {
	Format  string                 `json:"format"`
	Vectors []artifactDigestVector `json:"vectors"`
}

type artifactDigestVector struct {
	Name               string `json:"name"`
	MigrationSQLHex    string `json:"migration_sql_hex"`
	SnapshotJSONHex    string `json:"snapshot_json_hex"`
	MigrationSQLSHA256 string `json:"migration_sql_sha256"`
	SnapshotJSONSHA256 string `json:"snapshot_json_sha256"`
	CombinedSHA256     string `json:"combined_sha256"`
	migrationSQL       []byte
	snapshotJSON       []byte
}

func loadArtifactDigestVectors(t *testing.T) []artifactDigestVector {
	t.Helper()

	data, err := os.ReadFile("testdata/artifact_digest_vectors.json")
	if err != nil {
		t.Fatalf("read canonical artifact digest vectors: %v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var fixture artifactDigestVectorFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode canonical artifact digest vectors: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("canonical artifact digest vectors contain trailing JSON: %v", err)
	}
	if fixture.Format != "grizzle-artifact-digest-vectors-v1" {
		t.Fatalf("fixture format = %q, want %q", fixture.Format, "grizzle-artifact-digest-vectors-v1")
	}
	if len(fixture.Vectors) == 0 {
		t.Fatal("canonical artifact digest fixture has no vectors")
	}

	seenNames := make(map[string]struct{}, len(fixture.Vectors))
	for i := range fixture.Vectors {
		vector := &fixture.Vectors[i]
		if vector.Name == "" {
			t.Fatalf("vectors[%d].name must not be empty", i)
		}
		if _, duplicate := seenNames[vector.Name]; duplicate {
			t.Fatalf("duplicate vector name %q", vector.Name)
		}
		seenNames[vector.Name] = struct{}{}

		vector.migrationSQL = decodeArtifactVectorHex(t, vector.Name+".migration_sql_hex", vector.MigrationSQLHex)
		vector.snapshotJSON = decodeArtifactVectorHex(t, vector.Name+".snapshot_json_hex", vector.SnapshotJSONHex)
		validateArtifactVectorDigest(t, vector.Name+".migration_sql_sha256", vector.MigrationSQLSHA256)
		validateArtifactVectorDigest(t, vector.Name+".snapshot_json_sha256", vector.SnapshotJSONSHA256)
		validateArtifactVectorDigest(t, vector.Name+".combined_sha256", vector.CombinedSHA256)
	}
	return fixture.Vectors
}

func decodeArtifactVectorHex(t *testing.T, field, value string) []byte {
	t.Helper()
	if value != strings.ToLower(value) {
		t.Fatalf("%s must use lowercase hexadecimal", field)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("decode %s: %v", field, err)
	}
	return decoded
}

func validateArtifactVectorDigest(t *testing.T, field, value string) {
	t.Helper()
	decoded := decodeArtifactVectorHex(t, field, value)
	if len(decoded) != sha256.Size {
		t.Fatalf("%s decoded length = %d, want %d", field, len(decoded), sha256.Size)
	}
}

// TestArtifactDigest_CombinedGoldenVectors locks the exact CombinedSHA256
// output for several inputs against hex values computed independently with
// Python's hashlib. Treat any mismatch as a spec-breaking change to the
// digest formula and stop the build — published artifact digests must remain
// stable across releases.
func TestArtifactDigest_CombinedGoldenVectors(t *testing.T) {
	vectors := loadArtifactDigestVectors(t)
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)

	for i, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			// Use a unique directory name per vector since CreateArtifact is
			// the only public surface that exposes the populated digest.
			name := makeTestArtifactName(i)
			loaded := mustCreateArtifact(t, store, root, name, vector.migrationSQL, vector.snapshotJSON)
			gotHex := hex.EncodeToString(loaded.Digests.CombinedSHA256[:])
			if gotHex != vector.CombinedSHA256 {
				t.Errorf("CombinedSHA256 mismatch\n  got:  %s\n  want: %s", gotHex, vector.CombinedSHA256)
			}
		})
	}
}

// TestArtifactDigest_CombinedByteAssembly independently rebuilds the spec
// byte stream using stdlib primitives and confirms its SHA-256 equals the
// CombinedSHA256 returned by the store. This is a layered check on top of
// the golden hex vectors: if the goldens drift, byte-assembly still pins the
// algorithm; if the assembly drifts, the goldens still pin the digests.
func TestArtifactDigest_CombinedByteAssembly(t *testing.T) {
	vectors := loadArtifactDigestVectors(t)
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)

	for i, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			var buf bytes.Buffer
			buf.WriteString("grizzle-artifact-v1")
			buf.WriteByte(0x00)
			buf.WriteString("migration.sql")
			buf.WriteByte(0x00)
			var lenBuf [8]byte
			binary.BigEndian.PutUint64(lenBuf[:], uint64(len(vector.migrationSQL)))
			buf.Write(lenBuf[:])
			buf.Write(vector.migrationSQL)
			buf.WriteString("snapshot.json")
			buf.WriteByte(0x00)
			binary.BigEndian.PutUint64(lenBuf[:], uint64(len(vector.snapshotJSON)))
			buf.Write(lenBuf[:])
			buf.Write(vector.snapshotJSON)
			want := sha256.Sum256(buf.Bytes())

			name := makeTestArtifactName(i)
			loaded := mustCreateArtifact(t, store, root, name, vector.migrationSQL, vector.snapshotJSON)
			if loaded.Digests.CombinedSHA256 != filemigrate.Digest(want) {
				t.Errorf("CombinedSHA256 mismatch with byte-assembly\n  got:  %s\n  want: %s",
					hex.EncodeToString(loaded.Digests.CombinedSHA256[:]),
					hex.EncodeToString(want[:]))
			}
		})
	}
}

// TestArtifactDigest_PerFileGoldenVectors locks the exact per-file SHA-256
// values for known inputs. These tests pin per-file digests as plain SHA-256
// over raw bytes with no domain prefix or length framing. MigrationSQLSHA256 is
// what the DB history table records as `hash` for Drizzle RC.1 alignment, so
// any drift here would break history-hash compatibility.
func TestArtifactDigest_PerFileGoldenVectors(t *testing.T) {
	vectors := loadArtifactDigestVectors(t)
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)

	for i, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			name := makeTestArtifactName(i)
			loaded := mustCreateArtifact(t, store, root, name, vector.migrationSQL, vector.snapshotJSON)

			gotSQLHex := hex.EncodeToString(loaded.Digests.MigrationSQLSHA256[:])
			if gotSQLHex != vector.MigrationSQLSHA256 {
				t.Errorf("MigrationSQLSHA256 mismatch\n  got:  %s\n  want: %s", gotSQLHex, vector.MigrationSQLSHA256)
			}
			gotSnapshotHex := hex.EncodeToString(loaded.Digests.SnapshotJSONSHA256[:])
			if gotSnapshotHex != vector.SnapshotJSONSHA256 {
				t.Errorf("SnapshotJSONSHA256 mismatch\n  got:  %s\n  want: %s", gotSnapshotHex, vector.SnapshotJSONSHA256)
			}
		})
	}
}

// TestArtifactDigest_CombinedSwapDistinguishable verifies that the combined
// digest is sensitive to payload role and order. The two inputs are
// byte-distinct, so the swap is observable.
func TestArtifactDigest_CombinedSwapDistinguishable(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)

	a := mustCreateArtifact(t, store, root, "20240101000000_a",
		[]byte("payload-A"), []byte("payload-B"))
	b := mustCreateArtifact(t, store, root, "20240101000000_b",
		[]byte("payload-B"), []byte("payload-A"))

	if a.Digests.CombinedSHA256 == b.Digests.CombinedSHA256 {
		t.Errorf("swapping migration.sql and snapshot.json must change CombinedSHA256, got identical %s",
			hex.EncodeToString(a.Digests.CombinedSHA256[:]))
	}
}

// TestArtifactDigest_RawBytesAreNotNormalized proves that text-shaped artifacts
// are hashed as raw bytes. Each pair differs by exactly one normalization a
// caller might otherwise be tempted to apply.
func TestArtifactDigest_RawBytesAreNotNormalized(t *testing.T) {
	cases := []struct {
		name         string
		sqlA, sqlB   []byte
		snapA, snapB []byte
	}{
		{
			name:  "migration_sql_lf_vs_crlf",
			sqlA:  []byte("-- café\nSELECT '雪';\n"),
			sqlB:  []byte("-- café\r\nSELECT '雪';\r\n"),
			snapA: []byte(`{"note":"naïve"}`),
			snapB: []byte(`{"note":"naïve"}`),
		},
		{
			name:  "snapshot_json_lf_vs_crlf",
			sqlA:  []byte("SELECT 1;"),
			sqlB:  []byte("SELECT 1;"),
			snapA: []byte("{\n  \"note\": \"naïve\"\n}\n"),
			snapB: []byte("{\r\n  \"note\": \"naïve\"\r\n}\r\n"),
		},
		{
			name:  "migration_sql_comment_retained",
			sqlA:  []byte("-- café\nSELECT '雪';"),
			sqlB:  []byte("SELECT '雪';"),
			snapA: []byte(`{"note":"naïve"}`),
			snapB: []byte(`{"note":"naïve"}`),
		},
		{
			name:  "migration_sql_trailing_newline_retained",
			sqlA:  []byte("SELECT '雪';"),
			sqlB:  []byte("SELECT '雪';\n"),
			snapA: []byte(`{"note":"naïve"}`),
			snapB: []byte(`{"note":"naïve"}`),
		},
		{
			name:  "snapshot_json_trailing_newline_retained",
			sqlA:  []byte("SELECT 1;"),
			sqlB:  []byte("SELECT 1;"),
			snapA: []byte(`{"note":"naïve"}`),
			snapB: []byte("{\"note\":\"naïve\"}\n"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := filemigrate.NewMemArtifactStore()
			root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)
			a := mustCreateArtifact(t, store, root, "20240101000000_a", tc.sqlA, tc.snapA)
			b := mustCreateArtifact(t, store, root, "20240101000000_b", tc.sqlB, tc.snapB)

			if !bytes.Equal(tc.sqlA, tc.sqlB) && a.Digests.MigrationSQLSHA256 == b.Digests.MigrationSQLSHA256 {
				t.Error("raw migration.sql byte variants must have distinct per-file digests")
			}
			if !bytes.Equal(tc.snapA, tc.snapB) && a.Digests.SnapshotJSONSHA256 == b.Digests.SnapshotJSONSHA256 {
				t.Error("raw snapshot.json byte variants must have distinct per-file digests")
			}
			if a.Digests.CombinedSHA256 == b.Digests.CombinedSHA256 {
				t.Error("raw artifact byte variants must have distinct combined digests")
			}
		})
	}
}

// TestArtifactDigest_CombinedDeterministic verifies that the combined digest
// is a pure function of (migration.sql, snapshot.json): repeated creation
// with the same byte inputs (under different artifact names) yields the same
// CombinedSHA256. The directory name must not leak into the digest.
func TestArtifactDigest_CombinedDeterministic(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)

	sql := []byte("CREATE TABLE t (id INT);")
	snap := []byte(`{"version":"1"}`)

	a := mustCreateArtifact(t, store, root, "20240101000000_first", sql, snap)
	b := mustCreateArtifact(t, store, root, "20250202020202_second", sql, snap)
	if a.Digests.CombinedSHA256 != b.Digests.CombinedSHA256 {
		t.Errorf("CombinedSHA256 must be deterministic for identical bytes, got %s vs %s",
			hex.EncodeToString(a.Digests.CombinedSHA256[:]),
			hex.EncodeToString(b.Digests.CombinedSHA256[:]))
	}
}

// TestArtifactDigest_RoundtripsThroughRead pins digest consistency for
// unchanged bytes across MemArtifactStore CreateArtifact and ReadArtifact.
func TestArtifactDigest_RoundtripsThroughRead(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)

	sql := []byte("CREATE TABLE t (id INT);\n")
	snap := []byte(`{"version":"1","dialect":"postgresql"}`)
	created := mustCreateArtifact(t, store, root, "20240101000000_x", sql, snap)

	got, err := store.ReadArtifact(t.Context(), root, "20240101000000_x", filemigrate.ReadArtifactOptions{})
	if err != nil {
		t.Fatalf("ReadArtifact: %v", err)
	}
	if got.Digests != created.Digests {
		t.Errorf("digest mismatch across create/read roundtrip\n  create: combined=%s sql=%s snap=%s\n  read:   combined=%s sql=%s snap=%s",
			hex.EncodeToString(created.Digests.CombinedSHA256[:]),
			hex.EncodeToString(created.Digests.MigrationSQLSHA256[:]),
			hex.EncodeToString(created.Digests.SnapshotJSONSHA256[:]),
			hex.EncodeToString(got.Digests.CombinedSHA256[:]),
			hex.EncodeToString(got.Digests.MigrationSQLSHA256[:]),
			hex.EncodeToString(got.Digests.SnapshotJSONSHA256[:]))
	}
}

// makeTestArtifactName builds a unique, validation-passing migration directory
// name for the i-th vector within a test-local store.
func makeTestArtifactName(i int) string {
	return fmt.Sprintf("20240101000000_v%04d", i)
}
