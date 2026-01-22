# TUI vs GUI: Scope and Capabilities

How cub-scout (TUI) relates to ConfigHub (GUI) and what each can do.

## The Scope Rule

```
┌─────────────────────────────────────────────────────────────────┐
│  TUI (cub-scout)         │  GUI (confighub.com)                 │
├──────────────────────────┼──────────────────────────────────────┤
│  LIVE only               │  LIVE + GIT + Other sources          │
│  1 Cluster               │  N Clusters (Fleet)                  │
└──────────────────────────┴──────────────────────────────────────┘
```

**This is a design rule, not a limitation.**

- **TUI** = Fast, local, single-cluster. Derives everything from LIVE cluster data.
- **GUI** = Fleet-wide, multi-source. Aggregates LIVE (via Workers) + GIT (via provider integration).

---

## LIVE vs GIT Data

| From LIVE (Certain) | From GIT (Additional) |
|---------------------|----------------------|
| What resources exist | Other variants not deployed here |
| Who owns them | Base definitions (`apps/base/`) |
| Kustomization `spec.path` | What SHOULD exist (drift) |
| Applied revision/SHA | History, PRs, pending commits |

**Key insight:** You don't need Git to infer variant — the Kustomization stores `spec.path` in the cluster.

---

## Capability Comparison

| Capability | TUI (cub-scout) | GUI (confighub.com) |
|------------|-----------------|---------------------|
| **Single cluster** | ✓ Direct kubectl | ✓ Via Worker |
| **Multiple clusters** | Switch contexts | ✓ All Targets aggregated |
| **Ownership detection** | ✓ Labels/annotations | ✓ Same, fleet-wide |
| **Trace** | ✓ Full chain | ✓ Same + visual |
| **Scan** | ✓ PolicyReports | ✓ Same + history |
| **Infer variant** | ✓ From path | ✓ From path, all clusters |
| **Real-time** | ✓ Live query | Worker poll interval |
| **Git structure** | - | ✓ Provider integration |
| **Pending commits** | - | ✓ Compare Git vs Live |

---

## Architecture: Hub Owns Workers

```
HUB (owns Worker lifecycle)
├── Workers
│   ├── worker-east ──────────────────▶ prod-east (Target)
│   └── worker-west ──────────────────▶ prod-west (Target)
│
└── APP SPACES (select worker for deploy)
    └── payments-team
        ├── Unit: payment-api → deploys via worker-east
        └── Unit: payment-api → deploys via worker-west
```

- **Hub** owns Workers and their lifecycle
- **App Spaces** select which Worker to use for deploying Units
- **Workers** connect Hub to Targets and enable `refresh` / `import` operations

---

## Import Approaches

| Approach | Command | What It Sees |
|----------|---------|--------------|
| **From LIVE** | `cub-scout import -n myapp` | This cluster only |
| **From Fleet** | `cub-scout import --from-fleet` | All Targets via ConfigHub |
| **From Git** | GUI only | Complete structure + bases |

### Best Practice

1. Start with TUI for quick single-cluster checks
2. Connect to ConfigHub for fleet-wide visibility
3. Use GUI for import wizard with Git correlation

---

## When to Use Which

| Use Case | Recommended |
|----------|-------------|
| Quick health check | TUI: `cub-scout map` |
| Trace a resource | TUI: `cub-scout trace` |
| Find orphans | TUI: `cub-scout map orphans` |
| CI/CD integration | TUI: `cub-scout scan --json` |
| Fleet-wide view | GUI or TUI: `cub-scout map --hub` |
| Import from multiple clusters | GUI: Visual wizard |
| Compare Git vs Live | GUI: Drift detection |

---

## Summary

```
┌─────────────────────────────────────────────────────────────────────┐
│                           TUI (cub-scout)                            │
├─────────────────────────────────────────────────────────────────────┤
│  LIVE: ✓ Full access, one cluster at a time                        │
│  Best for: Quick checks, single cluster, CI/CD pipelines            │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                        GUI (confighub.com)                           │
├─────────────────────────────────────────────────────────────────────┤
│  LIVE: ✓ Fleet-wide, all targets aggregated                        │
│  GIT:  ✓ Provider integration, full structure                      │
│  Best for: Fleet view, import wizard, Git+Live correlation          │
└─────────────────────────────────────────────────────────────────────┘
```

---

## See Also

- [Live Cluster Inference](live-cluster-inference.md) — How detection works without Git
- [Architecture](architecture.md) — System design overview
