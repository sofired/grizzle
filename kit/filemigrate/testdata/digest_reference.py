#!/usr/bin/env python3
"""Reference implementation of the CombinedSHA256 artifact digest formula.

Spec: docs/spec/file-migrations-api.md:365-371

    SHA256(
      "grizzle-artifact-v1" || 0x00 ||
      "migration.sql" || 0x00 || uint64be(len(migration.sql)) || raw migration.sql bytes ||
      "snapshot.json" || 0x00 || uint64be(len(snapshot.json)) || raw snapshot.json bytes
    )

This script exists so the golden hex values in artifact_digest_test.go can be
regenerated independently of the Go production code. Run it after any
deliberate, spec-amending change to the digest formula to recompute the
goldens; do NOT regenerate goldens from the Go implementation — that defeats
the cross-implementation pinning.

Usage:
    python3 kit/filemigrate/testdata/digest_reference.py

Add a new vector by appending an entry to VECTORS below.
"""

import hashlib
import struct


def combined(sql: bytes, snap: bytes) -> str:
    """Return the hex CombinedSHA256 per the spec formula."""
    h = hashlib.sha256()
    h.update(b"grizzle-artifact-v1")
    h.update(b"\x00")
    h.update(b"migration.sql")
    h.update(b"\x00")
    h.update(struct.pack(">Q", len(sql)))
    h.update(sql)
    h.update(b"snapshot.json")
    h.update(b"\x00")
    h.update(struct.pack(">Q", len(snap)))
    h.update(snap)
    return h.hexdigest()


def per_file(content: bytes) -> str:
    """Return the hex SHA-256 of a single file's raw bytes."""
    return hashlib.sha256(content).hexdigest()


# Test vectors pinned by artifact_digest_test.go.
# Adding a new vector here? Also add the matching Go entry.
VECTORS = [
    ("both_empty", b"", b""),
    ("small_ascii", b"CREATE TABLE t (id INT);", b'{"version":"1"}'),
    ("empty_sql_with_snap", b"", b"{}"),
    ("empty_snap_with_sql", b"SELECT 1;", b""),
    (
        "full_byte_range_with_zeros_and_high_bytes",
        bytes(range(256)),
        (b"\xff" * 100) + (b"\x00" * 50),
    ),
    ("zeros_1024_each", b"\x00" * 1024, b"\x00" * 1024),
    # Per-file vectors (different snap shapes).
    ("per_file_small_ascii", b"CREATE TABLE t (id INT);", b'{"version":"1"}'),
    ("per_file_select1_with_empty_obj", b"SELECT 1;", b"{}"),
    ("per_file_empty_inputs", b"", b""),
    ("per_file_full_byte_range_sql", bytes(range(256)), b'{"v":"1"}'),
    # Swap/framing vectors.
    ("swap_a_sql_AAA_snap_BBB", b"AAA", b"BBB"),
    ("swap_b_sql_BBB_snap_AAA", b"BBB", b"AAA"),
]


def main() -> None:
    width = max(len(name) for name, _, _ in VECTORS) + 2
    print(f"{'vector':<{width}} {'sql_sha256':<64}  {'snap_sha256':<64}  combined_sha256")
    print("-" * (width + 1 + 64 + 2 + 64 + 2 + 64))
    for name, sql, snap in VECTORS:
        print(
            f"{name:<{width}} "
            f"{per_file(sql)}  "
            f"{per_file(snap)}  "
            f"{combined(sql, snap)}"
        )


if __name__ == "__main__":
    main()
