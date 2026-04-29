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
	}

	for _, tc := range cases {
		got := normalizeDefaultExpr(tc.in)
		if got != tc.want {
			t.Errorf("normalizeDefaultExpr(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
