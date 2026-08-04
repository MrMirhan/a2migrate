# Changelog

All notable changes to a2migrate are documented here. The format is
based on [Keep a Changelog](https://keepachangelog.com/), and this
project adheres to [Semantic Versioning](https://semver.org/).

Release commits follow [Conventional Commits](https://www.conventionalcommits.org/).
GoReleaser parses them to generate the GitHub Release notes; this file
is maintained by hand and ships inside the release archives.

## [Unreleased]

## [0.1.1] - 2026-08-04

First usable release. Migrates state between Claude Code and OpenCode in
both directions.

v0.1.0 was tagged with the module path `github.com/mirhan/a2migrate`,
which is a different account's namespace, so `go install` could never
resolve it. The Go module proxy caches versions immutably, so that
number cannot be corrected and was retired.

### Added

- `migrate <from> <to> [domain...]` moves sessions and artifacts between
  two tools. Swapping the arguments migrates the other way; omitting the
  domains migrates every domain both tools support.
- Domains: sessions, skills, commands, agents, rules, MCP servers, and
  the `CLAUDE.md` ↔ `AGENTS.md` system prompt.
- Session fidelity in both directions: user and assistant text,
  reasoning blocks, tool calls with inputs and outputs, subagent chains,
  and per-message token counts (`message.usage` ↔ `message.data.tokens`).
  OpenCode's per-message cost survives into Claude Code as a `cost_usd`
  extension field.
- `list`, `show`, `select`, `verify`, and `repair`, each taking the tool
  as an argument. `verify` reports which sessions were migrated in and
  which are native to the tool.
- `sync` reconciles both sides continuously: mtime last-writer-wins for
  file artifacts, uuid-deduped append-only for sessions.
- `--backup` snapshots the target before writing — the SQLite database
  for OpenCode, every JSONL file about to be overwritten (subagent
  transcripts included) for Claude Code.
- `--dry-run` on every write path, and idempotent re-runs: already
  migrated sessions are detected via `claude_code_origin` metadata and
  skipped.
- Filtering with `--search`, `--include`, `--exclude`, and renaming with
  `--rename old=new`. Selecting a parent session brings its subagents
  along rather than orphaning them.
- Post-migration repair for OpenCode's renderer invariants:
  assistant→user reparenting, step-part padding, step-start timestamps,
  and tool-state times.
- Shell completion driven by the tool registry. Domain arguments offer
  only what both tools declare support for, so an unsupported
  combination cannot be completed into existence.
- `tools` lists the registry and each tool's capability matrix.
- Prebuilt binaries for Linux, macOS, and Windows on amd64 and arm64,
  plus a Homebrew cask for macOS. Pure-Go SQLite, no CGo.

### Notes

- Claude Code and OpenCode are wired today. Adapter scaffolding exists
  for Codex, Qwen Code, Gemini CLI, and Factory Droid; their format
  parsers are not implemented yet.
- Pre-1.0: the command surface may still change. Pin to a minor version.
