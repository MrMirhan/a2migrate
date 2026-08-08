package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MrMirhan/a2migrate/internal/domain"
	"github.com/MrMirhan/a2migrate/internal/export"
	"github.com/MrMirhan/a2migrate/internal/interactive"
	"github.com/MrMirhan/a2migrate/internal/migrate"
	ccsrc "github.com/MrMirhan/a2migrate/internal/source/claudecode"
	ocsrc "github.com/MrMirhan/a2migrate/internal/source/opencode"
	"github.com/MrMirhan/a2migrate/internal/tools"
)

type exportFlags struct {
	format     string
	output     string
	search     string
	includes   []string
	excludes   []string
	all        bool
	cwd        string
	sourcePath string
}

// hasSessionFilter reports whether the user narrowed the session set on
// the command line. When they did, the picker stays out of the way.
func (f *exportFlags) hasSessionFilter() bool {
	return f.search != "" || len(f.includes) > 0 || len(f.excludes) > 0
}

func newExportCmd() *cobra.Command {
	f := &exportFlags{}
	c := &cobra.Command{
		Use:   "export <tool> [domain...]",
		Short: "Export sessions and artifacts to a readable document",
		Long: "Renders what a tool has on disk as Markdown, JSON, HTML, or plain text.\n\n" +
			"Omit the domain arguments to export every domain the tool supports.\n" +
			"Without --output the document goes to stdout; with it, one file is\n" +
			"written per session plus one for the artifacts.\n\n" +
			"Sessions are picked interactively unless --search, --include,\n" +
			"--exclude, or --all narrows them first.",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: completeToolThenDomain,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := resolveTool(args[0])
			if err != nil {
				return err
			}
			format, err := export.ParseFormat(f.format)
			if err != nil {
				return err
			}
			domains, err := resolveDomains(args[1:], t)
			if err != nil {
				return err
			}

			bundle := export.Bundle{Tool: t.DisplayName}
			if hasDomain(domains, "sessions") {
				sessions, err := collectSessions(cmd, t, f)
				if err != nil {
					return err
				}
				bundle.Sessions = sessions
			}
			if err := collectArtifacts(t, domains, f.cwd, &bundle); err != nil {
				return err
			}
			if bundle.IsEmpty() {
				cmd.Println("Nothing to export.")
				return nil
			}

			if f.output == "" {
				return export.Write(cmd.OutOrStdout(), bundle, format)
			}
			return writeExportDir(cmd, bundle, format, f.output)
		},
	}

	fl := c.Flags()
	fl.StringVar(&f.format, "format", "md", "Output format: "+strings.Join(export.Formats(), " | "))
	fl.StringVarP(&f.output, "output", "o", "", "Write into this directory instead of stdout")
	fl.StringVar(&f.search, "search", "", "Substring filter on session id or project path (OpenCode also matches the title)")
	fl.StringSliceVar(&f.includes, "include", nil, "Only export sessions whose id matches")
	fl.StringSliceVar(&f.excludes, "exclude", nil, "Skip sessions whose id matches")
	fl.BoolVar(&f.all, "all", false, "Export every session without prompting")
	fl.StringVar(&f.cwd, "cwd", "", "Project root for project-scoped artifacts (default: current directory)")
	fl.StringVar(&f.sourcePath, "source-path", "", "Override where the tool's state is read from")

	_ = c.RegisterFlagCompletionFunc("format",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return export.Formats(), cobra.ShellCompDirectiveNoFileComp
		})
	return c
}

func collectSessions(cmd *cobra.Command, t tools.Tool, f *exportFlags) ([]domain.Session, error) {
	switch t.ID {
	case toolClaudeCode:
		return collectClaudeCodeSessions(cmd, f)
	case toolOpenCode:
		return collectOpenCodeSessions(cmd, f)
	default:
		return nil, fmt.Errorf("exporting %s sessions is not implemented yet", t.DisplayName)
	}
}

func collectClaudeCodeSessions(cmd *cobra.Command, f *exportFlags) ([]domain.Session, error) {
	m := migrate.NewSessionMigrator(migrate.Options{
		From:     f.sourcePath,
		Search:   f.search,
		Includes: f.includes,
		Excludes: f.excludes,
	})
	refs, err := m.Discover(cmd.Context())
	if err != nil {
		return nil, err
	}
	refs = m.Selected(refs)

	if !f.all && !f.hasSessionFilter() {
		ids, err := pickSessions(cmd, ccPickerItems(refs))
		if err != nil {
			return nil, err
		}
		refs = filterCCRefs(refs, ids)
	}

	r := ccsrc.NewSessionReader(f.sourcePath)
	out := make([]domain.Session, 0, len(refs))
	for _, ref := range refs {
		s, err := r.ParseSession(ref.FilePath)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", ref.OriginID, err)
		}
		out = append(out, s)
	}
	return out, nil
}

func collectOpenCodeSessions(cmd *cobra.Command, f *exportFlags) ([]domain.Session, error) {
	dbPath := resolveOCDB(f.sourcePath)
	m := migrate.NewReverseMigrator(migrate.Options{
		From:     dbPath,
		Search:   f.search,
		Includes: f.includes,
		Excludes: f.excludes,
	})
	refs, err := m.Discover(cmd.Context())
	if err != nil {
		return nil, err
	}

	if !f.all && !f.hasSessionFilter() {
		ids, err := pickSessions(cmd, ocPickerItems(refs))
		if err != nil {
			return nil, err
		}
		refs = filterOCRefs(refs, ids)
	}

	r := ocsrc.NewSessionReader(dbPath)
	db, err := r.Open(cmd.Context())
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	out := make([]domain.Session, 0, len(refs))
	for _, ref := range refs {
		s, err := r.ParseSession(cmd.Context(), db, ref)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", ref.OCSessionID, err)
		}
		out = append(out, s)
	}
	return out, nil
}

// pickSessions runs the interactive picker. A nil id set means the
// terminal cannot prompt, which is an error here rather than a silent
// export of everything — exporting a whole history by accident is worse
// than a message telling the user which flag to add.
func pickSessions(cmd *cobra.Command, items []interactive.Item) (map[string]bool, error) {
	if len(items) == 0 {
		return map[string]bool{}, nil
	}
	picked, err := interactive.Run(items, cmd.InOrStdin(), cmd.OutOrStdout())
	if err != nil {
		return nil, err
	}
	if picked == nil {
		return nil, fmt.Errorf("no terminal to prompt on; narrow with --search/--include or pass --all")
	}
	ids := make(map[string]bool, len(picked))
	for _, it := range picked {
		ids[it.ID] = true
	}
	return ids, nil
}

func ccPickerItems(refs []ccsrc.SessionRef) []interactive.Item {
	items := make([]interactive.Item, 0, len(refs))
	for _, r := range refs {
		title := r.OriginID
		if r.IsSubagent {
			title = "↳ " + r.OriginID
		}
		items = append(items, interactive.Item{
			Title:    title,
			Subtitle: r.Worktree,
			ID:       r.OriginID,
			Sub:      r.IsSubagent,
		})
	}
	return items
}

func ocPickerItems(refs []ocsrc.SessionRef) []interactive.Item {
	items := make([]interactive.Item, 0, len(refs))
	for _, r := range refs {
		title := r.Title
		if title == "" {
			title = r.OCSessionID
		}
		if r.IsSubagent {
			title = "↳ " + title
		}
		items = append(items, interactive.Item{
			Title:    title,
			Subtitle: r.Worktree,
			ID:       r.OCSessionID,
			Sub:      r.IsSubagent,
		})
	}
	return items
}

func filterCCRefs(refs []ccsrc.SessionRef, ids map[string]bool) []ccsrc.SessionRef {
	out := make([]ccsrc.SessionRef, 0, len(ids))
	for _, r := range refs {
		if ids[r.OriginID] {
			out = append(out, r)
		}
	}
	return out
}

func filterOCRefs(refs []ocsrc.SessionRef, ids map[string]bool) []ocsrc.SessionRef {
	out := make([]ocsrc.SessionRef, 0, len(ids))
	for _, r := range refs {
		if ids[r.OCSessionID] {
			out = append(out, r)
		}
	}
	return out
}

// artifactReaders returns one reader per artifact domain for a tool. The
// two tools expose the same six readers, so the only thing that varies
// is which package they come from.
type artifactReaders struct {
	skills   func() ([]domain.Skill, error)
	commands func() ([]domain.Command, error)
	agents   func() ([]domain.AgentDef, error)
	rules    func() ([]domain.Rule, error)
	mcp      func() ([]domain.MCPServer, error)
	system   func() (*domain.SystemPrompt, error)
}

func readersFor(id tools.ID) (artifactReaders, bool) {
	switch id {
	case toolClaudeCode:
		return artifactReaders{
			skills:   ccsrc.ReadGlobalSkills,
			commands: ccsrc.ReadGlobalCommands,
			agents:   ccsrc.ReadGlobalAgents,
			rules:    ccsrc.ReadGlobalRules,
			mcp:      ccsrc.ReadGlobalMCP,
			system:   ccsrc.ReadGlobalSystemPrompt,
		}, true
	case toolOpenCode:
		return artifactReaders{
			skills:   ocsrc.ReadGlobalSkills,
			commands: ocsrc.ReadGlobalCommands,
			agents:   ocsrc.ReadGlobalAgents,
			rules:    ocsrc.ReadGlobalRules,
			mcp:      ocsrc.ReadGlobalMCP,
			system:   ocsrc.ReadGlobalSystemPrompt,
		}, true
	default:
		return artifactReaders{}, false
	}
}

func collectArtifacts(t tools.Tool, domains []domainSpec, cwd string, b *export.Bundle) error {
	wanted := make([]string, 0, len(domains))
	for _, d := range domains {
		if d.name != "sessions" {
			wanted = append(wanted, d.name)
		}
	}
	if len(wanted) == 0 {
		return nil
	}

	r, ok := readersFor(t.ID)
	if !ok {
		return fmt.Errorf("exporting %s artifacts is not implemented yet", t.DisplayName)
	}

	for _, name := range wanted {
		var err error
		switch name {
		case "skills":
			b.Skills, err = r.skills()
		case "commands":
			b.Commands, err = r.commands()
		case "agents":
			b.Agents, err = r.agents()
		case "rules":
			b.Rules, err = r.rules()
		case "mcp":
			b.MCP, err = r.mcp()
		case "system":
			b.System, err = r.system()
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
	}
	return nil
}

// writeExportDir splits the bundle across files: one per session, plus
// one holding whatever artifacts were requested.
func writeExportDir(cmd *cobra.Command, b export.Bundle, format export.Format, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	for _, s := range b.Sessions {
		name := sessionFileName(s, format)
		path := filepath.Join(dir, name)
		one := export.Bundle{Tool: b.Tool, Sessions: []domain.Session{s}}
		if err := writeExportFile(path, one, format); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
	}

	artifacts := b
	artifacts.Sessions = nil
	if artifacts.IsEmpty() {
		return nil
	}
	path := filepath.Join(dir, "artifacts."+format.Ext())
	if err := writeExportFile(path, artifacts, format); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
	return nil
}

func writeExportFile(path string, b export.Bundle, format export.Format) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := export.Write(f, b, format); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// sessionFileName builds a filename that survives every target
// filesystem: session ids are safe already, but titles are not, so only
// the id is used.
func sessionFileName(s domain.Session, format export.Format) string {
	id := firstNonEmptyString(s.OriginID, s.ID, "session")
	return safeFileStem(id) + "." + format.Ext()
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func safeFileStem(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "session"
	}
	return out
}
