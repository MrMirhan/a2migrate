package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirhan/a2migrate/internal/domain"
)

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"Foo Bar": "foo-bar",
		"über":   "ber",
		"$$$":    "untitled",
		"":       "untitled",
		"a/b/c":  "a-b-c",
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q want %q", in, got, want)
		}
	}
}

func TestRenderFrontmatter(t *testing.T) {
	fm := map[string]any{
		"name":        "foo",
		"description": "bar",
		"paths":       []string{"**/*.go"},
	}
	got := renderFrontmatter(fm)
	if !strings.HasPrefix(got, "---\n") {
		t.Fatalf("missing leading delim: %q", got)
	}
	if !strings.Contains(got, "name: \"foo\"") {
		t.Fatalf("name field wrong: %q", got)
	}
	if !strings.Contains(got, "paths: [\"**/*.go\"]") {
		t.Fatalf("paths field wrong: %q", got)
	}
	if !strings.HasSuffix(got, "---\n") {
		t.Fatalf("missing trailing delim: %q", got)
	}
}

func TestStripJSONC(t *testing.T) {
	src := `{"a": 1, /* comment */ "b": 2}
// line comment
"c": 3`
	got := stripJSONC(src)
	if strings.Contains(got, "comment") {
		t.Fatalf("comments not stripped: %q", got)
	}
}

func TestParseOCConfig_Missing(t *testing.T) {
	got, err := parseOCConfig([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("non-empty: %v", got)
	}
}

func TestParseOCConfig_Real(t *testing.T) {
	src := `{
  // comment
  "mcp": {
    "foo": {"type": "local", "command": ["x", "y"]}
  }
}`
	got, err := parseOCConfig([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	mcp, ok := got["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("mcp not map: %T", got["mcp"])
	}
	if _, ok := mcp["foo"]; !ok {
		t.Fatal("foo missing")
	}
}

func TestSkillWriter_WriteGlobal(t *testing.T) {
	dir := t.TempDir()
	w := &SkillWriter{Home: dir}
	skills := []domain.Skill{
		{Name: "Foo", Body: "do foo", Frontmatter: map[string]any{"description": "the foo skill"}},
	}
	written, err := w.WriteGlobal(skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("written = %d want 1", len(written))
	}
	body, err := os.ReadFile(written[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "description: \"the foo skill\"") {
		t.Fatalf("frontmatter missing: %s", body)
	}
	if !strings.Contains(string(body), "do foo") {
		t.Fatalf("body missing: %s", body)
	}
}

func TestCommandWriter_WriteProject(t *testing.T) {
	dir := t.TempDir()
	w := &CommandWriter{Home: dir, WorkDir: dir}
	cmds := []domain.Command{
		{Name: "review", Body: "review pr", ArgumentHint: "<id>", AllowedTools: []string{"bash"}},
	}
	written, err := w.WriteProject(cmds)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("written = %d want 1", len(written))
	}
	body, _ := os.ReadFile(written[0])
	s := string(body)
	if !strings.Contains(s, "argument-hint: \"<id>\"") {
		t.Fatalf("argument-hint missing: %s", s)
	}
	if !strings.Contains(s, "allowed-tools: [\"bash\"]") {
		t.Fatalf("allowed-tools missing: %s", s)
	}
}

func TestAgentWriter_WriteGlobal(t *testing.T) {
	dir := t.TempDir()
	w := &AgentWriter{Home: dir}
	agents := []domain.AgentDef{
		{Name: "reviewer", Body: "review things", Model: "anthropic/claude-opus", Tools: []string{"read", "bash"}},
	}
	written, err := w.WriteGlobal(agents)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("written = %d want 1", len(written))
	}
	body, _ := os.ReadFile(written[0])
	s := string(body)
	if !strings.Contains(s, "model: \"anthropic/claude-opus\"") {
		t.Fatalf("model missing: %s", s)
	}
}

func TestRuleWriter_WriteProject(t *testing.T) {
	dir := t.TempDir()
	w := &RuleWriter{Home: dir, WorkDir: dir}
	rules := []domain.Rule{
		{Name: "go-style", Paths: []string{"**/*.go"}, Body: "use gofmt"},
	}
	written, err := w.WriteProject(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("written = %d want 1", len(written))
	}
	body, _ := os.ReadFile(written[0])
	if !strings.Contains(string(body), "paths: [\"**/*.go\"]") {
		t.Fatalf("paths missing: %s", body)
	}
}

func TestMCPConfigWriter_Apply(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	w := &MCPConfigWriter{Path: path}

	patch := MCPConfigPatch{
		Servers: []domain.MCPServer{
			{Name: "linear", Type: "local", Enabled: true, Command: []string{"linear-mcp", "--token", "x"}},
		},
	}
	if _, err := w.Apply(patch); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatal(err)
	}
	mcp, ok := root["mcp"].(map[string]any)
	if !ok {
		t.Fatal("mcp not a map")
	}
	linear, ok := mcp["linear"].(map[string]any)
	if !ok {
		t.Fatal("linear missing")
	}
	cmd, _ := linear["command"].([]any)
	if len(cmd) != 3 {
		t.Fatalf("command = %v want 3 elements", cmd)
	}
	if linear["type"] != "local" {
		t.Fatalf("type = %v", linear["type"])
	}

	// Apply again to test merge idempotency.
	patch2 := MCPConfigPatch{Servers: []domain.MCPServer{{Name: "playwright", Type: "local", Enabled: true, Command: []string{"pwright"}}}}
	if _, err := w.Apply(patch2); err != nil {
		t.Fatal(err)
	}
	body2, _ := os.ReadFile(path)
	var root2 map[string]any
	if err := json.Unmarshal(body2, &root2); err != nil {
		t.Fatal(err)
	}
	mcp2 := root2["mcp"].(map[string]any)
	if _, ok := mcp2["linear"]; !ok {
		t.Fatal("linear lost on re-apply")
	}
	if _, ok := mcp2["playwright"]; !ok {
		t.Fatal("playwright missing")
	}
}

func TestMCPConfigWriter_Apply_CreatesMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "opencode.json")
	w := &MCPConfigWriter{Path: path}
	patch := MCPConfigPatch{Servers: []domain.MCPServer{{Name: "x", Type: "local", Enabled: true, Command: []string{"x"}}}}
	if _, err := w.Apply(patch); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestServerToOC_Remote(t *testing.T) {
	s := domain.MCPServer{
		Name:    "remote",
		Type:    "remote",
		URL:     "https://example.com",
		Headers: map[string]string{"Authorization": "Bearer x"},
		Enabled: true,
	}
	out := serverToOC(s)
	if out["url"] != "https://example.com" {
		t.Fatalf("url = %v", out["url"])
	}
	if out["type"] != "remote" {
		t.Fatalf("type = %v", out["type"])
	}
}