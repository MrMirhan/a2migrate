package sync

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/platform"
	"github.com/mirhan/a2migrate/internal/source/claudecode"
	ocsrc "github.com/mirhan/a2migrate/internal/source/opencode"
	"github.com/mirhan/a2migrate/internal/target/opencode"
)

// Sessions syncs every CC session that has an OC mirror into OC,
// appending only the messages that don't already exist on the OC side
// (matched by message.data.parentID or by OriginID when available).
//
// Idempotent: re-running sync with no CC changes produces zero writes.
func Sessions(ctx context.Context, dbPath string, dryRun bool) (*Report, error) {
	cc := claudecode.NewSessionReader("")
	r := &Report{CCHome: cc.CCHome, OCHome: dbPath}

	db, err := opencode.OpenDatabase(ctx, dbPath)
	if err != nil {
		return r, err
	}
	defer db.Close()

	refs, err := ocsrc.NewSessionReader(dbPath).DiscoverSessions(ctx, db)
	if err != nil {
		return r, err
	}

	for _, ref := range refs {
		if ref.OriginID == "" {
			continue
		}
		if err := syncOneSession(ctx, cc, db, ref, dryRun, r); err != nil {
			r.Errors = append(r.Errors, err)
		}
	}
	return r, nil
}

func syncOneSession(ctx context.Context, cc *claudecode.SessionReader, db *sql.DB, ref ocsrc.SessionRef, dryRun bool, r *Report) error {
	ccPath, err := findCCPathForOrigin(ref.OriginID, ref.IsSubagent, ref.ParentID)
	if err != nil {
		return err
	}
	ccInfo, err := os.Stat(ccPath)
	if err != nil {
		return fmt.Errorf("stat cc %s: %w", ccPath, err)
	}

	var ocMsec sql.NullInt64
	if err := db.QueryRowContext(ctx,
		`SELECT time_updated FROM session WHERE id = ?`, ref.OCSessionID,
	).Scan(&ocMsec); err != nil {
		return fmt.Errorf("query oc session: %w", err)
	}
	if ocMsec.Valid && !ccInfo.ModTime().After(time.UnixMilli(ocMsec.Int64)) {
		r.Skipped++
		return nil
	}

	ccSess, err := cc.ParseSession(ccPath)
	if err != nil {
		return fmt.Errorf("reparse cc: %w", err)
	}

	existing := map[string]bool{}
	rows, err := db.QueryContext(ctx,
		`SELECT json_extract(data, '$.parentID') FROM message WHERE session_id = ?`, ref.OCSessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p sql.NullString
		if err := rows.Scan(&p); err != nil {
			return err
		}
		if p.Valid && p.String != "" {
			existing[p.String] = true
		}
	}

	var newMsgs []domain.Message
	for _, m := range ccSess.Messages {
		if m.OriginID != "" && existing[m.OriginID] {
			continue
		}
		newMsgs = append(newMsgs, m)
	}
	if len(newMsgs) == 0 {
		r.Skipped++
		return nil
	}
	if dryRun {
		for range newMsgs {
			r.Applied = append(r.Applied, Result{Op: OpCopyCCtoOC, Path: ccPath})
		}
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var lastMsgID string
	for _, m := range newMsgs {
		msgID, err := genID(tx, "msg")
		if err != nil {
			return err
		}
		data, err := buildInsertData(&m, lastMsgID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO message(id, session_id, time_created, time_updated, data)
			 VALUES(?, ?, ?, ?, ?)`,
			msgID, ref.OCSessionID, msOrZero(m.CreatedAt), msOrZero(m.CreatedAt), data); err != nil {
			return fmt.Errorf("insert message: %w", err)
		}

		if m.Role == domain.RoleAssistant {
			if err := insertPart(tx, msgID, ref.OCSessionID, `{"type":"step-start"}`); err != nil {
				return fmt.Errorf("insert step-start: %w", err)
			}
		}
		for _, p := range m.Parts {
			pd, err := buildPartData(&p)
			if err != nil {
				return err
			}
			if err := insertPart(tx, msgID, ref.OCSessionID, pd); err != nil {
				return fmt.Errorf("insert part: %w", err)
			}
		}
		if m.Role == domain.RoleAssistant {
			if err := insertPart(tx, msgID, ref.OCSessionID, `{"type":"step-finish"}`); err != nil {
				return fmt.Errorf("insert step-finish: %w", err)
			}
		}
		lastMsgID = msgID
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE session SET time_updated = ? WHERE id = ?`,
		msOrZero(ccSess.UpdatedAt), ref.OCSessionID); err != nil {
		return fmt.Errorf("update session time_updated: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	for range newMsgs {
		r.Applied = append(r.Applied, Result{Op: OpCopyCCtoOC, Path: ccPath})
	}
	return nil
}

// findCCPathForOrigin resolves the JSONL file path for a given CC origin
// id, distinguishing main sessions from subagents.
func findCCPathForOrigin(originID string, isSubagent bool, parentOrigin string) (string, error) {
	projectsRoot := filepath.Join(platform.ClaudeCodeHome(), "projects")
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return "", err
	}
	for _, p := range entries {
		if !p.IsDir() {
			continue
		}
		encoded := p.Name()
		if isSubagent {
			base := filepath.Join(projectsRoot, encoded, parentOrigin, "subagents")
			candidate := filepath.Join(base, "agent-"+originID+".jsonl")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		} else {
			candidate := filepath.Join(projectsRoot, encoded, originID+".jsonl")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("no JSONL found for origin %s", originID)
}

func genID(tx *sql.Tx, prefix string) (string, error) {
	var id string
	err := tx.QueryRow(`SELECT lower(hex(randomblob(16)))`).Scan(&id)
	if err != nil {
		return "", err
	}
	return prefix + "_" + id, nil
}

func insertPart(tx *sql.Tx, msgID, sessionID, data string) error {
	partID, err := genID(tx, "prt")
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO part(id, message_id, session_id, data) VALUES(?, ?, ?, ?)`,
		partID, msgID, sessionID, data)
	return err
}

func buildInsertData(m *domain.Message, parentID string) (string, error) {
	var raw map[string]any
	switch m.Role {
	case domain.RoleUser:
		raw = map[string]any{
			"role": "user",
			"time": map[string]any{"created": msOrZero(m.CreatedAt)},
			"agent": "build",
			"model": map[string]any{
				"providerID": m.ProviderID,
				"modelID":    m.ModelID,
				"variant":    "thinking",
			},
			"summary": map[string]any{"diffs": []any{}},
		}
	case domain.RoleAssistant:
		raw = map[string]any{
			"role":    "assistant",
			"mode":    "build",
			"agent":   m.Agent,
			"variant": m.Variant,
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
	default:
		return "", fmt.Errorf("unsupported role %s", m.Role)
	}
	if parentID != "" {
		raw["parentID"] = parentID
	}
	b, err := json.Marshal(raw)
	return string(b), err
}

func buildPartData(p *domain.Part) (string, error) {
	switch p.Type {
	case domain.PartText:
		return jsonString(map[string]any{"type": "text", "text": p.Text})
	case domain.PartReasoning:
		return jsonString(map[string]any{"type": "reasoning", "text": p.Text})
	case domain.PartTool:
		return jsonString(map[string]any{
			"type":   "tool",
			"tool":   p.ToolName,
			"callID": p.ToolCallID,
			"state": map[string]any{
				"status": p.ToolStatus,
				"input":  p.ToolInput,
				"output": p.ToolOutput,
				"title":  p.ToolName,
			},
		})
	}
	return "", fmt.Errorf("unsupported part type %s", p.Type)
}

func jsonString(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func msOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// SessionsReverse syncs OC sessions that originated from CC back to
// their CC JSONL files. Append-only: messages in OC whose uuid doesn't
// already appear in the CC JSONL get appended.
func SessionsReverse(ctx context.Context, dbPath, ccHome string, dryRun bool) (*Report, error) {
	r := &Report{CCHome: ccHome, OCHome: dbPath}

	db, err := opencode.OpenDatabase(ctx, dbPath)
	if err != nil {
		return r, err
	}
	defer db.Close()

	refs, err := ocsrc.NewSessionReader(dbPath).DiscoverSessions(ctx, db)
	if err != nil {
		return r, err
	}

	cc := claudecode.NewSessionReader(ccHome)

	for _, ref := range refs {
		if ref.OriginID == "" {
			continue
		}
		if err := reverseSyncOne(ctx, dbPath, cc, db, ref, dryRun, r); err != nil {
			r.Errors = append(r.Errors, err)
		}
	}
	return r, nil
}

func reverseSyncOne(ctx context.Context, dbPath string, cc *claudecode.SessionReader, db *sql.DB, ref ocsrc.SessionRef, dryRun bool, r *Report) error {
	ccPath, err := findCCPathForOrigin(ref.OriginID, ref.IsSubagent, ref.ParentID)
	if err != nil {
		return err
	}

	ocMsec, err := ocMaxMessageTime(ctx, db, ref.OCSessionID)
	if err != nil {
		return err
	}
	ccMsec, err := ccJSONLMTimeMs(ccPath)
	if err != nil {
		return err
	}
	if !time.UnixMilli(ocMsec).After(time.UnixMilli(ccMsec)) {
		r.Skipped++
		return nil
	}

	existing := map[string]bool{}
	if _, err := os.Stat(ccPath); err == nil {
		f, err := os.Open(ccPath)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			var raw map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil {
				continue
			}
			if uuid, ok := raw["uuid"].(string); ok {
				existing[uuid] = true
			}
		}
		f.Close()
	}

	sess, err := ocsrc.NewSessionReader(dbPath).ParseSession(ctx, db, ref)
	if err != nil {
		return err
	}

	var newMsgs []domain.Message
	for _, m := range sess.Messages {
		if m.OriginID != "" && existing[m.OriginID] {
			continue
		}
		newMsgs = append(newMsgs, m)
	}
	if len(newMsgs) == 0 {
		r.Skipped++
		return nil
	}
	if dryRun {
		for range newMsgs {
			r.Applied = append(r.Applied, Result{Op: OpCopyOCtoCC, Path: ccPath})
		}
		return nil
	}
	if err := appendJSONLToCC(ccPath, newMsgs); err != nil {
		return err
	}
	for range newMsgs {
		r.Applied = append(r.Applied, Result{Op: OpCopyOCtoCC, Path: ccPath})
	}
	return nil
}

func ocMaxMessageTime(ctx context.Context, db *sql.DB, ocSessionID string) (int64, error) {
	var t sql.NullInt64
	err := db.QueryRowContext(ctx,
		`SELECT MAX(time_created) FROM message WHERE session_id = ?`, ocSessionID,
	).Scan(&t)
	if err != nil {
		return 0, err
	}
	if !t.Valid {
		return 0, nil
	}
	return t.Int64, nil
}

func ccJSONLMTimeMs(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.ModTime().UnixMilli(), nil
}

func appendJSONLToCC(path string, msgs []domain.Message) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	for _, m := range msgs {
		row := ccJSONLRow(m)
		if _, err := w.Write(append(row, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func ccJSONLRow(m domain.Message) []byte {
	row := map[string]any{
		"type":      string(m.Role),
		"uuid":      m.OriginID,
		"sessionId": "",
		"timestamp": m.CreatedAt.UTC().Format(time.RFC3339Nano),
		"message":   map[string]any{"role": string(m.Role), "content": ocPartsToCC(m.Parts)},
	}
	if m.Role == domain.RoleAssistant && !m.Tokens.IsZero() {
		row["usage"] = map[string]any{
			"input_tokens":                m.Tokens.Input,
			"output_tokens":               m.Tokens.Output,
			"cache_creation_input_tokens": m.Tokens.CacheWrite,
			"cache_read_input_tokens":     m.Tokens.CacheRead,
		}
	}
	if m.CostUSD > 0 {
		row["cost_usd"] = m.CostUSD
	}
	b, _ := json.Marshal(row)
	return b
}

func ocPartsToCC(parts []domain.Part) any {
	var blocks []map[string]any
	for _, p := range parts {
		switch p.Type {
		case domain.PartText:
			blocks = append(blocks, map[string]any{"type": "text", "text": p.Text})
		case domain.PartReasoning:
			blocks = append(blocks, map[string]any{"type": "thinking", "thinking": p.Text})
		case domain.PartTool:
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    p.ToolCallID,
				"name":  p.ToolName,
				"input": p.ToolInput,
			})
			if p.ToolOutput != "" {
				blocks = append(blocks, map[string]any{
					"type":        "tool_result",
					"tool_use_id": p.ToolCallID,
					"content":     p.ToolOutput,
				})
			}
		}
	}
	if len(blocks) == 0 {
		return ""
	}
	return blocks
}

// io is unused directly but kept for clarity.
var _ = io.Discard