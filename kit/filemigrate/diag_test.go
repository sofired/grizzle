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
