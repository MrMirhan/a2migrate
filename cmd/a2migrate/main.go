// Command a2migrate migrates AI coding session state and configuration
// between agents. The default direction in v1 is Claude Code → OpenCode.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mirhan/a2migrate/internal/cli"
	"github.com/mirhan/a2migrate/internal/logging"
	"github.com/mirhan/a2migrate/internal/version"
)

func main() {
	// Bootstrap logger as early as possible so even cobra errors are structured.
	logger := logging.Setup(logging.Options{
		Level:  logging.LevelInfo,
		Format: "text",
	})

	root := cli.NewRootCmd(logger)
	root.SetVersionTemplate(version.Info() + "\n")
	root.SilenceUsage = true
	root.SilenceErrors = true

	if err := root.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
