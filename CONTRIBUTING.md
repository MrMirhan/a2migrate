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