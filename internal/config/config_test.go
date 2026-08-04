package config

import (
	"strings"
	"testing"

	"github.com/mirhan/a2migrate/internal/tools"
)

const sampleConfig = `
# a2migrate multi-endpoint sync config.
version = 1

# Local machine's external drive.
[[endpoint]]
id   = "workstation-b"
kind = "local"
path = "/Volumes/external/a2migrate"
tools = ["opencode"]

# Remote VDS.
[[endpoint]]
id   = "vds-istanbul"
kind = "ssh"
host = "10.0.0.5"
user = "alice"
path = "/home/alice/.local/share/a2migrate"
tools = ["claude_code", "opencode"]
`

func TestParse_Sample(t *testing.T) {
	f, err := Parse(strings.NewReader(sampleConfig), "test.toml")
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != 1 {
		t.Errorf("version = %d", f.Version)
	}
	if len(f.Endpoints) != 2 {
		t.Fatalf("endpoints = %d want 2", len(f.Endpoints))
	}
	local := f.Endpoints[0]
	if local.ID != "workstation-b" || local.Kind != KindLocal || local.Path != "/Volumes/external/a2migrate" {
		t.Errorf("local endpoint malformed: %+v", local)
	}
	ssh := f.Endpoints[1]
	if ssh.ID != "vds-istanbul" || ssh.Kind != KindSSH || ssh.Host != "10.0.0.5" || ssh.User != "alice" {
		t.Errorf("ssh endpoint malformed: %+v", ssh)
	}
}

func TestValidate_MissingSSHHost(t *testing.T) {
	src := `
version = 1
[[endpoint]]
id   = "x"
kind = "ssh"
path = "/p"
`
	_, err := Parse(strings.NewReader(src), "test.toml")
	if err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("expected host error, got %v", err)
	}
}

func TestValidate_MissingPath(t *testing.T) {
	src := `
version = 1
[[endpoint]]
id   = "x"
kind = "local"
`
	_, err := Parse(strings.NewReader(src), "test.toml")
	if err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("expected path error, got %v", err)
	}
}

func TestValidate_UnknownKind(t *testing.T) {
	src := `
version = 1
[[endpoint]]
id   = "x"
kind = "magic"
path = "/p"
`
	_, err := Parse(strings.NewReader(src), "test.toml")
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected kind error, got %v", err)
	}
}

func TestValidate_VersionMismatch(t *testing.T) {
	src := `
version = 99
[[endpoint]]
id   = "x"
kind = "local"
path = "/p"
`
	_, err := Parse(strings.NewReader(src), "test.toml")
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestLookupTool(t *testing.T) {
	f, err := Parse(strings.NewReader(sampleConfig), "test.toml")
	if err != nil {
		t.Fatal(err)
	}
	// claude_code → only vds-istanbul
	got := f.LookupTool(tools.ID("claude_code"))
	if len(got) != 1 || got[0].ID != "vds-istanbul" {
		t.Errorf("claude_code endpoints = %+v", got)
	}
	// opencode → both
	got = f.LookupTool(tools.ID("opencode"))
	if len(got) != 2 {
		t.Errorf("opencode endpoints = %d want 2", len(got))
	}
	// unknown → 0
	got = f.LookupTool(tools.ID("gemini_cli"))
	if len(got) != 0 {
		t.Errorf("gemini_cli endpoints = %d want 0", len(got))
	}
}

func TestParse_InlineComments(t *testing.T) {
	// Comments on their own lines are stripped. Inline comments after a
	// value are not stripped by the minimal parser; document that.
	src := `
# leading comment
version = 1
# middle comment

[[endpoint]]
# section comment
id   = "x"
kind = "local"
path = "/p"
`
	if _, err := Parse(strings.NewReader(src), "test.toml"); err != nil {
		t.Fatal(err)
	}
}

func TestParse_ToolsListWithSingleQuotes(t *testing.T) {
	src := `
version = 1
[[endpoint]]
id   = "x"
kind = "local"
path = "/p"
tools = ['claude_code', 'opencode']
`
	f, err := Parse(strings.NewReader(src), "test.toml")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Endpoints) != 1 || len(f.Endpoints[0].Tools) != 2 {
		t.Fatalf("expected 1 endpoint with 2 tools, got %+v", f.Endpoints)
	}
}

func TestParse_UnknownTopLevelKey(t *testing.T) {
	src := `
version = 1
unknown_key = "x"
`
	if _, err := Parse(strings.NewReader(src), "test.toml"); err == nil {
		t.Fatal("expected error on unknown top-level key")
	}
}

func TestParse_UnknownTable(t *testing.T) {
	src := `
version = 1
[[unknown]]
id = "x"
`
	if _, err := Parse(strings.NewReader(src), "test.toml"); err == nil {
		t.Fatal("expected error on unknown table")
	}
}
