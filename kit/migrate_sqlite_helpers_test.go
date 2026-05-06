package kit

import (
	"reflect"
	"testing"
)

func TestSplitSQLStatements(t *testing.T) {
	cases := []struct {
		desc string
		in   string
		want []string
	}{
		{
			desc: "simple two-statement file",
			in:   "CREATE TABLE a (id INT);\nCREATE TABLE b (id INT);",
			want: []string{"CREATE TABLE a (id INT)", "CREATE TABLE b (id INT)"},
		},
		{
			desc: "semicolon inside single-quoted literal is not a split point",
			in:   "COMMENT ON TABLE users IS 'internal; do not drop';",
			want: []string{"COMMENT ON TABLE users IS 'internal; do not drop'"},
		},
		{
			desc: "escaped single-quote inside literal",
			in:   "INSERT INTO t VALUES ('it''s here; really');\nSELECT 1;",
			want: []string{"INSERT INTO t VALUES ('it''s here; really')", "SELECT 1"},
		},
		{
			desc: "semicolon inside double-quoted identifier is not a split point",
			in:   `ALTER TABLE t RENAME COLUMN "old;name" TO new_name;`,
			want: []string{`ALTER TABLE t RENAME COLUMN "old;name" TO new_name`},
		},
		{
			desc: "semicolon inside line comment is not a split point",
			in:   "-- step 1; setup\nCREATE TABLE t (id INT);",
			want: []string{"-- step 1; setup\nCREATE TABLE t (id INT)"},
		},
		{
			desc: "semicolon inside block comment is not a split point",
			in:   "/* step 1; setup */\nCREATE TABLE t (id INT);",
			want: []string{"/* step 1; setup */\nCREATE TABLE t (id INT)"},
		},
		{
			desc: "trailing semicolon produces no extra empty statement",
			in:   "SELECT 1;",
			want: []string{"SELECT 1"},
		},
		{
			desc: "empty input returns nil",
			in:   "",
			want: nil,
		},
		{
			desc: "whitespace-only input returns nil",
			in:   "   \n\t  ",
			want: nil,
		},
		{
			desc: "statement without trailing semicolon is captured",
			in:   "SELECT 1",
			want: []string{"SELECT 1"},
		},
		{
			desc: "check constraint with semicolon in literal",
			in:   "ALTER TABLE t ADD CONSTRAINT chk CHECK (col NOT LIKE '%;%');\nCREATE INDEX idx ON t (col);",
			want: []string{"ALTER TABLE t ADD CONSTRAINT chk CHECK (col NOT LIKE '%;%')", "CREATE INDEX idx ON t (col)"},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			got := splitSQLStatements(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("splitSQLStatements(%q)\n got  %#v\n want %#v", c.in, got, c.want)
			}
		})
	}
}

func TestStripLeadingCommentLines(t *testing.T) {
	cases := []struct {
		desc string
		in   string
		want string
	}{
		{
			desc: "pure comment-only fragment returns empty",
			in:   "-- ALTER COLUMN type change not supported",
			want: "",
		},
		{
			desc: "comment then SQL returns SQL",
			in:   "-- create users\nCREATE TABLE users (id INTEGER PRIMARY KEY)",
			want: "CREATE TABLE users (id INTEGER PRIMARY KEY)",
		},
		{
			desc: "leading blank line then comment then SQL",
			in:   "\n-- header comment\n\nCREATE TABLE t (id INTEGER)",
			want: "CREATE TABLE t (id INTEGER)",
		},
		{
			desc: "multiple leading comment lines then SQL",
			in:   "-- line 1\n-- line 2\nCREATE TABLE t (id INTEGER)",
			want: "CREATE TABLE t (id INTEGER)",
		},
		{
			desc: "no leading comment passes through unchanged",
			in:   "CREATE TABLE t (id INTEGER)",
			want: "CREATE TABLE t (id INTEGER)",
		},
		{
			desc: "empty input returns empty",
			in:   "",
			want: "",
		},
		{
			desc: "block comment is not stripped (valid SQL, left intact)",
			in:   "/* header */\nCREATE TABLE t (id INTEGER)",
			want: "/* header */\nCREATE TABLE t (id INTEGER)",
		},
		{
			desc: "indented comment line is stripped",
			in:   "   -- indented comment\nCREATE TABLE t (id INTEGER)",
			want: "CREATE TABLE t (id INTEGER)",
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			got := stripLeadingCommentLines(c.in)
			if got != c.want {
				t.Errorf("stripLeadingCommentLines(%q)\n got  %q\n want %q", c.in, got, c.want)
			}
		})
	}
}
