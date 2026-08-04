// Package version exposes build-time information.
package version

import (
	"fmt"
	"runtime"
)

// Set at build time via -ldflags "-X github.com/mirhan/a2migrate/internal/version.Version=..."
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info returns a multi-line human-readable build summary.
func Info() string {
	return fmt.Sprintf("a2migrate %s (commit %s, built %s, %s/%s, %s)",
		Version, Commit, BuildDate, runtime.GOOS, runtime.GOARCH, runtime.Version())
}
