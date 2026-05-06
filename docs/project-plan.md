# Grizzle Project Plan

This document combines the project-wide existing-code triage, GitHub issue triage, and forward-looking implementation plan for Grizzle into a single source. Triage findings are preamble; the implementation plan is the forward-looking body. Branch/PR-level handoff context is in [project-handoff.md](./project-handoff.md).

## Contents

- [Source Of Truth](#source-of-truth)
- [Current State](#current-state)
- [Code Triage](#code-triage)
- [Issue Triage](#issue-triage)
- [Plan](#plan)
- [Exit Criteria Before Slice 0](#exit-criteria-before-slice-0)

## Source Of Truth

Use this hierarchy for all classification and implementation decisions:

1. Grizzle specs under `docs/spec/`
2. tagged Drizzle ORM / Drizzle Kit `v1.0.0-rc.1` source for upstream behavior
3. Drizzle docs for the matching release line
4. current repository code only as implementation evidence, never as the target contract
5. open GitHub issues as planning records only

If current code and specs disagree, the code is implementation debt unless the spec is amended. If the specs and tagged Drizzle RC.1 source disagree, the specs must explicitly document the Grizzle decision. If issues conflict with specs, the issue is stale and must be rewritten or superseded.

## Current State

- The spec set covers the planned project capabilities, including schema, query builder, relations, codegen, dialects, transactions, migration kit, file migrations, pull/introspection, CLI/API shape, artifact execution, and implementation sequencing.
- Existing code is a working but partial implementation across several areas. `go test ./...` passes.
- File migrations are the most detailed implementation track because they cross many boundaries and because older issue/PR work attempted an incompatible direction.
- The project still needs holistic backlog normalization, not only file-migration issue cleanup.

# Code Triage

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

Triage posture:

- The RC.1 folder-per-migration workflow is the highest-risk implementation track because it crosses schema input, artifacts, history, execution, pull, CLI, and API boundaries.
- Old flat-file, `meta/`, live-diff, checksum, baseline, and legacy history-table behavior must remain quarantined or superseded.
- See [Detailed Migration-Kit Code Triage](#detailed-migration-kit-code-triage) below for `kit`, `cmd/grizzle`, schema input, artifact generation, history, execution, and pull.

## Keep / Adapt / Quarantine / Delete Summary

| Classification | Project-wide meaning | Examples |
| --- | --- | --- |
| Keep | Matches current specs or is harmless support infrastructure. | selected tests/fixtures; parts of `internal/testschema`; stable utility behavior |
| Adapt | Useful, but target shape or semantics must be adjusted to specs. | most of `schema/*`, `query/`, `expr/`, `gen/*`, `dialect/`, `driver/*`, `kit/diff.go`, `kit/sqlgen*.go`, `kit/introspect/*` |
| Quarantine | Temporarily reachable legacy behavior, not target precedent. | `kit/apply.go`, `kit/migrate*.go`, current CLI `snapshot`/`diff`/legacy `migrate`/`status` |
| Delete / Replace | Incompatible with target design. Remove or replace when safe. | current public snapshot/diff workflow, current `kit.Snapshot` on-disk artifact model, `_grizzle_migrations` history model |

## Detailed Migration-Kit Code Triage

### Investigation Notes

Inventory was taken from the current Go package layout and symbol surface.

Current packages:

- `cmd/grizzle`
- `dialect`
- `driver/pgx`
- `driver/sql`
- `expr`
- `gen/codegen`
- `gen/parser`
- `internal/testschema`
- `kit`
- `kit/introspect`
- `query`
- `schema/mysql`
- `schema/pg`
- `schema/sqlite`

Current validation status:

- `go test ./...` passes.

Important target gaps found in code:

- no `kit/filemigrate` package
- no `schema/ddl` typed DDL-expression package
- no target `Check`, `Generate`, or `Pull` implementation
- no target artifact discovery/loading model
- no RC.1-style `snapshot.json` validator or `DDLEntity` model
- no `LoadedArtifact`, `ArtifactStore`, source store, or resource-limit implementation
- no target migration session, DB lock, or history-table implementation
- no target `__grizzle_migrations` history table code
- no statement-breakpoint execution path

The current implementation is therefore migration-capable only in the legacy/current sense; the RC.1 file-migration workflow has not started in production code.

### Keep

These paths may remain as-is for the file-migration sequence unless later slices discover a concrete integration need.

| Path | Reason | Follow-up |
| --- | --- | --- |
| `query/` | Query builder behavior is separate from file-migration artifact generation/check/execution. | None for Slice 0. |
| `expr/` | Query-expression helpers remain useful, but they are not the target DDL-expression model. | Do not treat this package as `schema/ddl`; add a dedicated DDL expression path when needed. |
| `driver/pgx/` | Current DB wrapper is outside the artifact workflow. | May inform Slice 5 session adapters, but do not couple file migrations directly to query drivers by default. |
| `driver/sql/` | Same as `driver/pgx`; useful general DB abstraction, not the file-migration contract. | May inform Slice 5 session adapters. |
| `gen/codegen/` | Typed query code generation is separate from file-migration artifact work. | No Slice 0 action. Slice 2/7 may need generated metadata or source rendering separately. |
| `internal/testschema/` | Test fixtures can remain. | Reuse only where fixtures match strict file-migration schema input. |

### Keep / Light Adapt

| Path | Reason | Follow-up | Target slice |
| --- | --- | --- | --- |
| `dialect/` | Existing dialect feature matrix is useful and mostly orthogonal to file migrations. | Add migration-session capabilities only when Slice 5 needs transaction, locking, or statement-execution gating. | Slice 5 |

### Adapt

These paths contain useful implementation material, but they require new tests around the target behavior before they are changed or reused.

| Path | Useful current behavior | Required adaptation | Target slice(s) |
| --- | --- | --- | --- |
| `schema/pg/` | Table, column, enum, view, FK, constraint, and rename marker definitions. | Convert supported constructs into strict RC.1-style schema input. Raw string checks, raw view SQL, generated/on-update markers, and unsupported object families must fail or use an explicit trusted/typed DDL path according to the specs. | Slice 2, Slice 4, Slice 7 |
| `schema/mysql/` | MySQL table/column wrappers and dialect aliases. | Preserve useful schema definition surface, but emit/validate only supported RC.1 MySQL snapshot fields. Current embedded `pg.TableDef` model may need a stricter dialect-neutral intermediate representation. | Slice 2, Slice 4, Slice 7 |
| `schema/sqlite/` | SQLite table/column wrappers and rename markers. | Reject or explicitly model Grizzle-only conveniences that are not RC.1 SQLite parity, such as current SQLite `JSONB()` compatibility behavior. Add SQLite-specific supported field validation. | Slice 2, Slice 4, Slice 7 |
| `gen/parser/` | Static AST scanning of simple `pg/mysql/sqlite.Table` declarations and column builder chains. | Replace or harden into the file-migration schema loader: canonical no-follow paths, resource limits, regular files only, bounded AST traversal, strict unsupported construct diagnostics, source redaction, and support for required schema objects beyond simple tables. | Slice 0, Slice 2 |
| `kit/diff.go` | Deterministic in-memory diff ordering, rename handling, constraints, views, enums, and FK create ordering. | Port useful logic to RC.1 `ddl` entity snapshots and `check`/`generate` graph context. Do not let legacy `kit.Snapshot` define the target model. | Slice 2, Slice 3, Slice 4 |
| `kit/sqlgen.go` | PostgreSQL SQL rendering for current `Change` model. | Adapt rendering behind validated typed DDL/diff statements. Generated multi-statement artifacts must use full-line statement breakpoints. Errors must use stable redacted diagnostics. | Slice 4, Slice 6 |
| `kit/sqlgen_mysql.go` | MySQL SQL rendering for current `Change` model. | Same as PostgreSQL, with MySQL-specific RC.1 field support and transaction capability handling. | Slice 4, Slice 6 |
| `kit/sqlgen_sqlite.go` | SQLite SQL rendering for current `Change` model. | Same as above. Current comment-stub handling for unsupported SQLite ALTER operations must not be silently treated as applied target behavior. | Slice 4, Slice 6 |
| `kit/introspect/` | PostgreSQL/MySQL/SQLite live schema introspection foundations. | Convert into `pull` introspection adapters with strict object-family handling, source rendering, resource limits, broad-scan opt-in, secret-literal checks, redacted summaries, and managed bootstrap artifact planning. | Slice 7 |
| `kit/*_test.go` | Good regression seeds for rename detection, deterministic ordering, SQL rendering, views/enums, and SQLite behavior. | Split old-behavior tests from target tests. Keep low-level algorithm fixtures where still valid; rewrite old snapshot/live-diff expectations around RC.1 artifact snapshots and file-migration command boundaries. | All relevant slices |
| `kit/introspect/*_test.go` | Existing introspection helper coverage. | Reframe around target `pull` adapters, redaction, and supported object families. | Slice 7 |

#### Special Adaptation: Current Snapshot Assembly Helpers

The following concepts from `kit/snapshot.go` are useful but must not define the artifact format:

- `FromDefs`
- `FromSchema`
- `SchemaObjects`

They may be replaced by or adapted into a strict schema-input-to-RC.1-DDL snapshot planner.

The target public snapshot lives in `kit/filemigrate` and follows the envelope defined by [file-migrations-artifacts.md](./spec/file-migrations-artifacts.md):

```json
{
  "version": "<dialect-version>",
  "dialect": "<dialect-id>",
  "id": "<uuid>",
  "prevIds": ["<uuid>", "..."],
  "ddl": [],
  "renames": []
}
```

The file-migration API spec explicitly says the target `filemigrate.Snapshot` must not reuse current `kit.Snapshot`.

### Quarantine

These paths may remain temporarily reachable for compatibility while new internals are built, but each needs a cutover/removal issue before implementation begins.

| Path | Legacy behavior | Why quarantined | Cutover direction | Target slice |
| --- | --- | --- | --- | --- |
| `kit/apply.go` | `Push` / `DryRun` introspect a live DB, diff against schema definitions, and optionally apply generated DDL. | Direct-sync `push` is outside the file-migration sequence until a dedicated push spec exists. It must not feed RC.1 artifact generation or execution. | Keep behind legacy/direct-sync boundary. Do not expose from `kit/filemigrate`. | Deferred / blocked-by-push-spec |
| `kit/migrate.go` | PostgreSQL `Migrate` live-diffs DB state, applies generated SQL, and records a checksum/sql-batch history row. | Target `migrate` applies committed migration artifact directories only, runs `check`, and records rows by artifact `name`. | Replace command meaning only after Slice 6 is complete; final CLI cutover in Slice 8. | Slice 6, Slice 8 |
| `kit/migrate_mysql.go` | MySQL variant of legacy live-diff migrate/status/history. | Same incompatibility as PostgreSQL path. | Same cutover/removal path. | Slice 6, Slice 8 |
| `kit/migrate_sqlite.go` | SQLite variant of legacy live-diff migrate/status/history, including comment-stub skips. | Same incompatibility as PostgreSQL path; current unsupported operation handling is especially unsafe as a target precedent. | Same cutover/removal path. | Slice 6, Slice 8 |
| `cmd/grizzle` legacy migration command paths | Current CLI exposes `snapshot`, `diff`, legacy `migrate`, and `status`. | Target CLI is `generate`, `check`, `migrate`, `push`, `pull`, and `introspect`. Public `snapshot` and `diff` are explicitly omitted. | Leave current CLI untouched until internal target workflow is ready; rewire in Slice 8. | Slice 8 |
| `cmd/grizzle runSQL` | Prints fresh-database CREATE SQL from schema definitions. | Useful utility, but not part of the target file-migration workflow. | Keep separate from file-migration public workflow, or defer decision to cleanup. | Slice 8 / deferred |

### Delete / Replace

These are not necessarily immediate deletion tasks. They are target-design decisions: this behavior must not survive as the RC.1 file-migration implementation.

| Item | Current behavior | Replacement |
| --- | --- | --- |
| Public `grizzle snapshot` | Writes current `kit.Snapshot` JSON to `schema.snapshot.json`. | No public target command. Snapshot creation is internal to `generate` / `pull` artifact writing. |
| Public `grizzle diff` | Diffs current schema against `schema.snapshot.json` and prints SQL. | No public target command. Diff planning is internal to `generate` and `check`. |
| `kit.Snapshot` as an on-disk artifact shape | Current JSON has `version`, `created_at`, `tables`, `views`, and `enums`. | RC.1 artifact snapshot has `version`, `dialect`, `id`, `prevIds`, `ddl`, and `renames`. |
| `kit.SaveJSON` / `kit.LoadJSON` as public migration snapshot workflow | Reads/writes legacy snapshot files. | Replace with strict `snapshot.json` artifact loading/validation under `kit/filemigrate`. |
| `_grizzle_migrations` | Current legacy history table name. | Target default table is `__grizzle_migrations`; PostgreSQL default schema is `grizzle`. |
| `checksum`, `sql_batch`, `description` history columns | Stores generated SQL batch metadata for live diffs. | Target logical columns are `id`, `hash`, `created_at`, `name`, and `applied_at`. |
| `ChecksumSQL(stmts []string)` as migration identity/hash | Hashes generated statement strings with separators. | Target `hash` is SHA-256 over exact raw `migration.sql` file bytes. Pending detection is by migration `name`, not hash. |
| Legacy `migrate` meaning | Introspect live DB, diff against schema definitions, apply generated DDL. | Target `migrate` reads validated artifact directories, skips applied names, executes pending `migration.sql` segments, and records history. |

### Slice Mapping Summary

| Slice | Existing code involved | Initial triage action |
| --- | --- | --- |
| Slice 0: Package boundary and test harness | none directly; may use `dialect` types later | Create new `kit/filemigrate` package, diagnostics, resource limits, test stores. Keep legacy `kit` command paths isolated. |
| Slice 1: Artifact discovery and offline validation | none currently | Build new artifact loader/validator; do not reuse current `kit.LoadJSON`. |
| Slice 2: Snapshot and schema input planning | `schema/*`, `gen/parser`, `kit/snapshot.go` concepts | Adapt schema definitions and loader into strict RC.1 snapshot planning. Replace current snapshot model. |
| Slice 3: `check` | `kit/diff.go` concepts | Adapt graph/diff logic only after RC.1 snapshot validators exist. |
| Slice 4: `generate` | `kit/diff.go`, `kit/sqlgen*.go`, `schema/*`, `gen/parser` | Generate folder-per-migration artifacts with `migration.sql`, `snapshot.json`, names, renames, and breakpoints. |
| Slice 5: History, locking, sessions | `driver/*`, `dialect` maybe | Build new sessions/history/locking; do not adapt old history tables except as negative tests. |
| Slice 6: `migrate` | `kit/migrate*.go` only as negative precedent | Implement artifact execution from new internals. Quarantine legacy live-diff migrate until cutover. |
| Slice 7: `pull` / `pull --init` | `kit/introspect/*`, `schema/*`, `gen/codegen` maybe | Adapt introspection into managed source/bootstrap artifact planning with redaction and limits. |
| Slice 8: CLI cutover and cleanup | `cmd/grizzle/main.go`, legacy `kit` APIs | Rewire public commands and remove/deprecate old public `snapshot`, `diff`, legacy `migrate`, and `status` meanings. |

# Issue Triage

Issue inventory was pulled from GitHub on 2026-05-06. The repository had 75 open issues.

The goal is not to force every issue into file migrations. The goal is to prevent any issue from accidentally implementing behavior that conflicts with the specs.

## Project-Wide Buckets

| Bucket | Issues | Triage |
| --- | --- | --- |
| Migration kit / file migrations / pull | #153, #154, #157, #158, #169, #273, #274, #275, #276, #277, #278, #279, #280 | Normalize before Slice 0; detailed issue triage is below. |
| Schema DSL / type system / codegen | #172, #183, #216, #222, #234, #235, #236, #248, #253, #254, #259 | Map to `schema.md`, `types.md`, `codegen.md`, and file-migration unsupported-field rules where relevant. |
| Kit diff / SQL generation / introspection | #79, #82, #137, #225, #226, #240, #243, #244, #249, #250 | Some feed pull/file-migration slices; sequence support is blocked/deferred by initial unsupported-object-family rules. |
| Query builder / expressions / relations | #33, #81, #113, #128, #134, #139, #140, #141, #144, #162, #163, #164, #167, #171, #197, #203, #232, #233, #237, #263, #264, #271 | Work from `query-builder.md`, `relations.md`, `dialects.md`, and `types.md`; not a file-migration Slice 0 gate. |
| Drivers / transactions / prepared statements | #42, #88, #143, #159, #160, #166, #170, #223, #252, #267, #268 | Work from `transactions.md` and `query-builder.md`; file-migration sessions should reference but not wait on user-facing driver features unless required. |
| Docs / repo hygiene / release | #74, #135, #175, #221, #224, #228, #258, #262 | Normal backlog; not a file-migration gate except where specs must be corrected before code. |

## Immediate Normalization Priorities

### P0: Prevent old migration direction from leaking into implementation

Close, supersede, or rewrite issues that encode discarded behavior:

- #154 — old flat-file/live-diff/checksum/baseline migrate plan
- #153 — old `meta/snapshot.json` plus flat numbered SQL generate plan
- #273, #275, #280 — semicolon SQL splitter issues; target uses statement breakpoints
- #274, #276, #277, #278 — useful concerns but current bodies are tied to old PR #272 behavior and need rewriting or deferral

### P1: Project-Wide Parent Issues (Canonical)

Create parent issues/milestones for the major spec areas. This is the canonical list of project-wide (workstream-level) parent issues. Per-slice parents for the file-migration sequence live in [Per-Slice Parent Issues](#per-slice-parent-issues).

1. Schema DSL parity and strict schema input
2. Query builder parity and dialect gating
3. Codegen parity and type mapping
4. Driver/transaction/prepared statement parity
5. Dialect feature matrix and capability gating
6. Migration kit/file-migration workflow slices 0-8
7. Pull/introspection/source-generation workflow
8. Docs/spec synchronization and release policy

### P2: Apply labels/project fields consistently

Recommended fields:

- `area:schema`
- `area:query`
- `area:codegen`
- `area:driver`
- `area:dialect`
- `area:kit`
- `area:file-migrations`
- `area:pull`
- `phase:spec`
- `phase:implementation`
- `blocked-by-spec`
- `superseded`

For file-migration-specific work, keep `slice:0` through `slice:8`.

## Area Notes

### Schema / Types / Codegen

Issues #234, #235, #236, and #259 are near-term correctness/parity items. They should be evaluated against `schema.md`, `types.md`, and `codegen.md`, not delayed just because they are outside the file-migration slice names.

Issues #172, #253, and #254 should be treated carefully: generated columns are recognized in RC.1 snapshots but intentionally unsupported in the initial file-migration target. The first file-migration implementation should add negative validation for generated columns, not partial support through the old `kit.Snapshot` model.

Sequence issues #137, #248, #249, and #250 are similar. They remain future schema/kit backlog unless the specs are amended to include sequences in the initial artifact graph.

### Query / Relations

Query issues should continue independently against `query-builder.md` and `relations.md`. They are part of the holistic project plan and should not be hidden behind file-migration sequencing.

Important correctness issues include dialect gating (#264, #271), missing operators (#164), conflict/update APIs (#33, #162), and codegen alias/self-join support (#113).

### Drivers / Transactions / Prepared Statements

Driver and transaction issues are cross-cutting. They should not block file-migration Slice 0, but Slice 5 migration sessions should consult these issues to avoid duplicating incompatible abstractions.

Prepared statement work (#166, #252, #88, #223) belongs to query/driver parity, not migration-kit implementation.

### Migration Kit / Pull / File Migrations

The key correction is that `push` does not block `pull`, and `generate`/`check`/`migrate` must follow the ratified artifact workflow rather than the older flat-file transition issues.

## Detailed Migration-Kit Issue Triage

### Classification Terms

- `Map`: useful issue, broadly aligned with the ratified specs after label/milestone assignment.
- `Rework`: useful intent, but acceptance criteria, blockers, public shape, or sequencing conflict with the ratified specs.
- `Supersede`: issue tracks an old checksum/live-diff/flat-file/meta-direction and should be closed or replaced by a slice-specific issue.
- `Defer`: valid backlog, but outside the initial file-migration implementation sequence.
- `Blocked by spec`: no implementation should start until a dedicated spec or amendment exists.
- `Outside file migrations`: leave in the normal backlog; do not use it to gate Slice 0.

### High-Level Findings

- The open issue backlog still contains pre-spec file-migration issues whose acceptance criteria conflict with the ratified RC.1-style folder-per-migration design.
- The most important issue to neutralize is [#154](https://github.com/sofired/grizzle/issues/154), because it describes the discarded `kit.Migrate` direction: flat `.sql` files, `_grizzle_migrations`, `tag`, `is_baseline`, schema upgrade flags, and checksum/live-diff lineage.
- [#153](https://github.com/sofired/grizzle/issues/153) tracks the right command name (`generate`) but the wrong artifact model (`meta/snapshot.json` plus numbered flat SQL files). It should not become the Slice 4 implementation issue without being rewritten.
- Splitter issues from PR #272 ([#273](https://github.com/sofired/grizzle/issues/273), [#275](https://github.com/sofired/grizzle/issues/275), [#280](https://github.com/sofired/grizzle/issues/280)) are old-direction issues. The target execution model uses explicit `--> statement-breakpoint` segmentation, not semicolon splitting.
- `push`, sequence support, generated column support, non-interactive rename answers, and applied hash-drift reporting are valid backlog areas, but they are not Slice 0 blockers.

### Direct File-Migration Issues

| Issue | Current title | Triage | Recommended action | Target slice / label guidance |
| --- | --- | --- | --- | --- |
| [#153](https://github.com/sofired/grizzle/issues/153) | `feat: grizzle generate command — write SQL migration files to disk` | Rework / likely supersede | Keep the command intent, but replace the body. Current acceptance criteria use `migrations/meta/snapshot.json`, numbered flat `.sql` files, and an old `kit.Generate` shape. The target is folder-per-migration artifacts with `migration.sql` and `snapshot.json`, pre-write `check`, RC.1 snapshot envelope, `prevIds`, and breakpoints. | `area:file-migrations`, `phase:implementation`, `slice:4`; blocked by Slices 0-3 |
| [#154](https://github.com/sofired/grizzle/issues/154) | `feat: refactor kit.Migrate to file-based workflow (DEVIATION:BROKEN)` | Supersede | Close or mark superseded. It encodes the discarded transition plan: `_grizzle_migrations`, `tag`, `is_baseline`, automatic schema upgrade, `--baseline`, `--skip-schema-upgrade`, and flat `.sql` application. Create a new Slice 6 parent for artifact-based `migrate` instead. | `superseded`; replacement should be `area:file-migrations`, `phase:implementation`, `slice:6`, blocked by Slices 0-5 |
| [#169](https://github.com/sofired/grizzle/issues/169) | `feat: grizzle check command — validate migrations directory consistency` | Rework | Rewrite as the Slice 3 `check` issue. Current body says `check` validates current schema and optionally DB state; target `check` is offline artifact/snapshot/graph validation and must ignore DB credentials. | `area:file-migrations`, `phase:implementation`, `slice:3`; blocked by Slices 0-2 |
| [#158](https://github.com/sofired/grizzle/issues/158) | `feat: grizzle pull command (introspect live DB → Go schema definitions)` | Rework | Rewrite as Slice 7. Current issue uses direct `--db <dsn>` shape, omits managed bootstrap artifacts, broad-scan opt-in, redaction, source/artifact stores, and `pull --init`. Its stated dependency on `push` is obsolete. | `area:file-migrations`, `phase:implementation`, `slice:7`; blocked by Slices 0-6 |
| [#157](https://github.com/sofired/grizzle/issues/157) | `feat: grizzle push CLI command` | Defer / blocked by spec | Keep as backlog only. `push` is a public command boundary, but implementation-ready behavior needs a dedicated direct-sync spec. It must not block `pull`, `check`, or `generate`. | `blocked-by-spec`; optionally `area:file-migrations` only as command-boundary backlog, not `phase:implementation` |
| [#277](https://github.com/sofired/grizzle/issues/277) | `Support non-transactional DDL in PostgreSQL file-based migrations` | Rework | Map the need into Slice 6 execution semantics. Do not adopt the issue's proposed `-- grizzle:no-transaction` header unless the execution spec is amended. The initial design should follow dialect transaction/partial-application rules in `file-migrations-execution.md`. | `area:file-migrations`, `phase:implementation`, `slice:6`; maybe `spec-required` if new per-artifact transaction controls are desired |
| [#276](https://github.com/sofired/grizzle/issues/276) | `kit.Migrate: add MySQL and PostgreSQL integration tests for file-based workflow` | Rework | Useful integration-test intent, but current body references PR #272 concepts: schema upgrade, baseline, skip-schema-upgrade. Rewrite after target history/session/execution semantics exist. | `area:file-migrations`, `phase:implementation`, `slice:5`, `slice:6`, `testing` |
| [#274](https://github.com/sofired/grizzle/issues/274) | `PostgreSQL integration tests for file-based kit.Migrate` | Supersede (duplicate of #276) | Close #274 in favor of rewriting #276 as the consolidated Slice 6 integration-test parent. Do not rewrite #274 separately. | `superseded`; superseded by rewritten #276 |
| [#278](https://github.com/sofired/grizzle/issues/278) | `Detect checksum drift on already-applied migrations` | Defer / rework | The history spec intentionally follows name-based pending detection and says `migrate` must not add a default applied-hash-drift blocker. Keep only as future audit/status tooling, not as initial `migrate` behavior. | `area:file-migrations`, `blocked-by-spec` or future audit backlog; not Slice 6 default behavior |
| [#279](https://github.com/sofired/grizzle/issues/279) | `Track upstream Drizzle non-interactive generate rename/conflict resolution` | Defer | Already matches spec posture. Keep as future enhancement. Initial `generate` uses RC.1-style interactive prompts and omits non-interactive answer files. | `blocked-by-spec`, `enhancement-candidate`; not initial slices |
| [#273](https://github.com/sofired/grizzle/issues/273) | `Support dollar-quoted PL/pgSQL blocks in migration file SQL splitter` | Supersede | Old semicolon-splitter issue. Target execution splits by full-line statement breakpoints, not SQL tokenization. Close/supersede unless a future disabled-breakpoints parser is explicitly designed. | `superseded`; future work would be `blocked-by-spec` |
| [#275](https://github.com/sofired/grizzle/issues/275) | `kit.Migrate: handle semicolons in string literals and PL/pgSQL blocks` | Supersede | Same as #273. This should not drive Slice 6. | `superseded`; future parser work blocked by spec |
| [#280](https://github.com/sofired/grizzle/issues/280) | `return error from splitSQLStatements for unterminated constructs` | Supersede | Same splitter family as #273/#275. The target should not have this helper on the command path. | `superseded`; future parser work blocked by spec |

### Migration-Adjacent Issues To Adapt Or Defer

These are not all initial file-migration blockers, but they touch schema loading, diffing, introspection, or dialect execution and should be mapped before coding starts.

| Issue | Area | Triage | Recommended action | Slice guidance |
| --- | --- | --- | --- | --- |
| [#244](https://github.com/sofired/grizzle/issues/244) | `kit/diff` default comparison | Map / adapt | Keep as a useful regression for the old diff engine and for future schema/snapshot comparison logic. | Slice 2 / Slice 3 |
| [#243](https://github.com/sofired/grizzle/issues/243) | default-expression normalization | Defer / adapt | Low-priority unless the strict schema loader or RC.1 fixtures can emit dollar-quoted defaults. If kept, acceptance should reference validated snapshot/default rendering. | Slice 2 / Slice 3 or deferred |
| [#240](https://github.com/sofired/grizzle/issues/240) | view dependency ordering | Rework / possibly spec-required | Useful if RC.1-compatible dependency information is available. Do not implement by ad hoc SQL parsing unless specified. | Slice 3 / Slice 4; add `spec-required` if design is unclear |
| [#259](https://github.com/sofired/grizzle/issues/259) | unsafe/raw numeric defaults | Map | Relevant to strict schema input and literal validation. Acceptance should cite the file-migration literal/default rules rather than only current builder behavior. | Slice 2 |
| [#226](https://github.com/sofired/grizzle/issues/226) | introspection FK action fallback | Rework | Target behavior should fail or diagnose unsupported introspection values; a warning-only fallback is too weak for strict file migrations. | Slice 7, maybe Slice 2 validation fixtures |
| [#225](https://github.com/sofired/grizzle/issues/225) | PostgreSQL FK introspection integration test | Map | Useful for `pull`/introspection validation. Should run under integration-test infrastructure. | Slice 7, testing |
| [#79](https://github.com/sofired/grizzle/issues/79) | schema-qualified FK introspection | Map | Useful correctness issue for `pull` and live introspection adapters. | Slice 7 |
| [#82](https://github.com/sofired/grizzle/issues/82) | FK ordinal dead/unused fields | Map / defer | Pair with #225. Not a Slice 0 blocker. | Slice 7 or normal tech-debt backlog |
| [#183](https://github.com/sofired/grizzle/issues/183) | dialect methods on MySQL/SQLite table defs | Outside / optional support | Low-risk schema test. Useful for dialect-agnostic schema input confidence, but not a file-migration gate. | Optional Slice 2 support test |
| [#160](https://github.com/sofired/grizzle/issues/160) | MySQL/SQLite transaction wrappers | Outside / maybe inform | User-facing transaction wrappers are separate. Slice 5 migration sessions may use `database/sql` directly and should not wait on this. | Not a gate; maybe Slice 5 reference |
| [#159](https://github.com/sofired/grizzle/issues/159) | transaction isolation/access modes | Outside / maybe inform | Not required for initial file-migration sessions unless the execution spec requires configurable isolation. | Not a gate |

### Unsupported Initial Object Families / Features

The ratified file-migration specs intentionally treat these as unsupported in the initial target. Existing issues can remain as backlog, but they should not be implemented into `kit.Snapshot`, `Diff`, SQL generation, or `pull` before the initial RC.1-supported field matrix is in place.

| Issue | Feature | Triage | Recommended action |
| --- | --- | --- | --- |
| [#172](https://github.com/sofired/grizzle/issues/172) | generated columns umbrella | Defer / blocked by spec | Initial file migrations must validate recognized generated-column fields and fail `unsupported_feature`. Implementation support needs a later spec decision. |
| [#253](https://github.com/sofired/grizzle/issues/253) | generated-column DSL/diff/sqlgen | Defer / blocked by spec | Do not wire into old `kit/diff` as initial file-migration work. Replace with negative validation tests first. |
| [#254](https://github.com/sofired/grizzle/issues/254) | generated-column introspection/codegen | Defer / blocked by spec | Initial `pull` should detect and reject/report unsupported generated columns according to spec. |
| [#137](https://github.com/sofired/grizzle/issues/137) | PostgreSQL sequence support umbrella | Defer / blocked by spec | PostgreSQL standalone sequences are unsupported object families in the initial artifact graph. |
| [#248](https://github.com/sofired/grizzle/issues/248) | sequence schema DSL | Defer | Schema DSL-only work may be future backlog, but it must not imply initial artifact support. |
| [#249](https://github.com/sofired/grizzle/issues/249) | sequence support in `Snapshot`, `Diff`, SQL generation | Supersede / blocked by spec | Current acceptance criteria are explicitly tied to old `kit.Snapshot`. Rewrite later against RC.1 DDL entities if sequence support is approved. |
| [#250](https://github.com/sofired/grizzle/issues/250) | sequence introspection | Defer / blocked by spec | Initial `pull` should reject unsupported object families rather than silently dropping or serializing sequences. |

### Issues That Should Not Gate File-Migration Slice 0

These open issues are valuable normal backlog work, but they are outside the initial file-migration sequencing gate unless the project owner explicitly adds them to a slice.

| Area | Issues |
| --- | --- |
| Query builder / expression behavior | [#271](https://github.com/sofired/grizzle/issues/271), [#264](https://github.com/sofired/grizzle/issues/264), [#263](https://github.com/sofired/grizzle/issues/263), [#237](https://github.com/sofired/grizzle/issues/237), [#233](https://github.com/sofired/grizzle/issues/233), [#232](https://github.com/sofired/grizzle/issues/232), [#203](https://github.com/sofired/grizzle/issues/203), [#197](https://github.com/sofired/grizzle/issues/197), [#171](https://github.com/sofired/grizzle/issues/171), [#167](https://github.com/sofired/grizzle/issues/167), [#164](https://github.com/sofired/grizzle/issues/164), [#163](https://github.com/sofired/grizzle/issues/163), [#162](https://github.com/sofired/grizzle/issues/162), [#144](https://github.com/sofired/grizzle/issues/144), [#141](https://github.com/sofired/grizzle/issues/141), [#140](https://github.com/sofired/grizzle/issues/140), [#139](https://github.com/sofired/grizzle/issues/139), [#134](https://github.com/sofired/grizzle/issues/134), [#128](https://github.com/sofired/grizzle/issues/128), [#113](https://github.com/sofired/grizzle/issues/113), [#81](https://github.com/sofired/grizzle/issues/81), [#33](https://github.com/sofired/grizzle/issues/33) |
| Driver/prepared statement backlog | [#268](https://github.com/sofired/grizzle/issues/268), [#267](https://github.com/sofired/grizzle/issues/267), [#252](https://github.com/sofired/grizzle/issues/252), [#223](https://github.com/sofired/grizzle/issues/223), [#166](https://github.com/sofired/grizzle/issues/166), [#170](https://github.com/sofired/grizzle/issues/170), [#143](https://github.com/sofired/grizzle/issues/143), [#88](https://github.com/sofired/grizzle/issues/88), [#42](https://github.com/sofired/grizzle/issues/42) |
| Codegen/schema parity not specific to file migrations | [#236](https://github.com/sofired/grizzle/issues/236), [#235](https://github.com/sofired/grizzle/issues/235), [#234](https://github.com/sofired/grizzle/issues/234), [#222](https://github.com/sofired/grizzle/issues/222), [#216](https://github.com/sofired/grizzle/issues/216) |
| Docs/release/repo hygiene | [#262](https://github.com/sofired/grizzle/issues/262), [#258](https://github.com/sofired/grizzle/issues/258), [#228](https://github.com/sofired/grizzle/issues/228), [#224](https://github.com/sofired/grizzle/issues/224), [#221](https://github.com/sofired/grizzle/issues/221), [#175](https://github.com/sofired/grizzle/issues/175), [#135](https://github.com/sofired/grizzle/issues/135), [#74](https://github.com/sofired/grizzle/issues/74) |

### Per-Slice Parent Issues

This is the canonical list of per-slice parent issues for the file-migration sequence. It complements the project-wide workstream parents in [P1](#p1-project-wide-parent-issues-canonical).

The existing backlog does not cleanly provide one parent issue per implementation slice. Before coding Slice 0, create or rewrite parent issues as follows:

| Slice | Existing issue candidate | Required action |
| --- | --- | --- |
| Slice 0: package boundary/test harness | none | Create new parent issue. |
| Slice 1: artifact discovery/offline validation core | none | Create new parent issue. |
| Slice 2: snapshot/schema input planning | partial: #259, #172/#253/#254 negative cases | Create parent issue; link migration-adjacent issues as children or blockers. |
| Slice 3: `check` | #169 | Rewrite #169 or create replacement. |
| Slice 4: `generate` | #153 | Rewrite #153 or close and create replacement. |
| Slice 5: history/locking/sessions | none | Create new parent issue. Old #154 history criteria must not be reused. |
| Slice 6: `migrate` | #154, #277, #276/#274 | Close/supersede #154; create replacement and link reworked execution/test issues. |
| Slice 7: `pull` / `pull --init` | #158 plus #79/#82/#225/#226 | Rewrite #158 and link introspection issues. |
| Slice 8: CLI cutover/cleanup | none | Create new parent issue for public command rewiring and legacy command cleanup. |

### Label Recommendations

Apply labels or equivalent project fields after issue rewriting:

- `area:file-migrations`
- `phase:implementation`
- `slice:0` through `slice:8`
- `spec-required` for view dependency ordering or transaction-control extensions if the current specs are insufficient
- `blocked-by-spec` for `push`, generated-column implementation, sequence implementation, non-interactive rename answers, disabled-breakpoint SQL parsing, and hash-drift enforcement
- `superseded` for old flat-file/meta/checksum/live-diff issues

### Immediate Cleanup Recommendations

1. Close or mark superseded:
   - [#154](https://github.com/sofired/grizzle/issues/154)
   - [#273](https://github.com/sofired/grizzle/issues/273)
   - [#275](https://github.com/sofired/grizzle/issues/275)
   - [#280](https://github.com/sofired/grizzle/issues/280)
   - [#274](https://github.com/sofired/grizzle/issues/274) — close as duplicate of rewritten #276

2. Rewrite or replace before use:
   - [#153](https://github.com/sofired/grizzle/issues/153)
   - [#169](https://github.com/sofired/grizzle/issues/169)
   - [#158](https://github.com/sofired/grizzle/issues/158)
   - [#276](https://github.com/sofired/grizzle/issues/276) — rewrite as consolidated Slice 6 integration-test parent (absorbs #274)
   - [#277](https://github.com/sofired/grizzle/issues/277)

3. Defer as backlog / blocked by spec:
   - [#157](https://github.com/sofired/grizzle/issues/157)
   - [#172](https://github.com/sofired/grizzle/issues/172), [#253](https://github.com/sofired/grizzle/issues/253), [#254](https://github.com/sofired/grizzle/issues/254)
   - [#137](https://github.com/sofired/grizzle/issues/137), [#248](https://github.com/sofired/grizzle/issues/248), [#249](https://github.com/sofired/grizzle/issues/249), [#250](https://github.com/sofired/grizzle/issues/250)
   - [#278](https://github.com/sofired/grizzle/issues/278)
   - [#279](https://github.com/sofired/grizzle/issues/279)

# Plan

## Global Rules

- Specs in `docs/spec/` are authoritative for Grizzle behavior.
- Tagged Drizzle ORM / Drizzle Kit `v1.0.0-rc.1` source is authoritative for upstream parity details.
- Open issues and current code must be reconciled to specs before implementation.
- If a planned behavior is not specified, write or amend the spec before coding.
- Keep implementation slices small enough to test and revert.

## Workstreams

The "Planning action" column summarizes intent. The canonical lists of parent issues to create are [P1: Project-Wide Parent Issues](#p1-project-wide-parent-issues-canonical) (workstream-level) and [Per-Slice Parent Issues](#per-slice-parent-issues) (file-migration slices).

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

## Required Code Follow-up Issues

Code-area tracking issues for migration-kit follow-up work. Each should hang off the relevant slice or workstream parent in [P1](#p1-project-wide-parent-issues-canonical) or [Per-Slice Parent Issues](#per-slice-parent-issues). Before Slice 0 implementation starts, create or update tracking issues for:

1. `Quarantine legacy direct-sync push helpers`
   - paths: `kit/apply.go`, direct-sync docs/examples
   - labels: `area:file-migrations`, `phase:implementation`, `blocked-by-spec`
   - blocker: dedicated push/direct-sync spec

2. `Quarantine legacy live-diff migrate/status helpers`
   - paths: `kit/migrate.go`, `kit/migrate_mysql.go`, `kit/migrate_sqlite.go`, `cmd/grizzle` migrate/status wiring
   - labels: `area:file-migrations`, `phase:implementation`, `slice:6`, `slice:8`

3. `Replace legacy snapshot file model`
   - paths: `kit/snapshot.go`, `cmd/grizzle` snapshot/diff wiring, tests that use `schema.snapshot.json`
   - labels: `area:file-migrations`, `phase:implementation`, `slice:2`, `slice:8`

4. `Adapt schema definitions and static loader for strict file migrations`
   - paths: `schema/*`, `gen/parser/*`
   - labels: `area:file-migrations`, `phase:implementation`, `slice:2`

5. `Adapt diff and SQL rendering to RC.1 DDL entities`
   - paths: `kit/diff.go`, `kit/sqlgen*.go`
   - labels: `area:file-migrations`, `phase:implementation`, `slice:3`, `slice:4`

6. `Adapt introspection for pull and pull --init`
   - paths: `kit/introspect/*`
   - labels: `area:file-migrations`, `phase:implementation`, `slice:7`

7. `Create missing Slice 0 package/test harness`
   - paths: new `kit/filemigrate` package and internal test fixtures/stores
   - labels: `area:file-migrations`, `phase:implementation`, `slice:0`

# Exit Criteria Before Slice 0

A holistic backlog is ready for Slice 0 implementation only when all of the following are true. Each item is intended to be checkable mechanically against GitHub state plus this document.

Code-side gate (from [Code Triage](#code-triage)):

- every migration-related code path has an initial classification in [Package-Level Classification](#package-level-classification) and the detailed migration-kit tables
- old live-diff/checksum/snapshot behavior is identified as Quarantine or Delete/Replace
- the absence of RC.1 artifact/check/generate/migrate internals is explicit
- the seven [Required Code Follow-up Issues](#required-code-follow-up-issues) exist as tracking issues, each linked to its slice or workstream parent

Issue-side gate (from [Issue Triage](#issue-triage)):

- old-direction issues are closed, marked `superseded`, or rewritten (per [Immediate Cleanup Recommendations](#immediate-cleanup-recommendations))
- one parent issue exists for every slice 0 through 8 (per [Per-Slice Parent Issues](#per-slice-parent-issues))
- one parent issue exists for every project-wide workstream (per [P1: Project-Wide Parent Issues](#p1-project-wide-parent-issues-canonical))
- child issues are mapped to the relevant slice or workstream parent
- direct-sync `push` and unsupported object-family work are visibly deferred or `blocked-by-spec`
- no open issue body remains the de facto source of truth when it conflicts with `docs/spec/*` or Drizzle RC.1 source

Spec-side gate:

- specs are updated before implementation for any `blocked-by-spec` work
- if the [Workstreams](#workstreams) table cites a planned behavior that is not specified, the relevant spec is amended before coding starts
