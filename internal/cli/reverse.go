package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mirhan/a2migrate/internal/migrate"
	"github.com/mirhan/a2migrate/internal/platform"
	ocsrc "github.com/mirhan/a2migrate/internal/source/opencode"
	cctgt "github.com/mirhan/a2migrate/internal/target/claudecode"
)

// newOCSessionsCmd wires the OC-side sessions subcommand tree.
func newOCSessionsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "oc-sessions",
		Short: "Discover, list, and migrate OpenCode sessions back to Claude Code",
	}
	c.AddCommand(newOCSessionsListCmd())
	c.AddCommand(newOCSessionsShowCmd())
	c.AddCommand(newOCSessionsMigrateCmd())
	c.AddCommand(newOCSessionsVerifyCmd())
	return c
}

func newOCSessionsListCmd() *cobra.Command {
	var (
		from       string
		search     string
		skipNative bool
	)
	c := &cobra.Command{
		Use:   "list",
		Short: "List discovered OpenCode sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := migrate.ReverseOptions{From: resolveOCDB(from), Search: search, SkipNative: skipNative}
			m := migrate.NewReverseMigrator(opts)
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
		},
	}
	f := c.Flags()
	f.StringVar(&from, "from", "", "Override OpenCode database path")
	f.StringVar(&search, "search", "", "Substring filter on id or title")
	f.BoolVar(&skipNative, "skip-native", false, "Skip sessions not migrated from Claude Code")
	return c
}

func newOCSessionsShowCmd() *cobra.Command {
	var from string
	c := &cobra.Command{
		Use:   "show <oc-id>",
		Short: "Show one OpenCode session's details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := ocsrc.NewSessionReader(resolveOCDB(from))
			db, err := r.Open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			refs, err := r.DiscoverSessions(cmd.Context(), db)
			if err != nil {
				return err
			}
			id := args[0]
			for _, ref := range refs {
				if ref.OCSessionID != id {
					continue
				}
				sess, err := r.ParseSession(cmd.Context(), db, ref)
				if err != nil {
					return err
				}
				n := 0
				for _, m := range sess.Messages {
					n += len(m.Parts)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"id:        %s\ntitle:     %s\nproject:   %s\nsubagent:  %v\nmessages:  %d\nparts:     %d\n",
					sess.OriginID, sess.Title, sess.ProjectDir, sess.IsSubagent,
					len(sess.Messages), n)
				return nil
			}
			return fmt.Errorf("session %q not found", id)
		},
	}
	c.Flags().StringVar(&from, "from", "", "Override OpenCode database path")
	return c
}

func newOCSessionsMigrateCmd() *cobra.Command {
	var (
		dryRun     bool
		yes        bool
		from, to   string
		includes   []string
		excludes   []string
		search     string
		skipNative bool
	)
	c := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate OpenCode sessions back to Claude Code JSONL",
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := migrate.ReverseOptions{
				From:       resolveOCDB(from),
				To:         resolveCCHome(to),
				DryRun:     dryRun,
				Yes:        yes,
				Includes:   includes,
				Excludes:   excludes,
				Search:     search,
				SkipNative: skipNative,
			}
			m := migrate.NewReverseMigrator(opts)
			refs, err := m.Discover(cmd.Context())
			if err != nil {
				return err
			}
			report, err := m.Run(cmd.Context(), refs)
			if err != nil {
				return err
			}
			printReverseReport(cmd, report)
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only; do not write to disk")
	f.BoolVar(&yes, "yes", false, "Skip confirmation prompts")
	f.StringVar(&from, "from", "", "Override OpenCode database path")
	f.StringVar(&to, "to", "", "Override Claude Code home")
	f.StringSliceVar(&includes, "include", nil, "Only migrate sessions whose OC id matches")
	f.StringSliceVar(&excludes, "exclude", nil, "Skip sessions whose OC id matches")
	f.StringVar(&search, "search", "", "Substring filter")
	f.BoolVar(&skipNative, "skip-native", false, "Skip sessions not migrated from Claude Code")
	return c
}

func newOCSessionsVerifyCmd() *cobra.Command {
	var from string
	c := &cobra.Command{
		Use:   "verify",
		Short: "List what's in the OpenCode database",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := migrate.VerifyReverse(cmd.Context(), resolveOCDB(from))
			if err != nil {
				return err
			}
			printVerifyGroup(cmd, "migrated-from-claude-code", report.MigratedFromCC)
			printVerifyGroup(cmd, "native-opencode", report.Native)
			return nil
		},
	}
	c.Flags().StringVar(&from, "from", "", "Override OpenCode database path")
	return c
}

func printReverseReport(cmd *cobra.Command, r *migrate.ReverseReport) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "discovered=%d selected=%d successes=%d failures=%d\n",
		r.Discovered, r.Selected, r.Successes, r.Failures)
	for _, s := range r.Results {
		switch {
		case s.Error != nil:
			_, _ = fmt.Fprintf(out, "FAIL %s: %v\n", s.OCSessionID, s.Error)
		case r.DryRun:
			_, _ = fmt.Fprintf(out, "DRY  %s\t%s\n", s.OCSessionID, s.OutputPath)
		default:
			tag := ""
			if s.IsSubagent {
				tag = " [subagent]"
			}
			_, _ = fmt.Fprintf(out, "OK   %s\torigin=%s%s\t%s\n",
				s.OCSessionID, s.OriginID, tag, s.OutputPath)
		}
	}
}

func printVerifyGroup(cmd *cobra.Command, label string, rows []migrate.ReverseVerifyRow) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "[%s] %d session(s)\n", label, len(rows))
	for _, r := range rows {
		tag := ""
		if r.IsSubagent {
			tag = " [subagent]"
		}
		origin := r.OriginID
		if origin == "" {
			origin = "(native)"
		}
		_, _ = fmt.Fprintf(out, "  %s\torigin=%s%s\t%s\n",
			r.OCSessionID, origin, tag, r.Worktree)
	}
}

func resolveOCDB(from string) string {
	if from != "" {
		return from
	}
	return platform.OpenCodeDBPath()
}

func resolveCCHome(to string) string {
	if to != "" {
		return to
	}
	return platform.ClaudeCodeHome()
}

// newOCSkillsCmd, newOCCommandsCmd, newOCAgentsCmd, newOCRulesCmd,
// newOCMCPCmd implement the OC → CC artifact migrations.
//
// Each one uses the same structure: read OC-side, write CC-side via the
// target/claudecode writer. We don't need new orchestrators for these —
// the work is single-step per domain.

func newOCSkillsCmd() *cobra.Command {
	var (
		dryRun, yes bool
		cwd         string
	)
	c := &cobra.Command{
		Use:   "oc-skills",
		Short: "Copy OpenCode skills back to ~/.claude/skills/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReverseArtifacts(cmd.Context(), dryRun, yes, cwd, cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&cwd, "cwd", "", "Project root")
	return c
}

func newOCCommandsCmd() *cobra.Command {
	var (
		dryRun, yes bool
		cwd         string
	)
	c := &cobra.Command{
		Use:   "oc-commands",
		Short: "Copy OpenCode commands back to .claude/commands/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReverseArtifacts(cmd.Context(), dryRun, yes, cwd, cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&cwd, "cwd", "", "Project root")
	return c
}

func newOCAgentsCmd() *cobra.Command {
	var (
		dryRun, yes bool
		cwd         string
	)
	c := &cobra.Command{
		Use:   "oc-agents",
		Short: "Copy OpenCode agents back to .claude/agents/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReverseArtifacts(cmd.Context(), dryRun, yes, cwd, cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&cwd, "cwd", "", "Project root")
	return c
}

func newOCRulesCmd() *cobra.Command {
	var (
		dryRun, yes bool
		cwd         string
	)
	c := &cobra.Command{
		Use:   "oc-rules",
		Short: "Copy OpenCode rules back to .claude/rules/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReverseArtifacts(cmd.Context(), dryRun, yes, cwd, cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&cwd, "cwd", "", "Project root")
	return c
}

func newOCMCPCmd() *cobra.Command {
	var (
		dryRun, yes bool
		to          string
	)
	c := &cobra.Command{
		Use:   "oc-mcp",
		Short: "Transform and merge MCP config back into mcpServers{} (mcp -> mcpServers)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReverseMCP(cmd.Context(), dryRun, yes, resolveCCHome(to), cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&to, "to", "", "Override Claude Code home")
	return c
}

// runReverseArtifacts is the shared entry point for skills/commands/
// agents/rules OC→CC. Each domain reads OC files and writes CC files via
// the matching target/claudecode writer.
func runReverseArtifacts(ctx context.Context, dryRun, yes bool, cwd string, cmd *cobra.Command) error {
	if !yes && !dryRun {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
	}
	if dryRun {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "dry-run: nothing written")
		return nil
	}
	// Detect which command was invoked from Use.
	use := cmd.Name()
	switch use {
	case "oc-skills":
		skills, err := ocsrc.ReadGlobalSkills()
		if err != nil {
			return err
		}
		if p, err := ocsrc.ReadProjectSkills(cwd); err == nil {
			skills = append(skills, p...)
		}
		w := platformOpen().SkillWriterFor(cwd)
		written, err := w.WriteGlobal(skills)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %d skill(s)\n", len(written))
		for _, p := range written {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "skill: %s\n", p)
		}
	case "oc-commands":
		cmds, err := ocsrc.ReadGlobalCommands()
		if err != nil {
			return err
		}
		if p, err := ocsrc.ReadProjectCommands(cwd); err == nil {
			cmds = append(cmds, p...)
		}
		w := platformOpen().CommandWriterFor(cwd)
		written, err := w.WriteGlobal(cmds)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %d command(s)\n", len(written))
		for _, p := range written {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "command: %s\n", p)
		}
	case "oc-agents":
		agents, err := ocsrc.ReadGlobalAgents()
		if err != nil {
			return err
		}
		if p, err := ocsrc.ReadProjectAgents(cwd); err == nil {
			agents = append(agents, p...)
		}
		w := platformOpen().AgentWriterFor(cwd)
		written, err := w.WriteGlobal(agents)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %d agent(s)\n", len(written))
		for _, p := range written {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "agent: %s\n", p)
		}
	case "oc-rules":
		rules, err := ocsrc.ReadGlobalRules()
		if err != nil {
			return err
		}
		if p, err := ocsrc.ReadProjectRules(cwd); err == nil {
			rules = append(rules, p...)
		}
		w := platformOpen().RuleWriterFor(cwd)
		written, err := w.WriteGlobal(rules)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %d rule(s)\n", len(written))
		for _, p := range written {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "rule: %s\n", p)
		}
	default:
		return fmt.Errorf("unknown reverse artifact command: %s", use)
	}
	return nil
}

func runReverseMCP(ctx context.Context, dryRun, yes bool, ccHome string, cmd *cobra.Command) error {
	if !yes && !dryRun {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
	}
	servers, err := ocsrc.ReadGlobalMCP()
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no MCP servers found")
		return nil
	}
	if dryRun {
		for _, s := range servers {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "would merge %s\n", s.Name)
		}
		return nil
	}
	w := targetCCPath(ccHome)
	if _, err := w.Apply(servers); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "merged %d server(s)\n", len(servers))
	return nil
}

// runReverseSystemPrompt copies OC's ~/.config/opencode/AGENTS.md back to
// ~/.claude/CLAUDE.md. Triggered by both `oc-system` and the `reverse`
// meta-command.
func runReverseSystemPrompt(dryRun, yes bool, ccHome string, cmd *cobra.Command) error {
	if !yes && !dryRun {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
	}
	prompt, err := ocsrc.ReadGlobalSystemPrompt()
	if err != nil {
		return err
	}
	if prompt == nil {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no AGENTS.md found")
		return nil
	}
	if dryRun {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "would write %s -> %s\n", prompt.SourcePath, filepath.Join(ccHome, "CLAUDE.md"))
		return nil
	}
	w := cctgt.NewSystemPromptWriter()
	if ccHome != "" {
		w.Home = ccHome
	}
	out, err := w.Write(prompt)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", out)
	return nil
}

// avoid unused imports in conditional branches.
var _ = filepath.Join
