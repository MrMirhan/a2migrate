// Package claudecode also reads non-session artifacts: skills, commands,
// agents, rules, MCP config, and hooks.
package claudecode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/platform"
)

// ReadGlobalSkills returns all skills under ~/.claude/skills/*.md.
// Returns an empty slice if the directory does not exist.
func ReadGlobalSkills() ([]domain.Skill, error) {
	dir := platform.ClaudeCodeSkillsDir()
	return readMarkdownDir(dir, domain.OriginClaudeCode, true)
}

// ReadProjectSkills returns skills under ./.claude/skills/*.md.
// Returns nil (no error) if the directory does not exist.
func ReadProjectSkills(cwd string) ([]domain.Skill, error) {
	dir := filepath.Join(cwd, ".claude", "skills")
	return readMarkdownDir(dir, domain.OriginClaudeCode, true)
}

// ReadGlobalCommands returns all slash commands under ~/.claude/commands/*.md.
// Returns nil (no error) if the directory does not exist.
func ReadGlobalCommands() ([]domain.Command, error) {
	dir := filepath.Join(platform.ClaudeCodeHome(), "commands")
	entries, err := readMarkdownDir(dir, domain.OriginClaudeCode, true)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Command, 0, len(entries))
	for _, e := range entries {
		out = append(out, skillToCommand(e))
	}
	return out, nil
}

// ReadProjectCommands returns commands under ./.claude/commands/*.md.
// Returns nil if missing.
func ReadProjectCommands(cwd string) ([]domain.Command, error) {
	dir := filepath.Join(cwd, ".claude", "commands")
	entries, err := readMarkdownDir(dir, domain.OriginClaudeCode, true)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Command, 0, len(entries))
	for _, e := range entries {
		out = append(out, skillToCommand(e))
	}
	return out, nil
}

// ReadGlobalAgents returns agent definitions under ~/.claude/agents/*.md.
func ReadGlobalAgents() ([]domain.AgentDef, error) {
	dir := filepath.Join(platform.ClaudeCodeHome(), "agents")
	entries, err := readMarkdownDir(dir, domain.OriginClaudeCode, true)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentDef, 0, len(entries))
	for _, e := range entries {
		out = append(out, skillToAgent(e))
	}
	return out, nil
}

// ReadProjectAgents returns agents under ./.claude/agents/*.md.
// Returns nil if missing.
func ReadProjectAgents(cwd string) ([]domain.AgentDef, error) {
	dir := filepath.Join(cwd, ".claude", "agents")
	entries, err := readMarkdownDir(dir, domain.OriginClaudeCode, true)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentDef, 0, len(entries))
	for _, e := range entries {
		out = append(out, skillToAgent(e))
	}
	return out, nil
}

// ReadGlobalRules returns rule files under ~/.claude/rules/*.md.
// Returns nil if missing.
func ReadGlobalRules() ([]domain.Rule, error) {
	dir := filepath.Join(platform.ClaudeCodeHome(), "rules")
	entries, err := readMarkdownDir(dir, domain.OriginClaudeCode, true)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Rule, 0, len(entries))
	for _, e := range entries {
		out = append(out, skillToRule(e))
	}
	return out, nil
}

// ReadProjectRules returns rules under ./.claude/rules/*.md.
// Returns nil if missing.
func ReadProjectRules(cwd string) ([]domain.Rule, error) {
	dir := filepath.Join(cwd, ".claude", "rules")
	entries, err := readMarkdownDir(dir, domain.OriginClaudeCode, true)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Rule, 0, len(entries))
	for _, e := range entries {
		out = append(out, skillToRule(e))
	}
	return out, nil
}

// readMarkdownDir walks dir and parses every *.md file. If allowMissing is
// true, missing dirs return nil without error; otherwise they produce an error.
func readMarkdownDir(dir string, origin domain.Origin, allowMissing bool) ([]domain.Skill, error) {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		if allowMissing {
			return nil, nil
		}
		return nil, fmt.Errorf("directory not found: %s", dir)
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", dir, err)
	}
	var out []domain.Skill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		text, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", full, err)
		}
		fm, _ := ParseFrontmatter(string(text))
		name := nameFromFilename(e.Name())
		if n := fm.StringField("name"); n != "" {
			name = n
		}
		out = append(out, domain.Skill{
			Name:        name,
			Description: fm.StringField("description"),
			Origin:      origin,
			SourcePath:  full,
			Body:        fm.Body,
			Frontmatter: fm.Raw,
		})
	}
	return out, nil
}

func nameFromFilename(filename string) string {
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

func skillToCommand(s domain.Skill) domain.Command {
	return domain.Command{
		Name:         s.Name,
		Description:  s.Description,
		ArgumentHint: s.FrontmatterString("argument-hint"),
		AllowedTools: s.FrontmatterStringSlice("allowed-tools"),
		SourcePath:   s.SourcePath,
		Body:         s.Body,
		Frontmatter:  s.Frontmatter,
	}
}

func skillToAgent(s domain.Skill) domain.AgentDef {
	return domain.AgentDef{
		Name:        s.Name,
		Description: s.Description,
		Model:       s.FrontmatterString("model"),
		Tools:       s.FrontmatterStringSlice("tools"),
		SourcePath:  s.SourcePath,
		Body:        s.Body,
		Frontmatter: s.Frontmatter,
	}
}

func skillToRule(s domain.Skill) domain.Rule {
	return domain.Rule{
		Name:        s.Name,
		Paths:       s.FrontmatterStringSlice("paths"),
		SourcePath:  s.SourcePath,
		Body:        s.Body,
		Frontmatter: s.Frontmatter,
	}
}

// MCPSettings is the Claude Code ~/.claude/mcp.json shape.
type MCPSettings struct {
	MCPServers map[string]mcpServerRaw `json:"mcpServers"`
}

type mcpServerRaw struct {
	Command  any            `json:"command"`
	Args     []any          `json:"args"`
	Env      map[string]any `json:"env"`
	URL      string         `json:"url"`
	Type     string         `json:"type"`
	Enabled  *bool          `json:"enabled"`
	Headers  map[string]any `json:"headers"`
}

// ReadGlobalMCP reads ~/.claude/mcp.json and normalizes it to domain.MCPServer.
// Returns nil if the file is missing.
func ReadGlobalMCP() ([]domain.MCPServer, error) {
	path := platform.ClaudeCodeMCPPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseMCPServers(data)
}

// ParseMCPServers parses a CC mcp.json body. Tolerates either
// mcpServers{} (CC) or mcp{} (already-OC). Returns servers in alphabetical
// order for determinism.
func ParseMCPServers(data []byte) ([]domain.MCPServer, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parse mcp config: %w", err)
	}
	var blob json.RawMessage
	if v, ok := probe["mcpServers"]; ok {
		blob = v
	} else if v, ok := probe["mcp"]; ok {
		blob = v
	} else {
		return nil, nil
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(blob, &servers); err != nil {
		return nil, fmt.Errorf("parse mcp servers: %w", err)
	}
	out := make([]domain.MCPServer, 0, len(servers))
	for name, raw := range servers {
		s, err := parseOneMCPServer(name, raw)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	// Sort for determinism.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].Name > out[j].Name; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out, nil
}

func parseOneMCPServer(name string, raw json.RawMessage) (domain.MCPServer, error) {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return domain.MCPServer{}, err
	}
	s := domain.MCPServer{Name: name, Type: "local", Enabled: true}

	switch cmd := probe["command"].(type) {
	case string:
		s.Command = []string{cmd}
		if args, ok := probe["args"].([]any); ok {
			for _, a := range args {
				if str, ok := a.(string); ok {
					s.Command = append(s.Command, str)
				}
			}
		}
	case []any:
		for _, a := range cmd {
			if str, ok := a.(string); ok {
				s.Command = append(s.Command, str)
			}
		}
	}

	if env, ok := probe["env"].(map[string]any); ok {
		s.Environment = make(map[string]string, len(env))
		for k, v := range env {
			if str, ok := v.(string); ok {
				s.Environment[k] = str
			}
		}
	}
	if u, ok := probe["url"].(string); ok {
		s.URL = u
		s.Type = "remote"
	}
	if h, ok := probe["headers"].(map[string]any); ok {
		s.Headers = make(map[string]string, len(h))
		for k, v := range h {
			if str, ok := v.(string); ok {
				s.Headers[k] = str
			}
		}
	}
	if t, ok := probe["type"].(string); ok {
		s.Type = t
	}
	if en, ok := probe["enabled"].(bool); ok {
		s.Enabled = en
	}
	return s, nil
}

// ReadSettings returns the parsed ~/.claude/settings.json body.
func ReadSettings() (map[string]any, error) {
	path := platform.ClaudeCodeSettingsPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse settings.json: %w", err)
	}
	return out, nil
}

// ReadHooks extracts hooks from a parsed settings.json body.
// Returns an empty slice if no hooks are configured.
func ReadHooks(settings map[string]any) []domain.Hook {
	if settings == nil {
		return nil
	}
	rawHooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	var out []domain.Hook
	// Order matters for diff output.
	events := []string{"PreToolUse", "PostToolUse", "SessionStart", "UserPromptSubmit", "Stop"}
	for _, ev := range events {
		entries, ok := rawHooks[ev].([]any)
		if !ok {
			continue
		}
		for _, e := range entries {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			matcher, _ := entry["matcher"].(string)
			hookList, _ := entry["hooks"].([]any)
			for _, h := range hookList {
				hm, ok := h.(map[string]any)
				if !ok {
					continue
				}
				cmd, _ := hm["command"].(string)
				typ, _ := hm["type"].(string)
				timeoutF, _ := hm["timeout"].(float64)
				out = append(out, domain.Hook{
					Event:   ev,
					Matcher: matcher,
					Command: cmd,
					Type:    typ,
					Timeout: int(timeoutF),
				})
			}
		}
	}
	return out
}