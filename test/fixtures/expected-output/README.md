# Expected Output Fixtures

This directory contains expected output patterns for map command validation in connected mode.

## Hierarchy Modes

The map command supports two hierarchy modes that organize output differently:

### Admin Mode (`--mode=admin`)

Shows ConfigHub organizational hierarchy: **Org → Space → Unit**

```
ConfigHub Hierarchy:
Org → Space → Unit (with Resources, Targets, Workers)

ConfigHub (org: org_01jm...)
  └── tutorial (2 units, 22 targets, 2 workers)
    ├── chatapp-dev @ rev 5 → k8s-dev
    ├── prodchatapp-dev @ rev 2 → k8s-dev
  └── platform-dev (0 units, 21 targets, 1 workers)
  └── apptique-dev (11 units, 0 targets, 0 workers)
    ├── cartservice @ rev 5
    ├── frontend @ rev 6
    └── ... and 9 more units
```

**Expected patterns:**
- `ConfigHub Hierarchy:` header
- `ConfigHub (org: org_...)` organization line
- Space names with counts: `(N units, N targets, N workers)`
- Unit entries with revisions: `@ rev N`
- Target links: `→ target-slug`

### Fleet Mode (`--mode=fleet`)

Shows cluster-centric hierarchy: **Application → Variant → Cluster**

```
ConfigHub Fleet View
Hierarchy: Application → Variant → Cluster

unknown
├── variant: default
    └── cluster: docker-desktop

OWNERSHIP
────────────────────────────────────────────────
Argo(1) ConfigHub(2) Helm(1) Native(12)
████░░░░░░░░░░░░
```

**Expected patterns:**
- `ConfigHub Fleet View` or `Fleet Hierarchy` header
- `Hierarchy: Application → Variant → Cluster` structure
- Variant entries: `variant: X`
- Cluster entries: `cluster: X`
- Ownership visualization bar with `█` and `░` characters

## Issue #1 Regression Check

Both modes should NEVER contain null patterns from Issue #1:
- `null - unknown`
- `→ null`
- `: null`

## Test Command

```bash
./test/atk/verify-map-output              # Test both modes
./test/atk/verify-map-output --admin      # Admin mode only
./test/atk/verify-map-output --fleet      # Fleet mode only
./test/atk/verify-map-output --space=X    # Test specific space
```
