# a2migrate

Migrate AI coding session state between agents.

`a2migrate` is a Go CLI that ports Claude Code sessions, skills,
commands, agents, rules, and MCP server configuration into OpenCode. It
preserves chat content (user prompts, assistant text, reasoning, tool
calls, subagent chains) and re-emits them as native OpenCode SQLite
rows that the renderer accepts.

## Status

v1 in development. Tested against `opencode.db` from OpenCode 1.18.x.

## Features

- Discover Claude Code sessions automatically across all projects.
- List, search, and filter sessions.
- Interactive multi-select TUI for choosing sessions.
- Migrate one, many, or all sessions in a single transactional write.
- Migrate skills, commands, agents, rules, and MCP servers.
- Idempotent re-runs — already-migrated sessions are skipped.
- Dry-run mode that produces a full plan without writing.
- Optional timestamped DB backup before apply.
- Cross-platform (Linux, macOS, Windows) — pure-Go SQLite, no cgo.
- Structured logging via `log/slog`.

## Quick start

```sh
# install
go install github.com/mirhan/a2migrate/cmd/a2migrate@latest

# list available sessions
a2migrate sessions list

# see what a migration would do
a2migrate sessions migrate --dry-run

# migrate one session (with backup)
a2migrate sessions migrate --search "bug fix" --backup --yes

# interactive picker
a2migrate sessions select

# migrate everything (sessions + skills + commands + agents + rules + mcp)
a2migrate all --backup --yes

# verify what landed in the DB
a2migrate sessions verify

# re-run post-fix invariants without re-migrating
a2migrate sessions repair
```

## Commands

```
a2migrate
├── sessions
│   ├── list              list discovered sessions
│   ├── show <id>         show one session's metadata
│   ├── select            interactive picker
│   ├── migrate           migrate (one/many/all) with filters + flags
│   ├── verify            list sessions already in the OC db
│   └── repair            re-run post-fix invariants
├── skills                migrate ~/.claude/skills/ → ~/.config/opencode/skills/
├── commands              migrate .claude/commands/ → .opencode/command/
├── agents                migrate .claude/agents/ → .opencode/agent/
├── rules                 migrate .claude/rules/ → .opencode/rules/
├── mcp                   merge ~/.claude/mcp.json into opencode.json
├── all                   run every migration domain
└── version
```

## Session migration flags

| Flag | Description |
|---|---|
| `--from <path>` | Override Claude Code home (default `~/.claude`). |
| `--to <path>` | Override OpenCode database path. |
| `--include <id>` | Only migrate sessions whose id matches (repeatable). |
| `--exclude <id>` | Skip sessions whose id matches (repeatable). |
| `--search <sub>` | Substring filter on id or file path. |
| `--rename <old=new>` | Rename a session during migration (repeatable). |
| `--backup` | Create a timestamped DB backup before apply. |
| `--dry-run` | Plan only; do not write. |
| `--yes` | Skip confirmation prompts. |
| `--skip-repair` | Skip the four post-fix invariants. |

## How it works

Session transcripts live as JSONL on disk under
`~/.claude/projects/<encoded-cwd>/<sid>.jsonl`. `a2migrate` streams
each line, normalizes the CC record envelope into OpenCode's two-tier
shape (message envelope + typed parts), and writes them in a single
SQLite transaction.

Four post-fix invariants are then run idempotently:

1. **Reparent** — assistant messages must have a user parent. CC
   occasionally emits assistant→assistant chains during tool streaming;
   those are re-pointed at the most recent prior user message.
2. **Pad step parts** — bare step-start/step-finish get the native
   fields the renderer expects (`tokens`, `cost`, `metadata`, `time`).
3. **Step-start time** — every step-start gets a `time` block.
4. **Tool state time** — every tool part gets `state.time.compacted`.

The same algorithms drive the original Python script's four post-fix
helpers; this implementation merges them into one transactional stage.

## What's preserved vs. lost

**Preserved:** session title, project directory, message text, reasoning,
tool calls with input/output, subagent chains via `session.parent_id`,
timestamps (ms-epoch), per-message role and agent.

**Not migrated:**
- Cost and token counts (CC has them; OC schema requires real provider
  attribution we don't have).
- Reasoning signatures (CC stores `signature` for replay; OC has no
  equivalent).
- `last-prompt` and `attachment` records (CC bookkeeping, no OC analog).
- Settings, credentials, plugins, daemon state (CC-only).

## What about OC → CC?

`a2migrate` is one-way in v1: CC → OC. The reverse direction is
non-trivial because the renderer writes message-level fields the CC
importer doesn't consume, and because every OC session would need a
project-decoded-cwd directory to land in. Tracked for v2.

## Development

```
make build         # produces ./a2migrate
make test          # runs the suite
make test-race     # race detector
make lint          # go vet
make cover         # coverage report
```

Cross-platform CI matrix on Linux / macOS / Windows, Go 1.23 + 1.24.

## License

MIT. See [LICENSE](./LICENSE).