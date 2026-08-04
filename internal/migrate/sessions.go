// Package migrate is the orchestration layer that wires source readers
// and target writers together. It owns the user-facing workflow:
// discovery, filtering, planning, and execution. It does not know how
// individual records are parsed or persisted.
package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/source/claudecode"
	"github.com/mirhan/a2migrate/internal/target/opencode"
)

// Direction selects which side of a one-shot migration is the source and
// which is the target.
//
// This is distinct from sync.Direction (internal/sync): that type picks a
// last-writer-wins tie-break rule for continuous bidirectional sync
// (NewerWins/PreferCC/PreferOC), not a migration direction. The two are
// never interchangeable; always reference them package-qualified
// (migrate.Direction vs sync.Direction) where both are in scope.
type Direction string

const (
	// ToOpenCode migrates Claude Code sessions into OpenCode.
	ToOpenCode Direction = "to-opencode"
	// ToClaudeCode migrates OpenCode sessions back into Claude Code.
	ToClaudeCode Direction = "to-claudecode"
)

// Options configure a migration run in either Direction. Fields marked
// "ToOpenCode only" / "ToClaudeCode only" only affect that pipeline; the
// constructor for the other direction ignores them.
type Options struct {
	Direction   Direction // which pipeline runs; set by the constructor used, not by the caller
	From        string    // source override (CC home for ToOpenCode, OC db path for ToClaudeCode)
	To          string    // target override (OC db path for ToOpenCode, CC home for ToClaudeCode)
	BackupDir   string    // backup location before writing (empty = skip)
	DryRun      bool
	Yes         bool
	Renames     map[string]string // source session id -> new title
	Includes    []string          // source session ids to include
	Excludes    []string          // source session ids to skip
	Search      string            // case-insensitive substring filter
	SkipRepair  bool              // ToOpenCode only: skip post-fix invariants
	SkipNative  bool              // ToClaudeCode only: skip sessions without claude_code_origin
	Concurrency int               // reserved for future use
	Logger      *slog.Logger      // optional; defaults to slog.Default
}

// SessionResult is what a single migration run produced.
type SessionResult struct {
	OriginID        string
	Title           string
	IsSubagent      bool
	ProjectDir      string
	OCSessionID     string
	MessageCount    int
	PartCount       int
	ProjectCreated  bool
	AlreadyMigrated bool
	Error           error
}

// SessionReport summarises the full migration run.
type SessionReport struct {
	DryRun     bool
	Discovered int
	Selected   int
	Projects   int
	Successes  int
	Failures   int
	Skipped    int
	Reparents  int
	PadsStep   int
	StepStarts int
	ToolTimes  int
	BackupPath string
	Results    []SessionResult
}

// SessionMigrator orchestrates discovery → parse → plan → apply → repair.
type SessionMigrator struct {
	Source  *claudecode.SessionReader
	Options Options
	Logger  *slog.Logger
}

// NewSessionMigrator wires defaults for fields the caller left blank and
// pins Direction to ToOpenCode — this constructor only ever runs the
// CC -> OC pipeline, regardless of what the caller set.
func NewSessionMigrator(opts Options) *SessionMigrator {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	opts.Direction = ToOpenCode
	return &SessionMigrator{
		Source:  claudecode.NewSessionReader(opts.From),
		Options: opts,
		Logger:  opts.Logger,
	}
}

// Discover lists every session the migrator would consider migrating.
func (m *SessionMigrator) Discover(ctx context.Context) ([]claudecode.SessionRef, error) {
	refs, err := m.Source.DiscoverSessions()
	if err != nil {
		if errors.Is(err, claudecode.ErrNoSessions) {
			return nil, nil
		}
		return nil, err
	}
	return refs, nil
}

// Selected applies Include/Exclude/Search filters to refs.
// Subagent sessions whose parent was matched are pulled in automatically.
func (m *SessionMigrator) Selected(refs []claudecode.SessionRef) []claudecode.SessionRef {
	includes := setFromStrings(m.Options.Includes)
	excludes := setFromStrings(m.Options.Excludes)
	search := strings.ToLower(strings.TrimSpace(m.Options.Search))
	selected := map[string]bool{}
	var out []claudecode.SessionRef
	for _, r := range refs {
		if len(includes) > 0 && !includes[r.OriginID] {
			continue
		}
		if excludes[r.OriginID] {
			continue
		}
		if search != "" {
			hay := strings.ToLower(r.OriginID + " " + filepath.Base(r.FilePath))
			if !strings.Contains(hay, search) {
				continue
			}
		}
		selected[r.OriginID] = true
		out = append(out, r)
	}
	// Auto-include subagents whose parent was selected.
	for _, r := range refs {
		if r.IsSubagent && selected[r.ParentID] && !selected[r.OriginID] {
			selected[r.OriginID] = true
			out = append(out, r)
		}
	}
	// Re-sort for determinism (parent before subagent).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].FilePath != out[j].FilePath {
			return out[i].FilePath < out[j].FilePath
		}
		return out[i].OriginID < out[j].OriginID
	})
	return out
}

// ErrNoTarget is returned when no target database path was supplied.
var ErrNoTarget = errors.New("no target database path")

// Run executes the migration. It returns a report even on partial failure;
// check Failures and Results[i].Error for details.
func (m *SessionMigrator) Run(ctx context.Context, refs []claudecode.SessionRef) (*SessionReport, error) {
	report := &SessionReport{DryRun: m.Options.DryRun}
	refs = m.Selected(refs)
	report.Discovered = len(refs)
	report.Selected = len(refs)

	if len(refs) == 0 {
		return report, nil
	}

	if m.Options.To == "" {
		return report, ErrNoTarget
	}

	db, err := opencode.OpenDatabase(ctx, m.Options.To)
	if err != nil {
		return report, err
	}
	defer func() { _ = db.Close() }()

	if !m.Options.DryRun && m.Options.BackupDir != "" {
		backupPath, err := opencode.Backup(m.Options.To, m.Options.BackupDir)
		if err != nil {
			return report, fmt.Errorf("backup: %w", err)
		}
		report.BackupPath = backupPath
	}

	// Parse every selected session.
	var sessions []domain.Session
	for _, ref := range refs {
		sess, err := m.Source.ParseSession(ref.FilePath)
		if err != nil {
			report.Failures++
			report.Results = append(report.Results, SessionResult{
				OriginID: ref.OriginID,
				Error:    fmt.Errorf("parse %s: %w", ref.FilePath, err),
			})
			continue
		}
		if newTitle, ok := m.Options.Renames[ref.OriginID]; ok && newTitle != "" {
			sess.Title = newTitle
		}
		sessions = append(sessions, sess)
	}

	if m.Options.DryRun {
		w := opencode.NewSessionWriter(db)
		plan, err := w.PlanSessions(ctx, sessions)
		if err != nil {
			return report, err
		}
		report.Projects = len(plan.NewProjects)
		for _, r := range sessions {
			report.Results = append(report.Results, SessionResult{
				OriginID:     r.OriginID,
				Title:        r.Title,
				IsSubagent:   r.IsSubagent,
				ProjectDir:   r.ProjectDir,
				MessageCount: len(r.Messages),
				PartCount:    partCount(r),
			})
		}
		return report, nil
	}

	writer := opencode.NewSessionWriter(db)
	plan, err := writer.PlanSessions(ctx, sessions)
	if err != nil {
		return report, fmt.Errorf("plan: %w", err)
	}
	report.Projects = len(plan.NewProjects)
	if err := writer.Apply(ctx, plan); err != nil {
		report.Failures++
		report.Results = append(report.Results, SessionResult{Error: err})
		return report, err
	}

	ocByOrigin := map[string]string{}
	for _, s := range plan.Sessions {
		var meta struct {
			Origin string `json:"claude_code_origin"`
		}
		_ = json.Unmarshal([]byte(s.Metadata), &meta)
		if meta.Origin != "" {
			ocByOrigin[meta.Origin] = s.ID
		}
	}
	for _, r := range refs {
		ocID, ok := ocByOrigin[r.OriginID]
		if !ok {
			// Session may already be migrated (idempotent re-run).
			already, existingOC, _ := lookupMigrated(ctx, db, r.OriginID)
			if already {
				report.Skipped++
				report.Results = append(report.Results, SessionResult{
					OriginID:        r.OriginID,
					IsSubagent:      r.IsSubagent,
					ProjectDir:      r.Worktree,
					OCSessionID:     existingOC,
					AlreadyMigrated: true,
				})
				continue
			}
			report.Failures++
			report.Results = append(report.Results, SessionResult{
				OriginID:   r.OriginID,
				IsSubagent: r.IsSubagent,
				ProjectDir: r.Worktree,
				Error:      fmt.Errorf("no OC session id for origin %s", r.OriginID),
			})
			continue
		}
		sr := SessionResult{
			OriginID:    r.OriginID,
			IsSubagent:  r.IsSubagent,
			ProjectDir:  r.Worktree,
			OCSessionID: ocID,
		}
		for _, s := range sessions {
			if s.OriginID == r.OriginID {
				sr.Title = s.Title
				sr.MessageCount = len(s.Messages)
				sr.PartCount = partCount(s)
				sr.ProjectCreated = true
				break
			}
		}
		report.Successes++
		report.Results = append(report.Results, sr)
	}

	if !m.Options.SkipRepair {
		rep, err := opencode.Repair(ctx, db, nil)
		if err != nil {
			m.Logger.Warn("repair failed", "err", err)
		} else {
			report.Reparents = rep.Reparents
			report.PadsStep = rep.PadsStepParts
			report.StepStarts = rep.AddedStepStartTimes
			report.ToolTimes = rep.AddedToolStateTimes
		}
	}

	return report, nil
}

func partCount(s domain.Session) int {
	n := 0
	for _, m := range s.Messages {
		n += len(m.Parts)
	}
	return n
}

func setFromStrings(xs []string) map[string]bool {
	out := map[string]bool{}
	for _, x := range xs {
		out[x] = true
	}
	return out
}

// ParsedRenames converts --rename old=new (repeatable) into a map.
// Returns an error if any spec is malformed.
func ParsedRenames(specs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, s := range specs {
		idx := strings.Index(s, "=")
		if idx < 0 {
			return nil, fmt.Errorf("invalid --rename %q: expected old=new", s)
		}
		out[s[:idx]] = s[idx+1:]
	}
	return out, nil
}

// lookupMigrated returns (alreadyMigrated, ocSessionID, error) for an origin.
func lookupMigrated(ctx context.Context, db *sql.DB, originID string) (bool, string, error) {
	var ocID string
	err := db.QueryRowContext(ctx,
		`SELECT id FROM session
		 WHERE json_extract(metadata, '$.claude_code_origin') = ?`,
		originID,
	).Scan(&ocID)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, ocID, nil
}

// Verify checks what's already in the target DB.
type VerifyReport struct {
	Migrated []VerifyRow
}

type VerifyRow struct {
	OCSessionID  string
	OriginID     string
	IsSubagent   bool
	ParentOrigin string
	Title        string
	ProjectID    string
	UpdatedAt    int64
}

// Verify returns one row per migrated session in the target DB.
func Verify(ctx context.Context, dbPath string) (*VerifyReport, error) {
	db, err := opencode.OpenDatabase(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(ctx,
		`SELECT id, metadata, title, project_id, time_updated
		 FROM session WHERE metadata LIKE '%claude_code_origin%'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out VerifyReport
	for rows.Next() {
		var (
			id, metadata, title, projectID string
			updated                        int64
		)
		if err := rows.Scan(&id, &metadata, &title, &projectID, &updated); err != nil {
			return nil, err
		}
		var meta struct {
			Origin     string `json:"claude_code_origin"`
			Parent     string `json:"claude_code_parent"`
			IsSubagent bool   `json:"is_subagent"`
		}
		_ = json.Unmarshal([]byte(metadata), &meta)
		out.Migrated = append(out.Migrated, VerifyRow{
			OCSessionID:  id,
			OriginID:     meta.Origin,
			IsSubagent:   meta.IsSubagent,
			ParentOrigin: meta.Parent,
			Title:        title,
			ProjectID:    projectID,
			UpdatedAt:    updated,
		})
	}
	return &out, rows.Err()
}
