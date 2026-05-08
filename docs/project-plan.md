# Grizzle Project Plan

Single source of truth for the Grizzle implementation roadmap and backlog state. The specs in `docs/spec/` are normative for *behavior*; this document is normative for *sequencing* and *backlog hygiene*.

_Last refreshed: 2026-05-08._

## Contents

- [Implementation Frontier](#implementation-frontier)
- [Source Of Truth](#source-of-truth)
- [Roadmap](#roadmap)
  - [Workstreams](#workstreams)
  - [Recommended Order](#recommended-order)
  - [Slice Plan](#slice-plan)
  - [Parallel Work Guidance](#parallel-work-guidance)
- [Code Triage](#code-triage)
  - [Package-Level Classification](#package-level-classification)
  - [Quarantine And Delete/Replace](#quarantine-and-deletereplace)
- [Label Scheme](#label-scheme)
- [Backlog Hygiene Checklist](#backlog-hygiene-checklist)

## Implementation Frontier

| Item | Status |
| --- | --- |
| Last completed | **[Slice 0](https://github.com/sofired/grizzle/milestone/7)** (closed). `kit/filemigrate/` package + diagnostics + resource limits + source/artifact/managed test stores landed in PR #303. |
| Active | **[Slice 1](https://github.com/sofired/grizzle/milestone/8)** — artifact discovery and offline validation core. Slice-tagged hardening from Slice 0 dev (TOCTOU, naming validation, race hardening) folds into this milestone. |
| Next | **[Slice 2](https://github.com/sofired/grizzle/milestone/9)** — snapshot and schema input planning. |
| Parallel-safe workstreams | Schema (#282), Query (#283), Codegen (#284), Driver (#285), Dialect (#286), Pull (#288), Docs (#289). See [Parallel Work Guidance](#parallel-work-guidance). |
| Blocked-by-spec | Direct-sync `push` (#157, #296). Sequence support (#137, #248, #250). Generated columns (#172, #253, #254). Non-transactional DDL header (#277). Hash-drift detection (#278). Non-interactive rename answers (#279). View dependency ordering (#240). |

The full slice-by-slice picture is in [Slice Plan](#slice-plan).

## Source Of Truth

Use this hierarchy for all classification and implementation decisions:

1. Grizzle specs under `docs/spec/`
2. tagged Drizzle ORM / Drizzle Kit `v1.0.0-rc.1` (referred to as RC.1 throughout this document) source for upstream behavior
3. Drizzle docs for the matching release line
4. current repository code only as implementation evidence, never as the target contract
5. open GitHub issues as planning records only

If current code and specs disagree, the code is implementation debt unless the spec is amended. If the specs and tagged Drizzle RC.1 source disagree, the specs must explicitly document the Grizzle decision. If issues conflict with specs, the issue is stale and must be rewritten or superseded.

Background specs not directly cited below but worth reading when classifying migration-kit code or issues: [`file-migrations-workflow.md`](./spec/file-migrations-workflow.md) for the end-to-end workflow narrative, and [`file-migrations-upstream-mapping.md`](./spec/file-migrations-upstream-mapping.md) for RC.1 → Grizzle behavior mapping.

## Roadmap

### Workstreams

| Workstream | Parent issue | Source specs | Posture |
| --- | --- | --- | --- |
| Schema DSL and type system | [#282](https://github.com/sofired/grizzle/issues/282) | `schema.md`, `types.md`, `dialects.md` | Partial implementation with known parity gaps. |
| Query builder and relations | [#283](https://github.com/sofired/grizzle/issues/283) | `query-builder.md`, `relations.md`, `dialects.md` | Substantial implementation with remaining gaps and dialect-gating bugs. |
| Codegen | [#284](https://github.com/sofired/grizzle/issues/284) | `codegen.md`, `types.md` | Implemented but narrower than target; Slice 0 PR aligned `numeric` mapping. |
| Drivers, transactions, prepared statements | [#285](https://github.com/sofired/grizzle/issues/285) | `transactions.md`, `query-builder.md` | Partial; pgx stronger than database/sql. |
| Dialects | [#286](https://github.com/sofired/grizzle/issues/286) | `dialects.md` | Useful current matrix; some spec/interface drift. |
| Migration kit / file migrations | [#287](https://github.com/sofired/grizzle/issues/287) | `kit.md`, `file-migrations-*.md` | Slice 0 complete; Slice 1 next. Legacy `kit/migrate*.go`, `kit/apply.go`, `kit/snapshot.go` quarantined pending Slice 8 cleanup. |
| Pull / introspection | [#288](https://github.com/sofired/grizzle/issues/288) | `pull.md`, `file-migrations-snapshot-fields.md`, `schema.md`, `codegen.md` | Live introspection in `kit/introspect/`; `pull` workflow absent. |
| Docs/release/policy | [#289](https://github.com/sofired/grizzle/issues/289) | `README.md`, `docs/spec/README.md`, `overview.md` | Pre-release posture; refresh this doc whenever frontier changes. |

### Recommended Order

This order is about dependency safety, not exclusivity. Independent query/schema/driver fixes proceed when they are spec-aligned and do not conflict with the migration-kit sequence.

1. Build file-migration slices 0-8 in order (see [Slice Plan](#slice-plan)).
2. Continue non-blocking workstream parity work in parallel when scoped per [Parallel Work Guidance](#parallel-work-guidance).
3. Pause new file-migration implementation if [Backlog Hygiene Checklist](#backlog-hygiene-checklist) items regress.

### Slice Plan

Each slice has its own GitHub Milestone. Live child-issue tracking, open/closed counts, and progress live there. The slice parent issues that previously served this role were retired on 2026-05-08 with redirect comments.

The Grizzle repo uses **two milestone axes**:

- **Slice milestones** (this section) — implementation sequence for the file-migration workflow, one per slice 0-8. Granular, ordered, time-bounded.
- **Release/capability milestones** (`M1: Clean house`, `M4: Query builder parity`, `M5: Schema DSL + codegen completeness`, `M6: Advanced features`) — what ships when, across all workstreams. `M2: Dialect foundation` and `M3: Kit workflow` are now closed; M3's scope was absorbed into the slice milestones.

Both axes are GitHub Milestones, and a GitHub issue can only carry **one milestone**, so the axes are mutually exclusive per issue. Assignment rule: if the issue is part of a file-migration slice, it goes on the slice milestone (the slice progress rollup depends on this being complete); otherwise it goes on the relevant M-milestone. Cross-axis association (e.g., a Slice 3 issue that also serves M5 capability completeness) is captured in the issue body, not by a second milestone.

| Slice | Status | Milestone | Spec | Code involvement |
| --- | --- | --- | --- | --- |
| Slice 0: package boundary and test harness | Done (PR #303) | [Slice 0](https://github.com/sofired/grizzle/milestone/7) (closed) | `file-migrations-implementation-sequence.md` §Slice 0 | `kit/filemigrate/` (new). |
| Slice 1: artifact discovery and offline validation | Active | [Slice 1](https://github.com/sofired/grizzle/milestone/8) | `file-migrations-implementation-sequence.md` §Slice 1 | Build artifact loader/validator; do not reuse `kit.LoadJSON`. |
| Slice 2: snapshot and schema input planning | Upcoming | [Slice 2](https://github.com/sofired/grizzle/milestone/9) | `file-migrations-implementation-sequence.md` §Slice 2 | Adapt `schema/*`, `gen/parser/*`, `kit/snapshot.go` concepts into RC.1 snapshot planning. |
| Slice 3: `check` | Upcoming | [Slice 3](https://github.com/sofired/grizzle/milestone/10) | `file-migrations-check.md` | Adapt `kit/diff.go` graph/diff logic. |
| Slice 4: `generate` | Upcoming | [Slice 4](https://github.com/sofired/grizzle/milestone/11) | `file-migrations-generate.md`, `file-migrations-artifacts.md` | `kit/diff.go`, `kit/sqlgen*.go`, `schema/*`, `gen/parser`. |
| Slice 5: history, locking, sessions | Upcoming | [Slice 5](https://github.com/sofired/grizzle/milestone/12) | `file-migrations-history.md`, `file-migrations-execution.md` | New session/history/locking under `kit/filemigrate`; `driver/*`, `dialect/`. |
| Slice 6: `migrate` | Upcoming | [Slice 6](https://github.com/sofired/grizzle/milestone/13) | `file-migrations-execution.md` | Implement artifact execution; quarantine `kit/migrate*.go`. |
| Slice 7: `pull` and `pull --init` | Upcoming | [Slice 7](https://github.com/sofired/grizzle/milestone/14) | `pull.md` | Adapt `kit/introspect/`, `schema/*`, `gen/codegen`. |
| Slice 8: CLI cutover and cleanup | Upcoming | [Slice 8](https://github.com/sofired/grizzle/milestone/15) | `file-migrations-api.md`, `kit.md` | Rewire `cmd/grizzle/main.go`; remove legacy `kit/migrate*.go`, `kit/apply.go`, snapshot/diff CLI commands. |

Code-area follow-up tracking issues hang off the milestones above:

- [#296](https://github.com/sofired/grizzle/issues/296) — Quarantine direct-sync push helpers (blocked-by-spec)
- [#297](https://github.com/sofired/grizzle/issues/297) — Quarantine legacy live-diff migrate/status helpers (slice:8)
- [#298](https://github.com/sofired/grizzle/issues/298) — Replace legacy snapshot file model (slice:2)
- [#299](https://github.com/sofired/grizzle/issues/299) — Adapt schema definitions and static loader for strict file migrations (slice:2)
- [#300](https://github.com/sofired/grizzle/issues/300) — Adapt diff and SQL rendering to RC.1 DDL entities (slice:4)
- [#301](https://github.com/sofired/grizzle/issues/301) — Adapt introspection for `pull` and `pull --init` (slice:7)

### Parallel Work Guidance

Safe to work in parallel with file-migration slices when scoped and spec-backed:

- Query-builder bug fixes and missing operators (under #283).
- Codegen type-mapping fixes (under #284); the Slice 0 PR aligned `numeric` and noted the `int`/`int32` deviation.
- Schema-builder parity fixes (under #282).
- Dialect doc/interface synchronization (under #286).
- Driver tests and transaction-wrapper work (under #285).

Do **not** work in parallel if it changes:

- public migration command meanings,
- `kit.Snapshot`/artifact semantics,
- history-table semantics,
- schema input accepted by file migrations, or
- generated source format consumed by `pull` or file migrations,

unless explicitly coordinated with the relevant slice.

## Code Triage

The classifications below describe the disposition of *existing* code under spec precedence. They are not behavioral contracts — specs (per [Source Of Truth](#source-of-truth)) remain authoritative even when a path is classified Keep.

### Package-Level Classification

| Path | Project area | Classification | Why |
| --- | --- | --- | --- |
| `cmd/grizzle` | CLI | Adapt / Quarantine | Six current commands (`snapshot`, `diff`, `migrate`, `status`, `gen`, `sql`); `gen` and `sql` keep, the rest are quarantined for Slice 8 cutover. |
| `dialect/` | Dialect feature matrix | Adapt | Useful matrix; needs fail-fast capability gating and Slice 5 migration-session hooks. |
| `driver/pgx/` | PostgreSQL driver | Adapt | Useful driver; prepared-statement and transaction gaps remain. |
| `driver/sql/` | database/sql driver | Adapt | Useful generic driver for MySQL/SQLite. |
| `expr/` | SQL expression builders | Keep (file-migration scope) / Adapt (project-wide) | Useful for query expressions; do not treat as the file-migration DDL-expression model. |
| `gen/codegen/` | typed codegen | Adapt | Slice 0 PR fixed `numeric` mapping; remaining type-mapping decisions tracked under #284. |
| `gen/parser/` | static schema parser | Adapt / replace | Adapted into the file-migration schema loader during Slice 2 (#299). |
| `internal/testschema/` | fixtures | Keep / Adapt | Useful fixtures; reuse only where they match strict file-migration schema input. |
| `kit/` | migration kit (legacy + new) | Split: Adapt / Quarantine / Delete-replace | Split: `diff.go`, `sqlgen*.go`, `introspect/` adapt; `apply.go`, `migrate*.go`, `snapshot.go` quarantine; legacy `kit.Snapshot` shape and `_grizzle_migrations` history are Delete/Replace. |
| `kit/filemigrate/` | RC.1 file-migration core | **Keep** (target) | Slice 0 deliverable. Stores, diagnostics, resource limits, error codes. Slices 1-8 build inside this package. |
| `kit/introspect/` | live DB introspection | Adapt | Foundation for Slice 7 `pull` adapters. |
| `query/` | query builder | Keep (file-migration scope) / Adapt (project-wide) | Substantial; remaining gaps tracked under #283. |
| `schema/pg/`, `schema/mysql/`, `schema/sqlite/` | schema DSL | Adapt | Useful builders; Slice 2 hardens for strict RC.1 input. |

### Quarantine And Delete/Replace

These are forward-looking targets for Slice 6 and Slice 8 cleanup. The work is tracked on the slice parents and code follow-up issues; the table is reference, not action.

| Item | Current behavior | Target replacement | Slice |
| --- | --- | --- | --- |
| `kit/apply.go` (`Push`, `DryRun`) | Direct-sync push against a live DB. | No public target until a push spec exists ([#296](https://github.com/sofired/grizzle/issues/296)). | Deferred (blocked-by-spec) |
| `kit/migrate.go`, `kit/migrate_mysql.go`, `kit/migrate_sqlite.go` | Live-diff migrate/status; reads `_grizzle_migrations`. | Artifact-based `migrate` from `kit/filemigrate`; history table `__grizzle_migrations` (PG schema `grizzle`); pending detection by name; SHA-256 over raw bytes. | Slice 6 / Slice 8 ([#297](https://github.com/sofired/grizzle/issues/297)) |
| `cmd/grizzle` legacy commands | Public `snapshot`, `diff`, legacy `migrate`, `status`. | Target CLI: `generate`, `check`, `migrate`, `push`, `pull`, `introspect`. | Slice 8 |
| `kit/snapshot.go` (`kit.Snapshot`, `SaveJSON`, `LoadJSON`, `FromDefs`, `FromSchema`, `SchemaObjects`) | Legacy snapshot envelope (`tables`/`views`/`enums`). | RC.1 snapshot envelope under `kit/filemigrate` (`version`, `dialect`, `id`, `prevIds`, `ddl`, `renames`). Useful concepts may be adapted, but the legacy shape must not define the artifact format. | Slice 2 / Slice 8 ([#298](https://github.com/sofired/grizzle/issues/298)) |
| `_grizzle_migrations` history table; `checksum`, `sql_batch`, `description` columns; `ChecksumSQL` | Legacy live-diff history. | Target `__grizzle_migrations` with `id`, `hash`, `created_at`, `name`, `applied_at`. SHA-256 over raw `migration.sql` bytes. | Slice 5 / Slice 6 |

## Label Scheme

GitHub labels with project-defined semantics:

| Family | Members | Semantics |
| --- | --- | --- |
| `area:*` | `area:schema`, `area:query`, `area:codegen`, `area:driver`, `area:dialect`, `area:kit`, `area:file-migrations`, `area:pull`, `area:docs` | Primary code area the issue affects. One per issue, with the explicit exception that `area:kit` and `area:file-migrations` may co-occur per the `area:kit` vs `area:file-migrations` row below. `area:docs` covers documentation, repo hygiene, CI, and release policy. |
| `slice:*` | `slice:0` through `slice:8` | **Issue is delivered as part of Slice N** — authored, reviewed, merged within that slice's PR. **One slice tag per issue.** Cross-slice dependencies belong in the issue body, not as additional slice tags. Each `slice:N` label has a corresponding [GitHub Milestone](https://github.com/sofired/grizzle/milestones); the milestone is the live tracker (open/closed counts, progress), the label is the searchable filter. |
| `priority:*` | `priority:critical`, `priority:high`, `priority:medium`, `priority:low` | Scheduling priority. (Renamed from `P0`-`P3` on 2026-05-08.) |
| `phase:implementation` | — | Implementation work (code/tests). Spec-only work uses `blocked-by-spec` instead. |
| `blocked-by-spec` | — | No implementation until a spec is written or amended. |
| `superseded` | — | Old direction; replaced by another issue or closed. |
| `enhancement-candidate` | — | Out of scope for the current phase; reconsider in next planning cycle. |
| `area:kit` vs `area:file-migrations` | — | `area:kit` is shared migration-kit infrastructure (diff, SQL rendering, introspection adapters). `area:file-migrations` is the RC.1 folder-per-migration workflow specifically. An issue may carry both when the line is unclear. |
| GitHub defaults | `bug`, `enhancement`, `testing`, `documentation`, `good first issue`, `help wanted`, `in-progress`, `ready-for-merge`, `dependencies`, `github_actions`, `go`, `invalid`, `wontfix`, `duplicate`, `question`, `needs-triage` | No project-defined semantics; standard GitHub usage. The project-defined scheme is the rows above. |

**Combination rule:** `blocked-by-spec` and `slice:N` may coexist on a single issue; the combination means "tentatively planned for Slice N, but no implementation until the relevant spec is written or amended." Examples: #277 (Slice 6, awaiting non-transactional DDL spec amendment).

## Backlog Hygiene Checklist

Apply this checklist when promoting between slices or reviewing backlog drift. Each item is mechanically checkable.

Code-side gate (from [Code Triage](#code-triage)):

- [x] Every migration-related code path has an initial classification in [Package-Level Classification](#package-level-classification).
- [x] Old live-diff/checksum/snapshot behavior is identified as Quarantine or Delete/Replace.
- [x] `kit/filemigrate` exists with diagnostics, resource limits, and source/artifact/managed test stores (Slice 0 complete).
- [ ] The six remaining open code-area follow-up issues (#296-#301) close as their slice ships.

Issue-side gate:

- [x] Old-direction issues are closed or marked `superseded` (#154, #249, #273-#275, #280).
- [x] Defer targets carry `blocked-by-spec` (#157, #296, #137, #248, #250, #172, #253, #254, #277, #278, #279, #240).
- [x] One milestone exists for every Slice 0-8 (milestones 7-15).
- [x] One workstream parent exists for every project-wide area (#282-#289).
- [x] Every open issue carries an `area:*` label.
- [x] Every implementation issue with a target slice carries exactly one `slice:N` tag (the delivery slice).
- [x] Direct-sync `push` and unsupported object-family work remain `blocked-by-spec` until spec exists.

Spec-side gate:

- [x] Specs are updated before implementation for any `blocked-by-spec` work (none satisfied yet).
- [ ] If the [Workstreams](#workstreams) table cites a planned behavior that is not specified, the relevant spec is amended before coding starts.
