// Package gemini_cli writes Gemini CLI state. STUB.
package gemini_cli

import (
	"errors"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

type SkillWriter struct{ Home string }

func NewSkillWriter() *SkillWriter { return &SkillWriter{Home: ".gemini"} }

func (w *SkillWriter) WriteGlobal(skills []domain.Skill) ([]string, error) {
	return nil, errors.New("gemini_cli: not implemented (see ROADMAP.md)")
}

type MCPConfigWriter struct{ Path string }

func NewMCPConfigWriter() *MCPConfigWriter { return &MCPConfigWriter{Path: ".gemini/settings.json"} }

func (w *MCPConfigWriter) Apply(servers []domain.MCPServer) (string, error) {
	return "", errors.New("gemini_cli: not implemented (see ROADMAP.md)")
}

type SystemPromptWriter struct{ Home string }

func NewSystemPromptWriter() *SystemPromptWriter { return &SystemPromptWriter{Home: ".gemini"} }

func (w *SystemPromptWriter) Write(p *domain.SystemPrompt) (string, error) {
	return "", errors.New("gemini_cli: not implemented (see ROADMAP.md)")
}
