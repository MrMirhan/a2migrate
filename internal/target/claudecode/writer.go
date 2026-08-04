// Package claudecode writes Claude Code state on disk. It is the
// target-side counterpart to internal/source/claudecode.
//
// Two responsibilities live here:
//
//	SessionWriter — emit one JSONL file per session (main + subagents)
//	                with the line-discriminated envelope CC expects.
//	MCPServer     — render domain.MCPServer as the mcpServers{} JSON.
//	Skill/Command/Agent/Rule — emit markdown files into the canonical
//	                CC directories with frontmatter round-tripped.
//
// Nothing in this package reads from disk for input.
package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/platform"
)

// SessionWriter emits JSONL files into a CC projects/ directory.
type SessionWriter struct {
	CCHome string
}

// NewSessionWriter returns a writer rooted at the platform default CC home.
func NewSessionWriter(ccHome string) *SessionWriter {
	if ccHome == "" {
		ccHome = platform.ClaudeCodeHome()
	}
	return &SessionWriter{CCHome: ccHome}
}

// WriteSession emits a single domain.Session to disk. The file path is
// computed from ProjectDir (encoded) and OriginID. If OriginID is empty
// (native OC session), a fresh ulid-style id is generated and returned
// via the OriginID field of the written entry.
//
// parentOriginID is the parent session's OriginID (CC id), used as the
// enclosing directory name when this session is a subagent. Empty for
// main sessions.
func (w *SessionWriter) WriteSession(sess domain.Session, parentOriginID string) (string, error) {
	if sess.ProjectDir == "" {
		return "", fmt.Errorf("session %s: missing project dir", sess.OriginID)
	}
	encoded := platform.EncodeCWD(sess.ProjectDir)
	dir := filepath.Join(w.CCHome, "projects", encoded)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	origin := sess.OriginID
	if origin == "" {
		origin = generateSessionID()
		sess.OriginID = origin
	}

	var sessionID string
	var out string
	if sess.IsSubagent {
		parentDir := parentOriginID
		if parentDir == "" {
			parentDir = origin
		}
		subDir := filepath.Join(dir, parentDir, "subagents")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			return "", err
		}
		sessionID = parentDir
		// Don't double-prefix if origin already starts with "agent-".
		filename := origin
		if !strings.HasPrefix(filename, "agent-") {
			filename = "agent-" + filename
		}
		out = filepath.Join(subDir, filename+".jsonl")
	} else {
		sessionID = origin
		out = filepath.Join(dir, origin+".jsonl")
	}

	if err := os.WriteFile(out, nil, 0o644); err != nil {
		return "", err
	}
	if err := appendJSONL(out, buildJSONL(sess, sessionID, parentOriginID)); err != nil {
		return "", err
	}
	return out, nil
}

// buildJSONL renders the per-line JSON entries that make up a CC session.
func buildJSONL(sess domain.Session, sessionID string, parentOCID string) []byte {
	var b strings.Builder
	if sessionID == "" {
		sessionID = sess.OriginID
	}

	// CC session metadata: emit an ai-title entry first so the title is
	// captured even before any user message.
	if sess.Title != "" {
		writeJSON(&b, map[string]any{
			"type":      "ai-title",
			"title":     sess.Title,
			"timestamp": formatTime(sess.CreatedAt),
			"sessionId": sessionID,
		})
	}

	// Walk messages in order, emitting one JSONL record per CC entry.
	var prevMsgID string
	for i := range sess.Messages {
		m := &sess.Messages[i]
		msgID := generateMessageID()
		ts := formatTime(m.CreatedAt)
		switch m.Role {
		case domain.RoleUser:
			text := textFromUserParts(m.Parts)
			writeJSON(&b, map[string]any{
				"type":      "user",
				"uuid":      msgID,
				"parentUuid": pickParent(prevMsgID),
				"sessionId":  sessionID,
				"timestamp":  ts,
				"cwd":        pickCWD(m, sess),
				"message":    map[string]any{"role": "user", "content": text},
			})
		case domain.RoleAssistant:
			content := blocksFromAssistantParts(m.Parts)
			if len(content) == 0 {
				continue
			}
			msgPayload := map[string]any{"role": "assistant", "content": content}
			if !m.Tokens.IsZero() {
				msgPayload["usage"] = tokensToUsage(m.Tokens)
			}
			entry := map[string]any{
				"type":       "assistant",
				"uuid":       msgID,
				"parentUuid": pickParent(prevMsgID),
				"sessionId":  sessionID,
				"timestamp":  ts,
				"cwd":        pickCWD(m, sess),
				"message":    msgPayload,
				"requestId":  generateMessageID(),
				"userType":   "external",
			}
			if m.CostUSD > 0 {
				entry["cost_usd"] = m.CostUSD
			}
			if m.FinishedAt.IsZero() == false {
				entry["completedAt"] = formatTime(m.FinishedAt)
			}
			if m.ModelID != "" {
				entry["model"] = m.ModelID
			}
			writeJSON(&b, entry)
		default:
			continue
		}
		prevMsgID = msgID
	}

	// If this is a subagent, leave a `bridge-session` record so CC can
	// resolve the link back to the parent session.
	if sess.IsSubagent && parentOCID != "" {
		writeJSON(&b, map[string]any{
			"type":       "bridge-session",
			"timestamp":  formatTime(sess.CreatedAt),
			"sessionId":  sessionID,
			"parentOcid": parentOCID,
		})
	}
	return []byte(b.String())
}

func pickParent(prevMsgID string) any {
	if prevMsgID == "" {
		return nil
	}
	return prevMsgID
}

func pickCWD(m *domain.Message, sess domain.Session) string {
	// OC messages don't carry per-message cwd; fall back to session.
	return sess.ProjectDir
}

func textFromUserParts(parts []domain.Part) string {
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == domain.PartText {
			sb.WriteString(p.Text)
			sb.WriteString("\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// tokensToUsage renders an OC token block in the Anthropic-style usage
// shape CC expects. The inverse of the parse-side FromUsage. We add a
// `reasoning_tokens` slot because OC carries it; CC will silently
// ignore it.
func tokensToUsage(t domain.Tokens) map[string]any {
	out := map[string]any{
		"input_tokens":                t.Input,
		"output_tokens":               t.Output,
		"cache_creation_input_tokens": t.CacheWrite,
		"cache_read_input_tokens":     t.CacheRead,
		"reasoning_tokens":            t.Reasoning,
	}
	if t.ServiceTier != "" {
		out["service_tier"] = t.ServiceTier
	}
	if t.Speed != "" {
		out["speed"] = t.Speed
	}
	return out
}

func blocksFromAssistantParts(parts []domain.Part) []map[string]any {
	var out []map[string]any
	for _, p := range parts {
		switch p.Type {
		case domain.PartText:
			out = append(out, map[string]any{"type": "text", "text": p.Text})
		case domain.PartReasoning:
			out = append(out, map[string]any{"type": "thinking", "thinking": p.Text})
		case domain.PartTool:
			out = append(out, map[string]any{
				"type":  "tool_use",
				"id":    p.ToolCallID,
				"name":  p.ToolName,
				"input": p.ToolInput,
			})
			if p.ToolOutput != "" || p.ToolStatus == "error" {
				out = append(out, map[string]any{
					"type":       "tool_result",
					"tool_use_id": p.ToolCallID,
					"content":    p.ToolOutput,
					"is_error":   p.ToolStatus == "error",
				})
			}
		}
	}
	return out
}

// writeJSON encodes one entry + newline into the buffer.
func writeJSON(b *strings.Builder, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	b.Write(data)
	b.WriteByte('\n')
}

// appendJSONL opens path in append mode and writes bytes. If path doesn't
// exist, it is created.
func appendJSONL(path string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

// generateSessionID returns a deterministic-ish id seeded by time.Now().
// We rely on unix nanos so collisions within one process are negligible.
var sessionCounter int64

func generateSessionID() string {
	sessionCounter++
	return fmt.Sprintf("oc-%016x", platform.NowUnixNanos(sessionCounter))
}

// generateMessageID returns a unique id for one CC message line.
func generateMessageID() string {
	sessionCounter++
	return fmt.Sprintf("m-%016x", platform.NowUnixNanos(sessionCounter))
}

func formatTime(t interface{}) string {
	// t is either time.Time or zero; rely on platform helper for formatting.
	if t == nil {
		return ""
	}
	if ts, ok := t.(interface{ UnixNano() int64 }); ok {
		return platform.FormatUnixMillis(ts.UnixNano() / 1e6)
	}
	return ""
}

// MCPConfigWriter writes ~/.claude/mcp.json from domain.MCPServer values.
type MCPConfigWriter struct {
	Path string
}

// NewMCPConfigWriter points at platform.ClaudeCodeMCPPath().
func NewMCPConfigWriter() *MCPConfigWriter {
	return &MCPConfigWriter{Path: platform.ClaudeCodeMCPPath()}
}

// Apply writes the given servers as the mcpServers{} block, replacing any
// existing entries with the same name.
func (w *MCPConfigWriter) Apply(servers []domain.MCPServer) (string, error) {
	root, err := readMCPConfig(w.Path)
	if err != nil {
		return "", err
	}
	servers_map, _ := root["mcpServers"].(map[string]any)
	if servers_map == nil {
		servers_map = map[string]any{}
	}
	for _, s := range servers {
		entry := map[string]any{
			"type": s.Type,
			"env":  s.Environment,
		}
		if len(s.Command) > 0 {
			entry["command"] = s.Command[0]
			if len(s.Command) > 1 {
				entry["args"] = s.Command[1:]
			}
		}
		if s.URL != "" {
			entry["url"] = s.URL
			entry["type"] = "remote"
		}
		if len(s.Headers) > 0 {
			entry["headers"] = s.Headers
		}
		servers_map[s.Name] = entry
	}
	root["mcpServers"] = servers_map
	if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(w.Path, out, 0o644); err != nil {
		return "", err
	}
	return w.Path, nil
}

func readMCPConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse mcp.json: %w", err)
	}
	return root, nil
}

// SkillWriter copies OC skill markdown files into ~/.claude/skills/ or
// <cwd>/.claude/skills/. It strips OC-only fields from frontmatter that
// CC wouldn't recognize (paths, allowed-tools, argument-hint) only when
// they don't make sense — actually CC accepts most YAML keys, so we just
// copy as-is.
type SkillWriter struct {
	Home    string
	WorkDir string
}

// SystemPromptWriter writes the top-level CLAUDE.md instructions file.
type SystemPromptWriter struct {
	Home string
}

// NewSystemPromptWriter points at platform.ClaudeCodeHome().
func NewSystemPromptWriter() *SystemPromptWriter {
	return &SystemPromptWriter{Home: platform.ClaudeCodeHome()}
}

// Write emits ~/.claude/CLAUDE.md. Preserves source mtime when the
// source file is provided so an immediate re-run is a no-op for sync.
func (w *SystemPromptWriter) Write(p *domain.SystemPrompt) (string, error) {
	if p == nil || p.Body == "" {
		return "", nil
	}
	if err := os.MkdirAll(w.Home, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(w.Home, "CLAUDE.md")
	if err := os.WriteFile(out, []byte(p.Body), 0o644); err != nil {
		return "", err
	}
	if p.SourcePath != "" {
		if info, err := os.Stat(p.SourcePath); err == nil {
			_ = os.Chtimes(out, info.ModTime(), info.ModTime())
		}
	}
	return out, nil
}

// NewSkillWriter points at the platform defaults.
func NewSkillWriter() *SkillWriter {
	return &SkillWriter{
		Home:    platform.ClaudeCodeHome(),
		WorkDir: mustGetwd(),
	}
}

// WriteGlobal writes ~/.claude/skills/*.md.
func (w *SkillWriter) WriteGlobal(skills []domain.Skill) ([]string, error) {
	return writeMarkdownDir(filepath.Join(w.Home, "skills"), skills)
}

// WriteProject writes <cwd>/.claude/skills/*.md.
func (w *SkillWriter) WriteProject(skills []domain.Skill) ([]string, error) {
	return writeMarkdownDir(filepath.Join(w.WorkDir, ".claude", "skills"), skills)
}

// CommandWriter copies OC slash commands into .claude/commands/ (plural).
type CommandWriter struct {
	Home    string
	WorkDir string
}

// NewCommandWriter points at platform defaults.
func NewCommandWriter() *CommandWriter {
	return &CommandWriter{
		Home:    platform.ClaudeCodeHome(),
		WorkDir: mustGetwd(),
	}
}

// WriteGlobal writes ~/.claude/commands/*.md.
func (w *CommandWriter) WriteGlobal(cmds []domain.Command) ([]string, error) {
	return writeMarkdownDir(filepath.Join(w.Home, "commands"), cmds)
}

// WriteProject writes <cwd>/.claude/commands/*.md.
func (w *CommandWriter) WriteProject(cmds []domain.Command) ([]string, error) {
	return writeMarkdownDir(filepath.Join(w.WorkDir, ".claude", "commands"), cmds)
}

// AgentWriter copies OC agent files into .claude/agents/ (plural).
type AgentWriter struct {
	Home    string
	WorkDir string
}

// NewAgentWriter points at platform defaults.
func NewAgentWriter() *AgentWriter {
	return &AgentWriter{
		Home:    platform.ClaudeCodeHome(),
		WorkDir: mustGetwd(),
	}
}

// WriteGlobal writes ~/.claude/agents/*.md.
func (w *AgentWriter) WriteGlobal(agents []domain.AgentDef) ([]string, error) {
	return writeMarkdownDir(filepath.Join(w.Home, "agents"), agents)
}

// WriteProject writes <cwd>/.claude/agents/*.md.
func (w *AgentWriter) WriteProject(agents []domain.AgentDef) ([]string, error) {
	return writeMarkdownDir(filepath.Join(w.WorkDir, ".claude", "agents"), agents)
}

// RuleWriter copies OC rule files into .claude/rules/.
type RuleWriter struct {
	Home    string
	WorkDir string
}

// NewRuleWriter points at platform defaults.
func NewRuleWriter() *RuleWriter {
	return &RuleWriter{
		Home:    platform.ClaudeCodeHome(),
		WorkDir: mustGetwd(),
	}
}

// WriteGlobal writes ~/.claude/rules/*.md.
func (w *RuleWriter) WriteGlobal(rules []domain.Rule) ([]string, error) {
	return writeMarkdownDir(filepath.Join(w.Home, "rules"), rules)
}

// WriteProject writes <cwd>/.claude/rules/*.md.
func (w *RuleWriter) WriteProject(rules []domain.Rule) ([]string, error) {
	return writeMarkdownDir(filepath.Join(w.WorkDir, ".claude", "rules"), rules)
}

// writeMarkdownDir emits one markdown file per item, using the item's
// Name as the filename (sanitized).
func writeMarkdownDir(dir string, items any) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var written []string
	switch xs := items.(type) {
	case []domain.Skill:
		for _, it := range xs {
			w, err := writeOneMarkdown(dir, it.Name, it.Frontmatter, it.Body)
			if err != nil {
				return written, err
			}
			written = append(written, w)
		}
	case []domain.Command:
		for _, it := range xs {
			w, err := writeOneMarkdown(dir, it.Name, it.Frontmatter, it.Body)
			if err != nil {
				return written, err
			}
			written = append(written, w)
		}
	case []domain.AgentDef:
		for _, it := range xs {
			w, err := writeOneMarkdown(dir, it.Name, it.Frontmatter, it.Body)
			if err != nil {
				return written, err
			}
			written = append(written, w)
		}
	case []domain.Rule:
		for _, it := range xs {
			w, err := writeOneMarkdown(dir, it.Name, it.Frontmatter, it.Body)
			if err != nil {
				return written, err
			}
			written = append(written, w)
		}
	default:
		return nil, fmt.Errorf("unsupported item type %T", items)
	}
	return written, nil
}

func writeOneMarkdown(dir, name string, fm map[string]any, body string) (string, error) {
	out := filepath.Join(dir, sanitizeFilenameCC(name)+".md")
	var sb strings.Builder
	if len(fm) > 0 {
		sb.WriteString("---\n")
		keys := sortedKeysCC(fm)
		for _, k := range keys {
			v := fm[k]
			switch tv := v.(type) {
			case string:
				fmt.Fprintf(&sb, "%s: %q\n", k, tv)
			case []string:
				fmt.Fprintf(&sb, "%s: %s\n", k, renderSliceCC(tv))
			default:
				jb, _ := json.Marshal(v)
				fmt.Fprintf(&sb, "%s: %s\n", k, string(jb))
			}
		}
		sb.WriteString("---\n")
	}
	sb.WriteString(body)
	if err := os.WriteFile(out, []byte(sb.String()), 0o644); err != nil {
		return "", err
	}
	return out, nil
}

func sortedKeysCC(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

func renderSliceCC(xs []string) string {
	if len(xs) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteString("[")
	for i, s := range xs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", s)
	}
	b.WriteString("]")
	return b.String()
}

func sanitizeFilenameCC(s string) string {
	if s == "" {
		return "untitled"
	}
	bad := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", " "}
	out := s
	for _, c := range bad {
		out = strings.ReplaceAll(out, c, "-")
	}
	out = strings.Trim(out, "-")
	if out == "" {
		return "untitled"
	}
	return strings.ToLower(out)
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}