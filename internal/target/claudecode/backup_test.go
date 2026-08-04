package claudecode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirhan/a2migrate/internal/domain"
)

// TestSessionWriter_BackupDir_MainAndSubagent proves the reverse
// direction's --backup covers subagent JSONL files too, not just main
// sessions.
func TestSessionWriter_BackupDir_MainAndSubagent(t *testing.T) {
	ccHome := t.TempDir()
	backups := filepath.Join(t.TempDir(), "backups")
	proj := "/tmp/proj"

	plain := &SessionWriter{CCHome: ccHome}
	mainPath, err := plain.WriteSession(domain.Session{
		OriginID: "s1", ProjectDir: proj, Title: "first",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	subPath, err := plain.WriteSession(domain.Session{
		OriginID: "a1", ProjectDir: proj, Title: "sub", IsSubagent: true,
	}, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte("original-main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subPath, []byte("original-sub\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	backing := &SessionWriter{CCHome: ccHome, BackupDir: backups}
	if _, err := backing.WriteSession(domain.Session{
		OriginID: "s1", ProjectDir: proj, Title: "rewritten",
	}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := backing.WriteSession(domain.Session{
		OriginID: "a1", ProjectDir: proj, Title: "rewritten sub", IsSubagent: true,
	}, "s1"); err != nil {
		t.Fatal(err)
	}

	saved := map[string]bool{}
	err = filepath.Walk(backups, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		saved[strings.TrimSpace(string(body))] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk backups: %v", err)
	}
	for _, want := range []string{"original-main", "original-sub"} {
		if !saved[want] {
			t.Errorf("backup missing %q; saved=%v", want, saved)
		}
	}
}

// TestBackupFile_MissingSourceIsNoOp matches opencode.Backup's contract.
func TestBackupFile_MissingSourceIsNoOp(t *testing.T) {
	root := t.TempDir()
	got, err := BackupFile(filepath.Join(root, "nope.jsonl"), root, filepath.Join(root, "backups"))
	if err != nil {
		t.Fatalf("BackupFile on missing source: %v", err)
	}
	if got != "" {
		t.Fatalf("expected no backup path, got %q", got)
	}
}
