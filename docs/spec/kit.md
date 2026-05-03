# Migration Kit Specification

The migration kit is the Go equivalent of [Drizzle Kit](https://orm.drizzle.team/docs/kit-overview).

## Target workflow

Drizzle Kit's workflow is the target. It separates concerns into discrete steps:

```
schema definitions
       ↓
   generate          ← produce SQL migration files (inspectable, editable)
       ↓
  (optional edit)    ← developer modifies the generated SQL before applying
       ↓
   migrate           ← apply pending migration files to the database
```

`push` is a shortcut that collapses generate + migrate into one step. It is intended for **development only** — it applies changes directly without producing a file. Drizzle's own docs warn against using `push` in production.

### Why this matters

Without the generate step, schema changes that require human judgement (e.g. `ALTER COLUMN … TYPE` requiring a `USING` clause, or column renames that are ambiguous between rename and drop+add) are applied immediately without any opportunity to review or edit the SQL. This was the root cause identified in PR #47.

---

## Current CLI

These commands exist today (`grizzle help` shows them all):

| Command | What it does | Notes |
|---|---|---|
| `grizzle gen` | Schema definitions → typed Go structs | See [codegen.md](./codegen.md) |
| `grizzle sql` | Schema definitions → `CREATE TABLE` SQL on stdout | Fresh DB init; no DB connection |
| `grizzle snapshot` | Schema definitions → JSON snapshot file on disk | Saves current schema state |
| `grizzle diff` | Schema vs snapshot → migration SQL on stdout | No DB connection; no file written |
| `grizzle migrate` | Schema vs live DB → applies DDL + records history | Introspects live DB; see note below |
| `grizzle status` | Shows applied migration history + pending changes | Read-only |

**Important:** `grizzle migrate` (and `kit.Migrate`) currently does **not** read `.sql` migration files. It introspects the live database, computes a diff against the schema definitions, and applies immediately — recording the result in `_grizzle_migrations`. This is **DEVIATION:BROKEN** relative to the Drizzle-parity target described below.

`grizzle diff` is the closest current equivalent to Drizzle's `generate` — it computes SQL and prints it — but it does not write a file, does not maintain a migrations directory, and does not track what has been applied.

---

## Target commands

### `generate` — DEVIATION:GAP (not designed)

**Drizzle:** `drizzle-kit generate` computes the diff between the current snapshot and the schema definitions and writes one or more `.sql` migration files to the configured `out` directory. It also updates the snapshot journal.

**Grizzle target:**
```sh
grizzle generate [--out ./migrations] [--schema ./schema] [--dialect postgres|mysql|sqlite]
```

- Reads schema definitions from Go source
- Loads the last snapshot from `./migrations/meta/snapshot.json` (or treats as empty if none)
- Computes diff
- Writes a new numbered `.sql` migration file (e.g. `0003_add_users_table.sql`). Sequence numbers are zero-padded to four digits (`0001`, `0002`, …). The next number is `max(existing prefix) + 1`, or `0001` if the directory is empty. Duplicate sequence prefixes are rejected.
- Updates `./migrations/meta/snapshot.json`
- Does **not** connect to a database; does **not** apply anything

The generated file must be readable and editable before applying. This is the critical missing piece of the Drizzle-parity workflow.

Library API target:
```go
result, err := kit.Generate(defs, kit.GenerateOptions{
    OutDir:  "./migrations",
    Dialect: dialect.Postgres,
})
// result.SQL      — the generated statements
// result.FilePath — path of the written .sql file
```

### `migrate` — DEVIATION:BROKEN (current) → target below

**Drizzle:** `drizzle-kit migrate` reads `.sql` files from the migrations directory that have not yet been applied (checked against the journal) and applies them in order.

**Grizzle target:**
```sh
grizzle migrate [--migrations ./migrations] [--db <dsn>] [--dialect postgres|mysql|sqlite]
```

- Reads pending `.sql` files from the migrations directory
- Applies them in order in a transaction
- Records each applied migration in `_grizzle_migrations`

Library API target:
```go
result, err := kit.Migrate(ctx, pool, kit.MigrateOptions{
    MigrationsDir: "./migrations",
})
```

**Transition plan:** See [History table transition plan](#history-table-transition-plan) below. The `_grizzle_migrations` schema gains a `tag` column; the upgrade is automatic on first run. Existing checksum-based rows are preserved. Deployments switching from the old workflow must run `grizzle migrate --baseline <tag>` once to avoid re-applying already-applied SQL.

### `push` — PARITY (in intent)

**Drizzle:** `drizzle-kit push` computes the diff and applies directly to the database without generating migration files. Intended for development only.

**Grizzle current state:** `kit.Push` / `grizzle push` (note: `push` is a library function only; no CLI command exists yet — **DEVIATION:GAP (designed)**):

```go
result, err := kit.Push(ctx, pool, tables...)
```

The library function exists and works. A `grizzle push` CLI command should be added as a thin wrapper, matching Drizzle's `drizzle-kit push` flags.

### `pull` — DEVIATION:GAP (not designed)

**Drizzle:** `drizzle-kit pull` (formerly `introspect`) connects to a live database and generates a Drizzle schema file from the current schema.

**Grizzle target:** `grizzle pull` generates Go schema definitions (`schema/pg` builder calls) from a live database.

```sh
grizzle pull --db <dsn> --out ./schema/db [--dialect postgres|mysql|sqlite]
```

Introspection exists internally (`kit/introspect`). The pull command (introspect → Go schema code generation) is a separate, unimplemented step.

### `check` — DEVIATION:GAP (not designed)

**Drizzle:** `drizzle-kit check` validates that all schema changes have a corresponding migration file and that the migrations directory is consistent.

**Grizzle target:** `grizzle check` — same behaviour. Only meaningful once `generate` is implemented.

### `studio` — out of scope

Drizzle Studio is a visual schema browser. No equivalent is planned for Grizzle.

---

## Migration files

### Directory layout

```
migrations/
  0001_initial_schema.sql
  0002_add_realms.sql
  0003_add_users_email_index.sql
  meta/
    snapshot.json      ← current schema snapshot (used by generate to compute next diff)
```

This layout mirrors Drizzle Kit's migrations directory. It does not currently exist — it is the target once `generate` is implemented.

### Snapshot format

`meta/snapshot.json` is the serialised `kit.Snapshot` struct written by `grizzle snapshot` today and by `grizzle generate` in the future. Current shape:

```json
{
  "version": "1",
  "created_at": "2026-03-15T00:00:00Z",
  "tables": {
    "users": {
      "name": "users",
      "schema": "",
      "columns": [...],
      "constraints": [...]
    }
  }
}
```

Drizzle maintains a dialect-specific snapshot alongside the journal. Grizzle's snapshot is dialect-agnostic (it captures the schema definition, not rendered SQL). Whether to adopt Drizzle's per-dialect snapshot format is **DEVIATION:GAP (not designed)**.

---

## Diff engine

### Change types

| Change | DDL | Status |
|---|---|---|
| New table | `CREATE TABLE …` | PARITY |
| Dropped table | `DROP TABLE IF EXISTS …` | PARITY |
| New column | `ALTER TABLE … ADD COLUMN …` | PARITY |
| Dropped column | `ALTER TABLE … DROP COLUMN …` | PARITY |
| Column type change | `ALTER TABLE … ALTER COLUMN … TYPE …` | PARITY (SQL); see `USING` note below |
| Column nullability change | `ALTER TABLE … ALTER COLUMN … SET / DROP NOT NULL` | PARITY |
| Column default change | `ALTER TABLE … ALTER COLUMN … SET DEFAULT / DROP DEFAULT` | PARITY |
| Column rename | `ALTER TABLE … RENAME COLUMN … TO …` | DEVIATION:GAP — see rename detection below |
| Table rename | `ALTER TABLE … RENAME TO …` | DEVIATION:GAP — see rename detection below |
| New index | `CREATE [UNIQUE] INDEX …` | PARITY |
| Dropped index | `DROP INDEX …` | PARITY |
| New constraint | `ALTER TABLE … ADD CONSTRAINT …` | PARITY |
| Dropped constraint | `ALTER TABLE … DROP CONSTRAINT …` | PARITY |

### Column type changes and `USING`

Both Drizzle Kit and Grizzle emit `ALTER COLUMN … TYPE <newtype>` without a `USING` clause. PostgreSQL rejects casts that are not implicitly castable.

The workflows differ:
- **Drizzle:** SQL is written to a migration file; the developer can add `USING <expr>` before applying.
- **Grizzle (current `push`/`migrate`):** SQL is applied immediately — the missing `USING` clause surprises at apply time.

Once `generate` is implemented, the same edit opportunity exists. Until then, `push` and `migrate` should emit a warning when column type changes are present in the diff.

### Rename detection — DEVIATION:GAP (not designed)

Without rename detection, a column or table rename appears as drop + add, causing data loss. This is a **data-loss risk** in the current implementation.

Drizzle Kit handles this by prompting the user interactively during `push`. Grizzle must implement equivalent detection. Options (decision required before Kit is considered production-ready):

1. **Interactive prompt** — CLI asks "Did you rename column X to Y?" during `push` and `generate`
2. **Schema annotation** — `pg.C("new_name", ...).RenamedFrom("old_name")` expresses the rename explicitly in the schema definition
3. **Both** — annotation takes priority; prompt as fallback

---

## History table

### Current schema (checksum-based)

The `_grizzle_migrations` table (PostgreSQL) is created automatically on first use by the current `kit.Migrate`:

```sql
CREATE TABLE IF NOT EXISTS _grizzle_migrations (
    id          BIGSERIAL    PRIMARY KEY,
    applied_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    checksum    TEXT         NOT NULL,
    sql_batch   TEXT         NOT NULL,
    description TEXT         NOT NULL DEFAULT ''
);
```

This is the equivalent of Drizzle's `__drizzle_migrations` table. Each row records the SHA-256 checksum and full SQL batch of a single live-diff apply. MySQL and SQLite equivalents exist in `kit/migrate_mysql.go` and `kit/migrate_sqlite.go`.

### Target schema (file-based)

Once `generate` and the file-based `kit.Migrate` are implemented, the table gains a `tag` column (migration filename stem) and an `is_baseline` column (distinguishes normal applies from baselined rows — see [transition plan](#history-table-transition-plan)):

```sql
CREATE TABLE IF NOT EXISTS _grizzle_migrations (
    id           BIGSERIAL    PRIMARY KEY,
    applied_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    tag          TEXT         NOT NULL DEFAULT '',
    checksum     TEXT         NOT NULL DEFAULT '',
    sql_batch    TEXT         NOT NULL DEFAULT '',
    description  TEXT         NOT NULL DEFAULT '',
    is_baseline  BOOLEAN      NOT NULL DEFAULT FALSE
);
```

The file-based `kit.Migrate` uses `tag` — not `checksum` — to determine which migration files are already applied. `checksum` is the SHA-256 of the `.sql` file's byte contents (not the diff output). `sql_batch` retains the applied SQL for audit purposes. `is_baseline = TRUE` means the row was inserted by `--baseline` (no SQL was executed).

MySQL target:

```sql
CREATE TABLE IF NOT EXISTS _grizzle_migrations (
    id           INT AUTO_INCREMENT PRIMARY KEY,
    applied_at   DATETIME(6)     NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    tag          VARCHAR(255)    NOT NULL DEFAULT '',
    checksum     VARCHAR(64)     NOT NULL DEFAULT '',
    sql_batch    LONGTEXT        NOT NULL DEFAULT '',
    description  TEXT            NOT NULL DEFAULT '',
    is_baseline  TINYINT(1)      NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
```

SQLite target:

```sql
CREATE TABLE IF NOT EXISTS _grizzle_migrations (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    applied_at   TEXT     NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    tag          TEXT     NOT NULL DEFAULT '',
    checksum     TEXT     NOT NULL DEFAULT '',
    sql_batch    TEXT     NOT NULL DEFAULT '',
    description  TEXT     NOT NULL DEFAULT '',
    is_baseline  INTEGER  NOT NULL DEFAULT 0
)
```

---

## History table transition plan

This section covers how an existing deployment running the old checksum-based `kit.Migrate` upgrades to the file-based workflow.

### Tag format validation

`tag` values (from migration filenames and from the `--baseline` argument) are validated against `^[a-zA-Z0-9_-]+$` before any database operation. The library (`kit.Migrate`) enforces this regardless of whether the tag originates from the CLI or from Go code. Tags that fail validation are rejected with an error; no database operation is attempted. All tag comparisons and inserts use parameterized queries (`$1` / `?` placeholders) — the tag value is never interpolated into SQL.

### Schema upgrade

**Automatic, on first run.** When the new `kit.Migrate` connects to a database that has an existing `_grizzle_migrations` table missing the `tag` or `is_baseline` columns, it detects which columns are absent and adds only those:

```sql
-- PostgreSQL
ALTER TABLE _grizzle_migrations ADD COLUMN IF NOT EXISTS tag TEXT NOT NULL DEFAULT '';
ALTER TABLE _grizzle_migrations ADD COLUMN IF NOT EXISTS is_baseline BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE _grizzle_migrations ALTER COLUMN checksum SET DEFAULT '';
ALTER TABLE _grizzle_migrations ALTER COLUMN sql_batch SET DEFAULT '';
```

```sql
-- MySQL (check column existence before running; ADD COLUMN IF NOT EXISTS requires 8.0.29+)
ALTER TABLE _grizzle_migrations ADD COLUMN tag VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE _grizzle_migrations ADD COLUMN is_baseline TINYINT(1) NOT NULL DEFAULT 0;
ALTER TABLE _grizzle_migrations MODIFY COLUMN checksum VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE _grizzle_migrations MODIFY COLUMN sql_batch LONGTEXT NOT NULL DEFAULT '';
```

```sql
-- SQLite (check column existence before running; IF NOT EXISTS on ADD COLUMN requires 3.37.0+)
ALTER TABLE _grizzle_migrations ADD COLUMN tag TEXT NOT NULL DEFAULT '';
ALTER TABLE _grizzle_migrations ADD COLUMN is_baseline INTEGER NOT NULL DEFAULT 0;
```

The upgrade is idempotent: each column addition is guarded by an existence check. No downtime is required. On PostgreSQL, `ADD COLUMN ... DEFAULT ''` is a metadata-only operation and does not rewrite the table.

**Privilege requirement:** The schema upgrade requires `ALTER TABLE` privilege on `_grizzle_migrations`. In environments where the application runtime credential does not have `ALTER TABLE`, run `grizzle migrate` once under an elevated credential (or apply the DDL above manually). Use `--skip-schema-upgrade` to suppress the automatic upgrade and manage it out-of-band. If `--skip-schema-upgrade` is passed and the `tag` or `is_baseline` columns are absent, `kit.Migrate` returns an error immediately — it does not fall back to the old schema or silently degrade. The operator must apply the upgrade DDL manually before proceeding.

**SQLite concurrency:** The column-existence check and `ADD COLUMN` are not atomic on SQLite. If two processes race at startup (e.g., serverless cold starts), the second `ADD COLUMN` will fail. The schema upgrade must run on a single, serialized connection; the calling application should ensure exclusive startup locking.

### What happens to old rows

Old checksum-based rows **cannot be reliably mapped to migration filenames** — the SQL batch was generated from a live diff and may not correspond to any file on disk. These rows are **preserved as-is** with `tag = ''` and `is_baseline = FALSE`. They remain in the history table for audit purposes and do not interfere with the file-based workflow: the new `kit.Migrate` only checks rows where `tag != ''` when determining which files are already applied.

### The baseline problem

After upgrading, an existing deployment typically has:

- A populated `_grizzle_migrations` table (old rows, `tag = ''`)
- A live database schema that already matches those old migrations

`grizzle generate` reads Go schema definitions and diffs them against `meta/snapshot.json`. With no existing snapshot, it treats the baseline as empty and produces a migration file containing `CREATE TABLE` DDL for every defined table (e.g. `0001_initial_schema.sql`). Running `grizzle migrate` would then attempt to apply that file — re-creating tables that already exist — and fail.

**Solution: `--baseline` flag**

```sh
grizzle migrate --baseline 0001_initial_schema
```

`--baseline <tag>` inserts a history record for the named migration (and all preceding migrations, by sequence number) **without executing their SQL**. The value passed to `--baseline` becomes the `tag` column value in the inserted rows. This marks those files as already applied, allowing the file-based workflow to continue from that point forward.

Library API:

```go
result, err := kit.Migrate(ctx, pool, kit.MigrateOptions{
    MigrationsDir: "./migrations",
    Baseline:      "0001_initial_schema",
})
```

When `Baseline` is set:

- All migration files whose sequence number is ≤ the baseline tag's sequence number are inserted into `_grizzle_migrations` with `is_baseline = TRUE`, `checksum` = SHA-256 of the file's byte contents, and `sql_batch = ''`. The sequence number is the leading decimal integer before the first underscore in the filename (e.g. `0001` from `0001_initial_schema.sql`). If two files in the directory share the same numeric prefix, `kit.Migrate` returns an error — duplicate prefixes are invalid and are rejected at `grizzle generate` time as well. All baseline inserts for a single `--baseline` run are committed in a single transaction; a crash mid-run leaves the table unchanged, and re-running will resume from the first tag that is still absent.
- Subsequent migrations (higher sequence numbers) are applied normally.
- If a row already exists for a given tag, that tag is skipped (idempotent).
- If a row already exists for a given tag with `is_baseline = FALSE` (previously applied normally), `--baseline` leaves that row untouched.
- If `--baseline` names a tag that does not correspond to any file in the migrations directory, `kit.Migrate` returns an error and applies nothing.

`grizzle migrate --baseline` emits a list to stderr of every migration being baselined (filename and SHA-256 of the file), so operators have an out-of-band record of the operation.

`--baseline` must **not** be used on a fresh database. On a fresh install with an empty `_grizzle_migrations`, run `grizzle migrate` without `--baseline`; all migration files will be applied and recorded normally.

### Recommended upgrade path

For a deployment currently using the old `kit.Migrate`:

1. **Upgrade Grizzle.**
2. **Run `grizzle generate`** — reads the Go schema definitions and, with no existing snapshot, produces a migration file containing `CREATE TABLE` DDL for all defined tables (e.g. `migrations/0001_initial_schema.sql`). The filename sequence starts at `0001_` when the migrations directory is empty. This command does not connect to a database.
3. **Run `grizzle migrate --baseline 0001_initial_schema`** — automatically upgrades the history table schema (adds `tag` and `is_baseline` columns), then marks `0001_initial_schema` as baselined without executing it.
4. **Resume normal workflow** — all future `grizzle generate` + `grizzle migrate` calls follow the file-based path.

**Multi-node deployments:** Ensure all nodes are updated to the new Grizzle version before running step 3. The schema upgrade (`ADD COLUMN tag`, `ADD COLUMN is_baseline`) is safe to run while old-version nodes are still connected — existing rows are unaffected. However, running `--baseline` while old-version nodes are actively writing to `_grizzle_migrations` opens a race window where the same migration could be applied twice. Coordinate the upgrade using a deployment strategy that prevents concurrent old/new writes (e.g., stop old nodes before running step 3).

### Decision summary

| Question | Decision |
|---|---|
| Is the schema upgrade automatic or manual? | **Automatic** — on first run of the new `kit.Migrate` |
| Are old checksum-based records preserved? | **Yes** — preserved with `tag = ''`, never deleted or rewritten |
| Are old records migrated to tags? | **No** — a reliable mapping does not exist |
| Is a CLI flag needed? | **Yes** — `--baseline <tag>` for existing deployments switching to file-based workflow |
| Should `--baseline` be used on fresh installs? | **No** — on a fresh database, use `grizzle migrate` without `--baseline` |
| Are baseline rows distinguishable from normal applies? | **Yes** — `is_baseline = TRUE` / `FALSE`; baseline rows store the file checksum but have `sql_batch = ''` |

---

## Known bugs

- ~~**#114** — FK `ON DELETE`/`ON UPDATE` actions silently dropped for SQLite and MySQL schemas.~~ **Fixed** in PR #180: `gen/parser/eval.go` now evaluates FK actions for all three dialect packages (pg, mysql, sqlite).
- **#110** — PostgreSQL-only locking clauses (`FOR NO KEY UPDATE`, `FOR KEY SHARE`) emitted for MySQL, producing invalid SQL. Dialect interface needs feature-detection methods to prevent this.
