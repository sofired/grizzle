# Grizzle — Project Overview

## Purpose
Go port of Drizzle ORM + Drizzle Kit. Type-safe database toolkit for Go.
Module: `github.com/sofired/grizzle`

## Architecture

### Package Structure
```
dialect/          — Dialect interface + Postgres/MySQL/SQLite implementations
expr/             — Expression system (type-safe WHERE clause building)
  context.go      — BuildContext (arg accumulator + dialect carrier)
  expr.go         — Expression interface, And/Or/Not, internal expression types
  columns.go      — ColBase, UUIDColumn, StringColumn, IntColumn, BoolColumn, TimestampColumn, JSONBColumn[T]
schema/pg/        — PostgreSQL schema DSL
  column.go       — Column builders: UUID, Varchar, Text, Boolean, Integer, BigInt, Serial, Timestamp, JSONB, Numeric
  constraint.go   — Constraint builders: Index, UniqueIndex, Check, ForeignKey, CompositePrimaryKey
  table.go        — Table(), C(), TableDef, SchemaTable()
query/            — Query builders (all immutable, method chaining)
  query.go        — TableSource interface, joinClause, shared helpers
  select.go       — SelectBuilder (From/Where/Join/OrderBy/GroupBy/Limit/Offset)
  insert.go       — InsertBuilder (Values/ValueSlice/Returning + struct reflection)
  update.go       — UpdateBuilder (Set/SetStruct/Where/Returning + struct reflection)
  delete.go       — DeleteBuilder (Where/Returning)
driver/pgx/       — pgx v5 adapter
  db.go           — DB, Tx, ScanAll[T], ScanOne[T], ScanOneOpt[T], FromSelect[T]
internal/testschema/ — Test schema mirroring uncloak-identity patterns
kit/              — Migration tooling (stubbed directories, not yet implemented)
```

## Key Design Decisions
1. **Immutable query builders** — every method returns a copy, safe to share/extend
2. **ColBase embedding** — all column types embed ColBase for common IsNull/IsNotNull/Asc/Desc
3. **TableSource interface** — GRizTableName() + GRizTableAlias() — implemented by generated table types
4. **Nil-safe And/Or** — nil expressions silently dropped, key for dynamic WHERE
5. **BuildContext** — carries dialect + accumulates args, threaded through all ToSQL calls
6. **structToColVals reflection** — Insert/Update accept db-tagged structs; nil pointers = omit
7. **ScanAll/ScanOne generics** — thin wrappers over pgx.CollectRows + pgx.RowToStructByName[T]

## Generated Code Pattern
Schema definition (user writes) → `grizzle gen` → typed Go structs:
- `RealmsTable` struct with typed column fields (UUIDColumn, StringColumn, etc.)
- `var RealmsT = RealmsTable{...}` singleton for use in queries
- `RealmSelect` struct (all columns, nullable = pointer types)
- `RealmInsert` struct (required fields plain, optional/defaulted = pointers with omitempty)
- `RealmUpdate` struct (all pointer fields for partial updates)

## What's Next
1. `grizzle gen` — code generator reading schema.go → emitting *_gen.go files
2. `kit/` — snapshot serialization, differ, SQL generation, CLI (generate/migrate/push/pull)
3. MySQL dialect query builder
4. Prepared statements
5. Relations definition + relational query API

## Go Not Installed on Dev Machine
Go is not installed in /home/claude/ environment. Run `go mod tidy && go test ./...` locally.
The code has been manually reviewed for correctness.
