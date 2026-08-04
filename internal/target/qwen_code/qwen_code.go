// Package qwen_code writes Qwen Code state. STUB.
package qwen_code

import (
	"errors"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

type SkillWriter struct{ Home string }

func NewSkillWriter() *SkillWriter { return &SkillWriter{Home: ".qwen"} }

func (w *SkillWriter) WriteGlobal(skills []domain.Skill) ([]string, error) {
	return nil, errors.New("qwen_code: not implemented (see ROADMAP.md)")
}

type MCPConfigWriter struct{ Path string }

func NewMCPConfigWriter() *MCPConfigWriter { return &MCPConfigWriter{Path: ".qwen/config.json"} }

func (w *MCPConfigWriter) Apply(servers []domain.MCPServer) (string, error) {
	return "", errors.New("qwen_code: not implemented (see ROADMAP.md)")
}

type SystemPromptWriter struct{ Home string }

func NewSystemPromptWriter() *SystemPromptWriter { return &SystemPromptWriter{Home: ".qwen"} }

func (w *SystemPromptWriter) Write(p *domain.SystemPrompt) (string, error) {
	return "", errors.New("qwen_code: not implemented (see ROADMAP.md)")
}
