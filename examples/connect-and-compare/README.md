# Connect and Compare (60-second flow)

Fixture-first demo for the v1.6 connected-mode story:

1. `doctor` shows immediate standalone cluster signal.
2. operator connects with `cub auth login`.
3. compare view shows intent vs observed state using offline fixtures.
4. `history` shows who changed what from ChangeSets.

This example is deterministic and does not require a live cluster.

## Run

```bash
# Generate demo output snapshots
./examples/connect-and-compare/demo.sh

# Verify generated output against committed snapshots
./examples/connect-and-compare/demo.sh --verify
```

## Record

```bash
./examples/connect-and-compare/record-demo.sh
```

## Artifacts

- `testdata/doctor_input.json`: fixture input for `cub-scout doctor`
- `testdata/history_changesets.json`: fixture input for `cub-scout history`
- `expected-output/`: committed expected snapshots for verification
- `sample-output/`: generated output from the last local run (not committed)

## Notes

- Synthetic/demo history remains explicitly fixture-driven in this flow.
- No ConfigHub write operations are executed by this example.
- Compare step uses `cub-scout compare --git-path ... --bundle ... --json` for
  deterministic intent-vs-observed evidence from offline fixtures.
