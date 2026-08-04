# Changelog

All notable changes to a2migrate are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/), and this
project adheres to [Semantic Versioning](https://semver.org/).

Release commits follow [Conventional Commits](https://www.conventionalcommits.org/).
GoReleaser parses them and groups entries by type.

## [Unreleased]

### Added

- Bidirectional sync (`a2migrate sync`): mtime last-writer-wins for
  file artifacts (skills, commands, agents, rules, MCP,
  CLAUDE.md/AGENTS.md); uuid-deduped append-only for sessions.
- OC → CC migration, via `a2migrate migrate opencode claude-code`.
- Per-message token attribution preserved in both directions
  (CC `message.usage` ↔ OC `message.data.tokens`).
- `CLAUDE.md` ↔ `AGENTS.md` migration as one of the artifact domains
  (top-level system-prompt file).
- Shell completion for tool and domain arguments, driven by the tool
  registry. The domain positions offer only what both tools declare
  support for.
- `--backup` now covers the Claude Code target too, snapshotting every
  JSONL file about to be overwritten, subagent transcripts included.

### Changed

- **Breaking:** tools are arguments instead of commands. The 19
  top-level commands collapse to nine verbs:
  `migrate <from> <to> [domain...]`, `list`, `show`, `select`,
  `verify`, `repair`, plus `sync`, `tools`, `version`. Adding a tool no
  longer adds commands.
- **Breaking:** `all` and `reverse` are gone. Omitting the domain
  arguments migrates every shared domain; swapping `<from>` and `<to>`
  migrates the other way.
- **Breaking:** every `oc-*` command is removed with no alias. Use
  `a2migrate migrate opencode claude-code <domain>`.
- **Breaking:** the `--from` / `--to` path overrides are renamed
  `--source-path` / `--target-path`, since `<from>` and `<to>` now name
  tools.
- Naming one artifact domain now migrates only that domain. Previously
  `a2migrate skills` also migrated commands, agents, rules, MCP, and the
  system prompt.
- Session migration emits `ccOriginID` in the OC `message.data` blob
  so sync can deduplicate appends by uuid.

### Notes

- This is the v0.1 release line. Public API may evolve; pin to
  minor versions.
