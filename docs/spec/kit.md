# Migration Kit Specification

This document is a thin index for Grizzle's migration-kit behavior.

Grizzle's migration-kit target is pinned to:

- Drizzle ORM / Drizzle Kit `v1.0.0-rc.1`

## Purpose

Grizzle's migration kit is the Go equivalent of Drizzle Kit. Its file-based migration workflow is specified in the dedicated file-migration documents rather than here.

Use this file as the entry point for the migration-kit spec set.

## Public Commands

Target public surface:

- `grizzle generate`
- `grizzle migrate`
- `grizzle check`
- `grizzle push`
- `grizzle pull`
- `grizzle introspect`

Alias rule:

- `grizzle introspect` is an alias of `grizzle pull`, matching Drizzle RC.1's `pull` / `introspect` command relationship.

Not part of the target RC.1-aligned public workflow:

- `grizzle up` (**DEVIATION:INTENTIONAL**; excluded from the initial public workflow)
- `grizzle studio` (**DEVIATION:INTENTIONAL**; excluded from the initial public workflow)
- `grizzle export` (**DEVIATION:INTENTIONAL**; deferred until a dedicated export spec exists)
- public `grizzle snapshot`
- public `grizzle diff`

`grizzle export` is **DEVIATION:INTENTIONAL** and explicitly deferred from the initial file-migration target. Drizzle RC.1 exposes `export`, but Grizzle has not specified an equivalent data/schema export workflow, output format, safety model, or Go API surface. It must not be implemented or documented as parity until a dedicated spec is written.

## Spec Map

Normative migration-kit behavior lives in:

- [file-migrations-workflow.md](./file-migrations-workflow.md)
- [file-migrations-generate.md](./file-migrations-generate.md)
- [file-migrations-artifacts.md](./file-migrations-artifacts.md)
- [file-migrations-snapshot-fields.md](./file-migrations-snapshot-fields.md)
- [file-migrations-history.md](./file-migrations-history.md)
- [file-migrations-check.md](./file-migrations-check.md)
- [file-migrations-execution.md](./file-migrations-execution.md)
- [file-migrations-api.md](./file-migrations-api.md)
- [file-migrations-implementation-sequence.md](./file-migrations-implementation-sequence.md)
- [pull.md](./pull.md)

Supporting context:

- [file-migrations-upstream-mapping.md](./file-migrations-upstream-mapping.md)
- [codegen.md](./codegen.md)

## Command Roles

- `generate`
  - schema definitions to migration artifacts
- `migrate`
  - migration artifacts to applied database changes
- `check`
  - offline artifact, snapshot, and migration-graph validation
- `push`
  - public direct-sync command boundary; full CLI/API behavior requires a dedicated push spec before implementation-ready work resumes
- `pull`
  - live database introspection to generated Go schema source, with optional bootstrap migration metadata behavior as defined in [pull.md](./pull.md)
- `introspect`
  - alias of `pull`

## Current State

Implementation status and parity tracking belong in the spec index and in the dedicated specs:

- [README.md](./README.md)
- [overview.md](./overview.md)

This file should not define legacy migration contracts or duplicate the normative file-migration specifications.
