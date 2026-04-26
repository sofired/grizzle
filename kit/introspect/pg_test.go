package introspect

// Integration tests for queryForeignKeys require a live PostgreSQL database
// and are not included here. Specifically, the multi-column FK path — where
// rows arrive ordered by ordinal_position and must be grouped by constraint
// name with local/ref columns matched positionally — is only exercised by
// running against a real database. See the acceptance criteria in issue #36
// for the round-trip integration test requirements.
