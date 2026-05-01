# Grizzle-specific deepreview checklist extensions
#
# These items are appended to the base /deepreview checklist when running
# in this project. Section headings must match agent names from the skill.

## Agent 1 — Go/Code Engineer

CORRECTNESS
[ ] No scenario produces duplicate warnings or errors for the same root cause (e.g. a schema drift detected by two independent code paths)
[ ] Every nil-guard branch in GenerateChangeSQL-style dispatch functions (switch on ChangeKind) is tested

TESTS
[ ] Every new ChangeKind value has at least one SQL output test for each supported dialect (PostgreSQL, MySQL, SQLite)

## Agent 2 — Architect

SYSTEM RISKS
[ ] Any operation with dialect-specific restrictions (e.g. no LIMIT on UPDATE in PostgreSQL) is guarded per dialect or documented with a clear error

## Agent 3 — Technical Writer

GODOC COMPLETENESS
[ ] Schema-prefixed constructors (e.g. SchemaView, SchemaCreateEnum) have the same panic documentation as their non-schema counterparts

SPEC ACCURACY
[ ] Parity labels (PARITY, GRIZZLE-ONLY, DEVIATION:*) are accurate against the pinned Drizzle version in go.mod
[ ] No spec entry says a behaviour is "not supported" when the code actually supports it (or vice versa)
[ ] Spec entries for dialect-limited operations correctly reflect which dialects are supported
[ ] Breaking API changes are documented in kit.md and/or CHANGELOG — not just in the PR description

## Agent 4 — Security Engineer

SQL SAFETY IN GENERATED DDL
[ ] All object identifiers (table names, column names, type names, schema names) flow through a quoting function before being embedded in SQL strings
[ ] All literal string values embedded in SQL (enum labels, default values) are single-quote-escaped
[ ] View/query body SQL is noted in godoc as must-be-trusted developer input (not runtime user input)

INPUT VALIDATION
[ ] Every public constructor that accepts a schema argument validates it does not contain a dot (which would break identifier parsing)
[ ] No validation gap between a Schema* constructor and its non-schema counterpart

INTROSPECTION QUERIES
[ ] All parameterised introspection queries use bind variables for any externally-sourced value (schema names, table names from the DB)

## Agent 5 — QA / SDET

CONSTRUCTOR TESTS
[ ] Schema-prefixed constructors (SchemaView, SchemaCreateEnum, etc.) have the same test coverage as their non-schema counterparts
[ ] Dot-in-schema panic (if guarded in code) is tested for each constructor that enforces it

DISPATCH / SWITCH COVERAGE
[ ] Every new case in GenerateChangeSQL (or equivalent dispatch function) has at least one SQL output test
[ ] Every new ChangeKind is tested for all relevant dialects (PostgreSQL, MySQL, SQLite)
