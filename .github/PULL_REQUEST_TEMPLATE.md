## What changed

<!-- One or two sentences. What does this do, and why? -->

## Checklist

- [ ] `make test-race` passes
- [ ] `make lint` passes and `gofmt -l .` is empty
- [ ] New behavior has a test in the same package
- [ ] Layer boundaries respected: `source/` never writes, `target/`
      never reads disk for input, only `cli/` knows about cobra
      (see [CONTRIBUTING.md](../CONTRIBUTING.md))

## Migration safety

<!-- Delete this section if the change cannot touch user data. -->

- [ ] `--dry-run` still reports exactly what a real run would do
- [ ] Re-running the command is still a no-op (idempotent)
- [ ] Checked in both directions, not just the one I was working on

## Notes for the reviewer

<!-- Anything you are unsure about, deliberately left out, or want
     argued with. Known gaps are more useful here than in a follow-up. -->
