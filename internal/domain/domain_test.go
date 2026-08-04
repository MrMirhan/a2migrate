package domain

import (
	"testing"
	"time"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "session"},
		{"   ", "session"},
		{"Hello, World!", "hello-world"},
		{"foo_bar baz", "foo-bar-baz"},
		{"---trim---", "trim"},
		{"MIXED/CASE\\path", "mixed-case-path"},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRole_Valid(t *testing.T) {
	if !RoleUser.Valid() || !RoleAssistant.Valid() || !RoleSystem.Valid() {
		t.Fatal("expected user/assistant/system to be valid")
	}
	if Role("wizard").Valid() {
		t.Fatal("expected wizard to be invalid")
	}
}

func TestSession_Validate(t *testing.T) {
	now := time.Now()
	good := Session{
		OriginID:   "abc",
		Title:      "t",
		ProjectDir: "/x",
		CreatedAt:  now,
		Messages: []Message{
			{OriginID: "m1", SessionID: "abc", Role: RoleUser, CreatedAt: now},
		},
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("good session rejected: %v", err)
	}
	bad := good
	bad.Title = ""
	if err := bad.Validate(); err == nil {
		t.Fatal("empty title should fail")
	}
	bad = good
	bad.Messages[0].Role = "wizard"
	if err := bad.Validate(); err == nil {
		t.Fatal("invalid role should fail")
	}
}

func TestPart_Validate(t *testing.T) {
	if err := (Part{Type: PartTool, ToolName: ""}).Validate(); err == nil {
		t.Fatal("tool part without name should fail")
	}
	if err := (Part{Type: PartText}).Validate(); err != nil {
		t.Fatalf("text part should validate: %v", err)
	}
	if err := (Part{Type: ""}).Validate(); err == nil {
		t.Fatal("empty part type should fail")
	}
}

func TestMCPServer_IsLocal(t *testing.T) {
	def := MCPServer{}
	if !def.IsLocal() {
		t.Fatal("default (no URL) should be local")
	}
	remote := MCPServer{URL: "https://x"}
	if remote.IsLocal() {
		t.Fatal("with URL should not be local")
	}
}

func TestRelativePath(t *testing.T) {
	got := RelativePath("/home/me", "/home/me/projects/x")
	if got == "/home/me/projects/x" {
		t.Fatal("expected relative output")
	}
}