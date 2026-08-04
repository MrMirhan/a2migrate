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
- OC → CC reverse migration (`a2migrate reverse` / `a2migrate oc-*`).
- Per-message token attribution preserved in both directions
  (CC `message.usage` ↔ OC `message.data.tokens`).
- `CLAUDE.md` ↔ `AGENTS.md` migration as part of `all`/`reverse`
  (top-level system-prompt file).

### Changed

- Session migration emits `ccOriginID` in the OC `message.data` blob
  so sync can deduplicate appends by uuid.

### Notes

- This is the v0.1 release line. Public API may evolve; pin to
  minor versions.
