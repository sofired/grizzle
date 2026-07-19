# API and Stability Policy

This document is the canonical policy for deciding which Grizzle interfaces are
supported and what compatibility guarantees they receive. Grizzle is currently
pre-v1 and is not production-ready, but pre-v1 does not mean that every exported
identifier has the same status or may change without explanation.

## API classifications

Grizzle uses three classifications:

- **Supported public**: intended for application use. Before v1 these APIs may
  still change under the pre-v1 rules below. Starting with v1, they receive the
  semantic-versioning and deprecation guarantees in this policy.
- **Experimental public**: available for evaluation, but its design or behavior
  is not yet stable. Experimental APIs do not receive source- or
  behavior-compatibility guarantees, even after v1, until they are explicitly
  promoted to supported public API.
- **Non-public implementation**: maintained for Grizzle's own implementation and
  tests. These packages and symbols are unsupported even when Go makes them
  importable or they contain exported identifiers.

### Current Go package boundaries

Every package currently built by `go list ./...` is classified below.

| Package | Classification |
|---|---|
| `github.com/sofired/grizzle/dialect` | Supported public |
| `github.com/sofired/grizzle/expr` | Supported public |
| `github.com/sofired/grizzle/query` | Supported public |
| `github.com/sofired/grizzle/schema/pg` | Supported public |
| `github.com/sofired/grizzle/schema/mysql` | Supported public |
| `github.com/sofired/grizzle/schema/sqlite` | Supported public |
| `github.com/sofired/grizzle/driver/pgx` | Supported public |
| `github.com/sofired/grizzle/driver/sql` | Supported public |
| `github.com/sofired/grizzle/kit` | Experimental public |
| `github.com/sofired/grizzle/kit/filemigrate` | Experimental public |
| `github.com/sofired/grizzle/cmd/grizzle` | Non-public implementation |
| `github.com/sofired/grizzle/gen/codegen` | Non-public implementation |
| `github.com/sofired/grizzle/gen/parser` | Non-public implementation |
| `github.com/sofired/grizzle/kit/introspect` | Non-public implementation |
| `github.com/sofired/grizzle/internal/testschema` | Non-public implementation |

All packages below any `internal/` directory are non-public implementation,
whether or not they are individually listed above. Importable implementation
packages such as `gen/codegen`, `gen/parser`, and `kit/introspect` may move under
`internal/` or otherwise change without a compatibility period.

### Other user-visible surfaces

Package classification does not by itself classify commands, generated files,
or data formats. The following surfaces are explicitly experimental:

- all `grizzle` CLI command names, flags, configuration, exit behavior, and
  machine-readable output;
- code-generation commands and configuration, and the generated Go source shape;
- migration directory layouts, artifacts, snapshots, and history-table formats.

`cmd/grizzle` is therefore a non-public Go implementation package while the CLI
it builds is an experimental public surface. Likewise, `gen/codegen` and
`gen/parser` are non-public packages while code generation's user-visible
behavior is experimental.

### Symbol boundary

In a supported public package, exported identifiers and their documented
behavior are supported public API unless their documentation explicitly marks
them `Experimental`. This includes public method sets, interfaces, accepted
inputs, returned errors, and documented SQL behavior.

`Deprecated` is lifecycle metadata, not a separate classification. Marking a
supported public package or symbol deprecated does not remove its compatibility
guarantee before the removal permitted by this policy. Experimental APIs remain
experimental when deprecated.

All exported identifiers in an experimental package inherit that package's
experimental status unless a future policy revision explicitly promotes a
smaller surface. Exported identifiers in non-public packages receive no
compatibility guarantee.

Unexported identifiers, tests, repository scripts, and implementation details
are non-public. Examples do not create a compatibility promise beyond the
public APIs and documented behavior they demonstrate.

New packages and user-visible surfaces must be added to this policy with an
explicit classification before they are presented as supported.

## What counts as a breaking change

A change is breaking when a reasonable consumer of a covered surface must
change code, configuration, generated files, stored artifacts, or documented
expectations to upgrade. Examples include:

- removing or renaming a package or exported identifier;
- changing a function signature, public type, method set, or interface;
- adding a method to an interface implemented by consumers;
- changing documented query construction, SQL rendering, errors, or runtime
  behavior incompatibly;
- changing a covered command, configuration field, generated-code contract, or
  persistent format incompatibly.

Additive APIs are normally compatible, but additions to interfaces implemented
by consumers are not. Fixing undocumented behavior is normally compatible;
changing behavior that the docs or specifications told consumers to rely on is
not merely an implementation detail.

## Pre-v1 policy

Before v1, breaking changes to supported public API are allowed when needed to
converge on the specification, correct the design, or remove a demonstrably
unsafe contract. They require all of the following:

1. A tracked issue or PR explains the reason and identifies the affected public
   packages and symbols.
2. The same change updates relevant specifications, documentation, examples,
   and tests.
3. The PR contains migration notes describing the old and new usage. Release
   notes must repeat them once tagged releases begin.
4. A compatibility shim or deprecation period is considered and used when its
   maintenance cost is reasonable, although it is not mandatory before v1.

After v0 tags begin, planned breaking changes to supported public API belong in
a new v0 minor release. V0 patch releases should remain compatible except for a
narrow security or critical-correctness exception described below.

Experimental public APIs may change or be removed before v1 without a
deprecation period. User-visible experimental changes must still be called out
in the PR and release notes; experimental status is not permission for silent
changes. Non-public implementation may change at any time.

## V1 and later policy

Starting with v1, Grizzle follows semantic versioning for supported public API:

- patch releases contain compatible fixes;
- minor releases may add compatible functionality and deprecations;
- incompatible changes require a new major version.

A supported public package or symbol scheduled for removal must have a Go doc
`Deprecated:` notice that names its replacement or migration path. For a
package, the notice must appear in its package documentation. The package or
symbol must remain available for the rest of the current major version and be
deprecated for at least one minor release before removal in the next major
version.

Documented behavior is covered alongside source compatibility. A bug fix may
restore documented behavior in a patch release, but an incompatible correction
to behavior that consumers were explicitly told to rely on must use a
compatibility path or wait for a major release.

Experimental public surfaces remain outside the v1 compatibility guarantee
until promoted by an explicit policy update. Their incompatible changes should
normally ship in a minor release and must be documented. Non-public
implementation remains outside semantic-versioning guarantees.

### Security and critical-correctness exception

When preserving compatibility would expose a security vulnerability, data
corruption, or comparably critical correctness failure, maintainers may make the
smallest necessary incompatible change without the normal release window. The
issue or security advisory and release notes must explain the exception,
affected versions, and migration path. Ordinary spec drift or cleanup does not
qualify.

## Specifications and compatibility

The files under [`docs/spec`](./spec/README.md) define Grizzle's intended behavior. This
policy defines the compatibility process for moving an implementation toward
that behavior. Both apply:

- A spec correction, a newly ratified design, or an upstream Drizzle change does
  not automatically waive compatibility requirements.
- Before v1, bringing supported public API into spec conformance may break it
  only under the pre-v1 rules above.
- Starting with v1, an incompatible conformance fix needs a shim, deprecation
  path, or major release unless the security and critical-correctness exception
  applies.
- `DEVIATION:GAP` does not make an existing exported API non-public, and
  `DEVIATION:BROKEN` is not by itself an emergency exception.
- A spec change that affects a supported or experimental surface must identify
  the compatibility impact and update this classification when necessary.

When implementation, specifications, and compatibility commitments conflict,
the change must be evaluated explicitly rather than silently choosing one of
them. Maintainers should record the decision in the governing issue or PR and
add migration guidance when consumers are affected.
