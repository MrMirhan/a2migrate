// Package cli wires cobra commands and shared flags. It is the only place
// that knows about the CLI surface; everything below it is library code.
package cli

import (
	"github.com/spf13/cobra"
	"log/slog"

	"github.com/mirhan/a2migrate/internal/logging"
)

// NewRootCmd constructs the root command and attaches all subcommands.
// Logging is injected so subcommands can decorate logs with command context.
func NewRootCmd(logger *slog.Logger) *cobra.Command {
	root := &cobra.Command{
		Use:           "a2migrate",
		Short:         "Migrate AI coding session state between agents",
		Long:          "a2migrate ports Claude Code sessions, skills, commands, agents, rules, and MCP servers into OpenCode.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Global flags
	var (
		verbose   bool
		noColor   bool
		jsonOut   bool
		logFormat string
		logLevel  string
	)

	pf := root.PersistentFlags()
	pf.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging")
	pf.BoolVar(&noColor, "no-color", false, "Disable color output")
	pf.BoolVar(&jsonOut, "json", false, "Emit machine-readable JSON output where supported")
	pf.StringVar(&logFormat, "log-format", "text", "Log format: text | json")
	pf.StringVar(&logLevel, "log-level", "info", "Log level: error | warn | info | debug")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		level := logging.ParseLevel(logLevel)
		if verbose {
			level = logging.LevelDebug
		}
		logging.Setup(logging.Options{
			Level:  level,
			Format: logFormat,
		})
		return nil
	}

	root.AddCommand(newSessionsCmd())
	root.AddCommand(newSkillsCmd())
	root.AddCommand(newCommandsCmd())
	root.AddCommand(newAgentsCmd())
	root.AddCommand(newRulesCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newAllCmd())
	root.AddCommand(newOCSessionsCmd())
	root.AddCommand(newOCSkillsCmd())
	root.AddCommand(newOCCommandsCmd())
	root.AddCommand(newOCAgentsCmd())
	root.AddCommand(newOCRulesCmd())
	root.AddCommand(newOCMCPCmd())
	root.AddCommand(newReverseCmd())
	root.AddCommand(newSyncCmd())
	root.AddCommand(newVersionCmd())

	if logger == nil {
		// Defensive default if a caller forgets to inject.
		logger = slog.Default()
	}
	_ = logger
	return root
}