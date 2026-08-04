// Package sync implements bidirectional state synchronization between
// Claude Code and OpenCode. Two halves:
//
//	Artifact sync  - file-by-file, mtime last-writer-wins
//	Session sync   - per-message dedup by uuid; append-only on both sides
//
// The package is intentionally idempotent: re-running sync with no
// intervening changes produces zero writes.
package sync

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mirhan/a2migrate/internal/platform"
)

// Direction selects which side wins when both have a file with the same
// name. "newer" means last-writer-wins by mtime; "prefer-cc" /
// "prefer-oc" force a specific direction regardless of mtime.
type Direction string

const (
	NewerWins Direction = "newer"
	PreferCC  Direction = "prefer-cc"
	PreferOC  Direction = "prefer-oc"
)

// Plan describes one sync operation.
type Plan struct {
	Copies  []Copy
	Deletes []Delete // never emitted in v1; reserved for future prune mode
}

// Copy is one file replication: src → dst.
type Copy struct {
	Src       string
	Dst       string
	Reason    string // human-readable: "newer" / "missing on dst" / "forced"
	SizeBytes int64
}

// Delete is one file removal from the destination.
type Delete struct {
	Path   string
	Reason string
}

// Op is the kind of action taken during a sync run.
type Op string

const (
	OpCopyCCtoOC Op = "cc→oc"
	OpCopyOCtoCC Op = "oc→cc"
	OpSkip       Op = "skip"
)

// Result summarizes one executed sync.
type Result struct {
	Op    Op
	Path  string
	Bytes int64
}

// Report aggregates everything sync produced.
type Report struct {
	Plan    Plan
	Applied []Result
	Errors  []error
	Skipped int
	CCHome  string
	OCHome  string
}

// SyncArtifacts reconciles the file artifacts (skills, commands, agents,
// rules) between CC and OC. Direction controls how ties are broken.
//
// CC path:    <CCHome>/<domain>/   (and  <cwd>/.claude/<domain>/)
// OC path:    <OCHome>/<domain>/   (and  <cwd>/.opencode/<domain>/)
//
// For each pair (one file on each side with the same basename) the
// newer mtime wins. Files that exist on only one side propagate to the
// other side. Files with equal mtime are left alone.
func SyncArtifacts(direction Direction, dryRun bool) (*Report, error) {
	ccHome := platform.ClaudeCodeHome()
	ocHome := platform.OpenCodeConfigHome()
	return SyncArtifactsAt(ccHome, ocHome, direction, dryRun)
}

// SyncArtifactsAt is the same as SyncArtifacts but with explicit roots,
// used by tests and by callers that need to override the platform defaults.
func SyncArtifactsAt(ccHome, ocHome string, direction Direction, dryRun bool) (*Report, error) {
	r := &Report{CCHome: ccHome, OCHome: ocHome}

	domains := []string{"skills", "commands", "agents", "rules"}
	for _, domain := range domains {
		if err := syncOneDomain(ccHome, ocHome, domain, direction, dryRun, r); err != nil {
			r.Errors = append(r.Errors, err)
		}
	}
	// Top-level instructions file (CLAUDE.md ↔ AGENTS.md) — file-level,
	// not directory-level. Treat the file as its own basename.
	ccSys := filepath.Join(ccHome, "CLAUDE.md")
	ocSys := filepath.Join(ocHome, "AGENTS.md")
	if _, err := reconcileOne(ccSys, ocSys, direction, dryRun, r); err != nil {
		r.Errors = append(r.Errors, err)
	}
	return r, nil
}

func syncOneDomain(ccHome, ocHome, domain string, direction Direction, dryRun bool, r *Report) error {
	ccDir := filepath.Join(ccHome, domain)
	ocDir := filepath.Join(ocHome, domain)

	ccFiles, err := listMarkdown(ccDir)
	if err != nil {
		return fmt.Errorf("list cc %s: %w", domain, err)
	}
	ocFiles, err := listMarkdown(ocDir)
	if err != nil {
		return fmt.Errorf("list oc %s: %w", domain, err)
	}

	seen := map[string]bool{}
	for name, ccPath := range ccFiles {
		seen[name] = true
		ocPath := filepath.Join(ocDir, name)
		op, err := reconcileOne(ccPath, ocPath, direction, dryRun, r)
		if err != nil {
			r.Errors = append(r.Errors, err)
		}
		if op == "" {
			r.Skipped++
		}
		_ = op
	}
	for name, ocPath := range ocFiles {
		if seen[name] {
			continue
		}
		ccPath := filepath.Join(ccDir, name)
		_, err := reconcileOne(ccPath, ocPath, direction, dryRun, r)
		if err != nil {
			r.Errors = append(r.Errors, err)
		}
	}
	return nil
}

// reconcileOne decides which way to copy based on mtime. Returns "" if
// no action needed. Returns the op that was applied (or "" if skipped).
func reconcileOne(ccPath, ocPath string, direction Direction, dryRun bool, r *Report) (string, error) {
	ccInfo, ccErr := os.Stat(ccPath)
	ocInfo, ocErr := os.Stat(ocPath)

	switch {
	case ccErr != nil && ocErr != nil:
		return "", nil
	case ccErr != nil:
		// Only OC has it; OC→CC.
		return apply(ocPath, ccPath, OpCopyOCtoCC, "only on oc", dryRun, r)
	case ocErr != nil:
		// Only CC has it; CC→OC.
		return apply(ccPath, ocPath, OpCopyCCtoOC, "only on cc", dryRun, r)
	}

	decide := decideDirection(ccInfo.ModTime(), ocInfo.ModTime(), direction)
	if decide == "" {
		return "", nil
	}
	if decide == "cc" {
		return apply(ccPath, ocPath, OpCopyCCtoOC, "newer", dryRun, r)
	}
	return apply(ocPath, ccPath, OpCopyOCtoCC, "newer", dryRun, r)
}

// decideDirection returns "cc" or "oc" or "" based on mtime + preference.
// Returns "" when no action is required (already in sync).
func decideDirection(ccM, ocM timeT, dir Direction) string {
	switch dir {
	case PreferCC:
		return "cc"
	case PreferOC:
		return "oc"
	}
	if ccM.After(ocM) {
		return "cc"
	}
	if ocM.After(ccM) {
		return "oc"
	}
	return ""
}

// apply copies src→dst when not in dry-run, otherwise records the intent.
func apply(src, dst string, op Op, reason string, dryRun bool, r *Report) (string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if !dryRun {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := copyFile(src, dst); err != nil {
			return "", err
		}
	}
	r.Applied = append(r.Applied, Result{Op: op, Path: dst, Bytes: info.Size()})
	return string(op), nil
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	// Preserve source mtime so a re-run is a no-op. The bug here was
	// that copyFile set dst mtime to "now" via OpenFile/O_CREATE, which
	// meant NewerWins always picked dst as newer and re-copied forever.
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

// listMarkdown returns base → full path for every *.md file in dir.
// Returns empty map (no error) when dir is missing.
func listMarkdown(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !endsWithMD(e.Name()) {
			continue
		}
		out[e.Name()] = filepath.Join(dir, e.Name())
	}
	return out, nil
}

func endsWithMD(s string) bool {
	return len(s) >= 3 && s[len(s)-3:] == ".md"
}
