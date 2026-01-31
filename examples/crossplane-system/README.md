# Crossplane system ownership example

This directory contains a small set of **Crossplane control-plane** resources (package manager + API extension resources).

Expected behavior:
- `DetectOwnership` returns `type=crossplane, subType=system` for these resources.
- They do **not** show up as "unmanaged" / "orphan" in summaries.

See `crossplane-system.yaml`.
