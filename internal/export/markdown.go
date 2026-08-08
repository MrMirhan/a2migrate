package export

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

func writeMarkdown(w io.Writer, b Bundle) error {
	m := &mdWriter{w: w}
	m.printf("# a2migrate export — %s\n\n", b.Tool)

	for i := range b.Sessions {
		if i == 0 {
			m.printf("## Sessions\n\n")
		}
		m.session(b.Sessions[i])
	}

	m.skills(b.Skills)
	m.commands(b.Commands)
	m.agents(b.Agents)
	m.rules(b.Rules)
	m.mcp(b.MCP)
	m.system(b.System)
	return m.err
}

// mdWriter accumulates the first write error so the renderer body stays
// free of error plumbing.
type mdWriter struct {
	w   io.Writer
	err error
}

func (m *mdWriter) printf(format string, args ...any) {
	if m.err != nil {
		return
	}
	_, m.err = fmt.Fprintf(m.w, format, args...)
}

func (m *mdWriter) session(s domain.Session) {
	title := s.Title
	if title == "" {
		title = s.OriginID
	}
	m.printf("### %s\n\n", mdEscapeHeading(title))

	m.printf("| field | value |\n|---|---|\n")
	m.row("id", firstNonEmpty(s.OriginID, s.ID))
	m.row("origin", string(s.Origin))
	m.row("project", s.ProjectDir)
	if s.IsSubagent {
		m.row("subagent of", s.ParentID)
	}
	m.row("created", stamp(s.CreatedAt))
	m.row("updated", stamp(s.UpdatedAt))
	m.row("messages", fmt.Sprintf("%d", len(s.Messages)))
	m.printf("\n")

	for _, msg := range s.Messages {
		m.message(msg)
	}
	m.printf("---\n\n")
}

func (m *mdWriter) row(k, v string) {
	if v == "" {
		return
	}
	m.printf("| %s | %s |\n", k, mdEscapeCell(v))
}

func (m *mdWriter) message(msg domain.Message) {
	m.printf("#### %s\n\n", messageLabel(msg))

	for _, p := range msg.Parts {
		if skipPart(p) {
			continue
		}
		switch p.Type {
		case domain.PartText:
			m.printf("%s\n\n", strings.TrimRight(p.Text, "\n"))
		case domain.PartReasoning:
			m.printf("<details>\n<summary>reasoning</summary>\n\n%s\n\n</details>\n\n",
				strings.TrimRight(p.Text, "\n"))
		case domain.PartTool:
			m.tool(p)
		}
	}

	if meta := messageFooter(msg); meta != "" {
		m.printf("_%s_\n\n", meta)
	}
}

func (m *mdWriter) tool(p domain.Part) {
	name := p.ToolName
	if name == "" {
		name = "tool"
	}
	summary := name
	if p.ToolStatus != "" && p.ToolStatus != "completed" {
		summary += " (" + p.ToolStatus + ")"
	}
	m.printf("<details>\n<summary>%s</summary>\n\n", summary)
	if in := toolInputJSON(p.ToolInput); in != "" {
		m.printf("input\n\n```json\n%s\n```\n\n", in)
	}
	if out := strings.TrimRight(p.ToolOutput, "\n"); out != "" {
		m.printf("output\n\n```\n%s\n```\n\n", fenceSafe(out))
	}
	m.printf("</details>\n\n")
}

func (m *mdWriter) skills(items []domain.Skill) {
	if len(items) == 0 {
		return
	}
	m.printf("## Skills\n\n")
	for _, s := range items {
		m.printf("### %s\n\n", mdEscapeHeading(s.Name))
		if s.Description != "" {
			m.printf("%s\n\n", s.Description)
		}
		m.body(s.Body)
	}
}

func (m *mdWriter) commands(items []domain.Command) {
	if len(items) == 0 {
		return
	}
	m.printf("## Commands\n\n")
	for _, c := range items {
		m.printf("### /%s\n\n", mdEscapeHeading(c.Name))
		if c.Description != "" {
			m.printf("%s\n\n", c.Description)
		}
		if c.ArgumentHint != "" {
			m.printf("- arguments: `%s`\n", c.ArgumentHint)
		}
		if len(c.AllowedTools) > 0 {
			m.printf("- allowed tools: %s\n", strings.Join(c.AllowedTools, ", "))
		}
		m.printf("\n")
		m.body(c.Body)
	}
}

func (m *mdWriter) agents(items []domain.AgentDef) {
	if len(items) == 0 {
		return
	}
	m.printf("## Agents\n\n")
	for _, a := range items {
		m.printf("### %s\n\n", mdEscapeHeading(a.Name))
		if a.Description != "" {
			m.printf("%s\n\n", a.Description)
		}
		if a.Model != "" {
			m.printf("- model: `%s`\n", a.Model)
		}
		if len(a.Tools) > 0 {
			m.printf("- tools: %s\n", strings.Join(a.Tools, ", "))
		}
		m.printf("\n")
		m.body(a.Body)
	}
}

func (m *mdWriter) rules(items []domain.Rule) {
	if len(items) == 0 {
		return
	}
	m.printf("## Rules\n\n")
	for _, r := range items {
		m.printf("### %s\n\n", mdEscapeHeading(r.Name))
		if len(r.Paths) > 0 {
			m.printf("- paths: %s\n\n", strings.Join(r.Paths, ", "))
		}
		m.body(r.Body)
	}
}

func (m *mdWriter) mcp(items []domain.MCPServer) {
	if len(items) == 0 {
		return
	}
	m.printf("## MCP servers\n\n")
	m.printf("| name | command |\n|---|---|\n")
	for _, s := range items {
		m.printf("| %s | `%s` |\n", mdEscapeCell(s.Name), strings.Join(s.Command, " "))
	}
	m.printf("\n")
}

func (m *mdWriter) system(p *domain.SystemPrompt) {
	if p == nil {
		return
	}
	m.printf("## System prompt\n\n")
	if p.SourcePath != "" {
		m.printf("_%s_\n\n", p.SourcePath)
	}
	m.body(p.Body)
}

func (m *mdWriter) body(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	m.printf("%s\n\n", demoteHeadings(s, artifactHeadingDepth))
}

// artifactHeadingDepth is how far an embedded body's headings are pushed
// down. Artifacts are titled at level 3, so their own "# Title" lands at
// level 4 and cannot be mistaken for an export section.
const artifactHeadingDepth = 3

// demoteHeadings rewrites ATX headings to sit below the export's own
// structure. Headings inside fenced code blocks are left alone: a shell
// comment is not a heading.
func demoteHeadings(s string, by int) string {
	const maxLevel = 6
	lines := strings.Split(s, "\n")
	fenced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || !strings.HasPrefix(line, "#") {
			continue
		}
		n := 0
		for n < len(line) && line[n] == '#' {
			n++
		}
		// "#tag" is not a heading; a heading needs whitespace after the runs.
		if n >= len(line) || (line[n] != ' ' && line[n] != '\t') {
			continue
		}
		level := min(n+by, maxLevel)
		lines[i] = strings.Repeat("#", level) + line[n:]
	}
	return strings.Join(lines, "\n")
}

// messageFooter renders the model and usage line shown under a turn.
func messageFooter(msg domain.Message) string {
	parts := make([]string, 0, 3)
	if model := modelLabel(msg); model != "" {
		parts = append(parts, model)
	}
	if tok := tokenLabel(msg.Tokens); tok != "" {
		parts = append(parts, tok)
	}
	if msg.CostUSD > 0 {
		parts = append(parts, fmt.Sprintf("$%.4f", msg.CostUSD))
	}
	return strings.Join(parts, " · ")
}

func toolInputJSON(in map[string]any) string {
	if len(in) == 0 {
		return ""
	}
	b, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// fenceSafe keeps tool output from escaping its code fence. Content with
// a ``` run of its own would otherwise terminate the block early.
func fenceSafe(s string) string {
	if !strings.Contains(s, "```") {
		return s
	}
	return strings.ReplaceAll(s, "```", "'''")
}

// mdEscapeHeading keeps a heading on one line; newlines inside a title
// would split it into stray paragraphs.
func mdEscapeHeading(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
}

// mdEscapeCell keeps a value inside its table cell.
func mdEscapeCell(s string) string {
	s = mdEscapeHeading(s)
	return strings.ReplaceAll(s, "|", "\\|")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
