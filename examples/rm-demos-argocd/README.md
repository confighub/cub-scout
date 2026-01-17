# Rendered Manifest Pattern Demos — ArgoCD Edition

> **⚠️ SIMULATION DEMOS** — These scripts print simulated output to demonstrate what ConfigHub WILL do when the Rendered Manifest features are implemented. They do NOT connect to real clusters or ConfigHub. Use them for storytelling and presentations.

---

> **The goal isn't to show ConfigHub features. It's to make people feel: "I need this NOW."**

These demos are designed to motivate feedback and planning steps — they are not feature walkthroughs.

## Quick Start

```bash
# Run any demo
./scenarios/monday-panic/demo.sh      # 3 min: Find the problem across 47 clusters
./scenarios/2am-kubectl/demo.sh       # 4 min: Catch and fix drift
./scenarios/security-patch/demo.sh    # 4 min: Patch 847 services in one command
```

## The 3 Demos

| Demo | Duration | Pain Point | "Aha" Moment |
|------|----------|------------|--------------|
| **Monday Panic** | 3 min | 47 clusters, where's the problem? | "Found it in 30 seconds, not 45 minutes" |
| **2AM kubectl** | 4 min | Someone edited prod directly | "Full audit trail + bulk remediate" |
| **Security Patch** | 4 min | CVE affects 847 services | "One command, not 847 PRs" |

## Visceral Reactions We Want

| Demo | Audience Says |
|------|--------------|
| **Monday Panic** | "I literally did this last week. It took me 2 hours." |
| **2AM kubectl** | "Wait, ArgoCD says Synced but it's actually drifted? That's terrifying." |
| **Security Patch** | "847 PRs would take us a month. This is... 15 minutes?" |

## Directory Structure

```
rm-demos-argocd/
├── README.md                     # This file
│
├── scenarios/                    # Runnable demo scripts
│   ├── monday-panic/
│   │   └── demo.sh              # "Where's the problem?"
│   ├── 2am-kubectl/
│   │   └── demo.sh              # "Who changed what?"
│   └── security-patch/
│       └── demo.sh              # "Patch everything NOW"
│
├── repo-patterns/               # "Your structure doesn't matter"
│   ├── monorepo/                # Single repo, folders per app
│   ├── multi-repo/              # Repo per team
│   ├── helm-umbrella/           # Umbrella charts
│   └── applicationsets/         # ArgoCD ApplicationSets
│
└── confighub/                   # ConfigHub configuration
    ├── hub.yaml                 # Platform governance
    └── spaces/
        ├── payments-team.yaml
        ├── orders-team.yaml
        └── platform-team.yaml
```

## Your Repo Structure? Doesn't Matter.

These demos work regardless of how you've organized things. See `repo-patterns/` for examples:

| Your Setup | ConfigHub Sees |
|------------|----------------|
| Monorepo with 50 folders | 50 Units in one App Space |
| Multi-repo, one per team | Multiple App Spaces |
| Helm umbrella charts | Units with Helm values |
| ApplicationSets | Generator Units → Instance Units |
| **Mixed (all of the above)** | **All visible in one hierarchy** |

## Why ArgoCD Users Should Care

ConfigHub doesn't replace ArgoCD — it **completes** it.

| ArgoCD Does | ConfigHub Adds |
|-------------|----------------|
| Sync from source | Fleet-wide visibility |
| Deploy to cluster | Cross-cluster queries |
| Show app health | Drift detection (beyond sync) |
| Per-cluster UI | Unified control plane |
| Manual promotion | One-command rollouts |
| Git as source | Git + OCI + WET config |

**The pitch:** "Keep ArgoCD. Add ConfigHub. See everything. Control everything."

## Single-Cluster-First Verification

**Key principle:** If the repo layout works with one cluster, it should work with N clusters.

Before running fleet demos, verify with single cluster:

```bash
./cub-scout map                           # See what's running
./cub-scout map -q "owner=ArgoCD"         # Verify ownership detection
./cub-scout import -n <namespace>         # Import to ConfigHub
cub unit list --space <space>             # Verify hierarchy
```

See [REPO-SKELETON-TAXONOMY.md](../../docs/planning/REPO-SKELETON-TAXONOMY.md) for the full verification checklist.

## See Also

- [RM-DEMOS-ARGOCD.md](../../docs/planning/RM-DEMOS-ARGOCD.md) — Full demo documentation
- [REPO-SKELETON-TAXONOMY.md](../../docs/planning/REPO-SKELETON-TAXONOMY.md) — Skeleton classification & single-cluster-first principle
- [IMPORT-GIT-REFERENCE-ARCHITECTURES.md](../../docs/IMPORT-GIT-REFERENCE-ARCHITECTURES.md) — Pattern → ConfigHub mapping
- [RENDERED-MANIFEST-PATTERN.md](../../docs/planning/RENDERED-MANIFEST-PATTERN.md) — Pattern overview
- [kostis-argocd-best-practices.md](../../docs/planning/reference/kostis-argocd-best-practices.md) — ArgoCD best practices (Kostis Kapelonis/Codefresh)
