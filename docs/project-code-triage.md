# Project-Wide Existing-Code Triage

This document broadens the existing-code review beyond file migrations.

Source of truth for classification:

1. the Grizzle specs under `docs/spec/`
2. tagged Drizzle ORM / Drizzle Kit `v1.0.0-rc.1` source for upstream behavior
3. Drizzle docs for the matching release line
4. current repository code only as implementation evidence, never as the target contract

If current code and specs disagree, the code is implementation debt unless the spec is amended. If the specs and tagged Drizzle RC.1 source disagree, the specs must explicitly document the Grizzle decision.

## Package-Level Classification

| Path | Project area | Classification | Why | Primary spec(s) |
| --- | --- | --- | --- | --- |
| `cmd/grizzle` | CLI | Adapt / Quarantine | Current CLI has useful command parsing, but public command shape is not the target. `gen` remains useful; `snapshot`, `diff`, legacy `migrate`, and `status` are not target file-migration commands. | `kit.md`, `file-migrations-api.md`, `codegen.md` |
| `dialect/` | Dialect feature matrix | Adapt | Current interface and implementations are useful, but specs call out interface/listing drift and remaining fail-fast capability gating work. Migration sessions may also need dialect capability hooks. | `dialects.md`, `query-builder.md`, `file-migrations-execution.md` |
| `driver/pgx/` | PostgreSQL driver | Adapt | Useful current driver. Still has prepared statement and transaction/test gaps. Migration sessions should not be forced to use this API shape unless it matches the session spec. | `transactions.md`, `query-builder.md`, `file-migrations-api.md` |
| `driver/sql/` | database/sql driver | Adapt | Useful generic driver for MySQL/SQLite. Prepared statement API and dialect-specific wrappers remain planned work. | `transactions.md`, `query-builder.md` |
| `expr/` | SQL expression builders | Adapt | Current query expressions are useful. Some dialect guards and operators remain gaps/bugs. Do not treat query expressions as the file-migration DDL expression model. | `query-builder.md`, `types.md`, `dialects.md` |
| `gen/codegen/` | `grizzle gen` typed codegen | Adapt | Current codegen works but has spec gaps for type mapping, nullable assignment, JSON/JSONB, managed-output safety, and source metadata needed by file migrations/pull. | `codegen.md`, `types.md`, `schema.md` |
| `gen/parser/` | static schema parser | Adapt / likely replace for strict loader | Useful AST parsing foundation, but it lacks the target no-follow/resource-limit/static-loader/redaction contract and only covers a subset of schema objects. | `file-migrations-api.md`, `file-migrations-generate.md`, `codegen.md` |
| `internal/testschema/` | fixtures | Keep / Adapt | Useful fixtures. Some may need expansion or replacement for spec-parity cases. | all relevant test specs |
| `kit/` | migration kit current implementation | Split: Adapt / Quarantine / Delete-replace | `diff` and SQL rendering are useful foundations; current live-diff push/migrate/status and legacy snapshot file format must not define the RC.1 file-migration target. | `kit.md`, all `file-migrations-*.md`, `pull.md` |
| `kit/introspect/` | live DB introspection | Adapt | Useful foundation for `pull`, codegen, and schema comparison. Needs strict unsupported-object handling, redaction, broad-scan controls, source rendering, and managed bootstrap artifacts. | `pull.md`, `file-migrations-snapshot-fields.md`, `schema.md` |
| `query/` | query builder | Adapt | Core query builder is substantially implemented, but specs list remaining gaps and bugs. Query work should continue from `query-builder.md`, not from file-migration sequence docs. | `query-builder.md`, `relations.md`, `dialects.md` |
| `schema/pg/` | PostgreSQL schema DSL | Adapt | Useful current DSL. Planned parity gaps remain; some current raw-string conveniences are not strict file-migration input. | `schema.md`, `types.md`, `file-migrations-snapshot-fields.md` |
| `schema/mysql/` | MySQL schema DSL | Adapt | Useful wrapper surface, but still inherits PG internals and has MySQL parity gaps. | `schema.md`, `types.md`, `dialects.md` |
| `schema/sqlite/` | SQLite schema DSL | Adapt | Useful wrapper surface, but some Grizzle-only compatibility helpers must be distinguished from RC.1 parity. | `schema.md`, `types.md`, `dialects.md` |

## Project Areas And Current Direction

### Schema DSL

Current code has a useful table/column/constraint foundation across PostgreSQL, MySQL, and SQLite.

Triage posture:

- `Adapt` the existing builders, but do not let current builder availability define parity.
- Implement missing parity according to `schema.md` and `types.md`.
- Treat raw SQL checks/views as legacy/trusted conveniences unless the strict file-migration DDL-expression path accepts them explicitly.
- For generated columns and standalone sequences, initial file migrations should recognize and reject where specified rather than silently support partial old-model behavior.

### Query Builder And Relations

Current query code is not a file-migration blocker, but it is part of the planned project capability set.

Triage posture:

- Continue query work from `query-builder.md` and `relations.md`.
- Fix dialect-gating bugs and missing operators independently of the file-migration sequence.
- Prepared statements, relational query API, lateral joins, streaming/cursors, and window-frame extensions remain separate implementation tracks.

### Codegen

Current `grizzle gen` is an existing user-facing capability and should be reviewed against `codegen.md`.

Triage posture:

- Keep current codegen behavior only where it matches specs.
- Fix type mapping decisions before they become inputs to schema/pull/file-migration workflows.
- Separate query-code generation from file-migration artifact generation; they may share schema input but not public command semantics.

### Dialects And Drivers

Dialect and driver packages are cross-cutting infrastructure.

Triage posture:

- Keep useful capabilities but adapt to spec names and fail-fast semantics.
- Driver transaction gaps should be handled through `transactions.md`.
- File-migration sessions/locking/history should be implemented from `file-migrations-api.md`, `file-migrations-history.md`, and `file-migrations-execution.md`; current query-driver APIs can inform adapters but should not dictate them.

### Migration Kit / File Migrations / Pull

The previous code and issue triage documents were scoped too narrowly by name, but their findings remain useful for the migration-kit slice of the holistic project plan.

Triage posture:

- Use [file-migrations-code-triage.md](./file-migrations-code-triage.md) as the detailed migration-kit code appendix.
- The RC.1 folder-per-migration workflow is still the highest-risk implementation track because it crosses schema input, artifacts, history, execution, pull, CLI, and API boundaries.
- Old flat-file, `meta/`, live-diff, checksum, baseline, and legacy history-table behavior must remain quarantined or superseded.

## Keep / Adapt / Quarantine / Delete Summary

| Classification | Project-wide meaning | Examples |
| --- | --- | --- |
| Keep | Matches current specs or is harmless support infrastructure. | selected tests/fixtures; parts of `internal/testschema`; stable utility behavior |
| Adapt | Useful, but target shape or semantics must be adjusted to specs. | most of `schema/*`, `query/`, `expr/`, `gen/*`, `dialect/`, `driver/*`, `kit/diff.go`, `kit/sqlgen*.go`, `kit/introspect/*` |
| Quarantine | Temporarily reachable legacy behavior, not target precedent. | `kit/apply.go`, `kit/migrate*.go`, current CLI `snapshot`/`diff`/legacy `migrate`/`status` |
| Delete / Replace | Incompatible with target design. Remove or replace when safe. | current public snapshot/diff workflow, current `kit.Snapshot` on-disk artifact model, `_grizzle_migrations` history model |

## Follow-up

Use this document as the project-wide entry point. Use the existing focused migration document as a detailed appendix rather than the only code review:

- [file-migrations-code-triage.md](./file-migrations-code-triage.md)
