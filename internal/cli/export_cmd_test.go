package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrMirhan/a2migrate/internal/domain"
	"github.com/MrMirhan/a2migrate/internal/export"
	"github.com/MrMirhan/a2migrate/internal/tools"
)

func TestExportFlags_HasSessionFilter(t *testing.T) {
	if (&exportFlags{}).hasSessionFilter() {
		t.Error("bare flags should not count as a filter")
	}
	for name, f := range map[string]*exportFlags{
		"search":  {search: "auth"},
		"include": {includes: []string{"id"}},
		"exclude": {excludes: []string{"id"}},
	} {
		if !f.hasSessionFilter() {
			t.Errorf("%s should count as a filter", name)
		}
	}
}

func TestSafeFileStem(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ab812a85-bf05-46bf", "ab812a85-bf05-46bf"},
		{"agent_x1", "agent_x1"},
		{"../../etc/passwd", "etc-passwd"},
		{"a b/c", "a-b-c"},
		{"", "session"},
		{"///", "session"},
	}
	for _, tt := range tests {
		if got := safeFileStem(tt.in); got != tt.want {
			t.Errorf("safeFileStem(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSessionFileName_PrefersOriginID(t *testing.T) {
	s := domain.Session{ID: "ses_9", OriginID: "uuid-1", Title: "a title/with slash"}
	if got := sessionFileName(s, export.FormatMarkdown); got != "uuid-1.md" {
		t.Errorf("sessionFileName = %q, want uuid-1.md", got)
	}
	if got := sessionFileName(domain.Session{ID: "ses_9"}, export.FormatJSON); got != "ses_9.json" {
		t.Errorf("sessionFileName = %q, want ses_9.json", got)
	}
}

// TestWriteExportDir_SplitsSessionsAndArtifacts pins the on-disk layout:
// one file per session so a single transcript can be shared on its own,
// plus one file holding the artifacts.
func TestWriteExportDir_SplitsSessionsAndArtifacts(t *testing.T) {
	dir := t.TempDir()
	b := export.Bundle{
		Tool: "Claude Code",
		Sessions: []domain.Session{
			{OriginID: "uuid-1", Title: "first"},
			{OriginID: "uuid-2", Title: "second"},
		},
		Skills: []domain.Skill{{Name: "deploy", Body: "run make."}},
	}

	cmd := NewRootCmd(nil)
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := writeExportDir(cmd, b, export.FormatMarkdown, dir); err != nil {
		t.Fatalf("writeExportDir: %v", err)
	}

	for _, name := range []string{"uuid-1.md", "uuid-2.md", "artifacts.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s on disk: %v", name, err)
		}
	}

	first, err := os.ReadFile(filepath.Join(dir, "uuid-1.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "first") {
		t.Error("session file is missing its own title")
	}
	if strings.Contains(string(first), "second") {
		t.Error("session file leaked another session")
	}

	artifacts, err := os.ReadFile(filepath.Join(dir, "artifacts.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(artifacts), "uuid-1") {
		t.Error("artifacts file leaked a session")
	}
	if !strings.Contains(string(artifacts), "deploy") {
		t.Error("artifacts file is missing the skill")
	}
}

// TestWriteExportDir_SkipsArtifactFileWhenNoneRequested keeps an
// empty-but-present artifacts file out of a sessions-only export.
func TestWriteExportDir_SkipsArtifactFileWhenNoneRequested(t *testing.T) {
	dir := t.TempDir()
	b := export.Bundle{Tool: "Claude Code", Sessions: []domain.Session{{OriginID: "uuid-1"}}}

	cmd := NewRootCmd(nil)
	cmd.SetOut(&bytes.Buffer{})
	if err := writeExportDir(cmd, b, export.FormatJSON, dir); err != nil {
		t.Fatalf("writeExportDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "artifacts.json")); !os.IsNotExist(err) {
		t.Error("artifacts file should not exist when no artifact domain was exported")
	}

	raw, err := os.ReadFile(filepath.Join(dir, "uuid-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got export.Bundle
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("per-session file is not valid JSON: %v", err)
	}
	if len(got.Sessions) != 1 {
		t.Errorf("sessions in file = %d, want 1", len(got.Sessions))
	}
}

func TestReadersFor(t *testing.T) {
	for _, id := range []tools.ID{toolClaudeCode, toolOpenCode} {
		r, ok := readersFor(id)
		if !ok {
			t.Fatalf("readersFor(%s) reported no readers", id)
		}
		if r.skills == nil || r.commands == nil || r.agents == nil ||
			r.rules == nil || r.mcp == nil || r.system == nil {
			t.Errorf("readersFor(%s) left a domain unwired", id)
		}
	}
	if _, ok := readersFor("codex"); ok {
		t.Error("codex has no artifact readers yet")
	}
}
