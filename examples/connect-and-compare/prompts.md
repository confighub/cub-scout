# Copyable Prompts

## 1. Orient Me First

Read this example and do not mutate anything yet.

Explain:

- what this example is for
- what data it reads (fixtures, not live cluster)
- what it writes (generated snapshots only)
- what success looks like

Then run only:

```bash
./demo.sh --verify
```

## 2. Safe Walkthrough

Guide me through `connect-and-compare` step by step.

Before each step:

- explain what it does
- confirm it is read-only
- tell me what evidence surface it affects

Use this path:

```bash
./demo.sh
./demo.sh --verify
```

## 3. Verify The Output

After the demo runs, verify:

- `doctor` output matches expected snapshot
- `compare` output shows aligned and non-aligned entries
- `history` output shows ChangeSet timeline
- `./demo.sh --verify` exits 0

## 4. Call Out The Remaining Gap

Evaluate this example honestly.

Say whether:

- the fixture data covers the full connected-mode value story
- the comparison between intent and observed state is deterministic
- the example works without a live cluster or ConfigHub connection
