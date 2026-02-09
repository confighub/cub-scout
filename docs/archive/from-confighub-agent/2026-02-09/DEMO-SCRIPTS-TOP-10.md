# ConfigHub Agent — Top 10 Feature Demos

**Target Audience:** DevOps practitioners, Platform engineers, GitOps teams
**Format:** Technical deep-dives (2-5 min each) or combined 15-min overview

---

## Feature 1: Ownership Detection — "Who Owns This?"

**Problem:** Multi-tool chaos. Flux + Argo + Helm + kubectl in same cluster. Who manages what?

**Demo Script (2 min):**

```bash
# Show the cluster with mixed ownership
./test/atk/map workloads
```

**What to show:**
```
STATUS  NAMESPACE        NAME              OWNER      MANAGED-BY            IMAGE
─────────────────────────────────────────────────────────────────────────────────
     demo-payments    frontend          ArgoCD     frontend-app          frontend:3.1.0
     demo-orders      order-processor   ConfigHub  order-processor-prod  processor:1.8.0
     atk-flux-basic   podinfo           Flux       podinfo               podinfo:6.5.0
     demo-orders      postgresql        Helm       orders-db             postgres:15
     argocd           argocd-server     Native     -                     argocd:v3.2.3
```

**Talking Points:**
- One command shows ALL ownership across ALL tools
- No more "who deployed this?" mystery
- Detects 7 owner types: Flux, Argo CD, Helm, ConfigHub, Terraform, Native K8s, Unknown
- Read-only — safe to run anywhere

**Call to action:** "Try it on your cluster: `./test/atk/map workloads`"

---

## Feature 2: Risk Scanning — "Find Config Bugs Before They Break Production"

**Problem:** The BIGBANK 4-hour outage was caused by a whitespace bug in a ConfigMap.

**Demo Script (3 min):**

```bash
# Deploy vulnerable config (the BIGBANK bug)
./test/atk/demo ccve

# Scan finds it instantly
./test/atk/scan
```

**What to show:**
```
CONFIG CVE SCAN: kind-atk

CRITICAL (1)
────────────────────────────────────────────────────────────────────
[RISK-2025-0027] demo-monitoring/grafana
  Sidecar NAMESPACE contains whitespace
  Fix: kubectl set env deployment/grafana -n monitoring NAMESPACE="monitoring,grafana"

Summary: 1 critical, 0 warning, 3 info
```

**Talking Points:**
- 1,700+ patterns in the Risk database
- Catches real bugs that caused real outages
- Shows exact fix commands
- Categories: SOURCE, RENDER, APPLY, DRIFT, DEPEND, STATE, ORPHAN, CONFIG

**Call to action:** "Run `./test/atk/scan` on your cluster right now"

---

## Feature 3: Trace — "Follow the Chain Back to Source"

**Problem:** "This deployment is broken. Where did this config come from?"

**Demo Script (2 min):**

```bash
# Trace a Flux-managed deployment
cub-agent trace deployment/podinfo -n atk-flux-basic
```

**What to show:**
```
TRACE: deployment/podinfo

  ┌─────────────────────────────────────────────────────────────────┐
  │  deployment/podinfo                                              │
  │  namespace: atk-flux-basic                                       │
  │  owner: Flux                                                     │
  └─────────────────────────────────────────────────────────────────┘
           │
           ▼
  ┌─────────────────────────────────────────────────────────────────┐
  │  Kustomization/podinfo                                           │
  │  namespace: flux-system                                          │
  │  revision: sha1:2c3d4e5f                                         │
  └─────────────────────────────────────────────────────────────────┘
           │
           ▼
  ┌─────────────────────────────────────────────────────────────────┐
  │  GitRepository/podinfo                                           │
  │  url: https://github.com/stefanprodan/podinfo                    │
  │  branch: master                                                  │
  └─────────────────────────────────────────────────────────────────┘
```

**Talking Points:**
- Full provenance chain: Resource → Deployer → Source
- Works for Flux AND Argo CD
- Shows exact revision/commit
- "Where did this config come from?" answered in seconds

---

## Feature 4: Import Wizard — "5 Minutes from Kubectl to GitOps"

**Problem:** Migrating workloads to ConfigHub is tedious. What namespace? What structure?

**Demo Script (3 min):**

```bash
# Launch the interactive wizard
cub-agent import --wizard
```

**What to show:**
- Step 1: Select namespaces (checkboxes, multi-select)
- Step 2: Review discovered workloads with owner detection
- Step 3: Organize into Hub/App Space structure (drag/drop treeview)
- Step 4: Apply to ConfigHub
- Step 5: ArgoCD cleanup (if applicable)

**Talking Points:**
- 5-step guided flow
- Auto-detects existing ownership (Flux/Argo/Helm/Native)
- Interactive treeview with edit mode
- E2E verification built-in (press `t` to test)

---

## Feature 5: Map Overview — "Cluster Health at a Glance"

**Problem:** "Is everything healthy? What's broken? How many resources?"

**Demo Script (2 min):**

```bash
./test/atk/map
```

**What to show:**
```
 5 FAILURE(S)   kind-atk

  Deployers  1/4
  Workloads  14/17

  PROBLEMS
  ────────────────────────────────────────────────
  HelmRelease/redis-cache  SourceNotReady
  Application/frontend-app  null
  Kustomization/monitoring-stack  suspended
  demo-orders/order-processor  0/2 pods
  demo-payments/payment-api  0/3 pods

  OWNERSHIP
  ────────────────────────────────────────────────
  Flux(1) Argo(1) ConfigHub(2) Helm(1) Native(12)
```

**Talking Points:**
- One glance: health, problems, ownership distribution
- Health bars show percentage healthy
- Problems listed with specific errors
- Works standalone (no ConfigHub needed)

---

## Feature 6: Query Language — "Find the Needle in the Haystack"

**Problem:** "What's managed by GitOps? Find all orphans. Show me production only."

**Demo Script (2 min):**

```bash
# Find orphaned resources (not managed by GitOps)
cub-agent map list -q "owner=Native"

# Show only GitOps-managed
cub-agent map list -q "owner!=Native"

# Production namespaces only
cub-agent map list -q "namespace=prod*"

# Cross-cutting label query
cub-agent map list -q "labels[app]=payment"
```

**What to show:**
```
# Orphan hunt result
NAMESPACE     KIND        NAME              OWNER
staging       Deployment  debug-pod         Native
monitoring    ConfigMap   leftover-config   Native

Total: 2 resources
By Owner: Native(2)
```

**Talking Points:**
- Powerful filtering: exact match, regex, wildcards, IN lists
- Logical operators: AND, OR
- Query by labels across all resources
- "Who kubectl'd this at 2am?" — now you can find out

---

## Feature 7: Fleet View — "All Apps, All Clusters, One View"

**Problem:** "Which clusters are behind on this app version?"

**Demo Script (2 min):**

```bash
# Connect to ConfigHub
cub context set space platform-prod

# View fleet status
./test/atk/map   # or cub-agent map fleet
```

**What to show:**
```
ConfigHub Fleet View
Hierarchy: Application -> Variant -> Cluster

  order-processor
  |-- variant: prod
  |   |-- cluster-east @ rev 89
  |   |-- cluster-west @ rev 89
  |   |-- cluster-eu @ rev 87    <- behind!
  |-- variant: staging
      |-- cluster-staging @ rev 92

  payment-api
  |-- variant: prod
      |-- cluster-east @ rev 127
      |-- cluster-west @ rev 127
```

**Talking Points:**
- See which clusters are behind instantly
- Hierarchy: Application → Variant → Target
- Powered by ConfigHub Hub/App Space model
- Answer "Is this rollout complete?" in seconds

---

## Feature 8: Demo Scenarios — "See Real Problems, Real Solutions"

**Problem:** "I want to see how this helps with real scenarios."

**Demo Script (3 min):**

```bash
# The BIGBANK incident story (30 seconds vs 4 hours)
./test/atk/demo scenario bigbank-incident

# Find orphaned resources
./test/atk/demo scenario orphan-hunt

# Monday morning health check
./test/atk/demo scenario monday-morning

# Platform vs app config protection
./test/atk/demo scenario clobber
```

**What to show:**
- Each scenario deploys fixtures, runs demo, shows solution
- Real enterprise patterns from IITS research
- Self-cleaning (use `--cleanup`)

**Talking Points:**
- 6 pre-built scenarios
- Based on real enterprise problems
- Perfect for workshops and training
- "Try before you buy" experience

---

## Feature 9: GSF Output — "Pipe to Anything"

**Problem:** "I need this data in my monitoring/ticketing/automation system."

**Demo Script (2 min):**

```bash
# JSON output
cub-agent map list --json | jq '.resources | length'

# Snapshot to file
cub-agent snapshot --output gsf.json

# Parse and transform
cat gsf.json | jq '.resources[] | select(.owner == "Native")'
```

**What to show:**
```json
{
  "resources": [
    {
      "kind": "Deployment",
      "namespace": "demo-payments",
      "name": "frontend",
      "owner": "ArgoCD",
      "managedBy": "frontend-app",
      "labels": {"app": "frontend"}
    }
  ],
  "summary": {
    "total": 17,
    "healthy": 14,
    "byOwner": {"Flux": 1, "ArgoCD": 1, "Native": 12}
  }
}
```

**Talking Points:**
- GitOps State Format (GSF) — standard JSON schema
- Pipe to jq, monitoring, ticketing, automation
- Snapshot for point-in-time analysis
- Build integrations without writing code

---

## Feature 10: Kyverno Policy Scan (KPOL) — "460 Best Practice Policies"

**Problem:** "Beyond risks, are we following Kubernetes best practices?"

**Demo Script (2 min):**

```bash
# Run Kyverno policy scan
cub-agent scan

# Show policy categories
cub-agent scan --list-policies | head -20
```

**What to show:**
```
KYVERNO POLICY SCAN: kind-atk

WARN (5)
────────────────────────────────────────────────────────────────────
[KPOL-001] demo-payments/frontend: Missing resource limits
[KPOL-002] demo-orders/processor: No readiness probe
[KPOL-003] monitoring/grafana: Running as root
[KPOL-004] demo-payments/api: No network policy
[KPOL-005] argocd/server: Privileged container

Categories: security(12), reliability(8), efficiency(5)
```

**Talking Points:**
- 460 Kyverno-based policies
- Categories: security, reliability, efficiency, compliance
- Complements risks (risks = bugs, KPOL = best practices)
- No Kyverno installation required — runs standalone

---

## Combined Demo Flow (15 min)

For a complete walkthrough:

1. **Context** (1 min): Show messy cluster with mixed tools
2. **Map** (2 min): One-glance health view
3. **Ownership** (2 min): "Who owns what?"
4. **Scan** (3 min): Find risks, show BIGBANK bug
5. **Trace** (2 min): Follow chain to source
6. **Query** (2 min): Find orphans, filter GitOps-only
7. **Import** (2 min): Quick wizard walkthrough
8. **Wrap-up** (1 min): Try it yourself, links

---

## Quick Commands Reference

| Feature | Command | Time |
|---------|---------|------|
| Map overview | `./test/atk/map` | 2s |
| Workloads + owners | `./test/atk/map workloads` | 2s |
| Risk scan | `./test/atk/scan` | 5s |
| Trace resource | `cub-agent trace deploy/X -n Y` | 2s |
| Query orphans | `cub-agent map list -q "owner=Native"` | 2s |
| Import wizard | `cub-agent import --wizard` | Interactive |
| Demo scenario | `./test/atk/demo scenario X` | 30s |
| Kyverno scan | `cub-agent scan` | 5s |

---

## Recording Tips

1. **Terminal setup**: Use a clean terminal with large font (14-16pt)
2. **Prompt**: Simple prompt like `$` or include cluster name
3. **Pre-stage**: Have fixtures deployed before recording
4. **Pause**: Pause 1-2s after each command for viewers to read
5. **Narrate**: Talk through what you're seeing, not just typing
6. **Cleanup**: Always show cleanup at the end

---

## Files for Claude Desktop

Share this file with Claude Desktop along with:
- `docs/EXAMPLES-TUI-MAP-FLEET-IITS-STUDIES.md` — Real output examples
- `docs/HOW-IT-WORKS.md` — Technical architecture
- `docs/HOW-TO-TEST.md` — Testing guide

Claude Desktop can use these to create:
- Slide decks for each feature
- Interactive React demos
- Storyboards for video production
- Detailed shot lists with timing
