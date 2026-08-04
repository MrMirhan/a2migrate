package opencode

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirhan/a2migrate/internal/domain"
)

func newTestOCDB(t *testing.T) (dbPath string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "oc.db")
	return dbPath, func() {}
}

func TestReader_DiscoverSessions_Empty(t *testing.T) {
	dbPath, _ := newTestOCDB(t)
	r := NewSessionReader(dbPath)
	ctx := context.Background()
	db, err := r.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	refs, err := r.DiscoverSessions(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0, got %d", len(refs))
	}
}

func TestReader_Parse_NativeSession(t *testing.T) {
	dbPath, _ := newTestOCDB(t)
	r := NewSessionReader(dbPath)
	ctx := context.Background()
	db, err := r.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Seed a project + session + 3 messages + parts.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO project(id, worktree, name, time_created, time_updated, sandboxes)
		 VALUES ('p1', '/tmp/proj', 'proj', 1000, 2000, '[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO session(id, project_id, slug, directory, title, version, metadata, time_created, time_updated)
		 VALUES ('ses1', 'p1', 'slug', '/tmp/proj', 'Hello', 'native', '{"claude_code_origin":""}', 1000, 2000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO message(id, session_id, time_created, time_updated, data)
		 VALUES ('msg1', 'ses1', 1000, 1000,
		         '{"role":"user","time":{"created":1000},"agent":"build","model":{"providerID":"p","modelID":"m","variant":"v"},"summary":{"diffs":[]}}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO part(id, message_id, session_id, data) VALUES
		 ('prt1', 'msg1', 'ses1', '{"type":"text","text":"hi"}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO message(id, session_id, time_created, time_updated, data)
		 VALUES ('msg2', 'ses1', 1100, 1100,
		         '{"role":"assistant","mode":"build","agent":"build","variant":"v","path":{"cwd":"/tmp/proj","root":"/"},"cost":0,"tokens":{"input":0,"output":0,"reasoning":0,"cache":{"read":0,"write":0}},"modelID":"m","providerID":"p","time":{"created":1100}}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO part(id, message_id, session_id, data) VALUES
		 ('prt2', 'msg2', 'ses1', '{"type":"reasoning","text":"think"}'),
		 ('prt3', 'msg2', 'ses1', '{"type":"text","text":"hello back"}'),
		 ('prt4', 'msg2', 'ses1', '{"type":"step-start"}'),
		 ('prt5', 'msg2', 'ses1', '{"type":"step-finish"}')`); err != nil {
		t.Fatal(err)
	}

	refs, err := r.DiscoverSessions(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1, got %d", len(refs))
	}
	if refs[0].Title != "Hello" {
		t.Fatalf("title = %q", refs[0].Title)
	}
	sess, err := r.ParseSession(ctx, db, refs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("messages = %d want 2 (step-start/step-finish dropped)", len(sess.Messages))
	}
	if sess.Messages[0].Role != domain.RoleUser {
		t.Fatalf("first role = %s", sess.Messages[0].Role)
	}
	if len(sess.Messages[1].Parts) != 2 {
		t.Fatalf("assistant parts = %d want 2 (reasoning + text, no step parts)", len(sess.Messages[1].Parts))
	}
}

func TestReader_Parse_ToolPart(t *testing.T) {
	dbPath, _ := newTestOCDB(t)
	r := NewSessionReader(dbPath)
	ctx := context.Background()
	db, err := r.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO project(id, worktree, sandboxes) VALUES ('p1', '/x', '[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO session(id, project_id, slug, directory, title, version, metadata) VALUES
		 ('ses1', 'p1', 's', '/x', 't', 'native', '{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO message(id, session_id, time_created, data) VALUES
		 ('msg1', 'ses1', 1000,
		  '{"role":"assistant","modelID":"m","providerID":"p","time":{"created":1000}}')`); err != nil {
		t.Fatal(err)
	}
	toolJSON := `{"type":"tool","tool":"Read","callID":"toolu_1","state":{"status":"completed","input":{"path":"/x"},"output":"content","title":"Read"}}`
	if _, err := db.ExecContext(ctx,
		`INSERT INTO part(id, message_id, session_id, data) VALUES ('prt1', 'msg1', 'ses1', ?)`,
		toolJSON); err != nil {
		t.Fatal(err)
	}

	refs, _ := r.DiscoverSessions(ctx, db)
	sess, err := r.ParseSession(ctx, db, refs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("messages = %d", len(sess.Messages))
	}
	parts := sess.Messages[0].Parts
	if len(parts) != 1 || parts[0].Type != domain.PartTool {
		t.Fatalf("expected 1 tool part, got %v", parts)
	}
	if parts[0].ToolName != "Read" || parts[0].ToolStatus != "completed" {
		t.Fatalf("tool part = %+v", parts[0])
	}
}

func TestReader_Parse_Subagent(t *testing.T) {
	dbPath, _ := newTestOCDB(t)
	r := NewSessionReader(dbPath)
	ctx := context.Background()
	db, err := r.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO project(id, worktree, sandboxes) VALUES ('p1', '/x', '[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO session(id, project_id, slug, directory, title, version, metadata, parent_id) VALUES
		 ('ses1', 'p1', 's', '/x', 'parent', 'native', '{"claude_code_origin":"cc-1"}', NULL),
		 ('ses2', 'p1', 's2', '/x', 'child', 'native', '{"claude_code_origin":"cc-2","claude_code_parent":"cc-1","is_subagent":true}', 'ses1')`); err != nil {
		t.Fatal(err)
	}

	refs, err := r.DiscoverSessions(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs = %d want 2", len(refs))
	}
	var main, sub *SessionRef
	for i := range refs {
		if refs[i].IsSubagent {
			sub = &refs[i]
		} else {
			main = &refs[i]
		}
	}
	if main == nil || sub == nil {
		t.Fatal("expected one main + one subagent")
	}
	if !sub.IsSubagent {
		t.Fatal("sub should be subagent")
	}
	if sub.ParentID != "cc-1" {
		t.Fatalf("sub.ParentID = %q want cc-1", sub.ParentID)
	}
	if sub.OriginID != "cc-2" {
		t.Fatalf("sub.OriginID = %q want cc-2", sub.OriginID)
	}
	if main.OriginID != "cc-1" {
		t.Fatalf("main.OriginID = %q want cc-1", main.OriginID)
	}
}

func TestReader_StripJSONC(t *testing.T) {
	in := `{"a": 1, /* x */ "b": 2}
// line
"c": 3`
	got := stripJSONC(in)
	if strings.Contains(got, "/*") || strings.Contains(got, "//") {
		t.Fatalf("comments not stripped: %q", got)
	}
}

func TestReader_ParseOCMCP(t *testing.T) {
	in := []byte(`{
  "mcp": {
    "linear": {"type":"local","command":["linear-mcp","--flag"],"enabled":true},
    "remote": {"type":"remote","url":"https://x","enabled":true,"headers":{"Auth":"x"}}
  }
}`)
	servers, err := parseOCMCP(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("servers = %d want 2", len(servers))
	}
	if servers[0].Name != "linear" {
		t.Fatalf("first name = %s", servers[0].Name)
	}
	if servers[0].Command[0] != "linear-mcp" {
		t.Fatalf("first command = %v", servers[0].Command)
	}
	if !servers[1].IsLocal() == servers[1].IsLocal() {
		t.Fatal("comparison")
	}
	// second server should be remote
	for _, s := range servers {
		if s.Name == "remote" {
			if s.Type != "remote" {
				t.Fatalf("remote type = %s", s.Type)
			}
			if s.URL != "https://x" {
				t.Fatalf("remote url = %s", s.URL)
			}
		}
	}
}

func TestReader_SplitFrontmatter(t *testing.T) {
	in := "---\nname: foo\ndescription: bar\n---\nbody"
	fm, body := splitFrontmatter(in)
	if fmStringField(fm, "name") != "foo" {
		t.Fatalf("name = %v", fm)
	}
	if !strings.HasPrefix(body, "body") {
		t.Fatalf("body = %q", body)
	}
}

func TestReader_SplitFrontmatter_NoFM(t *testing.T) {
	in := "just body\n"
	fm, body := splitFrontmatter(in)
	if fm != nil {
		t.Fatalf("expected nil fm, got %v", fm)
	}
	if body != "just body\n" {
		t.Fatalf("body = %q", body)
	}
}
