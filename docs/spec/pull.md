# Pull Specification

## Status

Draft

## Purpose

Define the `grizzle pull` command as the database-to-schema reverse-generation workflow.

Pinned upstream target:

- Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`

## Scope

This document defines:

- command purpose and boundaries
- CLI/API surface
- configuration inputs
- supported dialects and notable exclusions
- output files and overwrite behavior
- relation to migration artifacts
- `--init` behavior
- filtering and casing behavior
- Go-specific file and identifier generation rules

## Upstream References

Primary upstream references used for this document:

- Drizzle docs:
  - `drizzle-kit pull`
  - `drizzle-config-file`
- Drizzle RC.1 source:
  - `drizzle-kit/src/cli/schema.ts`
  - `drizzle-kit/src/cli/commands/utils.ts`
  - `drizzle-kit/src/cli/validations/cli.ts`
  - `drizzle-kit/src/cli/validations/common.ts`
  - `drizzle-kit/src/cli/commands/pull-postgres.ts`
  - `drizzle-kit/src/cli/commands/pull-mysql.ts`
  - `drizzle-kit/src/cli/commands/pull-sqlite.ts`
  - `drizzle-kit/src/cli/commands/pull-common.ts`
  - `drizzle-kit/src/cli/commands/generate-common.ts`
  - `drizzle-kit/src/dialects/pull-utils.ts`

## Upstream RC.1 Summary

Drizzle RC.1 `pull`:

- is the same command as `introspect` via CLI alias
- connects to a live database
- introspects the current schema
- writes generated schema source into the configured `out` directory
- writes relations source for dialect paths that emit relation metadata in RC.1
- if the output directory has no migration snapshots, also generates an initial introspection migration artifact
- if the output directory already has snapshots, does not generate new SQL migration artifacts
- optionally uses `--init` to mark the pulled schema as applied in the database migration table

Source-authority rule for this spec:

- if Drizzle docs and tagged RC.1 source disagree, tagged RC.1 source is canonical

## Grizzle Target

`grizzle pull` is the Go equivalent of Drizzle's database-to-schema introspection command.

It is:

- the reverse-generation workflow from live database state to source-controlled schema definitions
- distinct from `generate`, which goes from schema definitions to migration artifacts
- distinct from `migrate`, which applies migration artifacts to a database
- distinct from `gen`, which generates typed Go helper code from existing Go schema definitions

This is a Go-language adaptation of Drizzle's `pull`:

- Drizzle writes TypeScript schema files
- Grizzle writes Go schema-definition source files

## Command Surface

Target CLI commands:

- `grizzle pull`
- `grizzle introspect`

`grizzle introspect` must be treated as an alias of `grizzle pull`, mirroring Drizzle RC.1 `pull` / `introspect`.

## Responsibilities

`pull` owns these responsibilities:

- connect to a live database
- introspect database schema and relations
- translate that introspected structure into Go schema definition source
- write the generated schema-definition files into the configured `schema-out` directory
- optionally bootstrap migration metadata for the pulled schema

`pull` does not:

- apply migration SQL to the database
- generate typed `*_gen.go` query helper code
- replace `generate`
- replace `migrate`
- mutate application schema objects

## CLI Inputs

The target Grizzle `pull` command must accept the upstream-equivalent inputs adapted to Go.

Required:

- `dialect`
- database connection credential references/config

Optional:

- `schema-out`
- `migrations-out`
- `breakpoints`
- `introspect-casing`
- `init`
- `all-schemas` / `AllowBroadScan` for filesystem-mutating broad scans without schema/table filters
- `allow-secret-literals` / `AllowSecretLiterals` as explicit risk acceptance for high-confidence secret-literal findings
- `tablesFilter`
- `schemaFilter` in config
- `schemaFilters` as a Grizzle CLI convenience only if implemented as an intentional source-bug/UX fix
- `extensionsFilters` from config; a Grizzle CLI spelling must normalize to the same array shape before use
- migration table/schema config
- `assume-quiescent` for `--init` only when the adapter cannot prove schema stability

### Dialect

Supported upstream RC.1 dialect family:

- `postgresql`
- `mysql`
- `sqlite`
- `turso` / libSQL-style SQLite
- `singlestore`
- `mssql`
- `cockroach`

Unsupported upstream RC.1 case verified in source:

- `duckdb`

Grizzle must mirror this support envelope only insofar as the corresponding internal introspection support exists.

Initial Grizzle support:

| Dialect | Initial `pull` status |
| --- | --- |
| PostgreSQL | PARITY target; required initial scope |
| MySQL | PARITY target; required initial scope |
| SQLite | PARITY target; required initial scope |
| Turso / libSQL | DEVIATION:GAP (not designed) unless backed by the SQLite introspector without behavior changes |
| SingleStore | DEVIATION:GAP (not designed) |
| MSSQL | DEVIATION:GAP (not designed) |
| Cockroach | DEVIATION:GAP (not designed) |
| DuckDB | Unsupported, matching the reviewed RC.1 command path |

Unsupported initial dialects must fail with `unsupported_dialect` before opening a database connection.

### Database Credentials

Like Drizzle, `pull` is a database-connected command.

It must resolve:

- DSN / URL-style connection inputs where available, supplied through a credential reference rather than a literal secret-bearing CLI argument
- split host/user/database style inputs where appropriate
- password, token, private-key, URL userinfo, or DSN secret fields through documented references such as environment-variable names, protected secret-file references, or a future prompt/fd mechanism
- driver-specific credential shapes where dialect/runtime requires them

Credential redaction rules from [file-migrations-api.md](./file-migrations-api.md#library-input-contracts) apply to `pull` config parsing, connection errors, CLI output, and diagnostics. `pull` must not print raw DSNs, URL userinfo, password query parameters, or split secret fields. The initial CLI must not accept literal password/token/secret-bearing DSN arguments because they can leak through process lists and shell history; non-secret direct flags are allowed.

### Output Directories

Drizzle RC.1 uses a single `out` directory for:

- generated schema source files
- generated relations source files
- initial introspection migration artifacts when bootstrapping

Grizzle intentionally diverges here for a Go-native repository layout. This is DEVIATION:INTENTIONAL filesystem-layout adaptation.

Grizzle target:

- `schema-out` is the target output directory for generated Go schema source
- `migrations-out` is the target output directory for bootstrap migration artifacts and snapshots

Default targets:

- `schema-out`: `./schema`
- `migrations-out`: `./grizzle`

Normative rule:

- Grizzle does not preserve Drizzle's single-`out` directory model for `pull`
- generated Go source files are written under `schema-out`
- bootstrap introspection migration artifacts are written under `migrations-out`
- the `pull` CLI must not use `--out` because it is ambiguous in Grizzle's split-output layout
- config `out`, when accepted for RC.1 familiarity, maps only to `migrations-out`
- omitted `schema-out` resolves to `./schema`; `schemaOut` in config or `--schema-out` on the CLI overrides only that default
- explicit CLI/config collision rules from [file-migrations-api.md](./file-migrations-api.md) apply; CLI `--schema-out` or `--migrations-out` is allowed with config only when config did not supply that same target through `schemaOut`, `migrationsOut`, or `out`, and must not silently override config values. This split-output fill-in behavior is DEVIATION:INTENTIONAL from RC.1 introspect, which only allows `--init` as a config-mode overlay.
- after canonicalization, `schema-out` and `migrations-out` must not overlap in either direction; nesting generated source under the migration artifact root would create child directories that later artifact discovery must reject
- this is an intentional Go-native filesystem-layout divergence, not a workflow divergence

### Breakpoints

RC.1 `pull` accepts `breakpoints`.

Its purpose is not to change schema-source generation. It affects the initial introspection migration artifact generated when `migrations-out` has no migration snapshots.

Grizzle must preserve this behavior:

- omitted or true `breakpoints` keeps generated introspection migration SQL segmentation enabled
- it does not change the generated Go schema definition files
- the initial implementation must reject `breakpoints=false` as **DEVIATION:INTENTIONAL** safety hardening because disabled-breakpoint artifacts are out of initial scope

### Introspect Casing

RC.1 exposes `--introspect-casing` with:

- `camel`
- `preserve`

Drizzle uses this for naming keys in generated schema/relations source.

Grizzle must preserve the same option concept, adapted to Go:

- `camel`
  - generate Go identifiers in camel-style naming
- `preserve`
  - preserve original database naming as much as possible in generated identifiers

Normative rule:

- casing options affect generated source identifiers
- casing options do not change the literal database table or column names encoded in the schema builders

Deterministic Go identifier normalization:

- split database names on non-letter/non-digit Unicode separators and on ASCII `_`, `-`, `.`, and whitespace
- for `camel`, convert split words to exported PascalCase for top-level table, view, enum, and relation identifiers; lower camel is used only for generated local helper identifiers
- for `preserve`, preserve existing letter case within words but still remove separators and produce exported top-level identifiers
- if the first resulting rune is not a Unicode letter or `_`, prefix `Schema_`
- remove or replace unsupported identifier runes with `_` before casing
- Go keywords receive a `_schema` suffix
- predeclared identifiers that would be confusing as top-level schema objects receive a `_schema` suffix
- common acronyms are not special-cased in the initial implementation; casing is deterministic word-based transformation only
- if two database objects normalize to the same Go identifier after these rules, `pull` fails with `identifier_collision`

### Filters

RC.1 supports:

- `tablesFilter`
- config `schemaFilter`
- CLI `schemaFilters` exists in the RC.1 command surface but reviewed RC.1 source does not wire it into the effective pull filters
- `extensionsFilters`, whose effective validated shape is an array of supported extension names such as `postgis`
- entity-based filters such as roles in newer config shapes

Grizzle must preserve the table/schema/extension filter model for initially supported object families. Entity-based filters for roles or other unsupported object families are rejected as `unsupported_feature` in the initial implementation. If Grizzle exposes a working CLI `schemaFilters` flag, it is DEVIATION:INTENTIONAL UX/source-bug hardening relative to RC.1's partially wired CLI option and must map to the same semantics as config `schemaFilter`.

Filter semantics derived from reviewed RC.1 source:

- `tablesFilter` uses glob-style matching against table names only, matching RC.1; schema selection remains separate through config `schemaFilter`
- config `schemaFilter` accepts one or more schema filters
- `extensionsFilters` allows filtering extension-owned database objects and must resolve to a validated array before introspection

Important RC.1 note:

- when no schema/table filters are supplied in the reviewed RC.1 `pull` path, the effective runtime filter allows all visible schemas/tables for that dialect path
- some docs still describe `public` as the default schema filter
- for Grizzle's RC.1-targeted port, tagged source is canonical over mixed-version docs

Accordingly, this spec treats omitted filters as:

- no explicit schema/table filtering
- all visible schemas and tables are eligible for introspection, subject to dialect-specific visibility rules
- the configured Grizzle migration history table/schema is always excluded from generated schema output and bootstrap snapshots, even when omitted filters otherwise allow all visible objects
- `HistoryOptions` must be resolved for every `pull` so the migration history table/schema can be excluded from introspection output; only `pull --init` may create, read, or write migration history metadata
- filesystem-mutating broad scans require explicit `PullOptions.AllowBroadScan=true` / CLI `--all-schemas`; without that opt-in, `pull` fails with `broad_introspection_requires_opt_in` before broad introspection, file writes, or history metadata

Security note:

- this follows the reviewed RC.1 source behavior, but it can expose internal schemas, tenant-specific objects, extension-owned objects, or sensitive names in generated Go files
- Grizzle must summarize the introspected object set before writing when omitted filters produce a broad scan
- the library summary path is `PullOptions.BeforeWrite`; CLI `pull` must install a callback that prints the required payload-redacted summary before any managed source, bootstrap artifact, or `--init` history metadata write
- users should set `schemaFilter` and `tablesFilter` in shared or multi-tenant databases
- requiring `--all-schemas` / `AllowBroadScan` for filesystem-mutating broad scans is **DEVIATION:INTENTIONAL** safety hardening from RC.1's broad default introspection
- production and multi-tenant workflows should treat omitted filters as unsafe even with the opt-in
- when no schema/table filters are configured and the broad-scan opt-in is present, CLI `pull` must show a pre-write object summary for all filesystem-mutating runs, including non-interactive runs, and must label the broad scan as an intentional RC.1-behavior opt-in with security risk
- CLI output must summarize broad scans with counts and object-family categories only
- the library `PullResult.IntrospectionSummary` must carry the same payload-redacted object summary with `BroadScan=true`, diagnostics must include `DiagnosticBroadIntrospection`, and `Objects` must be nil or empty for broad scans unless a future explicit unsafe library opt-in exposes full object refs; public broad-scan diagnostics in callback plans and results must contain counts/categories only and no full object names in message/path fields. Any future object-ref opt-in must be documented as unsafe to log before it is implemented
- defaults, check expressions, view definitions, and generated artifacts can contain sensitive SQL literals copied from the live database; high-confidence secret-literal findings fail closed by default unless `AllowSecretLiterals=true` / `--allow-secret-literals` is explicitly supplied, and absence of a warning must not be treated as proof that output is secret-free
- broad scans and large schemas must still obey the shared resource limits for object counts, object-name length, per-payload raw default/check/view SQL length, aggregate raw introspection payload bytes (`MaxIntrospectionPayloadBytes`), rendered source bytes, bootstrap artifact bytes, and total planned write bytes; exceeding a limit fails with `resource_limit` before publishing files or writing history metadata. `pull --init` applies the same limits to both the initial introspection and the fresh validation introspection under the lock/stability window.

Plain `pull` lifecycle:

- plain `pull` introspects and writes managed source/bootstrap files only; it must not acquire the migration lock, begin a migration/history transaction, create/read/write the migration history table, or call the runtime migrator
- it must still validate the connected session dialect before introspection and resolve `HistoryOptions` so Grizzle's own history objects can be excluded from generated output
- before introspection, if no schema/table filters are configured and the command will write managed source or bootstrap artifact files, it must require `AllowBroadScan=true` / CLI `--all-schemas`
- only `pull --init` may enter the migration lock/history lifecycle described below

### Init

RC.1 supports `--init` on `pull`.

Its purpose is:

- mark the pulled schema as applied migration metadata in the database
- establish the pulled state as the migration baseline for future generated diffs

Grizzle must preserve this behavior.

`init` is not a request to execute migration SQL. Reviewed RC.1 source calls the runtime migrator in init mode after introspection; that migrator records the sole local migration in `out` when database history is empty, without verifying that the migration is an introspection artifact.

Grizzle intentionally tightens this behavior: `pull --init` may record only a managed introspection artifact that was produced by `pull` and validated against the fresh live database model. This is DEVIATION:INTENTIONAL bootstrap safety hardening.

Grizzle also validates that the live database snapshot still matches the pulled artifact before inserting history metadata. This is DEVIATION:INTENTIONAL bootstrap hardening relative to RC.1, whose reviewed runtime init path does not perform an additional live-schema equality check.

`pull --init` requires a stable schema observation window. The migration history lock coordinates Grizzle migration runners, but it does not necessarily block external DDL issued outside Grizzle.

Normative stability rule:

- `pull --init` must fail with `init_precondition` unless the adapter reports stable schema snapshot/DDL-lock support for the target database, or the user explicitly supplies `--assume-quiescent` / `PullOptions.AssumeQuiescent`
- `--assume-quiescent` is an explicit risk acceptance, not a default; CLI output and `PullResult` diagnostics must state that schema stability was assumed rather than proven
- even with `--assume-quiescent`, `pull --init` must still re-introspect under the pinned migration session, validate live-snapshot equality, and insert history from the exact post-publish checked artifact bytes
- when adapter-proven stability is used, the runner must call `BeginStableSchemaSnapshot` after acquiring the migration lock and must keep that stable window active through fresh introspection, live equality validation, history schema/table creation or validation, empty-history verification, and history insertion; `EndStableSchemaSnapshot` receives the returned state and runs during cleanup after the metadata insert or failure path

## Output Contract

This is where Grizzle intentionally adapts Drizzle's TypeScript output into Go source output.

### Generated Source Files

Drizzle RC.1 commonly writes:

- `schema.ts`
- `relations.ts`

Reviewed RC.1 source shows dialect-specific variation. For example, the MSSQL pull path writes schema source but does not use the same relations-file path.

Grizzle target:

- `schema.go`
- `relations.go`

These files are written into the configured `schema-out` directory.

Normative rules:

- `schema.go` contains Go schema-definition builders derived from the live database
- `relations.go` contains Go relation definitions derived from the live database where relation extraction is supported
- if a dialect has no relation output, `relations.go` must still be generated as a valid managed Go file with no relation declarations
- view output must use the dialect-native typed view DSL: `pg.CreateView(...).As(...)` / `pg.SchemaView(...).As(...)`, `mysql.CreateView(...).As(...)`, or `sqlite.CreateView(...).As(...)`; schema-qualified MySQL view output is outside the initial file-migration target because RC.1 Kit snapshots do not serialize MySQL view schemas
- the view DSL used by `pull` must be the typed DDL-expression view API, not legacy raw-string constructors; generated output should prefer typed `ddl.Select(...)` expressions where possible, and may emit `ddl.RawTrusted(literalSQL)` only when introspection cannot be losslessly represented by the typed DDL expression subset
- unsupported view options or definitions that cannot be represented in the dialect-native Go DSL must fail with `unsupported_feature` rather than being dropped
- both files are managed outputs of `pull`
- subsequent `pull` runs overwrite these files
- overwrites are allowed only when the existing file carries the managed `grizzle pull` header; this is DEVIATION:INTENTIONAL overwrite-safety hardening from RC.1, which writes its schema/relations files directly
- unowned `schema.go` or `relations.go` files must fail with `managed_file_overwrite` rather than being clobbered

Exact managed source header:

```go
// Code generated by grizzle pull v1; DO NOT EDIT.
```

Header detection rules:

- the exact header line must appear in the emitted prelude position, immediately before the `package` clause after only permitted UTF-8 BOM handling, build tags, blank lines, and package documentation
- arbitrary pre-package comments containing the header text do not prove ownership; misleading comments in manually authored `schema.go` or `relations.go` must still fail with `managed_file_overwrite`
- generated files must include the header in that exact prelude position
- only header version `v1` is recognized by the initial implementation
- files without the recognized header are unowned and must not be overwritten in the initial implementation
- overwrite tests must cover missing headers, wrong versions, header text in ordinary comments, and otherwise valid files whose header is not in the exact emitted prelude position

Managed source root safety:

- `schema-out` is resolved independently from `migrations-out`
- existing `schema-out` roots must be `Lstat` checked and symlinked roots rejected unless a future explicit opt-in is designed
- absent `schema-out` roots must be created only after canonicalizing the parent directory, then canonicalized again after creation
- `schema.go` and `relations.go` must be created or replaced with no-follow semantics and must reject symlinks, hardlinks where detectable, non-regular files, and paths escaping `schema-out`
- no force overwrite mode exists in the initial pull target; any future force behavior requires a dedicated spec/API change and must not be inferred from the managed-write API

This overwrite behavior matches upstream RC.1 more closely than the repo's older “edit freely unless force” note.

Practical implication:

- users should not rely on editing the generated files in place if they intend to run `pull` again
- customization should live in separate files or follow a documented regeneration workflow

### Package Naming — DEVIATION:LANGUAGE

Because Go source files require a package, Grizzle must define package naming explicitly.

Target rule:

- generated package name defaults to a sanitized form of the basename of `schema-out`

This is a Go-specific adaptation with no Drizzle equivalent.

Initial command contract:

- no separate package-override flag is required
- basename-of-`schema-out` is the deterministic package input for the initial implementation
- invalid package-name characters are converted to underscores
- package names beginning with a digit are prefixed with `schema_`
- Go keywords are suffixed with `_schema`
- if sanitization produces an empty package name, `pull` fails

### Initial Migration Artifact Generation

This is a critical upstream behavior.

In RC.1:

- `pull` checks the output directory for existing migration snapshots
- if no snapshots exist, it generates an initial introspection migration artifact
- if snapshots already exist, it does not generate SQL again

Grizzle must preserve this behavior.

Target behavior:

- `pull` must run the local artifact check against `migrations-out` before deciding whether the directory is eligible for bootstrap artifact generation; this is DEVIATION:INTENTIONAL hardening beyond RC.1's existing-snapshot check
- if the configured `migrations-out` directory contains no migration snapshots, `pull` must generate:
  - `schema.go`
  - `relations.go`
  - one initial introspection migration directory containing:
    - `migration.sql`
    - `snapshot.json`
- if migration snapshots already exist in `migrations-out`, `pull` must still refresh `schema.go` and `relations.go`
- but it must not generate a new introspection migration artifact in that case

Initial introspection artifact semantics:

- the initial introspection migration represents "this is the database state we pulled"
- it participates in the same snapshot graph, check, and history-table model as generated migrations
- its `snapshot.json` must have a fresh non-sentinel `id` that is not `00000000-0000-0000-0000-000000000000`
- it must have exactly one snapshot root parent in `prevIds`: the sentinel origin UUID
- its snapshot `ddl` must represent the live introspected DDL, and bootstrap diff rename metadata must be stored in `renames`
- it must use the same folder naming, `snapshot.json`, and `migration.sql` artifact shape as other migrations
- it must follow Drizzle RC.1's commented-SQL introspection posture while adding a managed header for bootstrap safety

### Pull Metadata Model

The introspection result used for source generation must be richer than the migration `snapshot.json` DDL array.

Required pull-only metadata:

- view-column metadata for generated Go view/source definitions
- relation-generation metadata derived from foreign keys and unique constraints
- dialect-specific source-generation hints that are validated but not serialized into `snapshot.json`

These records are not additional fields in migration `snapshot.json`. They are `pull`/source-generation inputs analogous to RC.1's dialect-specific `viewColumns` and relation pull structures.

Introspection SQL construction safety:

- catalog metadata queries must use bind parameters for filter values where the database supports them
- when a dialect requires object names inside SQL text, such as MySQL-style `SHOW CREATE VIEW`, the adapter must quote each catalog-derived identifier with the dialect identifier helper
- adapters must not concatenate untrusted catalog names, filter values, or database metadata as raw SQL fragments
- conformance tests must include object names containing quotes, backticks, newlines, comment delimiters, Unicode controls, and delimiter-like text
- generated Go output must quote literal database names with Go string escaping and must use imports from a fixed dialect allowlist only

Root-normalization note:

- reviewed RC.1 source varies by dialect for bootstrap `prevIds`
- Grizzle uses the sentinel-parent root rule for supported dialects as an intentional graph-normalization divergence

### Introspection Migration SQL Form

RC.1 writes introspection SQL as a commented-out migration file rather than executable-by-default migration text.

That is, it generates a file conceptually equivalent to:

- comment header explaining the SQL came from introspection
- SQL wrapped in a block comment until the developer intentionally enables it

Grizzle must preserve the same safety posture for the initial introspection migration artifact, but must not rely on a raw block comment around untrusted introspected SQL.

Normative SQL form:

- generated introspection SQL is inert by default
- generated introspection SQL must start with this exact managed introspection header:

```text
-- grizzle:managed-introspection v1
```

- the managed header is DEVIATION:INTENTIONAL bootstrap-safety metadata; it avoids adding a sidecar kind file while allowing `pull --init` and `migrate` to identify unmodified bootstrap artifacts
- raw introspected SQL must not be wrapped in a block comment without escaping because identifiers or SQL text can contain `*/`, newlines, or delimiter-like content
- Grizzle must use an inert encoding that cannot become executable SQL
- accepted initial inert encoding is line-comment encoding: normalize CRLF/CR to LF, reject NUL bytes and other unsupported control characters, split into physical lines, and prefix every line of introspected SQL payload with `-- `
- the exact managed header line is not part of the introspected SQL payload and must remain the first non-empty physical line
- alternative encodings require a future spec and must include tests proving the payload cannot introduce executable SQL or active delimiter lines
- disabled breakpoint markers must not appear as active full-line `--> statement-breakpoint` delimiters
- normal `migrate` must reject a pending/unrecorded managed introspection bootstrap artifact with `bootstrap_init_required`
- a migration is managed-introspection if the first non-empty physical line of `migration.sql`, after an optional UTF-8 BOM, is exactly `-- grizzle:managed-introspection v1`
- to convert the artifact into a normal custom migration, the developer must remove that header and make the SQL executable intentionally; comment-only SQL with schema-changing snapshot state remains invalid outside `pull --init`
- `pull --init` records the managed introspection artifact's raw `migration.sql` bytes in history without executing them; the SQL is operationally no-op only because it is inert/commented
- `pull --init` records the artifact as applied without executing SQL, matching Drizzle RC.1 init-mode behavior

Bootstrap safety note:

- `pull --init` is required when the intent is to mark an existing database state as already applied
- rejecting managed introspection artifacts in normal `migrate` is DEVIATION:INTENTIONAL bootstrap safety hardening relative to Drizzle RC.1

## Init Contract

After completing introspection output, RC.1 `pull --init` calls the runtime migrator in `init` mode.

Observed RC.1 behavior:

- it points the migrator at the migrations output directory
- it passes configured migration table/schema
- it inserts migration metadata only
- it errors if local migrations already exist in a way incompatible with init
- it errors if the database already has migration metadata set
- reviewed runtime source does not perform an additional live-schema equality check before inserting init metadata

Grizzle target:

- `pull --init` must only be allowed when local outputs and database history are eligible for initial bootstrap
- it must not silently override existing local migration history
- it must not silently override existing database migration metadata
- it must acquire the same DB-backed migration lock required by `migrate`
- it must create the migration history schema/table if absent, using the supported Grizzle history schema
- it must record exactly one local introspection artifact as applied
- it must validate that the local introspection artifact still matches the live database snapshot before recording metadata
- it must identify the artifact from the current `pull` result or from a single existing managed introspection artifact produced by a prior plain `pull` or failed `pull --init`, whose header and snapshot match the fresh live snapshot
- it must not execute the introspection artifact's SQL
- it must not mutate application schema objects
- the target database must either be proven stable by the adapter's DDL-serializing schema-lock/metadata-lock capability or explicitly treated as quiescent through the user-selected `--assume-quiescent` risk-acceptance path
- future Grizzle schema-mutating commands, including `push`, must define compatible locking in their own specs before mutating schema or migration metadata
- the live-snapshot equality check is DEVIATION:INTENTIONAL bootstrap hardening relative to RC.1

Live-snapshot equality rule:

- equality compares canonical dialect, normalized DDL entity payloads, and pull-only source-generation metadata needed to render `schema.go` / `relations.go`, including view-column and relation metadata
- equality excludes volatile snapshot fields: `id`, `prevIds`, and `renames`
- DDL normalization must use the same dialect validator/entity ordering rules used by `check`
- pull-only metadata normalization must use the same ordering and identifier rules used by `RenderSchema`
- mismatch fails with `init_precondition`

Required `pull --init` algorithm:

1. before introspection, if no schema/table filters are configured, require `AllowBroadScan=true` / CLI `--all-schemas`
2. introspect the live database, enforcing pull resource limits while collecting metadata
3. render `schema.go` and `relations.go` into staging
4. run local artifact check on the current `migrations-out`
5. if check reports zero snapshots, prepare one staged bootstrap introspection artifact from the first live snapshot
6. otherwise, if exactly one existing managed introspection artifact is present from a prior plain `pull` or failed `pull --init`, prepare to reuse it after fresh live-state validation and do not stage a new artifact
7. otherwise, fail with `init_precondition`
8. verify that the adapter reports stable schema snapshot/DDL-lock support, or that the user explicitly selected `--assume-quiescent`
9. acquire the DB-backed migration lock on a pinned migration session
10. if adapter-proven stability is used, begin the stable schema snapshot/lock lifecycle on that pinned session and retain the returned `StableSchemaSnapshot` state
11. begin the history metadata transaction with the adapter's supported `TransactionMode`, unless the returned stable-schema state has `OwnsTransaction=true`
12. re-introspect the live database, or otherwise obtain an equivalent fresh live snapshot under the active connection/session constraints
13. validate that the staged or reused introspection artifact snapshot and pull-only source-generation metadata equal the fresh live database model
14. re-render `schema.go` and `relations.go` from the fresh live model that passed validation, replacing any earlier staged source output, and re-check resource limits for rendered source bytes and planned artifact bytes
15. call `PullOptions.BeforeWrite` when configured, using the fresh pre-write plan and limit-status summary; if it errors, abort before schema-file publish, bootstrap artifact publish, history schema/table creation, or history insert
16. create or validate the migration history schema/table under the lock, transaction, and stable schema window when present
17. require that the history table has no recorded migrations
18. publish staged `schema.go` and `relations.go`; if the chosen branch staged a new bootstrap artifact, publish only that artifact through the artifact store. If atomic publication is unavailable, fail closed unless a future fallback spec preserves no-discoverable-partial, no-follow, revalidation, fsync, and digest invariants.
19. run local artifact check again against the published `migrations-out`
20. require exactly one local managed introspection artifact from this invocation, or exactly one existing managed introspection artifact from a previous plain `pull` or failed `pull --init`
21. insert one row for the introspection artifact with its name, raw-byte SHA-256 hash of `migration.sql`, `created_at`, and current `applied_at`
22. commit the history metadata transaction through `MigrationSession.Commit`; this applies both when the caller began the transaction explicitly and when `StableSchemaSnapshot.OwnsTransaction=true` caused `BeginStableSchemaSnapshot` to open it
23. end the stable schema snapshot/lock lifecycle when present, then release the migration lock

The history row insertion must use the `name`, `created_at`, and hash from the exact post-publish `LoadedArtifact` bytes or stable no-follow file handle returned by the final post-publish artifact check. `pull --init` must not compute the hash from a fresh best-effort path read after validation.

If any step after beginning the history metadata transaction fails, `pull --init` must roll back that transaction through `MigrationSession.Rollback` when the adapter can roll it back. This applies both to explicitly begun transactions and transactions opened by `BeginStableSchemaSnapshot`. If rollback cannot guarantee removal of a partially inserted history row, the command must fail with `partial_application` and include history-stage details under the shared partial-application contract.

Failure cleanup:

- staged local generated files must not be published before lock acquisition and live-state validation have succeeded
- local generated files may be left on disk after a publish-stage or database metadata failure, but they must remain managed outputs that a later `pull` can overwrite safely
- if artifact creation fails, no valid partial migration directory may be left behind
- if metadata insertion fails after the local bootstrap artifact is written, a later `pull --init` may reuse that single managed introspection artifact only if it still matches the fresh live snapshot

Failure conditions:

- local migrations already exist in a way incompatible with init bootstrap
- no introspection artifact exists after bootstrap generation
- more than one local migration snapshot exists
- the local introspection artifact hash/name does not match the artifact that would be recorded
- the live database snapshot no longer matches the pulled introspection snapshot
- database already has recorded migration metadata
- target dialect/driver cannot support init-mode migration metadata bootstrap

## Read/Write Boundaries

`pull` without `--init` is database-read / filesystem-write.

Normative rules:

- `pull` reads live database schema
- `pull` writes local generated source files
- `pull` may write local bootstrap migration artifacts to `migrations-out` under the rules above
- `pull` must not mutate live application schema
- `pull` must not apply migration SQL
- normal `pull` must not write database migration history

`pull --init` is the only case where `pull` may write migration metadata to the database, and even then:

- it writes migration bookkeeping only
- it may create or write the Grizzle migration history schema/table
- it does not alter application schema objects

## Relationship to Other Commands

- `pull` is the reverse of schema-to-migration generation
- `pull` is not a replacement for `generate`
- `pull` is not a replacement for `migrate`
- `pull` is not the same as `gen`

Recommended sequence after `pull`:

1. review `schema.go`
2. review `relations.go`
3. if this is a bootstrap workflow, review the initial introspection migration artifact if one was generated
4. if recording the pulled state as already applied, run `pull --init` only when the init preconditions are satisfied
5. use `generate` for subsequent schema-driven migration work

## Errors

At minimum, `pull` must fail clearly for:

- unsupported dialect/driver combinations
- missing required DB credential references or unresolved secrets
- database connection failures
- introspection failures
- invalid or unsupported output bootstrap state for `--init`
- missing or duplicate local introspection artifacts during `--init`
- live database drift between introspection and init metadata recording
- non-empty existing database migration history during `--init`
- inability to acquire the migration lock
- filesystem write failures

All error messages and diagnostics in this section must apply the shared credential-redaction and SQL-output redaction rules before reaching CLI output, logs, or returned public errors.

## Go-Specific Generation Rules

Because Drizzle emits TypeScript and Grizzle emits Go, `pull` must define a few Go-specific contracts explicitly.

### File Names

Initial contract:

- `schema.go`
- `relations.go`

No table-per-file generation is required for the initial implementation.

### Identifier Rules

Generated Go identifiers must be valid Go identifiers.

Normative rules:

- exported top-level table variables must use PascalCase names
- relation identifiers must follow the selected casing strategy, adapted into valid Go identifiers
- invalid identifier characters must be normalized into valid Go identifier forms
- if normalization would cause a collision between generated identifiers, `pull` must fail with a deterministic identifier-collision error
- generated Go string literals, raw SQL fragments, and comments must be emitted with safe Go quoting/escaping
- database names containing quotes, backticks, newlines, comment delimiters, Unicode control characters, or delimiter-like SQL text must not be able to break generated Go syntax or create misleading comments
- generated imports must come from a fixed allowlist determined by the dialect renderer, not from database metadata

### Reserved Words

If a generated identifier would conflict with a Go keyword or predeclared identifier in a way that would make the file invalid:

- `pull` must normalize the identifier deterministically
- if deterministic normalization still produces a collision, `pull` must fail

### Multi-Schema Naming

When introspecting multiple schemas:

- literal database schema names must be preserved in the schema builder metadata
- generated Go identifiers must remain unique across all emitted tables and relations
- if two objects from different schemas would map to the same Go identifier, `pull` must fail with `identifier_collision`; deterministic multi-schema disambiguation is future scope until a specific algorithm is specified

## Machine-Readable Diagnostics

Initial scope decision:

- the library `Pull` API returns a structured `PullResult`
- the CLI may render human-readable output only
- no JSON output mode is required before the initial `pull` implementation
