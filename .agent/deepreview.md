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

PAIRED OPERATION CONSISTENCY
[ ] For every paired operation in this PR (Create/Replace, Add/Drift, schema-qualified/unqualified, etc.) verify that any deferral, guard, special-case logic, or test variant present in one member is also present in the other — or is explicitly absent with a comment explaining why

## Agent 2 — Architect

SYSTEM RISKS
[ ] Any operation with dialect-specific restrictions (e.g. no LIMIT on UPDATE in PostgreSQL) is guarded per dialect or documented with a clear error

SPEC / CODE ALIGNMENT
[ ] Every spec or doc claim about what is supported, not supported, or coming soon is cross-checked against the corresponding implementation file — even if that source file is not itself in this diff
[ ] Paired operations (Create/Replace, Add/Drift, etc.) are audited as a set: any architectural property of one must be intentionally present or absent in its pair

## Agent 3 — Technical Writer

GODOC COMPLETENESS
[ ] Schema-prefixed constructors (e.g. SchemaView, SchemaCreateEnum) have the same panic documentation as their non-schema counterparts
[ ] Every new exported *method* added to an existing type has a doc comment — not just new types and constructors
[ ] Security or trust warnings on a DSL-layer type (e.g. ViewDef.SQL) are mirrored on the corresponding snapshot-layer type (e.g. ViewSnap.SQL)

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
[ ] SQL comment strings that are assembled and then executed by the migration runner are treated as live SQL — any identifier embedded in them uses %q or a quoting function, not raw %s

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

PAIRED OPERATION TEST SYMMETRY
[ ] For every paired operation (ChangeCreateView/ChangeReplaceView, enumAddedValues/enumDrift, schema-qualified/unqualified, etc.) verify that schema-qualified and non-qualified test variants exist for BOTH members of the pair
[ ] Every small utility function introduced in this PR (e.g. normalizeViewSQL, qualifyName) has at least one direct unit test — not only exercised transitively

FILTER / AGGREGATION REGRESSION
[ ] Every existing filter function (e.g. SQLiteApplyableChanges) is exercised with each new ChangeKind value added in this PR
[ ] Every existing aggregation function (e.g. AllChangeSQLMySQL, AllChangeSQLSQLite) is tested with a mixed change list that includes the new object types
