package opencode

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// Schema returns the canonical CREATE TABLE statements for a fresh
// OpenCode database. Used by tests and by SetupSchema which can be called
// to bootstrap an empty database.
const Schema = `
CREATE TABLE IF NOT EXISTS project (
  id TEXT PRIMARY KEY,
  worktree TEXT NOT NULL,
  vcs TEXT,
  name TEXT,
  icon_url TEXT,
  icon_color TEXT,
  icon_url_override TEXT,
  time_created INTEGER,
  time_updated INTEGER,
  time_initialized INTEGER,
  sandboxes TEXT NOT NULL DEFAULT '[]',
  commands TEXT
);

CREATE TABLE IF NOT EXISTS session (
  id TEXT PRIMARY KEY,
  project_id TEXT,
  workspace_id TEXT,
  parent_id TEXT,
  slug TEXT NOT NULL,
  directory TEXT NOT NULL,
  path TEXT,
  title TEXT NOT NULL,
  version TEXT NOT NULL,
  share_url TEXT,
  summary_additions INTEGER,
  summary_deletions INTEGER,
  summary_files TEXT,
  summary_diffs TEXT,
  metadata TEXT,
  cost REAL DEFAULT 0,
  tokens_input INTEGER DEFAULT 0,
  tokens_output INTEGER DEFAULT 0,
  tokens_reasoning INTEGER DEFAULT 0,
  tokens_cache_read INTEGER DEFAULT 0,
  tokens_cache_write INTEGER DEFAULT 0,
  revert TEXT,
  permission TEXT,
  agent TEXT,
  model TEXT,
  time_created INTEGER,
  time_updated INTEGER,
  time_compacting INTEGER,
  time_archived INTEGER
);

CREATE TABLE IF NOT EXISTS message (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  time_created INTEGER,
  time_updated INTEGER,
  data TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS part (
  id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  time_created INTEGER,
  time_updated INTEGER,
  data TEXT NOT NULL
);
`

// expectedTables lists every table SetupSchema creates. MissingTables
// reports the subset not present in db.
var expectedTables = []string{"project", "session", "message", "part"}

// SetupSchema creates all tables if missing. Idempotent.
func SetupSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, Schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

// MissingTables returns the subset of expectedTables not present in db.
func MissingTables(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		return nil, fmt.Errorf("query sqlite_master: %w", err)
	}
	defer func() { _ = rows.Close() }()

	present := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		present[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var missing []string
	for _, want := range expectedTables {
		if !present[want] {
			missing = append(missing, want)
		}
	}
	return missing, nil
}

// OpenDatabase opens or creates an SQLite database at path with safe pragmas.
// The caller owns the *sql.DB and must Close it.
func OpenDatabase(ctx context.Context, path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("empty database path")
	}
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	if err := SetupSchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// ExistingIDs returns the union of all primary keys across session/message/part.
func ExistingIDs(ctx context.Context, db *sql.DB) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	for _, table := range []string{"session", "message", "part"} {
		rows, err := db.QueryContext(ctx, "SELECT id FROM "+table)
		if err != nil {
			return nil, fmt.Errorf("query %s ids: %w", table, err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return nil, err
			}
			out[id] = struct{}{}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}
	return out, nil
}

// ExistingProjectIDs returns all currently-known project ids so the writer
// can skip projects that are already in the database.
func ExistingProjectIDs(ctx context.Context, db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, "SELECT id FROM project")
	if err != nil {
		return nil, fmt.Errorf("query project ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// ExistingOriginIDs returns every CC origin id already migrated (extracted
// from session.metadata). Used to make PlanSessions idempotent.
func ExistingOriginIDs(ctx context.Context, db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT json_extract(metadata, '$.claude_code_origin') FROM session
		 WHERE json_extract(metadata, '$.claude_code_origin') IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("query origin ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

// IsConstraintError reports whether err looks like a SQLite UNIQUE / PRIMARY
// KEY violation.
func IsConstraintError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "UNIQUE constraint failed") ||
		strings.Contains(s, "PRIMARY KEY constraint failed")
}
