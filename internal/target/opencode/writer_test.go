package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mirhan/a2migrate/internal/domain"
)

func sampleSession() domain.Session {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	return domain.Session{
		OriginID:   "cc-sess-1",
		Origin:     domain.OriginClaudeCode,
		Title:      "Test Session",
		Slug:       "",
		ProjectDir: "/tmp/proj",
		CreatedAt:  now,
		UpdatedAt:  now.Add(2 * time.Second),
		Messages: []domain.Message{
			{
				OriginID:  "u1",
				Role:      domain.RoleUser,
				CreatedAt: now,
				Parts:     []domain.Part{{Type: domain.PartText, Text: "hello"}},
			},
			{
				OriginID:  "a1",
				Role:      domain.RoleAssistant,
				CreatedAt: now.Add(1 * time.Second),
				Agent:     "build",
				Variant:   "thinking",
				Parts: []domain.Part{
					{Type: domain.PartText, Text: "hi"},
					{Type: domain.PartTool, ToolName: "Read", ToolCallID: "toolu_1", ToolInput: map[string]any{"path": "/x"}, ToolOutput: "content", ToolStatus: "completed"},
				},
			},
			{
				OriginID:  "u2",
				Role:      domain.RoleUser,
				CreatedAt: now.Add(2 * time.Second),
				Parts:     []domain.Part{{Type: domain.PartText, Text: "thanks"}},
			},
		},
	}
}

func TestWriter_Plan_OneSession(t *testing.T) {
	db := newTestDB(t)
	w := NewSessionWriter(db)
	ctx := context.Background()
	sess := sampleSession()

	plan, err := w.PlanSessions(ctx, []domain.Session{sess})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.NewProjects) != 1 {
		t.Fatalf("projects = %d want 1", len(plan.NewProjects))
	}
	if plan.NewProjects[0].ID != ProjectIDForWorktree("/tmp/proj") {
		t.Fatalf("project id = %s want %s", plan.NewProjects[0].ID, ProjectIDForWorktree("/tmp/proj"))
	}
	if len(plan.Sessions) != 1 {
		t.Fatalf("sessions = %d want 1", len(plan.Sessions))
	}
	if len(plan.Messages) != 3 {
		t.Fatalf("messages = %d want 3", len(plan.Messages))
	}
	// Each user has 1 part, the assistant has 2 parts + step-start + step-finish = 4.
	if len(plan.Parts) != 1+4+1 {
		t.Fatalf("parts = %d want %d", len(plan.Parts), 1+4+1)
	}
}

func TestWriter_Apply_Roundtrip(t *testing.T) {
	db := newTestDB(t)
	w := NewSessionWriter(db)
	ctx := context.Background()
	sess := sampleSession()

	plan, err := w.PlanSessions(ctx, []domain.Session{sess})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	// Verify rows landed.
	var sessions, messages, parts int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM session").Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Fatalf("sessions = %d want 1", sessions)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM message").Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if messages != 3 {
		t.Fatalf("messages = %d want 3", messages)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM part").Scan(&parts); err != nil {
		t.Fatal(err)
	}
	if parts != 6 {
		t.Fatalf("parts = %d want 6", parts)
	}
}

func TestWriter_Apply_SubagentChain(t *testing.T) {
	db := newTestDB(t)
	w := NewSessionWriter(db)
	ctx := context.Background()

	main := sampleSession()
	sub := sampleSession()
	sub.OriginID = "cc-sub-1"
	sub.IsSubagent = true
	sub.ParentID = main.OriginID

	plan, err := w.PlanSessions(ctx, []domain.Session{main, sub})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	// Sub session's parent_id should match the main session's id.
	var parentID *string
	if err := db.QueryRowContext(ctx,
		`SELECT parent_id FROM session WHERE json_extract(metadata, '$.claude_code_origin') = 'cc-sub-1'`,
	).Scan(&parentID); err != nil {
		t.Fatal(err)
	}
	if parentID == nil || *parentID == "" {
		t.Fatal("subagent parent_id was not set")
	}
	// The other way: the main session should have parent_id NULL.
	var mainParent *string
	if err := db.QueryRowContext(ctx,
		`SELECT parent_id FROM session WHERE json_extract(metadata, '$.claude_code_origin') = 'cc-sess-1'`,
	).Scan(&mainParent); err != nil {
		t.Fatal(err)
	}
	if mainParent != nil {
		t.Fatalf("main session should have NULL parent, got %v", *mainParent)
	}
}

func TestWriter_Apply_RollsBackOnError(t *testing.T) {
	db := newTestDB(t)
	w := NewSessionWriter(db)
	ctx := context.Background()
	sess := sampleSession()

	plan, err := w.PlanSessions(ctx, []domain.Session{sess})
	if err != nil {
		t.Fatal(err)
	}
	// Force a duplicate part id so the part INSERT fails mid-tx.
	if len(plan.Parts) >= 2 {
		plan.Parts[1].ID = plan.Parts[0].ID
	}

	if err := w.Apply(ctx, plan); err == nil {
		t.Fatal("expected error from duplicate part id")
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM session").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected rollback (0 session rows), got %d", n)
	}
}

func TestWriter_BuildMessageData(t *testing.T) {
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)
	user := &domain.Message{OriginID: "u", Role: domain.RoleUser, CreatedAt: now}
	data, err := buildMessageData(user, "")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["role"] != "user" {
		t.Fatalf("role = %v", raw["role"])
	}
	if _, ok := raw["parentID"]; ok {
		t.Fatal("first user message should not have parentID")
	}
	if raw["agent"] != "build" {
		t.Fatalf("agent = %v want build", raw["agent"])
	}
	assistant := &domain.Message{OriginID: "a", Role: domain.RoleAssistant, CreatedAt: now, Agent: "build", Variant: "thinking"}
	data, err = buildMessageData(assistant, "parent-msg-id")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["parentID"] != "parent-msg-id" {
		t.Fatalf("parentID = %v", raw["parentID"])
	}
	if raw["tokens"].(map[string]any)["cache"].(map[string]any)["read"].(float64) != 0 {
		t.Fatal("tokens.cache.read should be 0")
	}
}

func TestWriter_BuildPartData(t *testing.T) {
	p := &domain.Part{Type: domain.PartTool, ToolName: "Read", ToolCallID: "toolu_1", ToolInput: map[string]any{"x": 1}, ToolStatus: "completed", ToolOutput: "ok"}
	data, err := buildPartData(p)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		t.Fatal(err)
	}
	state := raw["state"].(map[string]any)
	if state["status"] != "completed" {
		t.Fatalf("status = %v", state["status"])
	}
	if _, ok := state["time"]; ok {
		t.Fatal("planner should not emit state.time (repair adds it)")
	}

	if _, err := buildPartData(&domain.Part{Type: domain.PartType("invalid")}); err == nil {
		t.Fatal("expected error for invalid part type")
	}
	if _, err := buildPartData(&domain.Part{Type: domain.PartTool}); err == nil {
		t.Fatal("expected error for tool part without name")
	}
}

func TestWriter_BackupIdempotent(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/db.sqlite"
	db, err := OpenDatabase(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), "INSERT INTO project(id, worktree, sandboxes) VALUES ('p','/x','[]')"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	dst := dir + "/backups"
	b1, err := Backup(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if b1 == "" {
		t.Fatal("backup path empty")
	}
	// Second backup in same second should still write a fresh file (timestamp
	// differs at second granularity only when clocks advance).
	b2, err := Backup(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	if b2 == "" {
		t.Fatal("second backup returned empty")
	}
}

// sanity: errors is in stdlib so we can use errors.New.
var _ = errors.New