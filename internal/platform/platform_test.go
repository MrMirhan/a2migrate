package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrent(t *testing.T) {
	if Current() == Unknown {
		t.Fatalf("expected known OS, got %q", Current())
	}
}

func TestEncodeDecodeCWD(t *testing.T) {
	tests := []struct {
		path, encoded string
	}{
		{"/home/mirhan/works", "-home-mirhan-works"},
		{"/home/mirhan/works/ppg", "-home-mirhan-works-ppg"},
		{"/", "-"},
		{"/tmp", "-tmp"},
	}
	for _, tt := range tests {
		if got := EncodeCWD(tt.path); got != tt.encoded {
			t.Errorf("EncodeCWD(%q) = %q, want %q", tt.path, got, tt.encoded)
		}
		if got := DecodeCWD(tt.encoded); got != tt.path {
			t.Errorf("DecodeCWD(%q) = %q, want %q", tt.encoded, got, tt.path)
		}
	}
}

func TestPaths(t *testing.T) {
	if ClaudeCodeHome() == "" {
		t.Fatal("ClaudeCodeHome empty")
	}
	if OpenCodeDataHome() == "" {
		t.Fatal("OpenCodeDataHome empty")
	}
	if OpenCodeDBPath() == "" {
		t.Fatal("OpenCodeDBPath empty")
	}
	if !filepath.IsAbs(OpenCodeDBPath()) && Current() != Windows {
		t.Fatalf("OpenCodeDBPath %q not absolute", OpenCodeDBPath())
	}
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "x.txt")
	if err := AtomicWriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hi" {
		t.Fatalf("got %q want hi", got)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyFile(src, dst, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "x" {
		t.Fatal("copy mismatch")
	}
	if err := CopyFile(src, dst, false); err == nil {
		t.Fatal("expected ErrExist when dst exists and overwrite=false")
	}
	if err := CopyFile(src, dst, true); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}
}

func TestBackupFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	bak, err := BackupFile(src, "")
	if err != nil {
		t.Fatal(err)
	}
	if !Exists(bak) {
		t.Fatalf("backup %s missing", bak)
	}
}
