package filemigrate_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/sofired/grizzle/kit/filemigrate"
)

// Spec references for ArtifactDigest.CombinedSHA256:
//
//   - docs/spec/file-migrations-api.md:363-373
//   - docs/spec/file-migrations-artifacts.md:528-536
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
//  1. Golden hex vectors — pre-computed offline with a different language/runtime
//     (Python's hashlib). These catch any change to the formula, including
//     endianness flips, separator swaps, payload reordering, or label edits.
//  2. Byte-assembly — re-derive the byte stream inside the test using stdlib
//     primitives that do not share code with the implementation, then hash it
//     and compare. This catches behavioral drift even if the goldens get
//     updated without independent recomputation.

// combinedGoldenVector is one (sql, snap, expected combined hex) triple
// pre-computed independently of the production implementation.
type combinedGoldenVector struct {
	name    string
	sql     []byte
	snap    []byte
	wantHex string
}

// goldenCombinedVectors are independently computed via Python:
//
//	h = hashlib.sha256()
//	h.update(b'grizzle-artifact-v1'); h.update(b'\x00')
//	h.update(b'migration.sql');       h.update(b'\x00')
//	h.update(struct.pack('>Q', len(sql))); h.update(sql)
//	h.update(b'snapshot.json');       h.update(b'\x00')
//	h.update(struct.pack('>Q', len(snap))); h.update(snap)
//	h.hexdigest()
//
// Any change to the spec byte layout (separator byte, domain string, label
// strings, label order, endianness, length-prefix width, or omission of any
// component) will cause every vector below to flip, failing this test loudly.
var goldenCombinedVectors = []combinedGoldenVector{
	{
		name:    "both_empty",
		sql:     []byte(""),
		snap:    []byte(""),
		wantHex: "a3c7cae6b5098053bcd8fec18a37d4e257278db8c4fd851dcc9d0c86cc707852",
	},
	{
		name:    "small_ascii",
		sql:     []byte("CREATE TABLE t (id INT);"),
		snap:    []byte(`{"version":"1"}`),
		wantHex: "0cafd83a585887b16d54263041c29c3309bd35a85068ad21a420d988a40ac95d",
	},
	{
		name:    "empty_sql_with_snap",
		sql:     []byte(""),
		snap:    []byte("{}"),
		wantHex: "ebd7e42d73531a9252aac54d14e4961e2ae205a0c24e73585eba36a4334fcf02",
	},
	{
		name:    "empty_snap_with_sql",
		sql:     []byte("SELECT 1;"),
		snap:    []byte(""),
		wantHex: "aadbc247869c70cfb56f3d7c942340cb409fe113714eac88035c7c8d66f459e6",
	},
	{
		// 256-byte sequence covering every byte value, paired with a snapshot
		// containing 0x00 and 0xFF runs. Any single-byte misencoding in the
		// content path (e.g. mis-stripping 0x00 inside payload, treating the
		// payload as a C string) would flip this digest.
		name:    "full_byte_range_with_zeros_and_high_bytes",
		sql:     fullByteRange(),
		snap:    append(bytes.Repeat([]byte{0xff}, 100), bytes.Repeat([]byte{0x00}, 50)...),
		wantHex: "65c1350c4e33e8986cda857b9b0bfab3643ca88694873b7de47558663dbc4d92",
	},
	{
		// 1 KiB of zero bytes for both payloads — exercises the length-prefix
		// path with a payload that is otherwise indistinguishable from absence
		// without the length header.
		name:    "zeros_1024_each",
		sql:     bytes.Repeat([]byte{0x00}, 1024),
		snap:    bytes.Repeat([]byte{0x00}, 1024),
		wantHex: "aa9060b3befac5d93f98d046517e93bebcf336c1471ae8851ff37f6042f2947b",
	},
}

// fullByteRange returns the 256-byte slice 0x00, 0x01, ..., 0xFF.
func fullByteRange() []byte {
	out := make([]byte, 256)
	for i := range out {
		out[i] = byte(i)
	}
	return out
}

// TestArtifactDigest_CombinedGoldenVectors locks the exact CombinedSHA256
// output for several inputs against hex values computed independently with
// Python's hashlib. Treat any mismatch as a spec-breaking change to the
// digest formula and stop the build — published artifact digests must remain
// stable across releases.
func TestArtifactDigest_CombinedGoldenVectors(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)

	for i, v := range goldenCombinedVectors {
		t.Run(v.name, func(t *testing.T) {
			// Use a unique directory name per vector since CreateArtifact is
			// the only public surface that exposes the populated digest.
			name := makeTestArtifactName(i)
			loaded := mustCreateArtifact(t, store, root, name, v.sql, v.snap)
			gotHex := hex.EncodeToString(loaded.Digests.CombinedSHA256[:])
			if gotHex != v.wantHex {
				t.Errorf("CombinedSHA256 mismatch\n  got:  %s\n  want: %s", gotHex, v.wantHex)
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
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)

	for i, v := range goldenCombinedVectors {
		t.Run(v.name, func(t *testing.T) {
			var buf bytes.Buffer
			buf.WriteString("grizzle-artifact-v1")
			buf.WriteByte(0x00)
			buf.WriteString("migration.sql")
			buf.WriteByte(0x00)
			var lenBuf [8]byte
			binary.BigEndian.PutUint64(lenBuf[:], uint64(len(v.sql)))
			buf.Write(lenBuf[:])
			buf.Write(v.sql)
			buf.WriteString("snapshot.json")
			buf.WriteByte(0x00)
			binary.BigEndian.PutUint64(lenBuf[:], uint64(len(v.snap)))
			buf.Write(lenBuf[:])
			buf.Write(v.snap)
			want := sha256.Sum256(buf.Bytes())

			name := makeTestArtifactName(i + len(goldenCombinedVectors))
			loaded := mustCreateArtifact(t, store, root, name, v.sql, v.snap)
			if loaded.Digests.CombinedSHA256 != filemigrate.Digest(want) {
				t.Errorf("CombinedSHA256 mismatch with byte-assembly\n  got:  %s\n  want: %s",
					hex.EncodeToString(loaded.Digests.CombinedSHA256[:]),
					hex.EncodeToString(want[:]))
			}
		})
	}
}

// TestArtifactDigest_PerFileGoldenVectors locks the exact per-file SHA-256
// values for known inputs. Per-file digests are plain SHA-256 over raw bytes
// (no domain prefix, no length framing) per
// docs/spec/file-migrations-artifacts.md:528-536. The MigrationSQLSHA256 is
// what the DB history table records as `hash` for Drizzle RC.1 alignment, so
// any drift here would corrupt cross-version history reads.
func TestArtifactDigest_PerFileGoldenVectors(t *testing.T) {
	cases := []struct {
		name        string
		sql         []byte
		snap        []byte
		wantSQLHex  string
		wantSnapHex string
	}{
		{
			name:        "small_ascii",
			sql:         []byte("CREATE TABLE t (id INT);"),
			snap:        []byte(`{"version":"1"}`),
			wantSQLHex:  "f423e61fbd021a13b6ae0afb423f2da5c3cf7cc0647ddb7348266dbfd281d6fe",
			wantSnapHex: "aa5bc61f44d5f633935d04cbccf2654c56806fc924b0083a6cb6b7545369ad64",
		},
		{
			name:        "select1_with_empty_obj",
			sql:         []byte("SELECT 1;"),
			snap:        []byte("{}"),
			wantSQLHex:  "17db4fd369edb9244b9f91d9aeed145c3d04ad8ba6e95d06247f07a63527d11a",
			wantSnapHex: "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
		},
		{
			name:        "empty_inputs",
			sql:         []byte(""),
			snap:        []byte(""),
			wantSQLHex:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantSnapHex: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:        "full_byte_range_sql",
			sql:         fullByteRange(),
			snap:        []byte(`{"v":"1"}`),
			wantSQLHex:  "40aff2e9d2d8922e47afd4648e6967497158785fbd1da870e7110266bf944880",
			wantSnapHex: "", // computed below; cross-check via raw sha256 since snap is short
		},
	}
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := makeTestArtifactName(i + 2*len(goldenCombinedVectors))
			loaded := mustCreateArtifact(t, store, root, name, tc.sql, tc.snap)

			gotSQLHex := hex.EncodeToString(loaded.Digests.MigrationSQLSHA256[:])
			if gotSQLHex != tc.wantSQLHex {
				t.Errorf("MigrationSQLSHA256 mismatch\n  got:  %s\n  want: %s", gotSQLHex, tc.wantSQLHex)
			}

			gotSnapHex := hex.EncodeToString(loaded.Digests.SnapshotJSONSHA256[:])
			if tc.wantSnapHex != "" {
				if gotSnapHex != tc.wantSnapHex {
					t.Errorf("SnapshotJSONSHA256 mismatch\n  got:  %s\n  want: %s", gotSnapHex, tc.wantSnapHex)
				}
			} else {
				// Fall back to recomputing with stdlib for cases where a hand-
				// pinned snapshot hex would be redundant.
				want := sha256.Sum256(tc.snap)
				if loaded.Digests.SnapshotJSONSHA256 != filemigrate.Digest(want) {
					t.Errorf("SnapshotJSONSHA256 mismatch\n  got:  %s\n  want: %s",
						gotSnapHex, hex.EncodeToString(want[:]))
				}
			}
		})
	}
}

// TestArtifactDigest_CombinedSwapDistinguishable verifies that swapping the
// two payloads produces a different combined digest. This guards against an
// implementation that drops the labels or treats the two sections as
// interchangeable. The two inputs are byte-distinct, so the swap is observable.
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

// TestArtifactDigest_CombinedLengthFramingDistinguishable verifies that the
// uint64be length prefix actually distinguishes inputs that would otherwise
// collide under naive concatenation. The pair below has the same total payload
// bytes but different splits between migration.sql and snapshot.json, and must
// produce different combined digests.
func TestArtifactDigest_CombinedLengthFramingDistinguishable(t *testing.T) {
	store := filemigrate.NewMemArtifactStore()
	root := mustResolveRoot(t, store, "/m", filemigrate.RootEnsureForWrite)

	a := mustCreateArtifact(t, store, root, "20240101000000_a",
		[]byte("AAA"), []byte("B"))
	b := mustCreateArtifact(t, store, root, "20240101000000_b",
		[]byte("AA"), []byte("AB"))

	if a.Digests.CombinedSHA256 == b.Digests.CombinedSHA256 {
		t.Errorf("length framing must distinguish (sql=%q,snap=%q) from (sql=%q,snap=%q), got identical %s",
			"AAA", "B", "AA", "AB",
			hex.EncodeToString(a.Digests.CombinedSHA256[:]))
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

// TestArtifactDigest_RoundtripsThroughRead pins that ReadArtifact reproduces
// the same digests that CreateArtifact emitted. This rules out a class of bug
// where digests are computed only on the write path (or only on the read
// path), or where the read path silently rehashes from a normalized buffer.
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

// makeTestArtifactName builds a deterministic, validation-passing migration
// directory name for the i-th vector. The names are unique per call so that
// CreateArtifact does not reject duplicates inside a single store instance.
func makeTestArtifactName(i int) string {
	const digits = "0123456789"
	// Build "20240101000000_v<NN>" with i zero-padded to width 4. We avoid
	// fmt.Sprintf to keep this helper trivial and dependency-free.
	pad := []byte{digits[(i/1000)%10], digits[(i/100)%10], digits[(i/10)%10], digits[i%10]}
	return "20240101000000_v" + string(pad)
}
