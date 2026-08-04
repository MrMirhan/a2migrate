package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirhan/a2migrate/internal/target/opencode"
)

// TestSessionMigrator_EndToEnd runs the full pipeline against a fixture
// CC home and verifies the OC DB ends up with the right rows.
func TestSessionMigrator_EndToEnd(t *testing.T) {
	ccRoot := t.TempDir()
	encoded := "-tmp-fixture"
	projDir := filepath.Join(ccRoot, "projects", encoded)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionID := "abc-123"
	jsonlPath := filepath.Join(projDir, sessionID+".jsonl")
	body := strings.Join([]string{
		`{"type":"user","uuid":"u1","parentUuid":null,"sessionId":"` + sessionID + `","timestamp":"2026-07-20T10:00:00Z","cwd":"/fixture","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","uuid":"a1","parentUuid":"u1","sessionId":"` + sessionID + `","timestamp":"2026-07-20T10:00:01Z","cwd":"/fixture","message":{"role":"assistant","content":[{"type":"text","text":"hello back"}]}}`,
		`{"type":"ai-title","title":"Greeting"}`,
		"",
	}, "\n")
	if err := os.WriteFile(jsonlPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "oc.db")
	m := NewSessionMigrator(Options{From: ccRoot, To: dbPath, Yes: true})
	refs, err := m.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	refs = m.Selected(refs)
	if len(refs) != 1 {
		t.Fatalf("expected 1 session, got %d", len(refs))
	}
	report, err := m.Run(context.Background(), refs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Successes != 1 {
		t.Fatalf("successes = %d want 1", report.Successes)
	}
	if report.Results[0].OCSessionID == "" {
		t.Fatal("OC session id missing")
	}

	db, err := opencode.OpenDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var nSessions, nMessages int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM session").Scan(&nSessions); err != nil {
		t.Fatal(err)
	}
	if nSessions != 1 {
		t.Fatalf("sessions = %d want 1", nSessions)
	}
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM message").Scan(&nMessages); err != nil {
		t.Fatal(err)
	}
	if nMessages != 2 {
		t.Fatalf("messages = %d want 2", nMessages)
	}
	var title string
	if err := db.QueryRowContext(context.Background(), "SELECT title FROM session").Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Greeting" {
		t.Fatalf("title = %q want Greeting", title)
	}

	// Re-run: should be a no-op (already migrated).
	m2 := NewSessionMigrator(Options{From: ccRoot, To: dbPath, Yes: true})
	refs2, _ := m2.Discover(context.Background())
	refs2 = m2.Selected(refs2)
	report2, err := m2.Run(context.Background(), refs2)
	if err != nil {
		t.Fatal(err)
	}
	if report2.Skipped != 1 {
		t.Fatalf("expected 1 skipped on re-run, got %+v", report2)
	}
}
