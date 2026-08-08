package export

import (
	"fmt"
	"io"
	"strings"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

const textRule = "================================================================"

func writeText(w io.Writer, b Bundle) error {
	t := &txtWriter{w: w}
	t.printf("a2migrate export — %s\n\n", b.Tool)

	for i := range b.Sessions {
		t.session(b.Sessions[i])
	}

	t.section("SKILLS", len(b.Skills))
	for _, s := range b.Skills {
		t.entry(s.Name, s.Description, s.Body)
	}
	t.section("COMMANDS", len(b.Commands))
	for _, c := range b.Commands {
		t.entry("/"+c.Name, c.Description, c.Body)
	}
	t.section("AGENTS", len(b.Agents))
	for _, a := range b.Agents {
		t.entry(a.Name, a.Description, a.Body)
	}
	t.section("RULES", len(b.Rules))
	for _, r := range b.Rules {
		t.entry(r.Name, strings.Join(r.Paths, ", "), r.Body)
	}
	t.section("MCP SERVERS", len(b.MCP))
	for _, s := range b.MCP {
		t.printf("  %s: %s\n", s.Name, strings.Join(s.Command, " "))
	}
	if len(b.MCP) > 0 {
		t.printf("\n")
	}
	if b.System != nil {
		t.section("SYSTEM PROMPT", 1)
		t.entry(b.System.SourcePath, "", b.System.Body)
	}
	return t.err
}

type txtWriter struct {
	w   io.Writer
	err error
}

func (t *txtWriter) printf(format string, args ...any) {
	if t.err != nil {
		return
	}
	_, t.err = fmt.Fprintf(t.w, format, args...)
}

func (t *txtWriter) section(name string, n int) {
	if n == 0 {
		return
	}
	t.printf("%s\n%s\n%s\n\n", textRule, name, textRule)
}

func (t *txtWriter) entry(name, subtitle, body string) {
	t.printf("--- %s ---\n", name)
	if subtitle != "" {
		t.printf("%s\n", subtitle)
	}
	if s := strings.TrimSpace(body); s != "" {
		t.printf("\n%s\n", s)
	}
	t.printf("\n")
}

func (t *txtWriter) session(s domain.Session) {
	title := s.Title
	if title == "" {
		title = s.OriginID
	}
	t.printf("%s\nSESSION: %s\n%s\n", textRule, mdEscapeHeading(title), textRule)
	t.field("id", firstNonEmpty(s.OriginID, s.ID))
	t.field("origin", string(s.Origin))
	t.field("project", s.ProjectDir)
	if s.IsSubagent {
		t.field("subagent of", s.ParentID)
	}
	t.field("created", stamp(s.CreatedAt))
	t.field("updated", stamp(s.UpdatedAt))
	t.field("messages", fmt.Sprintf("%d", len(s.Messages)))
	t.printf("\n")

	for _, msg := range s.Messages {
		t.message(msg)
	}
}

func (t *txtWriter) field(k, v string) {
	if v == "" {
		return
	}
	t.printf("%-12s %s\n", k+":", v)
}

func (t *txtWriter) message(msg domain.Message) {
	header := strings.ToUpper(messageLabel(msg))
	if meta := messageFooter(msg); meta != "" {
		header += "  [" + meta + "]"
	}
	t.printf("---------------- %s ----------------\n", header)

	for _, p := range msg.Parts {
		if skipPart(p) {
			continue
		}
		switch p.Type {
		case domain.PartText:
			t.printf("%s\n\n", strings.TrimRight(p.Text, "\n"))
		case domain.PartReasoning:
			t.printf("[reasoning]\n%s\n\n", indent(strings.TrimRight(p.Text, "\n")))
		case domain.PartTool:
			t.tool(p)
		}
	}
}

func (t *txtWriter) tool(p domain.Part) {
	name := p.ToolName
	if name == "" {
		name = "tool"
	}
	status := ""
	if p.ToolStatus != "" {
		status = " (" + p.ToolStatus + ")"
	}
	t.printf("[tool: %s%s]\n", name, status)
	if in := toolInputJSON(p.ToolInput); in != "" {
		t.printf("  input:\n%s\n", indent(in))
	}
	if out := strings.TrimRight(p.ToolOutput, "\n"); out != "" {
		t.printf("  output:\n%s\n", indent(out))
	}
	t.printf("\n")
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n")
}
