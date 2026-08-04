package cli

import "github.com/spf13/cobra"

func newSessionsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "sessions",
		Short: "Discover, list, and migrate Claude Code sessions",
	}
	c.AddCommand(newSessionsListCmd())
	c.AddCommand(newSessionsShowCmd())
	c.AddCommand(newSessionsSelectCmd())
	c.AddCommand(newSessionsMigrateCmd())
	c.AddCommand(newSessionsVerifyCmd())
	c.AddCommand(newSessionsRepairCmd())
	return c
}