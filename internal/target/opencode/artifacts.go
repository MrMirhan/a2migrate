// Package opencode also contains writers for filesystem artifacts:
// skills, commands, agents, rules, and MCP config. They live alongside
// the SQLite writer because they all emit OpenCode-side state.
package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MrMirhan/a2migrate/internal/domain"
	"github.com/MrMirhan/a2migrate/internal/platform"
)

// SkillWriter writes skill markdown files into OpenCode's skill locations.
// OpenCode scans these directories, in priority order:
//  1. <cwd>/.opencode/skills/
//  2. <cwd>/.claude/skills/ (shared with Claude Code)
//  3. ~/.config/opencode/skills/
//  4. ~/.claude/skills/       (shared with Claude Code)
//
// a2migrate writes to (3) for global skills and (1) for project skills,
// leaving the (2)/(4) shared directories untouched.
type SkillWriter struct {
	Home    string
	WorkDir string
}

// NewSkillWriter constructs a writer rooted at platform paths.
func NewSkillWriter() *SkillWriter {
	return &SkillWriter{
		Home:    platform.OpenCodeConfigHome(),
		WorkDir: mustGetwd(),
	}
}

// WriteGlobal copies global skills into ~/.config/opencode/skills/.
func (w *SkillWriter) WriteGlobal(skills []domain.Skill) ([]string, error) {
	return w.writeDir(filepath.Join(w.Home, "skills"), skills, "global")
}

// WriteProject copies project skills into <cwd>/.opencode/skills/.
func (w *SkillWriter) WriteProject(skills []domain.Skill) ([]string, error) {
	return w.writeDir(filepath.Join(w.WorkDir, ".opencode", "skills"), skills, "project")
}

func (w *SkillWriter) writeDir(dir string, skills []domain.Skill, scope string) ([]string, error) {
	if len(skills) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	var written []string
	for _, s := range skills {
		out := filepath.Join(dir, sanitizeFilename(s.Name)+".md")
		body := renderFrontmatter(s.Frontmatter) + s.Body
		if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
			return written, fmt.Errorf("write %s: %w", out, err)
		}
		written = append(written, out)
	}
	return written, nil
}

// CommandWriter copies slash commands into .opencode/command/ (singular).
type CommandWriter struct {
	Home    string
	WorkDir string
}

// NewCommandWriter constructs a writer.
func NewCommandWriter() *CommandWriter {
	return &CommandWriter{
		Home:    platform.OpenCodeConfigHome(),
		WorkDir: mustGetwd(),
	}
}

// WriteGlobal writes ~/.config/opencode/command/*.md.
func (w *CommandWriter) WriteGlobal(cmds []domain.Command) ([]string, error) {
	return w.writeDir(filepath.Join(w.Home, "command"), cmds, "global")
}

// WriteProject writes <cwd>/.opencode/command/*.md.
func (w *CommandWriter) WriteProject(cmds []domain.Command) ([]string, error) {
	return w.writeDir(filepath.Join(w.WorkDir, ".opencode", "command"), cmds, "project")
}

func (w *CommandWriter) writeDir(dir string, cmds []domain.Command, scope string) ([]string, error) {
	return writeMarkdownDir(dir, cmds, scope, func(c domain.Command) map[string]any {
		fm := map[string]any{}
		if c.Name != "" {
			fm["name"] = c.Name
		}
		if c.Description != "" {
			fm["description"] = c.Description
		}
		if c.ArgumentHint != "" {
			fm["argument-hint"] = c.ArgumentHint
		}
		if len(c.AllowedTools) > 0 {
			fm["allowed-tools"] = c.AllowedTools
		}
		return fm
	})
}

// AgentWriter copies agent definitions into .opencode/agent/ (singular).
type AgentWriter struct {
	Home    string
	WorkDir string
}

// NewAgentWriter constructs a writer.
func NewAgentWriter() *AgentWriter {
	return &AgentWriter{
		Home:    platform.OpenCodeConfigHome(),
		WorkDir: mustGetwd(),
	}
}

// WriteGlobal writes ~/.config/opencode/agent/*.md.
func (w *AgentWriter) WriteGlobal(agents []domain.AgentDef) ([]string, error) {
	return w.writeDir(filepath.Join(w.Home, "agent"), agents, "global")
}

// WriteProject writes <cwd>/.opencode/agent/*.md.
func (w *AgentWriter) WriteProject(agents []domain.AgentDef) ([]string, error) {
	return w.writeDir(filepath.Join(w.WorkDir, ".opencode", "agent"), agents, "project")
}

func (w *AgentWriter) writeDir(dir string, agents []domain.AgentDef, scope string) ([]string, error) {
	return writeMarkdownDir(dir, agents, scope, func(a domain.AgentDef) map[string]any {
		fm := map[string]any{}
		if a.Name != "" {
			fm["name"] = a.Name
		}
		if a.Description != "" {
			fm["description"] = a.Description
		}
		if a.Model != "" {
			fm["model"] = a.Model
		}
		if len(a.Tools) > 0 {
			fm["tools"] = a.Tools
		}
		return fm
	})
}

// RuleWriter copies rule files into .opencode/rules/.
type RuleWriter struct {
	Home    string
	WorkDir string
}

// NewRuleWriter constructs a writer.
func NewRuleWriter() *RuleWriter {
	return &RuleWriter{
		Home:    platform.OpenCodeConfigHome(),
		WorkDir: mustGetwd(),
	}
}

// WriteGlobal writes ~/.config/opencode/rules/*.md.
func (w *RuleWriter) WriteGlobal(rules []domain.Rule) ([]string, error) {
	return w.writeDir(filepath.Join(w.Home, "rules"), rules, "global")
}

// WriteProject writes <cwd>/.opencode/rules/*.md.
func (w *RuleWriter) WriteProject(rules []domain.Rule) ([]string, error) {
	return w.writeDir(filepath.Join(w.WorkDir, ".opencode", "rules"), rules, "project")
}

func (w *RuleWriter) writeDir(dir string, rules []domain.Rule, scope string) ([]string, error) {
	return writeMarkdownDir(dir, rules, scope, func(r domain.Rule) map[string]any {
		fm := map[string]any{}
		if r.Name != "" {
			fm["name"] = r.Name
		}
		if len(r.Paths) > 0 {
			fm["paths"] = r.Paths
		}
		return fm
	})
}

// writeMarkdownDir is shared by command/agent/rule writers. It emits each
// item as a markdown file with rendered YAML frontmatter.
func writeMarkdownDir[T any](dir string, items []T, scope string, frontmatter func(T) map[string]any) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	var written []string
	for _, item := range items {
		name := extractName(item)
		if name == "" {
			continue
		}
		out := filepath.Join(dir, sanitizeFilename(name)+".md")
		body := renderFrontmatter(frontmatter(item)) + extractBody(item)
		if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
			return written, fmt.Errorf("write %s: %w", out, err)
		}
		written = append(written, out)
	}
	return written, nil
}

// MCPConfigPatch is the merge unit for the MCP writer. Each patch is
// applied atomically to the existing opencode.json file.
type MCPConfigPatch struct {
	Servers []domain.MCPServer
}

// MCPConfigWriter merges MCP server definitions into opencode.json.
type MCPConfigWriter struct {
	Path string
}

// NewMCPConfigWriter points at platform.OpenCodeConfigPath().
func NewMCPConfigWriter() *MCPConfigWriter {
	return &MCPConfigWriter{Path: platform.OpenCodeConfigPath()}
}

// SystemPromptWriter writes the top-level instructions file (AGENTS.md)
// into ~/.config/opencode/.
type SystemPromptWriter struct {
	Home string
}

// NewSystemPromptWriter points at platform defaults.
func NewSystemPromptWriter() *SystemPromptWriter {
	return &SystemPromptWriter{Home: platform.OpenCodeConfigHome()}
}

// Write emits the AGENTS.md file. Preserves source mtime when the source
// file is provided so an immediate re-run is a no-op for sync.
func (w *SystemPromptWriter) Write(p *domain.SystemPrompt) (string, error) {
	if p == nil || p.Body == "" {
		return "", nil
	}
	if err := os.MkdirAll(w.Home, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(w.Home, "AGENTS.md")
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

// Apply reads opencode.json (creating if missing), merges the supplied
// servers under mcp{}, and writes the result back via atomic rename.
func (w *MCPConfigWriter) Apply(patch MCPConfigPatch) (string, error) {
	root, err := readOCConfig(w.Path)
	if err != nil {
		return "", err
	}
	mcp, _ := root["mcp"].(map[string]any)
	if mcp == nil {
		mcp = map[string]any{}
	}
	for _, s := range patch.Servers {
		mcp[s.Name] = serverToOC(s)
	}
	root["mcp"] = mcp
	if err := os.MkdirAll(filepath.Dir(w.Path), 0o755); err != nil {
		return "", err
	}
	out, err := marshalOC(root)
	if err != nil {
		return "", err
	}
	tmp := w.Path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, w.Path); err != nil {
		return "", err
	}
	return w.Path, nil
}

// readOCConfig reads an opencode.json / opencode.jsonc file. Tolerates
// missing files (returns empty root). Strips // and /* */ comments first.
func readOCConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseOCConfig(data)
}

func parseOCConfig(data []byte) (map[string]any, error) {
	stripped := stripJSONC(string(data))
	var root map[string]any
	if err := json.Unmarshal([]byte(stripped), &root); err != nil {
		return nil, fmt.Errorf("parse opencode.json: %w", err)
	}
	return root, nil
}

func stripJSONC(s string) string {
	// Single-line comments.
	var out []byte
	inStr := false
	inLineComment := false
	inBlockComment := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inLineComment:
			if c == '\n' {
				inLineComment = false
				out = append(out, c)
			}
		case inBlockComment:
			if c == '*' && i+1 < len(s) && s[i+1] == '/' {
				inBlockComment = false
				i++
			}
		case inStr:
			out = append(out, c)
			if c == '\\' && i+1 < len(s) {
				out = append(out, s[i+1])
				i++
				continue
			}
			if c == '"' {
				inStr = false
			}
		default:
			if c == '/' && i+1 < len(s) && s[i+1] == '/' {
				inLineComment = true
				i++
				continue
			}
			if c == '/' && i+1 < len(s) && s[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
			if c == '"' {
				inStr = true
			}
			out = append(out, c)
		}
	}
	return string(out)
}

// marshalOC writes the config back as canonical JSON, indented for human
// readability.
func marshalOC(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// serverToOC renders a domain.MCPServer as the OC "mcp" sub-document shape.
func serverToOC(s domain.MCPServer) map[string]any {
	out := map[string]any{
		"type":    s.Type,
		"command": s.Command,
		"enabled": s.Enabled,
	}
	if len(s.Environment) > 0 {
		out["environment"] = s.Environment
	}
	if s.URL != "" {
		out["url"] = s.URL
	}
	if len(s.Headers) > 0 {
		out["headers"] = s.Headers
	}
	return out
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
