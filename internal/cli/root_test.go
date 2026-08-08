package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestRoot_CommandSet pins the top-level surface. It is an exact set
// match, not a substring check: the whole point of routing tools through
// arguments is that this list stays constant as tools are added.
func TestRoot_CommandSet(t *testing.T) {
	want := []string{
		"export", "list", "migrate", "repair", "select", "show", "sync", "tools", "verify", "version",
	}
	root := NewRootCmd(nil)
	var got []string
	for _, c := range root.Commands() {
		if c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		got = append(got, c.Name())
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("top-level commands:\n got %v\nwant %v", got, want)
	}
}

func TestResolveTool_AliasesAndSpellings(t *testing.T) {
	for _, arg := range []string{"cc", "claude-code", "claude_code", "CLAUDE-CODE"} {
		got, err := resolveTool(arg)
		if err != nil {
			t.Fatalf("resolveTool(%q): %v", arg, err)
		}
		if got.ID != toolClaudeCode {
			t.Errorf("resolveTool(%q) = %q", arg, got.ID)
		}
	}
	if _, err := resolveTool("nope"); err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestMigrate_RejectsSameToolBothSides(t *testing.T) {
	_, err := runCmd(t, "migrate", "claude-code", "cc")
	if err == nil {
		t.Fatal("expected error when source and target are the same tool")
	}
	if !strings.Contains(err.Error(), "both") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigrate_RejectsUnknownDomain(t *testing.T) {
	_, err := runCmd(t, "migrate", "cc", "oc", "bogus")
	if err == nil || !strings.Contains(err.Error(), "unknown domain") {
		t.Fatalf("expected unknown-domain error, got: %v", err)
	}
}

// TestMigrate_ScopesToRequestedDomain is the regression test for the bug
// this refactor fixes: naming one domain must not migrate the others.
func TestMigrate_ScopesToRequestedDomain(t *testing.T) {
	_, ocConfig := seedArtifacts(t)

	out, err := runCmd(t, "migrate", "claude-code", "opencode", "skills", "--yes")
	if err != nil {
		t.Fatalf("migrate skills: %v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(ocConfig, "skills", "demo.md")); err != nil {
		t.Fatalf("skills domain did not migrate: %v", err)
	}
	for _, unwanted := range []string{"command", "agent", "rules", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(ocConfig, unwanted)); !os.IsNotExist(err) {
			t.Errorf("%s was migrated but only skills were requested", unwanted)
		}
	}
}

// TestMigrate_NoDomainArgsMigratesAll covers the replacement for the
// deleted `all` command: omitting the domain arguments migrates every
// domain both tools share.
func TestMigrate_NoDomainArgsMigratesAll(t *testing.T) {
	_, ocConfig := seedArtifacts(t)

	out, err := runCmd(t, "migrate", "cc", "oc", "--yes")
	if err != nil {
		t.Fatalf("migrate all: %v\n%s", err, out)
	}
	for _, want := range []string{
		filepath.Join("skills", "demo.md"),
		filepath.Join("command", "demo.md"),
		"AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(ocConfig, want)); err != nil {
			t.Errorf("expected %s to be migrated: %v", want, err)
		}
	}
	if !strings.Contains(out, "Claude Code -> OpenCode") {
		t.Errorf("expected the resolved direction in the output, got:\n%s", out)
	}
}

// TestMigrate_ReverseDirection covers the replacement for the deleted
// `reverse` command: swapping the arguments migrates the other way.
func TestMigrate_ReverseDirection(t *testing.T) {
	ccHome := t.TempDir()
	ocConfig := t.TempDir()
	t.Setenv("CLAUDE_CODE_HOME", ccHome)
	t.Setenv("OPENCODE_CONFIG_HOME", ocConfig)
	t.Setenv("OPENCODE_DATA_HOME", t.TempDir())
	writeFile(t, filepath.Join(ocConfig, "AGENTS.md"), "# from opencode\n")

	out, err := runCmd(t, "migrate", "opencode", "claude-code", "system", "--yes")
	if err != nil {
		t.Fatalf("reverse migrate: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(ccHome, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md not written: %v", err)
	}
	if !strings.Contains(string(got), "from opencode") {
		t.Fatalf("unexpected CLAUDE.md contents: %q", got)
	}
	if !strings.Contains(out, "OpenCode -> Claude Code") {
		t.Errorf("expected the resolved direction in the output, got:\n%s", out)
	}
}

func TestList_ArtifactDomain(t *testing.T) {
	seedArtifacts(t)
	out, err := runCmd(t, "list", "claude-code", "skills")
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	if !strings.Contains(out, "demo") {
		t.Fatalf("expected the seeded skill in the listing, got:\n%s", out)
	}
}

func TestVerify_RejectsToolWithoutProvenance(t *testing.T) {
	_, err := runCmd(t, "verify", "claude-code")
	if err == nil || !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("expected a provenance error, got: %v", err)
	}
}

func TestRepair_RejectsToolWithoutInvariants(t *testing.T) {
	_, err := runCmd(t, "repair", "claude-code")
	if err == nil || !strings.Contains(err.Error(), "repairable") {
		t.Fatalf("expected a repairable-invariants error, got: %v", err)
	}
}

// seedArtifacts writes one skill, one command, and a CLAUDE.md into a
// fresh Claude Code home, pointing OpenCode at empty dirs. Returns both
// roots.
func seedArtifacts(t *testing.T) (ccHome, ocConfig string) {
	t.Helper()
	ccHome = t.TempDir()
	ocConfig = t.TempDir()
	t.Setenv("CLAUDE_CODE_HOME", ccHome)
	t.Setenv("OPENCODE_CONFIG_HOME", ocConfig)
	t.Setenv("OPENCODE_DATA_HOME", t.TempDir())

	writeFile(t, filepath.Join(ccHome, "skills", "demo.md"),
		"---\nname: demo\ndescription: demo skill\n---\n\nbody\n")
	writeFile(t, filepath.Join(ccHome, "commands", "demo.md"),
		"---\ndescription: demo command\n---\n\nrun it\n")
	writeFile(t, filepath.Join(ccHome, "CLAUDE.md"), "# instructions\n")
	return ccHome, ocConfig
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRoot_LogLevelFromEnv covers the A2MIGRATE_LOG_LEVEL override the
// README documents; the flag must still win when both are given.
func TestRoot_LogLevelFromEnv(t *testing.T) {
	t.Setenv("A2MIGRATE_LOG_LEVEL", "warn")
	if got := NewRootCmd(nil).PersistentFlags().Lookup("log-level").DefValue; got != "warn" {
		t.Fatalf("log-level default = %q, want the env value", got)
	}

	t.Setenv("A2MIGRATE_LOG_LEVEL", "")
	if got := NewRootCmd(nil).PersistentFlags().Lookup("log-level").DefValue; got != "info" {
		t.Fatalf("log-level default = %q, want info when the env is unset", got)
	}
}
