package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/MrMirhan/a2migrate/internal/domain"
	"github.com/MrMirhan/a2migrate/internal/interactive"
	"github.com/MrMirhan/a2migrate/internal/migrate"
	ccsrc "github.com/MrMirhan/a2migrate/internal/source/claudecode"
	ocsrc "github.com/MrMirhan/a2migrate/internal/source/opencode"
	"github.com/MrMirhan/a2migrate/internal/tools"
)

// namesOf projects a slice of artifacts down to their display names.
func namesOf[T any](items []T, name func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, name(it))
	}
	return out
}

// lister adapts a typed artifact reader into a name-only reader so every
// domain can be listed through one signature.
func lister[T any](read func() ([]T, error), name func(T) string) func() ([]string, error) {
	return func() ([]string, error) {
		items, err := read()
		if err != nil {
			return nil, err
		}
		return namesOf(items, name), nil
	}
}

func skillName(s domain.Skill) string      { return s.Name }
func commandName(c domain.Command) string  { return c.Name }
func agentName(a domain.AgentDef) string   { return a.Name }
func ruleName(r domain.Rule) string        { return r.Name }
func serverName(m domain.MCPServer) string { return m.Name }
func promptPath(p *domain.SystemPrompt) string {
	if p == nil {
		return ""
	}
	return p.SourcePath
}

// artifactListers holds one name-reader per artifact domain, per tool.
// Sessions are absent: they have their own discovery pipeline.
func artifactListers(id tools.ID) map[string]func() ([]string, error) {
	switch id {
	case toolClaudeCode:
		return map[string]func() ([]string, error){
			"skills":   lister(ccsrc.ReadGlobalSkills, skillName),
			"commands": lister(ccsrc.ReadGlobalCommands, commandName),
			"agents":   lister(ccsrc.ReadGlobalAgents, agentName),
			"rules":    lister(ccsrc.ReadGlobalRules, ruleName),
			"mcp":      lister(ccsrc.ReadGlobalMCP, serverName),
			"system":   singleton(ccsrc.ReadGlobalSystemPrompt),
		}
	case toolOpenCode:
		return map[string]func() ([]string, error){
			"skills":   lister(ocsrc.ReadGlobalSkills, skillName),
			"commands": lister(ocsrc.ReadGlobalCommands, commandName),
			"agents":   lister(ocsrc.ReadGlobalAgents, agentName),
			"rules":    lister(ocsrc.ReadGlobalRules, ruleName),
			"mcp":      lister(ocsrc.ReadGlobalMCP, serverName),
			"system":   singleton(ocsrc.ReadGlobalSystemPrompt),
		}
	default:
		return nil
	}
}

// singleton adapts a reader that returns at most one artifact.
func singleton(read func() (*domain.SystemPrompt, error)) func() ([]string, error) {
	return func() ([]string, error) {
		p, err := read()
		if err != nil || p == nil {
			return nil, err
		}
		return []string{promptPath(p)}, nil
	}
}

func newListCmd() *cobra.Command {
	var search string
	c := &cobra.Command{
		Use:               "list <tool> [domain]",
		Short:             "List what a tool has on disk",
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: completeToolThenDomain,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := resolveTool(args[0])
			if err != nil {
				return err
			}
			name := "sessions"
			if len(args) == 2 {
				name = args[1]
			}
			d, ok := domainByName(name)
			if !ok {
				return fmt.Errorf("unknown domain %q", name)
			}
			if !t.Has(d.cap) {
				return fmt.Errorf("%s does not support %s", t.DisplayName, d.name)
			}
			if d.name == "sessions" {
				return listSessions(cmd, t, search)
			}
			listers := artifactListers(t.ID)
			if listers == nil {
				return fmt.Errorf("listing %s is not implemented yet", t.DisplayName)
			}
			names, err := listers[d.name]()
			if err != nil {
				return err
			}
			if len(names) == 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "no %s found\n", d.name)
				return nil
			}
			for _, n := range names {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), n)
			}
			return nil
		},
	}
	c.Flags().StringVar(&search, "search", "", "Substring filter on session id, path, or title")
	return c
}

func listSessions(cmd *cobra.Command, t tools.Tool, search string) error {
	switch t.ID {
	case toolClaudeCode:
		m := migrate.NewSessionMigrator(migrate.Options{Search: search})
		refs, err := m.Discover(cmd.Context())
		if err != nil {
			return err
		}
		refs = m.Selected(refs)
		if len(refs) == 0 {
			cmd.Println("No sessions found.")
			return nil
		}
		for _, r := range refs {
			tag := ""
			if r.IsSubagent {
				tag = " [subagent]"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s%s\t%s\n", r.OriginID, tag, r.Worktree)
		}
		return nil
	case toolOpenCode:
		m := migrate.NewReverseMigrator(migrate.Options{From: resolveOCDB(""), Search: search})
		refs, err := m.Discover(cmd.Context())
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			cmd.Println("No sessions found.")
			return nil
		}
		for _, r := range refs {
			tag := ""
			if r.IsSubagent {
				tag = " [subagent]"
			}
			origin := r.OriginID
			if origin == "" {
				origin = "(native)"
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\torigin=%s%s\t%s\n",
				r.OCSessionID, origin, tag, r.Worktree)
		}
		return nil
	default:
		return fmt.Errorf("listing %s sessions is not implemented yet", t.DisplayName)
	}
}

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "show <tool> <id>",
		Short:             "Show one session's details",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: completeToolOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := resolveTool(args[0])
			if err != nil {
				return err
			}
			id := args[1]
			switch t.ID {
			case toolClaudeCode:
				return showClaudeCodeSession(cmd, id)
			case toolOpenCode:
				return showOpenCodeSession(cmd, id)
			default:
				return fmt.Errorf("showing %s sessions is not implemented yet", t.DisplayName)
			}
		},
	}
}

func showClaudeCodeSession(cmd *cobra.Command, id string) error {
	r := ccsrc.NewSessionReader("")
	refs, err := r.DiscoverSessions()
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref.OriginID != id {
			continue
		}
		sess, err := r.ParseSession(ref.FilePath)
		if err != nil {
			return err
		}
		printSessionDetail(cmd, sess.OriginID, sess.Title, sess.ProjectDir, sess.IsSubagent, sess.Messages)
		return nil
	}
	return fmt.Errorf("session %q not found", id)
}

func showOpenCodeSession(cmd *cobra.Command, id string) error {
	r := ocsrc.NewSessionReader(resolveOCDB(""))
	db, err := r.Open(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	refs, err := r.DiscoverSessions(cmd.Context(), db)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref.OCSessionID != id {
			continue
		}
		sess, err := r.ParseSession(cmd.Context(), db, ref)
		if err != nil {
			return err
		}
		printSessionDetail(cmd, sess.OriginID, sess.Title, sess.ProjectDir, sess.IsSubagent, sess.Messages)
		return nil
	}
	return fmt.Errorf("session %q not found", id)
}

func printSessionDetail(cmd *cobra.Command, id, title, project string, subagent bool, msgs []domain.Message) {
	n := 0
	for _, m := range msgs {
		n += len(m.Parts)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(),
		"id:        %s\ntitle:     %s\nproject:   %s\nsubagent:  %v\nmessages:  %d\nparts:     %d\n",
		id, title, project, subagent, len(msgs), n)
}

func newSelectCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "select <tool>",
		Short:             "Interactively pick sessions",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeToolOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := resolveTool(args[0])
			if err != nil {
				return err
			}
			if t.ID != toolClaudeCode {
				return fmt.Errorf("selecting %s sessions is not implemented yet", t.DisplayName)
			}
			m := migrate.NewSessionMigrator(migrate.Options{})
			refs, err := m.Discover(cmd.Context())
			if err != nil {
				return err
			}
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
			picked, err := interactive.Run(items, cmd.InOrStdin(), cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if picked == nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "non-interactive: use --include/--search instead")
				return nil
			}
			for _, it := range picked {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "selected: %s\n", it.ID)
			}
			return nil
		},
	}
}

func newVerifyCmd() *cobra.Command {
	var path string
	c := &cobra.Command{
		Use:               "verify <tool>",
		Short:             "Report what has been migrated into a tool",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeToolOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := resolveTool(args[0])
			if err != nil {
				return err
			}
			if t.ID != toolOpenCode {
				return fmt.Errorf("%s does not record migration provenance yet", t.DisplayName)
			}
			report, err := migrate.VerifyReverse(cmd.Context(), resolveOCDB(path))
			if err != nil {
				return err
			}
			printVerifyGroup(cmd, "migrated-from-claude-code", report.MigratedFromCC)
			printVerifyGroup(cmd, "native-opencode", report.Native)
			return nil
		},
	}
	c.Flags().StringVar(&path, "path", "", "Override where the tool's state is read from")
	return c
}

func newRepairCmd() *cobra.Command {
	var path string
	c := &cobra.Command{
		Use:               "repair <tool>",
		Short:             "Re-run post-migration invariants on a tool's store",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeToolOnly,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := resolveTool(args[0])
			if err != nil {
				return err
			}
			// Repair fixes OpenCode's session-schema invariants
			// (reparenting, step padding); no other target stores
			// state that can drift this way.
			if t.ID != toolOpenCode {
				return fmt.Errorf("%s has no repairable invariants", t.DisplayName)
			}
			db, err := openTargetDB(cmd.Context(), resolveOCDB(path))
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			report, err := migrateRepair(cmd.Context(), db)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"scanned=%d reparent=%d pad=%d step-start-time=%d tool-state-time=%d\n",
				report.SessionsScanned, report.Reparents, report.PadsStepParts,
				report.AddedStepStartTimes, report.AddedToolStateTimes)
			return nil
		},
	}
	c.Flags().StringVar(&path, "path", "", "Override where the tool's state is read from")
	return c
}
