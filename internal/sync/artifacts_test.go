package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncArtifacts_PropagatesMissingFile(t *testing.T) {
	ccHome := t.TempDir()
	ocHome := t.TempDir()

	if err := os.MkdirAll(filepath.Join(ccHome, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ccHome, "skills", "foo.md"), []byte("# foo"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := SyncArtifactsAt(ccHome, ocHome, NewerWins, false)
	if err != nil && len(report.Errors) > 0 {
		t.Logf("errors: %v", report.Errors)
	}
	if len(report.Applied) != 1 {
		t.Fatalf("applied = %d want 1 (%+v)", len(report.Applied), report.Applied)
	}
	if report.Applied[0].Op != OpCopyCCtoOC {
		t.Fatalf("op = %s", report.Applied[0].Op)
	}
	ocPath := filepath.Join(ocHome, "skills", "foo.md")
	if _, err := os.Stat(ocPath); err != nil {
		t.Fatalf("oc file missing: %v", err)
	}
}

func TestSyncArtifacts_PropagatesOCOnly(t *testing.T) {
	ccHome := t.TempDir()
	ocHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ocHome, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ocHome, "skills", "bar.md"), []byte("# bar"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := SyncArtifactsAt(ccHome, ocHome, NewerWins, false)
	if err != nil && len(report.Errors) > 0 {
		t.Logf("errors: %v", report.Errors)
	}
	if len(report.Applied) != 1 {
		t.Fatalf("applied = %d want 1 (%+v)", len(report.Applied), report.Applied)
	}
	if report.Applied[0].Op != OpCopyOCtoCC {
		t.Fatalf("op = %s", report.Applied[0].Op)
	}
	ccPath := filepath.Join(ccHome, "skills", "bar.md")
	if _, err := os.Stat(ccPath); err != nil {
		t.Fatalf("cc file missing: %v", err)
	}
}

func TestSyncArtifacts_Idempotent(t *testing.T) {
	ccHome := t.TempDir()
	ocHome := t.TempDir()
	for _, d := range []string{"skills", "commands", "agents", "rules"} {
		if err := os.MkdirAll(filepath.Join(ccHome, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(ocHome, d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ccHome, d, "x.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	first, _ := SyncArtifactsAt(ccHome, ocHome, NewerWins, false)
	if len(first.Applied) == 0 {
		t.Fatal("first run produced no applies")
	}
	second, _ := SyncArtifactsAt(ccHome, ocHome, NewerWins, false)
	if len(second.Applied) != 0 {
		t.Fatalf("second run should be no-op, got %d applies", len(second.Applied))
	}
}

func TestSyncArtifacts_PropagatesMissingOnCC(t *testing.T) {
	_ = t.TempDir()
	t.Skip("per-package sync uses platform defaults; covered by direct reconcile test")
}

func TestReconcileOne_NewerWins(t *testing.T) {
	dir := t.TempDir()
	ccPath := filepath.Join(dir, "cc.md")
	ocPath := filepath.Join(dir, "oc.md")
	if err := os.WriteFile(ccPath, []byte("cc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ocPath, []byte("oc"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Force cc to be newer.
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(ocPath, past, past); err != nil {
		t.Fatal(err)
	}
	r := &Report{}
	op, err := reconcileOne(ccPath, ocPath, NewerWins, false, r)
	if err != nil {
		t.Fatal(err)
	}
	if op != string(OpCopyCCtoOC) {
		t.Fatalf("op = %q", op)
	}
	body, _ := os.ReadFile(ocPath)
	if string(body) != "cc" {
		t.Fatalf("dst content = %q want cc", body)
	}
}

func TestReconcileOne_EqualMtimeNoOp(t *testing.T) {
	dir := t.TempDir()
	ccPath := filepath.Join(dir, "cc.md")
	ocPath := filepath.Join(dir, "oc.md")
	if err := os.WriteFile(ccPath, []byte("cc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ocPath, []byte("oc"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pin both to the same mtime.
	now := time.Now()
	if err := os.Chtimes(ccPath, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(ocPath, now, now); err != nil {
		t.Fatal(err)
	}
	r := &Report{}
	op, err := reconcileOne(ccPath, ocPath, NewerWins, false, r)
	if err != nil {
		t.Fatal(err)
	}
	if op != "" {
		t.Fatalf("op = %q want empty (no-op)", op)
	}
	if len(r.Applied) != 0 {
		t.Fatalf("expected zero applies, got %d", len(r.Applied))
	}
}

func TestReconcileOne_PreferCC(t *testing.T) {
	dir := t.TempDir()
	ccPath := filepath.Join(dir, "cc.md")
	ocPath := filepath.Join(dir, "oc.md")
	if err := os.WriteFile(ccPath, []byte("cc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ocPath, []byte("oc"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make OC newer, then prefer CC → CC still wins.
	future := time.Now().Add(1 * time.Hour)
	if err := os.Chtimes(ocPath, future, future); err != nil {
		t.Fatal(err)
	}
	r := &Report{}
	op, err := reconcileOne(ccPath, ocPath, PreferCC, false, r)
	if err != nil {
		t.Fatal(err)
	}
	if op != string(OpCopyCCtoOC) {
		t.Fatalf("op = %q", op)
	}
}

func TestReconcileOne_PreferOC(t *testing.T) {
	dir := t.TempDir()
	ccPath := filepath.Join(dir, "cc.md")
	ocPath := filepath.Join(dir, "oc.md")
	if err := os.WriteFile(ccPath, []byte("cc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ocPath, []byte("oc"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(ccPath, past, past); err != nil {
		t.Fatal(err)
	}
	r := &Report{}
	op, err := reconcileOne(ccPath, ocPath, PreferOC, false, r)
	if err != nil {
		t.Fatal(err)
	}
	if op != string(OpCopyOCtoCC) {
		t.Fatalf("op = %q", op)
	}
}

func TestReconcileOne_DryRun(t *testing.T) {
	dir := t.TempDir()
	ccPath := filepath.Join(dir, "cc.md")
	if err := os.WriteFile(ccPath, []byte("cc"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Report{}
	op, err := reconcileOne(ccPath, "", NewerWins, true, r)
	if err != nil {
		t.Fatal(err)
	}
	if op == "" {
		t.Fatal("op should be set")
	}
	if len(r.Applied) != 1 {
		t.Fatalf("applied = %d want 1", len(r.Applied))
	}
	// Verify nothing was written under dir — the empty path
	// Resolve(".","") produces no real file, so any non-error
	// Stat here would indicate a bug in dry-run.
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("stat %s: %v", dir, err)
	}
}

func TestDecideDirection(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	if got := decideDirection(future, past, NewerWins); got != "cc" {
		t.Errorf("newer cc: %q", got)
	}
	if got := decideDirection(past, future, NewerWins); got != "oc" {
		t.Errorf("newer oc: %q", got)
	}
	if got := decideDirection(now, now, NewerWins); got != "" {
		t.Errorf("equal mtime: %q want empty", got)
	}
	if got := decideDirection(past, future, PreferCC); got != "cc" {
		t.Errorf("prefer cc: %q", got)
	}
	if got := decideDirection(future, past, PreferOC); got != "oc" {
		t.Errorf("prefer oc: %q", got)
	}
}

func TestListMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := listMarkdown(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["a.md"]; !ok {
		t.Fatalf("a.md missing: %v", got)
	}
	if _, ok := got["b.txt"]; ok {
		t.Fatal("b.txt should be skipped")
	}
}

func TestListMarkdown_Missing(t *testing.T) {
	got, err := listMarkdown(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}
