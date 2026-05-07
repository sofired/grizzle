# Project Handoff

This is the project-wide handoff for continuing Grizzle implementation work.

It incorporates the earlier narrow file-migration handoff so this document is the single handoff entry point.

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

Project-wide entry point:

- [project-plan.md](./project-plan.md) — combines code triage, issue triage, and the forward-looking implementation plan into a single source. Triage findings are preamble; the plan is the forward-looking body.

Detailed migration-kit content is incorporated directly into `project-plan.md` rather than linked as separate handoff files.

## Important Correction

The phrase "file migrations" in the prior session's handoff and triage does not limit the project scope. The implementation plan must remain project-wide:

- query builder work continues from `query-builder.md`
- schema/type/codegen work continues from `schema.md`, `types.md`, and `codegen.md`
- transaction/driver work continues from `transactions.md`
- dialect work continues from `dialects.md`
- migration kit and pull work continue from `kit.md`, `pull.md`, and the `file-migrations-*` specs

## Next Steps

1. Review and merge `project-plan.md`.
2. Normalize GitHub issues across all areas, not just migration issues.
3. Create parent issues/milestones for major spec areas.
4. Create one parent issue per file-migration implementation slice before starting Slice 0.
5. Close or rewrite old-direction migration issues before any implementation branch can accidentally reuse them as target behavior.
6. Start implementation from the relevant spec area, using Drizzle RC.1 source to resolve upstream details.

## Historical Migration-Kit Handoff Context

The following content is preserved from the earlier focused file-migration handoff for historical context. Branch/PR instructions in this historical section may be obsolete; the source-of-truth and next-step rules above supersede them.

This note summarizes the spec-first file-migration documentation work and the intended next steps for continuing in a new session.

### Branch

Remote branch:

- `docs/file-migrations-spec`

Current documentation commit before this handoff note:

- `3c4927b docs(spec): define Drizzle RC.1 file migration workflow`

Pull request URL:

- https://github.com/sofired/grizzle/pull/new/docs/file-migrations-spec

### Summary

The work moved away from issue/PR `#154` implementation changes and created a documentation-only branch based directly on `origin/main`.

The branch defines a spec-first target for Grizzle's Drizzle RC.1-style file-migration workflow. The target behavior is pinned to Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`.

Major changes:

- Added detailed file-migration specs under `docs/spec/`.
- Defined artifact layout, `generate`, `check`, `migrate`, `pull`, history schema, snapshot fields, API shape, execution behavior, and implementation sequence.
- Converted `docs/spec/kit.md` into a thin index that points to dedicated specs.
- Added `docs/spec/file-migrations-implementation-sequence.md` with slice-based implementation order and GitHub/code triage rules.
- Simplified `README.md` into a conservative pre-release status page with badges and links to specs.
- Removed root-level planning/review scratch docs from the final docs commit.
- Rebased the docs work onto `origin/main` so the branch does not include the issue-154 implementation commits.

### Important State

- The remote branch exists and is cleanly based on `origin/main`.
- Local upstream tracking was not set in the linked worktree because the sandbox could not write the common `.git/config`, but the remote push succeeded.
- The attempted GitHub-tool close of PR `#272` was cancelled, so PR `#272` likely still needs to be closed manually or from the next session.

### Next Steps

1. Open a PR from `docs/file-migrations-spec` to `main`.
2. Close PR `#272` without merging if that remains the desired outcome.
3. Triage existing migration-related code, PRs, and issues against `docs/spec/file-migrations-implementation-sequence.md`.
4. Create GitHub issues or milestones for implementation slices `0` through `8`.
5. Start implementation with Slice 0 only after the triage gate is complete.

## Obsolete Notes From The Narrow Handoff

The old handoff's PR-opening instruction is obsolete because the changes are already present in this repository state.

The old handoff's remaining useful content is the warning that file-migration implementation should not begin until existing code and GitHub backlog are triaged against the ratified specs.
