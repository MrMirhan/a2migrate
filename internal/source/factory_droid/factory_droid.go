// Package factory_droid reads Factory Droid's on-disk state.
//
// Status: STUB. See ROADMAP.md.
//
// On-disk layout (placeholder):
//
//	~/.factory/
//	├── droid.json        // settings, MCP
//	└── sessions/<sid>.jsonl
package factory_droid

import (
	"errors"

	"github.com/MrMirhan/a2migrate/internal/domain"
)

func ReadGlobalSkills() ([]domain.Skill, error) {
	return nil, errors.New("factory_droid: not implemented (see ROADMAP.md)")
}

func ReadGlobalMCP() ([]domain.MCPServer, error) {
	return nil, errors.New("factory_droid: not implemented (see ROADMAP.md)")
}

func ReadGlobalSystemPrompt() (*domain.SystemPrompt, error) {
	return nil, nil
}
