package claudecode

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/platform"
)

// EntryKind discriminates the type of a Claude Code JSONL record.
type EntryKind string

const (
	KindUser       EntryKind = "user"
	KindAssistant  EntryKind = "assistant"
	KindAITitle    EntryKind = "ai-title"
	KindLastPrompt EntryKind = "last-prompt"
	KindOther      EntryKind = "other"
)

var migrateKinds = map[EntryKind]bool{
	KindUser:      true,
	KindAssistant: true,
	KindAITitle:   true,
}

// entry is the untyped view of one JSONL record.
type entry struct {
	Kind        EntryKind
	UUID        string
	ParentUUID  string
	SessionID   string
	Timestamp   time.Time
	CWD         string
	Message     messagePayload
	IsSidechain bool
	AgentID     string
	Title       string // for ai-title
	LeafUUID    string // for last-prompt
}

// messagePayload is a partial mirror of CC's `message` field. We only model
// the fields we care about; the rest stays in raw.
type messagePayload struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// toolResultBlock is one {type:"tool_result"} block in a user message.
type toolResultBlock struct {
	Type      string          `json:"type"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// ParseSession opens path, streams it line-by-line, and produces a domain.Session.
// Malformed lines are skipped with a debug log; the function returns nil error
// unless the file cannot be opened at all.
func (r *SessionReader) ParseSession(path string) (domain.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return domain.Session{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	base := filepath.Base(path)
	origin := base
	if i := strings.LastIndex(base, "."); i > 0 {
		origin = base[:i]
	}
	ref := SessionRef{
		FilePath: path,
		OriginID: origin,
		Worktree: lookupWorktreeFromPath(path),
	}
	// Detect subagent files by path layout: <projects>/<enc>/<sid>/subagents/<file>
	if strings.Contains(filepath.ToSlash(path), "/subagents/") {
		ref.IsSubagent = true
		dir := filepath.Dir(filepath.ToSlash(path))
		// dir is .../<sid>/subagents — parent basename is the sid
		parts := strings.Split(dir, "/")
		if len(parts) >= 1 {
			ref.ParentID = parts[len(parts)-2]
		}
	}
	return parseSessionStream(f, ref, slog.Default())
}

// lookupWorktreeFromPath derives the worktree from the parent project dir
// segment of a JSONL file path. Path layout:
//   ~/.claude/projects/<encoded>/<file>.jsonl
//   ~/.claude/projects/<encoded>/<sid>/subagents/agent-<id>.jsonl
func lookupWorktreeFromPath(path string) string {
	slash := filepath.ToSlash(path)
	parts := strings.Split(slash, "/projects/")
	if len(parts) < 2 {
		return ""
	}
	rest := parts[1]
	segments := strings.Split(rest, "/")
	if len(segments) == 0 {
		return ""
	}
	return platform.DecodeCWD(segments[0])
}

func parseSessionStream(r io.Reader, ref SessionRef, logger *slog.Logger) (domain.Session, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // up to 16MB/line

	var entries []entry
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			logger.Debug("skip malformed jsonl line", "file", ref.FilePath, "line", lineNo, "err", err)
			continue
		}
		e, ok := parseEntry(raw)
		if !ok {
			continue
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return domain.Session{}, fmt.Errorf("scan: %w", err)
	}

	return entriesToSession(entries, ref)
}

func parseEntry(raw map[string]json.RawMessage) (entry, bool) {
	var kindStr string
	if err := json.Unmarshal(raw["type"], &kindStr); err != nil {
		return entry{}, false
	}
	e := entry{Kind: EntryKind(kindStr)}
	_ = json.Unmarshal(raw["uuid"], &e.UUID)
	_ = json.Unmarshal(raw["parentUuid"], &e.ParentUUID)
	_ = json.Unmarshal(raw["sessionId"], &e.SessionID)
	_ = json.Unmarshal(raw["isSidechain"], &e.IsSidechain)
	_ = json.Unmarshal(raw["agentId"], &e.AgentID)
	_ = json.Unmarshal(raw["leafUuid"], &e.LeafUUID)

	if ts, ok := raw["timestamp"]; ok {
		var s string
		if err := json.Unmarshal(ts, &s); err == nil {
			e.Timestamp = parseTimestamp(s)
		}
	}
	if cwd, ok := raw["cwd"]; ok {
		_ = json.Unmarshal(cwd, &e.CWD)
	}
	if msg, ok := raw["message"]; ok {
		_ = json.Unmarshal(msg, &e.Message)
	}
	if title, ok := raw["title"]; ok {
		_ = json.Unmarshal(title, &e.Title)
	}
	return e, true
}

// parseTimestamp tolerates both RFC3339 and RFC3339Nano forms.
func parseTimestamp(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// entriesToSession flattens the raw entries into a domain.Session, producing
// the message/part stream the target writer expects.
func entriesToSession(entries []entry, ref SessionRef) (domain.Session, error) {
	if len(entries) == 0 {
		return domain.Session{}, errors.New("empty session")
	}

	sess := domain.Session{
		OriginID:   ref.OriginID,
		Origin:     domain.OriginClaudeCode,
		ProjectDir: ref.Worktree,
		IsSubagent: ref.IsSubagent,
		ParentID:   ref.ParentID,
	}

	// First non-zero timestamp = session start.
	var firstTS, lastTS time.Time
	for _, e := range entries {
		if e.Timestamp.IsZero() {
			continue
		}
		if firstTS.IsZero() || e.Timestamp.Before(firstTS) {
			firstTS = e.Timestamp
		}
		if e.Timestamp.After(lastTS) {
			lastTS = e.Timestamp
		}
	}
	sess.CreatedAt = firstTS
	sess.UpdatedAt = lastTS
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Unix(0, 0).UTC()
	}
	if sess.UpdatedAt.IsZero() {
		sess.UpdatedAt = sess.CreatedAt
	}
	if sess.OriginID == "" {
		sess.OriginID = ref.OriginID
	}

	// Build tool_result map across the whole session, so assistant tool_use
	// can pair with the user message that delivers its result.
	toolResults := collectToolResults(entries)

	// Derive title.
	sess.Title = deriveTitle(entries)

	// Build message stream.
	var messages []domain.Message
	var current *domain.Message
	flush := func() {
		if current != nil {
			messages = append(messages, *current)
			current = nil
		}
	}

	for i := range entries {
		e := entries[i]
		if !migrateKinds[e.Kind] {
			continue
		}
		switch e.Kind {
		case KindUser:
			if isPureToolResultUser(e) {
				continue
			}
			flush()
			msg := domain.Message{
				OriginID:  e.UUID,
				SessionID: ref.OriginID,
				Role:      domain.RoleUser,
				CreatedAt: e.Timestamp,
			}
			text := extractTextFromUserContent(e.Message.Content, toolResults)
			if text == "" {
				continue
			}
			msg.Parts = []domain.Part{{
				Type: domain.PartText,
				Text: text,
			}}
			current = &msg
		case KindAssistant:
			flush()
			parts := collectAssistantParts(e.Message.Content, toolResults)
			if len(parts) == 0 {
				continue
			}
			msg := domain.Message{
				OriginID:  e.UUID,
				SessionID: ref.OriginID,
				Role:      domain.RoleAssistant,
				CreatedAt: e.Timestamp,
				Agent:     "build",
				Variant:   "thinking",
				Parts:     parts,
			}
			current = &msg
		case KindAITitle:
			// Already used to derive title; skip.
		}
	}
	flush()
	sess.Messages = messages
	return sess, nil
}

// collectToolResults walks every user entry and indexes tool_result blocks
// by tool_use_id. The map is consumed when assembling assistant tool_use
// parts to attach their outputs.
func collectToolResults(entries []entry) map[string]toolResultBlock {
	out := map[string]toolResultBlock{}
	for _, e := range entries {
		if e.Kind != KindUser {
			continue
		}
		var blocks []toolResultBlock
		if err := json.Unmarshal(e.Message.Content, &blocks); err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_result" && b.ToolUseID != "" {
				out[b.ToolUseID] = b
			}
		}
	}
	return out
}

// isPureToolResultUser returns true if a user entry's content is only
// tool_result blocks (no real user text). These don't form their own
// OpenCode message — they only annotate the prior assistant turn.
func isPureToolResultUser(e entry) bool {
	if e.Message.Content == nil {
		return false
	}
	var blocks []toolResultBlock
	if err := json.Unmarshal(e.Message.Content, &blocks); err != nil {
		return false
	}
	if len(blocks) == 0 {
		return false
	}
	for _, b := range blocks {
		if b.Type != "tool_result" {
			return false
		}
	}
	return true
}

// extractTextFromUserContent renders a user entry's content to plain text.
// String content passes through. List content joins text blocks; tool_result
// blocks are rendered as `[tool_result for <id>]` markers so downstream
// pipelines can pair them with their originating tool_use.
func extractTextFromUserContent(raw json.RawMessage, toolResults map[string]toolResultBlock) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, b := range blocks {
		typ, _ := b["type"].MarshalJSON()
		switch string(typ) {
		case `"text"`:
			var t string
			_ = json.Unmarshal(b["text"], &t)
			sb.WriteString(t)
			sb.WriteString("\n")
		case `"tool_result"`:
			id, _ := b["tool_use_id"].MarshalJSON()
			isErr := false
			if v, ok := b["is_error"]; ok {
				_ = json.Unmarshal(v, &isErr)
			}
			fmt.Fprintf(&sb, "[tool_result for %s] %s\n\n", strings.Trim(string(id), `"`), renderToolResultBody(b["content"], isErr))
		}
	}
	return strings.TrimSpace(sb.String())
}

// renderToolResultBody turns the body of a tool_result block into a one-line
// preview. Long output is truncated with an ellipsis.
func renderToolResultBody(raw json.RawMessage, isErr bool) string {
	if len(raw) == 0 {
		if isErr {
			return "[ERROR]"
		}
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return truncate(s, 2000)
	}
	// List form — join text elements.
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if t, ok := b["type"]; ok && string(t) == `"text"` {
				var v string
				_ = json.Unmarshal(b["text"], &v)
				sb.WriteString(v)
			}
		}
		return truncate(sb.String(), 2000)
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// collectAssistantParts translates CC assistant blocks into OC parts:
// text → text, thinking → reasoning, tool_use → tool (paired with result).
func collectAssistantParts(raw json.RawMessage, toolResults map[string]toolResultBlock) []domain.Part {
	if len(raw) == 0 {
		return nil
	}
	var blocks []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	var parts []domain.Part
	for _, b := range blocks {
		typ, _ := b["type"].MarshalJSON()
		switch string(typ) {
		case `"text"`:
			var t string
			_ = json.Unmarshal(b["text"], &t)
			if t == "" {
				continue
			}
			parts = append(parts, domain.Part{Type: domain.PartText, Text: t})
		case `"thinking"`:
			var t string
			_ = json.Unmarshal(b["thinking"], &t)
			if t == "" {
				continue
			}
			// signature is dropped on purpose (OC has no equivalent).
			parts = append(parts, domain.Part{Type: domain.PartReasoning, Text: t})
		case `"tool_use"`:
			var (
				id, name string
				input    map[string]any
			)
			_ = json.Unmarshal(b["id"], &id)
			_ = json.Unmarshal(b["name"], &name)
			_ = json.Unmarshal(b["input"], &input)
			status := "completed"
			output := ""
			if tr, ok := toolResults[id]; ok {
				if tr.IsError {
					status = "error"
				}
				output = renderToolResultBody(tr.Content, tr.IsError)
			}
			parts = append(parts, domain.Part{
				Type:       domain.PartTool,
				ToolName:   name,
				ToolCallID: id,
				ToolInput:  input,
				ToolOutput: output,
				ToolStatus: status,
			})
		}
	}
	return parts
}

// deriveTitle picks a title in priority order: explicit ai-title, first user
// text truncated to 80 chars, or a placeholder.
func deriveTitle(entries []entry) string {
	for _, e := range entries {
		if e.Kind == KindAITitle && e.Title != "" {
			return e.Title
		}
	}
	for _, e := range entries {
		if e.Kind != KindUser {
			continue
		}
		if isPureToolResultUser(e) {
			continue
		}
		text := extractTextFromUserContent(e.Message.Content, nil)
		if text == "" {
			continue
		}
		text = strings.Join(strings.Fields(text), " ")
		if len(text) > 80 {
			text = text[:80] + "…"
		}
		return text
	}
	return "<claude code session>"
}

// Summarize is a fast header-only view used by `sessions list`. It reads
// at most maxRecords entries or until maxBytes is hit, whichever first.
func (r *SessionReader) Summarize(path string, maxRecords, maxBytes int) (Summary, error) {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}, err
	}
	defer f.Close()

	ref, _ := os.Stat(path)
	out := Summary{
		FilePath:  path,
		SizeBytes: ref.Size(),
		UpdatedAt: ref.ModTime().Unix(),
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	count := 0
	bytes := 0
	for scanner.Scan() {
		count++
		bytes += len(scanner.Bytes())
		line := scanner.Bytes()
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		var kind string
		_ = json.Unmarshal(raw["type"], &kind)
		out.MessageCount++
		if kind == "assistant" {
			out.AssistantCount++
		}
		if kind == "user" {
			out.UserCount++
		}
		if count >= maxRecords || bytes >= maxBytes {
			out.Truncated = true
			break
		}
	}
	return out, scanner.Err()
}

// Summary is the lightweight view produced by Summarize.
type Summary struct {
	FilePath       string
	SizeBytes      int64
	UpdatedAt      int64
	MessageCount   int
	UserCount      int
	AssistantCount int
	Truncated      bool
	OriginID       string
	IsSubagent     bool
}

// SortSummaries orders summaries so main sessions precede subagents within
// the same directory, and groups by project directory.
func SortSummaries(s []Summary) {
	sort.Slice(s, func(i, j int) bool {
		ip := projectFromPath(s[i].FilePath)
		jp := projectFromPath(s[j].FilePath)
		if ip != jp {
			return ip < jp
		}
		if s[i].IsSubagent != s[j].IsSubagent {
			return !s[i].IsSubagent
		}
		return s[i].FilePath < s[j].FilePath
	})
}

func projectFromPath(p string) string {
	// ~/.claude/projects/<encoded>/<file>
	parts := strings.Split(p, string(os.PathSeparator))
	if len(parts) >= 3 {
		return parts[len(parts)-3]
	}
	return p
}