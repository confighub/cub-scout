# AI Start Here

Use this page when you want to drive `connect-and-compare` safely with
Codex, Claude, Cursor, or another AI assistant.

## What This Example Is For

This is the 60-second connected-mode value story. It shows the progression
from standalone cluster signal (`doctor`) through ConfigHub connection
(`cub auth login`) to intent-vs-observed comparison (`compare`) and change
history (`history`).

This example is deterministic and does not require a live cluster.
All data comes from fixtures in `testdata/`.

## Read-Only First

Everything in this example is read-only. No commands mutate cluster state
or ConfigHub.

```bash
cd examples/connect-and-compare
./demo.sh
```

To verify generated output against committed snapshots:

```bash
./demo.sh --verify
```

## Important Boundaries

- `./demo.sh` generates output snapshots from fixture data — it does not
  connect to a cluster or ConfigHub
- `./demo.sh --verify` compares generated output against committed expected
  snapshots and exits non-zero on mismatch
- `./record-demo.sh` captures a terminal recording of the demo flow
- No commands in this example write to ConfigHub or modify cluster state

## What To Verify

After running `./demo.sh --verify`, check:

- exit code is 0 (all snapshots match)
- `doctor` output shows cluster signal from fixture input
- `compare` output shows intent-vs-observed alignment from fixture data
- `history` output shows ChangeSet timeline from fixture data

## Artifacts

| File | Purpose |
|------|---------|
| `testdata/doctor_input.json` | Fixture input for `cub-scout doctor` |
| `testdata/history_changesets.json` | Fixture input for `cub-scout history` |
| `expected-output/` | Committed expected snapshots for verification |

## Related Files

- [README.md](./README.md)
- [prompts.md](./prompts.md)
- [contracts.md](./contracts.md)
