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

| Area | Progress / target status |
|---|---|
| Schema DSL — PostgreSQL column types | Partial — see [schema.md](./schema.md) and [codegen.md](./codegen.md) for current per-type status. |
| Schema DSL — MySQL column types | Partial — see [schema.md](./schema.md) and [codegen.md](./codegen.md) for current per-type status. |
| Schema DSL — SQLite column types | Mostly complete — see [schema.md](./schema.md) and [codegen.md](./codegen.md) for current per-type status. |
| Schema DSL — `generatedAlwaysAs` | Designed rejection — recognized in RC.1 snapshots but unsupported in initial schema input; fail with `unsupported_feature`. |
| Query builder — SELECT / INSERT / UPDATE / DELETE | Mostly complete target design — generated table/view/alias no-arg selects, non-recursive SELECT CTE SQL behavior, and dialect-supported FOR UPDATE/FOR SHARE target PARITY; Go CTE helper shape is DEVIATION:LANGUAGE; no-arg derived-source selects, no-arg joined select result shapes, mutation CTE builders, scalar subquery select-list helpers, insert runtime-hook omission, `INSERT ... SELECT`, SQLite ordered multiple conflict clauses, error-returning `Build`, and fail-fast dialect gating remain gaps; see [query-builder.md](./query-builder.md). |
| Query builder — missing operators | `NOT LIKE`, `NOT ILIKE`, `NOT BETWEEN`, designed array helpers, conflict predicates, and `UPDATE…FROM` remain gaps. Query-order `NULLS FIRST`/`NULLS LAST` is GRIZZLE-ONLY; RC.1 uses those helpers for PostgreSQL index configuration rather than query ordering. |
| Query builder — prepared statements | Designed — DEVIATION:GAP until param-capable operators/builders, `BuildPrepared`, ordered prepared-argument plans, per-execution `query.Params`, adaptation of pgx handles/registries, and MySQL/SQLite one-time prepared driver helpers are implemented. |
| Query builder — window frame spec | Boundary sentinels exist, but typed frame construction remains a GRIZZLE-ONLY DEVIATION:GAP. |
| Query builder — lateral join, cursor/streaming | DEVIATION:GAP (not designed); dialect scope and public APIs require ratification. |
| Kit — legacy/current helpers (`diff` / `sql` / `snapshot` / `status`) | Implemented current surface; not part of the target RC.1 public file-migration workflow — see [docs/kit/overview.md](../kit/overview.md) and [kit.md](./kit.md). |
| Kit — `migrate` | DEVIATION:BROKEN — current code does not yet implement the complete artifact, history, session, and validation contract end to end. |
| Kit — `generate` | DEVIATION:GAP (designed); planning, typed rendering, and atomic artifact publication remain incomplete. |
| Kit — `push` | Public implementation is intentionally deferred until a dedicated direct-sync safety specification is ratified. |
| Kit — `pull` | DEVIATION:GAP (designed); introspection exists internally but the managed Pull workflow is incomplete. |
| Kit — `check` | DEVIATION:GAP (designed); the reusable offline validation API and command handler remain incomplete. |
| Kit — rename resolution during `generate` | DEVIATION:GAP (designed); initial behavior uses interactive resolution, while non-interactive/config-based resolution remains future scope. |
| Relations — JOIN helpers (`JoinRel`, `InnerJoinRel`) | Implemented — GRIZZLE-ONLY, as documented in [query-builder.md](./query-builder.md). |
| Relations — relational query API (`findMany`) | DEVIATION:GAP (not designed); manual batch loading is the current approach. |
| Code generation (`grizzle gen`) | Partially implemented for PostgreSQL, MySQL, and SQLite; static schema metadata, resource limits, managed-output safety, redacted diagnostics, nullable assignments, JSON/JSONB, and MySQL marker contracts remain incomplete. |
| Transactions — core callback shape | Implemented — PARITY for begin/callback/commit-or-rollback shape; validation, redacted error contracts, and row ownership still require conformance work. |
| Transactions — isolation levels | DEVIATION:GAP (designed) across the public driver contract. |
| Transactions — savepoints / nested transactions | DEVIATION:GAP (designed). |
| Transactions — MySQL / SQLite wrappers | Generic `database/sql` wrappers exist; integration conformance and option mapping remain incomplete. |
| Dialects — PostgreSQL | PARITY target with the listed gaps; required initial scope. |
| Dialects — MySQL / SQLite | DEVIATION:GAP for remaining column types, interface reconciliation, and fail-fast capability gating. |
