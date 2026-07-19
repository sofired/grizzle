#!/usr/bin/env python3
"""Independently check or regenerate the canonical artifact digest vectors.

The normative formulas are documented in docs/spec/file-migrations-api.md.
This utility reads all vector inputs from artifact_digest_vectors.json; it does
not maintain a second vector inventory.

Usage:
    python3 kit/filemigrate/testdata/digest_reference.py --check
    python3 kit/filemigrate/testdata/digest_reference.py --write
"""

import argparse
import hashlib
import json
from pathlib import Path
import re
import struct
import sys
from typing import Any


FIXTURE_FORMAT = "grizzle-artifact-digest-vectors-v1"
FIXTURE_PATH = Path(__file__).with_name("artifact_digest_vectors.json")
INPUT_FIELDS = {"name", "migration_sql_hex", "snapshot_json_hex"}
DIGEST_FIELDS = {
    "migration_sql_sha256",
    "snapshot_json_sha256",
    "combined_sha256",
}
LOWER_HEX_RE = re.compile(r"[0-9a-f]*\Z")
LOWER_SHA256_RE = re.compile(r"[0-9a-f]{64}\Z")
VECTOR_NAME_RE = re.compile(r"[a-z0-9][a-z0-9_]*\Z")


class FixtureError(ValueError):
    """Raised when the canonical vector fixture is invalid."""


def combined(sql: bytes, snapshot: bytes) -> str:
    """Return lowercase hexadecimal CombinedSHA256 per the specification."""
    digest = hashlib.sha256()
    digest.update(b"grizzle-artifact-v1")
    digest.update(b"\x00")
    digest.update(b"migration.sql")
    digest.update(b"\x00")
    digest.update(struct.pack(">Q", len(sql)))
    digest.update(sql)
    digest.update(b"snapshot.json")
    digest.update(b"\x00")
    digest.update(struct.pack(">Q", len(snapshot)))
    digest.update(snapshot)
    return digest.hexdigest()


def per_file(content: bytes) -> str:
    """Return lowercase hexadecimal SHA-256 of one file's exact raw bytes."""
    return hashlib.sha256(content).hexdigest()


def reject_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    """Reject duplicate JSON object keys instead of silently keeping one."""
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise FixtureError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def load_fixture(path: Path) -> dict[str, Any]:
    """Load the fixture as UTF-8 JSON with duplicate-key detection."""
    try:
        text = path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as error:
        raise FixtureError(f"cannot read {path}: {error}") from error

    try:
        data = json.loads(text, object_pairs_hook=reject_duplicate_keys)
    except (json.JSONDecodeError, FixtureError) as error:
        raise FixtureError(f"cannot parse {path}: {error}") from error

    if not isinstance(data, dict):
        raise FixtureError("fixture root must be a JSON object")
    return data


def require_keys(
    value: dict[str, Any],
    required: set[str],
    allowed: set[str],
    location: str,
) -> None:
    """Require all named keys and reject unknown keys."""
    missing = sorted(required - value.keys())
    if missing:
        raise FixtureError(f"{location} missing fields: {', '.join(missing)}")
    unknown = sorted(value.keys() - allowed)
    if unknown:
        raise FixtureError(f"{location} has unknown fields: {', '.join(unknown)}")


def decode_input_hex(value: Any, location: str) -> bytes:
    """Decode canonical lowercase, whitespace-free, even-length hexadecimal."""
    if not isinstance(value, str):
        raise FixtureError(f"{location} must be a string")
    if len(value) % 2 != 0 or LOWER_HEX_RE.fullmatch(value) is None:
        raise FixtureError(f"{location} must be lowercase, even-length hexadecimal")
    return bytes.fromhex(value)


def validate_digest(value: Any, location: str) -> str:
    """Validate a lowercase hexadecimal SHA-256 value."""
    if not isinstance(value, str) or LOWER_SHA256_RE.fullmatch(value) is None:
        raise FixtureError(f"{location} must be 64 lowercase hexadecimal characters")
    return value


def parse_vectors(
    data: dict[str, Any],
    *,
    require_expected: bool,
) -> list[tuple[dict[str, Any], bytes, bytes]]:
    """Validate fixture structure and decode every vector input."""
    require_keys(data, {"format", "vectors"}, {"format", "vectors"}, "fixture")
    if data["format"] != FIXTURE_FORMAT:
        raise FixtureError(f"fixture format must be {FIXTURE_FORMAT!r}")
    if not isinstance(data["vectors"], list) or not data["vectors"]:
        raise FixtureError("fixture vectors must be a non-empty array")

    required = INPUT_FIELDS | (DIGEST_FIELDS if require_expected else set())
    allowed = INPUT_FIELDS | DIGEST_FIELDS
    parsed: list[tuple[dict[str, Any], bytes, bytes]] = []
    names: set[str] = set()
    for index, vector in enumerate(data["vectors"]):
        location = f"vectors[{index}]"
        if not isinstance(vector, dict):
            raise FixtureError(f"{location} must be a JSON object")
        require_keys(vector, required, allowed, location)

        name = vector["name"]
        if not isinstance(name, str) or VECTOR_NAME_RE.fullmatch(name) is None:
            raise FixtureError(f"{location}.name must be a stable lowercase identifier")
        if name in names:
            raise FixtureError(f"duplicate vector name: {name}")
        names.add(name)

        sql = decode_input_hex(vector["migration_sql_hex"], f"{location}.migration_sql_hex")
        snapshot = decode_input_hex(vector["snapshot_json_hex"], f"{location}.snapshot_json_hex")
        if require_expected:
            for field in sorted(DIGEST_FIELDS):
                validate_digest(vector[field], f"{location}.{field}")
        parsed.append((vector, sql, snapshot))
    return parsed


def calculated_vector(vector: dict[str, Any], sql: bytes, snapshot: bytes) -> dict[str, str]:
    """Return the canonical serialized form with independently computed digests."""
    return {
        "name": vector["name"],
        "migration_sql_hex": vector["migration_sql_hex"],
        "snapshot_json_hex": vector["snapshot_json_hex"],
        "migration_sql_sha256": per_file(sql),
        "snapshot_json_sha256": per_file(snapshot),
        "combined_sha256": combined(sql, snapshot),
    }


def check_fixture(path: Path) -> int:
    """Recompute and compare every digest without modifying the fixture."""
    data = load_fixture(path)
    parsed = parse_vectors(data, require_expected=True)
    mismatches: list[str] = []
    for vector, sql, snapshot in parsed:
        calculated = calculated_vector(vector, sql, snapshot)
        for field in sorted(DIGEST_FIELDS):
            if vector[field] != calculated[field]:
                mismatches.append(
                    f"{vector['name']}.{field}: got {vector[field]}, "
                    f"want {calculated[field]}"
                )
    if mismatches:
        raise FixtureError("digest mismatch:\n  " + "\n  ".join(mismatches))

    print(f"ok: verified {len(parsed)} artifact digest vectors in {path}")
    return 0


def write_fixture(path: Path) -> int:
    """Intentionally regenerate expected digests from the fixture inputs."""
    data = load_fixture(path)
    parsed = parse_vectors(data, require_expected=False)
    canonical = {
        "format": FIXTURE_FORMAT,
        "vectors": [
            calculated_vector(vector, sql, snapshot)
            for vector, sql, snapshot in parsed
        ],
    }
    encoded = json.dumps(canonical, indent=2, ensure_ascii=True) + "\n"
    temporary = path.with_name(path.name + ".tmp")
    try:
        temporary.write_text(encoded, encoding="utf-8")
        temporary.replace(path)
    except OSError as error:
        raise FixtureError(f"cannot write {path}: {error}") from error

    print(f"wrote {len(parsed)} artifact digest vectors to {path}")
    return 0


def parse_args() -> argparse.Namespace:
    """Parse the explicit checker/generator command mode."""
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--check", action="store_true", help="verify without modifying the fixture")
    mode.add_argument("--write", action="store_true", help="regenerate expected digests in the fixture")
    parser.add_argument("--fixture", type=Path, default=FIXTURE_PATH, help=argparse.SUPPRESS)
    return parser.parse_args()


def main() -> int:
    """Run the selected checker or generator mode."""
    args = parse_args()
    try:
        if args.check:
            return check_fixture(args.fixture)
        return write_fixture(args.fixture)
    except FixtureError as error:
        print(f"error: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
