package migrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirhan/a2migrate/internal/source/opencode"
)

// seedOCDB creates a fresh OC db, inserts one main + one subagent
// session with a few messages, and returns the db path.
func seedOCDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "oc.db")
	r := opencode.NewSessionReader(dbPath)
	db, err := r.Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO project(id, worktree, sandboxes) VALUES ('p1', '/fixture', '[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO session(id, project_id, slug, directory, title, version, metadata, parent_id) VALUES
		 ('ses_main', 'p1', 's', '/fixture', 'OC main', 'native', '{"claude_code_origin":"cc-1"}', NULL),
		 ('ses_sub', 'p1', 's2', '/fixture', 'OC sub', 'native', '{"claude_code_origin":"cc-2","claude_code_parent":"cc-1","is_subagent":true}', 'ses_main')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO message(id, session_id, time_created, data) VALUES
		 ('msg_main_u', 'ses_main', 1000, '{"role":"user","time":{"created":1000},"agent":"build","model":{"providerID":"p","modelID":"m","variant":"v"},"summary":{"diffs":[]}}'),
		 ('msg_main_a', 'ses_main', 1100, '{"role":"assistant","mode":"build","agent":"build","variant":"v","path":{"cwd":"/fixture","root":"/"},"cost":0,"tokens":{"input":0,"output":0,"reasoning":0,"cache":{"read":0,"write":0}},"modelID":"m","providerID":"p","time":{"created":1100}}'),
		 ('msg_sub_u', 'ses_sub', 1200, '{"role":"user","time":{"created":1200},"agent":"build","model":{"providerID":"p","modelID":"m","variant":"v"},"summary":{"diffs":[]}}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO part(id, message_id, session_id, data) VALUES
		 ('p1', 'msg_main_u', 'ses_main', '{"type":"text","text":"hi"}'),
		 ('p2', 'msg_main_a', 'ses_main', '{"type":"text","text":"hello back"}'),
		 ('p3', 'msg_main_a', 'ses_main', '{"type":"step-start"}'),
		 ('p4', 'msg_main_a', 'ses_main', '{"type":"step-finish"}'),
		 ('p5', 'msg_sub_u', 'ses_sub', '{"type":"text","text":"sub query"}')`); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

func TestReverseMigrator_Run_Basic(t *testing.T) {
	dbPath := seedOCDB(t)
	ccRoot := t.TempDir()
	m := NewReverseMigrator(ReverseOptions{From: dbPath, To: ccRoot, Yes: true})
	refs, err := m.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs = %d want 2", len(refs))
	}
	report, err := m.Run(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	if report.Successes != 2 {
		t.Fatalf("successes = %d want 2", report.Successes)
	}

	// Files should exist in ccRoot.
	mainPath := filepath.Join(ccRoot, "projects", "-fixture", "cc-1.jsonl")
	if _, err := os.Stat(mainPath); err != nil {
		t.Fatalf("main file missing: %v", err)
	}
	body, _ := os.ReadFile(mainPath)
	if !strings.Contains(string(body), `"role":"user"`) || !strings.Contains(string(body), `"role":"assistant"`) {
		t.Fatalf("missing user/assistant entries: %s", body)
	}

	// Subagent file.
	subPath := filepath.Join(ccRoot, "projects", "-fixture", "cc-1", "subagents", "agent-cc-2.jsonl")
	if _, err := os.Stat(subPath); err != nil {
		t.Fatalf("subagent file missing: %v", err)
	}
}

func TestReverseMigrator_Run_DryRun(t *testing.T) {
	dbPath := seedOCDB(t)
	ccRoot := t.TempDir()
	m := NewReverseMigrator(ReverseOptions{From: dbPath, To: ccRoot, DryRun: true})
	refs, _ := m.Discover(context.Background())
	report, err := m.Run(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	if !report.DryRun {
		t.Fatal("expected DryRun=true")
	}
	// ccRoot should still be empty (no files written).
	entries, _ := os.ReadDir(ccRoot)
	if len(entries) != 0 {
		t.Fatalf("dry-run wrote files: %v", entries)
	}
}

func TestReverseMigrator_FilterNative(t *testing.T) {
	dbPath := seedOCDB(t)
	m := NewReverseMigrator(ReverseOptions{From: dbPath, SkipNative: true})
	refs, err := m.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Both rows have origin (cc-1, cc-2) — neither is native.
	if len(refs) != 2 {
		t.Fatalf("refs = %d want 2 (both migrated from CC)", len(refs))
	}
}

func TestContainsFold(t *testing.T) {
	if !containsFold("Hello WORLD", "world") {
		t.Fatal("case-insensitive match failed")
	}
	if containsFold("abc", "xyz") {
		t.Fatal("non-match should fail")
	}
	if !containsFold("anything", "") {
		t.Fatal("empty needle always matches")
	}
}

func TestPlatformEncode(t *testing.T) {
	if platformEncode("/home/me/proj") != "-home-me-proj" {
		t.Fatal("encode mismatch")
	}
	if platformEncode("/") != "-" {
		t.Fatalf("root encode = %q", platformEncode("/"))
	}
}
