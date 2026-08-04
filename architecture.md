# Architecture

`a2migrate` is structured as five layers, with strict directional
dependencies from top to bottom:

```
cli/                       cobra commands, flag parsing, output formatting
  │
  ▼
migrate/                   orchestration: discovery → plan → apply
  │
  ├──► source/<system>/    reads from disk, produces domain types
  │
  └──► target/<system>/    consumes domain types, writes to disk/DB
                            │
                            └──► domain/    pure data types (no IO)
```

`domain` is a leaf package. Nothing below it. `platform` is also a leaf
(OS-specific path conventions, atomic IO).

## Packages

| Package | Purpose |
|---|---|
| `cmd/a2migrate` | Entrypoint, parses flags, exits. |
| `internal/cli` | Cobra command tree. Each command is a thin shim that wires flags into `migrate.Options` and prints the report. |
| `internal/migrate` | Orchestration. `SessionMigrator`, `ArtifactsMigrator`, `Verify`. |
| `internal/source/claudecode` | Reader for CC state — JSONL parser, project discovery, frontmatter parser, MCP/hooks readers. |
| `internal/target/opencode` | Writer for OC state — SQLite session/message/part writer, repair pipeline, filesystem writers for skills/commands/agents/rules/MCP. |
| `internal/domain` | `Session`, `Message`, `Part`, `Skill`, `Command`, `AgentDef`, `Rule`, `MCPServer`, `Hook`, `Project`. |
| `internal/interactive` | Bubbletea multi-select picker used by `sessions select`. |
| `internal/logging` | slog setup. |
| `internal/platform` | XDG-aware path resolution, atomic file IO. |
| `internal/version` | Build-time info. |

## Session migration data flow

```
JSONL on disk
    │
    ▼
parser.go (parseSessionStream)
    │
    ├──► collectToolResults   (index by tool_use_id)
    │
    ├──► deriveTitle         (ai-title > first user text > placeholder)
    │
    ├──► for each entry:
    │      user:    text part (tool_results rendered inline)
    │      assistant: text / reasoning / tool parts
    │
    ▼
domain.Session (in memory)
    │
    ▼
writer.go (PlanSessions)
    │
    ├──► load existing ids, project ids, origin ids
    │
    ├──► skip already-migrated (idempotency)
    │
    ├──► for each session:
    │      1. assign OC id = "ses_" + sha1(originID) base32
    │      2. INSERT project row (or skip if exists)
    │      3. INSERT session row + metadata JSON
    │      4. for each message: assign id, build data JSON
    │         emit step-start + parts + step-finish for assistants
    │
    ▼
opencode.db (transactional)

    ▼
repair.go (idempotent invariants)
    │
    ├──► reparent: assistant→assistant becomes assistant→user
    ├──► padStepParts: add native fields to step-start/step-finish
    ├──► addStepStartTime: add time block to bare step-start
    └──► addToolStateTime: add state.time.compacted to tool parts
```

## OpenCode SQLite schema

Four tables. The schema lives in `internal/target/opencode/schema.go`.
`SetupSchema` is idempotent (CREATE TABLE IF NOT EXISTS).

```
project   (id, worktree, ..., sandboxes, commands)
session   (id, project_id, parent_id, slug, directory, title,
           version, metadata, time_created, time_updated, ...)
message   (id, session_id, time_created, time_updated, data JSON)
part      (id, message_id, session_id, time_created, time_updated,
           data JSON)
```

`message.data` and `part.data` are JSON envelopes. The keys are
discovered empirically from a live `opencode.db` — they may drift in
future OpenCode versions. The Repair stage adapts to whatever the
renderer requires at the time.

## Adding a new target system

1. Create `internal/target/<system>/` with:
   - DDL / setup function (idempotent).
   - `Plan<Type>(...)` returning rows.
   - `Apply(plan)` running in a single transaction.
   - Repair invariants if the renderer needs padded fields.
2. Add `<system>Options` to `internal/migrate`.
3. Add a `<system>` cobra command under `internal/cli`.
4. Wire it into `internal/cli/all.go`.

The source-side stays the same; only the target-side is new.

## Adding a new source system

Mirror the structure:

1. `internal/source/<system>/` with readers for sessions + artifacts.
2. `internal/migrate/<system>.go` orchestrator.
3. Cobra command.

## Adding a new artifact type

Most artifacts (skills, commands, agents, rules, MCP) have the same
shape: source reader + target writer + a few flags. To add one:

1. Add a `domain.X` type.
2. Implement `Read<Global|Project>X()` in `source/claudecode`.
3. Add `XWriter` in `target/opencode` with `WriteGlobal` + `WriteProject`.
4. Wire into `ArtifactsMigrator` + cobra command.