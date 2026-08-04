// Package factory_droid writes Factory Droid state. STUB.
package factory_droid

import (
	"errors"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

type SkillWriter struct{ Home string }

func NewSkillWriter() *SkillWriter { return &SkillWriter{Home: ".factory"} }

func (w *SkillWriter) WriteGlobal(skills []domain.Skill) ([]string, error) {
	return nil, errors.New("factory_droid: not implemented (see ROADMAP.md)")
}

type MCPConfigWriter struct{ Path string }

func NewMCPConfigWriter() *MCPConfigWriter { return &MCPConfigWriter{Path: ".factory/droid.json"} }

func (w *MCPConfigWriter) Apply(servers []domain.MCPServer) (string, error) {
	return "", errors.New("factory_droid: not implemented (see ROADMAP.md)")
}

type SystemPromptWriter struct{ Home string }

func NewSystemPromptWriter() *SystemPromptWriter { return &SystemPromptWriter{Home: ".factory"} }

func (w *SystemPromptWriter) Write(p *domain.SystemPrompt) (string, error) {
	return "", errors.New("factory_droid: not implemented (see ROADMAP.md)")
}
