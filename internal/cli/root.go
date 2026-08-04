// Package cli wires cobra commands and shared flags. It is the only place
// that knows about the CLI surface; everything below it is library code.
package cli

import (
	"github.com/spf13/cobra"
	"log/slog"

	"github.com/mirhan/a2migrate/internal/logging"
	"github.com/mirhan/a2migrate/internal/platform"
)

// RootLogger is the slog logger the root command was last configured
// with. Tests inspect it to assert pre-run hook ordering.
var RootLogger = slog.Default()

// NewRootCmd constructs the root command and attaches all subcommands.
// Logging is injected so subcommands can decorate logs with command context.
func NewRootCmd(logger *slog.Logger) *cobra.Command {
	if logger != nil {
		RootLogger = logger
	}

	root := &cobra.Command{
		Use:   "a2migrate",
		Short: "Migrate AI coding session state between agents",
		Long: "a2migrate ports sessions, skills, commands, agents, rules, MCP servers, and system\n" +
			"prompts between AI coding tools.\n\n" +
			"Tools are arguments, not commands: `a2migrate migrate claude-code opencode sessions`.\n" +
			"Run `a2migrate tools` to see which tools and domains are available.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

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
	// The flag's default comes from the environment so A2MIGRATE_LOG_LEVEL
	// applies when --log-level is absent, and loses to it when present.
	pf.StringVar(&logLevel, "log-level", platform.EnvOr("A2MIGRATE_LOG_LEVEL", "info"),
		"Log level: error | warn | info | debug (env: A2MIGRATE_LOG_LEVEL)")

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

	root.AddCommand(newMigrateCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newShowCmd())
	root.AddCommand(newSelectCmd())
	root.AddCommand(newVerifyCmd())
	root.AddCommand(newRepairCmd())
	root.AddCommand(newSyncCmd())
	root.AddCommand(newToolsCmd())
	root.AddCommand(newVersionCmd())

	return root
}
