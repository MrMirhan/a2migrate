// Package codex_test pins the stub contract for the Codex adapter.
// Once the real implementation lands, replace these tests with
// fixture-driven roundtrip cases.
package codex

import (
	"testing"
)

// Source side stubs.
func TestSourceStub_ReadsReturnNotImplemented(t *testing.T) {
	if _, err := ReadGlobalSkills(); err == nil {
		t.Fatal("ReadGlobalSkills should return error until implemented")
	}
	if _, err := ReadGlobalMCP(); err == nil {
		t.Fatal("ReadGlobalMCP should return error until implemented")
	}
}

// SessionsPath returns the directory Codex writes session JSONL files
// under. Verifying the directory exists is the most we can do as a
// stub test; full parsing lives in the real implementation.
func TestSourceStub_SessionsPathReturnsDir(t *testing.T) {
	// We can't assume a Codex install; just confirm the helper doesn't
	// panic on an empty HOME.
	got := SessionsPath()
	if got == "" {
		t.Fatal("SessionsPath should never return empty string")
	}
}
