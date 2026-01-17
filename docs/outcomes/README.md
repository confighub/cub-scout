# Business Outcomes: Why Map Matters

Map transforms how platform teams understand and operate Kubernetes.

## The 30-Second vs 45-Minute Problem

| Question | Before Map | With Map |
|----------|------------|----------|
| "What's running in prod?" | kubectl + grep + spreadsheets | `map list` |
| "Who owns this deployment?" | Check labels, ask team | Automatic detection |
| "Find shadow IT" | No way to know | `map orphans` |
| "Which cluster is behind?" | Check 47 ArgoCD UIs | `map list -q "status=BEHIND"` |
| "Trace config to source" | Manual detective work | `map trace` |

**Time saved per task:** 30-45 minutes → 30 seconds

---

## The App Hierarchy: From Code to Running Apps

Map shows the complete picture of how apps flow from source to production:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        THE APP HIERARCHY                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. REPOS (DRY + Code)                                                      │
│     │   Your source of truth: Git repos with Helm charts, Kustomize        │
│     │   bases, app code, and configuration templates.                       │
│     │                                                                       │
│     │   Examples:                                                           │
│     │   ├── git@github.com:org/platform-config.git                          │
│     │   ├── git@github.com:org/app-manifests.git                            │
│     │   └── oci://registry.example.com/charts                               │
│     │                                                                       │
│     ▼                                                                       │
│  2. DRY TEMPLATES (GitOps Deployers)                                        │
│     │   GitOps controllers that read repos and manage deployment.           │
│     │                                                                       │
│     │   Patterns:                                                           │
│     │   ├── App of Apps (ArgoCD parent→children)                            │
│     │   ├── ApplicationSets (ArgoCD generator→apps)                         │
│     │   ├── Kustomizations (Flux base+overlays)                             │
│     │   └── HelmReleases (Flux chart+values)                                │
│     │                                                                       │
│     ▼                                                                       │
│  3. WET CONFIG (Rendered Data)                                              │
│     │   The actual manifests after templates are rendered.                  │
│     │   This is what gets deployed.                                         │
│     │                                                                       │
│     │   Storage options:                                                    │
│     │   ├── ConfigHub Units (source of truth, central store)                │
│     │   ├── OCI Artifacts (immutable packages, transport)                   │
│     │   └── Git branches (traditional rendered-to-git)                      │
│     │                                                                       │
│     ▼                                                                       │
│  4. LIVE APPS & RESOURCES                                                   │
│         The actual running state in your Kubernetes clusters.               │
│                                                                             │
│         What you see:                                                       │
│         ├── Namespaces                                                      │
│         ├── Deployments, Services, ConfigMaps                               │
│         ├── Custom Resources (CRDs)                                         │
│         └── Pods (actual containers running)                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### What Map Shows at Each Level

| Level | What Map Shows | Command |
|-------|----------------|---------|
| **1. Repos** | Git URLs, OCI registries | `map trace` → shows source |
| **2. DRY Templates** | Kustomizations, HelmReleases, Applications | `map deployers` |
| **3. WET Config** | ConfigHub Units, revisions | `map --hub` |
| **4. Live Resources** | Deployments, Services, actual state | `map list` |

---

## The Value Ladder: Adoption → Connection → Extension

```
┌────────────────────────────────────────────────────────────────────────┐
│  1. ADOPTION (OSS - Free)                                              │
│     Flux/Argo/Helm user discovers cub-scout                            │
│     → Runs `cub-scout map` on their cluster                            │
│     → Falls in love with TUI (ownership visibility, trace, scan)       │
│     → This works standalone, no account needed                         │
│     → VALUE: "I finally know who owns what"                            │
├────────────────────────────────────────────────────────────────────────┤
│  2. CONNECT (ConfigHub Free Tier)                                      │
│     → Signs up for ConfigHub                                           │
│     → `cub-scout map --hub` shows multi-cluster view                   │
│     → Sees DRY→WET→Live visibility across fleet                        │
│     → VALUE: "I can see my whole fleet in one place"                   │
├────────────────────────────────────────────────────────────────────────┤
│  3. UPGRADE (Paid)                                                     │
│     → "Make a change" operations (import, apply, destroy)              │
│     → AI-powered trace and actions                                     │
│     → Platform team collaboration (Hub/AppSpace model)                 │
│     → Enterprise features (audit, RBAC, compliance)                    │
│     → VALUE: "I can operate at scale"                                  │
└────────────────────────────────────────────────────────────────────────┘
```

---

## Zero-Friction Proof: Any App Works

Map works with ANY existing Flux, ArgoCD, or Helm deployment:

```bash
# Step 1: You have existing GitOps
kubectl get kustomizations,applications -A   # Shows your deployers

# Step 2: Zero-friction map (just run it)
cub-scout map                                 # Instant ownership visibility

# Step 3: Zero-friction import to ConfigHub
cub-scout import                              # Wizard guides through import
```

### Reference App: apptique (Google Online Boutique)

The `apptique` example demonstrates all major patterns:

| Pattern | Example | Directory |
|---------|---------|-----------|
| **Flux Monorepo** | Kustomize overlays per environment | `apptique-examples/flux-monorepo/` |
| **ArgoCD ApplicationSet** | Directory generator | `apptique-examples/argo-applicationset/` |
| **ArgoCD App of Apps** | Parent manages children | `apptique-examples/argo-app-of-apps/` |

```bash
# Try apptique with map
cd examples/apptique-examples/flux-monorepo
kubectl apply -k overlays/dev
cub-scout map list -n boutique-dev
```

### Tested Reference Architectures

| Architecture | Pattern | Proof |
|--------------|---------|-------|
| Monorepo + Kustomize | Single repo, overlays per env | apptique/flux-monorepo |
| Multi-repo + ApplicationSets | Separate repos, generator | IITS examples |
| App of Apps | Parent→children Applications | apptique/argo-app-of-apps |
| Helm Umbrella | Chart of charts | confighub/examples |
| Mixed Flux + ArgoCD | Both tools together | Internal testing |

---

## Detailed Outcomes

- [Ownership Visibility](ownership-visibility.md) - The Native bucket insight
- [ConfigHub Integration](confighub-integration.md) - DRY → WET → Live journey
- [Break Glass Scenarios](break-glass-scenarios.md) - When GitOps isn't fast enough (TODO)

---

## Demo It

```bash
# Build first
go build ./cmd/cub-scout

# 30-second ownership detection
cub-scout map list

# Find orphan resources (Native)
cub-scout map list -q "owner=Native"

# CCVE scanning
cub-scout scan

# Trace ownership chain
cub-scout trace deployment/nginx -n default

# Query across fleet
cub-scout map list -q "owner=Flux OR owner=ArgoCD"
```

---

## Summary

**For Platform Engineers:**
- See what's running across all clusters
- Detect shadow IT (Native resources)
- Trace any resource to its source
- Scan for configuration issues

**For Decision Makers:**
- Zero-friction adoption (no setup, no account)
- Clear upgrade path to ConfigHub
- Proven with standard GitOps patterns
- 30-second vs 45-minute value proposition
