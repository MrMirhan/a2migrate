package cli

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/migrate"
	"github.com/mirhan/a2migrate/internal/source/claudecode"
	ocsrc "github.com/mirhan/a2migrate/internal/source/opencode"
	"github.com/mirhan/a2migrate/internal/target/opencode"
)

func openTargetDB(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		path = defaultOCDB()
	}
	return opencode.OpenDatabase(ctx, path)
}

func defaultOCDB() string { return "" }

func migrateRepair(ctx context.Context, db *sql.DB) (opencode.RepairReport, error) {
	return opencode.Repair(ctx, db, nil)
}

func newSkillsCmd() *cobra.Command {
	var (
		dryRun, yes bool
		cwd         string
	)
	c := &cobra.Command{
		Use:   "skills",
		Short: "Migrate Claude Code skills to OpenCode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes && !dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
			}
			return runArtifacts(cmd.Context(), migrate.ArtifactsMigrator{CWD: cwd, DryRun: dryRun}, cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&cwd, "cwd", "", "Project root (default: current directory)")
	return c
}

func newCommandsCmd() *cobra.Command {
	var (
		dryRun, yes bool
		cwd         string
	)
	c := &cobra.Command{
		Use:   "commands",
		Short: "Migrate slash commands from .claude/commands/ to .opencode/command/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runArtifacts(cmd.Context(), migrate.ArtifactsMigrator{CWD: cwd, DryRun: dryRun}, cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&cwd, "cwd", "", "Project root")
	return c
}

func newAgentsCmd() *cobra.Command {
	var (
		dryRun, yes bool
		cwd         string
	)
	c := &cobra.Command{
		Use:   "agents",
		Short: "Migrate agent definitions from .claude/agents/ to .opencode/agent/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runArtifacts(cmd.Context(), migrate.ArtifactsMigrator{CWD: cwd, DryRun: dryRun}, cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&cwd, "cwd", "", "Project root")
	return c
}

func newRulesCmd() *cobra.Command {
	var (
		dryRun, yes bool
		cwd         string
	)
	c := &cobra.Command{
		Use:   "rules",
		Short: "Migrate path-scoped rules from .claude/rules/ to .opencode/rules/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runArtifacts(cmd.Context(), migrate.ArtifactsMigrator{CWD: cwd, DryRun: dryRun}, cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&cwd, "cwd", "", "Project root")
	return c
}

func newMCPCmd() *cobra.Command {
	var (
		dryRun, yes bool
	)
	c := &cobra.Command{
		Use:   "mcp",
		Short: "Transform and merge MCP server config (mcpServers -> mcp)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes && !dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
			}
			servers, err := claudecode.ReadGlobalMCP()
			if err != nil {
				return err
			}
			if len(servers) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no MCP servers found")
				return nil
			}
			if dryRun {
				for _, s := range servers {
					fmt.Fprintf(cmd.OutOrStdout(), "would merge %s (%s)\n", s.Name, s.Type)
				}
				return nil
			}
			w := opencode.NewMCPConfigWriter()
			if _, err := w.Apply(opencode.MCPConfigPatch{Servers: servers}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "merged %d server(s)\n", len(servers))
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	return c
}

func newAllCmd() *cobra.Command {
	var (
		dryRun, yes, backup bool
		cwd, to, search     string
	)
	c := &cobra.Command{
		Use:   "all",
		Short: "Migrate everything (sessions, skills, commands, agents, rules, mcp, CLAUDE.md)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes && !dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
			}
			if err := runArtifacts(cmd.Context(), migrate.ArtifactsMigrator{CWD: cwd, DryRun: dryRun}, cmd); err != nil {
				return err
			}
			flags := &sessionsMigrateFlags{dryRun: dryRun, yes: yes, backup: backup, to: to, search: search}
			_, err := runSessionsMigrate(cmd.Context(), flags, migrate.Options{})
			if err != nil {
				return err
			}
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.BoolVar(&backup, "backup", false, "Backup before apply")
	f.StringVar(&cwd, "cwd", "", "Project root")
	f.StringVar(&to, "to", "", "Override OpenCode database path")
	f.StringVar(&search, "search", "", "Substring filter on session id or file path")
	return c
}

// newReverseCmd is the OC → CC counterpart to `all`. It walks the OC db,
// writes JSONL sessions back to ~/.claude/projects/, and copies every
// artifact domain back to its CC location.
func newReverseCmd() *cobra.Command {
	var (
		dryRun, yes        bool
		from, to, cwd      string
		skipNative         bool
		includes, excludes []string
		search             string
	)
	c := &cobra.Command{
		Use:   "reverse",
		Short: "Migrate OpenCode state back to Claude Code (sessions + artifacts)",
		Long:  "Reads opencode.db and writes JSONL sessions back into ~/.claude/projects/. Also copies OC artifacts (skills, commands, agents, rules, MCP, AGENTS.md) back to their CC locations.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes && !dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
			}
			if !dryRun {
				if err := runReverseArtifactsForDomain(cmd.Context(), "oc-skills", yes, cwd, cmd); err != nil {
					return err
				}
				if err := runReverseArtifactsForDomain(cmd.Context(), "oc-commands", yes, cwd, cmd); err != nil {
					return err
				}
				if err := runReverseArtifactsForDomain(cmd.Context(), "oc-agents", yes, cwd, cmd); err != nil {
					return err
				}
				if err := runReverseArtifactsForDomain(cmd.Context(), "oc-rules", yes, cwd, cmd); err != nil {
					return err
				}
			}
			if err := runReverseMCP(cmd.Context(), dryRun, yes, resolveCCHome(to), cmd); err != nil {
				return err
			}
			if !dryRun {
				if err := runReverseSystemPrompt(dryRun, yes, resolveCCHome(to), cmd); err != nil {
					return err
				}
			}
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
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&from, "from", "", "Override OpenCode database path")
	f.StringVar(&to, "to", "", "Override Claude Code home")
	f.StringVar(&cwd, "cwd", "", "Project root for project-scoped artifacts")
	f.BoolVar(&skipNative, "skip-native", false, "Skip sessions not migrated from Claude Code")
	f.StringSliceVar(&includes, "include", nil, "Only migrate sessions whose OC id matches")
	f.StringSliceVar(&excludes, "exclude", nil, "Skip sessions whose OC id matches")
	f.StringVar(&search, "search", "", "Substring filter on id or title")
	return c
}

// newOCSystemCmd wires `a2migrate oc-system` for AGENTS.md → CLAUDE.md.
func newOCSystemCmd() *cobra.Command {
	var (
		dryRun, yes bool
		to          string
	)
	c := &cobra.Command{
		Use:   "oc-system",
		Short: "Copy OpenCode AGENTS.md back to ~/.claude/CLAUDE.md",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReverseSystemPrompt(dryRun, yes, resolveCCHome(to), cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&to, "to", "", "Override Claude Code home")
	return c
}

// newSystemCmd wires `a2migrate system` for CLAUDE.md → AGENTS.md.
func newSystemCmd() *cobra.Command {
	var (
		dryRun, yes bool
	)
	c := &cobra.Command{
		Use:   "system",
		Short: "Copy ~/.claude/CLAUDE.md to ~/.config/opencode/AGENTS.md",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSystemPrompt(dryRun, yes, cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	return c
}

// runSystemPrompt copies ~/.claude/CLAUDE.md to ~/.config/opencode/AGENTS.md.
// Triggered by both `system` and the `all` meta-command.
func runSystemPrompt(dryRun, yes bool, cmd *cobra.Command) error {
	if !yes && !dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
	}
	prompt, err := claudecode.ReadGlobalSystemPrompt()
	if err != nil {
		return err
	}
	if prompt == nil {
		fmt.Fprintln(cmd.OutOrStdout(), "no CLAUDE.md found")
		return nil
	}
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "would write %s -> %s\n", prompt.SourcePath, "~/.config/opencode/AGENTS.md")
		return nil
	}
	w := opencode.NewSystemPromptWriter()
	out, err := w.Write(prompt)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", out)
	return nil
}

// runReverseArtifactsForDomain is a copy of runReverseArtifacts that
// takes the domain as an explicit string so the reverse meta command can
// invoke each one. The original runReverseArtifacts infers the domain
// from cmd.Name(), which doesn't work when called from a different
// command.
func runReverseArtifactsForDomain(ctx context.Context, domain string, yes bool, cwd string, cmd *cobra.Command) error {
	if !yes {
		fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
	}
	switch domain {
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
		fmt.Fprintf(cmd.OutOrStdout(), "skills: %d\n", len(written))
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
		fmt.Fprintf(cmd.OutOrStdout(), "commands: %d\n", len(written))
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
		fmt.Fprintf(cmd.OutOrStdout(), "agents: %d\n", len(written))
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
		fmt.Fprintf(cmd.OutOrStdout(), "rules: %d\n", len(written))
	default:
		return fmt.Errorf("unknown domain: %s", domain)
	}
	return nil
}

func runArtifacts(_ context.Context, m migrate.ArtifactsMigrator, cmd *cobra.Command) error {
	rep, err := m.Migrate()
	if err != nil {
		return err
	}
	if rep.DryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "dry-run: nothing written")
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"skills=%d commands=%d agents=%d rules=%d mcp=%d\n",
		len(rep.SkillsWritten), len(rep.CommandsWritten),
		len(rep.AgentsWritten), len(rep.RulesWritten), len(rep.MCPMerged))
	for _, p := range rep.SkillsWritten {
		fmt.Fprintf(cmd.OutOrStdout(), "skill: %s\n", p)
	}
	for _, p := range rep.CommandsWritten {
		fmt.Fprintf(cmd.OutOrStdout(), "command: %s\n", p)
	}
	for _, p := range rep.AgentsWritten {
		fmt.Fprintf(cmd.OutOrStdout(), "agent: %s\n", p)
	}
	for _, p := range rep.RulesWritten {
		fmt.Fprintf(cmd.OutOrStdout(), "rule: %s\n", p)
	}
	for _, n := range rep.MCPMerged {
		fmt.Fprintf(cmd.OutOrStdout(), "mcp: %s\n", n)
	}
	return nil
}

// satisfy unused-domain import (kept for future hooks implementation).
var _ = domain.Hook{}