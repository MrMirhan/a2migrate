package claudecode

import (
	"path/filepath"
	"testing"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

func TestParseSession_Minimal(t *testing.T) {
	sess, err := NewSessionReader("").ParseSession(filepath.Join("testdata", "minimal.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sess.OriginID != "minimal" {
		t.Fatalf("origin id = %q want minimal", sess.OriginID)
	}
	// u1 (user text), a1 (assistant text), a2 (assistant reasoning+text) = 3.
	if len(sess.Messages) != 3 {
		t.Fatalf("messages = %d want 3", len(sess.Messages))
	}
	if sess.Messages[0].Role != domain.RoleUser {
		t.Fatalf("first role = %s want user", sess.Messages[0].Role)
	}
	if sess.Messages[1].Role != domain.RoleAssistant {
		t.Fatalf("second role = %s want assistant", sess.Messages[1].Role)
	}
	// Third message has a reasoning block followed by a text block.
	if len(sess.Messages[2].Parts) != 2 {
		t.Fatalf("third message parts = %d want 2 (reasoning+text)", len(sess.Messages[2].Parts))
	}
	if sess.Messages[2].Parts[0].Type != domain.PartReasoning {
		t.Fatalf("first part of msg[2] = %s want reasoning", sess.Messages[2].Parts[0].Type)
	}
}

func TestParseSession_WithSubagents(t *testing.T) {
	sess, err := NewSessionReader("").ParseSession(filepath.Join("testdata", "with-subagents.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// 5 entries: u1(user) a1(assistant) u2(user with text) a2(assistant) u3(pure tool_result, skipped).
	if len(sess.Messages) != 4 {
		t.Fatalf("messages = %d want 4", len(sess.Messages))
	}
	// First assistant has tool_use → tool part.
	asst := sess.Messages[1]
	if len(asst.Parts) < 2 {
		t.Fatalf("assistant parts = %d want >=2", len(asst.Parts))
	}
	found := false
	for _, p := range asst.Parts {
		if p.Type == domain.PartTool && p.ToolName == "ToolSearch" && p.ToolStatus == "completed" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected tool part with status=completed")
	}
	// Second assistant has error tool_use. It's at messages[3] after
	// u1, a1, u2 in our fixture.
	asst2 := sess.Messages[3]
	errFound := false
	for _, p := range asst2.Parts {
		if p.Type == domain.PartTool && p.ToolName == "Read" {
			if p.ToolStatus == "error" {
				errFound = true
			}
			if p.ToolOutput == "" {
				t.Fatal("expected tool output to be populated")
			}
		}
	}
	if !errFound {
		t.Fatal("expected at least one tool part with status=error")
	}
}

func TestParseSession_Malformed(t *testing.T) {
	sess, err := NewSessionReader("").ParseSession(filepath.Join("testdata", "malformed.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Fixture: u1 (valid), bad JSON, a1 (valid), a2 (invalid ts but parseable),
	// u3 (no ts). Bad JSON is dropped; the rest survive with CreatedAt=zero
	// for the records that lacked a timestamp.
	if len(sess.Messages) != 4 {
		t.Fatalf("messages = %d want 4", len(sess.Messages))
	}
	// a2 has an invalid timestamp; CreatedAt should be zero, not zero-by-accident.
	if !sess.Messages[2].CreatedAt.IsZero() {
		t.Fatal("a2 CreatedAt should be zero (invalid ts)")
	}
}

func TestParseSession_TitleFromAITitle(t *testing.T) {
	sess, err := NewSessionReader("").ParseSession(filepath.Join("testdata", "with-ai-title.jsonl"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sess.Title != "Go language overview" {
		t.Fatalf("title = %q want 'Go language overview'", sess.Title)
	}
	// The last-prompt entry is captured metadata only; not emitted as a message.
	for _, m := range sess.Messages {
		if m.Role == domain.RoleUser && m.OriginID == "last-prompt" {
			t.Fatal("last-prompt should not be a message")
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"2026-07-20T10:00:00Z", true},
		{"2026-07-20T10:00:00.123Z", true},
		{"2026-07-20T10:00:00+02:00", true},
		{"garbage", false},
		{"", false},
	}
	for _, c := range cases {
		got := parseTimestamp(c.in)
		if c.want && got.IsZero() {
			t.Errorf("parseTimestamp(%q) returned zero", c.in)
		}
		if !c.want && !got.IsZero() {
			t.Errorf("parseTimestamp(%q) = %v want zero", c.in, got)
		}
	}
}

func TestParseSession_UsageBlock(t *testing.T) {
	sess, err := NewSessionReader("").ParseSession(filepath.Join("testdata", "with-usage.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var asst *domain.Message
	for i := range sess.Messages {
		if sess.Messages[i].Role == domain.RoleAssistant {
			asst = &sess.Messages[i]
		}
	}
	if asst == nil {
		t.Fatal("no assistant message")
	}
	if asst.Tokens.Input != 120 {
		t.Errorf("input = %d want 120", asst.Tokens.Input)
	}
	if asst.Tokens.Output != 42 {
		t.Errorf("output = %d want 42", asst.Tokens.Output)
	}
	if asst.Tokens.CacheRead != 200000 {
		t.Errorf("cache_read = %d want 200000", asst.Tokens.CacheRead)
	}
	if asst.Tokens.CacheWrite != 5000 {
		t.Errorf("cache_write = %d want 5000", asst.Tokens.CacheWrite)
	}
	if asst.Tokens.ServiceTier != "standard" {
		t.Errorf("service_tier = %q", asst.Tokens.ServiceTier)
	}
	if asst.ModelID != "claude-opus-4-5" {
		t.Errorf("model = %q", asst.ModelID)
	}
}

func TestFrontmatter_ParseAndStrip(t *testing.T) {
	src := "---\nname: foo\ndescription: hi\n---\nbody line 1\nbody line 2\n"
	fm, err := ParseFrontmatter(src)
	if err != nil {
		t.Fatal(err)
	}
	if fm.StringField("name") != "foo" {
		t.Fatalf("name = %q want foo", fm.StringField("name"))
	}
	if fm.StringField("description") != "hi" {
		t.Fatalf("description = %q want hi", fm.StringField("description"))
	}
	if fm.Body != "body line 1\nbody line 2\n" {
		t.Fatalf("body = %q", fm.Body)
	}
	got := fm.StringSliceField("paths")
	if got != nil {
		t.Fatalf("paths = %v want nil", got)
	}
}

func TestFrontmatter_StringSlice(t *testing.T) {
	src := "---\npaths: [\"**/*.go\", \"**/*.md\"]\n---\nbody"
	fm, err := ParseFrontmatter(src)
	if err != nil {
		t.Fatal(err)
	}
	got := fm.StringSliceField("paths")
	if len(got) != 2 || got[0] != "**/*.go" {
		t.Fatalf("paths = %v", got)
	}
}

func TestFrontmatter_NoFrontmatter(t *testing.T) {
	fm, err := ParseFrontmatter("just body\n")
	if err != nil {
		t.Fatal(err)
	}
	if fm.Body != "just body\n" {
		t.Fatalf("body = %q", fm.Body)
	}
	if fm.StringField("name") != "" {
		t.Fatal("no frontmatter should yield empty fields")
	}
}

func TestFrontmatter_InvalidYAML(t *testing.T) {
	src := "---\n: bad\n---\nbody"
	if _, err := ParseFrontmatter(src); err == nil {
		t.Fatal("expected YAML parse error")
	}
}
