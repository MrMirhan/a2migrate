// Command a2migrate migrates AI coding session state and configuration
// between agents. The default direction in v1 is Claude Code → OpenCode.
//
// Network and telemetry policy: this binary performs zero network I/O
// outside of the user's explicit commands. There is no usage reporting,
// no crash dump uploading, no update check, and no phoning home. Any
// PR that adds one is rejected — even behind an opt-in flag.
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
