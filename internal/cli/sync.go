package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mirhan/a2migrate/internal/platform"
	syncpkg "github.com/mirhan/a2migrate/internal/sync"
)

// newSyncCmd wires the bidirectional `sync` command tree.
func newSyncCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "sync",
		Short: "Bidirectional state sync between Claude Code and OpenCode",
		Long: `Reconcile CC and OC state using mtime-based last-writer-wins for
artifacts (skills, commands, agents, rules, MCP) and uuid-deduped
append-only for sessions. Idempotent.`,
	}
	c.AddCommand(newSyncAllCmd())
	c.AddCommand(newSyncArtifactsCmd())
	c.AddCommand(newSyncSessionsCmd())
	c.AddCommand(newSyncReverseCmd())
	return c
}

func newSyncAllCmd() *cobra.Command {
	var (
		dryRun, yes bool
		from, to    string
		direction   string
	)
	c := &cobra.Command{
		Use:   "all",
		Short: "Sync artifacts + sessions in both directions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes && !dryRun {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(use --yes to skip this confirmation)")
			}
			if err := runSyncArtifacts(direction, dryRun, cmd); err != nil {
				return err
			}
			report, err := syncpkg.Sessions(cmd.Context(), resolveOCDB(to), dryRun)
			if err != nil {
				return err
			}
			printSyncReport(cmd, report)
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only; do not write")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&direction, "direction", "newer", "newer | prefer-cc | prefer-oc")
	f.StringVar(&to, "to", "", "Override OpenCode database path")
	f.StringVar(&from, "from", "", "Override Claude Code home")
	return c
}

func newSyncArtifactsCmd() *cobra.Command {
	var (
		dryRun, yes bool
		direction   string
	)
	c := &cobra.Command{
		Use:   "artifacts",
		Short: "Sync skills/commands/agents/rules/MCP via mtime last-writer-wins",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSyncArtifacts(direction, dryRun, cmd)
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&direction, "direction", "newer", "newer | prefer-cc | prefer-oc")
	return c
}

func newSyncSessionsCmd() *cobra.Command {
	var (
		dryRun, yes bool
		to          string
	)
	c := &cobra.Command{
		Use:   "sessions",
		Short: "Sync CC sessions → OC database (uuid-deduped, append-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := syncpkg.Sessions(cmd.Context(), resolveOCDB(to), dryRun)
			if err != nil {
				return err
			}
			printSyncReport(cmd, report)
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&to, "to", "", "Override OpenCode database path")
	return c
}

func newSyncReverseCmd() *cobra.Command {
	var (
		dryRun, yes bool
		from, to    string
	)
	c := &cobra.Command{
		Use:   "reverse",
		Short: "Sync OC sessions → CC JSONL (uuid-deduped, append-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ccHome := platform.ClaudeCodeHome()
			if from != "" {
				ccHome = from
			}
			report, err := syncpkg.SessionsReverse(cmd.Context(), resolveOCDB(to), ccHome, dryRun)
			if err != nil {
				return err
			}
			printSyncReport(cmd, report)
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.StringVar(&to, "to", "", "Override OpenCode database path")
	f.StringVar(&from, "from", "", "Override Claude Code home")
	return c
}

func runSyncArtifacts(direction string, dryRun bool, cmd *cobra.Command) error {
	dir := syncpkg.NewerWins
	switch direction {
	case "prefer-cc":
		dir = syncpkg.PreferCC
	case "prefer-oc":
		dir = syncpkg.PreferOC
	}
	report, err := syncpkg.SyncArtifacts(dir, dryRun)
	if err != nil {
		return err
	}
	printSyncReport(cmd, report)
	return nil
}

func printSyncReport(cmd *cobra.Command, r *syncpkg.Report) {
	out := cmd.OutOrStdout()
	if r == nil {
		return
	}
	for _, a := range r.Applied {
		_, _ = fmt.Fprintf(out, "%s\t%s\n", a.Op, a.Path)
	}
	if r.Skipped > 0 {
		_, _ = fmt.Fprintf(out, "skipped: %d\n", r.Skipped)
	}
	for _, e := range r.Errors {
		_, _ = fmt.Fprintf(out, "ERROR: %v\n", e)
	}
}
