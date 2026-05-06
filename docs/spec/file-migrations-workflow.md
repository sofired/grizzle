# File Migrations Workflow Specification

## Status

Draft

## Purpose

Define the end-to-end Drizzle-style file-based migration workflow for Grizzle.

Pinned upstream target:

- Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`

## Scope

This document must define:

- `generate`
- `migrate`
- `check`
- `push`
- `pull`
- relationship to existing `snapshot` and `diff`
- development workflow
- deployment workflow

## Upstream References

- Drizzle Kit overview
- Drizzle Kit migrate
- Drizzle Kit check
- [file-migrations-upstream-mapping.md](./file-migrations-upstream-mapping.md)
- [file-migrations-generate.md](./file-migrations-generate.md)

## Current Grizzle State

Current Grizzle behavior is split across two models:

- `snapshot` and `diff` already support schema snapshotting and diff generation
- pre-branch `migrate` applied live schema diffs directly against a database
- in-progress file-migration work reads SQL files from disk but does not yet implement the full RC.1 artifact, check, and history contract

This means Grizzle currently has:

- partial ingredients for a file-based workflow
- but not a complete Drizzle-style workflow contract

Most importantly, `generate` does not yet exist as a first-class command, so the file-based workflow is not end-to-end.

## Target Workflow

The target workflow must mirror Drizzle's command responsibilities:

1. `generate`
   - read schema definitions
   - compare current schema snapshot against previous migration metadata
   - emit new migration artifacts
2. `migrate`
   - read generated migration artifacts
   - determine pending migrations from artifact metadata plus DB history
   - execute pending migrations
   - record execution history
3. `check`
   - validate migration artifact consistency
   - detect collisions / ordering problems / unsupported legacy formats
4. `push`
   - public direct-sync command boundary only in this spec package
   - full direct-apply behavior, destructive-change handling, dry-run behavior, locking, and CI safety require a dedicated push spec before implementation-ready work resumes
5. `pull`
   - introspect a live database into schema code or related artifacts
   - write Go schema definition files from the live database shape

Grizzle-specific defaults and scope decisions already chosen:

- default migration output directory: `./grizzle`
- default history table: `__grizzle_migrations` (**DEVIATION:INTENTIONAL** Grizzle namespace/branding divergence from RC.1's `__drizzle_migrations`)
- default PostgreSQL migrations schema: `grizzle` (**DEVIATION:INTENTIONAL** Grizzle namespace/branding divergence from RC.1's `drizzle`)
- custom migrations are in scope
- configurable migration table/schema are in scope
- `meta/` is not part of the target artifact model
- Drizzle RC.1's `export` command is **DEVIATION:INTENTIONAL** and explicitly deferred from the initial Grizzle file-migration target until a dedicated export spec defines its workflow, output format, safety model, and Go API surface

The critical workflow rule is:

`migrate` must not become the primary file-based migration entrypoint until `generate`, artifact semantics, and `check` are defined.

The initial scope rule is:

Grizzle supports only the RC.1-style artifact and history formats. It does not attempt to upgrade older migration folder formats or older history-table schemas.

This is intentional initial-scope reduction. Drizzle needs `up` because Drizzle has shipped older migration layouts; Grizzle is starting from the RC.1-style model and has no released old file-migration layout to convert.

The target validation rule is:

- `check` is part of the primary file-based workflow
- `generate` and `migrate` must run local artifact `check` internally before mutating local artifacts or the database
- Grizzle will not provide a conflict-bypass flag analogous to Drizzle's `--ignore-conflicts`; this is DEVIATION:INTENTIONAL safety hardening relative to Drizzle RC.1

## Command Responsibilities

High-level responsibilities derived from Drizzle:

- `generate` owns artifact creation
- `migrate` owns artifact execution
- `check` owns artifact consistency validation
- `push` owns direct schema application
- `pull` owns reverse-generation from database state

Normative workflow boundaries:

- `generate` creates or extends the migration artifact graph
- `generate` follows the algorithm defined in [file-migrations-generate.md](./file-migrations-generate.md)
- `migrate` applies existing migration artifacts only
- `migrate` does not generate new artifacts
- `check` validates artifacts and graph coherence but does not mutate database schema
- `push` is a separate non-file-based workflow and must not be conflated with `migrate`
- `push` is intended for development and controlled local workflows, not as the primary shared-environment deployment path
- `pull` introspects a live database and writes Go schema definition files
- `pull` may generate an initial commented introspection migration artifact when the migrations output location has no existing snapshots
- `pull` does not apply migrations
- `pull` is the reverse-generation workflow from database state to Go schema source

## User Workflows

### Development

Target development workflow:

1. edit schema definitions
2. run `generate`; the command runs `check` internally before writing artifacts
3. inspect the new migration artifact directory
4. run `check` again against the updated graph
5. run `migrate` locally or in controlled environments

This matches Drizzle's two-step file workflow plus explicit validation.

Why `check` is required:

- migration history can become branched when multiple migrations are generated independently from the same prior snapshot
- `check` validates that graph before new artifacts are generated or applied
- if the graph is branched but commutative, `check` may provide the effective branch context needed for the next generation step
- if the graph is non-commutative, the workflow must stop for manual resolution

First-run rule:

- an empty project with no migrations yet is a valid state
- `check` must succeed on an empty migrations directory
- the first `generate` creates the initial migration artifact set from that empty state

### Deployment

Target deployment workflow:

1. commit generated migration artifacts
2. validate artifacts in CI with `check`
3. run `migrate` in deployment environments
4. do not generate migrations during deployment

Normative deployment rule:

- production and shared-environment deployments apply already-committed artifacts only
- artifact generation belongs in development or controlled release preparation, not in deploy-time mutation paths
- `push` must not be the recommended production deployment mechanism
- `migrate` must fail on absent or empty artifact roots unless an explicit allow-empty option is selected and database history is absent or empty

### Introspection

Target introspection workflow:

1. connect to a live database
2. run `pull`
3. review generated Go schema definition files
4. if `pull` generated an initial introspection migration artifact, review that migration directory as part of the pulled state
5. move customizations into separate hand-maintained files if needed
6. if recording the pulled database state as already applied, run `pull --init` only after reviewing the introspection artifact
7. commit the generated schema files and any generated introspection migration artifact before subsequent migration work

`pull` is the inverse of schema-to-migration generation. It is not the same workflow as `generate`, and it must not be documented as a migration-application command.

Initial introspection migration artifacts record an existing database shape for future diffs. Grizzle preserves Drizzle RC.1's inert generated SQL and `pull --init` metadata-recording model, but tightens bootstrap safety by requiring `pull --init` for managed introspection artifacts.

Bootstrap recommendation:

- use `pull --init` when the intent is to mark an existing live database state as already applied
- an artifact is managed-introspection when the first non-empty physical line of `migration.sql`, after an optional UTF-8 BOM, is exactly `-- grizzle:managed-introspection v1`
- normal `migrate` must reject pending/unrecorded managed-introspection artifacts with `bootstrap_init_required`; once `pull --init` records the artifact by `name`, later `migrate` runs treat it as already applied and skip it
- if a developer intentionally edits the artifact into executable SQL and removes that header, normal `migrate` treats it as a normal custom migration artifact

## Relationship to Existing Commands

Current `snapshot` and `diff` are existing implementation ingredients, not proof that the file-based workflow is already complete.

Current scope note:

- Grizzle intentionally omits an `up`-style migration-format upgrade workflow initially
- Grizzle must avoid adding migration-format compatibility commands unless it later has its own released older supported formats
- To stay aligned with Drizzle's public workflow, `snapshot` and `diff` must not remain first-class public commands in the target file-migrations surface
- They may remain internal implementation concepts or compatibility-layer building blocks during development, but not primary public workflow commands

Normative direction:

- `snapshot` and `diff` may remain internal implementation building blocks
- they are not part of the target public workflow contract for file-based migrations
- users must be guided toward `generate`, `check`, and `migrate` instead

## Explicit Replacement Rules

`migrate` is not allowed to change meaning permanently until all of the following exist:

- artifact format specification
- history specification
- `check`
- artifact discovery and snapshot validators used by `check`
- `generate`
- execution semantics strong enough for generated artifacts

Implementation conclusion:

- the target semantics are now defined closely enough that future implementation work must follow these specs rather than reinterpret Drizzle behavior ad hoc

## Push Spec Scope

Initial scope decision:

- `push` remains part of the migration-kit surface
- new `push` CLI/API work is out of scope until a dedicated direct-sync spec exists
- the file-migrations implementation does not depend on that document because `push` is not part of artifact generation or artifact execution
- `push` must not be exposed from `kit/filemigrate`; it needs a direct-sync API and safety contract covering destructive detection, force behavior, dry-run defaults, CI/non-interactive use, and direct-sync locking before any schema mutation
