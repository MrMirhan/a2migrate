package migrate

import (
	"testing"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/source/claudecode"
)

// These tests exercise the migrator wiring without touching real Claude Code
// or OpenCode state. They use temp dirs and a sandbox CC home.

func TestSessionMigrator_Selected_FilterSearch(t *testing.T) {
	opts := Options{Search: "bug"}
	m := NewSessionMigrator(opts)
	refs := []claudecode.SessionRef{
		{OriginID: "s1", FilePath: "/x/bug.jsonl"},
		{OriginID: "s2", FilePath: "/x/other.jsonl"},
	}
	out := m.Selected(refs)
	if len(out) != 1 || out[0].OriginID != "s1" {
		t.Fatalf("selected = %v want [s1]", out)
	}
}

func TestSessionMigrator_Selected_FilterIncludes(t *testing.T) {
	opts := Options{Includes: []string{"a", "b"}}
	m := NewSessionMigrator(opts)
	refs := []claudecode.SessionRef{
		{OriginID: "a"}, {OriginID: "b"}, {OriginID: "c"},
	}
	out := m.Selected(refs)
	if len(out) != 2 {
		t.Fatalf("selected = %v want 2", out)
	}
}

func TestSessionMigrator_Selected_FilterExcludes(t *testing.T) {
	opts := Options{Excludes: []string{"b"}}
	m := NewSessionMigrator(opts)
	refs := []claudecode.SessionRef{
		{OriginID: "a"}, {OriginID: "b"}, {OriginID: "c"},
	}
	out := m.Selected(refs)
	if len(out) != 2 {
		t.Fatalf("selected = %v want 2", out)
	}
}

func TestParsedRenames(t *testing.T) {
	out, err := ParsedRenames([]string{"old=new", "foo=bar"})
	if err != nil {
		t.Fatal(err)
	}
	if out["old"] != "new" || out["foo"] != "bar" {
		t.Fatalf("got %v", out)
	}
	if _, err := ParsedRenames([]string{"invalid"}); err == nil {
		t.Fatal("expected error on missing '='")
	}
}

func TestPartCount(t *testing.T) {
	s := domain.Session{
		Messages: []domain.Message{
			{Parts: []domain.Part{{}, {}, {}}},
			{Parts: []domain.Part{{}}},
		},
	}
	if got := partCount(s); got != 4 {
		t.Fatalf("partCount = %d want 4", got)
	}
}

func TestSetFromStrings(t *testing.T) {
	got := setFromStrings([]string{"a", "b"})
	if !got["a"] || !got["b"] || got["c"] {
		t.Fatalf("got %v", got)
	}
}