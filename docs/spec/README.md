# Grizzle Specification

This directory is the authoritative specification for Grizzle's behavior.

## Purpose

Grizzle is a Go port of [Drizzle ORM](https://orm.drizzle.team) and [Drizzle Kit](https://orm.drizzle.team/docs/kit-overview). The primary design goal is **faithful parity with Drizzle**, adapted only where Go's type system or runtime model makes the original design impossible or significantly inferior.

Every intentional deviation from Drizzle is documented here with an explicit rationale. Any behavior that diverges without a documented reason is a bug.

This spec set is currently pinned to **Drizzle ORM / Drizzle Kit v1.0.0-rc.1** for the file-migrations and migration-kit workflow. When Drizzle changes behavior in a way Grizzle intends to track, update the relevant spec files and record the new target version here.

Source-of-truth hierarchy for migration-kit work:

1. Grizzle specs in this directory
2. tagged Drizzle `v1.0.0-rc.1` source
3. Drizzle docs for the matching release line
4. newer or mixed-version Drizzle docs and tutorials

When tagged source and docs disagree, the tagged source is the upstream fact. When Grizzle intentionally tightens or adapts that behavior, the Grizzle spec is authoritative and must label the divergence.

## How to use this spec

- **Before implementing a feature**, check the relevant spec file to understand the Drizzle target behavior.
- **When a PR deviates from Drizzle** without a documented reason, it should be rejected or a rationale added here first.
- **When Drizzle itself changes**, update the relevant spec file and note the new target behavior.

## Files

| File | Covers |
|---|---|
| [overview.md](./overview.md) | Design philosophy, parity commitment, deviation taxonomy |
| [schema.md](./schema.md) | Schema DSL — table, column, and constraint definitions |
| [kit.md](./kit.md) | Migration Kit — current CLI, target workflow, generate / migrate / push / pull |
| [pull.md](./pull.md) | `grizzle pull` / `grizzle introspect` — database-to-schema reverse generation |
| [file-migrations-workflow.md](./file-migrations-workflow.md) | End-to-end file migration workflow |
| [file-migrations-generate.md](./file-migrations-generate.md) | `grizzle generate` — schema definitions to migration artifacts |
| [file-migrations-artifacts.md](./file-migrations-artifacts.md) | Migration artifact layout, naming, snapshots, and SQL files |
| [file-migrations-snapshot-fields.md](./file-migrations-snapshot-fields.md) | Drizzle RC.1 snapshot fields and initial support/failure status for PostgreSQL, MySQL, and SQLite |
| [file-migrations-history.md](./file-migrations-history.md) | Database history table schema and row semantics |
| [file-migrations-check.md](./file-migrations-check.md) | Offline artifact, snapshot, and graph validation |
| [file-migrations-execution.md](./file-migrations-execution.md) | `grizzle migrate` execution semantics |
| [file-migrations-api.md](./file-migrations-api.md) | File migration CLI and Go API contracts |
| [file-migrations-implementation-sequence.md](./file-migrations-implementation-sequence.md) | Approved implementation sequence for the file-migration workflow |
| [query-builder.md](./query-builder.md) | SELECT, INSERT, UPDATE, DELETE builders |
| [relations.md](./relations.md) | Relation definitions and relational queries |
| [codegen.md](./codegen.md) | `grizzle gen` — typed Go struct generation from schema definitions |
| [dialects.md](./dialects.md) | Dialect support matrix and dialect-specific behavior |
| [types.md](./types.md) | Column type system, Go type mappings, and `BuildContext` contract |
| [transactions.md](./transactions.md) | Transaction API, isolation levels, savepoints |

## Deviation taxonomy

Each spec file marks individual items with one of these labels:

- **PARITY** — matches Drizzle exactly
- **DEVIATION:LANGUAGE** — differs from Drizzle due to a hard Go language constraint (e.g. Go struct tags instead of TypeScript object-key inference)
- **DEVIATION:INTENTIONAL** — consciously differs from Drizzle for a documented reason; requires explicit sign-off
- **DEVIATION:GAP** — not yet implemented; behavior should be brought to parity
- **DEVIATION:BROKEN** — implemented but incorrectly relative to the target; higher priority than GAP
- **GRIZZLE-ONLY** — a Grizzle addition with no Drizzle equivalent

Anything not labelled targets PARITY.

`DEVIATION:GAP` items are further qualified:
- `(designed)` — the target API is specified in this file; ready to implement
- `(not designed)` — the target API is not yet specified; design work required first

The upstream mapping document also uses review-planning labels:

- `Copy` maps to **PARITY** unless the target spec adds a narrower divergence note
- `Adapt` maps to a documented Go or Grizzle adaptation
- `Diverge` maps to **DEVIATION:INTENTIONAL**

## Current completeness

| Area | Progress / target status | Tracking |
|---|---|---|
| Schema DSL — PostgreSQL column types | Partial — see [schema.md](./schema.md) and [codegen.md](./codegen.md) for current per-type status | M5 (#32, #137, #144) |
| Schema DSL — MySQL column types | Partial — see [schema.md](./schema.md) and [codegen.md](./codegen.md) for current per-type status | M5 (#32) |
| Schema DSL — SQLite column types | Mostly complete — see [schema.md](./schema.md) and [codegen.md](./codegen.md) for current per-type status | — |
| Schema DSL — `generatedAlwaysAs` | Designed rejection — recognized in RC.1 snapshots but unsupported in initial schema input; fail with `unsupported_feature` | Unmilestoned (#172) |
| Query builder — SELECT / INSERT / UPDATE / DELETE | Mostly complete target design — generated table/view/alias no-arg selects, non-recursive SELECT CTE SQL behavior, and dialect-supported FOR UPDATE/FOR SHARE target PARITY; Go CTE helper shape is DEVIATION:LANGUAGE; no-arg derived-source selects, no-arg joined select result shapes, mutation CTE builders, scalar subquery select-list helpers, insert runtime-hook omission, `INSERT ... SELECT`, SQLite ordered multiple conflict clauses, error-returning `Build`, and fail-fast dialect gating remain gaps; see [query-builder.md](./query-builder.md) for remaining gaps | — |
| Query builder — missing operators | Not started — `NOT LIKE`, `NOT ILIKE`, `NOT BETWEEN` (P1); array helpers `arrayContains`, `arrayContained`, `arrayOverlaps` (designed); `WHERE` on `ON CONFLICT` (P1); `UPDATE…FROM` target design is now covered in [query-builder.md](./query-builder.md) but remains an implementation gap; `CROSS JOIN` (designed). Query-order `NULLS FIRST`/`NULLS LAST` would be GRIZZLE-ONLY if added; RC.1 uses those helpers for PostgreSQL index config, not query ordering. | M4 (#164, #163, #162, #167) |
| Query builder — prepared statements | Designed — DEVIATION:GAP until param-capable operators/builders, `BuildPrepared`, ordered prepared-argument plans, per-execution `query.Params`, pgx reusable handles/registries, and MySQL/SQLite one-time prepared driver helpers are implemented | M6 (#166) |
| Query builder — window frame spec | Not started — GRIZZLE-ONLY future extension; DEVIATION:GAP (not designed) | M6 (#139) |
| Query builder — lateral join, cursor/streaming | Not started — DEVIATION:GAP (not designed) | M6 (#171, #170) |
| Kit — legacy/current helpers (`diff` / `sql` / `snapshot` / `status`) | Implemented current surface; not part of the target RC.1 public file-migration workflow — see [docs/kit/overview.md](../kit/overview.md) and [kit.md](./kit.md) | — |
| Kit — `migrate` | DEVIATION:BROKEN — current branch work reads migration SQL from a directory, but the full RC.1-style artifact, history, and validation contract is not yet implemented end-to-end | M3 (#154, P0) |
| Kit — `generate` (write SQL migration files to disk) | Not started — DEVIATION:GAP (designed); highest-priority Kit gap | M3 (#153, P0) |
| Kit — `push` CLI command | Public command surface retained, but new CLI/API work is blocked until a dedicated direct-sync safety spec exists | M3 (#157, P1) |
| Kit — `pull` (DB → Go schema definitions) | Not started — DEVIATION:GAP (designed); introspection exists internally | M3 (#158, P1) |
| Kit — `check` command | Not started — DEVIATION:GAP (designed); required before `generate` and `migrate`, with richer branch-collision value after artifacts exist | M3 (#169) |
| Kit — rename resolution during `generate` | Not started — DEVIATION:GAP (designed); initial target is Drizzle RC.1-style interactive prompts, with non-interactive/config-based resolution deferred | M3 (#153, P0); track upstream follow-on in `sofired/grizzle#279` |
| Relations — JOIN helpers (`JoinRel`, `InnerJoinRel`) | Implemented — GRIZZLE-ONLY (documented in query-builder.md) | — |
| Relations — relational query API (`findMany`) | Not started — DEVIATION:GAP (not designed); manual batch-loading is the current approach | Not milestoned |
| Code generation (`grizzle gen`) | Partially implemented for PostgreSQL, MySQL, and SQLite; current helper generation is narrower than the target static schema loader, resource-limit, managed-output safety, redacted diagnostics, nullable assignment, JSON/JSONB, and MySQL marker contracts — see [codegen.md](./codegen.md) | M3 follow-up |
| Transactions — core callback shape | Implemented — PARITY for begin/callback/commit-or-rollback shape; DB/Tx row-vs-exec validation, nil/typed-nil handling, redacted error-code contract, and row ownership rules are specified target behavior but remain implementation gaps until the driver package satisfies [transactions.md](./transactions.md) | M6 follow-up |
| Transactions — isolation levels | Not started — DEVIATION:GAP (designed) | M6 (#159, P1) |
| Transactions — savepoints / nested transactions | Not started — DEVIATION:GAP (designed) | M6 (#143, P1) |
| Transactions — MySQL / SQLite wrappers | Not started — DEVIATION:GAP (designed) | M6 (#160, P1) |
| Dialects — PostgreSQL | PARITY target with listed gaps; required initial scope | — |
| Dialects — MySQL / SQLite | DEVIATION:GAP (designed) for remaining column type gaps; target dialect interface rename/shim work and fail-fast dialect gating remain implementation gaps | M5 for type gaps |
