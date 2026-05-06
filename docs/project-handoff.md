# Project Handoff

This is the project-wide handoff for continuing Grizzle implementation work.

It supersedes any narrow interpretation of [file-migrations-handoff.md](./file-migrations-handoff.md). The file-migration handoff remains useful history for the migration-kit docs work, but it is not the whole project plan.

## Source Of Truth

Use this hierarchy for all implementation decisions:

1. Grizzle specs in `docs/spec/`
2. tagged Drizzle ORM / Drizzle Kit `v1.0.0-rc.1` source for upstream behavior
3. Drizzle docs for the matching release line
4. current repository code as implementation evidence only
5. open GitHub issues as planning records only

If code or issues disagree with the specs, code/issues are stale unless the spec is amended.

## Current State

- The spec set now covers the planned project capabilities, including schema, query builder, relations, codegen, dialects, transactions, migration kit, file migrations, pull/introspection, CLI/API shape, artifact execution, and implementation sequencing.
- Existing code is a working but partial implementation across several areas.
- File migrations are the most detailed implementation sequence because they cross many boundaries and because older issue/PR work attempted an incompatible direction.
- The project still needs holistic backlog normalization, not only file-migration issue cleanup.

## New Review Artifacts

Project-wide entry points:

- [project-code-triage.md](./project-code-triage.md)
- [project-issue-triage.md](./project-issue-triage.md)
- [project-implementation-plan.md](./project-implementation-plan.md)

Focused migration-kit appendices:

- [file-migrations-code-triage.md](./file-migrations-code-triage.md)
- [file-migrations-issue-triage.md](./file-migrations-issue-triage.md)
- [file-migrations-handoff.md](./file-migrations-handoff.md)

## Important Correction

The phrase "file migrations" in the prior session's handoff and triage does not limit the project scope. The implementation plan must remain project-wide:

- query builder work continues from `query-builder.md`
- schema/type/codegen work continues from `schema.md`, `types.md`, and `codegen.md`
- transaction/driver work continues from `transactions.md`
- dialect work continues from `dialects.md`
- migration kit and pull work continue from `kit.md`, `pull.md`, and the `file-migrations-*` specs

## Next Steps

1. Review and merge the project-wide triage docs.
2. Normalize GitHub issues across all areas, not just migration issues.
3. Create parent issues/milestones for major spec areas.
4. Create one parent issue per file-migration implementation slice before starting Slice 0.
5. Close or rewrite old-direction migration issues before any implementation branch can accidentally reuse them as target behavior.
6. Start implementation from the relevant spec area, using Drizzle RC.1 source to resolve upstream details.

## Obsolete Notes From The Narrow Handoff

The old handoff's PR-opening instruction is obsolete because the changes are already present in this repository state.

The old handoff's remaining useful content is the warning that file-migration implementation should not begin until existing code and GitHub backlog are triaged against the ratified specs.
