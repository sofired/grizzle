# Drizzle ORM Parity Strategy

Grizzle is designed to mirror Drizzle ORM's behaviour and API design. This document describes how we prevent parity drift from occurring and growing undetected.

## What "Parity" Means

Parity has three dimensions:

| Dimension | Description | Examples |
|---|---|---|
| **SQL output** | Same SQL generated for equivalent inputs | Type mappings, migration DDL, locking clauses |
| **Behaviour** | Same runtime behaviour for equivalent operations | Error on arity mismatch, rename detection |
| **API shape** | Conceptually equivalent API surface | Typed `.set()`, `sql` template equivalent |


## What Caused Past Drift

| Gap | Root cause |
|---|---|
| SQLite type translation (#124) | PR updated docs to match broken code instead of fixing code |
| Rename annotation lifecycle (#125) | Design decision made without checking Drizzle's approach |
| FK parser pg-only (#114) | Feature implemented for one dialect, not generalised to others |
| MySQL locking clauses (#110) | PG-only syntax emitted for all non-SQLite dialects |

Only one of these would have been caught by automated side-by-side testing. The others were design and generalisation failures requiring process changes, not just more tests.

## Why Not Side-by-Side Runtime Tests

Running Drizzle (TypeScript/Node.js) alongside Grizzle in Go CI is a reasonable instinct but has significant maintenance cost:

- Schema definitions must be maintained in both TypeScript and Go
- Drizzle releases frequently (~weekly); keeping up adds continuous overhead
- Intentional divergences create false failures that must be suppressed
- SQL output may differ in whitespace, quoting, or ordering without being semantically wrong
- CI becomes sensitive to Drizzle npm package changes

Side-by-side testing is worth considering for the **type mapping layer** (SQLite/MySQL type translations), where the comparison is mechanical and Drizzle's output is stable. For everything else, the approaches below provide better coverage at lower cost.

## Features Beyond Drizzle's Scope

The following features were implemented in Grizzle PRs but have no equivalent in Drizzle ORM. They have been deferred until the baseline parity release is complete. Once parity is established, these can be evaluated as post-parity extensions on their own merits.

| Feature | Drizzle status | Enhancement issue |
|---|---|---|
| pgx-native batch API (`Batch`, `SendBatch`) | `db.batch()` is serverless-driver-only | #138 |
| Typed window frame spec (ROWS/RANGE/GROUPS BETWEEN) | `sql` template only | #139 |
| Typed FTS + regex operators (`ts_rank`, `~`, `~~`, etc.) | `sql` template only (tsvector column type is parity) | #140 |
| Typed row locking (FOR UPDATE/SHARE, SKIP LOCKED, NOWAIT, OF) | No locking support in Drizzle | #141 |
| Raw savepoint API (Savepoint/RollbackToSavepoint/ReleaseSavepoint) | Not exposed; Drizzle uses nested `tx.transaction()` | #142 |
| PostgreSQL range column types (int4range, daterange, etc.) | Not supported natively | #144 |

Note: Drizzle-style **nested transactions** (`tx.transaction()`) are a parity feature and are tracked separately in #143.

## Prevention Mechanisms

### 1. PR Template: Explicit Parity Checkpoint (highest leverage)

Every PR that touches a feature Drizzle also implements must include a parity declaration. Add to `.github/PULL_REQUEST_TEMPLATE.md`:

```markdown
## Drizzle parity
- [ ] This feature has a Drizzle equivalent → verified behaviour matches (link to Drizzle docs/source)
- [ ] This feature intentionally diverges from Drizzle → documented in issue #___
- [ ] This feature has no Drizzle equivalent (Grizzle extension)
```

This changes the incentive structure for contributors — drift is caught at authoring time, not in post-hoc review.

### 2. Parity Test Tag Convention

Tests that explicitly verify Drizzle-equivalent behaviour are tagged with a `drizzle:parity` comment linking to the relevant Drizzle documentation or source:

```go
// drizzle:parity https://orm.drizzle.team/docs/column-types/sqlite
func TestSQLiteTypeTranslation(t *testing.T) { ... }
```

This makes the parity surface grep-searchable and reveals gaps when Drizzle adds new behaviours. Run `grep -r 'drizzle:parity' .` to audit coverage.

### 3. SQL Golden File Tests

For the type mapping and DDL generation layers, run Drizzle once in a setup script, capture its SQL output as committed fixture files, and assert Grizzle's output matches. No Node.js in CI — the committed files are the contract.

```
testdata/
  drizzle_parity/
    sqlite_type_mapping.sql     # generated from Drizzle, committed
    mysql_type_mapping.sql
    pg_create_table.sql
    pg_migrations.sql
```

Golden files are updated manually and intentionally when Drizzle's behaviour changes, providing a clear audit trail of parity decisions.

### 4. Dialect Coverage Smoke Tests

A test matrix that runs each public query-builder feature against all three dialects and asserts the output does not contain dialect-specific syntax in the wrong dialect. This catches the class of bug where a PG-only feature leaks into MySQL or SQLite output.

```go
// For each locking mode, assert MySQL output contains no PG-only syntax
func TestLockingModes_NoLeakToMySQL(t *testing.T) { ... }
```

### 5. Drizzle Changelog Watch (lightweight)

A monthly GitHub Actions workflow that checks Drizzle's latest published version against a pinned baseline and opens a triage issue if the version has advanced. Keeps parity on the radar without blocking PRs.


## Known Active Parity Gaps

| Issue | Description | Priority |
|---|---|---|
| #114 | FK `ON DELETE`/`ON UPDATE` silently dropped for SQLite and MySQL schemas | High — silent data loss in DDL |
| #110 | `FOR NO KEY UPDATE` / `FOR KEY SHARE` emitted for MySQL (invalid SQL) | High — invalid SQL |
| #130 | MySQL codegen missing `mediumint`, `enum`, `set`, `year` | Medium |
| #129 | `RawArgs` arity mismatch emits literal `$?` instead of erroring | Medium |

## Resolved Gaps

| Issue | Description | Fixed in |
|---|---|---|
| #124 | SQLite type translation: canonical PG types passed through unchanged | PR #72 |
| #125 | Rename table: annotation-based approach vs Drizzle's interactive prompt | PR #70 |
