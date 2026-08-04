package opencode

import (
	"context"
	"testing"

	"github.com/mirhan/a2migrate/internal/domain"
)

// seedPlan writes a plan with custom message data so we can test repair.
func seedCustomPlan(t *testing.T, w *SessionWriter, sess domain.Session, mutate func(p *Plan)) Plan {
	t.Helper()
	ctx := context.Background()
	plan, err := w.PlanSessions(ctx, []domain.Session{sess})
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(&plan)
	}
	if err := w.Apply(ctx, plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestRepair_Reparent_AssistantToUser(t *testing.T) {
	db := newTestDB(t)
	w := NewSessionWriter(db)

	sess := sampleSession()
	plan := seedCustomPlan(t, w, sess, nil)

	var assistantID string
	for _, m := range plan.Messages {
		if contains(m.Data, `"role":"assistant"`) {
			assistantID = m.ID
			break
		}
	}
	if assistantID == "" {
		t.Fatal("no assistant message in plan")
	}
	_ = plan

	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`UPDATE message SET data = json_set(data, '$.parentID', ?) WHERE id = ?`,
		assistantID, assistantID,
	); err != nil {
		t.Fatal(err)
	}

	rep, err := Repair(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Reparents == 0 {
		t.Fatal("expected at least one reparent")
	}
	var blob string
	if err := db.QueryRowContext(ctx, `SELECT data FROM message WHERE id = ?`, assistantID).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	if contains(blob, `"parentID":"`+assistantID+`"`) {
		t.Fatalf("self-loop parentID still present: %s", blob)
	}
}

func TestRepair_PadStepParts(t *testing.T) {
	db := newTestDB(t)
	w := NewSessionWriter(db)

	sess := sampleSession()
	seedCustomPlan(t, w, sess, nil)

	ctx := context.Background()
	// Sanity: bare step-start/step-finish before repair.
	rows, err := db.QueryContext(ctx,
		`SELECT count(*) FROM part WHERE json_extract(data, '$.type') IN ('step-start','step-finish') AND json_extract(data, '$.time') IS NULL`)
	if err != nil {
		t.Fatal(err)
	}
	rows.Next()
	var zero int
	_ = rows.Scan(&zero)
	rows.Close()
	if zero == 0 {
		t.Fatal("expected some step parts without time before repair (the planner emits bare parts)")
	}

	rep, err := Repair(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.PadsStepParts == 0 {
		t.Fatal("expected padStepParts to fix at least one row")
	}

	// Re-run: should be a no-op (idempotent).
	rep2, err := Repair(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.PadsStepParts != 0 {
		t.Fatalf("second repair should be 0 changes, got %d", rep2.PadsStepParts)
	}
}

func TestRepair_AddStepStartTime(t *testing.T) {
	db := newTestDB(t)
	w := NewSessionWriter(db)
	sess := sampleSession()
	seedCustomPlan(t, w, sess, nil)

	ctx := context.Background()
	rep, err := Repair(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	// padStepParts already adds time to step-finish. addStepStartTime adds
	// time to step-start. Both run.
	if rep.AddedStepStartTimes == 0 {
		t.Fatal("expected addStepStartTime to add time blocks")
	}
	// Verify every step-start now has a time block.
	rows, err := db.QueryContext(ctx,
		`SELECT count(*) FROM part WHERE json_extract(data, '$.type') = 'step-start' AND json_extract(data, '$.time') IS NULL`)
	if err != nil {
		t.Fatal(err)
	}
	rows.Next()
	var n int
	_ = rows.Scan(&n)
	rows.Close()
	if n != 0 {
		t.Fatalf("step-starts missing time after repair: %d", n)
	}
}

func TestRepair_AddToolStateTime(t *testing.T) {
	db := newTestDB(t)
	w := NewSessionWriter(db)
	sess := sampleSession()
	seedCustomPlan(t, w, sess, nil)

	ctx := context.Background()
	rep, err := Repair(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.AddedToolStateTimes == 0 {
		t.Fatal("expected tool state.time.compacted to be added")
	}
	// Every tool part should now have state.time.compacted=false.
	rows, err := db.QueryContext(ctx,
		`SELECT count(*) FROM part WHERE json_extract(data, '$.type') = 'tool' AND json_extract(data, '$.state.time.compacted') IS NULL`)
	if err != nil {
		t.Fatal(err)
	}
	rows.Next()
	var n int
	_ = rows.Scan(&n)
	rows.Close()
	if n != 0 {
		t.Fatalf("tool parts missing state.time.compacted: %d", n)
	}
}

func TestRepair_AllFour_Idempotent(t *testing.T) {
	db := newTestDB(t)
	w := NewSessionWriter(db)
	sess := sampleSession()
	seedCustomPlan(t, w, sess, nil)

	ctx := context.Background()
	rep1, err := Repair(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep1.Reparents+rep1.PadsStepParts+rep1.AddedStepStartTimes+rep1.AddedToolStateTimes == 0 {
		t.Fatal("expected first run to make changes")
	}
	rep2, err := Repair(ctx, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Reparents+rep2.PadsStepParts+rep2.AddedStepStartTimes+rep2.AddedToolStateTimes != 0 {
		t.Fatalf("second run should make 0 changes, got %+v", rep2)
	}
}

func TestRepair_NothingToDo_OnEmpty(t *testing.T) {
	db := newTestDB(t)
	rep, err := Repair(context.Background(), db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.SessionsScanned != 0 {
		t.Fatalf("scanned = %d want 0", rep.SessionsScanned)
	}
}

func TestRepair_ScopeByID(t *testing.T) {
	db := newTestDB(t)
	w := NewSessionWriter(db)

	sess1 := sampleSession()
	sess2 := sampleSession()
	sess2.OriginID = "cc-sess-2"
	sess2.ProjectDir = "/tmp/other"

	ctx := context.Background()
	p1, err := w.PlanSessions(ctx, []domain.Session{sess1})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := w.PlanSessions(ctx, []domain.Session{sess2})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Apply(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if err := w.Apply(ctx, p2); err != nil {
		t.Fatal(err)
	}
	rep, err := Repair(ctx, db, []string{p2.Sessions[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if rep.SessionsScanned != 1 {
		t.Fatalf("scanned = %d want 1", rep.SessionsScanned)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
