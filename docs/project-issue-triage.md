# Project-Wide GitHub Issue Triage

This document broadens issue review beyond the file-migration issue names.

Issue inventory was pulled from GitHub on 2026-05-06. The repository had 75 open issues.

Source of truth for issue classification:

1. `docs/spec/*`
2. tagged Drizzle ORM / Drizzle Kit `v1.0.0-rc.1` source
3. current code only as implementation evidence

The goal is not to force every issue into file migrations. The goal is to prevent any issue from accidentally implementing behavior that conflicts with the specs.

## Project-Wide Buckets

| Bucket | Issues | Triage |
| --- | --- | --- |
| Migration kit / file migrations / pull | #153, #154, #157, #158, #169, #273, #274, #275, #276, #277, #278, #279, #280 | Normalize before Slice 0; see [file-migrations-issue-triage.md](./file-migrations-issue-triage.md). |
| Schema DSL / type system / codegen | #172, #183, #216, #222, #234, #235, #236, #248, #253, #254, #259 | Map to `schema.md`, `types.md`, `codegen.md`, and file-migration unsupported-field rules where relevant. |
| Kit diff / SQL generation / introspection | #79, #82, #137, #225, #226, #240, #243, #244, #249, #250 | Some feed pull/file-migration slices; sequence support is blocked/deferred by initial unsupported-object-family rules. |
| Query builder / expressions / relations | #33, #81, #113, #128, #134, #139, #140, #141, #144, #162, #163, #164, #167, #171, #197, #203, #232, #233, #237, #263, #264, #271 | Work from `query-builder.md`, `relations.md`, `dialects.md`, and `types.md`; not a file-migration Slice 0 gate. |
| Drivers / transactions / prepared statements | #42, #88, #143, #159, #160, #166, #170, #223, #252, #267, #268 | Work from `transactions.md` and `query-builder.md`; file-migration sessions should reference but not wait on user-facing driver features unless required. |
| Docs / repo hygiene / release | #74, #135, #175, #221, #224, #228, #258, #262 | Normal backlog; not a file-migration gate except where specs must be corrected before code. |

## Immediate Normalization Priorities

### P0: Prevent old migration direction from leaking into implementation

Close, supersede, or rewrite issues that encode discarded behavior:

- #154 — old flat-file/live-diff/checksum/baseline migrate plan
- #153 — old `meta/snapshot.json` plus flat numbered SQL generate plan
- #273, #275, #280 — semicolon SQL splitter issues; target uses statement breakpoints
- #274, #276, #277, #278 — useful concerns but current bodies are tied to old PR #272 behavior and need rewriting or deferral

### P1: Create project-wide parent issues

The backlog should not rely only on file-migration slices. Create parent issues/milestones for the major spec areas:

1. Schema DSL parity and strict schema input
2. Query builder parity and dialect gating
3. Codegen parity and type mapping
4. Driver/transaction/prepared statement parity
5. Migration kit/file-migration workflow slices 0-8
6. Pull/introspection/source-generation workflow
7. Docs/spec synchronization and release policy

### P2: Apply labels/project fields consistently

Recommended fields:

- `area:schema`
- `area:query`
- `area:codegen`
- `area:driver`
- `area:dialect`
- `area:kit`
- `area:file-migrations`
- `area:pull`
- `phase:spec`
- `phase:implementation`
- `blocked-by-spec`
- `superseded`

For file-migration-specific work, keep `slice:0` through `slice:8`.

## Area Notes

### Schema / Types / Codegen

Issues #234, #235, #236, and #259 are near-term correctness/parity items. They should be evaluated against `schema.md`, `types.md`, and `codegen.md`, not delayed just because they are outside the file-migration slice names.

Issues #172, #253, and #254 should be treated carefully: generated columns are recognized in RC.1 snapshots but intentionally unsupported in the initial file-migration target. The first file-migration implementation should add negative validation for generated columns, not partial support through the old `kit.Snapshot` model.

Sequence issues #137, #248, #249, and #250 are similar. They remain future schema/kit backlog unless the specs are amended to include sequences in the initial artifact graph.

### Query / Relations

Query issues should continue independently against `query-builder.md` and `relations.md`. They are part of the holistic project plan and should not be hidden behind file-migration sequencing.

Important correctness issues include dialect gating (#264, #271), missing operators (#164), conflict/update APIs (#33, #162), and codegen alias/self-join support (#113).

### Drivers / Transactions / Prepared Statements

Driver and transaction issues are cross-cutting. They should not block file-migration Slice 0, but Slice 5 migration sessions should consult these issues to avoid duplicating incompatible abstractions.

Prepared statement work (#166, #252, #88, #223) belongs to query/driver parity, not migration-kit implementation.

### Migration Kit / Pull / File Migrations

Use [file-migrations-issue-triage.md](./file-migrations-issue-triage.md) as the detailed appendix for issues whose current bodies mention migration filenames, `kit.Migrate`, `generate`, `check`, `push`, or `pull`.

The key correction is that `push` does not block `pull`, and `generate`/`check`/`migrate` must follow the ratified artifact workflow rather than the older flat-file transition issues.

## Exit Criteria Before Slice 0

A holistic backlog is ready for implementation only when:

- every open issue is assigned to a project area or explicitly left unassigned as backlog
- old migration-direction issues are closed, superseded, or rewritten
- every implementation slice has a parent issue
- non-file-migration project areas have parent issues/milestones too
- specs are updated before implementation for any `blocked-by-spec` work
- no issue body remains the de facto source of truth when it conflicts with `docs/spec/*` or Drizzle RC.1 source
