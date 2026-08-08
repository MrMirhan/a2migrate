// Package domain defines the pure data types used across a2migrate.
//
// These types intentionally carry no behavior beyond accessors and basic
// constructors. They exist to decouple source readers (Claude Code JSONL,
// frontmatter files, MCP config) from target writers (OpenCode SQLite,
// copy/symlink operations).
package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ErrInvalid is returned when a domain value fails validation.
var ErrInvalid = errors.New("invalid domain value")

// Origin identifies the source system a record came from.
type Origin string

const (
	OriginClaudeCode Origin = "claude_code"
	OriginOpenCode   Origin = "opencode"
)

// Session represents one chat session in either Claude Code or OpenCode form.
type Session struct {
	ID         string    `json:"id"`                  // OpenCode: "ses_<26b32>"; Claude Code: uuid filename
	OriginID   string    `json:"origin_id,omitempty"` // original ID in source system (e.g. CC session UUID)
	Origin     Origin    `json:"origin"`
	Title      string    `json:"title,omitempty"`
	Slug       string    `json:"slug,omitempty"`
	ProjectDir string    `json:"project_dir,omitempty"` // absolute working directory the session was opened in
	IsSubagent bool      `json:"is_subagent,omitempty"`
	ParentID   string    `json:"parent_id,omitempty"` // OriginID of parent session (only set when IsSubagent)
	CreatedAt  time.Time `json:"created_at,omitzero"`
	UpdatedAt  time.Time `json:"updated_at,omitzero"`
	Messages   []Message `json:"messages,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
}

// Message is one turn envelope. Part content lives in []Part.
type Message struct {
	ID         string    `json:"id"`
	OriginID   string    `json:"origin_id,omitempty"`
	SessionID  string    `json:"session_id,omitempty"`
	ParentID   string    `json:"parent_id,omitempty"` // OriginID of predecessor message (or "")
	Role       Role      `json:"role"`
	CreatedAt  time.Time `json:"created_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"` // zero if not yet finished
	Agent      string    `json:"agent,omitempty"`
	ModelID    string    `json:"model_id,omitempty"`
	ProviderID string    `json:"provider_id,omitempty"`
	Variant    string    `json:"variant,omitempty"`
	// Tokens holds the per-message usage block. Populated from CC's
	// message.usage on forward migration; populated from OC's
	// message.data.tokens on reverse migration. Zero-value means
	// "unknown / not preserved" — never fabricate.
	Tokens Tokens `json:"tokens,omitzero"`
	// CostUSD is the OC-side cost figure. Always zero on forward
	// migration (CC doesn't track cost). Preserved on reverse.
	CostUSD float64 `json:"cost_usd,omitempty"`
	Parts   []Part  `json:"parts,omitempty"`
}

// Tokens is the portable per-message usage shape. Both CC and OC carry
// these values (CC as input/output/cache_*/..., OC as the same plus a
// reasoning_tokens slot that Anthropic doesn't currently emit).
type Tokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	Reasoning  int64 `json:"reasoning"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
	// ServiceTier and Speed pass through from CC's usage block. OC
	// doesn't track these so they're informational.
	ServiceTier string `json:"service_tier,omitempty"`
	Speed       string `json:"speed,omitempty"`
}

// IsZero reports whether the token block is empty.
func (t Tokens) IsZero() bool {
	return t == Tokens{}
}

// Role is the speaker role.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
)

// Valid reports whether r is a recognized role.
func (r Role) Valid() bool {
	switch r {
	case RoleUser, RoleAssistant, RoleSystem:
		return true
	}
	return false
}

// PartType is the discriminant for Part.
type PartType string

const (
	PartText       PartType = "text"
	PartReasoning  PartType = "reasoning"
	PartTool       PartType = "tool"
	PartStepStart  PartType = "step-start"
	PartStepFinish PartType = "step-finish"
)

// Part is one typed chunk within a message.
type Part struct {
	ID         string         `json:"id,omitempty"`
	OriginID   string         `json:"origin_id,omitempty"`
	Type       PartType       `json:"type"`
	Text       string         `json:"text,omitempty"`         // text / reasoning
	ToolName   string         `json:"tool_name,omitempty"`    // tool parts
	ToolCallID string         `json:"tool_call_id,omitempty"` // tool parts
	ToolInput  map[string]any `json:"tool_input,omitempty"`   // tool parts
	ToolOutput string         `json:"tool_output,omitempty"`  // tool parts
	ToolStatus string         `json:"tool_status,omitempty"`  // "pending" | "running" | "completed" | "error"
	CreatedAt  time.Time      `json:"created_at,omitzero"`
}

// Skill is one Claude Code / OpenCode skill definition.
type Skill struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Origin      Origin         `json:"origin,omitempty"`
	SourcePath  string         `json:"source_path,omitempty"` // file on disk in source system
	Body        string         `json:"body,omitempty"`        // markdown body (after frontmatter)
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
}

// Command is one slash command (markdown + frontmatter).
type Command struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	ArgumentHint string         `json:"argument_hint,omitempty"`
	AllowedTools []string       `json:"allowed_tools,omitempty"`
	SourcePath   string         `json:"source_path,omitempty"`
	Body         string         `json:"body,omitempty"`
	Frontmatter  map[string]any `json:"frontmatter,omitempty"`
}

// AgentDef is one subagent/agent definition.
type AgentDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Model       string         `json:"model,omitempty"`
	Tools       []string       `json:"tools,omitempty"`
	SourcePath  string         `json:"source_path,omitempty"`
	Body        string         `json:"body,omitempty"`
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
}

// Rule is one path-scoped or global rule.
type Rule struct {
	Name        string         `json:"name"`
	Paths       []string       `json:"paths,omitempty"`
	SourcePath  string         `json:"source_path,omitempty"`
	Body        string         `json:"body,omitempty"`
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
}

// MCPServer describes one MCP server entry.
type MCPServer struct {
	Name        string            `json:"name"`
	Command     []string          `json:"command,omitempty"` // normalized: executable + args
	Environment map[string]string `json:"environment,omitempty"`
	Type        string            `json:"type,omitempty"` // "local" | "remote"
	Enabled     bool              `json:"enabled"`
	URL         string            `json:"url,omitempty"` // remote only
	Headers     map[string]string `json:"headers,omitempty"`
}

// IsLocal returns true if the server runs as a local subprocess.
func (m MCPServer) IsLocal() bool { return m.Type == "local" || m.URL == "" }

// Hook is one lifecycle event hook from Claude Code settings.json.
// Stored structurally; a2migrate only reports these — OC plugins are manual.
type Hook struct {
	Event   string // PreToolUse | PostToolUse | SessionStart | UserPromptSubmit | Stop | ...
	Matcher string
	Command string
	Timeout int
	Type    string
}

// SystemPrompt is the top-level instructions file that a tool injects into
// every session's context. CC: ~/.claude/CLAUDE.md. OC: ~/.config/opencode/AGENTS.md.
type SystemPrompt struct {
	Origin     Origin `json:"origin,omitempty"`
	SourcePath string `json:"source_path,omitempty"`
	Body       string `json:"body,omitempty"`
}

// Project represents one workspace root, shared by both systems.
type Project struct {
	ID          string // OpenCode: sha1(worktree)[:40] OR "global"
	Worktree    string // absolute path
	Name        string
	TimeCreated time.Time
	TimeUpdated time.Time
	Sandboxes   []string
}

// Validate sanity-checks a Session. Used by tests and apply-mode preflight.
func (s Session) Validate() error {
	switch {
	case s.OriginID == "":
		return fmt.Errorf("%w: session origin id required", ErrInvalid)
	case s.Title == "":
		return fmt.Errorf("%w: session title required", ErrInvalid)
	case s.ProjectDir == "":
		return fmt.Errorf("%w: session project dir required", ErrInvalid)
	case s.CreatedAt.IsZero():
		return fmt.Errorf("%w: session created_at required", ErrInvalid)
	}
	for i, m := range s.Messages {
		if err := m.Validate(); err != nil {
			return fmt.Errorf("messages[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate sanity-checks a Message.
func (m Message) Validate() error {
	if !m.Role.Valid() {
		return fmt.Errorf("%w: unknown role %q", ErrInvalid, m.Role)
	}
	if m.SessionID == "" {
		return fmt.Errorf("%w: message session id required", ErrInvalid)
	}
	for i, p := range m.Parts {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("parts[%d]: %w", i, err)
		}
	}
	return nil
}

// Validate sanity-checks a Part.
func (p Part) Validate() error {
	switch p.Type {
	case PartText, PartReasoning:
		return nil
	case PartTool:
		if p.ToolName == "" {
			return fmt.Errorf("%w: tool part needs name", ErrInvalid)
		}
		return nil
	case PartStepStart, PartStepFinish:
		return nil
	case "":
		return fmt.Errorf("%w: part type required", ErrInvalid)
	default:
		return fmt.Errorf("%w: unknown part type %q", ErrInvalid, p.Type)
	}
}

// RelativePath returns path relative to base, or path if not under base.
func RelativePath(base, path string) string {
	if r, err := filepath.Rel(base, path); err == nil {
		return r
	}
	return path
}

// Slugify converts an arbitrary string into a URL/filesystem-safe slug.
// Returns "session" for empty input.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "session"
	}
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if out == "" {
		return "session"
	}
	return out
}
