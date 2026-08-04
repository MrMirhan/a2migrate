package cli

import (
	"github.com/spf13/cobra"

	"github.com/mirhan/a2migrate/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(version.Info())
			return nil
		},
	}
}