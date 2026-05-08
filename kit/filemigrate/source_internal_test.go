package filemigrate

import (
	"strings"
	"testing"
)

// TestAssertNoSymlinkInChain_EmptyComponentFailsClosed locks in the
// fail-closed guard for empty path components. filepath.Clean normalises
// most real inputs and removes empty segments, so the guard is rarely
// exercised through the public API — but if a path like "/" slips through
// (Clean preserves the leading separator), Split produces empty
// components. The function must reject rather than silently iterate.
func TestAssertNoSymlinkInChain_EmptyComponentFailsClosed(t *testing.T) {
	// "/" cleans to "/", and strings.Split("/", "/") yields ["", ""], so
	// the first iteration of the loop hits the empty-component guard.
	err := assertNoSymlinkInChain("/root", "/")
	if err == nil {
		t.Fatal("expected error for empty path component, got nil")
	}
	if !strings.Contains(err.Error(), "empty path component") {
		t.Errorf("expected 'empty path component' error, got %v", err)
	}
}
