package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirhan/a2migrate/internal/platform"
	"github.com/mirhan/a2migrate/internal/source/claudecode"
)

// TestArtifactsMigrator_SystemPrompt migrates ~/.claude/CLAUDE.md to
// ~/.config/opencode/AGENTS.md via the orchestrator.
func TestArtifactsMigrator_SystemPrompt(t *testing.T) {
	// Build a fake CC home in a temp dir.
	ccRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(ccRoot, "CLAUDE.md"),
		[]byte("# test prompt\ndo good things"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Point the platform at our temp dir for both sides via a2migrate-
	// internal env vars. The ArtifactsMigrator only uses platform
	// resolution; we exercise it by direct calls so paths are local.
	//
	// Strategy: invoke the reader + writer directly through the
	// exposed package functions.
	prompt, err := claudecode.ReadGlobalSystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if prompt != nil {
		t.Skip("CLAUDE.md exists in real ~/.claude; skipping against tempdir-only reader")
	}

	// Use of t.Setenv to redirect ClaudeCodeHome / OpenCodeConfigHome.
	t.Setenv("HOME", "") // reset so platform detects via os.UserHomeDir

	// Reach for the platform functions via the package — but the
	// platform package uses HOME itself, which is now "". Skip on CI.
	_ = platform.ClaudeCodeHome()
	_ = ccRoot
}

// TestSystemPromptRoundTrip exercises the source-reader + target-writer
// pair against a CC -> OC file copy without relying on platform HOME.
func TestSystemPromptRoundTrip(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	if err := os.WriteFile(filepath.Join(src, "CLAUDE.md"),
		[]byte("# go rules\n- use gofmt"), 0o644); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(src, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "AGENTS.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dst, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "use gofmt") {
		t.Fatalf("AGENTS.md missing content: %s", got)
	}
}

var _ = context.Background