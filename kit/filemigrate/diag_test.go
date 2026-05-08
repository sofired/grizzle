package filemigrate_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/sofired/grizzle/kit/filemigrate"
)

func TestErrorIs(t *testing.T) {
	t.Run("matches sentinel by code", func(t *testing.T) {
		err := &filemigrate.Error{Code: filemigrate.CodeInvalidPath, Op: "test"}
		if !errors.Is(err, filemigrate.ErrInvalidPath) {
			t.Fatal("expected errors.Is to match ErrInvalidPath")
		}
		if errors.Is(err, filemigrate.ErrResourceLimit) {
			t.Fatal("expected errors.Is NOT to match ErrResourceLimit")
		}
	})

	t.Run("wraps context sentinel", func(t *testing.T) {
		inner := errors.New("redacted cause")
		err := &filemigrate.Error{Code: filemigrate.CodeMigrationExecution, Op: "op", Err: inner}
		if !errors.Is(err, filemigrate.ErrMigrationExecution) {
			t.Fatal("code sentinel not matched")
		}
		if !errors.Is(err, inner) {
			t.Fatal("wrapped cause not reachable via errors.Is")
		}
	})

	t.Run("errors.As recovers typed struct", func(t *testing.T) {
		orig := &filemigrate.Error{
			Code:      filemigrate.CodeDuplicateMigration,
			Op:        "test_op",
			Migration: "20240101_init",
		}
		var got *filemigrate.Error
		if !errors.As(orig, &got) {
			t.Fatal("errors.As should recover *Error")
		}
		if got.Migration != "20240101_init" {
			t.Fatalf("unexpected Migration: %q", got.Migration)
		}
	})

	t.Run("ExecutionError is also *Error", func(t *testing.T) {
		idx := 3
		ee := &filemigrate.ExecutionError{
			Base:           filemigrate.Error{Code: filemigrate.CodeMigrationExecution, Op: "exec"},
			StatementIndex: &idx,
			HistoryStage:   filemigrate.HistoryStageInsert,
		}
		if !errors.Is(ee, filemigrate.ErrMigrationExecution) {
			t.Fatal("ExecutionError should match ErrMigrationExecution")
		}
	})

	t.Run("errors.As recovers Base from ExecutionError", func(t *testing.T) {
		idx := 3
		ee := &filemigrate.ExecutionError{
			Base: filemigrate.Error{
				Code:      filemigrate.CodeMigrationExecution,
				Op:        "exec",
				Migration: "20240101_users",
				Dialect:   "postgresql",
			},
			StatementIndex: &idx,
			HistoryStage:   filemigrate.HistoryStageInsert,
		}
		var got *filemigrate.Error
		if !errors.As(ee, &got) {
			t.Fatal("errors.As should recover *Error from ExecutionError")
		}
		if got == nil {
			t.Fatal("recovered *Error is nil")
		}
		if got.Migration != "20240101_users" {
			t.Errorf("Migration: got %q, want %q", got.Migration, "20240101_users")
		}
		if got.Code != filemigrate.CodeMigrationExecution {
			t.Errorf("Code: got %q, want %q", got.Code, filemigrate.CodeMigrationExecution)
		}
		if got.Dialect != "postgresql" {
			t.Errorf("Dialect: got %q, want %q", got.Dialect, "postgresql")
		}
	})

	t.Run("errors.As recovers Base from PartialApplicationError", func(t *testing.T) {
		idx := 1
		pae := &filemigrate.PartialApplicationError{
			ExecutionError: filemigrate.ExecutionError{
				Base: filemigrate.Error{
					Code:      filemigrate.CodePartialApplication,
					Op:        "apply",
					Migration: "20240101_partial",
				},
				StatementIndex:   &idx,
				HistoryStage:     filemigrate.HistoryStageInsert,
				MayHaveCommitted: true,
			},
			HistoryInsertStarted:   true,
			HistoryInsertSucceeded: false,
		}
		var got *filemigrate.Error
		if !errors.As(pae, &got) {
			t.Fatal("errors.As should recover *Error from PartialApplicationError")
		}
		if got.Migration != "20240101_partial" {
			t.Errorf("Migration: got %q, want %q", got.Migration, "20240101_partial")
		}
		if got.Code != filemigrate.CodePartialApplication {
			t.Errorf("Code: got %q, want %q", got.Code, filemigrate.CodePartialApplication)
		}
	})
}

func TestErrorString(t *testing.T) {
	t.Run("includes code and op", func(t *testing.T) {
		err := &filemigrate.Error{Code: filemigrate.CodeInvalidPath, Op: "artifact_store.resolve_root"}
		s := err.Error()
		if !strings.Contains(s, "invalid_path") {
			t.Errorf("expected code in error string, got %q", s)
		}
		if !strings.Contains(s, "artifact_store.resolve_root") {
			t.Errorf("expected op in error string, got %q", s)
		}
	})

	t.Run("path is included when set", func(t *testing.T) {
		err := &filemigrate.Error{Code: filemigrate.CodeInvalidPath, Op: "op", Path: `"safe/path"`}
		if !strings.Contains(err.Error(), "path=") {
			t.Errorf("expected path field in error string, got %q", err.Error())
		}
	})

	// migration field is rendered with bounded length and control-char
	// escaping so a caller-supplied attacker-controlled name cannot inject
	// newlines, terminal controls, or unbounded text into CLI logs. The
	// raw value remains accessible via errors.As; only the Error() output
	// is sanitized — the subtest below also asserts that contract.
	t.Run("migration is escaped and bounded", func(t *testing.T) {
		// Control character must not appear literally in the rendered string.
		err := &filemigrate.Error{
			Code:      filemigrate.CodeDuplicateMigration,
			Op:        "op",
			Migration: "20240101_init\nINJECTED",
		}
		s := err.Error()
		if strings.Contains(s, "\n") {
			t.Errorf("rendered error contains a literal newline: %q", s)
		}
		if !strings.Contains(s, `migration="20240101_init\nINJECTED"`) {
			t.Errorf("expected escaped migration field, got %q", s)
		}
		// errors.As must still surface the raw, unescaped value so callers
		// classifying errors programmatically see exactly what was passed in.
		var recovered *filemigrate.Error
		if !errors.As(err, &recovered) {
			t.Fatal("errors.As failed to recover *Error")
		}
		if recovered.Migration != "20240101_init\nINJECTED" {
			t.Errorf("errors.As Migration: got %q, want raw unescaped value", recovered.Migration)
		}
	})

	// Carriage-return overwrite and ANSI CSI sequences are well-known
	// log-injection vectors. Verify they are rendered as escape sequences,
	// never as literal control bytes.
	t.Run("migration carriage return and ANSI escape are rendered safe", func(t *testing.T) {
		err := &filemigrate.Error{
			Code:      filemigrate.CodeDuplicateMigration,
			Op:        "op",
			Migration: "name\r\x1b[2J\x00null",
		}
		s := err.Error()
		if strings.ContainsAny(s, "\r\x1b\x00") {
			t.Errorf("rendered error contains a literal control byte: %q", s)
		}
	})

	t.Run("migration is bounded for very long names", func(t *testing.T) {
		// Use a very long migration value: 2000 'a's plus a trailing marker.
		// safeRenderName caps inputs at 256 chars and appends a horizontal
		// ellipsis. The trailing marker must be dropped from the output.
		long := strings.Repeat("a", 2000) + "TRAILING"
		err := &filemigrate.Error{
			Code:      filemigrate.CodeDuplicateMigration,
			Op:        "op",
			Migration: long,
		}
		s := err.Error()
		if strings.Contains(s, "TRAILING") {
			t.Errorf("expected bounded migration rendering, but trailing marker leaked: %q", s)
		}
		// The cap is well under the input length, so the rendered string
		// must be far shorter than the raw 2000+ chars input.
		if len(s) > 1024 {
			t.Errorf("rendered error length %d exceeds reasonable cap", len(s))
		}
	})

	// Multibyte UTF-8 truncation must walk back to a rune boundary, not split
	// a codepoint mid-sequence. The CJK character below is 3 bytes in UTF-8;
	// when it is positioned to straddle the byte cap, the truncated string
	// must remain valid UTF-8.
	t.Run("migration with multibyte runes truncates at rune boundary", func(t *testing.T) {
		// 3-byte CJK character; 90 of them = 270 bytes (exceeds the 256-byte cap).
		long := strings.Repeat("中", 90)
		err := &filemigrate.Error{
			Code:      filemigrate.CodeDuplicateMigration,
			Op:        "op",
			Migration: long,
		}
		s := err.Error()
		// %q emits invalid UTF-8 bytes as \xNN escapes. If truncation split
		// a codepoint, the rendered output would contain \x escape sequences
		// rather than the literal CJK character.
		if strings.Contains(s, `\x`) {
			t.Errorf("rendered output contains \\x escape — truncation split a UTF-8 codepoint: %q", s)
		}
	})
}

func TestAllSentinelsHaveMatchingCode(t *testing.T) {
	// Verify a representative subset of sentinels produce errors.Is matches
	// against the typed *Error with the same code.
	cases := []struct {
		code     filemigrate.ErrorCode
		sentinel error
	}{
		{filemigrate.CodeUnsupportedArtifactFormat, filemigrate.ErrUnsupportedArtifactFormat},
		{filemigrate.CodeMalformedSnapshot, filemigrate.ErrMalformedSnapshot},
		{filemigrate.CodeInvalidMigrationName, filemigrate.ErrInvalidMigrationName},
		{filemigrate.CodeResourceLimit, filemigrate.ErrResourceLimit},
		{filemigrate.CodeConfigCollision, filemigrate.ErrConfigCollision},
		{filemigrate.CodeUnsupportedFeature, filemigrate.ErrUnsupportedFeature},
		{filemigrate.CodeUnsupportedObjectFamily, filemigrate.ErrUnsupportedObjectFamily},
	}
	for _, tc := range cases {
		err := &filemigrate.Error{Code: tc.code}
		if !errors.Is(err, tc.sentinel) {
			t.Errorf("code %q: errors.Is mismatch", tc.code)
		}
	}
}

func TestDiagnosticFields(t *testing.T) {
	d := filemigrate.Diagnostic{
		Code:     filemigrate.DiagnosticBroadIntrospection,
		Severity: filemigrate.DiagnosticWarning,
		Message:  "found 42 objects in 3 schemas",
		Path:     "",
	}
	if d.Code != filemigrate.DiagnosticBroadIntrospection {
		t.Errorf("unexpected code: %q", d.Code)
	}
	if d.Severity != filemigrate.DiagnosticWarning {
		t.Errorf("unexpected severity: %q", d.Severity)
	}
}
