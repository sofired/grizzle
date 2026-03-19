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

| Area | Implemented | Status |
|---|---|---|
| Schema DSL — PostgreSQL column types | Partial | Several types missing — see [schema.md](./schema.md) |
| Schema DSL — MySQL / SQLite column types | Partial | See [schema.md](./schema.md) |
| Query builder — SELECT / INSERT / UPDATE / DELETE | Mostly complete | Minor gaps — see [query-builder.md](./query-builder.md) |
| Query builder — CTEs, FOR UPDATE, prepared statements | Not started | DEVIATION:GAP |
| Kit — `diff` / `sql` / `snapshot` / `migrate` / `status` | Implemented | See [kit.md](./kit.md) for current vs target |
| Kit — `generate` (write SQL files to disk) | Not started | DEVIATION:GAP (not designed) — highest priority |
| Kit — `pull` (DB → Go schema) | Not started | DEVIATION:GAP (not designed) |
| Kit — rename detection | Not started | DEVIATION:GAP (not designed) — data-loss risk |
| Relations — JOIN helpers | Implemented | PARITY |
| Relations — relational query API (`findMany`) | Not started | DEVIATION:GAP (not designed) |
| Code generation (`grizzle gen`) | Implemented | Partial — see [codegen.md](./codegen.md) |
| Transactions | Implemented | Savepoints gap — see [transactions.md](./transactions.md) |
| Dialects — PostgreSQL | Primary | Most complete |
| Dialects — MySQL / SQLite | Partial | See [dialects.md](./dialects.md) |
