# a2migrate

Migrate AI coding session state between agents.

`a2migrate` is a Go CLI that ports Claude Code sessions, skills, commands,
agents, rules, and MCP server configuration into OpenCode. It preserves chat
content (user prompts, assistant text, reasoning, tool calls, subagent chains)
and re-emits them as native OpenCode SQLite rows that the renderer accepts.

## Status

v1 in development. See `architecture.md` for the design.

## Features

- Discover Claude Code sessions on disk
- List, search, and interactively select sessions
- Migrate one, many, or all sessions with idempotent, transactional writes
- Migrate skills, commands, agents, rules, and MCP servers
- Dry-run mode, progress indicators, structured logging
- Cross-platform (Linux, macOS, Windows)
- Safe by default: timestamped backups, per-domain confirmation, partial-failure recovery

## Quick start

```sh
go install github.com/mirhan/a2migrate/cmd/a2migrate@latest

a2migrate sessions list
a2migrate sessions migrate --dry-run
a2migrate all --dry-run
```

## License

MIT