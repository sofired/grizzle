# File Migrations GitHub Issue Triage

> Project-wide note: this is a focused migration-kit appendix. Use [project-issue-triage.md](./project-issue-triage.md) as the holistic issue review entry point.

This document records migration-kit GitHub issue triage before starting the RC.1-style file-migration implementation sequence.

Issue inventory was pulled from GitHub on 2026-05-06. At that time the repository had 75 open issues.

This document uses the same lens as [file-migrations-code-triage.md](./file-migrations-code-triage.md) and the issue rules in [file-migrations-implementation-sequence.md](./spec/file-migrations-implementation-sequence.md): existing issues are not implicitly accepted just because they already exist; they must be mapped, rewritten, deferred, or marked superseded against the ratified specs.

## Classification Terms

- `Map`: useful issue, broadly aligned with the ratified specs after label/milestone assignment.
- `Rework`: useful intent, but acceptance criteria, blockers, public shape, or sequencing conflict with the ratified specs.
- `Supersede`: issue tracks an old checksum/live-diff/flat-file/meta-direction and should be closed or replaced by a slice-specific issue.
- `Defer`: valid backlog, but outside the initial file-migration implementation sequence.
- `Blocked by spec`: no implementation should start until a dedicated spec or amendment exists.
- `Outside file migrations`: leave in the normal backlog; do not use it to gate Slice 0.

## High-Level Findings

- The open issue backlog still contains pre-spec file-migration issues whose acceptance criteria conflict with the ratified RC.1-style folder-per-migration design.
- The most important issue to neutralize is [#154](https://github.com/sofired/grizzle/issues/154), because it describes the discarded `kit.Migrate` direction: flat `.sql` files, `_grizzle_migrations`, `tag`, `is_baseline`, schema upgrade flags, and checksum/live-diff lineage.
- [#153](https://github.com/sofired/grizzle/issues/153) tracks the right command name (`generate`) but the wrong artifact model (`meta/snapshot.json` plus numbered flat SQL files). It should not become the Slice 4 implementation issue without being rewritten.
- Splitter issues from PR #272 ([#273](https://github.com/sofired/grizzle/issues/273), [#275](https://github.com/sofired/grizzle/issues/275), [#280](https://github.com/sofired/grizzle/issues/280)) are old-direction issues. The target execution model uses explicit `--> statement-breakpoint` segmentation, not semicolon splitting.
- `push`, sequence support, generated column support, non-interactive rename answers, and applied hash-drift reporting are valid backlog areas, but they are not Slice 0 blockers.

## Direct File-Migration Issues

| Issue | Current title | Triage | Recommended action | Target slice / label guidance |
| --- | --- | --- | --- | --- |
| [#153](https://github.com/sofired/grizzle/issues/153) | `feat: grizzle generate command — write SQL migration files to disk` | Rework / likely supersede | Keep the command intent, but replace the body. Current acceptance criteria use `migrations/meta/snapshot.json`, numbered flat `.sql` files, and an old `kit.Generate` shape. The target is folder-per-migration artifacts with `migration.sql` and `snapshot.json`, pre-write `check`, RC.1 snapshot envelope, `prevIds`, and breakpoints. | `file-migrations`, `phase:implementation`, `slice:4`; blocked by Slices 0-3 |
| [#154](https://github.com/sofired/grizzle/issues/154) | `feat: refactor kit.Migrate to file-based workflow (DEVIATION:BROKEN)` | Supersede | Close or mark superseded. It encodes the discarded transition plan: `_grizzle_migrations`, `tag`, `is_baseline`, automatic schema upgrade, `--baseline`, `--skip-schema-upgrade`, and flat `.sql` application. Create a new Slice 6 parent for artifact-based `migrate` instead. | `superseded`; replacement should be `file-migrations`, `phase:implementation`, `slice:6`, blocked by Slices 0-5 |
| [#169](https://github.com/sofired/grizzle/issues/169) | `feat: grizzle check command — validate migrations directory consistency` | Rework | Rewrite as the Slice 3 `check` issue. Current body says `check` validates current schema and optionally DB state; target `check` is offline artifact/snapshot/graph validation and must ignore DB credentials. | `file-migrations`, `phase:implementation`, `slice:3`; blocked by Slices 0-2 |
| [#158](https://github.com/sofired/grizzle/issues/158) | `feat: grizzle pull command (introspect live DB → Go schema definitions)` | Rework | Rewrite as Slice 7. Current issue uses direct `--db <dsn>` shape, omits managed bootstrap artifacts, broad-scan opt-in, redaction, source/artifact stores, and `pull --init`. Its stated dependency on `push` is obsolete. | `file-migrations`, `phase:implementation`, `slice:7`; blocked by Slices 0-6 |
| [#157](https://github.com/sofired/grizzle/issues/157) | `feat: grizzle push CLI command` | Defer / blocked by spec | Keep as backlog only. `push` is a public command boundary, but implementation-ready behavior needs a dedicated direct-sync spec. It must not block `pull`, `check`, or `generate`. | `blocked-by-spec`; optionally `file-migrations` only as command-boundary backlog, not `phase:implementation` |
| [#277](https://github.com/sofired/grizzle/issues/277) | `Support non-transactional DDL in PostgreSQL file-based migrations` | Rework | Map the need into Slice 6 execution semantics. Do not adopt the issue's proposed `-- grizzle:no-transaction` header unless the execution spec is amended. The initial design should follow dialect transaction/partial-application rules in `file-migrations-execution.md`. | `file-migrations`, `phase:implementation`, `slice:6`; maybe `spec-required` if new per-artifact transaction controls are desired |
| [#276](https://github.com/sofired/grizzle/issues/276) | `kit.Migrate: add MySQL and PostgreSQL integration tests for file-based workflow` | Rework | Useful integration-test intent, but current body references PR #272 concepts: schema upgrade, baseline, skip-schema-upgrade. Rewrite after target history/session/execution semantics exist. | `file-migrations`, `phase:implementation`, `slice:5`, `slice:6`, `testing` |
| [#274](https://github.com/sofired/grizzle/issues/274) | `PostgreSQL integration tests for file-based kit.Migrate` | Rework | Same as #276, narrower PG version. Merge/supersede into one target Slice 6 integration-test issue. Remove old baseline/schema-upgrade acceptance criteria. | `file-migrations`, `phase:implementation`, `slice:6`, `testing`; likely duplicate/superseded by #276 replacement |
| [#278](https://github.com/sofired/grizzle/issues/278) | `Detect checksum drift on already-applied migrations` | Defer / rework | The history spec intentionally follows name-based pending detection and says `migrate` must not add a default applied-hash-drift blocker. Keep only as future audit/status tooling, not as initial `migrate` behavior. | `file-migrations`, `blocked-by-spec` or future audit backlog; not Slice 6 default behavior |
| [#279](https://github.com/sofired/grizzle/issues/279) | `Track upstream Drizzle non-interactive generate rename/conflict resolution` | Defer | Already matches spec posture. Keep as future enhancement. Initial `generate` uses RC.1-style interactive prompts and omits non-interactive answer files. | `blocked-by-spec`, `enhancement-candidate`; not initial slices |
| [#273](https://github.com/sofired/grizzle/issues/273) | `Support dollar-quoted PL/pgSQL blocks in migration file SQL splitter` | Supersede | Old semicolon-splitter issue. Target execution splits by full-line statement breakpoints, not SQL tokenization. Close/supersede unless a future disabled-breakpoints parser is explicitly designed. | `superseded`; future work would be `blocked-by-spec` |
| [#275](https://github.com/sofired/grizzle/issues/275) | `kit.Migrate: handle semicolons in string literals and PL/pgSQL blocks` | Supersede | Same as #273. This should not drive Slice 6. | `superseded`; future parser work blocked by spec |
| [#280](https://github.com/sofired/grizzle/issues/280) | `return error from splitSQLStatements for unterminated constructs` | Supersede | Same splitter family as #273/#275. The target should not have this helper on the command path. | `superseded`; future parser work blocked by spec |

## Migration-Adjacent Issues To Adapt Or Defer

These are not all initial file-migration blockers, but they touch schema loading, diffing, introspection, or dialect execution and should be mapped before coding starts.

| Issue | Area | Triage | Recommended action | Slice guidance |
| --- | --- | --- | --- | --- |
| [#244](https://github.com/sofired/grizzle/issues/244) | `kit/diff` default comparison | Map / adapt | Keep as a useful regression for the old diff engine and for future schema/snapshot comparison logic. | Slice 2 / Slice 3 |
| [#243](https://github.com/sofired/grizzle/issues/243) | default-expression normalization | Defer / adapt | Low-priority unless the strict schema loader or RC.1 fixtures can emit dollar-quoted defaults. If kept, acceptance should reference validated snapshot/default rendering. | Slice 2 / Slice 3 or deferred |
| [#240](https://github.com/sofired/grizzle/issues/240) | view dependency ordering | Rework / possibly spec-required | Useful if RC.1-compatible dependency information is available. Do not implement by ad hoc SQL parsing unless specified. | Slice 3 / Slice 4; add `spec-required` if design is unclear |
| [#259](https://github.com/sofired/grizzle/issues/259) | unsafe/raw numeric defaults | Map | Relevant to strict schema input and literal validation. Acceptance should cite the file-migration literal/default rules rather than only current builder behavior. | Slice 2 |
| [#226](https://github.com/sofired/grizzle/issues/226) | introspection FK action fallback | Rework | Target behavior should fail or diagnose unsupported introspection values; a warning-only fallback is too weak for strict file migrations. | Slice 7, maybe Slice 2 validation fixtures |
| [#225](https://github.com/sofired/grizzle/issues/225) | PostgreSQL FK introspection integration test | Map | Useful for `pull`/introspection validation. Should run under integration-test infrastructure. | Slice 7, testing |
| [#79](https://github.com/sofired/grizzle/issues/79) | schema-qualified FK introspection | Map | Useful correctness issue for `pull` and live introspection adapters. | Slice 7 |
| [#82](https://github.com/sofired/grizzle/issues/82) | FK ordinal dead/unused fields | Map / defer | Pair with #225. Not a Slice 0 blocker. | Slice 7 or normal tech-debt backlog |
| [#183](https://github.com/sofired/grizzle/issues/183) | dialect methods on MySQL/SQLite table defs | Outside / optional support | Low-risk schema test. Useful for dialect-agnostic schema input confidence, but not a file-migration gate. | Optional Slice 2 support test |
| [#160](https://github.com/sofired/grizzle/issues/160) | MySQL/SQLite transaction wrappers | Outside / maybe inform | User-facing transaction wrappers are separate. Slice 5 migration sessions may use `database/sql` directly and should not wait on this. | Not a gate; maybe Slice 5 reference |
| [#159](https://github.com/sofired/grizzle/issues/159) | transaction isolation/access modes | Outside / maybe inform | Not required for initial file-migration sessions unless the execution spec requires configurable isolation. | Not a gate |

## Unsupported Initial Object Families / Features

The ratified file-migration specs intentionally treat these as unsupported in the initial target. Existing issues can remain as backlog, but they should not be implemented into `kit.Snapshot`, `Diff`, SQL generation, or `pull` before the initial RC.1-supported field matrix is in place.

| Issue | Feature | Triage | Recommended action |
| --- | --- | --- | --- |
| [#172](https://github.com/sofired/grizzle/issues/172) | generated columns umbrella | Defer / blocked by spec | Initial file migrations must validate recognized generated-column fields and fail `unsupported_feature`. Implementation support needs a later spec decision. |
| [#253](https://github.com/sofired/grizzle/issues/253) | generated-column DSL/diff/sqlgen | Defer / blocked by spec | Do not wire into old `kit/diff` as initial file-migration work. Replace with negative validation tests first. |
| [#254](https://github.com/sofired/grizzle/issues/254) | generated-column introspection/codegen | Defer / blocked by spec | Initial `pull` should detect and reject/report unsupported generated columns according to spec. |
| [#137](https://github.com/sofired/grizzle/issues/137) | PostgreSQL sequence support umbrella | Defer / blocked by spec | PostgreSQL standalone sequences are unsupported object families in the initial artifact graph. |
| [#248](https://github.com/sofired/grizzle/issues/248) | sequence schema DSL | Defer | Schema DSL-only work may be future backlog, but it must not imply initial artifact support. |
| [#249](https://github.com/sofired/grizzle/issues/249) | sequence support in `Snapshot`, `Diff`, SQL generation | Supersede / blocked by spec | Current acceptance criteria are explicitly tied to old `kit.Snapshot`. Rewrite later against RC.1 DDL entities if sequence support is approved. |
| [#250](https://github.com/sofired/grizzle/issues/250) | sequence introspection | Defer / blocked by spec | Initial `pull` should reject unsupported object families rather than silently dropping or serializing sequences. |

## Issues That Should Not Gate File-Migration Slice 0

These open issues are valuable normal backlog work, but they are outside the initial file-migration sequencing gate unless the project owner explicitly adds them to a slice.

| Area | Issues |
| --- | --- |
| Query builder / expression behavior | [#271](https://github.com/sofired/grizzle/issues/271), [#264](https://github.com/sofired/grizzle/issues/264), [#263](https://github.com/sofired/grizzle/issues/263), [#237](https://github.com/sofired/grizzle/issues/237), [#233](https://github.com/sofired/grizzle/issues/233), [#232](https://github.com/sofired/grizzle/issues/232), [#203](https://github.com/sofired/grizzle/issues/203), [#197](https://github.com/sofired/grizzle/issues/197), [#171](https://github.com/sofired/grizzle/issues/171), [#167](https://github.com/sofired/grizzle/issues/167), [#164](https://github.com/sofired/grizzle/issues/164), [#163](https://github.com/sofired/grizzle/issues/163), [#162](https://github.com/sofired/grizzle/issues/162), [#144](https://github.com/sofired/grizzle/issues/144), [#141](https://github.com/sofired/grizzle/issues/141), [#140](https://github.com/sofired/grizzle/issues/140), [#139](https://github.com/sofired/grizzle/issues/139), [#134](https://github.com/sofired/grizzle/issues/134), [#128](https://github.com/sofired/grizzle/issues/128), [#113](https://github.com/sofired/grizzle/issues/113), [#81](https://github.com/sofired/grizzle/issues/81), [#33](https://github.com/sofired/grizzle/issues/33) |
| Driver/prepared statement backlog | [#268](https://github.com/sofired/grizzle/issues/268), [#267](https://github.com/sofired/grizzle/issues/267), [#252](https://github.com/sofired/grizzle/issues/252), [#223](https://github.com/sofired/grizzle/issues/223), [#166](https://github.com/sofired/grizzle/issues/166), [#170](https://github.com/sofired/grizzle/issues/170), [#143](https://github.com/sofired/grizzle/issues/143), [#88](https://github.com/sofired/grizzle/issues/88), [#42](https://github.com/sofired/grizzle/issues/42) |
| Codegen/schema parity not specific to file migrations | [#236](https://github.com/sofired/grizzle/issues/236), [#235](https://github.com/sofired/grizzle/issues/235), [#234](https://github.com/sofired/grizzle/issues/234), [#222](https://github.com/sofired/grizzle/issues/222), [#216](https://github.com/sofired/grizzle/issues/216) |
| Docs/release/repo hygiene | [#262](https://github.com/sofired/grizzle/issues/262), [#258](https://github.com/sofired/grizzle/issues/258), [#228](https://github.com/sofired/grizzle/issues/228), [#224](https://github.com/sofired/grizzle/issues/224), [#221](https://github.com/sofired/grizzle/issues/221), [#175](https://github.com/sofired/grizzle/issues/175), [#135](https://github.com/sofired/grizzle/issues/135), [#74](https://github.com/sofired/grizzle/issues/74) |

## Missing Slice Parent Issues

The existing backlog does not cleanly provide one parent issue per implementation slice. Before coding Slice 0, create or rewrite parent issues as follows:

| Slice | Existing issue candidate | Required action |
| --- | --- | --- |
| Slice 0: package boundary/test harness | none | Create new parent issue. |
| Slice 1: artifact discovery/offline validation core | none | Create new parent issue. |
| Slice 2: snapshot/schema input planning | partial: #259, #172/#253/#254 negative cases | Create parent issue; link migration-adjacent issues as children or blockers. |
| Slice 3: `check` | #169 | Rewrite #169 or create replacement. |
| Slice 4: `generate` | #153 | Rewrite #153 or close and create replacement. |
| Slice 5: history/locking/sessions | none | Create new parent issue. Old #154 history criteria must not be reused. |
| Slice 6: `migrate` | #154, #277, #276/#274 | Close/supersede #154; create replacement and link reworked execution/test issues. |
| Slice 7: `pull` / `pull --init` | #158 plus #79/#82/#225/#226 | Rewrite #158 and link introspection issues. |
| Slice 8: CLI cutover/cleanup | none | Create new parent issue for public command rewiring and legacy command cleanup. |

## Label Recommendations

Apply labels or equivalent project fields after issue rewriting:

- `file-migrations`
- `phase:implementation`
- `slice:0` through `slice:8`
- `spec-required` for view dependency ordering or transaction-control extensions if the current specs are insufficient
- `blocked-by-spec` for `push`, generated-column implementation, sequence implementation, non-interactive rename answers, disabled-breakpoint SQL parsing, and hash-drift enforcement
- `superseded` for old flat-file/meta/checksum/live-diff issues

## Immediate Cleanup Recommendations

1. Close or mark superseded:
   - [#154](https://github.com/sofired/grizzle/issues/154)
   - [#273](https://github.com/sofired/grizzle/issues/273)
   - [#275](https://github.com/sofired/grizzle/issues/275)
   - [#280](https://github.com/sofired/grizzle/issues/280)

2. Rewrite or replace before use:
   - [#153](https://github.com/sofired/grizzle/issues/153)
   - [#169](https://github.com/sofired/grizzle/issues/169)
   - [#158](https://github.com/sofired/grizzle/issues/158)
   - [#276](https://github.com/sofired/grizzle/issues/276)
   - [#274](https://github.com/sofired/grizzle/issues/274)
   - [#277](https://github.com/sofired/grizzle/issues/277)

3. Defer as backlog / blocked by spec:
   - [#157](https://github.com/sofired/grizzle/issues/157)
   - [#172](https://github.com/sofired/grizzle/issues/172), [#253](https://github.com/sofired/grizzle/issues/253), [#254](https://github.com/sofired/grizzle/issues/254)
   - [#137](https://github.com/sofired/grizzle/issues/137), [#248](https://github.com/sofired/grizzle/issues/248), [#249](https://github.com/sofired/grizzle/issues/249), [#250](https://github.com/sofired/grizzle/issues/250)
   - [#278](https://github.com/sofired/grizzle/issues/278)
   - [#279](https://github.com/sofired/grizzle/issues/279)

## Exit Criteria Impact

This issue triage is not sufficient by itself to start Slice 0. The remaining pre-implementation gate is to normalize the backlog in GitHub:

- old-direction issues are closed, marked superseded, or rewritten
- one parent issue exists for every slice 0 through 8
- child issues are mapped to the relevant slice parent
- direct-sync `push` and unsupported object-family work are visibly deferred or blocked by spec
- no open issue remains that can accidentally pull old flat-file, `meta/`, checksum, baseline, or live-diff behavior into the RC.1 file-migration workflow
