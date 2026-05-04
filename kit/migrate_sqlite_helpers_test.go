package kit

import "testing"

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
