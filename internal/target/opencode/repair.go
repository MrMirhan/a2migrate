package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
)

// RepairReport summarises what Repair did.
type RepairReport struct {
	SessionsScanned     int
	Reparents           int
	PadsStepParts       int
	AddedStepStartTimes int
	AddedToolStateTimes int
	Errors              []error
}

// Repair runs all four post-fix invariants against every session whose
// metadata contains claude_code_origin. Each stage is idempotent.
//
// scopeIDs limits the work to the given session ids; pass nil to scan all
// migrated sessions.
func Repair(ctx context.Context, db *sql.DB, scopeIDs []string) (RepairReport, error) {
	logger := slog.Default()
	rep := RepairReport{}

	sessions, err := migratedSessions(ctx, db, scopeIDs)
	if err != nil {
		return rep, err
	}
	rep.SessionsScanned = len(sessions)
	if len(sessions) == 0 {
		return rep, nil
	}

	for _, sid := range sessions {
		if n, err := reparent(ctx, db, sid, logger); err != nil {
			rep.Errors = append(rep.Errors, err)
		} else {
			rep.Reparents += n
		}
		if n, err := padStepParts(ctx, db, sid, logger); err != nil {
			rep.Errors = append(rep.Errors, err)
		} else {
			rep.PadsStepParts += n
		}
		if n, err := addStepStartTime(ctx, db, sid, logger); err != nil {
			rep.Errors = append(rep.Errors, err)
		} else {
			rep.AddedStepStartTimes += n
		}
		if n, err := addToolStateTime(ctx, db, sid, logger); err != nil {
			rep.Errors = append(rep.Errors, err)
		} else {
			rep.AddedToolStateTimes += n
		}
	}
	return rep, nil
}

func migratedSessions(ctx context.Context, db *sql.DB, scope []string) ([]string, error) {
	q := `SELECT id FROM session WHERE metadata LIKE '%claude_code_origin%'`
	args := []any{}
	if len(scope) > 0 {
		q += ` AND id IN (` + placeholders(len(scope)) + `)`
		for _, s := range scope {
			args = append(args, s)
		}
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	out := make([]byte, 0, n*2)
	for i := 0; i < n; i++ {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, '?')
	}
	return string(out)
}

// reparent enforces "assistant.parentID.role == user" by walking each
// migrated session's messages in time order and rewriting parentID when the
// parent is also an assistant.
//
// Returns the number of messages reparented.
func reparent(ctx context.Context, db *sql.DB, sessionID string, logger *slog.Logger) (int, error) {
	msgs, err := loadMessages(ctx, db, sessionID)
	if err != nil {
		return 0, err
	}
	if len(msgs) == 0 {
		return 0, nil
	}
	type row struct {
		ID      string
		Role    string
		Parent  string
		Created int64
	}
	rows := make([]row, len(msgs))
	for i, m := range msgs {
		rows[i] = row{
			ID:      m.id,
			Role:    m.role(),
			Parent:  m.parentID(),
			Created: m.timeCreated,
		}
	}
	parentByID := map[string]string{}
	for _, r := range rows {
		parentByID[r.ID] = r.Parent
	}
	roleByID := map[string]string{}
	for _, r := range rows {
		roleByID[r.ID] = r.Role
	}

	lastUserByMsg := map[string]string{}
	var lastUser string
	for _, r := range rows {
		lastUserByMsg[r.ID] = lastUser
		if r.Role == "user" {
			lastUser = r.ID
		}
	}

	var updated int
	for _, r := range rows {
		if r.Role != "assistant" {
			continue
		}
		if r.Parent == "" {
			continue
		}
		if roleByID[r.Parent] == "user" {
			continue
		}
		newParent := lastUserByMsg[r.ID]
		if err := updateMessageParentID(ctx, db, r.ID, newParent); err != nil {
			return updated, err
		}
		updated++
		logger.Debug("reparented assistant", "session", sessionID, "msg", r.ID, "old_parent", r.Parent, "new_parent", newParent)
	}
	return updated, nil
}

// padStepParts fills in the native fields for step-start and step-finish
// parts that the planner emits bare. Idempotent (uses setdefault-style).
func padStepParts(ctx context.Context, db *sql.DB, sessionID string, logger *slog.Logger) (int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, message_id, time_created, data FROM part
		 WHERE session_id = ? AND json_extract(data, '$.type') IN ('step-start','step-finish')`,
		sessionID)
	if err != nil {
		return 0, err
	}
	type prow struct {
		ID    string
		MsgID string
		Ts    int64
		Data  string
	}
	var parts []prow
	for rows.Next() {
		var p prow
		if err := rows.Scan(&p.ID, &p.MsgID, &p.Ts, &p.Data); err != nil {
			_ = rows.Close()
			return 0, err
		}
		parts = append(parts, p)
	}
	_ = rows.Close()

	msgTs := map[string]int64{}
	for _, p := range parts {
		if _, ok := msgTs[p.MsgID]; !ok {
			msgTs[p.MsgID] = p.Ts
		}
	}
	// Also load actual message timestamps (since part.time_created may be 0).
	mrows, err := db.QueryContext(ctx,
		`SELECT id, time_created FROM message WHERE session_id = ?`, sessionID)
	if err != nil {
		return 0, err
	}
	for mrows.Next() {
		var id string
		var ts int64
		if err := mrows.Scan(&id, &ts); err != nil {
			_ = mrows.Close()
			return 0, err
		}
		if ts != 0 {
			msgTs[id] = ts
		}
	}
	_ = mrows.Close()

	var updated int
	for _, p := range parts {
		var raw map[string]any
		if err := json.Unmarshal([]byte(p.Data), &raw); err != nil {
			return updated, err
		}
		typ, _ := raw["type"].(string)
		ts := msgTs[p.MsgID]
		var changed bool
		switch typ {
		case "step-start":
			if _, ok := raw["snapshot"]; !ok {
				raw["snapshot"] = ""
				changed = true
			}
		case "step-finish":
			for k, v := range map[string]any{
				"reason":   "stop",
				"snapshot": "",
				"tokens": map[string]any{
					"total": 0, "input": 0, "output": 0, "reasoning": 0,
					"cache": map[string]any{"write": 0, "read": 0},
				},
				"cost":     0,
				"metadata": map[string]any{},
				"hash":     map[string]any{},
				"files":    map[string]any{},
				"time":     map[string]any{"created": ts, "completed": ts},
			} {
				if _, ok := raw[k]; !ok {
					raw[k] = v
					changed = true
				}
			}
		}
		if !changed {
			continue
		}
		out, err := json.Marshal(raw)
		if err != nil {
			return updated, err
		}
		if _, err := db.ExecContext(ctx, `UPDATE part SET data = ? WHERE id = ?`, string(out), p.ID); err != nil {
			return updated, err
		}
		updated++
		logger.Debug("padded step part", "session", sessionID, "part", p.ID, "type", typ)
	}
	return updated, nil
}

// addStepStartTime sets data.time for any step-start that lacks it. Uses the
// message timestamp as a fallback when the part timestamp is zero.
func addStepStartTime(ctx context.Context, db *sql.DB, sessionID string, logger *slog.Logger) (int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT p.id, p.message_id, p.time_created, m.time_created, p.data
		 FROM part p JOIN message m ON m.id = p.message_id
		 WHERE p.session_id = ?
		   AND json_extract(p.data, '$.type') = 'step-start'
		   AND json_extract(p.data, '$.time') IS NULL`,
		sessionID)
	if err != nil {
		return 0, err
	}
	type prow struct {
		ID, MsgID     string
		PartTS, MsgTS int64
		Data          string
	}
	var parts []prow
	for rows.Next() {
		var p prow
		if err := rows.Scan(&p.ID, &p.MsgID, &p.PartTS, &p.MsgTS, &p.Data); err != nil {
			_ = rows.Close()
			return 0, err
		}
		parts = append(parts, p)
	}
	_ = rows.Close()

	var updated int
	for _, p := range parts {
		var raw map[string]any
		if err := json.Unmarshal([]byte(p.Data), &raw); err != nil {
			return updated, err
		}
		ts := p.PartTS
		if ts == 0 {
			ts = p.MsgTS
		}
		raw["time"] = map[string]any{"created": ts, "completed": ts}
		out, err := json.Marshal(raw)
		if err != nil {
			return updated, err
		}
		if _, err := db.ExecContext(ctx, `UPDATE part SET data = ? WHERE id = ?`, string(out), p.ID); err != nil {
			return updated, err
		}
		updated++
		logger.Debug("added step-start time", "session", sessionID, "part", p.ID)
	}
	return updated, nil
}

// addToolStateTime writes state.time = {compacted: false} on every tool part
// that lacks state.time.
func addToolStateTime(ctx context.Context, db *sql.DB, sessionID string, logger *slog.Logger) (int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, data FROM part
		 WHERE session_id = ?
		   AND json_extract(data, '$.type') = 'tool'
		   AND json_extract(data, '$.state.time') IS NULL`,
		sessionID)
	if err != nil {
		return 0, err
	}
	type prow struct {
		ID   string
		Data string
	}
	var parts []prow
	for rows.Next() {
		var p prow
		if err := rows.Scan(&p.ID, &p.Data); err != nil {
			_ = rows.Close()
			return 0, err
		}
		parts = append(parts, p)
	}
	_ = rows.Close()

	var updated int
	for _, p := range parts {
		var raw map[string]any
		if err := json.Unmarshal([]byte(p.Data), &raw); err != nil {
			return updated, err
		}
		state, ok := raw["state"].(map[string]any)
		if !ok {
			state = map[string]any{}
		}
		state["time"] = map[string]any{"compacted": false}
		raw["state"] = state
		out, err := json.Marshal(raw)
		if err != nil {
			return updated, err
		}
		if _, err := db.ExecContext(ctx, `UPDATE part SET data = ? WHERE id = ?`, string(out), p.ID); err != nil {
			return updated, err
		}
		updated++
		logger.Debug("added tool state.time", "session", sessionID, "part", p.ID)
	}
	return updated, nil
}

type messageView struct {
	id          string
	timeCreated int64
	data        map[string]any
}

func (m messageView) role() string {
	if r, ok := m.data["role"].(string); ok {
		return r
	}
	return ""
}

func (m messageView) parentID() string {
	if r, ok := m.data["parentID"].(string); ok {
		return r
	}
	return ""
}

func loadMessages(ctx context.Context, db *sql.DB, sessionID string) ([]messageView, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, time_created, data FROM message WHERE session_id = ? ORDER BY time_created, id`,
		sessionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []messageView
	for rows.Next() {
		var (
			id   string
			ts   int64
			blob string
		)
		if err := rows.Scan(&id, &ts, &blob); err != nil {
			return nil, err
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(blob), &data); err != nil {
			return nil, err
		}
		out = append(out, messageView{id: id, timeCreated: ts, data: data})
	}
	return out, rows.Err()
}

func updateMessageParentID(ctx context.Context, db *sql.DB, msgID string, newParent string) error {
	var raw map[string]any
	var blob string
	if err := db.QueryRowContext(ctx, `SELECT data FROM message WHERE id = ?`, msgID).Scan(&blob); err != nil {
		return fmt.Errorf("load %s: %w", msgID, err)
	}
	if err := json.Unmarshal([]byte(blob), &raw); err != nil {
		return err
	}
	if newParent == "" {
		delete(raw, "parentID")
	} else {
		raw["parentID"] = newParent
	}
	out, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `UPDATE message SET data = ? WHERE id = ?`, string(out), msgID)
	return err
}
