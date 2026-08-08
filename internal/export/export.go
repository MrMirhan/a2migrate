// Package export renders a tool's sessions and artifacts into
// self-contained documents. Unlike migration, nothing is written back
// into another tool's store: the output is for reading, archiving, or
// feeding to something outside a2migrate.
package export

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

// Format is an output encoding.
type Format string

const (
	FormatMarkdown Format = "md"
	FormatJSON     Format = "json"
	FormatHTML     Format = "html"
	FormatText     Format = "txt"
)

// formatAliases maps what a user may type onto the canonical format.
// The canonical spelling doubles as the file extension.
var formatAliases = map[string]Format{
	"md":       FormatMarkdown,
	"markdown": FormatMarkdown,
	"json":     FormatJSON,
	"html":     FormatHTML,
	"htm":      FormatHTML,
	"txt":      FormatText,
	"text":     FormatText,
	"plain":    FormatText,
}

// ParseFormat resolves a command-line format argument.
func ParseFormat(s string) (Format, error) {
	key := strings.ToLower(strings.TrimSpace(s))
	if f, ok := formatAliases[key]; ok {
		return f, nil
	}
	return "", fmt.Errorf("unknown format %q (known: %s)", s, strings.Join(Formats(), ", "))
}

// Formats lists every accepted format spelling, for help text and shell
// completion.
func Formats() []string {
	out := make([]string, 0, len(formatAliases))
	for name := range formatAliases {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Ext is the file extension for f, without the dot.
func (f Format) Ext() string { return string(f) }

// Bundle is one export's worth of content. Every field is optional; the
// renderers skip whatever is empty.
type Bundle struct {
	Tool     string               `json:"tool"`
	Sessions []domain.Session     `json:"sessions,omitempty"`
	Skills   []domain.Skill       `json:"skills,omitempty"`
	Commands []domain.Command     `json:"commands,omitempty"`
	Agents   []domain.AgentDef    `json:"agents,omitempty"`
	Rules    []domain.Rule        `json:"rules,omitempty"`
	MCP      []domain.MCPServer   `json:"mcp,omitempty"`
	System   *domain.SystemPrompt `json:"system,omitempty"`
}

// IsEmpty reports whether there is nothing to render.
func (b Bundle) IsEmpty() bool {
	return len(b.Sessions) == 0 && len(b.Skills) == 0 && len(b.Commands) == 0 &&
		len(b.Agents) == 0 && len(b.Rules) == 0 && len(b.MCP) == 0 && b.System == nil
}

// Write renders b to w in the requested format.
func Write(w io.Writer, b Bundle, f Format) error {
	switch f {
	case FormatMarkdown:
		return writeMarkdown(w, b)
	case FormatText:
		return writeText(w, b)
	case FormatJSON:
		return writeJSON(w, b)
	case FormatHTML:
		return writeHTML(w, b)
	default:
		return fmt.Errorf("unknown format %q", f)
	}
}

// messageLabel is the speaker heading shown for a message. The agent
// name matters for subagent transcripts, where every turn is nominally
// "assistant" but came from a different agent.
func messageLabel(m domain.Message) string {
	role := string(m.Role)
	if role == "" {
		role = "unknown"
	}
	if m.Agent != "" && m.Agent != role {
		return role + " (" + m.Agent + ")"
	}
	return role
}

// modelLabel renders the provider/model pair, or "" when neither is
// recorded.
func modelLabel(m domain.Message) string {
	switch {
	case m.ProviderID != "" && m.ModelID != "":
		return m.ProviderID + "/" + m.ModelID
	case m.ModelID != "":
		return m.ModelID
	default:
		return ""
	}
}

// tokenLabel summarises a usage block for a one-line footer. Returns ""
// when nothing was preserved, so callers can omit the line entirely
// rather than print a row of zeros.
func tokenLabel(t domain.Tokens) string {
	if t.IsZero() {
		return ""
	}
	parts := make([]string, 0, 4)
	if t.Input > 0 {
		parts = append(parts, fmt.Sprintf("in %d", t.Input))
	}
	if t.Output > 0 {
		parts = append(parts, fmt.Sprintf("out %d", t.Output))
	}
	if t.Reasoning > 0 {
		parts = append(parts, fmt.Sprintf("reasoning %d", t.Reasoning))
	}
	if t.CacheRead > 0 || t.CacheWrite > 0 {
		parts = append(parts, fmt.Sprintf("cache %d/%d", t.CacheRead, t.CacheWrite))
	}
	return strings.Join(parts, ", ")
}

// skipPart reports whether a part carries nothing worth rendering.
// Step markers exist to satisfy OpenCode's renderer and say nothing to
// a human reader.
func skipPart(p domain.Part) bool {
	switch p.Type {
	case domain.PartStepStart, domain.PartStepFinish:
		return true
	case domain.PartText, domain.PartReasoning:
		return strings.TrimSpace(p.Text) == ""
	default:
		return false
	}
}
