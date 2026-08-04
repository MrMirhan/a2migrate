// Package tools catalogs the AI coding CLIs that a2migrate knows about.
//
// Each tool adapter is implemented as a pair of packages:
//
//	internal/source/<tool>     reader for that tool's on-disk state
//	internal/target/<tool>     writer for that tool's on-disk state
//
// The registry here is the directory: it enumerates which tools exist,
// where their state lives, and what artifact categories they expose
// (sessions / skills / commands / agents / rules / MCP / system prompt).
//
// When adding a new tool:
//
//	1. Append an entry to knownTools with its paths and capabilities.
//	2. Create internal/source/<tool> and internal/target/<tool>.
//	3. Cover the tool's artifacts in the orchestrators (migrate, sync).
//	4. Update ROADMAP.md and the capabilities matrix in README.md.
//
// Don't make this registry a megafunction — keep it declarative. New
// metadata should be a struct literal in knownTools, not a hard-coded
// if/else tree scattered across the codebase.
package tools

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/mirhan/a2migrate/internal/platform"
)

// ID is the canonical short name for a tool.
type ID string

// Capability is a category of state a tool stores on disk.
type Capability string

const (
	CapSessions       Capability = "sessions"
	CapSkills         Capability = "skills"
	CapCommands       Capability = "commands"
	CapAgents         Capability = "agents"
	CapRules          Capability = "rules"
	CapMCP            Capability = "mcp"
	CapSystemPrompt  Capability = "system_prompt"
)

// Tool describes one AI coding CLI.
//
// Path conventions:
//   - CC uses `~/.claude/` (configurable via $CLAUDE_CODE_HOME)
//   - OC uses `~/.config/opencode/` + `~/.local/share/opencode/opencode.db`
//
// New tools should declare their config root (markdown / db / JSON files)
// and a data root (db / large files). When unsure, prefer what the tool's
// own docs say; verify on a fresh install with `ls` before locking in.
type Tool struct {
	ID           ID
	DisplayName  string // human-readable
	ConfigRoot   string // path template under $HOME (XDG-aware)
	DataRoot     string // path template for db / sessions
	Capabilities []Capability
	SessionGlob  string // glob pattern relative to DataRoot, e.g. "projects/*/*.jsonl"
}

// Path pairs are home-template strings; substitute via expandPath.
func (t Tool) ConfigPath() string { return expandPath(t.ConfigRoot) }
func (t Tool) DataPath() string   { return expandPath(t.DataRoot) }

// AllCapabilities returns capabilities sorted alphabetically. Useful
// for printing tool capability matrices.
func (t Tool) AllCapabilities() []Capability {
	out := make([]Capability, len(t.Capabilities))
	copy(out, t.Capabilities)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (t Tool) Has(c Capability) bool {
	for _, x := range t.Capabilities {
		if x == c {
			return true
		}
	}
	return false
}

// knownTools is the source of truth. Order is stable for tests; do
// not sort at registration time — use All() for display order.
var knownTools = []Tool{
	{
		ID:           "claude_code",
		DisplayName:  "Claude Code",
		ConfigRoot:   ".claude",
		DataRoot:     ".claude",
		Capabilities: []Capability{CapSessions, CapSkills, CapCommands, CapAgents, CapRules, CapMCP, CapSystemPrompt},
		SessionGlob:  "projects/*/*.jsonl",
	},
	{
		ID:           "opencode",
		DisplayName:  "OpenCode",
		ConfigRoot:   ".config/opencode",
		DataRoot:     ".local/share/opencode",
		Capabilities: []Capability{CapSessions, CapSkills, CapCommands, CapAgents, CapRules, CapMCP, CapSystemPrompt},
		SessionGlob:  "opencode.db",
	},
}

var (
	registryMu sync.RWMutex
)

// Get returns the tool with the given ID, or false.
func Get(id ID) (Tool, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, t := range knownTools {
		if t.ID == id {
			return t, true
		}
	}
	return Tool{}, false
}

// MustGet returns the tool or panics. Tests only.
func MustGet(id ID) Tool {
	t, ok := Get(id)
	if !ok {
		panic("unknown tool: " + string(id))
	}
	return t
}

// All returns the registry. Returns a copy to prevent mutation.
func All() []Tool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Tool, len(knownTools))
	copy(out, knownTools)
	return out
}

// IDs returns the sorted list of tool IDs (for stable output).
func IDs() []ID {
	all := All()
	out := make([]ID, len(all))
	for i, t := range all {
		out[i] = t.ID
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Register adds a tool. Intended for tests + future plugin loading.
func Register(t Tool) {
	registryMu.Lock()
	defer registryMu.Unlock()
	knownTools = append(knownTools, t)
}

// ClearForTest empties the registry. Tests only.
func ClearForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	knownTools = nil
}

// expandPath resolves a "~/foo" template via the user's home directory.
// Uses XDG fallback ($XDG_DATA_HOME / $XDG_CONFIG_HOME) when the path
// starts with "." — only config-and-data tools that explicitly check
// XDG opt in.
func expandPath(template string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return template
	}
	if !filepath.IsAbs(template) {
		switch {
		case len(template) >= 6 && template[:6] == ".conf":
			if x := platform.EnvOr("XDG_CONFIG_HOME", ""); x != "" {
				return filepath.Join(x, template[len(".config/"):])
			}
		case len(template) >= 5 && template[:5] == ".loca":
			if x := platform.EnvOr("XDG_DATA_HOME", ""); x != "" {
				return filepath.Join(x, template[len(".local/share/"):])
			}
		}
		return filepath.Join(home, template)
	}
	return template
}
