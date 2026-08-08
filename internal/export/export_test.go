package export

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

func sampleBundle() Bundle {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return Bundle{
		Tool: "Claude Code",
		Sessions: []domain.Session{{
			ID:         "ses_1",
			OriginID:   "uuid-1",
			Origin:     domain.OriginClaudeCode,
			Title:      "fix the parser",
			ProjectDir: "/w/proj",
			CreatedAt:  at,
			UpdatedAt:  at,
			Messages: []domain.Message{
				{
					Role:  domain.RoleUser,
					Parts: []domain.Part{{Type: domain.PartText, Text: "why is it slow?"}},
				},
				{
					Role:    domain.RoleAssistant,
					ModelID: "claude-opus-5",
					Tokens:  domain.Tokens{Input: 10, Output: 20},
					Parts: []domain.Part{
						{Type: domain.PartReasoning, Text: "checking the loop"},
						{Type: domain.PartStepStart},
						{Type: domain.PartTool, ToolName: "Bash",
							ToolInput: map[string]any{"command": "go test"}, ToolOutput: "ok"},
						{Type: domain.PartText, Text: "the loop is quadratic"},
					},
				},
			},
		}},
		Skills: []domain.Skill{{Name: "deploy", Description: "ship it", Body: "# How\n\nrun make."}},
		MCP:    []domain.MCPServer{{Name: "fs", Command: []string{"npx", "server"}}},
	}
}

func render(t *testing.T, f Format) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, sampleBundle(), f); err != nil {
		t.Fatalf("Write(%s): %v", f, err)
	}
	return buf.String()
}

func TestParseFormat(t *testing.T) {
	for _, in := range []string{"md", "markdown", "JSON", " html ", "text", "plain"} {
		if _, err := ParseFormat(in); err != nil {
			t.Errorf("ParseFormat(%q) errored: %v", in, err)
		}
	}
	if _, err := ParseFormat("pdf"); err == nil {
		t.Error("ParseFormat(\"pdf\") should fail")
	}
}

// TestWrite_AllFormatsCarryContent guards the shared promise of every
// renderer: whatever the encoding, the transcript is actually in there.
func TestWrite_AllFormatsCarryContent(t *testing.T) {
	for _, f := range []Format{FormatMarkdown, FormatText, FormatHTML, FormatJSON} {
		out := render(t, f)
		for _, want := range []string{"fix the parser", "why is it slow?", "the loop is quadratic", "Bash"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s output is missing %q", f, want)
			}
		}
	}
}

// TestWrite_SkipsStepMarkers pins that renderer-only scaffolding stays
// out of a human-facing document.
func TestWrite_SkipsStepMarkers(t *testing.T) {
	for _, f := range []Format{FormatMarkdown, FormatText, FormatHTML} {
		if strings.Contains(render(t, f), "step-start") {
			t.Errorf("%s output leaked a step marker", f)
		}
	}
}

func TestWriteMarkdown_RendersUsageAndTools(t *testing.T) {
	out := render(t, FormatMarkdown)
	for _, want := range []string{
		"# a2migrate export — Claude Code",
		"### fix the parser",
		"#### user",
		"in 10, out 20",
		"claude-opus-5",
		"<summary>reasoning</summary>",
		"## MCP servers",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown is missing %q", want)
		}
	}
}

// TestWriteMarkdown_DemotesArtifactHeadings stops an embedded body from
// impersonating an export section.
func TestWriteMarkdown_DemotesArtifactHeadings(t *testing.T) {
	out := render(t, FormatMarkdown)
	if strings.Contains(out, "\n# How\n") {
		t.Error("skill body heading was left at level 1")
	}
	if !strings.Contains(out, "\n#### How\n") {
		t.Error("skill body heading was not demoted to level 4")
	}
}

func TestWriteHTML_EscapesContent(t *testing.T) {
	b := sampleBundle()
	b.Sessions[0].Title = `<script>alert(1)</script>`
	b.Sessions[0].Messages[0].Parts[0].Text = `5 < 6 && "quoted"`

	var buf bytes.Buffer
	if err := Write(&buf, b, FormatHTML); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Error("session title was not escaped")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("expected the escaped title in the output")
	}
	if !strings.Contains(out, "5 &lt; 6 &amp;&amp;") {
		t.Error("message text was not escaped")
	}
}

func TestWriteJSON_RoundTrips(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sampleBundle(), FormatJSON); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got Bundle
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Title != "fix the parser" {
		t.Fatalf("session did not survive the round trip: %+v", got.Sessions)
	}
	if n := len(got.Sessions[0].Messages[1].Parts); n != 4 {
		t.Errorf("parts = %d, want 4 (JSON keeps step markers; only the prose renderers drop them)", n)
	}
	if got.Sessions[0].Messages[1].Tokens.Input != 10 {
		t.Error("token usage did not survive the round trip")
	}
}

func TestBundle_IsEmpty(t *testing.T) {
	if !(Bundle{Tool: "x"}).IsEmpty() {
		t.Error("a bundle with only a tool name should be empty")
	}
	if sampleBundle().IsEmpty() {
		t.Error("the sample bundle should not be empty")
	}
}

func TestDemoteHeadings(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"single", "# a", "#### a"},
		{"caps at six", "###### a", "###### a"},
		{"not a heading", "#tag", "#tag"},
		{"body text untouched", "plain line", "plain line"},
		{
			"fenced code is left alone",
			"# real\n\n```sh\n# a comment\n```\n\n## also real",
			"#### real\n\n```sh\n# a comment\n```\n\n##### also real",
		},
	}
	for _, tt := range tests {
		if got := demoteHeadings(tt.in, 3); got != tt.want {
			t.Errorf("%s: demoteHeadings(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

// TestFenceSafe covers output that would otherwise close its own code
// block and spill the rest of the transcript into the document body.
func TestFenceSafe(t *testing.T) {
	if got := fenceSafe("a\n```\nb"); strings.Contains(got, "```") {
		t.Errorf("fenceSafe left a fence in place: %q", got)
	}
	if got := fenceSafe("no fence"); got != "no fence" {
		t.Errorf("fenceSafe rewrote clean text: %q", got)
	}
}

func TestTokenLabel(t *testing.T) {
	if got := tokenLabel(domain.Tokens{}); got != "" {
		t.Errorf("an empty usage block should render nothing, got %q", got)
	}
	got := tokenLabel(domain.Tokens{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4})
	if !strings.Contains(got, "in 1") || !strings.Contains(got, "cache 3/4") {
		t.Errorf("tokenLabel = %q", got)
	}
}

func TestMessageLabel(t *testing.T) {
	plain := domain.Message{Role: domain.RoleAssistant}
	if got := messageLabel(plain); got != "assistant" {
		t.Errorf("messageLabel = %q, want assistant", got)
	}
	sub := domain.Message{Role: domain.RoleAssistant, Agent: "reviewer"}
	if got := messageLabel(sub); got != "assistant (reviewer)" {
		t.Errorf("messageLabel = %q, want assistant (reviewer)", got)
	}
}
