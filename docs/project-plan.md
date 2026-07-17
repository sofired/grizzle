# Grizzle Project Plan

This document defines how Grizzle's roadmap is organized and how work moves from an idea to implementation. Specifications under [`docs/spec/`](./spec/) remain authoritative for behavior.

_Last reviewed: 2026-07-17._

## Planning model

Grizzle uses **one scheduling axis: GitHub Milestones**.

- A milestone answers **when and in what sequence** an issue is intended to ship.
- Native parent/sub-issue relationships answer **which outcome or workstream owns** the issue.
- Native blocked-by/blocking relationships record **hard execution dependencies**.
- Labels answer **what kind of work it is and its current lifecycle state**.

Workstream epics are navigation indexes, not a second roadmap. The retired `slice:*` labels and release/capability milestone axis must not be recreated. An issue body should explain the outcome, scope, and acceptance criteria; it should not duplicate dependency lists or milestone rollups.

## Source of truth

Use this precedence for product and implementation decisions:

1. Grizzle specifications under `docs/spec/`.
2. Tagged Drizzle ORM / Drizzle Kit `v1.0.0-rc.1` source for upstream behavior.
3. Drizzle documentation for the matching release line.
4. Current repository code as implementation evidence, not the target contract.
5. Open GitHub issues as planning records.

If an issue conflicts with a specification, rewrite or supersede the issue before implementation. If code conflicts with a specification, treat the code as implementation debt unless the specification is deliberately amended.

## Milestones

Each open delivery issue has at most one milestone. Milestone numbers are ordered and stable; names describe outcomes rather than internal planning jargon.

| Order | Milestone | Outcome |
| --- | --- | --- |
| 00 | File-migration foundation (complete) | Package boundary, diagnostics, limits, and test stores. |
| 01 | Backlog and API governance | Stable public API policy, correctness cleanup, and repository governance. |
| 02 | Artifact discovery and validation | Safely discover, load, hash, and validate raw artifacts. |
| 03 | Snapshot and schema planning | Typed snapshot entities, static schema input, metadata, and comparison inputs. |
| 04 | Check | Reusable offline check API and command handler. |
| 05 | Generate | Plan DDL changes and publish complete artifacts atomically. |
| 06 | Migration sessions and history | Session, capability, history, credential, and lock foundations. |
| 07 | Migrate | Execute validated artifacts and record deterministic history. |
| 08 | Pull and pull --init | Introspect, bootstrap, render, and publish managed sources safely. |
| 09 | CLI cutover and release | Register the unified CLI, remove legacy paths, and publish cutover guidance. |

Milestone progress is the authoritative delivery rollup. Do not put future enhancement candidates into a milestone merely to categorize them; keep them in the icebox until a planning review promotes them.

## Workstream indexes

Every issue is attached to one native parent. Cross-cutting concerns use labels and native dependencies rather than multiple parent checklists.

| Workstream | Parent issue | Primary specifications |
| --- | --- | --- |
| Schema DSL and type system | [#282](https://github.com/sofired/grizzle/issues/282) | `schema.md`, `types.md`, `dialects.md` |
| Query builder and relations | [#283](https://github.com/sofired/grizzle/issues/283) | `query-builder.md`, `relations.md`, `dialects.md` |
| Code generation and metadata | [#284](https://github.com/sofired/grizzle/issues/284) | `codegen.md`, `types.md` |
| Drivers and transactions | [#285](https://github.com/sofired/grizzle/issues/285) | `transactions.md`, `query-builder.md` |
| Dialect capabilities | [#286](https://github.com/sofired/grizzle/issues/286) | `dialects.md` |
| File migrations | [#287](https://github.com/sofired/grizzle/issues/287) | `file-migrations-*.md`, `kit.md` |
| Pull and introspection | [#288](https://github.com/sofired/grizzle/issues/288) | `pull.md`, `schema.md`, `codegen.md` |
| Documentation and policy | [#289](https://github.com/sofired/grizzle/issues/289) | `README.md`, `docs/spec/README.md`, `overview.md` |

Smaller delivery epics may sit below a workstream parent when an outcome has multiple independently workable tasks. They do not replace milestones.

## Current implementation frontier

Milestone 02 is the active file-migration frontier.

Recommended queue:

1. Complete the raw-byte digest work in [#372](https://github.com/sofired/grizzle/issues/372).
2. Complete filesystem safety [#304](https://github.com/sofired/grizzle/issues/304), then resolver validation [#316](https://github.com/sofired/grizzle/issues/316), then artifact-directory validation [#310](https://github.com/sofired/grizzle/issues/310).
3. Ratify the durable result contract in [#321](https://github.com/sofired/grizzle/issues/321) and SQL byte validation in [#374](https://github.com/sofired/grizzle/issues/374).
4. Implement the loader in [#319](https://github.com/sofired/grizzle/issues/319).
5. Complete malformed-layout and snapshot-envelope rejection in [#320](https://github.com/sofired/grizzle/issues/320).
6. Keep publication-race hardening [#308](https://github.com/sofired/grizzle/issues/308) ready for the first writer that needs it.

GitHub's native dependency view is authoritative if this list and issue state ever diverge.

Milestone 03 begins with the shared metadata contract [#434](https://github.com/sofired/grizzle/issues/434). Typed DDL, snapshot parsing, static source loading, and downstream code-generation work remain blocked until their native prerequisites are complete.

## Product decisions

- Direct-sync `push` remains specification-first. Implementation issue [#157](https://github.com/sofired/grizzle/issues/157) must not proceed until [#402](https://github.com/sofired/grizzle/issues/402) is ratified.
- Public API naming and stability are governed by [#74](https://github.com/sofired/grizzle/issues/74).
- Custom migration SQL is opaque: it is not parsed or executed to infer a snapshot. Its DDL state remains the effective parent snapshot.
- The CLI cutover removes the legacy `sql` command. A future read-only schema-rendering command requires a separate proposal.
- Unsupported dialect behavior fails before execution; builders must not silently remove clauses or substitute false/NULL expressions.

## Issue standard

### Titles

Use a concise conventional form:

```text
type(area): outcome
```

Common types are `feat`, `fix`, `test`, `docs`, `spec`, `refactor`, `chore`, and `epic`.

### Bodies

An implementation issue should normally contain:

1. **Outcome** — the observable result.
2. **Context** — why the work exists and the relevant specification.
3. **In scope** — bounded responsibilities.
4. **Out of scope** — nearby work deliberately excluded.
5. **Acceptance criteria** — testable completion conditions.

Specification issues should replace implementation scope with the decisions that must be ratified. Epics should state the outcome and definition of done; their native sub-issue list provides the rollup.

Do not maintain `Depends on`, `Blocked by`, workstream-parent, milestone, or delivery checklists in issue bodies. Use GitHub's native relationships and Milestone field.

### Labels

| Family | Meaning |
| --- | --- |
| `area:*` | Affected product/code area. Use one primary area; add a supporting area only when the issue genuinely crosses a package boundary. |
| `priority:*` | Product urgency: critical, high, medium, or low. |
| `status:ready` | Fully specified, unblocked, and suitable to start now. |
| `status:in-progress` | Active implementation or an active pull request exists. |
| `status:blocked` | Promoted work with an unresolved hard dependency or required decision. |
| `status:icebox` | Valid future work not currently scheduled. |
| `type:epic` | Navigational outcome with native sub-issues. |
| `type:spec` | A decision/specification record, not implementation. |
| `blocked-by-spec` | An implementation issue that cannot begin until a separate specification issue is ratified. |
| `superseded` | Closed record retained for history after consolidation. |
| `enhancement-candidate` | Optional capability awaiting a future planning review. |

Apply no more than one `status:*` label. Native dependency state is authoritative; the status label makes the current execution view easy to scan.

## Workflow

1. Triage a new report into the appropriate workstream parent.
2. Reconcile the request with the normative specification.
3. Split work until each implementation issue has one testable outcome.
4. Record hard prerequisites with native blocked-by links.
5. Assign the earliest milestone in which the outcome is actually required.
6. Apply `status:ready` only when all prerequisites and decisions are resolved.
7. Move to `status:in-progress` when work starts; close through the implementing pull request.
8. Review blocked and icebox work during milestone planning rather than through automated inactivity closure.

Issue stale-closing is disabled. Planned work should be closed only by an explicit product decision, completion, duplication, or supersession. Pull requests may still be marked stale so abandoned implementation branches remain visible.

## Review checklist

At milestone boundaries, verify:

- [ ] Open milestone issues have a single clear outcome and testable acceptance criteria.
- [ ] Every open issue has one native parent.
- [ ] Hard prerequisites use native dependencies and are absent from the body.
- [ ] Exactly one scheduling model—the Milestone field—is in use.
- [ ] No issue has more than one `status:*` label.
- [ ] Completed, duplicate, and obsolete work is explicitly closed with a redirect comment.
- [ ] Specifications are ratified before implementation issues carrying `blocked-by-spec` become ready.
- [ ] The active queue matches GitHub's dependency state.
