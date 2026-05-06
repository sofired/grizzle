# Overview

## What Grizzle is

Grizzle is a Go port of Drizzle ORM and Drizzle Kit. It provides the same three-layer toolkit:

1. **Schema DSL** — declare your database schema in Go code
2. **Query builder** — compose type-safe SQL queries against that schema
3. **Migration kit** — generate, review, check, and apply file-based migration artifacts

The TypeScript upstream reference is Drizzle ORM / Drizzle Kit. The current migration-kit and file-migrations target is **Drizzle ORM / Drizzle Kit v1.0.0-rc.1**.

For migration-kit behavior, source authority is:

1. Grizzle specs in this directory
2. tagged Drizzle `v1.0.0-rc.1` source
3. Drizzle docs for the matching release line
4. newer or mixed-version Drizzle docs and tutorials

Tagged source wins when Drizzle docs and implementation disagree. Grizzle specs win when they document an intentional Go or safety divergence from that tagged source.

## Parity commitment

Grizzle tracks Drizzle's design. When Drizzle changes its API or behavior, Grizzle follows unless there is a documented reason not to.

Deviations require explicit justification in the relevant spec file. An undocumented deviation is a bug.

## Go adaptation rules

Some Drizzle design choices cannot be ported directly because TypeScript and Go differ fundamentally. The following standing rules govern how to adapt:

### Type system

Drizzle uses TypeScript's structural type system and template literal types to enforce query correctness at compile time. Go's type system is nominal and simpler. Where Drizzle achieves a type-safety guarantee through TypeScript inference, Grizzle achieves the same guarantee through a different Go-idiomatic mechanism — usually interface constraints, generic type parameters, or code generation. If no mechanism achieves equivalent safety, the gap is documented as **DEVIATION:LANGUAGE**.

### Runtime hooks

Drizzle supports SQL defaults plus runtime insert/update hooks:

- `.default(value)` → maps to SQL `DEFAULT <expr>` in DDL — **PARITY**
- `$defaultFn` / `$default` → runtime insert hook aliases with no DDL equivalent; codegen records metadata/comment, and INSERT builders fail with `unsupported_feature` when Drizzle would invoke the hook until a Go hook API exists — **DEVIATION:GAP (designed)**
- `$onUpdateFn` / `$onUpdate` → runtime update/insert hook aliases; `.OnUpdate()` records metadata/comment, and INSERT plus UPDATE/UPSERT builders fail with `unsupported_feature` when Drizzle would invoke the hook until a Go hook API exists — **DEVIATION:GAP (designed)**

Note: Drizzle's `$default(fn)` / `$defaultFn(fn)` applies the function at query-construction time in JavaScript — it is not the same as a SQL `DEFAULT`.

### CLI vs library API

Drizzle Kit is a CLI tool (`drizzle-kit generate`, `drizzle-kit migrate`, etc.). Grizzle Kit exposes the documented RC.1 file-migration subset as both a Go library API and a CLI (`grizzle` command). The library API is idiomatic Go; the CLI follows Drizzle Kit command roles where they are in scope and labels intentional deviations such as omitted `studio` / `up` / `export`, secret-reference credentials, and safety-hardening flags.

### No magic / no reflection in hot paths

Drizzle relies on TypeScript type inference and object-key assignment mapping. Go has no equivalent. Grizzle uses code generation (`grizzle gen`) to produce typed structs and column handles, achieving the same user experience at the cost of a generation step. Reflection is only used for struct scanning (INSERT/UPDATE `db`-tag handling) as a DEVIATION:LANGUAGE adapter over Drizzle's object-key assignment semantics.

## Transactions

Drizzle exposes `db.transaction(async (tx) => { ... })` which automatically commits on success and rolls back on any thrown error.

Grizzle's equivalent is `db.Transaction(ctx, func(tx *pgxdb.Tx) error { ... })`. The `*pgxdb.Tx` type exposes the same `Query`, `Exec`, `QueryRaw`, `ExecRaw` methods as `*pgxdb.DB`. Return a non-nil error to trigger rollback; return nil to commit.

Nested transactions (Drizzle's `tx.transaction()`) use savepoints internally. Grizzle does not yet implement nested transactions — tracked as **#143**. See [transactions.md](./transactions.md).

## What Grizzle is not

- Not an Active Record ORM (no model objects, no `.save()`)
- Not a query planner, data model designer, or lint-style schema advisor. File migrations still perform strict schema-input, snapshot, artifact, and graph validation for the object families they claim to support; Drizzle Studio-style design validation is outside the initial target.
- Not a replacement for raw SQL when raw SQL is the right tool

## Versioning and stability

Grizzle's public API follows the Drizzle ORM API where possible. Until v1.0, breaking changes may occur to close parity gaps. After v1.0, breaking changes follow semver.
