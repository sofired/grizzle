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
- Writes a new numbered `.sql` migration file (e.g. `0003_add_users_table.sql`)
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

**Transition plan:** The current `kit.Migrate` (introspect → diff → apply) must be refactored once `generate` is implemented. Existing deployments using the current live-diff approach have a `_grizzle_migrations` table populated with checksum-based records. A migration from the current format to the file-based format will be needed. The transition plan must be designed before `generate` is implemented.

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
  },
  "views": {
    "active_users": {
      "name": "active_users",
      "schema": "",
      "sql": "SELECT id, username FROM users WHERE enabled = true"
    }
  },
  "enums": {
    "status": {
      "name": "status",
      "schema": "",
      "values": ["pending", "active", "archived"]
    }
  }
}
```

`views` and `enums` are omitted from the JSON when empty (`omitempty`), so existing snapshots without these fields load cleanly. Use `kit.FromSchema(kit.SchemaObjects{...})` instead of `kit.FromDefs(...)` when your schema includes views or enums.

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
| New view | `CREATE OR REPLACE VIEW …` (PostgreSQL) / `DROP … + CREATE …` (MySQL, SQLite) | GRIZZLE-ONLY |
| Changed view (SQL modified) | `DROP VIEW IF EXISTS … + CREATE VIEW …` | GRIZZLE-ONLY |
| Dropped view | `DROP VIEW IF EXISTS …` | GRIZZLE-ONLY |
| New named enum type | `CREATE TYPE … AS ENUM (…)` (PostgreSQL only) | PARITY |
| Enum values added | `ALTER TYPE … ADD VALUE … AFTER/BEFORE …` (PostgreSQL only) | PARITY |
| Enum values removed or reordered | WARNING SQL comment — PostgreSQL cannot perform these operations | PARITY (DDL limitation) |
| Dropped named enum type | `DROP TYPE IF EXISTS …` (PostgreSQL only) | PARITY |

**View and enum dependency ordering:** `Diff` emits changes in a fixed 9-phase sequence: new enums → altered enums → renamed/new tables → altered tables → new views → replaced views → dropped views → dropped tables → dropped enums. This prevents referential integrity errors (e.g. a view cannot be created before its base tables exist; an enum cannot be dropped before the tables referencing it are dropped).

**`ALTER TYPE … ADD VALUE` and transactions (PostgreSQL < 12):** In PostgreSQL 9.x–11.x, `ADD VALUE` cannot run inside a transaction. If your migration runner wraps all changes in a single transaction, any migration that adds enum values will fail on those versions. Run the statement outside a transaction or upgrade to PostgreSQL 12+.

**View–view dependency ordering:** Within the new-view and drop-view phases, views are sorted alphabetically only — intra-view dependencies are not resolved. If a newly-created view selects from another new view, the names must sort in dependency order, or the migration will fail at runtime.

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

The `_grizzle_migrations` table (PostgreSQL) is created automatically on first use:

```sql
CREATE TABLE IF NOT EXISTS _grizzle_migrations (
    id          BIGSERIAL    PRIMARY KEY,
    applied_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    checksum    TEXT         NOT NULL,
    sql_batch   TEXT         NOT NULL,
    description TEXT         NOT NULL DEFAULT ''
);
```

This is the equivalent of Drizzle's `__drizzle_migrations` table. Note: the current schema records the full SQL batch and a checksum of it. The file-based target workflow will need to also record the migration filename/tag — the table schema will need a `tag` column when `generate` + file-based `migrate` is implemented.

MySQL and SQLite equivalents exist in `kit/migrate_mysql.go` and `kit/migrate_sqlite.go`.

---

## Known bugs

- ~~**#114** — FK `ON DELETE`/`ON UPDATE` actions silently dropped for SQLite and MySQL schemas.~~ **Fixed** in PR #180: `gen/parser/eval.go` now evaluates FK actions for all three dialect packages (pg, mysql, sqlite).
- **#110** — PostgreSQL-only locking clauses (`FOR NO KEY UPDATE`, `FOR KEY SHARE`) emitted for MySQL, producing invalid SQL. Dialect interface needs feature-detection methods to prevent this.
