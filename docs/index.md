---
layout: home

hero:
  name: Grizzle
  text: Type-safe SQL for Go
  tagline: A code-generated query builder and migration toolkit inspired by Drizzle ORM. Compile-time column types. Composable builders. Multi-dialect.
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
    title: Composable query builders
    details: Build typed query fragments with clear dialect gates and checked SQL output. Copy-style builders may be retained as a Go adaptation, but SQL behavior is the parity target.

  - icon: 🗄️
    title: Multi-dialect
    details: Dialect-aware builders target PostgreSQL, MySQL/MariaDB, and SQLite. Shared SQL stays portable; dialect-specific mutation APIs handle differences like MySQL duplicate-key updates.

  - icon: 🔧
    title: Migration kit
    details: "Target workflow: generate migration artifacts, review them, run check, then migrate with database history tracking. Current legacy helpers remain separate until the RC.1 file workflow lands."

  - icon: ✨
    title: Code generation
    details: Run grizzle gen to turn schema definition files into typed table handles with a UUIDColumn, StringColumn, etc. for every column. No manual typing.

  - icon: 📦
    title: Zero magic
    details: No global state, controlled any-value escape hatches, and no hidden SQL. Target query builders return SQL, args, and a checked error before execution.
---
