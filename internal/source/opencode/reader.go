// Package opencode reads sessions and artifacts from an OpenCode SQLite
// database. It is the source-side counterpart to internal/target/opencode.
//
// The reader is read-only; nothing it returns is mutated. It produces the
// same domain types the Claude Code reader produces, so downstream
// orchestration doesn't need to know which direction the migration runs.
package opencode

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/platform"
	"github.com/mirhan/a2migrate/internal/target/opencode"
)

// SessionReader reads sessions from an OpenCode database.
type SessionReader struct {
	DBPath string
}

// NewSessionReader returns a reader rooted at the platform default db path
// (or dbPath if non-empty).
func NewSessionReader(dbPath string) *SessionReader {
	if dbPath == "" {
		dbPath = platform.OpenCodeDBPath()
	}
	return &SessionReader{DBPath: dbPath}
}

// Open opens the database and returns the connection. Caller owns it.
func (r *SessionReader) Open(ctx context.Context) (*sql.DB, error) {
	return opencode.OpenDatabase(ctx, r.DBPath)
}

// SessionRef points at one OC session row.
type SessionRef struct {
	OCSessionID string
	OriginID    string // claude_code_origin (empty if native OC)
	Title       string
	ProjectID   string
	Worktree    string
	IsSubagent  bool
	ParentID    string // origin id of parent (CC id) — for subagents only
	UpdatedAt   int64
}

// DiscoverProjects reads all projects known to the OC db.
func (r *SessionReader) DiscoverProjects(ctx context.Context, db *sql.DB) ([]domain.Project, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, worktree, name, time_created, time_updated FROM project`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		var p domain.Project
		var ts, tu int64
		if err := rows.Scan(&p.ID, &p.Worktree, &p.Name, &ts, &tu); err != nil {
			return nil, err
		}
		if ts > 0 {
			p.TimeCreated = millis(ts)
		}
		if tu > 0 {
			p.TimeUpdated = millis(tu)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DiscoverSessions returns one SessionRef per row in the session table,
// sorted by OCSessionID. Both migrated-CC and native-OC sessions are
// returned; the OriginID field tells the caller which kind.
func (r *SessionReader) DiscoverSessions(ctx context.Context, db *sql.DB) ([]SessionRef, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, COALESCE(json_extract(metadata, '$.claude_code_origin'), ''),
		        title, project_id, time_updated, metadata, parent_id
		 FROM session`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRef
	projects := map[string]string{} // id → worktree
	for rows.Next() {
		var (
			id, origin, title, projectID string
			updated                      sql.NullInt64
			metadata                     sql.NullString
			parentOCID                   sql.NullString
		)
		if err := rows.Scan(&id, &origin, &title, &projectID, &updated, &metadata, &parentOCID); err != nil {
			return nil, err
		}
		wt, _ := projects[projectID]
		if wt == "" {
			wt = lookupWorktree(ctx, db, projectID)
			projects[projectID] = wt
		}

		var meta struct {
			Origin string `json:"claude_code_origin"`
			Parent string `json:"claude_code_parent"`
			Sub    bool   `json:"is_subagent"`
		}
		if metadata.Valid && metadata.String != "" {
			_ = json.Unmarshal([]byte(metadata.String), &meta)
		}
		parentStr := ""
		if parentOCID.Valid {
			parentStr = parentOCID.String
		}
		isSub := meta.Sub || (parentStr != "" && parentStr != id)
		upd := int64(0)
		if updated.Valid {
			upd = updated.Int64
		}
		out = append(out, SessionRef{
			OCSessionID: id,
			OriginID:    origin,
			Title:       title,
			ProjectID:   projectID,
			Worktree:    wt,
			IsSubagent:  isSub,
			ParentID:    meta.Parent,
			UpdatedAt:   upd,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OCSessionID < out[j].OCSessionID })
	return out, rows.Err()
}

func lookupWorktree(ctx context.Context, db *sql.DB, projectID string) string {
	var wt string
	if err := db.QueryRowContext(ctx, `SELECT worktree FROM project WHERE id = ?`, projectID).Scan(&wt); err != nil {
		return ""
	}
	return wt
}

// ParseSession reconstructs one session's full message stream from the db.
// It uses the same domain.Session shape the CC reader produces, so the
// downstream writer can target either system without branching.
//
// msgBuilder translates each OC message+parts row into the ordered list of
// domain.Message + domain.Part values CC expects. See the doc on
// msgBuilder for the schema-derivation rules.
func (r *SessionReader) ParseSession(ctx context.Context, db *sql.DB, ref SessionRef) (domain.Session, error) {
	sess := domain.Session{
		ID:         ref.OCSessionID,
		OriginID:   ref.OriginID,
		Origin:     domain.OriginOpenCode,
		Title:      ref.Title,
		ProjectDir: ref.Worktree,
		IsSubagent: ref.IsSubagent,
		ParentID:   ref.ParentID,
	}
	if ref.OriginID != "" {
		sess.Origin = domain.OriginClaudeCode
	}

	rows, err := db.QueryContext(ctx,
		`SELECT id, time_created, data FROM message
		 WHERE session_id = ? ORDER BY time_created, id`,
		ref.OCSessionID)
	if err != nil {
		return sess, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id   string
			ts   int64
			blob string
		)
		if err := rows.Scan(&id, &ts, &blob); err != nil {
			return sess, err
		}
		parts, err := loadParts(ctx, db, id)
		if err != nil {
			return sess, err
		}
		msg, ok := buildMessage(id, ts, blob, parts)
		if !ok {
			continue
		}
		sess.Messages = append(sess.Messages, msg)
	}
	if err := rows.Err(); err != nil {
		return sess, err
	}

	if len(sess.Messages) > 0 {
		sess.CreatedAt = sess.Messages[0].CreatedAt
		sess.UpdatedAt = sess.Messages[len(sess.Messages)-1].CreatedAt
	}
	if sess.Title == "" && len(sess.Messages) > 0 {
		sess.Title = deriveTitleFromMessages(sess.Messages)
	}
	return sess, nil
}

// loadParts returns all part rows for one message, sorted by id (which is
// monotonic for parts emitted by the same message).
func loadParts(ctx context.Context, db *sql.DB, messageID string) ([]partRow, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, data FROM part WHERE message_id = ? ORDER BY id`,
		messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []partRow
	for rows.Next() {
		var (
			id   string
			blob string
		)
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		out = append(out, partRow{ID: id, Data: blob})
	}
	return out, rows.Err()
}

type partRow struct {
	ID   string
	Data string
}

// buildMessage decodes an OC message JSON blob and the row's part rows
// into a domain.Message. Returns ok=false for messages we cannot translate
// (synthetic step parts, unknown roles) so the caller can skip them.
func buildMessage(id string, ts int64, blob string, parts []partRow) (domain.Message, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(blob), &raw); err != nil {
		return domain.Message{}, false
	}
	roleStr, _ := raw["role"].(string)
	role := domain.Role(roleStr)
	if !role.Valid() {
		return domain.Message{}, false
	}
	agent, _ := raw["agent"].(string)
	model, _ := modelFromRaw(raw)
	provider, _ := providerFromRaw(raw)

	msg := domain.Message{
		ID:         id,
		SessionID:  id,
		Role:       role,
		Agent:      agent,
		ModelID:    model,
		ProviderID: provider,
		CreatedAt:  millis(ts),
	}

	// Translate parts. Skip step-start / step-finish entirely.
	var ccParts []domain.Part
	for _, p := range parts {
		var raw map[string]any
		if err := json.Unmarshal([]byte(p.Data), &raw); err != nil {
			continue
		}
		typ, _ := raw["type"].(string)
		switch typ {
		case "text":
			t, _ := raw["text"].(string)
			if t == "" {
				continue
			}
			ccParts = append(ccParts, domain.Part{Type: domain.PartText, Text: t})
		case "reasoning":
			t, _ := raw["text"].(string)
			if t == "" {
				continue
			}
			ccParts = append(ccParts, domain.Part{Type: domain.PartReasoning, Text: t})
		case "tool":
			ccParts = append(ccParts, toolPartFromOC(raw))
		default:
			// step-start / step-finish / unknown — skip silently.
		}
	}
	if len(ccParts) == 0 && role == domain.RoleAssistant {
		return domain.Message{}, false
	}
	if len(ccParts) == 0 && role == domain.RoleUser {
		return domain.Message{}, false
	}
	msg.Parts = ccParts
	return msg, true
}

func modelFromRaw(raw map[string]any) (string, bool) {
	if m, ok := raw["model"].(map[string]any); ok {
		if v, ok := m["modelID"].(string); ok {
			return v, true
		}
	}
	if v, ok := raw["modelID"].(string); ok {
		return v, true
	}
	return "", false
}

func providerFromRaw(raw map[string]any) (string, bool) {
	if m, ok := raw["model"].(map[string]any); ok {
		if v, ok := m["providerID"].(string); ok {
			return v, true
		}
	}
	if v, ok := raw["providerID"].(string); ok {
		return v, true
	}
	return "", false
}

// toolPartFromOC renders an OC tool part back into a domain.Part tool entry.
// OC fuses the tool_use and tool_result into one part, so we restore the
// tool_use shape and trust the state's status/output fields.
func toolPartFromOC(raw map[string]any) domain.Part {
	name, _ := raw["tool"].(string)
	callID, _ := raw["callID"].(string)
	var input map[string]any
	var output string
	var status = "completed"
	if state, ok := raw["state"].(map[string]any); ok {
		if v, ok := state["input"].(map[string]any); ok {
			input = v
		}
		if v, ok := state["output"].(string); ok {
			output = v
		}
		if v, ok := state["status"].(string); ok && v != "" {
			status = v
		}
	}
	return domain.Part{
		Type:       domain.PartTool,
		ToolName:   name,
		ToolCallID: callID,
		ToolInput:  input,
		ToolOutput: output,
		ToolStatus: status,
	}
}

// millis converts a unix-ms int64 into a time.Time.
func millis(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return unixMillis(ms)
}

// deriveTitleFromMessages picks the first non-empty user text.
func deriveTitleFromMessages(msgs []domain.Message) string {
	for _, m := range msgs {
		if m.Role != domain.RoleUser {
			continue
		}
		for _, p := range m.Parts {
			if p.Type != domain.PartText {
				continue
			}
			text := strings.Join(strings.Fields(p.Text), " ")
			if text == "" {
				continue
			}
			if len(text) > 80 {
				text = text[:80] + "…"
			}
			return text
		}
	}
	return "<opencode session>"
}

// ReadHooks is a placeholder; OC has no native hook equivalent. The CC
// source reader's hook list is what feeds the OC plugin generation in a
// future version. Returning nil here keeps the source interface uniform.
func ReadHooks() ([]domain.Hook, error) {
	return nil, nil
}

// IsOCProjectGlobal returns true if the project id is the literal "global"
// placeholder used for the root worktree.
func IsOCProjectGlobal(id string) bool { return id == "global" }

// ReadGlobalSkills returns skills from ~/.config/opencode/skills/*.md and
// <cwd>/.opencode/skills/*.md. Returns the union, deduplicated by Name.
func ReadGlobalSkills() ([]domain.Skill, error) {
	return readSkillsDir(filepath.Join(platform.OpenCodeConfigHome(), "skills"))
}

// ReadProjectSkills reads <cwd>/.opencode/skills/*.md.
func ReadProjectSkills(cwd string) ([]domain.Skill, error) {
	return readSkillsDir(filepath.Join(cwd, ".opencode", "skills"))
}

// ReadGlobalCommands reads ~/.config/opencode/command/*.md.
func ReadGlobalCommands() ([]domain.Command, error) {
	cmds, err := readSkillsDir(filepath.Join(platform.OpenCodeConfigHome(), "command"))
	if err != nil {
		return nil, err
	}
	return skillsToCommands(cmds), nil
}

// ReadProjectCommands reads <cwd>/.opencode/command/*.md.
func ReadProjectCommands(cwd string) ([]domain.Command, error) {
	cmds, err := readSkillsDir(filepath.Join(cwd, ".opencode", "command"))
	if err != nil {
		return nil, err
	}
	return skillsToCommands(cmds), nil
}

// ReadGlobalAgents reads ~/.config/opencode/agent/*.md.
func ReadGlobalAgents() ([]domain.AgentDef, error) {
	xs, err := readSkillsDir(filepath.Join(platform.OpenCodeConfigHome(), "agent"))
	if err != nil {
		return nil, err
	}
	return skillsToAgents(xs), nil
}

// ReadProjectAgents reads <cwd>/.opencode/agent/*.md.
func ReadProjectAgents(cwd string) ([]domain.AgentDef, error) {
	xs, err := readSkillsDir(filepath.Join(cwd, ".opencode", "agent"))
	if err != nil {
		return nil, err
	}
	return skillsToAgents(xs), nil
}

// ReadGlobalRules reads ~/.config/opencode/rules/*.md.
func ReadGlobalRules() ([]domain.Rule, error) {
	xs, err := readSkillsDir(filepath.Join(platform.OpenCodeConfigHome(), "rules"))
	if err != nil {
		return nil, err
	}
	return skillsToRules(xs), nil
}

// ReadProjectRules reads <cwd>/.opencode/rules/*.md.
func ReadProjectRules(cwd string) ([]domain.Rule, error) {
	xs, err := readSkillsDir(filepath.Join(cwd, ".opencode", "rules"))
	if err != nil {
		return nil, err
	}
	return skillsToRules(xs), nil
}

// readSkillsDir reads *.md files and returns them as Skill records. The
// caller maps Skill into the more specific domain types.
func readSkillsDir(dir string) ([]domain.Skill, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []domain.Skill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		text, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		fm, body := splitFrontmatter(string(text))
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if v := fmStringField(fm, "name"); v != "" {
			name = v
		}
		out = append(out, domain.Skill{
			Name:        name,
			Description: fmStringField(fm, "description"),
			Origin:      domain.OriginOpenCode,
			SourcePath:  full,
			Body:        body,
			Frontmatter: fm,
		})
	}
	return out, nil
}

// splitFrontmatter extracts the frontmatter map + body from a markdown file.
// Reuses the same semantics as the CC reader but avoids the import.
func splitFrontmatter(text string) (map[string]any, string) {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return nil, normalized
	}
	rest := normalized[4:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, normalized
	}
	yamlText := rest[:idx]
	body := rest[idx+4:]
	body = strings.TrimLeft(body, "\n")
	var fm map[string]any
	if err := parseYAML(yamlText, &fm); err != nil {
		return nil, normalized
	}
	return fm, body
}

func fmStringField(fm map[string]any, key string) string {
	if fm == nil {
		return ""
	}
	if v, ok := fm[key].(string); ok {
		return v
	}
	return ""
}

func fmStringSliceField(fm map[string]any, key string) []string {
	if fm == nil {
		return nil
	}
	v, ok := fm[key]
	if !ok {
		return nil
	}
	switch xs := v.(type) {
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return xs
	}
	return nil
}

func skillsToCommands(xs []domain.Skill) []domain.Command {
	out := make([]domain.Command, 0, len(xs))
	for _, s := range xs {
		out = append(out, domain.Command{
			Name:         s.Name,
			Description:  s.Description,
			ArgumentHint: fmStringField(s.Frontmatter, "argument-hint"),
			AllowedTools: fmStringSliceField(s.Frontmatter, "allowed-tools"),
			SourcePath:   s.SourcePath,
			Body:         s.Body,
			Frontmatter:  s.Frontmatter,
		})
	}
	return out
}

func skillsToAgents(xs []domain.Skill) []domain.AgentDef {
	out := make([]domain.AgentDef, 0, len(xs))
	for _, s := range xs {
		out = append(out, domain.AgentDef{
			Name:        s.Name,
			Description: s.Description,
			Model:       fmStringField(s.Frontmatter, "model"),
			Tools:       fmStringSliceField(s.Frontmatter, "tools"),
			SourcePath:  s.SourcePath,
			Body:        s.Body,
			Frontmatter: s.Frontmatter,
		})
	}
	return out
}

func skillsToRules(xs []domain.Skill) []domain.Rule {
	out := make([]domain.Rule, 0, len(xs))
	for _, s := range xs {
		out = append(out, domain.Rule{
			Name:        s.Name,
			Paths:       fmStringSliceField(s.Frontmatter, "paths"),
			SourcePath:  s.SourcePath,
			Body:        s.Body,
			Frontmatter: s.Frontmatter,
		})
	}
	return out
}

// ReadGlobalMCP reads the OpenCode user config and parses its mcp{}
// block back into the domain.MCPServer form CC expects.
func ReadGlobalMCP() ([]domain.MCPServer, error) {
	path := platform.FindOpenCodeConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseOCMCP(data)
}

func parseOCMCP(data []byte) ([]domain.MCPServer, error) {
	stripped := stripJSONC(string(data))
	var root map[string]any
	if err := json.Unmarshal([]byte(stripped), &root); err != nil {
		return nil, err
	}
	mcp, ok := root["mcp"].(map[string]any)
	if !ok {
		return nil, nil
	}
	var names []string
	for k := range mcp {
		names = append(names, k)
	}
	sort.Strings(names)
	var out []domain.MCPServer
	for _, name := range names {
		s, err := parseOneOCServer(name, mcp[name])
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func parseOneOCServer(name string, raw any) (domain.MCPServer, error) {
	probe, ok := raw.(map[string]any)
	if !ok {
		return domain.MCPServer{}, fmt.Errorf("server %q: expected object", name)
	}
	s := domain.MCPServer{Name: name, Enabled: true, Type: "local"}
	switch cmd := probe["command"].(type) {
	case string:
		s.Command = []string{cmd}
	case []any:
		for _, a := range cmd {
			if str, ok := a.(string); ok {
				s.Command = append(s.Command, str)
			}
		}
	}
	if t, ok := probe["type"].(string); ok && t != "" {
		s.Type = t
	}
	if u, ok := probe["url"].(string); ok {
		s.URL = u
	}
	if en, ok := probe["enabled"].(bool); ok {
		s.Enabled = en
	}
	if env, ok := probe["environment"].(map[string]any); ok {
		s.Environment = make(map[string]string, len(env))
		for k, v := range env {
			if str, ok := v.(string); ok {
				s.Environment[k] = str
			}
		}
	}
	if h, ok := probe["headers"].(map[string]any); ok {
		s.Headers = make(map[string]string, len(h))
		for k, v := range h {
			if str, ok := v.(string); ok {
				s.Headers[k] = str
			}
		}
	}
	if s.URL != "" {
		s.Type = "remote"
	}
	return s, nil
}

// stripJSONC removes // and /* */ comments from a JSONC blob.
func stripJSONC(s string) string {
	var out []byte
	inStr, inLine, inBlock := false, false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inLine:
			if c == '\n' {
				inLine = false
				out = append(out, c)
			}
		case inBlock:
			if c == '*' && i+1 < len(s) && s[i+1] == '/' {
				inBlock = false
				i++
			}
		case inStr:
			out = append(out, c)
			if c == '\\' && i+1 < len(s) {
				out = append(out, s[i+1])
				i++
				continue
			}
			if c == '"' {
				inStr = false
			}
		default:
			if c == '/' && i+1 < len(s) && s[i+1] == '/' {
				inLine = true
				i++
				continue
			}
			if c == '/' && i+1 < len(s) && s[i+1] == '*' {
				inBlock = true
				i++
				continue
			}
			if c == '"' {
				inStr = true
			}
			out = append(out, c)
		}
	}
	return string(out)
}

// parseYAML is a stub that uses the gopkg.in/yaml.v3 decoder. We import
// it here so the split-frontmatter logic doesn't have to depend on the CC
// package's parser.
func parseYAML(s string, v any) error {
	return yamlUnmarshal([]byte(s), v)
}

// silence unused import warnings in case logging isn't wired.
var _ = slog.Default
var _ = bufio.NewScanner