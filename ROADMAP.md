# Roadmap

a2migrate v0.1 covers Claude Code ↔ OpenCode, in both directions, on a
single machine, with idempotent file-based and DB-based sync. This
document tracks what's next.

## v0.2 — multi-tool adapters (stubbed, real implementation pending)

Planned tool support. Each line below lists what the v0.1 stubs already
declare in the registry, and the work that's still required before the
tool can be used in production.

### Codex CLI — `codex`

- **Status:** stub only. Source/target return "not implemented".
- **Open questions** (need answers before real impl):
  - Session JSONL field layout — `~/.codex/sessions/<sid>.jsonl`,
    exact key names, block format, tool naming.
  - MCP config location — `~/.codex/config.toml` is a guess; verify.
  - Skill directory layout — `~/.codex/skills/` is a guess.
- **Per-capability effort:** sessions ~2-3 days, MCP/skill/system ~half-day each.

### Qwen Code — `qwen_code`

- **Status:** stub only.
- **Open questions:**
  - Session transcript format (`.jsonl` vs `.json`).
  - Config file extension (`.json` vs `.toml`).
  - Skill location.
- **Per-capability:** sessions ~1-2 days.

### Gemini CLI — `gemini_cli`

- **Status:** stub only.
- **Open questions:**
  - Session transcripts are reportedly JSON not JSONL; need to confirm.
  - Whether Gemini reads a top-level system prompt equivalent.
- **Per-capability:** sessions ~2-3 days (parser complexity higher).

### Factory Droid — `factory_droid`

- **Status:** stub only.
- **Open questions:** session format, MCP config layout, skills path.
- **Per-capability:** sessions ~2-3 days.

When each lands:

1. Verify on-disk layout with `tree -a ~/.config/<tool>` on a fresh install.
2. Implement reader with fixture-driven tests (similar to
   `internal/source/claudecode/parser_test.go`).
3. Implement writer with idempotency tests.
4. Add to `migrate.ArtifactsMigrator` and `migrate.ReverseMigrator`.
5. Add to `sync.ArtifactsAt`-style artifact sync loop.
6. Update `internal/tools/README.md` capability matrix.

## v0.3 — remote multi-endpoint sync

Config schema shipped in v0.1 (`internal/config/`). What's missing:

1. **`a2migrate remote sync --config <file>`** command — iterates
   endpoints declared in the config, runs the per-tool sync against
   each.
2. **SSH transport** — `kind = "ssh"` is declared but unwired. Needs
   `golang.org/x/crypto/ssh` + a small rsync-style copy primitive.
   Alternative: shell out to `rsync`/`scp`. Decision pending.
3. **Per-endpoint capability filtering** — config has `tools = [...]`
   per endpoint but the sync loop doesn't honor it yet.
4. **Conflict markers** — if both local and remote edited the same
   session, decide which wins (sync's `prefer-cc` / `prefer-oc` is
   local-only today).
5. **Resume / retry semantics** — a 500-GB sync to a flaky VDS
   shouldn't restart from scratch. rsync's --partial model is the
   north star.
6. **Encryption-at-rest** — sessions may contain secrets. Out-of-scope
   for v0.3 but worth documenting.
7. **`a2migrate remote doctor`** — pre-flight that checks each
   endpoint reachable, schema version match, disk space, before
   kicking off a multi-hour sync.

## v0.4 — operational

- **`a2migrate init`** — write a starter `a2migrate.toml` config from
  `--from <template>` or interactive prompts.
- **`a2migrate doctor`** — verify a target DB isn't locked by another
  opencode instance.
- **`a2migrate watch <path>`** — incremental migration triggered by
  fsnotify on the CC JSONL directory. Out of scope for single-shot
  use; useful in a server context.
- **Backup rotation** — `--backup-dir` today writes one file per run.
  Add age-based rotation so you don't accumulate `.bak-*` forever.
- **`a2migrate verify --strict`** — checks for orphan records (CC side
  deleted a JSONL but OC row survives). Today verify is informational;
  strict mode would error.

## Decisions still pending

- **License** — MIT today. If any planned tool upstream is
  copyleft-licensed, decide whether a2migrate can interoperate via
  format conversion only.
- **Plugin model** — Go plugins (`build -buildmode=plugin`) are fragile
  cross-platform; embedding (compile-time) is simpler but requires a
  PR per adapter. We've gone with embedded for v0.1. Revisit if
  community grows.
- **Telemetry** — opt-in, anonymous, no session content. Implement
  before any cloud-mode add-on lands.
