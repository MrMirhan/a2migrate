// Package codex writes Codex CLI's on-disk state from domain types.
//
// Status: STUB. See ROADMAP.md.
package codex

import (
	"errors"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

// SkillWriter is a stub.
type SkillWriter struct{ Home string }

// NewSkillWriter defaults to ~/.codex/.
func NewSkillWriter() *SkillWriter { return &SkillWriter{Home: ".codex"} }

// WriteGlobal is a stub.
func (w *SkillWriter) WriteGlobal(skills []domain.Skill) ([]string, error) {
	return nil, errors.New("codex: not implemented (see ROADMAP.md)")
}

// MCPConfigWriter is a stub.
type MCPConfigWriter struct{ Path string }

// NewMCPConfigWriter defaults to ~/.codex/config.toml.
func NewMCPConfigWriter() *MCPConfigWriter { return &MCPConfigWriter{Path: ".codex/config.toml"} }

// Apply is a stub.
func (w *MCPConfigWriter) Apply(servers []domain.MCPServer) (string, error) {
	return "", errors.New("codex: not implemented (see ROADMAP.md)")
}

// SystemPromptWriter is a stub.
type SystemPromptWriter struct{ Home string }

// NewSystemPromptWriter defaults to ~/.codex/.
func NewSystemPromptWriter() *SystemPromptWriter { return &SystemPromptWriter{Home: ".codex"} }

// Write is a stub.
func (w *SystemPromptWriter) Write(p *domain.SystemPrompt) (string, error) {
	return "", errors.New("codex: not implemented (see ROADMAP.md)")
}
