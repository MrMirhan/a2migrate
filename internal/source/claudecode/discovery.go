// Package claudecode reads Claude Code on-disk state.
//
// Claude Code stores state under ~/.claude/ (or $CLAUDE_CODE_HOME) with this
// layout:
//
//	projects/<encoded-cwd>/<session-uuid>.jsonl          ← main session
//	projects/<encoded-cwd>/<session-uuid>/subagents/agent-*.jsonl
//	projects/<encoded-cwd>/<session-uuid>/tool-results/   ← skipped
//	skills/<name>.md                                      ← global skills
//	agents/<name>.md                                      ← global agents
//	commands/<name>.md                                    ← global slash cmds
//	settings.json                                         ← settings + hooks
//	mcp.json                                              ← global MCP
//
// This package is a reader; nothing it returns is mutated and nothing is
// written to disk.
package claudecode

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mirhan/a2migrate/internal/platform"
)

// ErrNoSessions is returned when DiscoverSessions finds no JSONL files.
var ErrNoSessions = errors.New("no claude code sessions found")

// SessionReader discovers and parses Claude Code session JSONL files.
type SessionReader struct {
	CCHome string
}

// NewSessionReader returns a reader rooted at ccHome. If ccHome is empty,
// defaults to platform.ClaudeCodeHome().
func NewSessionReader(ccHome string) *SessionReader {
	if ccHome == "" {
		ccHome = platform.ClaudeCodeHome()
	}
	return &SessionReader{CCHome: ccHome}
}

// Project describes one discovered workspace under ~/.claude/projects/.
type Project struct {
	ID        string // sha1(worktree)[:40] or "global"
	Encoded   string // encoded cwd (directory basename)
	Worktree  string // decoded absolute path
	IsNew     bool   // true if not yet present in OC db
	MainCount int    // number of main session files
	SubCount  int    // number of subagent files
}

// DiscoverProjects scans the projects directory and returns one Project per
// encoded-cwd directory. Sorted by Encoded for determinism.
func (r *SessionReader) DiscoverProjects() ([]Project, error) {
	dir := filepath.Join(r.CCHome, "projects")
	if !platform.IsDir(dir) {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read projects dir %s: %w", dir, err)
	}
	var out []Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		encoded := e.Name()
		worktree := platform.DecodeCWD(encoded)
		if worktree == "/home/" || worktree == "/home" {
			worktree = "/"
		}
		id := projectIDForWorktree(worktree)
		mains, subs := r.countSessionFiles(encoded)
		out = append(out, Project{
			ID:        id,
			Encoded:   encoded,
			Worktree:  worktree,
			MainCount: mains,
			SubCount:  subs,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Encoded < out[j].Encoded })
	return out, nil
}

func (r *SessionReader) countSessionFiles(encoded string) (main, sub int) {
	dir := filepath.Join(r.CCHome, "projects", encoded)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			if e.Name() == "tool-results" {
				continue
			}
			subDir := filepath.Join(dir, e.Name(), "subagents")
			subEntries, err := os.ReadDir(subDir)
			if err != nil {
				continue
			}
			for _, se := range subEntries {
				if !se.IsDir() && strings.HasPrefix(se.Name(), "agent-") && strings.HasSuffix(se.Name(), ".jsonl") {
					sub++
				}
			}
		} else if strings.HasSuffix(e.Name(), ".jsonl") {
			main++
		}
	}
	return
}

// SessionRef points at one JSONL transcript file on disk.
type SessionRef struct {
	FilePath   string
	OriginID   string // CC session UUID (basename without .jsonl)
	ProjectID  string
	Worktree   string
	IsSubagent bool
	ParentID   string // origin id of parent session if subagent
	SizeBytes  int64
	UpdatedAt  int64 // unix seconds
}

// DiscoverSessions returns all main and subagent session JSONL files
// across all projects. Sorted by FilePath for determinism.
func (r *SessionReader) DiscoverSessions() ([]SessionRef, error) {
	dir := filepath.Join(r.CCHome, "projects")
	if !platform.IsDir(dir) {
		return nil, ErrNoSessions
	}
	projects, err := r.DiscoverProjects()
	if err != nil {
		return nil, err
	}
	var out []SessionRef
	for _, p := range projects {
		projDir := filepath.Join(dir, p.Encoded)
		entries, err := os.ReadDir(projDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			full := filepath.Join(projDir, e.Name())
			if e.IsDir() {
				if e.Name() == "tool-results" {
					continue
				}
				subRefs := scanSubagentDir(filepath.Join(full, "subagents"), p, e.Name())
				out = append(out, subRefs...)
				continue
			}
			if !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			origin := strings.TrimSuffix(e.Name(), ".jsonl")
			info, _ := e.Info()
			out = append(out, SessionRef{
				FilePath:  full,
				OriginID:  origin,
				ProjectID: p.ID,
				Worktree:  p.Worktree,
				SizeBytes: info.Size(),
				UpdatedAt: info.ModTime().Unix(),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FilePath < out[j].FilePath })
	if len(out) == 0 {
		return nil, ErrNoSessions
	}
	return out, nil
}

func scanSubagentDir(dir string, p Project, parentSession string) []SessionRef {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []SessionRef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if !strings.HasPrefix(e.Name(), "agent-") {
			continue
		}
		info, _ := e.Info()
		out = append(out, SessionRef{
			FilePath:   filepath.Join(dir, e.Name()),
			OriginID:   strings.TrimSuffix(e.Name(), ".jsonl"),
			ProjectID:  p.ID,
			Worktree:   p.Worktree,
			IsSubagent: true,
			ParentID:   parentSession,
			SizeBytes:  info.Size(),
			UpdatedAt:  info.ModTime().Unix(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FilePath < out[j].FilePath })
	return out
}

// projectIDForWorktree returns the OpenCode project id for a worktree path.
// "/" → "global"; anything else → sha1(worktree)[:40].
func projectIDForWorktree(worktree string) string {
	if worktree == "/" || worktree == "" {
		return "global"
	}
	h := sha1.Sum([]byte(worktree))
	return hex.EncodeToString(h[:])[:40]
}

// ProjectIDForWorktree is the exported version used by target writers so
// both sides agree on the same id.
func ProjectIDForWorktree(worktree string) string {
	return projectIDForWorktree(worktree)
}
