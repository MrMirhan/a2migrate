package migrate

import (
	"testing"

	"github.com/MrMirhan/a2migrate/internal/domain"
	"github.com/MrMirhan/a2migrate/internal/source/claudecode"
)

// These tests exercise the migrator wiring without touching real Claude Code
// or OpenCode state. They use temp dirs and a sandbox CC home.

// Claude Code names each transcript after the session uuid, so the
// filename carries nothing the id does not. Search matches the id and
// the worktree, which is what a user remembers a session by.
func TestSessionMigrator_Selected_FilterSearch(t *testing.T) {
	refs := []claudecode.SessionRef{
		{OriginID: "s1", FilePath: "/p/s1.jsonl", Worktree: "/home/u/works/api"},
		{OriginID: "s2", FilePath: "/p/s2.jsonl", Worktree: "/home/u/works/site"},
	}
	tests := []struct {
		name, search, want string
	}{
		{"by worktree", "api", "s1"},
		{"by worktree, other side", "site", "s2"},
		{"by id", "s2", "s2"},
		{"case-insensitive", "API", "s1"},
	}
	for _, tt := range tests {
		out := NewSessionMigrator(Options{Search: tt.search}).Selected(refs)
		if len(out) != 1 || out[0].OriginID != tt.want {
			t.Errorf("%s: search %q selected %v, want [%s]", tt.name, tt.search, out, tt.want)
		}
	}

	if out := NewSessionMigrator(Options{Search: "nothing"}).Selected(refs); len(out) != 0 {
		t.Errorf("a search matching nothing selected %v", out)
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
