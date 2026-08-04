// Package qwen_code reads Qwen Code's on-disk state.
//
// Status: STUB. See ROADMAP.md.
//
// On-disk layout (placeholder):
//
//	~/.qwen/
//	├── config.json        // MCP + settings
//	├── skills/
//	└── sessions/<sid>.jsonl
package qwen_code

import (
	"errors"

	"github.com/mirhan/a2migrate/internal/domain"
)

func ReadGlobalSkills() ([]domain.Skill, error) {
	return nil, errors.New("qwen_code: not implemented (see ROADMAP.md)")
}

func ReadGlobalMCP() ([]domain.MCPServer, error) {
	return nil, errors.New("qwen_code: not implemented (see ROADMAP.md)")
}

func ReadGlobalSystemPrompt() (*domain.SystemPrompt, error) {
	return nil, nil
}
