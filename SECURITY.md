# Security Policy

## Supported versions

a2migrate is pre-1.0. Security fixes land on the latest minor release
only; there are no backports to earlier tags.

| Version | Supported |
|---|---|
| latest release | yes |
| anything older | no |

## Reporting a vulnerability

Report privately through GitHub's
[security advisory form](https://github.com/MrMirhan/a2migrate/security/advisories/new).
Do not open a public issue for a suspected vulnerability.

Include what you have: affected version (`a2migrate version`), platform,
the steps that trigger it, and what you observed. A failing command line
is worth more than a description of one.

Expect an acknowledgement within a week. If a report is confirmed, the
fix ships in the next release and the advisory credits the reporter
unless anonymity is requested.

## Threat model

Knowing what this tool does bounds what counts as a vulnerability.

a2migrate reads and writes local files: Claude Code JSONL transcripts,
an OpenCode SQLite database, and configuration under the two tools'
config directories. It makes no network calls, has no telemetry, ships
no daemon, and holds no credentials.

In scope:

- Path traversal or an escape from the configured home directories,
  including via crafted session ids, project paths, or artifact
  filenames.
- Writing outside the target a user named, or destroying data a
  `--dry-run` claimed would be untouched.
- Code execution triggered by parsing a malicious session file, MCP
  config, or artifact frontmatter.
- Leaking transcript contents to anywhere other than the target the
  user named.

Out of scope:

- Whatever the migrated tools do with the files afterwards.
- Secrets that were already stored in plaintext by Claude Code or
  OpenCode. a2migrate copies MCP server configuration as it finds it,
  environment variables and headers included; it does not attempt to
  detect or redact credentials. Review what you migrate.
- Local attackers who already have read and write access to the
  directories involved. At that point the transcripts are theirs
  regardless.
