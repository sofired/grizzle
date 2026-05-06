# Project Implementation Plan

This is the project-wide implementation plan for Grizzle.

It does not replace the detailed file-migration slice plan in [docs/spec/file-migrations-implementation-sequence.md](./spec/file-migrations-implementation-sequence.md). Instead, it places that sequence in the broader project roadmap.

## Global Rules

- Specs in `docs/spec/` are authoritative for Grizzle behavior.
- Tagged Drizzle ORM / Drizzle Kit `v1.0.0-rc.1` source is authoritative for upstream parity details.
- Open issues and current code must be reconciled to specs before implementation.
- If a planned behavior is not specified, write or amend the spec before coding.
- Keep implementation slices small enough to test and revert.

## Workstreams

| Workstream | Source specs | Current posture | Planning action |
| --- | --- | --- | --- |
| Schema DSL and type system | `schema.md`, `types.md`, `dialects.md` | Partial implementation with known parity gaps. | Create parent issue for schema/type parity and strict file-migration schema input. |
| Query builder and relations | `query-builder.md`, `relations.md`, `dialects.md` | Substantial implementation with remaining gaps and dialect-gating bugs. | Create parent issue/milestone for query parity cleanup. |
| Codegen | `codegen.md`, `types.md` | Implemented but narrower than target. | Create parent issue for codegen type mapping, managed output, metadata, and nullable/JSON decisions. |
| Drivers, transactions, prepared statements | `transactions.md`, `query-builder.md` | Partial implementation; pgx stronger than database/sql. | Create parent issue for transaction/prepared driver parity. |
| Dialects | `dialects.md` | Useful current matrix; some spec/interface drift. | Keep synced with query/schema/driver work. |
| Migration kit / file migrations | `kit.md`, `file-migrations-*.md` | Specs detailed; target implementation largely absent; legacy code conflicts exist. | Follow slices 0-8 after backlog normalization. |
| Pull / introspection | `pull.md`, `file-migrations-snapshot-fields.md`, `schema.md`, `codegen.md` | Introspection exists, source generation/pull workflow absent. | Implement after core artifact/history/migrate path, per Slice 7. |
| Docs/release/policy | `README.md`, `docs/spec/README.md`, `overview.md` | Good pre-release posture; issue labels/milestones need normalization. | Keep docs/spec/current-state pages synchronized with implementation. |

## Recommended Order

This order is about dependency safety, not exclusivity. Independent query/schema/driver fixes can proceed when they are spec-aligned and do not conflict with the migration-kit sequence.

1. Normalize GitHub backlog project-wide.
2. Fix or decide near-term schema/codegen correctness issues that would poison generated schema metadata.
3. Start file-migration Slice 0 only after migration issues are rewritten/superseded and slice parent issues exist.
4. Build file-migration slices 0-6 to establish artifact generation/check/history/execution.
5. Implement `pull` / `pull --init` after the artifact/history path exists.
6. Perform CLI cutover and cleanup once target workflows are end-to-end.
7. Continue non-blocking query/driver/schema parity work in parallel when it has clear specs and disjoint code paths.

## File-Migration Slice Reference

The detailed sequence remains:

- Slice 0: package boundary and test harness
- Slice 1: artifact discovery and offline validation core
- Slice 2: snapshot and schema input planning
- Slice 3: `check`
- Slice 4: `generate`
- Slice 5: history, locking, and migration sessions
- Slice 6: `migrate`
- Slice 7: `pull` and `pull --init`
- Slice 8: CLI cutover and cleanup

Use [file-migrations-implementation-sequence.md](./spec/file-migrations-implementation-sequence.md) for the normative details.

## Parallel Work Guidance

Safe to work in parallel with file-migration slices when scoped and spec-backed:

- query-builder bug fixes and missing operators
- codegen type-mapping fixes
- schema builder parity fixes
- dialect doc/interface synchronization
- driver tests and transaction wrapper work

Do not work in parallel if it changes:

- public migration command meanings
- `kit.Snapshot`/artifact semantics
- history table semantics
- schema input accepted by file migrations
- generated source format consumed by `pull` or file migrations

unless it is explicitly coordinated with the relevant slice.
