package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

// seedArtifactFixtures writes one fixture per artifact domain under a
// fake Claude Code home: skills, commands, agents, rules, mcp.json, and
// CLAUDE.md. Used to prove Domains scoping actually skips domains rather
// than just reporting them empty.
func seedArtifactFixtures(t *testing.T, ccRoot string) {
	t.Helper()
	write := func(rel, body string) {
		full := filepath.Join(ccRoot, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("skills/foo.md", "---\nname: foo\ndescription: the foo skill\n---\ndo foo")
	write("commands/bar.md", "---\nname: bar\n---\ndo bar")
	write("agents/baz.md", "---\nname: baz\n---\ndo baz")
	write("rules/qux.md", "---\nname: qux\n---\nuse qux")
	write("mcp.json", `{"mcpServers":{"srv":{"command":"echo"}}}`)
	write("CLAUDE.md", "# system prompt\nbe good")
}

// artifactsEnv points CLAUDE_CODE_HOME / OPENCODE_CONFIG_HOME at fresh
// temp dirs so tests never touch the real machine's CC/OC state.
func artifactsEnv(t *testing.T) (ccRoot, ocRoot string) {
	t.Helper()
	ccRoot = t.TempDir()
	ocRoot = t.TempDir()
	t.Setenv("CLAUDE_CODE_HOME", ccRoot)
	t.Setenv("OPENCODE_CONFIG_HOME", ocRoot)
	return ccRoot, ocRoot
}

// TestArtifactsMigrator_Migrate_DomainsUnset_MigratesAll pins today's
// behavior: an unscoped ArtifactsMigrator still migrates all six domains.
func TestArtifactsMigrator_Migrate_DomainsUnset_MigratesAll(t *testing.T) {
	ccRoot, ocRoot := artifactsEnv(t)
	seedArtifactFixtures(t, ccRoot)

	m := &ArtifactsMigrator{CWD: t.TempDir()}
	rep, err := m.Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if len(rep.SkillsWritten) == 0 {
		t.Error("SkillsWritten empty, want skills migrated")
	}
	if len(rep.CommandsWritten) == 0 {
		t.Error("CommandsWritten empty, want commands migrated")
	}
	if len(rep.AgentsWritten) == 0 {
		t.Error("AgentsWritten empty, want agents migrated")
	}
	if len(rep.RulesWritten) == 0 {
		t.Error("RulesWritten empty, want rules migrated")
	}
	if len(rep.MCPMerged) == 0 {
		t.Error("MCPMerged empty, want mcp migrated")
	}
	if rep.SystemPromptWritten == "" {
		t.Error("SystemPromptWritten empty, want system prompt migrated")
	}

	for _, rel := range []string{"skills", "command", "agent", "rules", "opencode.json", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(ocRoot, rel)); err != nil {
			t.Errorf("%s: want written, stat error: %v", rel, err)
		}
	}
}

// TestArtifactsMigrator_Migrate_DomainsScoped_SkillsOnly is the regression
// test for the bug this task fixes: scoping Domains to skills must leave
// every other domain untouched, both in the report and on disk.
func TestArtifactsMigrator_Migrate_DomainsScoped_SkillsOnly(t *testing.T) {
	ccRoot, ocRoot := artifactsEnv(t)
	seedArtifactFixtures(t, ccRoot)

	m := &ArtifactsMigrator{CWD: t.TempDir(), Domains: []Domain{DomainSkills}}
	rep, err := m.Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if len(rep.SkillsWritten) == 0 {
		t.Fatal("SkillsWritten empty, want skills migrated")
	}
	if len(rep.CommandsWritten) != 0 {
		t.Errorf("CommandsWritten = %v, want none migrated", rep.CommandsWritten)
	}
	if len(rep.AgentsWritten) != 0 {
		t.Errorf("AgentsWritten = %v, want none migrated", rep.AgentsWritten)
	}
	if len(rep.RulesWritten) != 0 {
		t.Errorf("RulesWritten = %v, want none migrated", rep.RulesWritten)
	}
	if len(rep.MCPMerged) != 0 {
		t.Errorf("MCPMerged = %v, want none migrated", rep.MCPMerged)
	}
	if rep.SystemPromptWritten != "" {
		t.Errorf("SystemPromptWritten = %q, want empty", rep.SystemPromptWritten)
	}

	if _, err := os.Stat(filepath.Join(ocRoot, "skills")); err != nil {
		t.Errorf("skills dir: want written, stat error: %v", err)
	}
	for _, rel := range []string{"command", "agent", "rules", "opencode.json", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(ocRoot, rel)); !os.IsNotExist(err) {
			t.Errorf("%s: want untouched, stat error: %v", rel, err)
		}
	}
}

// TestArtifactsMigrator_Migrate_DryRun confirms DryRun still short-circuits
// before any domain runs, regardless of Domains.
func TestArtifactsMigrator_Migrate_DryRun(t *testing.T) {
	ccRoot, _ := artifactsEnv(t)
	seedArtifactFixtures(t, ccRoot)

	m := &ArtifactsMigrator{DryRun: true, Domains: []Domain{DomainSkills}}
	rep, err := m.Migrate()
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !rep.DryRun {
		t.Error("rep.DryRun = false, want true")
	}
	if len(rep.SkillsWritten) != 0 {
		t.Errorf("SkillsWritten = %v, want none written on dry-run", rep.SkillsWritten)
	}
}

// TestArtifactsMigrator_Migrate_UnknownDomain_Errors covers the lookup
// failure path in the dispatch table.
func TestArtifactsMigrator_Migrate_UnknownDomain_Errors(t *testing.T) {
	artifactsEnv(t)

	m := &ArtifactsMigrator{Domains: []Domain{Domain("bogus")}}
	if _, err := m.Migrate(); err == nil {
		t.Fatal("expected error for unknown domain")
	}
}
