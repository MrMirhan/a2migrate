# a2migrate

Migrate AI coding session state between agents.

`a2migrate` is a Go CLI that ports your Claude Code sessions, skills,
commands, agents, rules, MCP servers, and top-level instructions between
OpenCode and Claude Code in both directions. It preserves chat content,
tool calls, subagent chains, and per-message token usage.

`all` migrates CC → OC; `reverse` migrates OC → CC; `sync` keeps both
sides in lock-step over time. All three are idempotent.

## Install

```sh
go install github.com/mirhan/a2migrate/cmd/a2migrate@latest
```

Or download a binary from [GitHub Releases](#). macOS, Linux, Windows —
no CGo, single static binary.

Verify:

```sh
a2migrate version
# a2migrate dev (commit ..., built ..., linux/amd64, go1.24)
```

## Quick start

```sh
# See what would migrate, with zero side effects.
a2migrate sessions list
a2migrate sessions migrate --dry-run --search "refactor auth"

# Migrate one session (with timestamped DB backup before apply).
a2migrate sessions migrate --search "bug fix" --backup --yes

# Migrate everything in one shot.
a2migrate all --backup --yes

# Push OC sessions back into CC (e.g. you bounced between tools).
a2migrate reverse --to /tmp/cc-home --yes

# Keep both sides in sync after every session ends.
a2migrate sync
```

## Features

### Migration (one-shot, copy semantics)

- **Sessions** — JSONL transcripts → SQLite rows (CC→OC) or vice versa.
  Includes user prompts, assistant text, reasoning blocks, tool calls
  with input/output, subagent chains via `session.parent_id`. Preserves
  per-message `message.usage` (CC) ↔ `message.data.tokens` (OC).
- **Skills** — markdown files copied with frontmatter round-tripped;
  lives in `~/.config/opencode/skills/` and `<cwd>/.opencode/skills/`.
- **Commands** — slash command definitions copied with their
  `argument-hint` and `allowed-tools` fields preserved.
- **Agents** — subagent definitions with model + tools preserved.
- **Rules** — path-scoped rule files with their `paths:` glob patterns.
- **MCP servers** — `mcpServers{}` ↔ `mcp{}` with `command[]` split /
  merge.
- **CLAUDE.md ↔ AGENTS.md** — the top-level system-prompt file that
  each tool injects into every session's context.

### Sync (continuous, reconcile semantics)

- **mtime last-writer-wins** for file artifacts. Files on only one side
  propagate to the other. Mtime is preserved across copies, so re-runs
  are no-ops.
- **uuid-deduped append-only** for sessions. New CC messages get
  appended to OC, and vice versa, with the CC `uuid` recorded in OC's
  `message.data` so duplicates are detected and skipped.
- **Bail when nothing is newer** — `sync` finishes instantly if both
  sides are already in sync.

### Cross-cutting

- **Idempotent.** Every code path is safe to re-run. Already-migrated
  sessions are detected via `claude_code_origin` metadata and skipped.
- **Transactional.** Session writes run in a single SQL transaction
  with a `--backup` flag.
- **Self-healing.** Post-fix invariants restore the four renderer
  requirements: assistant→user reparents, step-start/step-finish get
  padded with native fields, bare step-start gets a `time` block, every
  tool part gets `state.time.compacted`.
- **Cross-platform.** Linux, macOS, Windows. Pure-Go SQLite (no CGo).
- **Cross-architecture.** Builds on `linux/amd64`, `linux/arm64`,
  `darwin/amd64`, `darwin/arm64`, `windows/amd64`.

## Commands

```
a2migrate
├── sessions                 CC source → OC target
│   ├── list
│   ├── show <id>
│   ├── select               interactive picker
│   ├── migrate [--backup] [--dry-run] [--search] [--include] [--exclude]
│   │             [--rename old=new] [--skip-repair] [--yes]
│   ├── verify
│   └── repair
├── skills                   ~/.claude/skills/ → ~/.config/opencode/skills/
├── commands                 .claude/commands/ → .opencode/command/
├── agents                   .claude/agents/ → .opencode/agent/
├── rules                    .claude/rules/ → .opencode/rules/
├── mcp                      mcpServers{} → mcp{}
├── system                   ~/.claude/CLAUDE.md → ~/.config/opencode/AGENTS.md
├── all                      every CC→OC domain above
│
├── oc-sessions              OC source → CC target
│   ├── list
│   ├── show <oc-id>
│   ├── migrate
│   └── verify
├── oc-skills                .opencode/skills/ → .claude/skills/
├── oc-commands              .opencode/command/ → .claude/commands/
├── oc-agents                .opencode/agent/ → .claude/agents/
├── oc-rules                 .opencode/rules/ → .claude/rules/
├── oc-mcp                   mcp{} → mcpServers{}
├── oc-system                AGENTS.md → CLAUDE.md
├── reverse                  every OC→CC domain above
│
├── sync                     bidirectional CC↔OC reconciler
│   ├── all                  artifacts + sessions in both directions
│   ├── artifacts            mtime last-writer-wins
│   ├── sessions             CC → OC, append-only
│   └── reverse              OC → CC, append-only
│
└── version
```

## What gets preserved

| Field | CC → OC | OC → CC |
|---|---|---|
| Message role / agent / model | yes | yes |
| Per-message `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens` | yes | yes |
| Reasoning blocks | yes | yes |
| Tool calls (input + output) | yes | yes |
| Subagent chains | yes (`session.parent_id`) | yes (`bridge-session` + dir nesting) |
| Session title | yes | yes |
| Project directory | yes | yes |
| `time.completed` per message | n/a (OC only) | yes |
| Cost (`cost_usd` extension field on JSONL) | n/a (CC has no cost data) | yes |
| `last-prompt` / `attachment` / `queue-operation` records | dropped (no OC analog) | dropped (no CC analog) |
| Reasoning signatures (for model-side replay) | dropped | n/a |
| Plugin marketplace state / daemon / settings / credentials | not touched | not touched |

## Environment variables

| Variable | Effect |
|---|---|
| `CLAUDE_CODE_HOME` | Override `~/.claude`. |
| `OPENCODE_DATA_HOME` | Override `~/.local/share/opencode/`. |
| `OPENCODE_CONFIG_HOME` | Override `~/.config/opencode/`. |
| `OPENCODE_DISABLE_CLAUDE_CODE` | If set, OpenCode will not read anything from `~/.claude/`. |
| `OPENCODE_DISABLE_CLAUDE_CODE_PROMPT` | Suppress only the `~/.claude/CLAUDE.md` fallback. |
| `A2MIGRATE_LOG_LEVEL` | Override log level (`error`, `warn`, `info`, `debug`). |

XDG Base Directory spec is followed on Linux. macOS uses
`~/Library/...`. Windows uses `%AppData%` / `%LocalAppData%`.

## When to use `all` vs `reverse` vs `sync`

- **Just switched tools and want everything in one place.** Run `all`
  (CC → OC) or `reverse` (OC → CC). Idempotent.
- **Live on both tools and don't want drift.** Run `sync` after each
  session, or wire it into a cron / hook. `sync` updates whichever side
  has newer content; equal mtimes cost nothing.
- **Daily driver, mostly CC.** Run `all` once, then `sync sessions`
  to pick up new CC sessions when you open OC.
- **Daily driver, mostly OC.** Same, but `reverse` then `sync reverse`.

## When not to use this

- You're a Claude Code only user with no plans to try OpenCode. Stop here.
- You want a bidirectional mirror server. `a2migrate` runs on demand;
  it does not watch files in real time. (Use `sync` periodically
  instead, or hook it into your session-end hooks.)
- You need full fidelity, byte-for-byte transcript replay. Some
  metadata (signatures, queue ops, raw attachment blobs) is dropped
  in both directions.

## Development

```sh
make build         # produces ./a2migrate
make test         # runs the suite
make test-race    # race detector
make lint         # go vet
make cover        # coverage report (output: coverage.html)
```

Layout (see `architecture.md` for the full version):

```
cmd/a2migrate/         entry point
internal/cli/          cobra commands, flags, printing
internal/migrate/      orchestration: discovery → plan → apply
internal/source/       readers (CC JSONL, OC SQLite + artifact files)
internal/target/       writers (OC SQLite + artifact files)
internal/domain/       pure data types (no IO)
internal/sync/         bidirectional reconciler
internal/interactive/  bubbletea multi-select picker
internal/logging/      slog setup
internal/platform/     OS paths, atomic file IO
internal/version/      build-time info
```

## License

MIT. See [LICENSE](./LICENSE).
