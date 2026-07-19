# Grizzle

[![CI](https://github.com/sofired/grizzle/actions/workflows/ci.yml/badge.svg)](https://github.com/sofired/grizzle/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/sofired/grizzle.svg)](https://pkg.go.dev/github.com/sofired/grizzle)
[![Go Report Card](https://goreportcard.com/badge/github.com/sofired/grizzle)](https://goreportcard.com/report/github.com/sofired/grizzle)

Grizzle is an early-stage Go port of [Drizzle ORM](https://orm.drizzle.team/) and Drizzle Kit concepts.

This repository is still in initial build-out. It has not been tagged, released, or published as a stable library. Public APIs, CLI commands, package boundaries, generated code shape, and migration behavior may change while the project is brought into alignment with the specification.

Do not treat the current repository state as production-ready or as a stable integration target.

The [API and stability policy](./docs/api-stability.md) identifies supported,
experimental, and non-public surfaces and defines the compatibility process
before and after v1.

## Documentation

The authoritative project specifications live under [docs/spec](./docs/spec/).

The file-migration work is pinned to Drizzle ORM / Drizzle Kit `v1.0.0-rc.1` behavior and is documented in the dedicated file-migration specs, starting with:

- [Migration Kit spec](./docs/spec/kit.md)
- [File migration workflow](./docs/spec/file-migrations-workflow.md)
- [Implementation sequence](./docs/spec/file-migrations-implementation-sequence.md)
- [Upstream mapping](./docs/spec/file-migrations-upstream-mapping.md)

The specs describe the intended implementation contract. They should not be read as evidence that every described behavior is already implemented.

## Project Status

Current development is spec-first. Before implementation continues, existing code, open PRs, and GitHub issues should be triaged against the ratified specs rather than assumed to represent the target design.

Until that work is complete, prefer the specs over examples, historical branch behavior, or in-progress implementation artifacts when evaluating intended Grizzle behavior.

## Acknowledgements

Grizzle is derived from and inspired by [Drizzle ORM](https://orm.drizzle.team/), originally created by the [Drizzle Team](https://github.com/drizzle-team).

Drizzle ORM is licensed under the [Apache License 2.0](https://github.com/drizzle-team/drizzle-orm/blob/main/LICENSE).

## License

MIT
