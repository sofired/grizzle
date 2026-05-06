# File Migrations Generate Specification

## Status

Draft

## Purpose

Define `grizzle generate`, the schema-to-migration-artifact workflow.

Pinned upstream target:

- Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`

## Scope

This document defines:

- command inputs
- required pre-generation validation
- schema loading
- previous snapshot selection
- diff generation
- rename handling
- no-op behavior
- custom migration behavior
- artifact writing
- deterministic implementation requirements

## Upstream References

- Drizzle Kit `generate`
- Drizzle RC.1 source:
  - `drizzle-kit/src/cli/schema.ts`
  - `drizzle-kit/src/cli/commands/check.ts`
  - `drizzle-kit/src/cli/commands/generate-common.ts`
  - `drizzle-kit/src/cli/commands/generate-*.ts`
  - `drizzle-kit/src/dialects/*/serializer.ts`
  - `drizzle-kit/src/dialects/*/diff.ts`

## Inputs

`generate` is a local filesystem operation.

Required inputs:

- schema definitions or schema loading configuration
- dialect
- migrations output directory

Optional inputs:

- explicit migration suffix/name
- custom migration mode
- breakpoints toggle after disabled-breakpoint execution is designed; initial implementation accepts only enabled breakpoints

Default behavior:

- breakpoints are enabled by default
- output directory defaults to `./grizzle`
- generated timestamp prefixes use UTC `YYYYMMDDHHmmss`

Schema loading trust boundary:

- default schema loading must use static parsing, generated registry data, or another non-executing representation of Go schema definitions
- static schema input traversal must follow the filesystem and resource-limit contract in [file-migrations-api.md](./file-migrations-api.md#schema-loader-boundary): canonicalized no-follow roots, regular files only, hardlink/symlink rejection where detectable, source byte/file/AST/declaration/literal caps, context cancellation, redacted diagnostics, and `invalid_path` / `resource_limit` errors
- executing arbitrary Go package initialization code during `generate` is not allowed unless the user explicitly selects a future trusted-code mode
- trusted-code mode, if added later, must be clearly marked as executing local project code
- generated snapshots must be built from the explicit `SchemaInput` contract, not from side effects hidden in package initialization
- the schema loader must be strict: recognized but unsupported schema constructs must fail with `unsupported_schema_construct`, `unsupported_feature`, or `unsupported_object_family` as appropriate rather than being dropped from the generated snapshot
- table-only parsing is not sufficient unless the selected schema input contains no supported non-table objects and no unsupported recognized constructs
- initial CLI `generate --schema` must either implement the static Go schema-loader contract in [file-migrations-api.md](./file-migrations-api.md#schema-loader-boundary) or fail as not implementation-ready; the library `Generate(SchemaInput)` path may be implemented first

## Required Pre-Generation Check

`generate` must run the same local artifact check as `grizzle check` before writing any artifact.

Normative rules:

- `generate` must resolve dialect and migrations output directory before invoking `check`
- if `check` fails, `generate` must fail without writing artifacts
- `generate` must consume the structured `CheckResult`
- `generate` must not infer the parent snapshot by simply choosing the lexicographically last migration directory
- initial implementation must reject disabled breakpoints before running diff or writing artifacts
- no `--ignore-conflicts` or conflict-bypass mode exists in the initial design; this is DEVIATION:INTENTIONAL safety hardening relative to Drizzle RC.1

The `--ignore-conflicts` omission is DEVIATION:INTENTIONAL safety hardening. Drizzle RC.1 exposes this flag, but Grizzle's initial design treats migration graph conflicts as mandatory stop conditions.

## Check Result Use

`CheckResult` supplies the effective migration-history basis for generation.

Normative cases:

- empty graph:
  - effective snapshot is the dialect dry/root snapshot
  - base ID is the sentinel origin UUID
  - effective parent IDs are `[00000000-0000-0000-0000-000000000000]`
  - generated snapshot `prevIds` must still be `[00000000-0000-0000-0000-000000000000]`
- linear graph:
  - effective snapshot is the single current leaf snapshot
  - effective parent IDs contain that leaf ID
- commutative branched graph:
  - effective snapshot is the materialized selected branch state returned by `check`
  - branch statements are typed dialect diff statements, not executable SQL
  - the new snapshot `prevIds` must use `CheckResult.EffectiveParentIDs`
- non-commutative or indeterminate graph:
  - `check` fails and `generate` must not continue

Sentinel origin UUID:

```text
00000000-0000-0000-0000-000000000000
```

## Generation Algorithm

Normative algorithm:

1. resolve config, dialect, schema input, migrations output directory, and breakpoint setting
2. validate explicit migration suffix if supplied
3. run local `check`
4. load current schema definitions for the selected dialect
5. convert schema definitions into the current dialect DDL model and fail on unsupported schema constructs
6. select `CheckResult.EffectiveSnapshot` as the previous effective snapshot
7. if `custom` mode is disabled, diff previous DDL against current DDL
8. if `custom` mode is disabled, resolve supported rename metadata
9. if no SQL statements are produced and custom mode is disabled, write no artifact and return a no-change result
10. if `custom` mode is enabled, skip SQL diff output, rename resolution, and no-change suppression after schema loading/validation succeeds
11. compute a new migration directory name
12. create a new snapshot with a fresh UUID and the selected `prevIds`
13. write `snapshot.json`
14. write `migration.sql`

`GenerateOptions.Schema` is required for both normal and custom generation, matching Drizzle RC.1's behavior of loading and validating schema before the custom branch. For `Custom=true`, the loaded current schema is validation input only; the effective parent snapshot from `CheckResult` supplies the DDL for the new custom snapshot.

## Rename Handling

Drizzle RC.1 uses interactive rename prompts during generation when created and deleted entities need to be classified as independent create/drop operations or as renames/moves.

Grizzle target rule:

- Grizzle must use interactive rename prompts in the initial implementation, matching Drizzle RC.1 workflow behavior
- prompts must be shown when generated and previous snapshots contain create/drop pairs that may represent renames or moves
- resolved renames must emit dialect-specific rename statements where supported
- resolved rename metadata must be written to `snapshot.json`
- rename metadata must use the RC.1-compatible `from->to` string encoding defined in [file-migrations-artifacts.md](./file-migrations-artifacts.md#snapshot-format)
- invalid cross-family resolver decisions or rename kinds unsupported by the selected dialect must fail with `unsupported_feature` before any artifact is written
- aborted prompts or unresolved rename ambiguity must fail generation without writing a partial artifact
- CLI prompt handling must require an interactive TTY and must fail with `interactive_required` when a prompt is needed but cannot be shown
- library generation must route rename decisions through a resolver/prompt interface rather than reading directly from standard input; this resolver is a DEVIATION:LANGUAGE Go library/test seam and may be implemented by callers without a TTY
- the resolver seam does not add a CLI answer-file, config-based rename mapping, or schema-annotation rename workflow; it only prevents the library API from depending on process-global standard input

Future scope:

- explicit config-based rename mappings are not part of the initial RC.1 target
- explicit schema-annotation rename mappings are not part of the initial RC.1 target
- non-interactive CLI answer files, CI flags, or config-based rename mappings are not part of the initial RC.1 target
- upstream demand and proposed Drizzle designs are tracked in GitHub issue `sofired/grizzle#279`

## No-Change Behavior

If the diff produces no SQL statements and custom mode is disabled:

- `generate` must write no migration directory
- `generate` must return success with a no-change result
- no snapshot-only artifact is written

This mirrors the RC.1 `writeResult()` behavior where normal generation with no SQL exits without writing a migration.

## Artifact Naming

`generate` computes the migration directory name from:

- UTC second-precision timestamp prefix
- explicit suffix if provided
- generated default suffix otherwise

Normative rules:

- explicit suffixes are validated, not silently normalized
- generated suffixes are normalized by Grizzle
- omitted suffixes use the production `NameGenerator`, which must copy Drizzle RC.1's default word-pair behavior: `adjective_hero`, selected from RC.1-derived adjective and hero word lists, then combined with the timestamp prefix as `<timestamp>_<adjective>_<hero>`
- if the computed full directory name already exists, generation fails with `duplicate_migration`
- the initial design does not require automatic retry or sub-second timestamp precision
- validating explicit suffixes is DEVIATION:INTENTIONAL path-safety hardening relative to RC.1, which passes the provided `--name` suffix directly into the folder name

## Snapshot Creation

Generated snapshots must follow [file-migrations-artifacts.md](./file-migrations-artifacts.md).

Normative rules:

- every generated migration gets a fresh snapshot `id`
- `prevIds` come from `CheckResult.EffectiveParentIDs`, using the sentinel origin UUID for empty-graph first generation
- for normal generation, `ddl` contains the current schema DDL after schema loading/conversion
- for custom generation, `ddl` contains `CheckResult.EffectiveSnapshot.DDL`, matching Drizzle RC.1's custom snapshot behavior of assigning a fresh `id` and `prevIds` to the previous snapshot payload
- for normal generation, `renames` contains the resolved rename metadata returned by the dialect diff process
- for custom generation, `renames` is empty
- the dialect/version pair must match the active dialect

## SQL Writing

Generated SQL statements are written to `migration.sql`.

Normative rules:

- when breakpoints are enabled, statements are joined with the newline-bounded delimiter `\n--> statement-breakpoint\n` so generated artifacts satisfy the stricter execution parser's full-line marker requirement; the leading-newline/full-line requirement is **DEVIATION:INTENTIONAL** artifact hardening over RC.1's `BREAKPOINT = "--> statement-breakpoint\n"` writer constant and substring-splitting runtime
- breakpoint-disabled migration artifacts are out of initial implementation scope
- `generate` and `pull` must reject `breakpoints=false` until a future design adds explicit statement-count metadata or a proven SQL parser/executor strategy
- this is **DEVIATION:INTENTIONAL** safety hardening relative to RC.1's breakpoints toggle; it avoids intentionally generating marker-free multi-statement artifacts before Grizzle has an explicit whole-payload execution design

## Custom Migrations

`generate --custom` creates a normal migration artifact with user-authored SQL placeholder content.

Normative rules:

- custom generation still runs `check`
- custom generation still loads and validates schema input before writing, matching Drizzle RC.1
- custom generation still validates dialect, output directory, and migration name
- custom generation writes both `migration.sql` and `snapshot.json`
- custom `migration.sql` starts with the Drizzle RC.1-style placeholder comment `-- Custom SQL migration file, put your code below! --`
- the custom snapshot must get a fresh `id`
- custom snapshot `prevIds` come from `CheckResult.EffectiveParentIDs`, using the sentinel origin UUID for empty-graph first generation
- by default, the custom snapshot `ddl` equals the effective parent DDL
- if Grizzle later supports supplying an updated schema snapshot for custom SQL, that must be an explicit option and not inferred from raw SQL text

Implication:

- custom SQL can express operations that the snapshot does not understand
- initial custom migrations are supported for data changes or operational SQL that does not change schema shape tracked by snapshots
- schema-changing custom SQL is unchecked and unsupported in the initial workflow unless a future explicit custom-snapshot workflow is added; CLI output and docs must warn that such SQL can make the live database diverge from `snapshot.json`
- this is DEVIATION:INTENTIONAL snapshot-coherence hardening relative to Drizzle's broader custom migration documentation
- custom migrations do not bypass graph ancestry or validation
- custom migrations are intended to be committed artifacts and should not contain secrets, seed passwords, API tokens, or personal data; high-confidence secret-literal findings, including SQL DDL credential clauses such as role/user password statements, fail closed by default unless `AllowSecretLiterals=true` / `--allow-secret-literals` is explicitly supplied

## Deterministic Implementation Hooks

The implementation must be testable without wall-clock or filesystem flakiness.

Library options must allow internal injection of:

- clock
- UUID source
- migration name generator
- rename resolver
- artifact store/filesystem
- hash function

CLI defaults may use real time, random UUIDs, the OS filesystem, and SHA-256.

Test fixtures must cover:

- empty graph first generation
- linear generation
- commutative branch generation
- non-commutative failure
- no-change generation
- custom generation
- duplicate artifact name
- invalid explicit suffix
