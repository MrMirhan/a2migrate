// Package gemini_cli reads Gemini CLI's on-disk state.
//
// Status: STUB. See ROADMAP.md.
//
// On-disk layout (placeholder):
//
//	~/.gemini/
//	├── settings.json     // MCP + settings
//	├── skills/
//	└── sessions/<sid>.json    // Gemini uses JSON, not JSONL
package gemini_cli

import (
	"errors"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

func ReadGlobalSkills() ([]domain.Skill, error) {
	return nil, errors.New("gemini_cli: not implemented (see ROADMAP.md)")
}

func ReadGlobalMCP() ([]domain.MCPServer, error) {
	return nil, errors.New("gemini_cli: not implemented (see ROADMAP.md)")
}

func ReadGlobalSystemPrompt() (*domain.SystemPrompt, error) {
	return nil, nil
}
