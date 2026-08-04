// Package version exposes build-time information.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Set at build time via -ldflags "-X github.com/MrMirhan/a2migrate/internal/version.Version=..."
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info returns a multi-line human-readable build summary.
func Info() string {
	v, c, d := resolve()
	return fmt.Sprintf("a2migrate %s (commit %s, built %s, %s/%s, %s)",
		v, c, d, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// resolve falls back to the metadata the toolchain embeds when the
// ldflags are absent, which is what `go install` produces.
func resolve() (version, commit, buildDate string) {
	version, commit, buildDate = Version, Commit, BuildDate

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version, commit, buildDate
	}
	if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if commit == "unknown" && s.Value != "" {
				commit = shortCommit(s.Value)
			}
		case "vcs.time":
			if buildDate == "unknown" && s.Value != "" {
				buildDate = s.Value
			}
		}
	}
	return version, commit, buildDate
}

func shortCommit(rev string) string {
	const short = 7
	if len(rev) > short {
		return rev[:short]
	}
	return rev
}
