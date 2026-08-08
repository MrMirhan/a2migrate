package export

import (
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

// htmlStyle is inlined rather than linked: an export is one file that
// has to render from a download folder with no network.
const htmlStyle = `
:root {
  --bg: #ffffff; --fg: #1a1a1a; --muted: #6b7280; --line: #e5e7eb;
  --code-bg: #f6f8fa; --user: #2563eb; --assistant: #7c3aed; --tool: #0f766e;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0d1117; --fg: #e6edf3; --muted: #8b949e; --line: #30363d;
    --code-bg: #161b22; --user: #58a6ff; --assistant: #bc8cff; --tool: #39c5bb;
  }
}
* { box-sizing: border-box; }
body {
  margin: 0 auto; padding: 2rem 1.25rem; max-width: 52rem;
  background: var(--bg); color: var(--fg);
  font: 16px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
}
h1 { font-size: 1.6rem; margin: 0 0 1.5rem; }
h2 { font-size: 1.25rem; margin: 2.5rem 0 1rem; padding-bottom: .35rem; border-bottom: 1px solid var(--line); }
h3 { font-size: 1.05rem; margin: 2rem 0 .75rem; }
a { color: var(--user); }
.meta { color: var(--muted); font-size: .875rem; margin: 0 0 1.25rem; }
.meta dl { display: grid; grid-template-columns: max-content 1fr; gap: .15rem .75rem; margin: 0; }
.meta dt { font-weight: 600; }
.meta dd { margin: 0; overflow-wrap: anywhere; }
.msg { border-left: 3px solid var(--line); padding: .1rem 0 .1rem 1rem; margin: 1.25rem 0; }
.msg.user { border-color: var(--user); }
.msg.assistant { border-color: var(--assistant); }
.role { font-size: .8rem; font-weight: 700; letter-spacing: .04em; text-transform: uppercase; color: var(--muted); }
.msg.user .role { color: var(--user); }
.msg.assistant .role { color: var(--assistant); }
.footer { color: var(--muted); font-size: .8rem; margin-top: .5rem; }
pre {
  background: var(--code-bg); border: 1px solid var(--line); border-radius: 6px;
  padding: .75rem; overflow-x: auto; font-size: .85rem; line-height: 1.45;
}
code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
p { white-space: pre-wrap; overflow-wrap: anywhere; margin: .6rem 0; }
details { margin: .6rem 0; border: 1px solid var(--line); border-radius: 6px; padding: .5rem .75rem; }
details summary { cursor: pointer; font-size: .85rem; color: var(--tool); font-weight: 600; }
details[open] summary { margin-bottom: .5rem; }
table { border-collapse: collapse; width: 100%; font-size: .9rem; }
th, td { text-align: left; padding: .4rem .6rem; border-bottom: 1px solid var(--line); }
hr { border: 0; border-top: 1px solid var(--line); margin: 2.5rem 0; }
`

func writeHTML(w io.Writer, b Bundle) error {
	h := &htmlWriter{w: w}
	title := "a2migrate export — " + b.Tool

	h.printf("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	h.printf("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	h.printf("<title>%s</title>\n<style>%s</style>\n</head>\n<body>\n", esc(title), htmlStyle)
	h.printf("<h1>%s</h1>\n", esc(title))

	for i := range b.Sessions {
		if i == 0 {
			h.printf("<h2>Sessions</h2>\n")
		}
		h.session(b.Sessions[i])
	}

	h.skills(b.Skills)
	h.commands(b.Commands)
	h.agents(b.Agents)
	h.rules(b.Rules)
	h.mcp(b.MCP)
	h.system(b.System)

	h.printf("</body>\n</html>\n")
	return h.err
}

type htmlWriter struct {
	w   io.Writer
	err error
}

func (h *htmlWriter) printf(format string, args ...any) {
	if h.err != nil {
		return
	}
	_, h.err = fmt.Fprintf(h.w, format, args...)
}

func (h *htmlWriter) session(s domain.Session) {
	title := s.Title
	if title == "" {
		title = s.OriginID
	}
	h.printf("<h3>%s</h3>\n", esc(mdEscapeHeading(title)))

	h.printf("<div class=\"meta\"><dl>\n")
	h.def("id", firstNonEmpty(s.OriginID, s.ID))
	h.def("origin", string(s.Origin))
	h.def("project", s.ProjectDir)
	if s.IsSubagent {
		h.def("subagent of", s.ParentID)
	}
	h.def("created", stamp(s.CreatedAt))
	h.def("updated", stamp(s.UpdatedAt))
	h.def("messages", fmt.Sprintf("%d", len(s.Messages)))
	h.printf("</dl></div>\n")

	for _, msg := range s.Messages {
		h.message(msg)
	}
	h.printf("<hr>\n")
}

func (h *htmlWriter) def(k, v string) {
	if v == "" {
		return
	}
	h.printf("<dt>%s</dt><dd>%s</dd>\n", esc(k), esc(v))
}

func (h *htmlWriter) message(msg domain.Message) {
	class := "msg"
	if role := string(msg.Role); role != "" {
		class += " " + role
	}
	h.printf("<div class=\"%s\">\n<div class=\"role\">%s</div>\n", esc(class), esc(messageLabel(msg)))

	for _, p := range msg.Parts {
		if skipPart(p) {
			continue
		}
		switch p.Type {
		case domain.PartText:
			h.printf("<p>%s</p>\n", esc(strings.TrimRight(p.Text, "\n")))
		case domain.PartReasoning:
			h.printf("<details><summary>reasoning</summary><p>%s</p></details>\n",
				esc(strings.TrimRight(p.Text, "\n")))
		case domain.PartTool:
			h.tool(p)
		}
	}

	if meta := messageFooter(msg); meta != "" {
		h.printf("<div class=\"footer\">%s</div>\n", esc(meta))
	}
	h.printf("</div>\n")
}

func (h *htmlWriter) tool(p domain.Part) {
	name := p.ToolName
	if name == "" {
		name = "tool"
	}
	summary := name
	if p.ToolStatus != "" && p.ToolStatus != "completed" {
		summary += " (" + p.ToolStatus + ")"
	}
	h.printf("<details><summary>%s</summary>\n", esc(summary))
	if in := toolInputJSON(p.ToolInput); in != "" {
		h.printf("<pre><code>%s</code></pre>\n", esc(in))
	}
	if out := strings.TrimRight(p.ToolOutput, "\n"); out != "" {
		h.printf("<pre><code>%s</code></pre>\n", esc(out))
	}
	h.printf("</details>\n")
}

func (h *htmlWriter) skills(items []domain.Skill) {
	if len(items) == 0 {
		return
	}
	h.printf("<h2>Skills</h2>\n")
	for _, s := range items {
		h.article(s.Name, s.Description, s.Body)
	}
}

func (h *htmlWriter) commands(items []domain.Command) {
	if len(items) == 0 {
		return
	}
	h.printf("<h2>Commands</h2>\n")
	for _, c := range items {
		sub := c.Description
		if c.ArgumentHint != "" {
			sub = strings.TrimSpace(sub + " — " + c.ArgumentHint)
		}
		h.article("/"+c.Name, sub, c.Body)
	}
}

func (h *htmlWriter) agents(items []domain.AgentDef) {
	if len(items) == 0 {
		return
	}
	h.printf("<h2>Agents</h2>\n")
	for _, a := range items {
		sub := a.Description
		if a.Model != "" {
			sub = strings.TrimSpace(sub + " — " + a.Model)
		}
		h.article(a.Name, sub, a.Body)
	}
}

func (h *htmlWriter) rules(items []domain.Rule) {
	if len(items) == 0 {
		return
	}
	h.printf("<h2>Rules</h2>\n")
	for _, r := range items {
		h.article(r.Name, strings.Join(r.Paths, ", "), r.Body)
	}
}

func (h *htmlWriter) mcp(items []domain.MCPServer) {
	if len(items) == 0 {
		return
	}
	h.printf("<h2>MCP servers</h2>\n<table>\n<tr><th>name</th><th>command</th></tr>\n")
	for _, s := range items {
		h.printf("<tr><td>%s</td><td><code>%s</code></td></tr>\n",
			esc(s.Name), esc(strings.Join(s.Command, " ")))
	}
	h.printf("</table>\n")
}

func (h *htmlWriter) system(p *domain.SystemPrompt) {
	if p == nil {
		return
	}
	h.printf("<h2>System prompt</h2>\n")
	h.article(p.SourcePath, "", p.Body)
}

func (h *htmlWriter) article(name, subtitle, body string) {
	h.printf("<h3>%s</h3>\n", esc(mdEscapeHeading(name)))
	if subtitle != "" {
		h.printf("<div class=\"meta\">%s</div>\n", esc(subtitle))
	}
	if s := strings.TrimSpace(body); s != "" {
		h.printf("<pre><code>%s</code></pre>\n", esc(s))
	}
}

func esc(s string) string { return html.EscapeString(s) }
