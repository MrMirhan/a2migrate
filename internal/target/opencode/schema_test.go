package opencode

import (
	"context"
	"database/sql"
	"testing"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenDatabase(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpenDatabase_InMemory(t *testing.T) {
	db := newTestDB(t)
	if db == nil {
		t.Fatal("nil db")
	}
}

func TestMissingTables_FreshDB(t *testing.T) {
	db := newTestDB(t)
	missing, err := MissingTables(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing tables after SetupSchema: %v", missing)
	}
}

func TestMissingTables_EmptyDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	missing, err := MissingTables(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 4 {
		t.Fatalf("expected 4 missing, got %v", missing)
	}
}

func TestExistingIDs_EmptyDB(t *testing.T) {
	db := newTestDB(t)
	ids, err := ExistingIDs(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty, got %d", len(ids))
	}
}

func TestExistingIDs_AfterInsert(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO project(id, worktree, sandboxes) VALUES ('p1','/x','[]')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO session(id, project_id, slug, directory, title, version) VALUES ('s1','p1','s','/x','t','1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO message(id, session_id, data) VALUES ('m1','s1','{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO part(id, message_id, session_id, data) VALUES ('prt1','m1','s1','{}')`); err != nil {
		t.Fatal(err)
	}
	ids, err := ExistingIDs(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"s1", "m1", "prt1"} {
		if _, ok := ids[want]; !ok {
			t.Fatalf("missing %s in ExistingIDs", want)
		}
	}
}

func TestIsConstraintError(t *testing.T) {
	if !IsConstraintError(errSQLite("UNIQUE constraint failed: foo")) {
		t.Fatal("expected unique to match")
	}
	if IsConstraintError(nil) {
		t.Fatal("nil should not match")
	}
}

type sqliteErr string

func (e sqliteErr) Error() string { return string(e) }
func errSQLite(s string) error    { return sqliteErr(s) }