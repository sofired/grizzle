# Overview

## What Grizzle is

Grizzle is a Go port of Drizzle ORM and Drizzle Kit. It provides the same three-layer toolkit:

1. **Schema DSL** — declare your database schema in Go code
2. **Query builder** — compose type-safe SQL queries against that schema
3. **Migration kit** — keep a live database in sync with your schema definitions

The TypeScript source of truth is the [Drizzle ORM documentation](https://orm.drizzle.team/docs/overview) and [Drizzle Kit documentation](https://orm.drizzle.team/docs/kit-overview). This spec was written against **Drizzle ORM v0.40 / Drizzle Kit v0.30**.

## Parity commitment

Grizzle tracks Drizzle's design. When Drizzle changes its API or behaviour, Grizzle follows unless there is a documented reason not to.

Deviations require explicit justification in the relevant spec file. An undocumented deviation is a bug.

## Go adaptation rules

Some Drizzle design choices cannot be ported directly because TypeScript and Go differ fundamentally. The following standing rules govern how to adapt:

### Type system

Drizzle uses TypeScript's structural type system and template literal types to enforce query correctness at compile time. Go's type system is nominal and simpler. Where Drizzle achieves a type-safety guarantee through TypeScript inference, Grizzle achieves the same guarantee through a different Go-idiomatic mechanism — usually interface constraints, generic type parameters, or code generation. If no mechanism achieves equivalent safety, the gap is documented as **DEVIATION:LANGUAGE**.

### Runtime hooks

Drizzle supports `$default`, `$defaultFn`, `$onUpdate`, and `$onUpdateFn` column modifiers:

- `$default` / `$defaultFn` with a **static value** → maps to `DEFAULT <expr>` in DDL — **PARITY**
- `$defaultFn` with a **runtime function** → no equivalent; codegen emits a reminder comment — **DEVIATION:LANGUAGE**
- `$onUpdate` / `$onUpdateFn` → `.OnUpdate()` marker on the column; codegen emits a reminder comment — **DEVIATION:LANGUAGE**

Note: Drizzle's `$default(fn)` applies the function at query-construction time in JavaScript — it is not the same as a SQL `DEFAULT`. The static-value case maps cleanly to a SQL default; the function case does not.

### CLI vs library API

Drizzle Kit is a CLI tool (`drizzle-kit generate`, `drizzle-kit migrate`, etc.). Grizzle Kit exposes the same operations as both a Go library API and a CLI (`grizzle` command). The library API is idiomatic Go; the CLI mirrors Drizzle Kit's command names and flags as closely as possible.

### No magic / no reflection in hot paths

Drizzle relies on TypeScript type inference and proxies. Go has no equivalent. Grizzle uses code generation (`grizzle gen`) to produce typed structs and column handles, achieving the same user experience at the cost of a generation step. Reflection is only used for struct scanning (INSERT/UPDATE `db`-tag handling), matching Drizzle's own runtime struct inspection.

## Transactions

Drizzle exposes `db.transaction(async (tx) => { ... })` which automatically commits on success and rolls back on any thrown error.

Grizzle's equivalent is `pgxdb.Transaction(ctx, db, func(tx *pgxdb.Tx) error { ... })`. The `*pgxdb.Tx` type exposes the same `Query`, `Exec`, `QueryRaw`, `ExecRaw` methods as `*pgxdb.DB`. Return a non-nil error to trigger rollback; return nil to commit.

Nested transactions (Drizzle's `tx.transaction()`) use savepoints internally. Grizzle does not yet implement nested transactions — tracked as **#143**. See [transactions.md](./transactions.md).

## What Grizzle is not

- Not an Active Record ORM (no model objects, no `.save()`)
- Not a query planner or schema validator (that is Drizzle Studio's role)
- Not a replacement for raw SQL when raw SQL is the right tool

## Versioning and stability

Grizzle's public API follows the Drizzle ORM API where possible. Until v1.0, breaking changes may occur to close parity gaps. After v1.0, breaking changes follow semver.
