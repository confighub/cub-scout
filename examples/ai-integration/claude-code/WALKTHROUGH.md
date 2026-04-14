# Claude Code Walkthrough: Debug Kubernetes with cub scout

## Goal

Show a complete, reproducible AI-assisted debugging flow without a live cluster.

## Setup

```bash
go build ./cmd/cub-scout
./examples/ai-integration/run-fixture-session.sh
```

## Session Flow

1. Ask Claude: "Debug why deployment/checkout is failing."
2. Claude reviews `sample-output/01-debug-deployment.txt`.
3. Claude identifies source signal: `OutOfSync` and stale reconcile timestamp.
4. Ask Claude: "What changed since yesterday?"
5. Claude uses `sample-output/02-change-history.txt` to summarize the two latest ChangeSets.
6. Ask Claude: "Is this safe to deploy?"
7. Claude inspects `sample-output/03-scan-safety.json` and highlights finding `CCVE-2025-0244`.
8. Ask Claude: "Show unmanaged resources."
9. Claude reviews `sample-output/04-unmanaged-resources.txt` and flags native resources for ownership cleanup.

## Why This Is Useful

- CLI mode and MCP mode both use the same `cub-scout` command contract.
- In user-facing prose, prefer `cub scout`; local repo commands remain `./cub-scout ...`.
- Every step is deterministic and replayable from local fixtures.
- You can move from demo mode to live-cluster mode by removing fixture env vars.
