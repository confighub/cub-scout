# cub-scout v0.5 — Contract-Locked Release

**v0.5 locks CLI behavior via golden tests.** Outputs are deterministic, documented, and safe to script against.

## What's Contracted

| Command | Contract |
|---------|----------|
| `map status` | `✓ healthy:` / `✗ <N> problem(s):` format, exit codes 0/1 |
| `map deployers --json` | Deployments only (not Flux/Argo CRDs), stable JSON schema |
| `map list --json` | Deterministic field ordering |
| `scan --file` | Static file scan with `STATIC FILE SCAN` header |
| `trace` | Ownership chain output, exit codes |

## Key Points

- **Deployers = Deployments** in v0.5 (GitOps CRDs deferred)
- **Glyphs are contract** (`✓` / `✗` in output)
- **Golden tests are source of truth** if docs diverge
- **No breaking changes** — existing behavior made explicit

## Build On This

v0.5 is the first version with an intentionally frozen CLI surface.
Script it. Integrate it. Extend it.
