# ConfigHub Integration: The DRY → WET → Live Journey

ConfigHub transforms how you manage the app hierarchy from source code to running applications.

## The App Hierarchy Problem

Without ConfigHub, you have visibility gaps:

```
REPOS (DRY)           DEPLOYERS              LIVE
┌───────────┐         ┌───────────┐         ┌───────────┐
│ Git repos │   ???   │ Flux/Argo │   ???   │ Clusters  │
│ Helm      │ ──────▶ │           │ ──────▶ │           │
│ Kustomize │         │           │         │           │
└───────────┘         └───────────┘         └───────────┘

Questions you can't answer:
- "Which version of payment-api is running in prod-east?"
- "What changed between v2.0.9 and v2.1.0?"
- "Which clusters got the security patch?"
- "Are any clusters running stale config?"
```

## The ConfigHub Solution

ConfigHub becomes the **WET store** — the source of truth for rendered configuration:

```
REPOS (DRY)       CONFIGHUB (WET)       OCI           DEPLOYERS        LIVE
┌───────────┐     ┌─────────────┐     ┌─────┐     ┌───────────┐     ┌─────────┐
│ Git repos │     │ Units       │     │     │     │ Flux/Argo │     │ Clusters│
│ Helm      │ ──▶ │ (store)     │ ──▶ │ OCI │ ──▶ │           │ ──▶ │         │
│ Kustomize │     │ Revisions   │     │     │     │           │     │         │
└───────────┘     └─────────────┘     └─────┘     └───────────┘     └─────────┘
                        │
                        ▼
                  VISIBILITY:
                  - What's deployed where
                  - Version comparison
                  - Change history
                  - Fleet-wide queries
```

## The Integration Journey

### Stage 1: OSS Map (Local Cluster)

You start with just the TUI:

```bash
cub-scout map                    # See ownership
cub-scout map orphans            # Find shadow IT
cub-scout map trace deploy/x     # Trace to source
```

**What you see:** Live resources + ownership + GitOps deployers

### Stage 2: Connect to ConfigHub

Sign up and connect:

```bash
cub auth login                   # Authenticate
cub-scout map --hub              # ConfigHub hierarchy TUI
```

**What you see:**
- ConfigHub hierarchy (Org → Space → Unit)
- Multi-cluster view
- DRY → WET → Live visibility

### Stage 3: Import Existing Workloads

Bring your Flux/ArgoCD workloads into ConfigHub:

```bash
cub-scout import                 # Launch import wizard
```

**What happens:**
1. Select workloads to import
2. ConfigHub creates Units
3. OCI transport set up
4. Flux/ArgoCD pulls from OCI
5. You have complete visibility

### Stage 4: Platform Team Operations

With ConfigHub managing your fleet:

```bash
# Fleet-wide queries
cub unit list --where "app=payment-api"

# See what version is where
cub unit get payment-api --all-spaces

# Make changes with confidence
cub unit apply payment-api
```

## The Hub/AppSpace Model

ConfigHub enables platform + app team collaboration:

```
┌─────────────────────────────────────────────────────────────────────┐
│                            ORGANIZATION                              │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌──────────────────────────────────────────────────────────┐       │
│  │                          HUB                             │       │
│  │  Platform team's base configs                            │       │
│  │  (ingress, cert-manager, monitoring, policies)           │       │
│  └──────────────────────────────────────────────────────────┘       │
│                              │                                      │
│              ┌───────────────┼───────────────┐                      │
│              ▼               ▼               ▼                      │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐            │
│  │  APP SPACE   │   │  APP SPACE   │   │  APP SPACE   │            │
│  │  Team A      │   │  Team B      │   │  Team C      │            │
│  │  (frontend)  │   │  (backend)   │   │  (data)      │            │
│  │              │   │              │   │              │            │
│  │  Overrides:  │   │  Overrides:  │   │  Overrides:  │            │
│  │  - replicas  │   │  - env vars  │   │  - storage   │            │
│  │  - resources │   │  - secrets   │   │  - backups   │            │
│  └──────────────┘   └──────────────┘   └──────────────┘            │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

**Why this matters:**
- Platform team updates don't clobber app team overrides
- App teams get autonomy within guardrails
- Changes are tracked, audited, reversible

## Visibility at Each Level

### With map list (Live)

```bash
cub-scout map list
```
```
NAME            NAMESPACE    OWNER        STATUS
payment-api     prod         ConfigHub    ✓ Synced
frontend        prod         ConfigHub    ✓ Synced
monitoring      system       ConfigHub    ✓ Synced
```

### With map --hub (WET → Live)

```bash
cub-scout map --hub
```
```
Organization: mycompany
└── Space: prod
    ├── Unit: payment-api
    │   └── Revision: 42 (current)
    │       └── Target: prod-east → ✓ deployed
    │       └── Target: prod-west → ✓ deployed
    ├── Unit: frontend
    │   └── Revision: 18 (current)
    │       └── Target: prod-east → ✓ deployed
    └── Unit: monitoring
        └── Revision: 7 (current)
            └── Target: prod-east → ✓ deployed
            └── Target: prod-west → ✓ deployed
```

### With cub unit (DRY → WET)

```bash
cub unit get payment-api
```
```
Unit: payment-api
Source: git@github.com:org/apps.git (path: services/payment)
Current Revision: 42
Last Change: 2h ago by user@example.com
Deployed To: prod-east (rev 42), prod-west (rev 42)
```

## Fleet-Wide Queries

Once connected, you can query across your entire fleet:

```bash
# Which clusters run payment-api?
cub unit list --where "name=payment-api"

# What version is running where?
cub unit list --where "app=payment" --show-revision

# Which clusters are behind?
cub unit list --where "revision.current < revision.latest"

# Find all units by team
cub unit list --where "labels.team=platform"
```

## The RM (Rendered Manifest) Pattern

ConfigHub enables the Rendered Manifest pattern:

**Traditional:** Git → Flux/Argo → Cluster (Git stores WET)
**RM Pattern:** Git → ConfigHub → OCI → Flux/Argo → Cluster (OCI stores WET)

### Why RM Pattern?

| Aspect | Git-stored WET | OCI-stored WET (RM) |
|--------|----------------|---------------------|
| Merge conflicts | Frequent (rendered output) | None |
| Immutability | Branches can change | Tags are immutable |
| Distribution | Clone entire repo | Pull only what's needed |
| Audit | Git history | OCI + ConfigHub history |
| Rollback | Git revert | OCI tag switch |

### What Map Shows with RM Pattern

```bash
cub-scout trace deploy/payment-api -n prod
```
```
TRACE: Deployment/payment-api in prod

  ✓ ConfigHub OCI/prod/us-west
    │ Space: prod
    │ Target: us-west
    │ Registry: oci.api.confighub.com
    │ Revision: v42@sha1:abc123
    │
    └─▶ ✓ Kustomization/payment-api
          │ Path: .
          │
          └─▶ ✓ Deployment/payment-api
                Status: Applied
                Replicas: 3/3 ready
```

**What this shows:**
- ConfigHub OCI registry is the source (`oci.api.confighub.com`)
- Space and target structure (`prod/us-west`)
- Revision tracking (`v42`)
- Full chain from OCI → Kustomization → Deployment

## Business Impact

| Metric | Before ConfigHub | After ConfigHub |
|--------|------------------|-----------------|
| "What version is in prod?" | Manual inspection | Instant query |
| Multi-cluster visibility | Check each cluster | Single view |
| Rollback time | Minutes (git revert + sync) | Seconds (revision switch) |
| Change audit | Git history | Structured audit log |
| Platform/app separation | Manual coordination | Built-in model |

## Getting Started

```bash
# 1. Sign up at app.confighub.com

# 2. Install cub CLI
curl -sL https://get.confighub.com | bash

# 3. Authenticate
cub auth login

# 4. Connect map
cub-scout map --hub

# 5. Import existing workloads
cub-scout import
```

## Summary

ConfigHub integration provides:
- **DRY → WET → Live visibility** across your entire fleet
- **Hub/AppSpace model** for platform + app team collaboration
- **RM Pattern support** for better config transport
- **Fleet-wide queries** to answer "what's running where"
- **Complete audit trail** from source to deployment
