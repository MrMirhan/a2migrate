package interactive

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestIsatty_NonTerminals guards the fallback that keeps the picker from
// launching where it cannot be driven. A pipe or a redirect must read as
// non-interactive, or bubbletea fails with an error the user cannot act
// on.
func TestIsatty_NonTerminals(t *testing.T) {
	if isatty(&bytes.Buffer{}) {
		t.Error("a buffer is not a terminal")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	if isatty(r) {
		t.Error("a pipe is not a terminal")
	}

	path := filepath.Join(t.TempDir(), "in.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if isatty(f) {
		t.Error("a regular file is not a terminal")
	}

	closed, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	if isatty(closed) {
		t.Error("a closed file cannot be stat'ed and is not a terminal")
	}
}

// TestRun_NonInteractiveReturnsNil pins the contract callers branch on:
// no terminal means no selection and no error, so they can fall back to
// flags instead of failing.
func TestRun_NonInteractiveReturnsNil(t *testing.T) {
	got, err := Run([]Item{{Title: "a", ID: "a"}}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run on a non-terminal errored: %v", err)
	}
	if got != nil {
		t.Errorf("Run on a non-terminal returned %v, want nil", got)
	}
}
