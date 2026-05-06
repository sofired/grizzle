# File Migrations Check Specification

## Status

Draft

## Purpose

Define Grizzle's equivalent of Drizzle's migration consistency checking.

## Scope

This document must define:

- collision detection
- ordering validation
- snapshot-graph consistency checks
- missing file detection
- duplicate identity handling
- edited file drift policy
- CI expectations

## Upstream References

- Drizzle Kit check docs
- Drizzle RC.1 source:
  - `drizzle-kit/src/cli/commands/check.ts`
  - `drizzle-kit/src/cli/schema.ts`
  - `drizzle-kit/src/cli/commands/generate-common.ts`
  - `drizzle-kit/src/cli/commands/generate-postgres.ts`
  - `drizzle-kit/src/cli/commands/generate-mysql.ts`
  - `drizzle-kit/src/cli/commands/generate-sqlite.ts`
- [file-migrations-upstream-mapping.md](./file-migrations-upstream-mapping.md)

## Responsibilities

Upstream Drizzle role of `check`:

- walk generated migrations
- detect race conditions / collisions
- validate migration history consistency for team workflows

Grizzle must treat this as a required workflow component, not a later optional enhancement.

Initial Grizzle translation:

- `check` validates the canonical RC.1-style migration artifact set
- `check` is part of the normal file-based workflow
- `check` applies equally to generated migrations and custom migrations
- `check` must gate both `generate` and `migrate`
- Grizzle will not expose a conflict-bypass flag such as Drizzle's `--ignore-conflicts`; this is DEVIATION:INTENTIONAL safety hardening relative to Drizzle RC.1
- `check` is an offline artifact/snapshot/graph validation step in the initial design

Strictness note:

- RC.1 exposes `--ignore-conflicts`; Grizzle omits it as DEVIATION:INTENTIONAL safety hardening
- RC.1 `checkHandler()` is centered on snapshot validation and commutativity analysis
- Grizzle additionally validates the full artifact shape and computes local digests as DEVIATION:INTENTIONAL artifact hardening

## Branch Model

In this specification, "branch" refers to the migration snapshot graph, not to Git itself.

Git branching is a common cause of migration branching, but the thing `check` validates is the migration graph formed by `snapshot.json` ancestry:

- each migration snapshot normally points to one parent snapshot
- if two migrations are generated independently from the same parent, the graph forks
- the current ends of that graph are its "leaf" migrations

Example:

```text
A
├── B
└── C
```

In this graph:

- `A` is the shared ancestor snapshot
- `B` and `C` are sibling migration branches
- `B` and `C` are both leaves

This matters because the next `generate` step needs a coherent prior context even when history is no longer linear.

## Key Terms

- `parent snapshot`: the snapshot a migration was generated from
- `leaf`: a migration snapshot that has no child snapshot yet
- `open branch`: a currently reachable branch ending in one of the current leaf snapshots
- `commutative branches`: sibling migration branches that can coexist without semantic conflict according to the checker
- `selected branch context`: the branch-analysis result that supplies the effective parent context for further generation

## Upstream RC.1 Behavior

Drizzle RC.1 `check` provides the roadmap for branch and commutativity analysis, but its CLI behavior is not a byte-for-byte contract for Grizzle. The reviewed RC.1 source:

- discovers snapshots from the migration artifact set
- validates snapshot structure and version enough to decide whether analysis can proceed
- reconstructs the migration ancestry graph
- identifies current leaf nodes
- analyzes non-commutativity across open branches
- fails on non-commutative conflicts
- may return a selected open commutative branch context for subsequent generation

Grizzle intentionally hardens the CLI/library contract:

- unsupported or non-latest snapshot versions are command failures, not successful no-op exits
- malformed graph state, missing non-root parents, and cycles are command failures
- the library always returns a structured `CheckResult` on success so `generate` and `migrate` can consume the validated graph without reinterpreting filesystem state
- these stricter rules are DEVIATION:INTENTIONAL validation hardening relative to RC.1's more permissive edge handling

RC.1 CLI parameter behavior:

- `prepareCheckParams()` resolves only `out` and `dialect`
- database credentials may appear in a shared config file, but they are not passed to `checkHandler()`
- Grizzle `check` therefore remains an offline artifact/snapshot/graph command for the initial design

When multiple open commutative branch candidates exist, RC.1 selects the candidate "closest to the leaves":

- candidates must cover exactly the current open leaf set
- if more than one candidate qualifies, the chosen candidate is the one with the fewest combined leaf statements

This is effectively a "most recent common valid ancestor" rule for continued generation.

## Validation Rules

At minimum, `check` must validate:

- artifact layout validity
- snapshot validity/version
- migration ordering consistency
- collision / non-commutativity rules
- custom migration inclusion within the same artifact/history graph
- current leaf discovery
- branch ancestry consistency
- object-family support for both snapshot validation and commutativity analysis

Normative validation categories:

- artifact validation
- snapshot validation
- graph validation
- ordering validation
- drift validation

`check` must fail if any required validation category fails.

Object-family rule:

- an object family may be accepted by `check` only when Grizzle can both validate its RC.1 snapshot shape and reason about its effect during branch commutativity analysis
- if a snapshot contains a recognized RC.1 object family whose commutativity behavior is not implemented, `check` must fail with `unsupported_object_family`
- if a supported object family contains a recognized field/property whose validation, diffing, or commutativity behavior is not implemented, `check` must fail with `unsupported_feature`
- schema recognition without commutativity support is not enough to treat an artifact graph as valid
- this keeps `check` from accepting branches that it cannot safely prove are coherent

## Artifact Validation

`check` must validate the artifact contract defined in [file-migrations-artifacts.md](./file-migrations-artifacts.md).

At minimum:

- if the configured output directory does not exist yet, `check` must treat it as an empty migration set
- each migration candidate must be an immediate child directory
- each migration directory must contain `migration.sql`
- each migration directory must contain `snapshot.json`
- each migration directory name must satisfy the supported identity format
- unsupported legacy markers such as `meta/_journal.json` must cause failure

`check` must not silently skip malformed migration directories.

Absent-root note:

- Drizzle RC.1 may create a missing `out` directory while preparing command parameters
- Grizzle `check` remains read-only for an absent migrations root and models it as an empty graph
- this is DEVIATION:INTENTIONAL read-only check behavior; `generate` and `pull` own root creation through write-mode artifact APIs

## Snapshot Validation

`check` must validate every discovered `snapshot.json`.

At minimum:

- snapshot loading and parsing must enforce `MaxSnapshotJSONBytes`, `MaxSnapshotJSONDepth`, `MaxSnapshotEntities`, total artifact bytes, artifact count, and context cancellation before unbounded allocation or recursion
- the file must be valid JSON
- it must match the supported snapshot schema family for the active dialect
- it must match the latest supported snapshot version for the current Grizzle release line
- its `ddl` payload must validate against the RC.1-derived dialect schema defined in [file-migrations-artifacts.md](./file-migrations-artifacts.md)
- its `renames` payload must be a string array for RC.1 shape parity; Grizzle additionally validates generated rename strings against the documented `from->to` encoder as DEVIATION:INTENTIONAL validation hardening

Failure conditions:

- malformed JSON
- unsupported version
- obsolete version requiring upgrade
- schema mismatch for the selected dialect
- invalid dialect-specific DDL or rename payload

## Graph Validation

`check` must reconstruct the migration graph from snapshot ancestry metadata.

At minimum:

- every non-root snapshot parent reference must resolve to an existing snapshot
- root snapshots must satisfy the Grizzle root rules
- cycles are invalid
- orphaned snapshots are invalid unless explicitly allowed by the snapshot schema
- leaf discovery must be deterministic

Root rules:

- generated first-migration snapshots use the sentinel origin UUID as their only parent:
  - `00000000-0000-0000-0000-000000000000`
- every stored artifact snapshot, including pull bootstrap snapshots, must have a fresh non-sentinel `id`; the sentinel is allowed only in `prevIds`
- `check` must treat that sentinel parent as a valid root marker rather than a missing-parent error
- non-root parent references other than the sentinel origin UUID must resolve to an existing local snapshot
- Drizzle RC.1 varies by dialect for `pull` bootstrap `prevIds`; Grizzle normalizes supported root snapshots to the sentinel-parent rule as DEVIATION:INTENTIONAL graph-normalization

Failure conditions:

- missing parent snapshot
- cyclic ancestry
- ambiguous or inconsistent parent metadata
- impossible leaf set reconstruction

Empty-project rule:

- if there are zero snapshots, `check` must succeed
- zero snapshots represent a valid empty migration graph for first-run generation
- `migrate` applies a stricter deployment rule and must fail on absent or empty artifact roots unless explicitly allowed; see [file-migrations-execution.md](./file-migrations-execution.md)

## Ordering Validation

`check` must validate that artifact ordering metadata and graph ancestry do not contradict each other.

At minimum:

- migration directory names must be sortable under the supported ordering rules
- duplicate migration `name` values are invalid
- the `created_at` value derived from artifact identity must be internally consistent
- snapshot ancestry must not imply a contradictory ordering impossible under the discovered artifact set

`check` must treat ordering contradictions as hard errors.

## Collision Detection

Grizzle must copy the RC.1 collision-detection role closely:

- detect when migration history is branched
- determine whether open branches are commutative
- fail when open branches are non-commutative

For initial scope, Grizzle must not provide a user-facing bypass mode for these failures.

If `check` cannot prove that the open migration graph is coherent, `generate` and `migrate` must not proceed.

Normative conflict behavior:

- if open branches are non-commutative, `check` must fail
- if branch analysis cannot determine whether open branches are commutative, `check` must fail
- if there is a single linear leaf, `check` may succeed without branch-selection output
- initial SQLite support is linear-graph validation only; if SQLite has multiple open leaves requiring commutativity analysis, `check` must fail with `unsupported_feature` until a SQLite commutativity engine is explicitly designed and tested

## Selected Branch Context Output

Successful `check` calls must produce a structured result consumable by `generate`, even when the CLI only renders human-readable output.

This complete result shape is an intentional Go API adaptation. RC.1 only exposes selected branch context in some commutative-branch paths and uses empty results in common linear paths; Grizzle normalizes all successful cases into one explicit carrier so downstream code does not need dialect-specific or path-specific inference.

Conceptual output fields:

- `baseSnapshot`
- `baseID`
- `effectiveSnapshot`
- `effectiveParentIDs`
- `branchStatements`
- `artifactDigests`
- `loadedArtifacts`

Normative behavior:

- `baseSnapshot` is the root/common-ancestor snapshot used by branch analysis
- `baseID` is the ID corresponding to `baseSnapshot`, or the sentinel origin UUID for an empty graph
- `effectiveSnapshot` is the materialized prior schema state for the next generation step
- `effectiveParentIDs` is the deterministic `prevIds` list the next generated snapshot must write; multiple selected leaf IDs are sorted lexicographically by snapshot `id`, matching Drizzle RC.1 `leafIds` ordering
- `branchStatements` are ordered dialect JSON diff/commutativity statements used to materialize `effectiveSnapshot` from `baseSnapshot`; they are not executable SQL
- `artifactDigests` must contain local SHA-256 digest metadata for each discovered artifact:
  - `migration.sql` digest computed over exact raw file bytes
  - `snapshot.json` digest computed over exact raw file bytes
  - combined artifact digest over both required files
- non-commutative failure diagnostics must be available through the returned typed error; they may also be copied into a partial diagnostic payload, but failed checks do not return a successful `CheckResult`
- `loadedArtifacts` or an equivalent internal carrier must allow `migrate` to execute the checked artifact set without trusting a later best-effort rescan
- `loadedArtifacts` must carry exact validated bytes or stable no-follow file handles; path names plus digest strings are not enough for execution
- if multiple branch candidates qualify, the chosen candidate must be the one closest to the leaves under the RC.1-style rule

Required result cases:

- empty graph:
  - `baseSnapshot` and `effectiveSnapshot` are the dialect root/dry snapshot
  - `baseID` is the sentinel origin UUID
  - `effectiveParentIDs` is `[00000000-0000-0000-0000-000000000000]`
  - `branchStatements` is empty
  - the first generated snapshot still uses the sentinel origin UUID as its sole `prevIds` entry
- linear graph:
  - `baseSnapshot` and `effectiveSnapshot` are the single current leaf snapshot
  - `baseID` is that leaf snapshot's `id`
  - `effectiveParentIDs` contains that leaf ID
- commutative branched graph:
  - `baseSnapshot` and `baseID` identify the selected common ancestor
  - `branchStatements` contain the selected branch context's typed diff statements
  - `effectiveSnapshot` is the materialized result of applying `branchStatements` to `baseSnapshot`
  - `effectiveParentIDs` contains the selected open leaf IDs
- non-commutative or indeterminate graph:
  - `check` fails and no successful `CheckResult` is returned

Failure diagnostics:

- failed checks return a non-nil typed error
- non-commutative and indeterminate graph failures must include a human-readable conflict report on the typed error
- implementations may attach a partial diagnostic payload to the typed error, but callers must not treat that payload as a successful `CheckResult`

This output may be internal API data, CLI structured output, or both. The exact Go carrier belongs in the API spec, but the information contract belongs here.

## Branch Selection for Generation

When migration history is branched but commutative, `check` must return a selected branch context for `generate`.

That selected context must include the same conceptual information Drizzle RC.1 returns:

- base/common-ancestor snapshot
- base/common-ancestor snapshot identity
- current leaf identities
- ordered typed branch statements associated with the selected open commutative branch
- materialized effective snapshot for Go callers

The purpose of this result is not to "pick a Git branch." It is to define the effective migration-history basis for the next generated migration.

Example:

```text
A
├── B
└── C
```

If `B` and `C` are commutative, the checker may determine that the next generation step can proceed using the branch context rooted at `A` and covering both open leaves.

In Grizzle's Go API, `A` is `baseSnapshot`, the typed branch statements describe the commutative changes from `A` through `B` and `C`, and `effectiveSnapshot` is the materialized schema state that `generate` diffs against.

The next generated migration is then produced with awareness of both open leaves, rather than by pretending the history is linear.

Normative handoff rule:

- `generate` must use `CheckResult.EffectiveSnapshot` as its previous schema state and `CheckResult.EffectiveParentIDs` as the next snapshot's `prevIds`
- `generate` must not independently pick the lexicographically last migration directory
- `generate` must not recompute a different branch context after `check` succeeds

## Drift Detection

`check` must validate local artifact drift and local identity inconsistencies.

Normative drift rules:

- if the same migration `name` appears multiple times locally, `check` must fail
- if the same migration `name` is associated with inconsistent artifact metadata locally, `check` must fail
- every discovered `migration.sql` must be hashed using SHA-256 over the exact raw file bytes using a bounded reader that enforces the active resource limits
- every discovered `snapshot.json` must be hashed using SHA-256 over the exact raw file bytes using a bounded reader that enforces the active resource limits
- those hashes must be included in the structured result as local artifact digest metadata
- `check` must expose a combined artifact digest for diagnostics and to prevent time-of-check/time-of-use drift
- local hash computation must not normalize line endings, comments, breakpoint markers, or trailing newlines

Initial scope note:

- local identity drift must be treated as a hard error, not a warning
- Grizzle must not normalize duplicate or internally inconsistent local migration identities as acceptable workflow
- database-backed applied-history drift is not part of the initial `check` contract
- applied-history hash differences are not enforced by `migrate`; applied migrations are skipped by `name`, matching Drizzle RC.1
- applied-history hash tolerance is a parity decision with a security tradeoff: teams must treat applied migration artifacts as immutable through source control review

Current scope note:

- custom migrations do not create a separate validation path
- they remain subject to the same consistency model as generated migrations

## Exit Conditions

`check` succeeds only when all of the following are true:

- every artifact is structurally valid
- every snapshot is valid and current-version
- the migration graph can be reconstructed
- ordering is internally coherent
- no unsupported legacy format is present
- no non-commutative conflicts are present
- no hard drift condition is present

Otherwise, `check` fails.

## CI Guidance

CI must treat `check` as a required validation step for repositories that commit migration artifacts.

Minimum expectation:

- run `check` on every change that affects schema or migration artifacts
- fail CI on malformed snapshots, non-latest snapshots, unsupported layouts, or non-commutative migration conflicts

Recommended expectation:

- run `check` before merge for schema-affecting branches
- run `check` in release/deploy pipelines before `migrate`
- do not allow CI-only bypass flags that weaken migration-history validation

## Machine-Readable Output

Initial scope decision:

- the library `Check` API returns structured results
- the CLI may render human-readable diagnostics only
- no JSON output mode is required before the initial file-migrations implementation
