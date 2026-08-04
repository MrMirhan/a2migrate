package claudecode

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirhan/a2migrate/internal/domain"
)

func sampleDomainSession() domain.Session {
	return domain.Session{
		OriginID:   "orig-1",
		Origin:     domain.OriginOpenCode,
		Title:      "Hello",
		Slug:       "hello",
		ProjectDir: "/tmp/proj",
		CreatedAt:  ms("2026-07-20T10:00:00Z"),
		UpdatedAt:  ms("2026-07-20T10:00:05Z"),
		Messages: []domain.Message{
			{
				OriginID:  "u1",
				Role:      domain.RoleUser,
				CreatedAt: ms("2026-07-20T10:00:00Z"),
				Parts:     []domain.Part{{Type: domain.PartText, Text: "hi"}},
			},
			{
				OriginID:  "a1",
				Role:      domain.RoleAssistant,
				CreatedAt: ms("2026-07-20T10:00:01Z"),
				Parts: []domain.Part{
					{Type: domain.PartReasoning, Text: "think"},
					{Type: domain.PartTool, ToolName: "Read", ToolCallID: "toolu_1", ToolInput: map[string]any{"x": 1}, ToolStatus: "completed", ToolOutput: "ok"},
					{Type: domain.PartText, Text: "done"},
				},
			},
		},
	}
}

func ms(s string) (t timeT) { return parseTime(s) }

func TestSessionWriter_WriteSession(t *testing.T) {
	ccRoot := t.TempDir()
	w := NewSessionWriter(ccRoot)
	sess := sampleDomainSession()
	out, err := w.WriteSession(sess, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out, ".jsonl") {
		t.Fatalf("unexpected path %s", out)
	}

	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines []map[string]any
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var m map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			t.Fatalf("line: %v\n%s", err, scanner.Text())
		}
		lines = append(lines, m)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	// Expect at least: ai-title + user + assistant = 3 lines.
	if len(lines) < 3 {
		t.Fatalf("lines = %d want >= 3", len(lines))
	}
	if lines[0]["type"] != "ai-title" {
		t.Fatalf("first type = %v", lines[0]["type"])
	}
	if lines[0]["title"] != "Hello" {
		t.Fatalf("title = %v", lines[0]["title"])
	}
	var assistant map[string]any
	for _, l := range lines {
		if l["type"] == "assistant" {
			assistant = l
			break
		}
	}
	if assistant == nil {
		t.Fatal("no assistant entry")
	}
	msg := assistant["message"].(map[string]any)
	content := msg["content"].([]any)
	if len(content) == 0 {
		t.Fatal("no content blocks")
	}
	// Expect: thinking, tool_use, tool_result, text
	if len(content) != 4 {
		t.Fatalf("content blocks = %d want 4", len(content))
	}
}

func TestSessionWriter_WriteSubagent(t *testing.T) {
	ccRoot := t.TempDir()
	w := NewSessionWriter(ccRoot)
	sess := sampleDomainSession()
	sess.IsSubagent = true
	sess.ParentID = "parent-uuid"
	sess.OriginID = "subagent-orig"

	out, err := w.WriteSession(sess, "parent-oc-id")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "/subagents/agent-") {
		t.Fatalf("path %s not in subagents dir", out)
	}
	body, _ := os.ReadFile(out)
	if !strings.Contains(string(body), `"bridge-session"`) {
		t.Fatalf("missing bridge-session entry: %s", body)
	}
}

func TestSessionWriter_MissingProjectDir(t *testing.T) {
	ccRoot := t.TempDir()
	w := NewSessionWriter(ccRoot)
	sess := sampleDomainSession()
	sess.ProjectDir = ""
	if _, err := w.WriteSession(sess, ""); err == nil {
		t.Fatal("expected error for missing project dir")
	}
}

func TestMCPConfigWriter_Apply(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	w := &MCPConfigWriter{Path: path}
	servers := []domain.MCPServer{
		{Name: "linear", Type: "local", Enabled: true, Command: []string{"linear-mcp", "--token", "x"}},
	}
	out, err := w.Apply(servers)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("empty path")
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), `"mcpServers"`) {
		t.Fatalf("missing mcpServers: %s", body)
	}
	if !strings.Contains(string(body), `"linear"`) {
		t.Fatalf("missing linear: %s", body)
	}
	// Re-apply — should still work (replaces same key).
	if _, err := w.Apply(servers); err != nil {
		t.Fatal(err)
	}
}

func TestMCPConfigWriter_Apply_Remote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	w := &MCPConfigWriter{Path: path}
	servers := []domain.MCPServer{
		{Name: "remote", Type: "remote", URL: "https://x", Headers: map[string]string{"Auth": "Bearer y"}, Enabled: true},
	}
	if _, err := w.Apply(servers); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), `"url"`) {
		t.Fatalf("missing url: %s", body)
	}
}

func TestMCPConfigWriter_PreservesExistingEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"existing":{"command":"x","args":[]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	w := &MCPConfigWriter{Path: path}
	if _, err := w.Apply([]domain.MCPServer{{Name: "new", Command: []string{"n"}}}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	s := string(body)
	if !strings.Contains(s, `"existing"`) {
		t.Fatalf("existing lost: %s", s)
	}
	if !strings.Contains(s, `"new"`) {
		t.Fatalf("new missing: %s", s)
	}
}

func TestSkillWriter_WriteGlobal(t *testing.T) {
	dir := t.TempDir()
	w := &SkillWriter{Home: dir}
	skills := []domain.Skill{
		{Name: "foo", Body: "do foo", Frontmatter: map[string]any{"description": "the foo skill"}},
	}
	written, err := w.WriteGlobal(skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("written = %d want 1", len(written))
	}
	body, _ := os.ReadFile(written[0])
	if !strings.Contains(string(body), "description:") {
		t.Fatalf("frontmatter missing: %s", body)
	}
}

func TestCommandWriter_WriteProject(t *testing.T) {
	dir := t.TempDir()
	w := &CommandWriter{Home: dir, WorkDir: dir}
	cmds := []domain.Command{
		{Name: "review", Body: "review pr"},
	}
	written, err := w.WriteProject(cmds)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("written = %d want 1", len(written))
	}
	if !strings.Contains(written[0], "/commands/") {
		t.Fatalf("path %s not in commands/", written[0])
	}
}

func TestAgentWriter_WriteGlobal(t *testing.T) {
	dir := t.TempDir()
	w := &AgentWriter{Home: dir}
	agents := []domain.AgentDef{{Name: "reviewer", Body: "review things"}}
	written, err := w.WriteGlobal(agents)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("written = %d want 1", len(written))
	}
}

func TestRuleWriter_WriteGlobal(t *testing.T) {
	dir := t.TempDir()
	w := &RuleWriter{Home: dir}
	rules := []domain.Rule{{Name: "go-style", Body: "use gofmt"}}
	written, err := w.WriteGlobal(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("written = %d want 1", len(written))
	}
}

func TestSanitizeFilenameCC(t *testing.T) {
	if sanitizeFilenameCC("Foo Bar") != "foo-bar" {
		t.Fatal("space not replaced")
	}
	if sanitizeFilenameCC("a/b") != "a-b" {
		t.Fatal("slash not replaced")
	}
	if sanitizeFilenameCC("") != "untitled" {
		t.Fatal("empty should become untitled")
	}
}