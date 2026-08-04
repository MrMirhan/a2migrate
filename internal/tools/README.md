# Tools Registry

This directory enumerates every AI coding CLI that a2migrate knows how to
read and write. The mapping lives in `registry.go`; this README is the
human-readable companion.

## Status

| Tool | ID | Sessions | Skills | Commands | Agents | Rules | MCP | System prompt |
|---|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| **Claude Code** | `claude_code` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| **OpenCode** | `opencode` | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

Planned (these IDs are reserved; the source/target packages are stubs):

| Tool | ID | Sessions | Skills | Commands | Agents | Rules | MCP | System prompt |
|---|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| **Codex CLI** | `codex` | ✓ (planned) | – | – | – | – | ✓ (planned) | ✓ (planned) |
| **Qwen Code** | `qwen_code` | ✓ (planned) | – | – | – | – | ✓ (planned) | ✓ (planned) |
| **Gemini CLI** | `gemini_cli` | ✓ (planned) | ✓ (planned) | – | – | – | ✓ (planned) | ✓ (planned) |
| **Factory Droid** | `factory_droid` | ✓ (planned) | – | – | – | – | ✓ (planned) | ✓ (planned) |

## On-disk locations

When a tool is unknown to a2migrate, the registry returns its default
path so a curious user can `ls` it. Each tool's data root is determined
by following the upstream docs and verifying on a fresh install:

| Tool | Config root | Data root | Session layout |
|---|---|---|---|
| Claude Code | `~/.claude/` | `~/.claude/` | `projects/<encoded-cwd>/<sid>.jsonl` |
| OpenCode | `~/.config/opencode/` | `~/.local/share/opencode/` | `opencode.db` (SQLite) |
| Codex | `~/.codex/` (planned) | `~/.codex/` (planned) | `<sid>.jsonl` (planned) |
| Qwen Code | `~/.qwen/` (planned) | `~/.qwen/` (planned) | `<sid>.jsonl` (planned) |
| Gemini CLI | `~/.gemini/` (planned) | `~/.gemini/` (planned) | `<sid>.json` (planned) |
| Factory Droid | `~/.factory/` (planned) | `~/.factory/` (planned) | `<sid>.jsonl` (planned) |

These paths are placeholders. Verify against each tool's official
docs before locking them into the registry.

## Naming convention

- ID is kebab-cased, all lowercase, stable across versions (no renames
  without an alias shim).
- Package paths are `internal/source/<id_underscored>` and
  `internal/target/<id_underscored>`. Example:
  `internal/source/codex/` for `ID = "codex"`.

## Adding a new tool

1. Verify the tool's storage layout (run it once, `tree -a ~/.config`,
   document what maps to what).
2. Append the entry to `knownTools` in `registry.go` with the planned
   capabilities marked.
3. Create `internal/source/<id>/` and `internal/target/<id>/` with
   `SessionReader`, `Read*`, and `*Writer` skeletons.
4. Wire into `internal/migrate` (Migrator + ReverseMigrator) and
   `internal/sync`.
5. Update the matrix table above and `ROADMAP.md`.
6. Add tests for every capability in `internal/source/<id>/` and
   `internal/target/<id>/`.
