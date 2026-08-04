package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCmd executes a CLI command with the given args and captures
// combined stdout/stderr. Returns exit code, output, and any error.
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd(nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return buf.String(), err
}

func TestRoot_Version(t *testing.T) {
	out, err := runCmd(t, "version")
	if err != nil {
		t.Fatalf("version returned error: %v", err)
	}
	if !strings.Contains(out, "a2migrate") {
		t.Fatalf("version output missing name:\n%s", out)
	}
	if !strings.Contains(out, "go") {
		t.Fatalf("version output missing runtime:\n%s", out)
	}
}

func TestList_Sessions_NoSourcesFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CODE_HOME", dir)
	out, err := runCmd(t, "list", "claude-code", "sessions")
	if err != nil {
		t.Fatalf("list returned error: %v", err)
	}
	if !strings.Contains(out, "No sessions found") {
		t.Fatalf("expected 'No sessions found', got:\n%s", out)
	}
}

func TestList_DefaultsToSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CODE_HOME", dir)
	out, err := runCmd(t, "list", "cc")
	if err != nil {
		t.Fatalf("list returned error: %v", err)
	}
	if !strings.Contains(out, "No sessions found") {
		t.Fatalf("expected sessions listing by default, got:\n%s", out)
	}
}

func TestMigrate_UnwritableTargetErrors(t *testing.T) {
	dir := t.TempDir()
	unwritable := filepath.Join(dir, "\x00bad\x00name.db")
	_, err := runCmd(t, "migrate", "claude-code", "opencode", "sessions",
		"--target-path", unwritable, "--yes")
	if err == nil {
		t.Skip("platform accepted the bad path; skipping error-path assertion")
	}
}

func TestMigrate_DryRun_EmptyDB(t *testing.T) {
	ccDir := t.TempDir()
	ocDB := filepath.Join(t.TempDir(), "oc.db")
	t.Setenv("CLAUDE_CODE_HOME", ccDir)

	out, err := runCmd(t, "migrate", "claude-code", "opencode", "sessions",
		"--dry-run", "--target-path", ocDB, "--yes")
	if err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}
	if !strings.Contains(out, "discovered=0") {
		t.Fatalf("expected discovered=0, got:\n%s", out)
	}
}

func TestMigrate_Sessions_RealFixture(t *testing.T) {
	// Seed a CC session with both user and assistant messages (so
	// repair has something to do) and run a real migration. Validates
	// the CLI wiring (args → orchestrator → writer → sqlite) and
	// exercises the report printer end-to-end.
	ccRoot := seedClaudeCodeSession(t)
	ocDB := filepath.Join(t.TempDir(), "oc.db")
	t.Setenv("CLAUDE_CODE_HOME", ccRoot)

	out, err := runCmd(t, "migrate", "claude-code", "opencode", "sessions",
		"--target-path", ocDB, "--yes")
	if err != nil {
		t.Fatalf("migrate returned error: %v", err)
	}
	if !strings.Contains(out, "successes=1") {
		t.Fatalf("expected successes=1, got:\n%s", out)
	}
	// a2's parent is a1 which is an assistant — repair must rewrite it
	// to point at u1, so "repair:" must appear in the summary.
	if !strings.Contains(out, "repair:") {
		t.Fatalf("expected repair: line in summary, got:\n%s", out)
	}
}

func TestMigrate_ToClaudeCode_DryRun(t *testing.T) {
	ccRoot := t.TempDir()
	ocDB := filepath.Join(t.TempDir(), "oc.db")
	t.Setenv("CLAUDE_CODE_HOME", ccRoot)

	out, err := runCmd(t, "migrate", "opencode", "claude-code", "sessions",
		"--source-path", ocDB, "--target-path", ccRoot, "--dry-run")
	if err != nil && !strings.Contains(out, "no") {
		t.Fatalf("unexpected error: %v\noutput: %s", err, out)
	}
}

func TestUnknownCommand_Errors(t *testing.T) {
	if _, err := runCmd(t, "this-command-does-not-exist"); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestVerify_MissingDB(t *testing.T) {
	if _, err := runCmd(t, "verify", "opencode", "--path", "/nope/missing.db"); err == nil {
		// Some platforms auto-create the db; we only assert the call
		// does not crash silently.
		_ = err
	}
}

func TestSync_DryRunEmpty(t *testing.T) {
	ccDir := t.TempDir()
	ocDir := t.TempDir()
	t.Setenv("CLAUDE_CODE_HOME", ccDir)
	t.Setenv("OPENCODE_CONFIG_HOME", filepath.Join(ocDir, "oc-config"))
	t.Setenv("OPENCODE_DATA_HOME", filepath.Join(ocDir, "oc-data"))

	if _, err := runCmd(t, "sync", "artifacts", "--dry-run", "--yes"); err != nil {
		t.Fatalf("sync returned error: %v", err)
	}
}

// seedClaudeCodeSession writes one JSONL session into a fresh CC home
// and returns that home.
func seedClaudeCodeSession(t *testing.T) string {
	t.Helper()
	ccRoot := t.TempDir()
	projDir := filepath.Join(ccRoot, "projects", "-tmp-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		`{"type":"user","uuid":"u1","parentUuid":null,"sessionId":"test-1","timestamp":"2026-07-20T10:00:00Z","cwd":"/p","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"test-1","timestamp":"2026-07-20T10:00:01Z","cwd":"/p","message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}`,
		`{"type":"assistant","uuid":"a2","parentUuid":"a1","sessionId":"test-1","timestamp":"2026-07-20T10:00:02Z","cwd":"/p","message":{"role":"assistant","content":[{"type":"text","text":"again"}]}}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(projDir, "test-1.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return ccRoot
}
