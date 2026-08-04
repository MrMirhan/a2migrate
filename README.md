# a2migrate

Migrate AI coding session state between agents.

`a2migrate` is a Go CLI that ports Claude Code sessions, skills,
commands, agents, rules, and MCP server configuration into OpenCode,
and back. It preserves chat content (user prompts, assistant text,
reasoning, tool calls, subagent chains) and re-emits them in the
target system's native shape.

Two directions:

- **CC → OC** (`a2migrate all`) — primary direction. Reads JSONL
  transcripts from `~/.claude/`, writes SQLite rows into
  `~/.local/share/opencode/opencode.db`, plus file copies for
  artifacts.
- **OC → CC** (`a2migrate reverse`) — secondary direction. Reads
  SQLite, writes JSONL back into `~/.claude/projects/`, plus file
  copies for artifacts.

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

# list available CC sessions
a2migrate sessions list

# see what a migration would do
a2migrate sessions migrate --dry-run

# migrate one session (with backup)
a2migrate sessions migrate --search "bug fix" --backup --yes

# interactive picker
a2migrate sessions select

# migrate everything (CC → OC)
a2migrate all --backup --yes

# verify what landed in the DB
a2migrate sessions verify

# re-run post-fix invariants without re-migrating
a2migrate sessions repair

# OC → CC: list what's in opencode.db
a2migrate oc-sessions list

# OC → CC: write JSONL back to ~/.claude/projects/
a2migrate oc-sessions migrate --yes

# OC → CC: everything (sessions + skills + commands + agents + rules + mcp)
a2migrate reverse --to /tmp/cc-home --yes
```

## Commands

```
a2migrate
├── sessions                        (CC source → OC target)
│   ├── list
│   ├── show <id>
│   ├── select                      interactive picker
│   ├── migrate
│   ├── verify
│   └── repair
├── skills                          CC→OC: ~/.claude/skills/ → ~/.config/opencode/skills/
├── commands                        CC→OC: .claude/commands/ → .opencode/command/
├── agents                          CC→OC: .claude/agents/ → .opencode/agent/
├── rules                           CC→OC: .claude/rules/ → .opencode/rules/
├── mcp                             CC→OC: mcpServers{} → mcp{} (opencode.json)
├── all                             run every CC→OC domain
│
├── oc-sessions                     (OC source → CC target)
│   ├── list                        list sessions in opencode.db
│   ├── show <oc-id>
│   ├── migrate                     write JSONL back into ~/.claude/projects/
│   └── verify                      list what's in opencode.db
├── oc-skills                       OC→CC: ~/.config/opencode/skills/ → ~/.claude/skills/
├── oc-commands                     OC→CC: .opencode/command/ → .claude/commands/
├── oc-agents                       OC→CC: .opencode/agent/ → .claude/agents/
├── oc-rules                        OC→CC: .opencode/rules/ → .claude/rules/
├── oc-mcp                          OC→CC: mcp{} → mcpServers{}
├── reverse                         run every OC→CC domain
│
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

`a2migrate` ships both directions in v1. The reverse path:
- Reads SQLite (`opencode.db`) — sessions, messages, parts, and
  metadata.
- Skips OC's synthetic `step-start` / `step-finish` parts (CC has no
  equivalent — the implicit turn boundary is the user/assistant
  alternation).
- Restores tool_use + tool_result pairs from OC's fused `tool` part.
- Emits JSONL with `parentUuid` / `uuid` / `sessionId` / `timestamp` /
  `cwd` per entry, plus a `bridge-session` record for subagents
  pointing at the parent's OC session id.
- Maps OC subagent metadata back into
  `~/.claude/projects/<encoded>/<parent-id>/subagents/agent-<id>.jsonl`.

Artifacts go file-by-file (singular → plural directory rename, mcp{}
→ mcpServers{} transform).

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