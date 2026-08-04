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

func newSessionsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List discovered Claude Code sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented yet")
			return nil
		},
	}
}

func newSessionsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show details of one session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented yet")
			return nil
		},
	}
}

func newSessionsSelectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "select",
		Short: "Interactively select sessions to migrate",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented yet")
			return nil
		},
	}
}

func newSessionsMigrateCmd() *cobra.Command {
	var (
		dryRun    bool
		yes       bool
		backup    bool
		from      string
		to        string
		renames   []string
		includes  []string
		excludes  []string
		search    string
	)
	c := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate one or many sessions to OpenCode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented yet")
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "Plan only; do not write to disk")
	f.BoolVar(&yes, "yes", false, "Skip confirmation prompts")
	f.BoolVar(&backup, "backup", false, "Create a timestamped DB backup before apply")
	f.StringVar(&from, "from", "", "Override Claude Code home")
	f.StringVar(&to, "to", "", "Override OpenCode database path")
	f.StringSliceVar(&renames, "rename", nil, "Rename a session during migration (old=new)")
	f.StringSliceVar(&includes, "include", nil, "Only migrate sessions whose id matches")
	f.StringSliceVar(&excludes, "exclude", nil, "Skip sessions whose id matches")
	f.StringVar(&search, "search", "", "Substring filter on session title or id")
	return c
}

func newSessionsVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Verify migrated sessions in the OpenCode database",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented yet")
			return nil
		},
	}
}

func newSessionsRepairCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "repair",
		Short: "Re-run post-migration invariants on already-migrated sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println("not implemented yet")
			return nil
		},
	}
}