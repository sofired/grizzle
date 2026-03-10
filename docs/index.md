---
layout: home

hero:
  name: Grizzle
  text: Type-safe SQL for Go
  tagline: A code-generated query builder and migration toolkit inspired by Drizzle ORM. Compile-time column types. Immutable builders. Multi-dialect.
  actions:
    - theme: brand
      text: Get Started
      link: /guide/getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/sofired/grizzle

features:
  - icon: 🔒
    title: Compile-time type safety
    details: Column types enforce correct operators at compile time. A UUIDColumn only accepts UUIDs. A StringColumn only accepts strings. Type mismatches are compiler errors, not runtime panics.

  - icon: ⚡
    title: Immutable query builders
    details: Every method returns a new copy. Share and extend query fragments safely across goroutines without unexpected mutation.

  - icon: 🗄️
    title: Multi-dialect
    details: One builder API targets PostgreSQL, MySQL/MariaDB, and SQLite. Placeholder style, RETURNING, and upsert syntax differences are handled automatically.

  - icon: 🔧
    title: Migration kit
    details: Introspect your live database, diff against your Go schema, and apply DDL atomically — with a migration history table tracked in the database.

  - icon: ✨
    title: Code generation
    details: Run grizzle gen to turn your schema.go into typed table handles with a UUIDColumn, StringColumn, etc. for every column. No manual typing.

  - icon: 📦
    title: Zero magic
    details: No global state, no interface{} surprises, no hidden SQL. Every query produces a plain (string, []any) that you execute however you like.
---
