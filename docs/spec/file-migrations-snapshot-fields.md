# RC.1 Snapshot Field Reference

This document records the Drizzle Kit `v1.0.0-rc.1` snapshot entity fields that Grizzle must validate for the initial PostgreSQL, MySQL, and SQLite file-migration target, and the initial support status for `check`, `generate`, and `pull`.

Authoritative upstream sources:

- `drizzle-kit/src/dialects/postgres/ddl.ts`
- `drizzle-kit/src/dialects/postgres/snapshot.ts`
- `drizzle-kit/src/dialects/mysql/ddl.ts`
- `drizzle-kit/src/dialects/mysql/snapshot.ts`
- `drizzle-kit/src/dialects/sqlite/ddl.ts`
- `drizzle-kit/src/dialects/sqlite/snapshot.ts`

The `snapshot.json` envelope remains the one defined in [file-migrations-artifacts.md](./file-migrations-artifacts.md): `version`, `dialect`, `id`, `prevIds`, `ddl`, and `renames`. The `ddl` array contains typed entity records produced by each dialect's `createDDL()`. Every entity record has `entityType` and `name` plus the fields listed below.

## JSON Shape Notation

RC.1's DDL collection helper materializes configured fields with `null` defaults before validation. In this document:

- `required non-null` means the key must be present and its value must match the stated non-null type
- `required nullable` means the key must be present and may be `null`; if non-null, it must match the stated type or nested shape
- `nested required` means a non-null object must contain the listed child keys according to the same required/nullable rules
- `unsupported_feature` means Grizzle must validate the field shape enough to diagnose it, then fail before accepting the artifact graph or generating output
- pull-only metadata is not serialized into `snapshot.json`; it is listed separately when needed by source generation

The implementation must include generated JSON schema, Go validator structs, or checked-in fixture catalogs derived from the tagged RC.1 `ddl.ts` files before code work proceeds. This markdown reference is the human-readable field index, not a substitute for executable validators.

## PostgreSQL

Envelope: `version = "8"`, `dialect = "postgres"`.

| Entity | Required non-null fields | Required nullable fields | Initial support notes |
| --- | --- | --- | --- |
| `schemas` | `name string` | none | Supported for `check`, `generate`, and `pull` |
| `tables` | `name string`, `schema string`, `isRlsEnabled boolean` | none | Supported only when `isRlsEnabled=false`; `true` fails with `unsupported_feature` for all commands until RLS/policies are specified |
| `enums` | `name string`, `schema string`, `values string[]` | none | Supported |
| `columns` | `name string`, `schema string`, `table string`, `type string`, `notNull boolean`, `dimensions number` | `typeSchema string`, `default string`, `generated object`, `identity object` | `generated` and `identity` validate shape then fail `unsupported_feature` for all commands in the initial target |
| `indexes` | `name string`, `schema string`, `table string`, `nameExplicit boolean`, `columns array`, `isUnique boolean`, `with string`, `method string`, `concurrently boolean` | `where string` | Supported only for the allowlist below; unsupported index methods/options, expression columns, and non-default opclasses fail `unsupported_feature` |
| `fks` | `name string`, `schema string`, `table string`, `nameExplicit boolean`, `columns string[]`, `schemaTo string`, `tableTo string`, `columnsTo string[]` | `onUpdate enum`, `onDelete enum` | Supported; FK actions are `NO ACTION`, `RESTRICT`, `SET NULL`, `CASCADE`, `SET DEFAULT`, or null |
| `pks` | `name string`, `schema string`, `table string`, `columns string[]`, `nameExplicit boolean` | none | Supported |
| `uniques` | `name string`, `schema string`, `table string`, `nameExplicit boolean`, `columns string[]`, `nullsNotDistinct boolean` | none | Supported when `nullsNotDistinct=false`; `true` fails `unsupported_feature` until the schema DSL/serializer exposes `.NullsNotDistinct()` |
| `checks` | `name string`, `schema string`, `table string`, `value string` | none | Supported when `value` comes from typed DDL-expression rendering for public schema input |
| `views` | `name string`, `schema string`, `materialized boolean` | `definition string`, `with object`, `withNoData boolean`, `using string`, `tablespace string` | Basic regular views supported as defined below; materialized views and unsupported options fail `unsupported_feature` |
| `sequences`, `roles`, `privileges`, `policies` | RC.1 fields exist | varies | Recognized but unsupported initial object families; validate shape when possible, then fail `unsupported_object_family` |

PostgreSQL index column shape: each entry has required non-null `value string`, `isExpression boolean`, `asc boolean`, `nullsFirst boolean`, and required nullable `opclass object`. A non-null `opclass` has nested required non-null `name string` and `default boolean`.

PostgreSQL initial index support allowlist:

- `method` must be `"btree"`.
- `with` must be the empty string.
- `concurrently` must be `false`.
- `where` may be `null` or a predicate rendered by Grizzle's typed DDL-expression renderer.
- every column must have `isExpression=false`.
- every column `opclass` must be `null` or `{ "default": true, ... }`; a non-default opclass name fails `unsupported_feature`.
- `asc` and `nullsFirst` are supported because RC.1 stores both for PostgreSQL index columns.

PostgreSQL `columns.generated`: required nullable object. When non-null, it has nested required non-null `type = "stored"` and `as string`. Recognized but unsupported in all initial commands.

PostgreSQL `columns.identity`: required nullable object. When non-null, it has nested required non-null `name string`, `type = "always" | "byDefault"` and nested required nullable `increment string`, `minValue string`, `maxValue string`, `startWith string`, `cache number`, `cycle boolean`. Recognized but unsupported in all initial commands.

PostgreSQL `views.with`: required nullable object. When non-null, it has nested keys `checkOption`, `securityBarrier`, `securityInvoker`, `fillfactor`, `toastTupleTarget`, `parallelWorkers`, `autovacuumEnabled`, `vacuumIndexCleanup`, `vacuumTruncate`, `autovacuumVacuumThreshold`, `autovacuumVacuumScaleFactor`, `autovacuumVacuumCostDelay`, `autovacuumVacuumCostLimit`, `autovacuumFreezeMinAge`, `autovacuumFreezeMaxAge`, `autovacuumFreezeTableAge`, `autovacuumMultixactFreezeMinAge`, `autovacuumMultixactFreezeMaxAge`, `autovacuumMultixactFreezeTableAge`, `logAutovacuumMinDuration`, `userCatalogTable`, each following the RC.1 enum/number/boolean nullable shape. Initial support accepts only `with=null` or a no-op object whose values are null/default-equivalent; non-default view options fail `unsupported_feature`.

PostgreSQL view support status:

| Field combination | `check` | `generate` | `pull` |
| --- | --- | --- | --- |
| `materialized=false`, non-null `definition`, no non-default `with`, `withNoData=null/false`, `using=null`, `tablespace=null` | Supported | Supported | Supported |
| `materialized=true` | `unsupported_feature` | `unsupported_feature` | `unsupported_feature` |
| `withNoData=true`, non-null `using`, non-null `tablespace`, or non-default `with` option | `unsupported_feature` | `unsupported_feature` | `unsupported_feature` |

PostgreSQL pull view-column metadata used for generated Go source, not serialized into `snapshot.json`: `schema string`, `view string`, `name string`, `type string`, `typeSchema string|null`, `typeDimensions number`, `dimensions number`, `notNull boolean`. Grizzle carries `dimensions` on `TypeRef.Dimensions` and preserves RC.1 `typeDimensions` separately on `IntrospectionViewColumn.TypeDimensions`. Grizzle also carries or derives `IntrospectionViewColumn.PropertyKey` as pull/source-generation metadata for generated Go view handles; it is not an RC.1 snapshot field.

## MySQL

Envelope: `version = "6"`, `dialect = "mysql"`.

| Entity | Required non-null fields | Required nullable fields | Initial support notes |
| --- | --- | --- | --- |
| `tables` | `name string` | none | Supported |
| `columns` | `name string`, `table string`, `type string`, `notNull boolean`, `autoIncrement boolean`, `onUpdateNow boolean` | `default string`, `onUpdateNowFsp number`, `charSet string`, `collation string`, `generated object` | Supported only when `onUpdateNow=false`, `onUpdateNowFsp=null`, `charSet=null`, and `collation=null`; `generated`, `onUpdateNow=true`, non-null `onUpdateNowFsp`, non-null `charSet`, and non-null `collation` validate shape then fail `unsupported_feature` |
| `pks` | `name string`, `table string`, `columns string[]` | none | Supported |
| `fks` | `name string`, `table string`, `columns string[]`, `tableTo string`, `columnsTo string[]`, `nameExplicit boolean` | `onUpdate enum`, `onDelete enum` | Supported; FK actions are `NO ACTION`, `RESTRICT`, `SET NULL`, `CASCADE`, `SET DEFAULT`, or null |
| `indexes` | `name string`, `table string`, `columns array`, `isUnique boolean`, `nameExplicit boolean` | `using enum`, `algorithm enum`, `lock enum` | Supported only when `using=null` or `btree`, `algorithm=null` or `default`, and `lock=null` or `default`; non-default `hash`, `inplace`, `copy`, `none`, `shared`, or `exclusive` values fail `unsupported_feature` |
| `checks` | `name string`, `table string`, `value string` | none | Supported when `value` comes from typed DDL-expression rendering for public schema input |
| `views` | `name string`, `definition string`, `algorithm enum`, `sqlSecurity enum` | `withCheckOption enum` | Basic views supported as defined below |

MySQL index column shape: each entry has required non-null `value string` and `isExpression boolean`.

MySQL `columns.generated`: required nullable object. When non-null, it has nested required non-null `type = "stored" | "virtual"` and `as string`. Recognized but unsupported in all initial commands.

MySQL view metadata: `algorithm = "undefined" | "merge" | "temptable"`, `sqlSecurity = "definer" | "invoker"`, required nullable `withCheckOption = "local" | "cascaded" | null`.

MySQL view support status:

| Field combination | `check` | `generate` | `pull` |
| --- | --- | --- | --- |
| non-null `definition`, `algorithm="undefined"`, `sqlSecurity="definer"`, `withCheckOption=null` | Supported | Supported | Supported |
| `algorithm!="undefined"`, `sqlSecurity!="definer"`, or non-null `withCheckOption` | `unsupported_feature` | `unsupported_feature` | `unsupported_feature` |

MySQL pull view-column metadata used for generated Go source, not serialized into `snapshot.json`: `view string`, `name string`, `type string`, `notNull boolean`. Grizzle also carries or derives `IntrospectionViewColumn.PropertyKey` as pull/source-generation metadata for generated Go view handles; it is not an RC.1 snapshot field.

## SQLite

Envelope: `version = "7"`, `dialect = "sqlite"`.

| Entity | Required non-null fields | Required nullable fields | Initial support notes |
| --- | --- | --- | --- |
| `tables` | `name string` | none | Supported |
| `columns` | `name string`, `table string`, `type string`, `notNull boolean` | `autoincrement boolean`, `default string`, `generated object` | `generated` validates shape then fails `unsupported_feature` in all initial commands |
| `indexes` | `name string`, `table string`, `columns array`, `isUnique boolean`, `origin enum` | `where string` | Supported only for the allowlist below; auto indexes and expression columns fail `unsupported_feature` |
| `fks` | `name string`, `table string`, `columns string[]`, `tableTo string`, `columnsTo string[]`, `onUpdate string`, `onDelete string`, `nameExplicit boolean` | none | Supported |
| `pks` | `name string`, `table string`, `columns string[]`, `nameExplicit boolean` | none | Supported |
| `uniques` | `name string`, `table string`, `columns string[]`, `nameExplicit boolean` | none | Supported |
| `checks` | `name string`, `table string`, `value string` | none | Supported when `value` comes from typed DDL-expression rendering for public schema input |
| `views` | `name string`, `isExisting boolean` | `definition string`, `error string` | Basic views supported as defined below |

SQLite index column shape: each entry has required non-null `value string` and `isExpression boolean`. `origin` is `manual` or `auto`.

SQLite initial index support allowlist:

- `origin` must be `"manual"`.
- `where` may be `null` or a predicate rendered by Grizzle's typed DDL-expression renderer.
- every column must have `isExpression=false`.

SQLite `columns.generated`: required nullable object. When non-null, it has nested required non-null `type = "stored" | "virtual"` and `as string`. Recognized but unsupported in all initial commands.

SQLite view support status:

| Field combination | `check` | `generate` | `pull` |
| --- | --- | --- | --- |
| `isExisting=false`, non-null `definition`, `error=null` | Supported | Supported | Supported |
| `isExisting=true`, `definition=null`, or non-null `error` | `unsupported_feature` | `unsupported_feature` | `unsupported_feature` |

SQLite pull view-column metadata used for generated Go source, not serialized into `snapshot.json`: `view string`, `name string`, `type string`, `notNull boolean`. Grizzle also carries or derives `IntrospectionViewColumn.PropertyKey` as pull/source-generation metadata for generated Go view handles; it is not an RC.1 snapshot field.

## Validation Rules

- Unknown entity families fail with `unsupported_object_family`.
- Known but unsupported families for the initial target fail with `unsupported_object_family`.
- Unknown fields, wrong scalar types, wrong enum values, and unsupported properties inside known families fail with `unsupported_feature` or `malformed_snapshot` according to whether the problem is unsupported scope or invalid JSON shape.
- Public `SchemaInput` must be lossless for every accepted field above or fail before snapshot serialization.
- Checked-in fixtures must include at least one representative `snapshot.json` per supported dialect covering tables, columns, indexes, FKs, PKs, uniques, checks, and views.
- Fixtures must include recognized-but-unsupported cases for generated columns, identity columns, RLS/policies, materialized views, non-default view options, and unsupported index options, proving they fail with the documented stable code.
