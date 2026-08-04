package tools

import (
	"path/filepath"
	"testing"
)

func TestGet_Known(t *testing.T) {
	for _, id := range []ID{"claude_code", "opencode"} {
		tool, ok := Get(id)
		if !ok {
			t.Errorf("Get(%q) not found", id)
			continue
		}
		if tool.ID != id {
			t.Errorf("Get(%q).ID = %q", id, tool.ID)
		}
		if tool.DisplayName == "" {
			t.Errorf("Get(%q).DisplayName empty", id)
		}
		if tool.ConfigPath() == "" {
			t.Errorf("Get(%q).ConfigPath() empty", id)
		}
	}
}

func TestGet_Missing(t *testing.T) {
	if _, ok := Get("does-not-exist"); ok {
		t.Fatal("expected Get(\"does-not-exist\") to be missing")
	}
}

func TestAll_CoversKnown(t *testing.T) {
	got := IDs()
	for _, want := range []ID{"claude_code", "opencode"} {
		found := false
		for _, id := range got {
			if id == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("All() missing %q", want)
		}
	}
}

func TestRegister_AddsAndRemoves(t *testing.T) {
	clearAndRestore := func() {
		// Save a snapshot, run the test, then restore the registry.
		saved := All()
		ClearForTest()
		for _, t := range saved {
			Register(t)
		}
	}
	clearAndRestore()

	t.Cleanup(clearAndRestore)

	const want ID = "codex"
	if _, ok := Get(want); ok {
		t.Fatalf("expected %q to be missing before register", want)
	}
	Register(Tool{ID: want, DisplayName: "Codex", ConfigRoot: ".codex"})
	got, ok := Get(want)
	if !ok {
		t.Fatalf("Get(%q) failed after Register", want)
	}
	if got.ID != want {
		t.Fatalf("Get(%q).ID = %q", want, got.ID)
	}
}

func TestExpandPath_Relative(t *testing.T) {
	got := expandPath(".claude")
	if !filepath.IsAbs(got) {
		t.Fatalf("expandPath(.claude) = %q (expected absolute)", got)
	}
	if filepath.Base(got) != ".claude" {
		t.Fatalf("expandPath(.claude) basename = %q", filepath.Base(got))
	}
}

func TestToolHas(t *testing.T) {
	cc := MustGet("claude_code")
	if !cc.Has(CapSessions) {
		t.Fatal("claude_code should have CapSessions")
	}
	if cc.Has(CapSystemPrompt) == false {
		t.Fatal("claude_code should have CapSystemPrompt")
	}
}

func TestToolAllCapabilities_Sorted(t *testing.T) {
	cc := MustGet("claude_code")
	caps := cc.AllCapabilities()
	for i := 1; i < len(caps); i++ {
		if caps[i-1] > caps[i] {
			t.Fatalf("capabilities not sorted: %v", caps)
		}
	}
}
