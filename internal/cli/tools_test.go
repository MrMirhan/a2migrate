package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestTools_List(t *testing.T) {
	cmd := NewRootCmd(nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"tools", "list"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"claude_code", "opencode", "Claude Code", "OpenCode"} {
		if !strings.Contains(out, want) {
			t.Errorf("tools list missing %q:\n%s", want, out)
		}
	}
}

func TestTools_Show_Known(t *testing.T) {
	cmd := NewRootCmd(nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"tools", "show", "claude_code"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "config:") || !strings.Contains(out, "data:") {
		t.Errorf("tools show missing fields:\n%s", out)
	}
}

func TestTools_Show_Unknown_Errors(t *testing.T) {
	cmd := NewRootCmd(nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"tools", "show", "this-tool-does-not-exist"})
	err := cmd.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected error for unknown tool ID")
	}
}
