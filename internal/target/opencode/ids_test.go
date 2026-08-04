package opencode

import (
	"strings"
	"testing"
)

func TestGenID_Deterministic(t *testing.T) {
	a := GenID("ses", "seed-1", map[string]struct{}{})
	b := GenID("ses", "seed-1", map[string]struct{}{})
	if a != b {
		t.Fatalf("non-deterministic: %s vs %s", a, b)
	}
	if !strings.HasPrefix(a, "ses_") {
		t.Fatalf("missing prefix: %s", a)
	}
	if len(a) != 4+26 {
		t.Fatalf("len = %d want 30", len(a))
	}
}

func TestGenID_DifferentSeedsDifferentIDs(t *testing.T) {
	a := GenID("ses", "one", map[string]struct{}{})
	b := GenID("ses", "two", map[string]struct{}{})
	if a == b {
		t.Fatalf("collision: %s", a)
	}
}

func TestGenID_CollisionAppendsCounter(t *testing.T) {
	existing := map[string]struct{}{}
	first := GenID("ses", "x", existing)
	second := GenID("ses", "x", existing)
	if first == second {
		t.Fatalf("expected distinct id on collision, both = %s", first)
	}
	if !strings.HasPrefix(second, "ses_") {
		t.Fatalf("missing prefix on collided id: %s", second)
	}
}

func TestProjectIDForWorktree(t *testing.T) {
	if ProjectIDForWorktree("/") != "global" {
		t.Fatalf("/ should produce global, got %s", ProjectIDForWorktree("/"))
	}
	if ProjectIDForWorktree("") != "global" {
		t.Fatalf("empty should produce global, got %s", ProjectIDForWorktree(""))
	}
	h := ProjectIDForWorktree("/home/me/projects")
	if len(h) != 40 {
		t.Fatalf("hash length = %d want 40", len(h))
	}
	if ProjectIDForWorktree("/home/me/projects") != h {
		t.Fatalf("not deterministic")
	}
}

func TestIsProjectGlobal(t *testing.T) {
	if !IsProjectGlobal("global") {
		t.Fatal("global should be global")
	}
	if IsProjectGlobal("anything else") {
		t.Fatal("non-global should not match")
	}
}

func TestHash16_Deterministic(t *testing.T) {
	a := Hash16("hello")
	b := Hash16("hello")
	if a != b {
		t.Fatalf("non-deterministic")
	}
	if len(a) != 16 {
		t.Fatalf("hex length = %d want 16", len(a))
	}
}