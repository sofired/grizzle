# File Migrations Implementation Sequence

This document is the Phase 4 implementation sequencing plan for the Drizzle RC.1-style file-migration workflow.

The implementation target is the spec package rooted at [kit.md](./kit.md), with behavior pinned to Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`.

## Status

- Phase 2 target specs are complete.
- Phase 3 broad documentation review found no remaining Medium or High findings.
- Remaining implementation work should proceed in the sequence below unless the project owner explicitly changes the order.

## Rationale

The RC.1-style workflow must be built as a complete system, not by replacing `migrate` in isolation.

For a Drizzle-style migration kit, `migrate` depends on the surrounding contract:

- schema input
- migration generation
- migration metadata, snapshots, and artifact history
- offline consistency checking
- database history-table semantics
- statement segmentation and dialect execution behavior
- CLI and library API behavior

Prior work attempted to repurpose `migrate` before the rest of that workflow existed. This sequence prevents that mistake by building artifact validation, snapshot planning, `check`, `generate`, history, locking, and execution before the final command cutover.

## Guardrails

- Do not let the existing `migrate` implementation define the target design; replace or reuse it only after the new artifact, `check`, history, and execution path work end-to-end.
- Keep direct-sync `push` outside this implementation sequence except for preserving the public command boundary.
- Treat compatibility with unreleased checksum-based Grizzle formats as out of scope.
- Keep `snapshot` and `diff` out of the target public RC.1 workflow.
- Use Drizzle RC.1 source behavior as the default answer when implementation questions arise; any divergence must be documented before code changes.
- Existing code, open PRs, and GitHub issues are triaged against this sequence before implementation starts. They are not implicitly accepted just because they already exist.
- Exploratory prototypes are allowed only to answer implementation questions; they must not be promoted into production code until reconciled against the ratified specs.
- Intermediate slices must remain easy to back out. If a slice stalls, either revert it or leave it behind non-command-path APIs until the missing spec or implementation dependency is resolved.

## GitHub And Existing-Code Triage

Before Slice 0 implementation starts, the repository and GitHub backlog must be normalized against the ratified spec.

### Existing Code

Classify current implementation by package/path:

- `Keep`: behavior already matches the ratified spec and can be covered by tests as-is.
- `Adapt`: useful code exists, but public shape, error contracts, safety checks, or data model must change before it is reused.
- `Quarantine`: legacy behavior remains temporarily reachable only to avoid breaking current users while new internals are built; it must not be used as the foundation for RC.1 file migrations.
- `Delete`: speculative or incompatible code that would fight the target design.

Rules:

- code classified as `Adapt` must be wrapped by new tests before being changed
- code classified as `Quarantine` must have an explicit cutover/removal issue
- code classified as `Delete` should be removed only in PRs that clearly prove it has no remaining dependency
- no existing code may override the spec; if code and spec disagree, the code is implementation debt unless the spec is explicitly amended

### Pull Requests

Every open PR touching migration, schema loading, codegen, query metadata, dialect execution, or driver/session behavior must be triaged before merge.

Classification:

- `Merge after rebase`: aligns with the spec and belongs in the current or next slice.
- `Split`: contains useful parts, but mixes multiple slices or unrelated cleanup.
- `Rework`: useful intent, but implementation conflicts with the spec.
- `Close / supersede`: implements the old checksum/live-diff approach, old artifact layout, unsafe migration behavior, or an unapproved divergence.

Rules:

- PRs must state which slice they implement
- PRs must link to the relevant spec section
- PRs must not mix workflow slices unless the dependency is unavoidable and documented
- PRs that change public command meaning must wait until Slice 8 unless explicitly scoped as internal-only preparation

### GitHub Issues

Existing issues should be mapped into implementation slices and milestones.

Required labels or equivalent project fields:

- `file-migrations`
- `phase:implementation`
- `slice:0` through `slice:8`
- `spec-required` for issues that need a spec amendment before implementation
- `blocked-by-spec` for issues intentionally deferred until a dedicated spec exists
- `superseded` for issues made obsolete by the RC.1 file-migration decision

Issue handling rules:

- close or mark superseded issues that only track the discarded checksum-based migration direction
- keep issues for genuine implementation gaps, but rewrite acceptance criteria to cite the ratified specs
- create missing issues for each implementation slice before coding that slice
- direct-sync `push`, `export`, legacy upgrade support, and non-interactive rename resolution remain backlog items unless separately approved

### Triage Deliverable

Create or update a GitHub project/milestone view with:

- one parent issue per implementation slice
- child issues for concrete implementation tasks
- linked PRs only after classification
- explicit blockers for cross-slice dependencies

Exit criteria:

- every open migration-related PR is classified
- every migration-related issue is mapped, superseded, or deferred
- Slice 0 has a concrete issue list ready for implementation
- no open PR remains that can accidentally merge old-direction behavior into the new workflow

## Slice 0: Package Boundary And Test Harness

Objective:
Create a safe place to build the new workflow without letting existing migration behavior shape the target design.

Build:

- internal/package boundaries for file-migration APIs defined in [file-migrations-api.md](./file-migrations-api.md)
- stable error codes and redacted diagnostic carriers used by all later slices
- resource-limit structs and test fixtures
- test filesystem/source/artifact stores with no-follow and containment behavior

Exit criteria:

- existing command behavior is not treated as a compatibility constraint unless the project owner explicitly says so
- new internal APIs compile but do not take over the CLI command path yet
- tests can exercise stores, diagnostics, and limits without a database

## Slice 1: Artifact Discovery And Offline Validation Core

Objective:
Implement the local artifact model before generating or applying anything.

Build:

- migration root discovery
- RC.1-style migration directory validation
- `migration.sql` / `snapshot.json` loading
- strict rejection of legacy `meta/`, unknown files, bad names, missing files, malformed snapshots, and unsafe paths
- raw-byte SQL digest calculation and combined artifact digest
- initial `CheckResult` data model

Exit criteria:

- artifact loading/checking can run on an empty directory and on fixture directories
- malformed artifact cases produce stable redacted errors
- no database connection is required

## Slice 2: Snapshot And Schema Input Planning

Objective:
Produce and compare Grizzle snapshots without writing migration artifacts.

Build:

- static schema loader needed by `generate`
- snapshot serializer/parser for supported PostgreSQL/MySQL/SQLite fields
- unsupported-field detection from [file-migrations-snapshot-fields.md](./file-migrations-snapshot-fields.md)
- snapshot graph comparison inputs used by `check` and `generate`
- generated table/view/source metadata needed by codegen and query builders where file migrations depend on it

Exit criteria:

- schema input can produce a deterministic target snapshot
- unsupported RC.1 fields fail before artifact writes
- snapshot fixtures round-trip or fail exactly as specified

## Slice 3: `check`

Objective:
Make `check` the trusted offline gate for later mutating commands.

Build:

- CLI/API entrypoints for `check`
- artifact ordering, branch, collision, snapshot graph, and digest validation
- omission of `--ignore-conflicts`
- structured result output for library callers

Exit criteria:

- `check` passes empty roots
- `check` catches malformed graphs and unsafe artifacts
- `check` is callable by later `generate` and `migrate` code without CLI coupling

## Slice 4: `generate`

Objective:
Create migration artifacts from schema definitions on top of `check`.

Build:

- generation planner from prior effective snapshot to target snapshot
- migration SQL rendering and breakpoint insertion
- artifact directory creation through `ArtifactStore`
- rename resolver interface and RC.1-style interactive CLI flow
- custom migration artifact creation, if included in command/API shape
- pre-write `check` integration

Exit criteria:

- empty-state generation creates a valid first migration
- subsequent generation extends the graph deterministically
- generated artifacts pass `check`
- failed generation leaves no partial artifacts

## Slice 5: History, Locking, And Migration Sessions

Objective:
Prepare the database-side foundation before executing migrations.

Build:

- migration session interfaces
- dialect capability validation
- history table creation/validation for the supported RC.1-style schema
- `UNIQUE(name)` / `NOT NULL` hardening
- DB-backed lock acquisition/release
- pinned session behavior and transaction capability checks

Exit criteria:

- history tables are created/validated per dialect
- unsupported or legacy history schemas fail as specified
- concurrent migration attempts are serialized or fail safely

## Slice 6: `migrate`

Objective:
Apply reviewed artifacts safely.

Build:

- pending detection by migration name
- internal `check` before execution
- statement-breakpoint execution
- transaction/partial-application behavior by dialect capability
- history insertion after successful migration execution
- failure, rollback, lock release, and row ownership behavior
- managed introspection artifact rejection until `pull --init`

Exit criteria:

- generated artifacts can be applied end-to-end
- applied migrations are skipped by name on later runs
- failed runs surface deterministic state and diagnostics
- only after this slice may the CLI `migrate` command be wired to the new workflow

## Slice 7: `pull` And `pull --init`

Objective:
Add database-to-schema bootstrapping after the core artifact/history path exists.

Build:

- introspection adapters and source rendering
- managed source writes
- broad-scan opt-in and redacted summaries
- secret-literal detection for generated source/artifacts
- bootstrap introspection artifact planning
- `pull --init` lock/history lifecycle and quiescent validation

Exit criteria:

- plain `pull` writes managed source/bootstrap artifacts without touching history
- `pull --init` records reviewed bootstrap state without executing its SQL
- broad-scan diagnostics and object refs follow the redaction contract

## Slice 8: CLI Cutover And Cleanup

Objective:
Expose the completed RC.1-style workflow and remove ambiguity.

Build:

- final command wiring for `generate`, `check`, `migrate`, `pull`, and `introspect`
- legacy behavior deprecation/removal as decided by the project owner
- docs/examples alignment with the new public behavior
- release-note migration guidance

Exit criteria:

- target workflow is the documented default
- old direct/live-diff paths are not reachable under the same public meaning
- compatibility concerns are tracked separately rather than mixed into initial implementation

## Deferred Work

- direct-sync `push` behavior, beyond preserving the command boundary
- `export`
- non-interactive/config-based rename resolution
- legacy Grizzle checksum/table upgrade support
- Drizzle dialects outside the initial PostgreSQL/MySQL/SQLite scope
- public `snapshot` / `diff` workflow

## Implementation Test Matrix

Each slice should add focused tests for the behavior it owns. Before the CLI cutover, the implementation should have coverage for:

- generation from an empty state
- generation from prior snapshot and artifact state
- duplicate migration identity and duplicate migration name detection
- missing `migration.sql` / `snapshot.json`
- malformed or unsupported snapshot fields
- edited pending artifact diagnostics
- applied migration hash drift skipped by name
- pending migration detection
- failed migration execution and partial-application reporting
- dialect-specific transaction and statement-execution behavior
- branch and collision detection in `check`
- history-table creation, validation, and constraints
- concurrent migration attempts and lock behavior
- path traversal, symlinked roots, symlinked artifact files, hardlink aliasing where detectable, and no-follow TOCTOU behavior
- reserved staging temporary directories and interrupted-write cleanup
- credential, bind-value, and raw-SQL redaction in errors, logs, verbose output, and panics
- static schema-loader non-execution of target-project package initialization code
- unsupported schema-loader constructs
- unsupported RC.1 entity fields and properties
- `pull --init` bootstrap preconditions, quiescent-target expectations, and reuse after failed metadata insertion
- destructive `push` boundary tests once a direct-sync spec exists
