package cli

import "github.com/spf13/cobra"

func newCommandsCmd() *cobra.Command {
	var dryRun, yes bool
	c := &cobra.Command{
		Use:   "commands",
		Short: "Migrate slash commands from .claude/commands/ to .opencode/command/",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented yet")
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only")
	f.BoolVar(&yes, "yes", false, "Skip confirmation")
	return c
}