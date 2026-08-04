// Package codex reads Codex CLI's on-disk state into domain types.
//
// Status: STUB. The Codex CLI was not verified against a live install
// at the time this package was added. Use at your own risk and please
// PR corrections once verified.
//
// On-disk layout (placeholders — verify before relying on them):
//
//	~/.codex/
//	├── config.toml         // MCP config + general settings
//	├── skills/             // *.md files, one per skill
//	└── sessions/           // <sid>.jsonl transcripts (similar to CC)
//
// The session JSONL is structurally close to CC's but with a few
// differences observed in early 2026:
//   - timestamps are RFC3339Nano with timezone offsets
//   - tool names use a different naming convention
//   - thinking blocks are absent in default mode
//
// See ROADMAP.md "Adding a new tool" for the work to complete.
package codex

import (
	"errors"
	"os"

	"github.com/mirhan/a2migrate/internal/domain"
	"github.com/mirhan/a2migrate/internal/platform"
)

// SessionsPath returns the directory Codex writes session JSONL files
// under. The exact layout has not been verified on a live install.
func SessionsPath() string {
	return platform.EnvOr("CODEX_HOME", "") + "/.codex/sessions"
}

// ReadGlobalSkills is a stub.
func ReadGlobalSkills() ([]domain.Skill, error) {
	return nil, errors.New("codex: not implemented (see ROADMAP.md)")
}

// ReadGlobalMCP is a stub.
func ReadGlobalMCP() ([]domain.MCPServer, error) {
	return nil, errors.New("codex: not implemented (see ROADMAP.md)")
}

// ReadGlobalSystemPrompt is a stub.
func ReadGlobalSystemPrompt() (*domain.SystemPrompt, error) {
	// TODO(sync): verify whether Codex reads a top-level instructions
	// file (likely AGENTS.md fallback). Until then return nil.
	_ = os.Getenv
	return nil, nil
}
