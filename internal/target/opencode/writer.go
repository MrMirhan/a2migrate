package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/mirhan/a2migrate/internal/domain"
)

// Plan is the in-memory description of all rows that an Apply will insert.
// All IDs are pre-generated and unique within the plan.
type Plan struct {
	NewProjects []projectRow
	Sessions    []sessionRow
	Messages    []messageRow
	Parts       []partRow
}

// projectRow describes one INSERT into `project`.
type projectRow struct {
	ID          string
	Worktree    string
	Name        string
	TimeCreated int64
	TimeUpdated int64
	Sandboxes   string
	Commands    *string
}

// sessionRow describes one INSERT into `session`.
type sessionRow struct {
	ID          string
	ProjectID   string
	ParentID    *string
	Slug        string
	Directory   string
	Path        string
	Title       string
	Version     string
	Metadata    string
	Agent       string
	TimeCreated int64
	TimeUpdated int64
}

// messageRow describes one INSERT into `message`.
type messageRow struct {
	ID          string
	SessionID   string
	TimeCreated int64
	TimeUpdated int64
	Data        string
}

// partRow describes one INSERT into `part`.
type partRow struct {
	ID          string
	MessageID   string
	SessionID   string
	TimeCreated int64
	TimeUpdated int64
	Data        string
}

// SessionWriter persists domain.Session values into an OpenCode SQLite db.
type SessionWriter struct {
	DB         *sql.DB
	Origin     string
	BackupPath string
	Logger     *slog.Logger
}

// NewSessionWriter wires a writer to an open database. Origin defaults to
// "claude_code" if left empty.
func NewSessionWriter(db *sql.DB) *SessionWriter {
	return &SessionWriter{
		DB:     db,
		Origin: "claude_code",
		Logger: slog.Default(),
	}
}

// Backup copies the SQLite database file at dbPath to a timestamped sibling.
// WAL-safe against quiescent writers; caller is expected to stop OpenCode
// before invoking for the live migration. Idempotent: if the destination
// already exists, the call is a no-op.
func Backup(dbPath, dstDir string) (string, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return "", nil
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	base := filepath.Base(dbPath)
	dst := filepath.Join(dstDir, base+".bak-"+stamp)

	in, err := os.Open(dbPath)
	if err != nil {
		return "", fmt.Errorf("open source %s: %w", dbPath, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("open dest %s: %w", dst, err)
	}
	defer func() { _ = out.Close() }()
	if _, err := out.ReadFrom(in); err != nil {
		return "", err
	}
	return dst, nil
}

// PlanSessions converts sessions into rows ready for INSERT. existingIDs is
// loaded by the caller via ExistingIDs(ctx, db) so the planner knows the
// current id space.
func (w *SessionWriter) PlanSessions(ctx context.Context, sessions []domain.Session) (Plan, error) {
	existing, err := ExistingIDs(ctx, w.DB)
	if err != nil {
		return Plan{}, err
	}
	projects, err := ExistingProjectIDs(ctx, w.DB)
	if err != nil {
		return Plan{}, err
	}
	migrated, err := ExistingOriginIDs(ctx, w.DB)
	if err != nil {
		return Plan{}, err
	}
	return w.planWithIDs(sessions, existing, projects, migrated)
}

func (w *SessionWriter) planWithIDs(sessions []domain.Session, existing map[string]struct{}, existingProjects map[string]struct{}, migrated map[string]struct{}) (Plan, error) {
	plan := Plan{}
	seenProjects := map[string]bool{}

	mainOCIDByCCOrigin := map[string]string{}

	addProject := func(s *domain.Session) {
		pid := ProjectIDForWorktree(s.ProjectDir)
		if seenProjects[pid] {
			return
		}
		if _, ok := existingProjects[pid]; ok {
			seenProjects[pid] = true
			return
		}
		seenProjects[pid] = true
		plan.NewProjects = append(plan.NewProjects, projectRow{
			ID:          pid,
			Worktree:    s.ProjectDir,
			Name:        filepath.Base(s.ProjectDir),
			TimeCreated: s.CreatedAt.UnixMilli(),
			TimeUpdated: s.UpdatedAt.UnixMilli(),
			Sandboxes:   "[]",
		})
	}

	for i := range sessions {
		s := &sessions[i]
		if s.IsSubagent {
			continue
		}
		if _, ok := migrated[s.OriginID]; ok {
			continue
		}
		mainOCIDByCCOrigin[s.OriginID] = ""
	}

	for i := range sessions {
		s := &sessions[i]
		if s.IsSubagent {
			continue
		}
		if _, ok := migrated[s.OriginID]; ok {
			continue
		}
		row, err := w.buildSessionRow(s, "", existing)
		if err != nil {
			return Plan{}, err
		}
		mainOCIDByCCOrigin[s.OriginID] = row.ID
		plan.Sessions = append(plan.Sessions, row)
		addProject(s)
	}

	for i := range sessions {
		s := &sessions[i]
		if !s.IsSubagent {
			continue
		}
		if _, ok := migrated[s.OriginID]; ok {
			continue
		}
		parentOC := mainOCIDByCCOrigin[s.ParentID]
		row, err := w.buildSessionRow(s, parentOC, existing)
		if err != nil {
			return Plan{}, err
		}
		mainOCIDByCCOrigin[s.OriginID] = row.ID
		plan.Sessions = append(plan.Sessions, row)
		addProject(s)
	}

	// Build messages + parts per session.
	for i := range sessions {
		s := &sessions[i]
		if _, ok := migrated[s.OriginID]; ok {
			continue
		}
		row := findSessionRow(plan.Sessions, mainOCIDByCCOrigin[s.OriginID])
		if row == nil {
			return Plan{}, fmt.Errorf("internal: missing session row for %s", s.OriginID)
		}
		msgs, parts, err := w.buildMessageRows(s, row, existing)
		if err != nil {
			return Plan{}, err
		}
		plan.Messages = append(plan.Messages, msgs...)
		plan.Parts = append(plan.Parts, parts...)
	}

	return plan, nil
}

func findSessionRow(rows []sessionRow, id string) *sessionRow {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i]
		}
	}
	return nil
}

func (w *SessionWriter) buildSessionRow(s *domain.Session, parentOC string, existing map[string]struct{}) (sessionRow, error) {
	slug := w.slugFor(s)
	id := GenID("ses", s.OriginID, existing)
	metadata := map[string]any{
		"claude_code_origin": s.OriginID,
		"is_subagent":        s.IsSubagent,
	}
	if s.IsSubagent && s.ParentID != "" {
		metadata["claude_code_parent"] = s.ParentID
	}
	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		return sessionRow{}, err
	}
	var parentPtr *string
	if parentOC != "" {
		parentPtr = &parentOC
	}
	return sessionRow{
		ID:          id,
		ProjectID:   ProjectIDForWorktree(s.ProjectDir),
		ParentID:    parentPtr,
		Slug:        slug,
		Directory:   s.ProjectDir,
		Path:        "",
		Title:       s.Title,
		Version:     "claude-code-migrated",
		Metadata:    string(metaBytes),
		Agent:       "build",
		TimeCreated: msOrZero(s.CreatedAt),
		TimeUpdated: msOrZero(s.UpdatedAt),
	}, nil
}

func msOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func (w *SessionWriter) slugFor(s *domain.Session) string {
	titleSlug := domain.Slugify(s.Title)
	if len(titleSlug) > 30 {
		titleSlug = titleSlug[:30]
	}
	body := Hash16(s.OriginID)[:8] + "-" + titleSlug
	// Sanitize collision suffix outside any loop; Apply() detects PRIMARY
	// KEY failures on the OC id (not the slug), so the slug is just for
	// display and may be repeated across sessions without harm.
	return body
}

// buildMessageRows emits message + part rows for one session.
func (w *SessionWriter) buildMessageRows(s *domain.Session, row *sessionRow, existing map[string]struct{}) ([]messageRow, []partRow, error) {
	var msgs []messageRow
	var parts []partRow
	prevID := ""
	for i := range s.Messages {
		m := &s.Messages[i]
		if len(m.Parts) == 0 {
			continue
		}
		msgID := GenID("msg", s.OriginID+":"+m.OriginID, existing)
		data, err := buildMessageData(m, prevID)
		if err != nil {
			return nil, nil, fmt.Errorf("session %s message %s: %w", s.OriginID, m.OriginID, err)
		}
		msgs = append(msgs, messageRow{
			ID:          msgID,
			SessionID:   row.ID,
			TimeCreated: msOrZero(m.CreatedAt),
			TimeUpdated: msOrZero(m.CreatedAt),
			Data:        data,
		})
		prevID = msgID

		if m.Role == domain.RoleAssistant {
			stepStart := partRow{
				ID:        GenID("prt", s.OriginID+":"+m.OriginID+":step-start", existing),
				MessageID: msgID,
				SessionID: row.ID,
				Data:      `{"type":"step-start"}`,
			}
			parts = append(parts, stepStart)
		}

		for j := range m.Parts {
			p := &m.Parts[j]
			prtID := GenID("prt", s.OriginID+":"+m.OriginID+":"+string(p.Type)+":"+partSeed(p, j), existing)
			pd, err := buildPartData(p)
			if err != nil {
				return nil, nil, err
			}
			parts = append(parts, partRow{
				ID:        prtID,
				MessageID: msgID,
				SessionID: row.ID,
				Data:      pd,
			})
		}

		if m.Role == domain.RoleAssistant {
			parts = append(parts, partRow{
				ID:        GenID("prt", s.OriginID+":"+m.OriginID+":step-finish", existing),
				MessageID: msgID,
				SessionID: row.ID,
				Data:      `{"type":"step-finish"}`,
			})
		}
	}
	return msgs, parts, nil
}

func partSeed(p *domain.Part, idx int) string {
	if p.ToolCallID != "" {
		return p.ToolCallID
	}
	return fmt.Sprintf("idx-%d", idx)
}

// buildMessageData renders the JSON blob for a message's data column.
func buildMessageData(m *domain.Message, parentID string) (string, error) {
	var raw map[string]any
	switch m.Role {
	case domain.RoleUser:
		raw = map[string]any{
			"role":  "user",
			"time":  map[string]any{"created": msOrZero(m.CreatedAt)},
			"agent": "build",
			"model": map[string]any{
				"providerID": m.ProviderID,
				"modelID":    m.ModelID,
				"variant":    orDefault(m.Variant, "thinking"),
			},
			"summary": map[string]any{"diffs": []any{}},
		}
	case domain.RoleAssistant:
		raw = map[string]any{
			"role":    "assistant",
			"mode":    "build",
			"agent":   orDefault(m.Agent, "build"),
			"variant": orDefault(m.Variant, "thinking"),
			"path":    map[string]any{"cwd": "", "root": "/"},
			"cost":    m.CostUSD,
			"tokens": map[string]any{
				"input":     m.Tokens.Input,
				"output":    m.Tokens.Output,
				"reasoning": m.Tokens.Reasoning,
				"cache": map[string]any{
					"read":  m.Tokens.CacheRead,
					"write": m.Tokens.CacheWrite,
				},
			},
			"modelID":    m.ModelID,
			"providerID": m.ProviderID,
			"time":       map[string]any{"created": msOrZero(m.CreatedAt)},
		}
		if m.Tokens.ServiceTier != "" {
			raw["serviceTier"] = m.Tokens.ServiceTier
		}
		if m.Tokens.Speed != "" {
			raw["speed"] = m.Tokens.Speed
		}
	default:
		return "", fmt.Errorf("unsupported role %s", m.Role)
	}
	if parentID != "" {
		raw["parentID"] = parentID
	}
	if m.OriginID != "" {
		raw["ccOriginID"] = m.OriginID
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// buildPartData renders the JSON blob for a part's data column.
func buildPartData(p *domain.Part) (string, error) {
	switch p.Type {
	case domain.PartText:
		return jsonString(map[string]any{"type": "text", "text": p.Text})
	case domain.PartReasoning:
		return jsonString(map[string]any{"type": "reasoning", "text": p.Text})
	case domain.PartTool:
		if p.ToolName == "" {
			return "", fmt.Errorf("tool part requires ToolName")
		}
		state := map[string]any{
			"status": orDefault(p.ToolStatus, "completed"),
			"input":  p.ToolInput,
			"output": p.ToolOutput,
			"title":  p.ToolName,
		}
		return jsonString(map[string]any{
			"type":   "tool",
			"tool":   p.ToolName,
			"callID": p.ToolCallID,
			"state":  state,
		})
	default:
		return "", fmt.Errorf("unsupported part type %s", p.Type)
	}
}

func jsonString(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Apply runs the plan in a single transaction. On any error the transaction
// is rolled back; on success, all rows are committed atomically.
func (w *SessionWriter) Apply(ctx context.Context, plan Plan) error {
	if w.BackupPath != "" {
		if _, err := Backup(deriveDBPath(w.DB), w.BackupPath); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
	}
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for _, p := range plan.NewProjects {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO project(id, worktree, name, time_created, time_updated, sandboxes)
			 VALUES(?, ?, ?, ?, ?, ?)`,
			p.ID, p.Worktree, p.Name, p.TimeCreated, p.TimeUpdated, p.Sandboxes)
		if err != nil {
			return fmt.Errorf("insert project %s: %w", p.ID, err)
		}
	}

	for _, s := range plan.Sessions {
		var parentVal any
		if s.ParentID != nil {
			parentVal = *s.ParentID
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO session(
				id, project_id, parent_id, slug, directory, path, title, version,
				metadata, agent, time_created, time_updated)
			 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, s.ProjectID, parentVal, s.Slug, s.Directory, s.Path,
			s.Title, s.Version, s.Metadata, s.Agent, s.TimeCreated, s.TimeUpdated)
		if err != nil {
			return fmt.Errorf("insert session %s: %w", s.ID, err)
		}
	}

	for _, m := range plan.Messages {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO message(id, session_id, time_created, time_updated, data)
			 VALUES(?, ?, ?, ?, ?)`,
			m.ID, m.SessionID, m.TimeCreated, m.TimeUpdated, m.Data)
		if err != nil {
			return fmt.Errorf("insert message %s: %w", m.ID, err)
		}
	}

	for _, p := range plan.Parts {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO part(id, message_id, session_id, time_created, time_updated, data)
			 VALUES(?, ?, ?, ?, ?, ?)`,
			p.ID, p.MessageID, p.SessionID, p.TimeCreated, p.TimeUpdated, p.Data)
		if err != nil {
			return fmt.Errorf("insert part %s: %w", p.ID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	w.Logger.Info("apply complete",
		"projects", len(plan.NewProjects),
		"sessions", len(plan.Sessions),
		"messages", len(plan.Messages),
		"parts", len(plan.Parts))
	return nil
}

// deriveDBPath extracts the file path from a *sql.DB connection. Best-effort:
// falls back to "" if introspection fails.
func deriveDBPath(db *sql.DB) string {
	rows, err := db.Query("PRAGMA database_list")
	if err != nil {
		return ""
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var seq int
		var name, path string
		if err := rows.Scan(&seq, &name, &path); err == nil {
			return path
		}
	}
	return ""
}
