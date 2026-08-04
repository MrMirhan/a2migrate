package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/platform"
	"github.com/mirhan/a2migrate/internal/source/opencode"
	"github.com/mirhan/a2migrate/internal/target/claudecode"
)

// ReverseOptions configure an OC → CC migration run.
type ReverseOptions struct {
	From       string // OC db path override
	To         string // CC home override
	DryRun     bool
	Yes        bool
	Includes   []string // OC session ids to include
	Excludes   []string // OC session ids to skip
	Search     string
	SkipNative bool   // skip sessions without claude_code_origin
	Logger     *slog.Logger
}

// ReverseResult is one migrated row.
type ReverseResult struct {
	OCSessionID string
	OriginID    string // CC origin id (if migrated from CC) or generated
	Title       string
	ProjectDir  string
	OutputPath  string
	IsSubagent  bool
	Skipped     bool
	SkippedReason string
	Error       error
}

// ReverseReport summarises an OC → CC migration run.
type ReverseReport struct {
	DryRun    bool
	Discovered int
	Selected   int
	Successes  int
	Failures   int
	Skipped    int
	Results    []ReverseResult
}

// ReverseMigrator orchestrates discovery → parse → emit JSONL.
type ReverseMigrator struct {
	Source  *opencode.SessionReader
	Options ReverseOptions
	Logger  *slog.Logger
}

// NewReverseMigrator wires defaults.
func NewReverseMigrator(opts ReverseOptions) *ReverseMigrator {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &ReverseMigrator{
		Source:  opencode.NewSessionReader(opts.From),
		Options: opts,
		Logger:  opts.Logger,
	}
}

// Discover returns every OC session row.
func (m *ReverseMigrator) Discover(ctx context.Context) ([]opencode.SessionRef, error) {
	db, err := m.Source.Open(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	refs, err := m.Source.DiscoverSessions(ctx, db)
	if err != nil {
		return nil, err
	}
	return m.applyFilters(refs), nil
}

func (m *ReverseMigrator) applyFilters(refs []opencode.SessionRef) []opencode.SessionRef {
	includes := setFromStrings(m.Options.Includes)
	excludes := setFromStrings(m.Options.Excludes)
	search := m.Options.Search
	var out []opencode.SessionRef
	for _, r := range refs {
		if len(includes) > 0 && !includes[r.OCSessionID] {
			continue
		}
		if excludes[r.OCSessionID] {
			continue
		}
		if m.Options.SkipNative && r.OriginID == "" {
			continue
		}
		if search != "" {
			hay := r.OCSessionID + " " + r.Title
			if !containsFold(hay, search) {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

// Run executes the migration. Returns the report even on partial failure.
func (m *ReverseMigrator) Run(ctx context.Context, refs []opencode.SessionRef) (*ReverseReport, error) {
	rep := &ReverseReport{DryRun: m.Options.DryRun}
	refs = m.applyFilters(refs)
	rep.Discovered = len(refs)
	rep.Selected = len(refs)
	if len(refs) == 0 {
		return rep, nil
	}

	db, err := m.Source.Open(ctx)
	if err != nil {
		return rep, err
	}
	defer db.Close()

	writer := claudecode.NewSessionWriter(m.Options.To)

	// First pass: parse main sessions.
	mainOCIDByOrigin := map[string]string{}
	for i := range refs {
		r := &refs[i]
		if r.IsSubagent {
			continue
		}
		sess, err := m.Source.ParseSession(ctx, db, *r)
		if err != nil {
			rep.Failures++
			rep.Results = append(rep.Results, ReverseResult{OCSessionID: r.OCSessionID, Error: fmt.Errorf("parse: %w", err)})
			continue
		}
		if m.Options.DryRun {
			mainOCIDByOrigin[r.OriginID] = r.OCSessionID
			ccHome := m.Options.To
			if ccHome == "" {
				ccHome = platform.ClaudeCodeHome()
			}
			rep.Results = append(rep.Results, ReverseResult{
				OCSessionID: r.OCSessionID,
				OriginID:    r.OriginID,
				Title:       r.Title,
				ProjectDir:  r.Worktree,
				OutputPath:  filepath.Join(ccHome, "projects", platformEncode(r.Worktree), r.OriginID+".jsonl"),
			})
			continue
		}
		out, err := writer.WriteSession(sess, "")
		if err != nil {
			rep.Failures++
			rep.Results = append(rep.Results, ReverseResult{OCSessionID: r.OCSessionID, Error: err})
			continue
		}
		mainOCIDByOrigin[r.OriginID] = r.OCSessionID
		rep.Successes++
		rep.Results = append(rep.Results, ReverseResult{
			OCSessionID: r.OCSessionID,
			OriginID:    r.OriginID,
			Title:       sess.Title,
			ProjectDir:  sess.ProjectDir,
			OutputPath:  out,
		})
	}

	// Second pass: subagents. We need the main session's OriginID so the
	// subagent JSONL can reference it as the parent.
	for i := range refs {
		r := &refs[i]
		if !r.IsSubagent {
			continue
		}
		sess, err := m.Source.ParseSession(ctx, db, *r)
		if err != nil {
			rep.Failures++
			rep.Results = append(rep.Results, ReverseResult{OCSessionID: r.OCSessionID, Error: fmt.Errorf("parse: %w", err)})
			continue
		}
		// Use the OC parent id if available; otherwise the parent's CC origin id.
		var parentOC string
		if r.ParentID != "" {
			parentOC = r.ParentID
		}
		if m.Options.DryRun {
			rep.Results = append(rep.Results, ReverseResult{
				OCSessionID: r.OCSessionID,
				OriginID:    r.OriginID,
				Title:       r.Title,
				ProjectDir:  r.Worktree,
				IsSubagent:  true,
				OutputPath:  "would-write subagent for parent " + parentOC,
			})
			continue
		}
		out, err := writer.WriteSession(sess, parentOC)
		if err != nil {
			rep.Failures++
			rep.Results = append(rep.Results, ReverseResult{OCSessionID: r.OCSessionID, Error: err})
			continue
		}
		rep.Successes++
		rep.Results = append(rep.Results, ReverseResult{
			OCSessionID: r.OCSessionID,
			OriginID:    r.OriginID,
			Title:       sess.Title,
			ProjectDir:  sess.ProjectDir,
			OutputPath:  out,
			IsSubagent:  true,
		})
	}
	return rep, nil
}

func containsFold(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	return stringContainsFold(haystack, needle)
}

func stringContainsFold(haystack, needle string) bool {
	hl := lower(haystack)
	nl := lower(needle)
	for i := 0; i+len(nl) <= len(hl); i++ {
		if hl[i:i+len(nl)] == nl {
			return true
		}
	}
	return false
}

func lower(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		out[i] = c
	}
	return string(out)
}

func platformEncode(worktree string) string {
	return platform.EncodeCWD(worktree)
}

// VerifyReverse lists what's in the OC db.
type ReverseVerifyReport struct {
	MigratedFromCC []ReverseVerifyRow
	Native         []ReverseVerifyRow
}

type ReverseVerifyRow struct {
	OCSessionID string
	OriginID    string
	IsSubagent  bool
	Title       string
	Worktree    string
}

// VerifyReverse returns one row per session in the OC db, partitioned by
// whether it was migrated from CC or is a native OC session.
func VerifyReverse(ctx context.Context, dbPath string) (*ReverseVerifyReport, error) {
	r := opencode.NewSessionReader(dbPath)
	db, err := r.Open(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	refs, err := r.DiscoverSessions(ctx, db)
	if err != nil {
		return nil, err
	}
	rep := &ReverseVerifyReport{}
	for _, ref := range refs {
		row := ReverseVerifyRow{
			OCSessionID: ref.OCSessionID,
			OriginID:    ref.OriginID,
			IsSubagent:  ref.IsSubagent,
			Title:       ref.Title,
			Worktree:    ref.Worktree,
		}
		if ref.OriginID != "" {
			rep.MigratedFromCC = append(rep.MigratedFromCC, row)
		} else {
			rep.Native = append(rep.Native, row)
		}
	}
	return rep, nil
}

// suppress unused import warnings in case future fields come in.
var _ = sql.ErrNoRows
var _ = domain.Hook{}