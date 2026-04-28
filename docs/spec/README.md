# Grizzle Specification

This directory is the authoritative specification for Grizzle's behaviour.

## Purpose

Grizzle is a Go port of [Drizzle ORM](https://orm.drizzle.team) and [Drizzle Kit](https://orm.drizzle.team/docs/kit-overview). The primary design goal is **faithful parity with Drizzle**, adapted only where Go's type system or runtime model makes the original design impossible or significantly inferior.

Every intentional deviation from Drizzle is documented here with an explicit rationale. Any behaviour that diverges without a documented reason is a bug.

This spec was written against **Drizzle ORM v0.40 / Drizzle Kit v0.30**. When Drizzle releases a version that changes behaviour, update the relevant spec file and record the new target version here.

## How to use this spec

- **Before implementing a feature**, check the relevant spec file to understand the Drizzle target behaviour.
- **When a PR deviates from Drizzle** without a documented reason, it should be rejected or a rationale added here first.
- **When Drizzle itself changes**, update the relevant spec file and note the new target behaviour.

## Files

| File | Covers |
|---|---|
| [overview.md](./overview.md) | Design philosophy, parity commitment, deviation taxonomy |
| [schema.md](./schema.md) | Schema DSL — table, column, and constraint definitions |
| [kit.md](./kit.md) | Migration Kit — current CLI, target workflow, generate / migrate / push / pull |
| [query-builder.md](./query-builder.md) | SELECT, INSERT, UPDATE, DELETE builders |
| [relations.md](./relations.md) | Relation definitions and relational queries |
| [codegen.md](./codegen.md) | `grizzle gen` — typed Go struct generation from schema definitions |
| [dialects.md](./dialects.md) | Dialect support matrix and dialect-specific behaviour |
| [types.md](./types.md) | Column type system, Go type mappings, and `BuildContext` contract |
| [transactions.md](./transactions.md) | Transaction API, isolation levels, savepoints |

## Deviation taxonomy

Each spec file marks individual items with one of these labels:

- **PARITY** — matches Drizzle exactly
- **DEVIATION:LANGUAGE** — differs from Drizzle due to a hard Go language constraint (e.g. no runtime `$onUpdate` hooks)
- **DEVIATION:INTENTIONAL** — consciously differs from Drizzle for a documented reason; requires explicit sign-off
- **DEVIATION:GAP** — not yet implemented; behaviour should be brought to parity
- **DEVIATION:BROKEN** — implemented but incorrectly relative to the target; higher priority than GAP
- **GRIZZLE-ONLY** — a Grizzle addition with no Drizzle equivalent

Anything not labelled targets PARITY.

`DEVIATION:GAP` items are further qualified:
- `(designed)` — the target API is specified in this file; ready to implement
- `(not designed)` — the target API is not yet specified; design work required first

## Current completeness

| Area | Status | Tracking |
|---|---|---|
| Schema DSL — PostgreSQL column types | Partial — core types done; date, time, real, doublePrecision, char, inet, bytea, interval, pgEnum, array, and geometry types are DEVIATION:GAP | M5 (#32, #137, #144) |
| Schema DSL — MySQL column types | Partial — datetime, date, time, float, double, decimal, json, smallint, tinyint, binary, varbinary, char are DEVIATION:GAP | M5 (#32) |
| Schema DSL — SQLite column types | Mostly complete — real, blob, numeric, JSON, JSONB are PARITY; no remaining column type gaps | — |
| Schema DSL — `generatedAlwaysAs` | Not started — DEVIATION:GAP (designed) | Unmilestoned (#172) |
| Query builder — SELECT / INSERT / UPDATE / DELETE | Mostly complete — CTEs (recursive + non-recursive) and FOR UPDATE/FOR SHARE are PARITY; see [query-builder.md](./query-builder.md) for remaining gaps | — |
| Query builder — missing operators | Not started — `NOT LIKE`, `NOT ILIKE`, `NOT BETWEEN` (P1); `NULLS FIRST`/`NULLS LAST` (P1); `WHERE` on `ON CONFLICT` (P1); `UPDATE…FROM` (P1); `CROSS JOIN` (designed) | M4 (#164, #163, #162, #167) |
| Query builder — prepared statements | Not started — DEVIATION:GAP (not designed) | M6 (#166) |
| Query builder — window frame spec | Not started — DEVIATION:GAP (designed) | M6 (#139) |
| Query builder — lateral join, cursor/streaming | Not started — DEVIATION:GAP (not designed) | M6 (#171, #170) |
| Kit — `diff` / `sql` / `snapshot` / `status` | Implemented — see [kit.md](./kit.md) | — |
| Kit — `migrate` | DEVIATION:BROKEN — currently introspects live DB rather than reading `.sql` files; must be refactored once `generate` is implemented | M3 (#154, P0) |
| Kit — `generate` (write SQL migration files to disk) | Not started — DEVIATION:GAP (designed); highest-priority Kit gap | M3 (#153, P0) |
| Kit — `push` CLI command | Not started — library function exists; CLI wrapper missing | M3 (#157, P1) |
| Kit — `pull` (DB → Go schema definitions) | Not started — DEVIATION:GAP (not designed); introspection exists internally | M3 (#158, P1) |
| Kit — `check` command | Not started — DEVIATION:GAP (not designed); only meaningful after `generate` | M3 (#169) |
| Kit — rename detection | Implemented via `RenamedFrom()` schema annotation — GRIZZLE-ONLY (documented in schema.md) | — |
| Relations — JOIN helpers (`JoinRel`, `InnerJoinRel`) | Implemented — GRIZZLE-ONLY (documented in query-builder.md) | — |
| Relations — relational query API (`findMany`) | Not started — DEVIATION:GAP (not designed); manual batch-loading is the current approach | Not milestoned |
| Code generation (`grizzle gen`) | Implemented for PostgreSQL, MySQL, and SQLite — see [codegen.md](./codegen.md); known limitation: `GrizTableAlias()` hardcoded (self-join workaround documented) | — |
| Transactions — core API | Implemented — PARITY | — |
| Transactions — isolation levels | Not started — DEVIATION:GAP (designed) | M6 (#159, P1) |
| Transactions — savepoints / nested transactions | Not started — DEVIATION:GAP (designed) | M6 (#143, P1) |
| Transactions — MySQL / SQLite wrappers | Not started — DEVIATION:GAP (designed) | M6 (#160, P1) |
| Dialects — PostgreSQL | Core dialect; most complete | — |
| Dialects — MySQL / SQLite | Partial — column type gaps above; dialect interface fully implemented (PARITY) | M5 for type gaps |
