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

The previous code and issue triage was initially scoped too narrowly by file-migration nomenclature, but the detailed migration-kit findings are now incorporated directly into this project-wide document.

Triage posture:

- The RC.1 folder-per-migration workflow is still the highest-risk implementation track because it crosses schema input, artifacts, history, execution, pull, CLI, and API boundaries.
- Old flat-file, `meta/`, live-diff, checksum, baseline, and legacy history-table behavior must remain quarantined or superseded.
- Treat the migration-kit section below as the detailed code triage for `kit`, `cmd/grizzle`, schema input, artifact generation, history, execution, and pull.

## Keep / Adapt / Quarantine / Delete Summary

| Classification | Project-wide meaning | Examples |
| --- | --- | --- |
| Keep | Matches current specs or is harmless support infrastructure. | selected tests/fixtures; parts of `internal/testschema`; stable utility behavior |
| Adapt | Useful, but target shape or semantics must be adjusted to specs. | most of `schema/*`, `query/`, `expr/`, `gen/*`, `dialect/`, `driver/*`, `kit/diff.go`, `kit/sqlgen*.go`, `kit/introspect/*` |
| Quarantine | Temporarily reachable legacy behavior, not target precedent. | `kit/apply.go`, `kit/migrate*.go`, current CLI `snapshot`/`diff`/legacy `migrate`/`status` |
| Delete / Replace | Incompatible with target design. Remove or replace when safe. | current public snapshot/diff workflow, current `kit.Snapshot` on-disk artifact model, `_grizzle_migrations` history model |

## Detailed Migration-Kit Code Triage

The following detailed migration-kit triage was previously recorded in a focused file-migrations review artifact and has been merged here so this document is the single project-wide code triage entry point.

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

### Required Follow-up Issues

Before Slice 0 implementation starts, create or update tracking issues for:

1. `Quarantine legacy direct-sync push helpers`
   - paths: `kit/apply.go`, direct-sync docs/examples
   - labels: `file-migrations`, `phase:implementation`, `blocked-by-spec`
   - blocker: dedicated push/direct-sync spec

2. `Quarantine legacy live-diff migrate/status helpers`
   - paths: `kit/migrate.go`, `kit/migrate_mysql.go`, `kit/migrate_sqlite.go`, `cmd/grizzle` migrate/status wiring
   - labels: `file-migrations`, `phase:implementation`, `slice:6`, `slice:8`

3. `Replace legacy snapshot file model`
   - paths: `kit/snapshot.go`, `cmd/grizzle` snapshot/diff wiring, tests that use `schema.snapshot.json`
   - labels: `file-migrations`, `phase:implementation`, `slice:1`, `slice:2`, `slice:8`

4. `Adapt schema definitions and static loader for strict file migrations`
   - paths: `schema/*`, `gen/parser/*`
   - labels: `file-migrations`, `phase:implementation`, `slice:2`

5. `Adapt diff and SQL rendering to RC.1 DDL entities`
   - paths: `kit/diff.go`, `kit/sqlgen*.go`
   - labels: `file-migrations`, `phase:implementation`, `slice:3`, `slice:4`

6. `Adapt introspection for pull and pull --init`
   - paths: `kit/introspect/*`
   - labels: `file-migrations`, `phase:implementation`, `slice:7`

7. `Create missing Slice 0 package/test harness`
   - paths: new `kit/filemigrate` package and internal test fixtures/stores
   - labels: `file-migrations`, `phase:implementation`, `slice:0`

### Exit Criteria Impact

This triage supports the implementation-sequence gate as follows:

- every migration-related code path has an initial classification
- old live-diff/checksum/snapshot behavior is identified as quarantine or replacement work
- the absence of RC.1 artifact/check/generate/migrate internals is explicit
- Slice 0 can start only after the corresponding tracking issues/milestones are created
