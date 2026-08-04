package cli

import "github.com/spf13/cobra"

func newAllCmd() *cobra.Command {
	var dryRun, yes, backup bool
	c := &cobra.Command{
		Use:   "all",
		Short: "Migrate everything (sessions, skills, commands, agents, rules, mcp)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented yet")
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	f.BoolVar(&backup, "backup", false, "Create a timestamped DB backup before apply")
	return c
}