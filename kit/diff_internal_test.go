package kit

import "testing"

func TestNormalizeDefaultExpr(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Cast-operator spacing variants
		{"'{}' ::jsonb", "'{}'::jsonb"},
		{"'{}' :: jsonb", "'{}'::jsonb"},
		{"'{}'::\tjsonb", "'{}'::jsonb"},
		{"'{}'::jsonb", "'{}'::jsonb"},
		{"'[]' ::jsonb", "'[]'::jsonb"},
		{"'[]'::jsonb", "'[]'::jsonb"},

		// Multiple casts in one expression
		{"NULL ::text ::citext", "NULL::text::citext"},

		// String literals containing :: are left verbatim
		{"'foo :: bar'::text", "'foo :: bar'::text"},
		{"'it''s :: here'::text", "'it''s :: here'::text"},

		// Non-cast expressions unchanged
		{"gen_random_uuid()", "gen_random_uuid()"},
		{"now()", "now()"},
		{"true", "true"},
		{"42", "42"},
		{"NULL", "NULL"},

		// Leading/trailing whitespace stripped
		{"  '{}'::jsonb  ", "'{}'::jsonb"},
		{"  gen_random_uuid()  ", "gen_random_uuid()"},

		// Empty string
		{"", ""},

		// Unterminated string literal — the scanner never finds a closing quote,
		// so the :: is left inside the in-progress literal copy and is not
		// treated as a cast operator. No explicit handling; behavior is a
		// consequence of normal scan flow.
		{"'unclosed ::jsonb", "'unclosed ::jsonb"},

		// Dollar-quoted strings: :: inside $$ is incorrectly collapsed because
		// dollar-quoting is not supported (see function doc). This test documents
		// the current behavior; fix tracked in a follow-up issue.
		{"$$foo :: bar$$::text", "$$foo::bar$$::text"},
	}

	for _, tc := range cases {
		got := normalizeDefaultExpr(tc.in)
		if got != tc.want {
			t.Errorf("normalizeDefaultExpr(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
