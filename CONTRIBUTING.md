# Contributing to Grizzle

> **Contributions are temporarily paused** while the project stabilises. We are not accepting pull requests from external contributors at this time. Feel free to open issues to report bugs or propose features — we will review them once contributions reopen.

Thank you for your interest in contributing! This document covers how to report bugs, propose features, and submit pull requests.

## Reporting bugs

Open an issue using the **Bug report** template. Include:
- Go version (`go version`)
- Grizzle version or commit SHA
- Minimal reproducer (schema definition + query builder + expected vs. actual SQL)

## Proposing features

Open an issue using the **Feature request** template before writing code. This lets us discuss the design and avoid duplicated effort.

Repository maintainers organize accepted work using a single scheduling model based on GitHub Milestones and the native issue relationships described in the [project plan](docs/project-plan.md). Issue bodies should focus on outcome, scope, and testable acceptance criteria rather than duplicating dependency lists.

## Development setup

```sh
git clone https://github.com/sofired/grizzle.git
cd grizzle
go test ./...
```

No external services are required for the unit tests. Integration tests (under `driver/pgx/`) require a PostgreSQL instance — set `GRIZZLE_TEST_DSN` to a connection string to run them.

## Pull requests when contributions reopen

1. **Fork** the repository and create a branch from `main`.
2. Keep changes focused — one logical change per PR.
3. Add or update tests for any behaviour you change.
4. Run `go test ./...` and `go vet ./...` locally before pushing.
5. Fill in the PR template fully, including the test plan.

All PRs are squash-merged onto `main`. Write a clear PR title — it becomes the commit message.

## Code style

- Standard `gofmt` formatting — no exceptions.
- Exported symbols must have doc comments.
- Prefer table-driven tests using `testing.T`.
- Query builders must remain immutable: every mutating method returns a copy.

## Licence

By submitting a pull request you agree that your contribution will be licensed under the [MIT Licence](LICENSE).
