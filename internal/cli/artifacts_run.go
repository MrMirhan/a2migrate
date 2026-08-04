package cli

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/migrate"
	"github.com/mirhan/a2migrate/internal/source/claudecode"
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
		cwd                 string
	)
	c := &cobra.Command{
		Use:   "all",
		Short: "Migrate everything (sessions, skills, commands, agents, rules, mcp)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes && !dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
			}
			if err := runArtifacts(cmd.Context(), migrate.ArtifactsMigrator{CWD: cwd, DryRun: dryRun}, cmd); err != nil {
				return err
			}
			flags := &sessionsMigrateFlags{dryRun: dryRun, yes: yes, backup: backup, from: "", to: "", skipRepair: false}
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
	return c
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