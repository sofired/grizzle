# File Migrations History Specification

## Status

Draft

## Purpose

Define the database-side history model for file-based migrations.

Pinned upstream target:

- Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`

## Scope

This document must define:

- history table name
- columns
- constraints
- ordering fields
- checksums / hashes
- dialect-specific schemas
- migration identity
- any compatibility stance for pre-file-based history

This document also locks the supported history-table scope for the initial implementation.

## Upstream References

- Drizzle Kit migrate docs
- Drizzle ORM dialect migration tables
- Drizzle config docs
- [file-migrations-upstream-mapping.md](./file-migrations-upstream-mapping.md)

## Table Identity

Upstream default identity:

- table name: `__drizzle_migrations`
- PostgreSQL schema default: `drizzle`

Initial Grizzle scope decision:

- support one canonical history-table schema derived from Drizzle RC.1
- do not support automatic upgrade from older Grizzle or pre-RC table layouts
- treat mismatched legacy table shapes as unsupported input unless and until compatibility work is explicitly prioritized later
- use Grizzle-branded defaults as **DEVIATION:INTENTIONAL** namespace/branding divergence from RC.1 defaults:
  - table name: `__grizzle_migrations`
  - PostgreSQL schema default: `grizzle`

Normative rules:

- Grizzle file-based migrations use exactly one history table per configured migration target
- the default table name is `__grizzle_migrations` (**DEVIATION:INTENTIONAL** from RC.1's `__drizzle_migrations`)
- the default PostgreSQL schema is `grizzle` (**DEVIATION:INTENTIONAL** from RC.1's `drizzle`)
- the table name must be configurable
- the PostgreSQL schema must be configurable
- configuration changes affect only table location and identifier names, not row semantics or required columns

## Columns

Upstream runtime evidence shows Drizzle stores at least:

- `id`
- `hash`
- `created_at`
- `name`
- `applied_at`

Grizzle adopts the same logical column set for the initial design.

## Supported History Table Schema

The initial Grizzle design must support the RC.1-era logical schema:

- `id`
  - primary key
- `hash`
  - migration SQL hash
- `created_at`
  - order field preserved from Drizzle lineage
- `name`
  - migration folder name / primary migration identity
- `applied_at`
  - timestamp of actual execution for newly applied rows

Grizzle must treat this as the required supported logical shape for initial file-based migrations.

Normative column semantics:

- `id`
  - surrogate primary key for history rows
  - not used as migration identity
- `hash`
  - stores the digest of the migration SQL artifact contents
  - records the applied artifact version for drift/audit checks
- `created_at`
  - stores the migration's artifact ordering timestamp derived from the directory name
  - is not the primary identity key
- `name`
  - stores the full migration directory name
  - is the canonical logical migration identity
  - must fit the cross-dialect migration-name limit defined in the artifact spec
- `applied_at`
  - stores when the migration was actually recorded as applied in the target database
  - reflects execution history rather than artifact naming

Initial logical row model:

- one applied migration corresponds to one history row
- one migration identity may appear at most once
- a row represents a successful application according to the active driver model

## Constraints

Initial required constraints:

- primary key on `id`
- unique constraint on `name`

Rationale:

- RC.1 pending detection is name-based
- `name` is the logical migration identity
- a uniqueness constraint prevents duplicate application records for the same migration identity
- Grizzle does not need to preserve Drizzle's omission of this constraint for compatibility reasons

Decision label:

- `UNIQUE(name)`, `name NOT NULL`, and `created_at NOT NULL` are DEVIATION:INTENTIONAL history hardening relative to Drizzle RC.1

Current scope note:

- RC.1 tagged runtime evidence reviewed so far does not show a uniqueness constraint on `name`
- RC.1 creates `name` and `created_at` as nullable in core migrators
- Grizzle intentionally tightens this behavior and enforces `UNIQUE(name)`, `name NOT NULL`, and `created_at NOT NULL`
- this is intentional schema hardening: Grizzle starts with the RC.1 file model and does not need Drizzle's transitional nullable history rows

Normative constraints:

- `id` must be the primary key
- `name` must be constrained unique
- `hash` must be `NOT NULL`
- `name` must be `NOT NULL`
- `created_at` must be `NOT NULL`

The initial spec does not require:

- uniqueness on `hash`
- uniqueness on `created_at`
- uniqueness on `(created_at, name)`

## Ordering Rules

RC.1 lineage indicates:

- `created_at` remains present as an order/legacy field
- `name` is the active migration identity for pending detection
- `applied_at` records actual execution time for newer rows

Implication for Grizzle:

- history semantics must distinguish ordering fields from identity fields
- the spec must not treat `created_at` and `name` as interchangeable

Normative rules:

- `name` is the migration identity used for pending-detection set membership
- `created_at` is the persisted artifact-order field
- `applied_at` is the persisted execution-time field
- history consumers must not infer migration identity from `created_at` alone
- history consumers must not infer application order solely from `id`

Expected relationship between artifact and history:

- `created_at` must correspond to the timestamp prefix encoded in the migration directory name
- `created_at` stores the UTC epoch-millisecond value derived from the `YYYYMMDDHHmmss` directory prefix using Drizzle RC.1-style `Date.UTC(...)` semantics
- `name` must match the full migration directory name exactly
- `name` must be no more than 255 bytes so the same artifact identity can be recorded by PostgreSQL, MySQL, and SQLite history implementations
- `applied_at` may differ from `created_at`, especially when older migrations are applied later in another environment

## Checksum / Hash Policy

RC.1 stores a hash even though pending detection is name-based.

Grizzle must preserve that model, but with a clearer stated purpose.

Normative hash policy:

- the history table must store a hash for each applied migration
- the hash is SHA-256 encoded as lowercase hexadecimal
- the hash is computed over the exact raw `migration.sql` file bytes
- hashing must not normalize newlines, strip comments, remove breakpoints, decode/re-encode text, or reformat SQL
- the hash is an audit metadata field, not the pending-detection key
- a migration is considered "already applied" by matching `name`, not by matching `hash`
- the DB history hash covers the raw `migration.sql` file bytes only; `snapshot.json` digests belong to local artifact validation unless a future manifest/history extension is explicitly scoped

Applied hash stance:

- Grizzle follows Drizzle RC.1 by skipping already-applied migrations by `name` without comparing the stored hash to the current local file hash
- an already-applied migration whose comments, whitespace, or formatting changed locally is still considered already applied if `name` matches
- Grizzle intentionally keeps raw-byte hashing for newly recorded rows, even though RC.1 hashes the JavaScript string read from `migration.sql`
- raw-byte hashing is DEVIATION:INTENTIONAL Go/filesystem determinism
- skipping already-applied migrations by `name` is a PARITY decision with a security tradeoff: applied artifacts must be treated as immutable in source control because existing environments will not reject a changed local file by hash
- `migrate` must not add a default hash-drift blocker or `--allow-applied-hash-drift` workflow in the initial RC.1-aligned implementation; that would reverse the accepted name-based parity decision
- future audit/status tooling may report applied hash drift, but it must not change pending detection or default migration execution semantics without an explicit new scope decision
- the accepted mitigation is operational immutability: committed migration artifacts must be protected by source-control review, CI `check`, and release discipline. Fresh-environment divergence after local artifact mutation is a known risk of the RC.1 parity decision, not a runtime blocker in the initial implementation.

This means the hash is metadata for audit and tooling, not an enforcement key for pending detection.

Current scope note:

- RC.1 runtime still computes and stores migration SQL hashes
- pending detection is name-based, not hash-based
- hash remains relevant for compatibility/upgrade logic in Drizzle lineage, but Grizzle is not required to inherit that upgrade path now

## Dialect Variants

### PostgreSQL

- table location is `<configured-schema>.<configured-table>`
- default schema is `grizzle` (**DEVIATION:INTENTIONAL** namespace/branding divergence from RC.1's `drizzle`)
- the configured schema must be created before the history table when absent
- schema and table identifiers must be quoted as identifiers, not interpolated as SQL fragments

### MySQL

- table location is the configured table name in the active database
- no separate schema concept equivalent to PostgreSQL's migration schema is required by this spec
- unset migration schema configuration resolves to empty for MySQL so Grizzle defaults do not self-fail
- an explicitly non-empty PostgreSQL-style migration schema configuration must fail for MySQL; this is DEVIATION:INTENTIONAL config-hardening relative to Drizzle RC.1, whose runtime ignores the schema value for MySQL
- table identifiers must be quoted as identifiers

### SQLite

- table location is the configured table name in the active database file/connection
- type affinity details may vary, but the logical column set and semantics must remain equivalent
- unset migration schema configuration resolves to empty for SQLite so Grizzle defaults do not self-fail
- an explicitly non-empty PostgreSQL-style migration schema configuration must fail for SQLite; this is DEVIATION:INTENTIONAL config-hardening relative to Drizzle RC.1, whose runtime ignores the schema value for SQLite
- `applied_at` is stored as text using UTC RFC3339/RFC3339Nano-compatible time
- table identifiers must be quoted as identifiers

## Identifier Rules

Configured history identifiers must be single SQL identifiers.

Normative rules:

- `HistoryTable` must be a single table identifier
- `HistorySchema` must be a single schema identifier where the dialect supports it
- dotted fragments such as `foo.bar` are invalid as table names
- callers must use `HistorySchema = "foo"` and `HistoryTable = "bar"` for schema-qualified PostgreSQL placement
- implementations must route all history identifiers through dialect identifier quoting
- identifier validation must reject empty strings, NUL bytes, path separators, and SQL fragments

## Compatibility Stance

Initial compatibility stance:

- supported: RC.1-style history table shape
- unsupported: automatic upgrade from older Drizzle-compatible shapes
- unsupported: automatic upgrade from Grizzle pre-file-based history tables
- unsupported: backfill/baseline logic for older local experiments unless explicitly re-scoped later

If Grizzle encounters an incompatible pre-existing table shape, the default behavior is to fail with a clear error instead of mutating the schema automatically.

Normative compatibility rules:

- the initial implementation supports only the RC.1-aligned logical schema documented here
- old three-column history tables are unsupported
- tables missing `name` are unsupported
- tables missing `applied_at` are unsupported
- Grizzle must not silently rewrite or upgrade an existing incompatible history table
- Grizzle must surface an explicit unsupported-schema error when an incompatible table is found
- absent history table is not an incompatible schema; it is a first-run state handled by `migrate` or `pull --init`
- a history row whose `name` is absent from local artifacts is an invalid deployment state by default and must fail with `history_artifact_missing`

Decision label:

- failing on applied DB history rows missing from the local artifact set is DEVIATION:INTENTIONAL deployment-state hardening relative to Drizzle RC.1

## Applied Row Semantics

History insertion semantics must be stable across dialects even when transaction mechanics differ.

Normative rules:

- a history row must be inserted only after the corresponding migration is considered successfully applied
- the inserted row must record:
  - the migration `name`
  - the migration `hash`
  - the migration `created_at`
  - the migration `applied_at`
- if a migration run fails before a given migration is considered applied, no committed history row for that migration may remain for transactional drivers
- for non-transactional drivers, the execution spec must document partial-application risk explicitly, but this history spec still treats a row as the record of successful application

`applied_at` contract:

- Grizzle follows the RC.1 storage posture and treats `applied_at` as a driver-native applied-time column
- `applied_at` does not need to be `NOT NULL` at the logical-schema layer
- Drizzle RC.1's insert behavior differs by dialect: PostgreSQL and MySQL define database defaults and omit `applied_at` from normal migration inserts, while SQLite inserts an explicit timestamp value
- Grizzle initial implementations must provide an explicit `applied_at` value when inserting history rows for all supported dialects; this is DEVIATION:INTENTIONAL cross-dialect determinism/auditability relative to RC.1's PostgreSQL/MySQL insert posture
- database defaults may exist as a physical convenience but are not required by the logical schema
- the observed row must represent the actual record time of successful application

## Logical Schemas

These schemas are normative at the logical-contract level. Dialect implementations may choose equivalent physical column types only when the validator preserves these semantics:

- `id` is a surrogate primary key
- `hash` is required and stores a lowercase hex SHA-256 of raw `migration.sql`
- `created_at` is required and stores the UTC timestamp prefix as epoch milliseconds
- `name` is required and has a unique constraint
- `applied_at` may be nullable but must represent the actual record time of successful application when present

PostgreSQL-style logical shape:

```sql
create table grizzle.__grizzle_migrations (
  id serial primary key,
  hash text not null,
  created_at bigint not null,
  name text not null unique,
  applied_at timestamp with time zone
);
```

MySQL-style logical shape:

```sql
create table __grizzle_migrations (
  id bigint unsigned not null auto_increment primary key,
  hash varchar(64) not null,
  created_at bigint not null,
  name varchar(255) not null,
  applied_at timestamp null default current_timestamp,
  unique key uq___grizzle_migrations_name (name)
);
```

MySQL physical-type note:

- Drizzle RC.1 creates broader text-shaped columns for `hash` and nullable `name` in its MySQL migration table path.
- Grizzle narrows `hash` to `varchar(64)` because the stored value is specified as lowercase SHA-256 hex.
- Grizzle narrows `name` to `varchar(255) not null` because migration identity is bounded by the artifact-name limit and protected by `UNIQUE(name)`.
- These MySQL physical type choices are DEVIATION:INTENTIONAL schema hardening, not exact RC.1 DDL parity.

SQLite-style logical shape:

```sql
create table "__grizzle_migrations" (
  id integer primary key autoincrement,
  hash text not null,
  created_at integer not null,
  name text not null unique,
  applied_at text
);
```

## Secondary Indexes

Initial scope decision:

- no secondary index on `created_at` is required for correctness
- implementations may add a non-unique `created_at` index later for operational lookup performance
- `name` remains the only required indexed logical migration identity beyond the primary key
