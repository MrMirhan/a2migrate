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
	ID         string // OpenCode: "ses_<26b32>"; Claude Code: uuid filename
	OriginID   string // original ID in source system (e.g. CC session UUID)
	Origin     Origin
	Title      string
	Slug       string
	ProjectDir string // absolute working directory the session was opened in
	IsSubagent bool
	ParentID   string // OriginID of parent session (only set when IsSubagent)
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Messages   []Message
	Tags       []string
}

// Message is one turn envelope. Part content lives in []Part.
type Message struct {
	ID         string
	OriginID   string
	SessionID  string
	ParentID   string // OriginID of predecessor message (or "")
	Role       Role
	CreatedAt  time.Time
	FinishedAt time.Time // zero if not yet finished
	Agent      string
	ModelID    string
	ProviderID string
	Variant    string
	Parts      []Part
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
	ID         string
	OriginID   string
	Type       PartType
	Text       string         // text / reasoning
	ToolName   string         // tool parts
	ToolCallID string         // tool parts
	ToolInput  map[string]any // tool parts
	ToolOutput string         // tool parts
	ToolStatus string         // "pending" | "running" | "completed" | "error"
	CreatedAt  time.Time
}

// Skill is one Claude Code / OpenCode skill definition.
type Skill struct {
	Name        string
	Description string
	Origin      Origin
	SourcePath  string // file on disk in source system
	Body        string // markdown body (after frontmatter)
	Frontmatter map[string]any
}

// Command is one slash command (markdown + frontmatter).
type Command struct {
	Name         string
	Description  string
	ArgumentHint string
	AllowedTools []string
	SourcePath   string
	Body         string
	Frontmatter  map[string]any
}

// AgentDef is one subagent/agent definition.
type AgentDef struct {
	Name        string
	Description string
	Model       string
	Tools       []string
	SourcePath  string
	Body        string
	Frontmatter map[string]any
}

// Rule is one path-scoped or global rule.
type Rule struct {
	Name        string
	Paths       []string
	SourcePath  string
	Body        string
	Frontmatter map[string]any
}

// MCPServer describes one MCP server entry.
type MCPServer struct {
	Name        string
	Command     []string // normalized: executable + args
	Environment map[string]string
	Type        string // "local" | "remote"
	Enabled     bool
	URL         string // remote only
	Headers     map[string]string
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
