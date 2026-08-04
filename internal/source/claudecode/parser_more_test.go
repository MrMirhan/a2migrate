package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mirhan/a2migrate/internal/domain"
)

// TestParseSession_SidechainFlag checks that isSidechain=true records are
// still parsed (we don't drop them); we just emit them normally.
func TestParseSession_SidechainFlag(t *testing.T) {
	src := strings.Join([]string{
		`{"type":"user","uuid":"u1","parentUuid":null,"sessionId":"s","timestamp":"2026-07-20T10:00:00Z","cwd":"/p","message":{"role":"user","content":"hi"},"isSidechain":true}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"s","timestamp":"2026-07-20T10:00:01Z","cwd":"/p","message":{"role":"assistant","content":[{"type":"text","text":"bye"}]}}`,
		"",
	}, "\n")
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "s.jsonl", src)
	sess, err := NewSessionReader("").ParseSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("messages = %d want 2", len(sess.Messages))
	}
}

// TestParseSession_AgentName covers the agentId path on assistant lines.
func TestParseSession_AgentName(t *testing.T) {
	src := strings.Join([]string{
		`{"type":"user","uuid":"u1","parentUuid":null,"sessionId":"s","timestamp":"2026-07-20T10:00:00Z","cwd":"/p","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"s","timestamp":"2026-07-20T10:00:01Z","cwd":"/p","agentId":"explore","message":{"role":"assistant","content":[{"type":"text","text":"x"}]}}`,
		"",
	}, "\n")
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "s.jsonl", src)
	if _, err := NewSessionReader("").ParseSession(path); err != nil {
		t.Fatal(err)
	}
}

// TestParseSession_TopLevelModel covers the model field on user lines.
func TestParseSession_TopLevelModel(t *testing.T) {
	src := strings.Join([]string{
		`{"type":"user","uuid":"u1","parentUuid":null,"sessionId":"s","timestamp":"2026-07-20T10:00:00Z","cwd":"/p","model":"claude-opus","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"s","timestamp":"2026-07-20T10:00:01Z","cwd":"/p","model":"claude-opus","message":{"role":"assistant","content":[{"type":"text","text":"y"}]}}`,
		"",
	}, "\n")
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "s.jsonl", src)
	sess, err := NewSessionReader("").ParseSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Messages[1].ModelID != "claude-opus" {
		t.Fatalf("ModelID = %q", sess.Messages[1].ModelID)
	}
}

// TestParseSession_CustomTimestamp covers the RFC3339 fallback path.
func TestParseSession_CustomTimestamp(t *testing.T) {
	ts := parseTimestamp("2026-12-31T23:59:59+02:00")
	want := time.Date(2026, 12, 31, 21, 59, 59, 0, time.UTC)
	if !ts.Equal(want) {
		t.Fatalf("got %v want %v", ts, want)
	}
}

// TestParseSession_GarbageLines tests that junk lines don't break the parse.
func TestParseSession_GarbageLines(t *testing.T) {
	src := strings.Join([]string{
		`{not even json`,
		``,
		`"a string"`,
		`{"type":"user","uuid":"u","parentUuid":null,"sessionId":"s","timestamp":"2026-07-20T10:00:00Z","cwd":"/p","message":{"role":"user","content":"hi"}}`,
		`{"type":42,"uuid":"x"}`, // type unmarshallable
		`{"type":"ai-title","title":"Real Title"}`,
		"",
	}, "\n")
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "s.jsonl", src)
	sess, err := NewSessionReader("").ParseSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Title != "Real Title" {
		t.Fatalf("title = %q", sess.Title)
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("messages = %d want 1", len(sess.Messages))
	}
}

// TestParseSession_EmptyFile covers the empty-session error path.
func TestParseSession_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "s.jsonl", "")
	_, err := NewSessionReader("").ParseSession(path)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty error, got %v", err)
	}
}

// TestParseSession_LastPromptNotEmitted covers that last-prompt records
// are captured for context but don't appear as user messages.
func TestParseSession_LastPromptNotEmitted(t *testing.T) {
	src := strings.Join([]string{
		`{"type":"user","uuid":"u","parentUuid":null,"sessionId":"s","timestamp":"2026-07-20T10:00:00Z","cwd":"/p","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","uuid":"a","parentUuid":"u","sessionId":"s","timestamp":"2026-07-20T10:00:01Z","cwd":"/p","message":{"role":"assistant","content":[{"type":"text","text":"reply"}]}}`,
		`{"type":"last-prompt","leafUuid":"a","timestamp":"2026-07-20T10:00:02Z","sessionId":"s"}`,
		"",
	}, "\n")
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "s.jsonl", src)
	sess, err := NewSessionReader("").ParseSession(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range sess.Messages {
		if m.OriginID == "a" && m.Role == domain.RoleUser {
			t.Fatal("last-prompt should not be emitted as a user message")
		}
	}
}

// TestParseSession_BridgeSession covers that bridge-session records are
// skipped (they only apply on the OC → CC direction).
func TestParseSession_BridgeSession(t *testing.T) {
	src := `{"type":"bridge-session","sessionId":"main","timestamp":"2026-07-20T10:00:00Z"}` + "\n"
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "main.jsonl", src)
	sess, err := NewSessionReader("").ParseSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Messages) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(sess.Messages))
	}
}

func TestParseEntry_UnknownType(t *testing.T) {
	// Unknown types like "attachment" or "system" are skipped silently.
	src := strings.Join([]string{
		`{"type":"system","subtype":"init","timestamp":"2026-07-20T10:00:00Z"}`,
		`{"type":"user","uuid":"u","parentUuid":null,"sessionId":"s","timestamp":"2026-07-20T10:00:01Z","cwd":"/p","message":{"role":"user","content":"hi"}}`,
		"",
	}, "\n")
	dir := t.TempDir()
	path := writeJSONLFile(t, dir, "s.jsonl", src)
	sess, err := NewSessionReader("").ParseSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("messages = %d want 1", len(sess.Messages))
	}
}

// Helper: write body to dir/file.jsonl and return the path.
func writeJSONLFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func init() {
	// suppress "imported and not used" for encoding/json
	_ = json.Valid
}
