// Package codex_test pins the stub contract for the Codex target
// adapter. Once the real implementation lands, replace these with
// fixture-driven roundtrip tests.
package codex

import (
	"testing"
)

func TestTargetStub_WritersReturnNotImplemented(t *testing.T) {
	w := NewSkillWriter()
	if _, err := w.WriteGlobal(nil); err == nil {
		t.Fatal("WriteGlobal should return error until implemented")
	}
	m := NewMCPConfigWriter()
	if _, err := m.Apply(nil); err == nil {
		t.Fatal("Apply should return error until implemented")
	}
	s := NewSystemPromptWriter()
	if _, err := s.Write(nil); err == nil {
		t.Fatal("Write should return error until implemented")
	}
}
