# ConfigHub Hierarchy — Apptique Examples

This directory shows how different GitOps repo skeletons map to ConfigHub's **Hub → App Space → Unit** hierarchy.

---

## The Core Mapping

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           REPO SKELETON                                  │
│  (how you organize your GitOps repo)                                    │
└─────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         CONFIGHUB HIERARCHY                              │
│                                                                          │
│   Hub: apptique-platform                                                │
│   ├── App Space: apptique                                               │
│   │   ├── Unit: frontend (variants: dev, prod)                          │
│   │   ├── Unit: cart-service (variants: dev, prod)                      │
│   │   └── Unit: checkout-service (variants: dev, prod)                  │
│   └── App Space: apptique-infra                                         │
│       ├── Unit: redis                                                   │
│       └── Unit: postgres                                                │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Skeleton → Hierarchy Mapping

### Pattern A1: Flux Monorepo

```
flux-monorepo/                    │  Hub: apptique-platform
├── apps/apptique/                │  │
│   ├── base/                     │  │   App Space: apptique
│   │   ├── deployment.yaml       │  │   │
│   │   └── service.yaml          │  │   │
│   └── overlays/                 │  │   │
│       ├── dev/                  │  │   ├── Unit: frontend
│       │   └── kustomization.yaml│  │   │   ├── variant: dev
│       └── prod/                 │  │   │   └── variant: prod
│           └── kustomization.yaml│  │   │
└── clusters/                     │  │   └── (more units...)
    ├── dev/                      │  │
    └── prod/                     │  └── (more spaces...)
```

**Key insight:** Each `overlays/{env}/` becomes a **variant** of the Unit.

### Pattern B1: Argo ApplicationSet

```
argo-applicationset/              │  Hub: apptique-platform
├── bootstrap/                    │  │
│   └── applicationset.yaml       │  │   Generator Unit: apptique-appset
└── apps/apptique/                │  │   │
    ├── dev/                      │  │   ├── instance: apptique-dev
    │   └── deployment.yaml       │  │   │   └── target: dev-cluster
    └── prod/                     │  │   └── instance: apptique-prod
        └── deployment.yaml       │  │       └── target: prod-cluster
```

**Key insight:** The ApplicationSet is a **generator Unit**. Each generated Application is an **instance Unit**.

### Pattern B4: Argo App-of-Apps

```
argo-app-of-apps/                 │  Hub: apptique-platform
├── root/                         │  │
│   └── root-app.yaml             │  │   [ignored - manages App CRs only]
├── apps/                         │  │
│   ├── apptique-dev.yaml         │  │   App Space: apptique
│   └── apptique-prod.yaml        │  │   │
└── manifests/apptique/           │  │   ├── Unit: frontend
    ├── dev/                      │  │   │   ├── variant: dev (via child app)
    │   └── deployment.yaml       │  │   │   └── variant: prod (via child app)
    └── prod/                     │  │   │
        └── deployment.yaml       │  │   └── (more units...)
```

**Key insight:** The root Application is **not imported** (it only manages Application CRs). Child Applications → Units.

---

## Visual TUI Demo

Run this to see the hierarchy in action:

```bash
# See the current hierarchy (requires ConfigHub auth)
./test/atk/map confighub

# Or use the demo script
./examples/apptique-examples/confighub/demo-hierarchy.sh
```

Expected output:

```
⚡ CONFIGHUB HIERARCHY

Hub: apptique-platform
├── App Space: apptique
│   ├── Unit: frontend
│   │   ├── dev     ✓ synced @ rev 127
│   │   └── prod    ✓ synced @ rev 127
│   ├── Unit: cart-service
│   │   ├── dev     ✓ synced @ rev 125
│   │   └── prod    ✓ synced @ rev 125
│   └── Unit: checkout-service
│       ├── dev     ⚠ drift detected
│       └── prod    ✓ synced @ rev 124
└── App Space: apptique-infra
    ├── Unit: redis
    │   └── prod    ✓ synced @ rev 89
    └── Unit: postgres
        └── prod    ✓ synced @ rev 91
```

---

## Files in This Directory

| File | Purpose |
|------|---------|
| `hub.yaml` | Hub definition (platform governance) |
| `spaces/apptique.yaml` | App Space for the frontend app |
| `spaces/apptique-infra.yaml` | App Space for infrastructure |
| `demo-hierarchy.sh` | TUI demo showing the mapping |

---

---

## IITS Enterprise Patterns

The apptique examples map directly to the **IITS Hub-and-Spoke** enterprise pattern. See [iits-patterns.yaml](iits-patterns.yaml) for the full mapping.

### The IITS Problems (and ConfigHub Solutions)

| Problem | Pain | Solution |
|---------|------|----------|
| **"What you see isn't what deploys"** | Mental compilation of overlays | WET manifests in Units |
| **Umbrella chart divergence** | Teams fork because defaults don't fit | Clone from Hub + customize |
| **Per-cluster values sprawl** | 50 clusters × N apps = explosion | Labels replace folders |
| **Silent patch breakage** | Overlays fail silently | Structural validation at import |
| **Multi-tool chaos** | Flux + Argo + Helm + kubectl | Single map view |
| **Can't query fleet** | "What version across 50 clusters?" | `cub query "app=X"` |

### Try the IITS Queries

```bash
# All prod instances
./cub-scout map list -q "namespace=apptique-*"

# Find orphans (kubectl'd resources)
./cub-scout map list -q "owner=Native"

# GitOps-managed only
./cub-scout map list -q "owner!=Native"

# Flux OR Argo
./cub-scout map list -q "owner=Flux OR owner=ArgoCD"
```

### IITS Deep Dive

| Resource | Description |
|----------|-------------|
| [TUI-MAP-FLEET-IITS-STUDIES.md](../../../docs/EXAMPLES-TUI-MAP-FLEET-IITS-STUDIES.md) | How TUI solves IITS problems |
| [08-CASE-STUDIES-IITS.md](../../../docs/planning/map/08-CASE-STUDIES-IITS.md) | 10 enterprise problems → solutions |
| [iits-patterns.yaml](iits-patterns.yaml) | Apptique → IITS pattern mapping |

---

## See Also

- [REPO-SKELETON-TAXONOMY.md](../../../docs/planning/REPO-SKELETON-TAXONOMY.md) — Full skeleton classification
- [RENDERED-MANIFEST-PATTERN.md](../../../docs/planning/RENDERED-MANIFEST-PATTERN.md) — How ConfigHub uses this
- [IMPORT-GIT-REFERENCE-ARCHITECTURES.md](../../../docs/IMPORT-GIT-REFERENCE-ARCHITECTURES.md) — Pattern → ConfigHub mapping
- [EXAMPLES-TUI-MAP-FLEET-IITS-STUDIES.md](../../../docs/EXAMPLES-TUI-MAP-FLEET-IITS-STUDIES.md) — IITS case studies with TUI
