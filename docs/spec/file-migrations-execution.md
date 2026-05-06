# File Migrations Execution Specification

## Status

Draft

## Purpose

Define exactly how Grizzle executes file-based migrations.

Pinned upstream target:

- Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`

## Scope

This document must define:

- pending migration detection
- execution order
- transaction boundaries
- statement segmentation
- failure behavior
- dialect-specific behavior
- malformed artifact handling
- concurrency expectations

## Upstream References

- Drizzle Kit migrate docs
- Drizzle ORM dialect migrators
- [file-migrations-upstream-mapping.md](./file-migrations-upstream-mapping.md)

## Pending Detection

Upstream RC.1 runtime:

- reads migration folder artifacts from disk
- reads DB migration history
- computes pending migrations by folder `name` using `getMigrationsToRun()`

Grizzle must define pending detection from:

- artifact metadata
- history table state

not from bare SQL-file enumeration alone.

Initial scope rule:

- execution only supports the canonical RC.1-style artifact format
- execution only supports the canonical RC.1-style history-table shape
- no automatic upgrade/conversion is attempted during execution

Target pending detection rule:

- pending migrations are determined by migration `name`
- a migration is pending if its artifact identity is absent from the applied-history set
- hash is not the primary pending-detection key

Normative pending-detection algorithm:

1. discover and validate local migration artifacts
2. read the configured history table
3. validate database history consistency
4. build the set of applied migration `name` values
5. preserve the ordered local migration list
6. mark as pending every local migration whose `name` is absent from the applied set

Execution must not:

- infer pending state from root-level `.sql` files
- infer pending state from `created_at` alone
- treat a matching `hash` with a different `name` as "already applied"
- continue when the database contains an applied migration name that is absent from local artifacts

History consistency rules:

- duplicate database history `name` values are invalid
- database history rows with malformed `name`, `created_at`, or `hash` are invalid
- any database row whose `name` exists locally is treated as already applied even if its stored `hash` differs from the current local artifact hash
- any database row whose `name` is absent from the local artifact set must fail by default with `history_artifact_missing`
- unsupported history-table shape fails before pending detection
- failing on database history rows absent from local artifacts is DEVIATION:INTENTIONAL deployment-state hardening relative to Drizzle RC.1, which only filters local migrations by database names

Empty-artifact safety:

- `check` may treat an absent or empty migrations directory as a valid empty graph for first-run generation
- `migrate` must not silently treat an absent migrations directory as a successful no-op
- `migrate` must fail with `empty_migrations_dir` when the configured artifact root is absent or contains zero valid migration artifacts unless an explicit `AllowEmpty` / `--allow-empty` option is supplied
- `AllowEmpty` / `--allow-empty` is permitted only when the database history table is absent or contains zero rows
- if database history contains any row and the local artifact set is absent or empty, `migrate` must fail with `history_artifact_missing` even when `AllowEmpty` is true
- the controlled no-op path must not create a missing history schema/table; it may inspect an existing history table, but an absent table remains absent
- this is DEVIATION:INTENTIONAL deployment safety hardening relative to Drizzle's more permissive folder scan behavior

State table:

| Local artifact set | Database history | `AllowEmpty` false | `AllowEmpty` true |
| --- | --- | --- | --- |
| absent or empty | absent or empty | fail `empty_migrations_dir` without creating history | succeed as controlled no-op without creating history |
| absent or empty | one or more rows | fail `history_artifact_missing` | fail `history_artifact_missing` |
| one or more artifacts | any supported history state | normal pending detection | normal pending detection |

## Execution Order

Execution order must copy RC.1 closely:

- discover valid migration directories
- sort them by migration directory name
- derive identity from full directory name
- derive chronological ordering from the timestamp prefix
- execute pending migrations in ordered sequence

Normative execution-order rules:

- candidate migrations must be sorted lexicographically by full directory name
- only pending migrations are executed
- execution order follows the sorted pending list exactly
- execution must not reorder migrations based solely on database timestamps or insertion order
- execution must not skip malformed earlier migrations and continue with later ones

## Transaction Boundaries

Drizzle RC.1 does not use one universal transaction model across every driver. Its behavior is:

- PostgreSQL async drivers: wrap the full pending-migration batch in a single transaction
- MySQL: wrap the full pending-migration batch in a single transaction
- SQLite sync: use explicit `BEGIN` / `COMMIT` / `ROLLBACK` around the full pending-migration batch
- SQLite async: wrap the full pending-migration batch in a single transaction
- Neon HTTP: does not support transactions, so migrations run without rollback protection

Grizzle must copy the upstream intent closely where the driver can actually provide it:

- use a single transaction for the full pending-migration batch when the driver/dialect supports it
- treat non-transactional execution as an explicit driver capability exception, not the default model
- document any non-transactional drivers clearly in dialect-specific docs

Normative rules:

- for drivers that can provide real transactional DDL semantics, one `migrate` run applies all pending migrations in one enclosing transaction
- within that transaction, each migration's statements execute before its history row is inserted
- if any statement fails, the enclosing transaction must abort
- if a driver cannot provide the required transaction behavior, that limitation must be explicit and surfaced to the caller
- MySQL DDL may cause implicit commits, so MySQL must be documented as partial-application-capable even if a transaction is opened around the run
- transaction behavior is a driver capability, not only a dialect name
- `MigrationSession.Capabilities()` is the authoritative source for driver-level transaction, lock, future whole-file execution, and partial-application behavior
- a command must fail before executing SQL when the selected adapter lacks a required capability

## Statement Segmentation

Upstream Drizzle runtime uses explicit statement breakpoints, not raw semicolon splitting.

That means Grizzle must treat naive `strings.Split(sql, ";")` as explicitly non-target behavior for generated artifact execution.

Normative rules:

- `migration.sql` must be parsed using the explicit `--> statement-breakpoint` delimiter contract
- `migration.sql` bytes must first satisfy the UTF-8, BOM, NUL, and control-character policy in [file-migrations-artifacts.md](./file-migrations-artifacts.md#sql-segmentation-format)
- the delimiter is a full line whose trimmed content is exactly `--> statement-breakpoint`
- CRLF, LF, and standalone CR line endings are supported for delimiter-line detection by normalizing CRLF and CR to LF during detection only
- when delimiter lines are present, each segment between delimiters is one executable statement payload
- when delimiter lines are absent, the whole file is one execution payload, matching Drizzle RC.1 runtime behavior
- adapters must enforce that each executable payload contains at most one statement according to the adapter's safe execution capability; if an adapter cannot prove or enforce that a payload is single-statement, it must reject the segment with `unsupported_feature` rather than executing multiple statements in one driver call. This is **DEVIATION:INTENTIONAL** SQL-safety hardening relative to RC.1, which forwards delimiter chunks without this cross-adapter guarantee.
- execution must preserve statement text exactly apart from delimiter handling
- semicolons inside SQL text must not be used as the general-purpose segmentation mechanism
- delimiter recognition must operate on active SQL text only; disabled introspection payloads must not contain exact delimiter lines
- empty executable segments are invalid; this is DEVIATION:INTENTIONAL SQL-safety hardening relative to RC.1's raw delimiter split
- comment-only executable segments are allowed only when the artifact snapshot `ddl` is unchanged from its effective parent, or when `pull --init` is recording a managed introspection baseline. A comment-only segment is conservatively defined as optional UTF-8 BOM plus blank lines and line comments whose first non-space text is `--`; block comments, raw comment delimiters inside SQL text, and dialect-specific comment forms are ambiguous and must not be treated as comment-only for schema-changing artifacts.
- normal `migrate` must reject pending no-op/comment-only artifacts whose `snapshot.json` changes schema state from the effective parent; this prevents bypassing `pull --init` live validation by deleting the managed introspection header

Breakpoint-disabled execution rule:

- disabled-breakpoint artifacts are out of initial implementation scope
- the initial generator must reject `breakpoints=false`, so supported generated multi-statement artifacts always contain explicit delimiters
- custom SQL that is intended to execute as multiple payloads must include explicit delimiter lines
- marker-free SQL is passed to the driver as one execution payload; adapters must not enable driver-level multi-statement execution for that payload unless a future disabled-breakpoint design explicitly allows it
- adapters must include conformance tests proving a marker-free two-statement payload cannot execute as two statements; adapters that cannot enforce this must reject marker-free executable artifacts with `unsupported_feature`
- semicolon splitting remains forbidden
- future support must add explicit statement-count metadata or a proven SQL parser/executor strategy before relying on `MigrationCapabilities.SupportsWholeFileMultiStatement`

Delimiter strictness note:

- RC.1 runtime uses raw string splitting on the delimiter substring
- Grizzle recognizes only active full-line delimiters so inert `pull` SQL cannot become executable through delimiter-like text in commented payloads
- this is DEVIATION:INTENTIONAL SQL-safety hardening

## Failure Semantics

Failure behavior follows from the transaction model:

- for transactional drivers, a failed statement must abort the migration run and roll back the enclosing transaction
- for non-transactional drivers, Grizzle must fail immediately and clearly document that partial effects may already have been applied
- migration history rows must only be recorded for migrations considered successfully applied under the active driver model

At minimum, failure conditions must include:

- unsupported migration artifact layout
- missing `migration.sql` in an expected migration directory
- unsupported history-table schema
- invalid statement segmentation
- applied history rows whose `name` is absent from local artifacts

Additional failure rules:

- if the history table cannot be read or validated, execution must fail before running statements
- if `check` is required by command contract and has not succeeded, the command must fail rather than proceed unsafely
- if a migration directory is missing `snapshot.json`, execution must fail as unsupported artifact state even if SQL text is present
- if a pending artifact is managed-introspection, normal `migrate` must fail with `bootstrap_init_required`; use `pull --init` to record that baseline after live-state validation
- if a managed-introspection artifact is already recorded in history by `name`, it is skipped like any other applied migration
- an artifact is managed-introspection when the first non-empty physical line of `migration.sql`, after an optional UTF-8 BOM, is exactly `-- grizzle:managed-introspection v1`
- if a developer intentionally edits an introspection artifact into executable migration SQL and removes that header, normal `migrate` treats it as a normal custom migration artifact

Partial-application failure contract:

- if a driver is partial-application-capable, for example MySQL DDL with implicit commits, Grizzle must stop on the first execution or history-insert failure
- if DDL may have committed but the history row was not inserted, the command must fail with `partial_application` and must not continue to later migrations
- the error must include the migration `name`, the statement index when known, and whether history insertion had started
- docs and CLI output must direct operators to inspect the database and either manually repair the schema/history row or restore from backup before rerunning
- Grizzle must not mark a partially applied migration as successful automatically

Applied-history hash rule:

- Grizzle follows Drizzle RC.1 pending detection by `name`
- if the database reports an applied migration `name`, that local artifact is skipped even when the stored `hash` differs from the current local `migration.sql`
- hash differences for already-applied migrations are metadata differences, not execution blockers
- this is a parity decision with a security tradeoff: applied migration artifacts must be treated as immutable through source control review
- stricter applied-hash verification is future audit tooling, not the default RC.1-aligned pending-detection behavior
- conformance tests must assert that an applied migration with matching `name` is skipped even when the stored hash differs, while separate check/audit tests may surface the drift as a diagnostic

## Validation Prerequisite

Grizzle must follow the RC.1 pattern of treating consistency checking as part of the normal workflow.

Target rule:

- `check` must run before `generate`
- `check` must run before `migrate`

Command enforcement rule:

- `generate` and `migrate` command handlers must invoke local artifact `check` internally before mutating local artifacts or the database
- CI may also run `check`, but CI is not the enforcement boundary
- the initial design has no check-bypass or conflict-bypass flag; omitting Drizzle RC.1's `--ignore-conflicts` is DEVIATION:INTENTIONAL safety hardening

## Dialect-Specific Notes

### PostgreSQL

- default target behavior is transactional execution for the full pending batch
- configurable migration schema and table are in scope
- non-transactional exceptions, if any, must be tied to specific drivers rather than the PostgreSQL dialect as a whole
- the configured migration schema must be created before the history table when it does not exist

### MySQL

- default target behavior is to open a transaction for the full pending batch where the driver allows it
- MySQL DDL may still cause implicit commits, so MySQL execution must be documented and surfaced as partial-application-capable

### SQLite

- default target behavior is transactional execution for the full pending batch
- sync and async implementations may differ internally, but the user-visible contract must remain the same where possible
- SQLite's `BEGIN IMMEDIATE` is both the transaction start and the migration lock for the configured database connection
- SQLite adapters must model this explicitly: `AcquireLock` performs `BEGIN IMMEDIATE` and records that the session is already inside the lock transaction; `Begin(Immediate)` after that must verify the active immediate transaction and must not issue a second `BEGIN`

## Concurrency Model

Grizzle intentionally diverges from Drizzle RC.1 runtime behavior here.

Upstream basis:

- RC.1 runtime migration paths reviewed for PostgreSQL, MySQL, SQLite, and Neon HTTP do not acquire an explicit migration lock around the full history-read, pending-computation, execution, and history-insert sequence
- upstream issue `drizzle-team/drizzle-orm#874` tracks simultaneous `migrate()` execution as an open `bug` / `priority` issue
- this is therefore specified as intentional upstream bug-fix / concurrency hardening, not as Drizzle parity

Normative goals:

- only one migrator may execute against a configured migration target at a time
- only one runner may record a given migration `name`
- concurrent runners must not both record success for the same migration identity

Initial guarantees:

- `migrate` must acquire a database-backed migration lock before reading history, computing pending migrations, executing SQL, or inserting metadata
- `pull --init` must acquire the same logical lock before validating the live baseline and inserting bootstrap metadata
- future `push` locking belongs to the dedicated direct-sync spec; it must not silently bypass migration safety, but its lock identity may differ from the file-migration history lock
- lock acquisition, history reads, pending computation, SQL execution, history inserts, and lock release must use the same pinned migration session or equivalent physical connection
- history schema/table creation and validation must also happen under the same lock and pinned session before history is read
- failure to acquire the lock must abort the migration run
- `UNIQUE(name)` in the history table is required
- a duplicate insert caused by concurrent runners must fail rather than create duplicate history rows
- the executor must treat duplicate-history insertion during a migrate run as a concurrency error, not as success

Dialect lock requirements:

- PostgreSQL must use `pg_try_advisory_lock(<lock-id>)` in a retry loop scoped to the configured history schema/table identity, then release with `pg_advisory_unlock(<lock-id>)`, or a documented equivalent with the same exclusion, timeout, pinned-session, and release guarantees
- MySQL must use `GET_LOCK(<lock-name>, <timeout-seconds>)` and release with `RELEASE_LOCK(<lock-name>)`, or a documented equivalent with the same exclusion, timeout, pinned-session, and release guarantees
- SQLite must rely on transaction/database write locking where it provides equivalent single-writer behavior, using `BEGIN IMMEDIATE` or an equivalent driver-supported mode before history reads
- any driver that cannot provide a lock must surface that limitation and fail unless explicitly documented as unsupported for safe concurrent migration execution
- lock acquisition must have a finite timeout or caller-controlled context deadline
- timeout must fail with `migration_lock` rather than blocking forever

The history `UNIQUE(name)` constraint remains required as a second line of defense, not as the primary concurrency control.

Lock identity derivation:

- the logical lock identity is `grizzle:migrate:<dialect>:<target-id>:<history-schema>:<history-table>`
- `<target-id>` is a non-secret database/catalog identity or stable target fingerprint for dialects that can share one lock namespace across databases
- the adapter must derive this value through `MigrationSession.LockIdentity(ctx, HistoryOptions)` after connecting; callers must not guess it from a DSN string
- PostgreSQL may use the current database name or an equivalent non-secret catalog identity
- MySQL must include the active database/catalog name because `GET_LOCK` names are scoped to the server, not to one schema
- SQLite uses the canonical database file identity when available, but its actual exclusion comes from the write lock rather than the string key
- history schema is the empty string for dialects without schema-qualified history tables
- PostgreSQL advisory locks derive a signed 64-bit integer from the first eight bytes of SHA-256 over the logical lock identity, interpreted big-endian
- MySQL `GET_LOCK` uses a deterministic 64-character ASCII lock name because MySQL user-level lock names are limited to 64 characters
- MySQL lock-name format is `grz-mig-` plus the first 56 lowercase hexadecimal characters of SHA-256 over the UTF-8 logical lock identity
- MySQL example: logical identity `grizzle:migrate:mysql:appdb::__grizzle_migrations` maps to `grz-mig-d0ebf66c99aab37b913efc7c94ee8d217f2a5b17999081773fcd309`
- tests must cover max-length enforcement, deterministic hashing, distinct history-table names, distinct active databases, and non-secret output
- SQLite relies on the migration transaction/write lock for the configured database connection and does not need a separate string lock key

## History Table Creation

Before reading history or running pending migrations, `migrate` must establish or validate the configured history table under the active migration lock and pinned session, except for the controlled empty-artifact no-op path where missing history must remain uncreated.

Normative rules:

- absent history table is a valid first-run state and must be created with the supported schema
- existing table with the supported schema is valid
- existing table with an unsupported schema must fail without automatic upgrade
- PostgreSQL must create the configured history schema when absent
- MySQL and SQLite do not support PostgreSQL-style history schema names; non-empty schema override must fail for those dialects
- history table/schema identifiers must be quoted through the dialect identifier-quoting API
- configured table/schema names must be single identifiers, not dotted SQL fragments

## Locking Implementation Notes

The initial lock-key derivation and lock primitives are specified above. Dialect-specific implementation docs may add driver-specific details, but they must not weaken the lock timeout, pinned-session, or release guarantees.
