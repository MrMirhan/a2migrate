# Contributing

Contributions welcome. Before opening a PR, please:

1. Run `make test-race` and ensure everything passes.
2. Run `make lint` (uses `go vet`).
3. Match the existing code style — small focused functions, table-driven
   tests, no comments unless explaining non-obvious *why*.
4. Add a test for new behavior in the same package.

## Architecture

See `architecture.md` for the package layout and module boundaries.
Key invariants:

- `internal/domain` types are pure data, no behavior beyond validation.
- `internal/source/<system>` packages read from disk and produce domain
  types. They never write.
- `internal/target/<system>` packages consume domain types and write
  somewhere. They never read disk for input.
- `internal/migrate` orchestrates the two layers.
- `internal/cli` is the only package that knows about cobra commands.

## Schema changes

If you change the OpenCode SQLite schema or the message/part JSON
envelopes, update both:

- `internal/target/opencode/schema.go` — DDL.
- `internal/target/opencode/writer.go` — INSERTs and JSON envelopes.
- The Repair invariants must keep working with the new shape.

The renderer (OpenCode itself) is the source of truth for what
constitutes a valid migrated session. When in doubt, look at a
native-row dump.

## Release process

Releases are cut by pushing a signed tag. CI does the rest.

### Local dry-run (no publish)

```sh
make release-check   # validate .goreleaser.yaml only
make snapshot        # build all 5 platform binaries to ./dist/
```

Both are safe and don't touch the GitHub release.

### Cutting a release

```sh
# One-time: configure a signing key.
git config --global user.signingkey <key-id>

# Tag and push.
make release TAG=v0.1.0
# git tag -s v0.1.0 -m "Release v0.1.0"
# git push origin v0.1.0
```

The `.github/workflows/release.yml` workflow then:

1. Runs `goreleaser release --clean --skip=sign` on the tagged commit
   (GoReleaser `v2.7.2`, pinned in the workflow for reproducibility).
2. Builds five targets: `linux/amd64`, `linux/arm64`,
   `darwin/amd64`, `darwin/arm64`, `windows/amd64`.
3. Generates `.tar.gz` / `.zip` archives plus a `.SHA256SUMS` file.
4. Drafts a GitHub Release from the conventional commits in the diff
   (`feat:`, `fix:`, `perf:`, `refactor:`, `docs:`, etc.).
5. Attaches the binaries.

Binaries appear at the GitHub Releases page within ~2 minutes of the
tag push.

### Commit conventions

Use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat: <description>` — new user-facing capability
- `fix: <description>` — bug fix
- `docs: <description>` — docs only
- `test: <description>` — tests only
- `refactor: <description>` — internal restructuring
- `ci: <description>` — CI workflow changes

GoReleaser parses these for changelog grouping. Commits that don't
match still land in the release under "Other".

### Hard rules

- **No telemetry.** The binary makes no network calls except those the
  user explicitly invokes. No usage reporting, no crash dumps, no
  update checks, no phoning home. If your PR adds one, it will be
  rejected — even an opt-in one. State this in the PR description so
  reviewers don't have to look.
- **No bundled analytics SDKs.** Same reasoning.
- **Vendoring:** `go mod tidy` and standard `go.mod` are enough. No
  replace directives; nothing should require `git clone` of an
  external repo to build.