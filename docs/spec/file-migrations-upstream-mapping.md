# File Migrations Upstream Mapping

## Status

Draft

## Purpose

Map Drizzle ORM / Drizzle Kit behavior into concrete Grizzle design inputs.

Pinned upstream target:

- Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`

This document records upstream mapping:

- identify the upstream behavior
- decide whether Grizzle should copy it directly, adapt it, or intentionally diverge
- point the later spec documents at the relevant upstream source

## Mapping Legend

- `Copy`: Grizzle should match Drizzle closely.
- `Adapt`: Grizzle should preserve the same behavior or workflow intent, but with Go-specific or Grizzle-specific implementation differences.
- `Diverge`: Grizzle should intentionally do something different; the reason must be documented in the target spec.

## Workflow Mapping

| Concern | Drizzle upstream | Grizzle direction | Classification | Notes |
| --- | --- | --- | --- | --- |
| Command model | Drizzle registers/documents `generate`, `migrate`, `push`, `pull`, `check`, `studio`, `up`, and `export` | Grizzle copies the non-`up` file-migration command roles (`generate`, `migrate`, `push`, `pull`, `check`) but does not include public `grizzle studio`, `grizzle up`, or `grizzle export` in the initial target RC.1-aligned workflow | Adapt | `studio`, `up`, and `export` are intentionally excluded in [kit.md](./kit.md); `push` remains a public boundary only until a direct-sync spec exists; `export` and `studio` require dedicated future specs |
| Two-step file workflow | Drizzle treats `generate` then `migrate` as the code-first migration flow | Grizzle should preserve this as the primary file-based path | Copy | `migrate` should not be repurposed before `generate` exists |
| Validation step | Drizzle has `check` for migration-history consistency / collisions | Grizzle should define a `check` equivalent before finalizing the file-based workflow | Copy | Details may adapt, but the workflow role is upstream-backed |
| Direct schema application | Drizzle has `push` as a distinct shortcut | Grizzle should retain a clear distinction between generated-file workflow and direct-apply workflow | Copy | Important for user expectations and docs |
| Introspection | Drizzle has `pull` | Grizzle should keep `pull` as a separate workflow with a dedicated specification, adapted to Go-native schema output layout | Adapt | Existing Grizzle internals differ, but workflow role is the same |
| Version target | Drizzle’s published docs and inspected runtime code show multiple migration artifact eras | Grizzle is now pinned to `v1.0.0-rc.1` | Copy | This removes the largest ambiguity from the migration-kit spec |

## Artifact Mapping

| Concern | Drizzle upstream | Grizzle direction | Classification | Notes |
| --- | --- | --- | --- | --- |
| Migrations folder | `out` defaults to `./drizzle` | Grizzle will preserve the same contract shape but use `./grizzle` as its default output directory | Adapt | Intent matches Drizzle, naming is Grizzle-branded |
| Meta folder | Some Drizzle docs/examples still mention `meta/`, but tagged RC.1 runtime does not require it for the active artifact model and treats `meta/_journal.json` as an old-format marker | Grizzle should omit `meta/` from the target artifact model | Copy | Runtime parity with RC.1; stale docs describe older layouts |
| Journal file | `v1.0.0-rc.1` runtime treats `meta/_journal.json` folders as old format and errors until `drizzle-kit up` is run | Grizzle should target the RC folder-per-migration format, not the old journal-based runtime layout | Copy |
| Journal entries | Older Drizzle runtimes used explicit journal entry models; RC.1 is centered on folder-per-migration artifacts plus DB history | Grizzle should not invent a legacy-style journal dependency if RC.1 no longer uses it as the active runtime contract | Copy | Runtime parity with RC.1; legacy journal semantics remain unsupported initial-scope input |
| Snapshot tracking | Drizzle generate compares current schema snapshot with prior snapshots and persists snapshot artifacts | Grizzle should define snapshot artifacts as first-class inputs to `generate` | Copy |
| Runtime snapshot requirement | RC.1 runtime can apply a migration folder containing `migration.sql` without requiring `snapshot.json` | Grizzle `migrate` must require `snapshot.json` so every applied migration remains part of the snapshot graph used by `check` and `generate` | Diverge | Intentional runtime-strictness divergence; avoids SQL-only artifacts that cannot participate in branch analysis |
| Generate parent selection | RC.1 `generate` uses `checkHandler()` output in selected commutative-branch paths, while common paths may return an empty result and some dialect paths infer locally | Grizzle `generate` must consume a normalized `CheckResult` rather than picking the last folder lexicographically | Adapt | Same branch-analysis intent, but Grizzle hardens the Go API by always returning a structured successful result |
| Generated migration identity | RC.1 artifact identity is the migration directory name, and generated metadata calls that value a `tag` while runtime records it as `name` | Grizzle should define migration identity in artifacts before defining history semantics | Copy |
| Migration folder style | `v1.0.0-rc.1` uses folder-per-migration artifacts such as `<YYYYMMDDHHmmss>_<name>/migration.sql` and snapshot data | Grizzle should mirror this structure closely unless there is a strong reason not to | Copy |
| Migration naming | `v1.0.0-rc.1` migration identity includes folder name; runtime reads `name` from the folder and uses that for pending detection | Grizzle should treat the folder name as the primary migration identity | Copy |
| Migration timestamp format | `v1.0.0-rc.1` runtime parses the first 14 characters of the folder name as UTC timestamp components (`YYYYMMDDHHmmss`) | Grizzle should mirror the same timestamp interpretation for folder discovery and ordering | Copy |
| Custom migration names | Drizzle docs support `generate --name ...`, and RC.1 source uses the provided suffix directly | Grizzle should support explicit human-readable migration suffixes but validate them before directory creation | Diverge | Intentional path-safety hardening |
| Prefix customization | Drizzle Kit removed `migrations.prefix` before RC.1; RC.1 `generate` always produces timestamp-prefixed migration directories | Grizzle should not support configurable migration prefixes in the initial RC.1 target | Copy | Current web/Context7 docs may still surface stale pre-RC prefix examples; tagged RC.1 source and changelog are canonical |
| Custom migrations | Drizzle docs support `generate --custom` for empty/user-authored migrations, including DDL not supported by Drizzle Kit; tagged RC-style generate handlers still write normal artifacts with the standard custom placeholder comment | Grizzle should include custom migrations in the initial file-based workflow and avoid a Grizzle-specific marker, but limit initial custom SQL to non-schema-shaping operations unless a future custom-snapshot workflow exists | Diverge | Intentional snapshot-coherence hardening |
| Introspection artifacts from pull | RC.1 `pull` writes an initial introspection migration artifact when no snapshots exist and comments out the SQL | Grizzle should preserve inert SQL and `pull --init` metadata recording, but normal `migrate` must reject pending/unrecorded artifacts whose `migration.sql` carries the managed introspection header; artifacts already recorded by `name` are skipped | Diverge | Intentional bootstrap safety hardening |
| Pull init live-state validation | RC.1 `pull --init` records metadata after local/database migration-metadata precondition checks; reviewed runtime source does not perform an additional live-schema equality check | Grizzle should revalidate that the bootstrap introspection artifact still matches the live database snapshot before recording metadata | Diverge | Intentional bootstrap hardening; avoids marking stale introspection output as the baseline |

## Execution Mapping

| Concern | Drizzle upstream | Grizzle direction | Classification | Notes |
| --- | --- | --- | --- | --- |
| Runtime input | `v1.0.0-rc.1` runtime scans migration folders, reads `migration.sql`, and rejects old journal-based layouts | Grizzle should move away from bare `*.sql` directory scanning to the RC folder-per-migration model | Copy |
| Statement segmentation | Drizzle runtime splits on the `--> statement-breakpoint` substring | Grizzle should not use raw semicolon splitting and should recognize only active full-line breakpoint delimiters | Diverge | Intentional SQL-safety hardening for inert `pull` payloads |
| Breakpoint config | Drizzle config exposes `breakpoints`; CLI `generate` and `pull` expose it; default is `true` | Grizzle defaults to enabled and rejects `breakpoints=false` in the initial implementation until disabled-breakpoint artifact execution has explicit metadata or parser/executor support | Diverge | Intentional implementation-safety gap for no-delimiter artifacts |
| Pending detection | `v1.0.0-rc.1` runtime computes local migrations, reads DB migrations, and filters by migration `name` presence using `getMigrationsToRun()` | Grizzle should define pending detection by migration identity/name, not just timestamps or file enumeration | Copy |
| Transaction boundaries | Drizzle executes the full pending batch transactionally where the driver supports it, but some drivers such as Neon HTTP are explicitly non-transactional | Grizzle should preserve the same high-level intent and define driver exceptions explicitly | Adapt |
| Missing / extra file handling | `v1.0.0-rc.1` runtime only treats subdirectories containing `migration.sql` as migrations, ignores extra files, and rejects old-folder layouts at load time | Grizzle should define strict artifact discovery rules, require `snapshot.json`, reject extra files inside migration directories, ignore only the documented root sidecar allowlist, and fail unknown root regular files | Diverge | Intentional artifact-shape hardening |
| Workflow validation | RC.1 CLI runs `check` before `generate` and `migrate` | Grizzle should preserve that workflow discipline | Copy |
| Rename workflow | RC.1 `generate` uses interactive resolver prompts to classify create/drop pairs as renames or moves, emits rename statements, and records resolved rename metadata in `snapshot.json` | Grizzle should support rename resolution during `generate` using the same interactive CLI workflow for the initial implementation; the library uses a resolver interface as a Go test/API seam | Copy plus DEVIATION:LANGUAGE seam | Explicit CLI answer files, config/schema-based mappings, and CI rename-resolution flags are not initial scope; track upstream demand in `sofired/grizzle#279` |
| DB-backed migration lock | RC.1 runtime does not acquire an explicit migration lock, and upstream issue `drizzle-team/drizzle-orm#874` tracks simultaneous `migrate()` execution as an open `bug` / `priority` issue | Grizzle should require a DB-backed lock before history read/pending computation/execution/history insert | Diverge | Intentional upstream bug-fix / concurrency-hardening divergence; `UNIQUE(name)` is a second line of defense, not the primary concurrency control |

## History Mapping

| Concern | Drizzle upstream | Grizzle direction | Classification | Notes |
| --- | --- | --- | --- | --- |
| History table default | Drizzle defaults to `__drizzle_migrations`; PostgreSQL schema default is `drizzle` | Grizzle should define equivalent defaults explicitly, using `__grizzle_migrations` and `grizzle` schema | Diverge | **DEVIATION:INTENTIONAL** namespace/branding divergence; same behavior class with Grizzle-specific default names |
| Stored fields | `v1.0.0-rc.1` extends the migration table from `(id, hash, created_at)` to `(id, hash, created_at, name, applied_at)` | Grizzle should target the RC.1 schema, not the older three-column layout | Copy |
| Custom table/schema | Drizzle config allows custom migrations table and PostgreSQL schema | Grizzle should include configurable migration table/schema in the initial design | Adapt |
| Name uniqueness | RC.1 runtime uses `name` as migration identity for pending detection, but reviewed tagged runtime evidence does not show a uniqueness constraint on `name` | Grizzle should tighten the schema and enforce `UNIQUE(name)` from day one | Diverge | Intentional schema hardening; duplicate identity rows undermine name-based pending detection |
| Name / created_at nullability | RC.1 upgrade/table creation code permits nullable `name` and `created_at` in several paths | Grizzle should require both as `NOT NULL` in the supported history schema | Diverge | Intentional schema hardening; Grizzle starts with the RC.1 file model and does not need Drizzle's transitional nullable history rows |
| Applied hash drift | RC.1 runtime computes file hashes but skips already-applied migrations by `name` without comparing stored hash to local file hash | Grizzle should skip already-applied migrations by `name` without blocking on hash differences | Copy | Hashes are stored metadata; innocuous comment/spacing edits to already-applied files do not affect pending detection |
| Hash input | RC.1 computes SHA-256 from the JS string read from `migration.sql` | Grizzle computes SHA-256 over the exact raw `migration.sql` bytes | Diverge | Raw-byte hashing is deterministic across encodings/newline handling and matches Go filesystem semantics |
| Table upgrades | RC.1 contains upgrade logic from version 0 `(id, hash, created_at)` to version 1 `(id, hash, created_at, name, applied_at)` | Grizzle will not implement legacy table upgrade support in the initial RC.1-based design | Diverge | Intentional initial-scope exclusion; Grizzle has no released pre-RC file-migration history schema to migrate |

## Validation Mapping

| Concern | Drizzle upstream | Grizzle direction | Classification | Notes |
| --- | --- | --- | --- | --- |
| Collision checks | Drizzle documents `check` as walking generated migrations to detect collisions / race conditions | Grizzle should define the same class of validation in a dedicated `check` spec | Copy |
| Artifact-shape checks and local digests | RC.1 `checkHandler()` is centered on snapshot validation and commutativity analysis | Grizzle `check` also validates required SQL/snapshot files and computes local SQL/snapshot digests | Diverge | Intentional artifact hardening and TOCTOU support |
| Check inputs | RC.1 `prepareCheckParams()` resolves only `out` and `dialect`; credentials in shared config are not passed to `checkHandler()` | Grizzle `check` should remain offline and ignore credential config for this workflow | Copy |
| Branch selection | RC.1 `check` reconstructs the migration graph, analyzes leaf nodes, and may select an open commutative branch context for continued generation | Grizzle should copy the conceptual model and document the branch terms explicitly | Copy |
| Conflict bypass flags | Drizzle RC.1 exposes `--ignore-conflicts` on `generate`, `migrate`, and `check` | Grizzle should intentionally omit conflict-bypass flags from the initial design | Diverge | The validation gate is meant to stop unsafe continuation, not be bypassed |
| Migration-folder upgrades | Drizzle has `up` for older folder structures | Grizzle will not implement `up` or migration-folder upgrade mechanics in the initial RC.1-based design | Diverge | Intentional initial-scope exclusion; Grizzle has no released old file-migration folder format to upgrade |
| Structured check result | RC.1 `checkHandler()` returns branch context consumed internally by generate | Grizzle library `Check` should return structured data even if CLI output remains human-readable | Adapt |
| Empty migrate root | RC.1's folder scan can produce no pending migrations when no migration files are discovered | Grizzle `migrate` should fail on absent or empty artifact roots unless explicitly allowed | Diverge | Intentional deployment-safety hardening |
| Push API boundary | RC.1 `push` is a direct schema-sync command, separate from file migration execution | Grizzle should keep `push` outside the initial `kit/filemigrate` package and specify it in a dedicated direct-sync spec | Adapt | Preserves workflow boundary while keeping the CLI command |

## Current Drizzle Evidence

### Docs

- Drizzle Kit overview:
  - documents `generate`, `migrate`, `push`, `pull`, `check`, `up`
- Drizzle Kit migrate/check/generate:
  - documents file-based migrate workflow
  - documents `__drizzle_migrations`
  - RC.1 source exposes `--ignore-conflicts` as a conflict-bypass option
- Drizzle Kit check:
  - documents migration-history consistency and collision detection
- Drizzle config:
  - documents `out`
  - documents `migrations.table` and `migrations.schema`
  - some web docs still document `migrations.prefix`, but this conflicts with RC.1 tagged source and the `1.0.0-beta.7` changelog
  - documents `breakpoints`
- Drizzle Kit generate:
  - documents snapshot comparison
  - documents timestamped migration folder output
  - documents rename prompts during generation
  - documents `--custom`
- Drizzle tutorials / examples:
  - some still show older flat migration layouts with `meta/_journal.json`
  - these appear inconsistent with the RC.1 tagged runtime
- Drizzle upgrade docs:
  - document migration-folder structure changes across versions
  - show that Drizzle artifact layout has evolved over time

### Runtime / Source

- `drizzle-orm/src/migrator.ts`
  - in `v1.0.0-rc.1`, rejects old `meta/_journal.json` folders
  - reads migration subdirectories containing `migration.sql`
  - derives migration identity from folder `name`
  - derives ordering timestamp from the first 14 characters of the folder name
  - splits SQL on `--> statement-breakpoint`
  - computes SHA-256 hash of the migration file
- `drizzle-orm/src/pg-core/async/session.ts`
  - implements PostgreSQL migration execution
  - creates or upgrades the history table through the migrator path
  - applies the pending batch transactionally for the async PostgreSQL session
- `drizzle-orm/src/mysql-core/dialect.ts`
  - applies the pending batch transactionally for MySQL
- `drizzle-orm/src/sqlite-core/dialect.ts`
  - applies the pending batch transactionally for SQLite sync and async paths
- `drizzle-orm/src/neon-http/migrator.ts`
  - explicitly documents non-transactional execution because the driver lacks transaction support
- `drizzle-orm/src/migrator.utils.ts`
  - RC.1 pending detection is name-based via `getMigrationsToRun()`
  - pending detection does not acquire a migration lock before filtering local migrations against database history
- RC.1 runtime migrator paths reviewed for PostgreSQL, MySQL, SQLite, and Neon HTTP:
  - do not wrap history read, pending computation, SQL execution, and history insert in a shared migration lock
  - rely on driver transactions where available, but a transaction alone does not serialize two independent migrators that both read the same pre-run history state
- `drizzle-orm/src/up-migrations/*.ts`
  - RC.1 contains explicit migration-table schema upgrade logic
- `drizzle-kit/src/index.ts`
  - RC.1 config type still exposes `migrations.table`, `migrations.schema`, `breakpoints`, `strict`, `verbose`
  - RC.1 config type does not expose `migrations.prefix`
- `drizzle-kit/src/cli/validations/common.ts`
  - active RC.1 config validation accepts only `migrations.table` and `migrations.schema`
  - older prefix enum remains only in `drizzle-kit/src/legacy/common.ts`
- `drizzle-kit/src/utils/words.ts` and `drizzle-kit/src/cli/commands/generate-common.ts`
  - `generate` and `pull` artifact writing derive the migration tag from `prepareSnapshotFolderName()` plus suffix
  - no prefix option is passed into `prepareMigrationMetadata()` or `writeResult()`
- `changelogs/drizzle-kit/1.0.0-beta.7.md`
  - explicitly says the `migrations.prefix` field was removed from `drizzle-kit` config
  - says there was no alternative for `prefix` at that time
- `drizzle-kit/src/cli/schema.ts` and `drizzle-kit/src/cli/index.ts`
  - RC.1 CLI registers `generate`, `migrate`, `pull`, `push`, `studio`, `up`, `check`, and `export`
  - `schema.ts` invokes `checkHandler` before both `generate` and `migrate`
- `drizzle-kit/src/cli/commands/check.ts`
  - reconstructs the snapshot graph
  - identifies current leaf nodes
  - detects non-commutative conflicts
  - selects the open commutative branch candidate closest to the leaves when multiple candidates qualify
- `drizzle-kit/src/cli/commands/generate-common.ts`
  - custom migrations still write normal migration directories with `snapshot.json` and `migration.sql`
  - normal no-change generation writes no artifact
  - generated custom SQL starts as placeholder comment content
  - resolved `renames` metadata is written into `snapshot.json`
- `drizzle-kit/src/cli/prompts.ts`
  - RC.1 implements interactive resolver prompts that ask whether a newly seen entity was created or renamed/moved from a deleted entity
- `drizzle-kit/src/dialects/*/diff.ts`
  - dialect diff functions consume resolver output as `renamedOrMoved`
  - resolved renames are turned into dialect-specific rename statements where supported
  - resolved rename metadata is serialized as strings such as `<from>-><to>`
- `drizzle-kit/src/cli/commands/pull-*.ts`
  - pull writes generated schema source
  - when no snapshots exist, pull writes an introspection artifact with commented SQL
  - `pull --init` delegates to runtime migrator init mode to record metadata only
  - reviewed runtime init paths check local/database migration metadata preconditions but do not perform a fresh live-schema equality check before inserting metadata

### Upstream Issues

- `https://github.com/drizzle-team/drizzle-orm/issues/874`
  - title: `[BUG]: migrate isn't protected against simultaneous execution`
  - status reviewed: 2026-05-05
  - labels include `bug`, `drizzle/kit`, and `priority`
  - describes concurrent `migrate()` calls applying the same migrations multiple times
  - expected behavior asks for a locking mechanism
  - maintainer follow-up identifies the issue as important and planned, but RC.1 source reviewed here still lacks the lock
- `https://github.com/drizzle-team/drizzle-orm/issues/5307`
  - title: `[FEATURE]: Non-interactive conflict resolution (AI-friendly)`
  - status reviewed: 2026-05-05
  - proposes a `generate --preflight` step to export questions and a `generate --answers` step to provide non-interactive answers
  - opened after RC.1 and remains future upstream tracking, not RC.1 behavior
- `https://github.com/drizzle-team/drizzle-orm/pull/5454`
  - title: `feat(drizzle-kit): support non-interactive generate conflicts`
  - status reviewed: 2026-05-05
  - proposes `--preflight`, `--answers`, non-interactive conflict resolvers, and API helpers
  - open and unmerged as of this spec update, so it is not part of the Grizzle RC.1 parity target
- `https://github.com/drizzle-team/drizzle-orm/issues/4651`
  - title: `[BUG]: drizzle-kit pushSchema requires manual input when renaming column`
  - status reviewed: 2026-05-05
  - documents adjacent programmatic/CI pain around rename prompts
  - applies to `pushSchema`, but reinforces the backlog need to track upstream non-interactive prompt handling

## Source Authority Rule

For the RC.1 port, when Drizzle docs/tutorial snippets and tagged RC.1 source disagree, Grizzle should treat the tagged RC.1 source as canonical.

That matters especially for:

- migration artifact layout
- active runtime discovery rules
- supported history-table schema
- pending-migration detection
- whether `meta/` has active runtime purpose
- whether an upstream behavior is intentional design or a known unresolved bug

## Immediate Implications for Grizzle

1. Grizzle is now pinned to the Drizzle RC.1 folder-per-migration artifact model.
2. Grizzle should stop treating the file-based system as "apply `.sql` files from a directory."
3. Grizzle should define the full RC.1-style artifact model before continuing runtime implementation.
4. Grizzle should define `generate` and `check` before replacing `migrate`.
5. Grizzle should replace naive semicolon splitting with RC-style breakpoint-driven execution.
6. Grizzle should target the RC.1 migration table schema shape when writing the history spec.
7. Grizzle should explicitly reject old migration folder formats and old migration-table schemas in the initial design.
8. Grizzle should treat upgrade/compatibility machinery as an intentional initial-scope exclusion until Grizzle has its own released legacy formats to support.
9. Grizzle should avoid importing doc-only or version-ambiguous Drizzle features into the initial target unless they are verified in the RC.1 tagged source or clearly documented for that release line. Configurable migration prefixes are explicitly not part of the RC.1 target.
10. Grizzle should keep `snapshot` and `diff` out of the target public workflow surface to stay aligned with Drizzle.
11. Grizzle should enforce `UNIQUE(name)` in its history schema even though reviewed Drizzle RC.1 runtime evidence does not appear to do so.
12. Grizzle will use its own branded defaults: `./grizzle`, `__grizzle_migrations`, and `grizzle` schema. The history table/schema default-name changes are **DEVIATION:INTENTIONAL** namespace/branding divergence from RC.1.
13. Grizzle will include custom migrations and configurable migration table/schema in the initial file-migrations scope.
14. Grizzle will omit `meta/` and legacy journal entries from the target artifact model as RC.1 runtime parity.
15. Grizzle will require `check` as part of the normal `generate` and `migrate` workflow.
16. Grizzle will document migration branches as a snapshot-graph concept, not a Git concept.
17. Grizzle will model `check` as a graph-validation and branch-selection step, not only as a file-format validator; the complete structured result is a Go API adaptation over RC.1's partial internal result shape.
18. Grizzle will copy RC.1's transactional intent for supported drivers and document explicit non-transactional driver exceptions.
19. Grizzle will not expose `--ignore-conflicts`-style escape hatches in the initial design; this is DEVIATION:INTENTIONAL safety hardening relative to Drizzle RC.1.
20. Grizzle will define `generate` as a first-class file-migration command with `CheckResult` as its parent-selection input.
21. Grizzle will preserve Drizzle RC.1's `pull` bootstrap artifact behavior: inert introspection SQL plus `pull --init` metadata recording without a sidecar kind marker, while rejecting pending/unrecorded artifacts whose `migration.sql` carries the managed introspection header in normal `migrate` as bootstrap safety hardening. Once `pull --init` records the artifact by `name`, later `migrate` runs skip it like any other applied migration.
22. Grizzle will compute migration hashes over exact raw SQL file bytes.
23. Grizzle will require DB-backed migration locking as an intentional upstream bug-fix / concurrency-hardening divergence, with `UNIQUE(name)` history protection as a second line of defense.
24. Grizzle will support rename resolution during `generate` using Drizzle RC.1-style interactive prompts in the initial implementation. Explicit config/schema-based and non-interactive rename mappings are deferred and tracked in `sofired/grizzle#279`.
25. Grizzle will use `github.com/sofired/grizzle/kit/filemigrate` for the file migration workflow APIs; direct `push` belongs to a separate direct-sync spec/API boundary.
26. Grizzle will add DEVIATION:INTENTIONAL bootstrap hardening to `pull --init` by validating the local introspection artifact against a fresh live database snapshot before recording baseline metadata.
