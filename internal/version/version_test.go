package version

import (
	"strings"
	"testing"
)

// TestInfo_UsesLdflagsWhenSet pins the release path: injected values win
// over anything the toolchain embedded.
func TestInfo_UsesLdflagsWhenSet(t *testing.T) {
	restore := func(v, c, d string) { Version, Commit, BuildDate = v, c, d }
	defer restore(Version, Commit, BuildDate)

	Version, Commit, BuildDate = "v1.2.3", "abc1234", "2026-01-02T03:04:05Z"
	got := Info()
	for _, want := range []string{"v1.2.3", "abc1234", "2026-01-02T03:04:05Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("Info() = %q, missing %q", got, want)
		}
	}
}

// TestResolve_FallsBackToBuildInfo covers `go install`, which leaves the
// ldflags unset. Under `go test` the main module reports "(devel)" and
// no VCS stamp, so the defaults must survive rather than turn into
// empty strings.
func TestResolve_FallsBackToBuildInfo(t *testing.T) {
	restore := func(v, c, d string) { Version, Commit, BuildDate = v, c, d }
	defer restore(Version, Commit, BuildDate)

	Version, Commit, BuildDate = "dev", "unknown", "unknown"
	v, c, d := resolve()
	for name, got := range map[string]string{"version": v, "commit": c, "buildDate": d} {
		if got == "" {
			t.Errorf("%s is empty; the fallback must never blank a field", name)
		}
	}
}

func TestShortCommit(t *testing.T) {
	if got := shortCommit("2daf43963dfc4874e72c7117f4a4b729aaa9208f"); got != "2daf439" {
		t.Errorf("shortCommit = %q, want 2daf439", got)
	}
	if got := shortCommit("abc"); got != "abc" {
		t.Errorf("shortCommit(%q) = %q, want it returned unchanged", "abc", got)
	}
}
