# TUI Map & Fleet Examples — IITS Case Studies

Real-world enterprise GitOps problems and how ConfigHub solves them.

**Status: Working** — All output from actual clusters.

---

## Background: The IITS Enterprise Patterns

These examples are based on real enterprise GitOps patterns documented by [IITS Consulting](https://www.iits-consulting.de):

| Source | Description |
|--------|-------------|
| [usecase-argocd-fleet-iits.pdf](planning/map/usecase-argocd-fleet-iits.pdf) | Argo CD fleet patterns at scale |
| [usecase-fluxcd-fleet-iits.pdf](planning/map/usecase-fluxcd-fleet-iits.pdf) | Flux CD fleet patterns at scale |
| [08-CASE-STUDIES-IITS.md](planning/map/08-CASE-STUDIES-IITS.md) | Mapped problems to ConfigHub solutions |

**Key insight from IITS:** Enterprise teams struggle with visibility across multi-cluster, multi-tool environments. The questions below come directly from their research.

---

## Try It Now

```bash
git clone https://github.com/confighub/cub-scout.git
cd cub-scout
./run.sh
```

Uses your current kubeconfig. Shows your cluster.

---

## The IITS Questions — Answered in Seconds

From [IITS fleet architecture papers](https://www.iits-consulting.de) and Artem Lajko's research with ~25-30 enterprise teams:

| Question | Command | Time |
|----------|---------|------|
| What's deployed across all clusters? | `./test/atk/map` | 2 sec |
| Who owns what? | `./test/atk/map workloads` | 2 sec |
| Which clusters are behind? | `./test/atk/map` (with auth) | 2 sec |
| What's broken? | `./test/atk/map problems` | 2 sec |
| What config bugs exist? | `./test/atk/scan` | 5 sec |
| GitOps-managed only? | `cub-scout map list -q "owner!=Native"` | 2 sec |
| Production namespaces? | `cub-scout map list -q "namespace=prod*"` | 2 sec |
| Orphan hunt? | `cub-scout map list -q "owner=Native"` | 2 sec |

**Without ConfigHub:** SSH into each cluster, check each Argo/Flux dashboard, grep through repos. Hours.

**With ConfigHub:** One command. Seconds.

---

## "What's running across all my clusters?"

> **IITS Pain Point:** "What you see in the Git repository isn't what actually gets deployed... you need to mentally compile all these layers" — [Flux PDF](planning/map/usecase-fluxcd-fleet-iits.pdf)

```bash
$ ./test/atk/map
```

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

  PIPELINES
  ────────────────────────────────────────────────
  stefanprodan/podinfo@6.5.0  ->  podinfo  ->  3 resources
  company/frontend/k8s@HEAD  ->  frontend-app  ->  demo-payments

  OWNERSHIP
  ────────────────────────────────────────────────
  Flux(1) Argo(1) ConfigHub(2) Helm(1) Native(12)
```

One glance: health, problems, ownership distribution.

---

## "Who owns each workload?"

> **IITS Pain Point:** "Multi-tool chaos — Flux + Argo + Helm + kubectl in same cluster" — [Case Studies](planning/map/08-CASE-STUDIES-IITS.md)

```bash
$ ./test/atk/map workloads
```

```
STATUS  NAMESPACE        NAME              OWNER      MANAGED-BY            IMAGE
────────────────────────────────────────────────────────────────────────────────────
     demo-payments    frontend          ArgoCD     frontend-app          frontend:3.1.0
     demo-orders      order-processor   ConfigHub  order-processor-prod  processor:1.8.0
     demo-payments    payment-api       ConfigHub  payment-api-prod      api:2.4.1
     atk-flux-basic   podinfo           Flux       podinfo               podinfo:6.5.0
     demo-orders      postgresql        Helm       orders-db             postgres:15
     argocd           argocd-server     Native     -                     argocd:v3.2.3
     demo-monitoring  grafana           Native     -                     grafana:10.2.0
```

**The question answered:** "Who do I page when this breaks?"

---

## "Which clusters are behind on this app?"

> **IITS Pain Point:** "Per-cluster sprawl — 50 clusters x N apps = explosion of config files" — [Argo PDF](planning/map/usecase-argocd-fleet-iits.pdf)

```bash
$ ./test/atk/map   # With ConfigHub auth
```

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

**The question answered:** "Is this rollout complete across all clusters?"

---

## "What config bugs exist right now?"

> **IITS Pain Point:** "Silent patch breakage — patches break silently when base resources change" — [Flux PDF](planning/map/usecase-fluxcd-fleet-iits.pdf)

```bash
$ ./test/atk/scan
```

```
CONFIG CVE SCAN: kind-atk

CRITICAL (1)
────────────────────────────────────────────────────────────────────
[CCVE-2025-0027] demo-monitoring/grafana

INFO (3)
────────────────────────────────────────────────────────────────────
[CCVE-FLUX-005] flux-system/monitoring-stack
[CCVE-2025-0019] demo-payments/debug-tools
[CCVE-2025-0019] demo-payments/payment-api

Summary: 1 critical, 0 warning, 3 info
```

**CCVE-2025-0027** is the Grafana namespace whitespace bug that caused [BIGBANK's 4-hour outage](https://www.youtube.com/watch?v=VJiuu-GqfXk).

---

## "What deployers do I have?"

> **IITS Pain Point:** "Umbrella chart divergence — teams fork because they don't like defaults" — [Case Studies](planning/map/08-CASE-STUDIES-IITS.md)

```bash
$ ./test/atk/map deployers
```

```
STATUS  KIND            NAME              NAMESPACE       REVISION   RESOURCES
─────────────────────────────────────────────────────────────────────────────────
      Kustomization   podinfo           atk-flux-basic  sha1:2c    3
      Kustomization   monitoring-stack  flux-system                0
      HelmRelease     redis-cache       flux-system                -
      Application     frontend-app      argocd          HEAD       0
```

Flux Kustomizations, Flux HelmReleases, Argo Applications — all in one view.

---

## "What's broken right now?"

```bash
$ ./test/atk/map problems
```

```
 HelmRelease/redis-cache in flux-system: SourceNotReady
 Application/frontend-app in argocd: null
 Kustomization/monitoring-stack in flux-system: suspended
 Deployment/order-processor in demo-orders: 0/2 ready
 Deployment/frontend in demo-payments: 0/2 ready
 Deployment/payment-api in demo-payments: 0/3 ready
```

No digging through dashboards. Just the problems.

---

## Query Language Reference

The query language solves "needle in haystack" fleet problems identified in the [IITS research](planning/map/08-CASE-STUDIES-IITS.md):

| Before | After |
|--------|-------|
| `kubectl get deploy -A` + grep + manual filtering | `./cub-scout map list -q "owner!=Native"` |
| Check each namespace for orphans | `./cub-scout map list -q "owner=Native"` |
| "What's in prod?" -> SSH to clusters | `./cub-scout map list -q "namespace=prod*"` |
| "Who manages this?" -> check labels manually | `./cub-scout map list -q "labels[app]=nginx"` |

### Query Syntax

| Pattern | Description |
|---------|-------------|
| `field=value` | Exact match (case-insensitive) |
| `field!=value` | Not equal |
| `field~=pattern` | Regex match |
| `field=val1,val2` | IN list (comma-separated) |
| `field=prefix*` | Wildcard |
| `AND` / `OR` | Logical operators |

**Available Fields:** `kind`, `namespace`, `name`, `owner`, `cluster`, `labels[key]`

### Quick Reference

| Use Case | Query |
|----------|-------|
| Orphan hunting | `-q "owner=Native"` — Find kubectl'd resources |
| GitOps audit | `-q "owner!=Native"` — Show only managed |
| Production filter | `-q "namespace=prod*"` — Wildcard matching |
| Multi-tool view | `-q "owner=Flux OR owner=ArgoCD"` — Combine |
| Label search | `-q "labels[app]=nginx"` — Cross-cutting |

---

## Query Examples

### "Show me GitOps-managed deployments only"

```bash
$ ./cub-scout map list -q "kind=Deployment AND owner!=Native"
```

```
NAMESPACE     KIND        NAME              OWNER
prod-east     Deployment  payment-api       Flux
prod-east     Deployment  payment-worker    Flux
prod-west     Deployment  order-api         ArgoCD
prod-west     Deployment  order-processor   ArgoCD
staging       Deployment  frontend          Flux
monitoring    Deployment  prometheus        ConfigHub
monitoring    Deployment  grafana           ConfigHub

Total: 7 resources
By Owner: ArgoCD(2) ConfigHub(2) Flux(3)
```

### "Find all resources in production namespaces"

```bash
$ ./cub-scout map list -q "namespace=prod*"
```

```
NAMESPACE     KIND        NAME              OWNER
prod-east     Deployment  payment-api       Flux
prod-east     Deployment  payment-worker    Flux
prod-east     Service     payment-api       Flux
prod-west     Deployment  order-api         ArgoCD
prod-west     Deployment  order-processor   ArgoCD
prod-west     Service     order-api         ArgoCD

Total: 6 resources
By Owner: ArgoCD(3) Flux(3)
```

### "What's managed by Flux OR Argo?"

```bash
$ ./cub-scout map list -q "owner=Flux OR owner=ArgoCD"
```

```
NAMESPACE     KIND        NAME              OWNER
prod-east     Deployment  payment-api       Flux
prod-east     Deployment  payment-worker    Flux
prod-west     Deployment  order-api         ArgoCD
staging       Deployment  frontend          Flux

Total: 4 resources
By Owner: ArgoCD(1) Flux(3)
```

### "Find orphaned debug resources"

> **IITS Pain Point:** "Can't query fleet — What version of redis across 50 clusters?" — [Case Studies](planning/map/08-CASE-STUDIES-IITS.md)

```bash
$ ./cub-scout map list -q "name~=debug.* AND owner=Native"
```

```
NAMESPACE     KIND        NAME              OWNER
staging       Deployment  debug-pod         Native

Total: 1 resources
By Owner: Native(1)
```

Someone `kubectl apply`'d this at 2am. Now you found it.

---

## Hub/App Space Model

> **IITS Solution:** "Labels replace folders — query instead of navigate" — [Case Studies](planning/map/08-CASE-STUDIES-IITS.md)

Import workloads with `app` and `variant` labels, then view them hierarchically.

### Step 1: Import

```bash
$ ./cub-scout import -n payments-prod --dry-run
```

```
+-------------------------------------------------------------+
| DISCOVERED                                                  |
+-------------------------------------------------------------+
  payments-prod (2 workloads)

+-------------------------------------------------------------+
| WILL CREATE                                                 |
+-------------------------------------------------------------+
  App Space: payments-team

  * payment-api-prod
    labels: app=payment-api, variant=prod
  * payment-worker-prod
    labels: app=payment-worker, variant=prod

  Total: 2 units
```

### Step 2: View with Fleet command

```bash
$ ./cub-scout map fleet --space payments-team
```

```
ConfigHub Fleet View (Hub/App Space Model)
Hierarchy: Application -> Variant -> Target

  payment-api
  |-- variant: dev
  |   |-- payments-team @ rev 92
  |-- variant: prod
      |-- payments-team @ rev 127
      |-- payments-team @ rev 87 <- behind!

  payment-worker
  |-- variant: prod
      |-- payments-team @ rev 45
```

**The question answered:** "Which variants of my app exist and are they in sync?"

### Fleet command options

| Command | What it shows |
|---------|---------------|
| `cub-scout map fleet` | All apps with app/variant labels |
| `cub-scout map fleet --app payment-api` | Just payment-api variants |
| `cub-scout map fleet --space payments-team` | Just that App Space |
| `cub-scout map fleet --json` | JSON output for tooling |

### The model

- One App Space per team (not per environment)
- Units have `app=X, variant=Y` labels
- All variants live together, queryable by label

**Queries enabled:**

| Query | What it finds |
|-------|---------------|
| `--where "Labels.app='payment-api'"` | All variants of payment-api |
| `--where "Labels.variant='prod'"` | All prod deployments |
| `--where "Labels.app='payment-api' AND Labels.variant='prod'"` | Specific variant |

See [02-HUB-APPSPACE-MODEL.md](planning/map/02-HUB-APPSPACE-MODEL.md) for the full design.

---

## Try the Demos

Want to see queries in action? Two options:

```bash
# Option 1: Show query syntax and examples (no cluster needed)
./examples/demos/fleet-queries-demo.sh

# Option 2: Live queries against your cluster (~1 min, applies fixtures)
./test/atk/demo query
```

> **Note:** Demos are **test fixtures** that create Kubernetes resources with GitOps labels
> (Flux, Argo, Helm) to demonstrate ownership detection, but run `nginx:alpine` as a placeholder.
> For converting to real apps, see [examples/README.md#converting-demos-to-real-apps](../examples/README.md#converting-demos-to-real-apps).

This applies fixtures simulating a multi-cluster environment:
- **prod-east** — Flux-managed payment services
- **prod-west** — Argo-managed order services
- **staging** — Mixed ownership (Flux, Helm, Native orphans)
- **monitoring** — ConfigHub-managed Prometheus/Grafana

Then runs live queries showing:
1. GitOps-managed deployments only
2. Production namespace resources
3. Flux OR Argo managed
4. Orphan detection

```bash
# Clean up when done
./test/atk/demo query --cleanup
```

---

## IITS Deep Dive Resources

For comprehensive analysis of the enterprise patterns and ConfigHub solutions:

### Case Studies & Analysis

| Document | Focus |
|----------|-------|
| [08-CASE-STUDIES-IITS.md](planning/map/08-CASE-STUDIES-IITS.md) | 10 enterprise problems mapped to solutions |

### Original Source Documents

| Document | Description |
|----------|-------------|
| [usecase-argocd-fleet-iits.pdf](planning/map/usecase-argocd-fleet-iits.pdf) | Argo CD fleet patterns (IITS original) |
| [usecase-fluxcd-fleet-iits.pdf](planning/map/usecase-fluxcd-fleet-iits.pdf) | Flux CD fleet patterns (IITS original) |

### Problem -> Solution Summary

From [08-CASE-STUDIES-IITS.md](planning/map/08-CASE-STUDIES-IITS.md):

| IITS Problem | ConfigHub Solution |
|--------------|-------------------|
| "What you see isn't what deploys" | WET manifests in Units |
| Umbrella chart divergence | Clone from Hub with customization |
| Per-cluster values sprawl | Labels replace folder hierarchy |
| Silent patch breakage | Structural validation at import |
| Multi-tool chaos | Single view across all deployers |
| Can't query fleet | `cub map --query "..."` |
| Hotfix -> Git hell | `cub drift accept` |
| No ownership boundaries | Hub = platform, App Space = team |

---

## See Also

- [CLI-REFERENCE.md](CLI-REFERENCE.md) — Full command reference
- [TUI-SCAN.md](TUI-SCAN.md) — CCVE scanning documentation
- [TUI-TRACE.md](TUI-TRACE.md) — Resource tracing
- [IMPORTING-WORKLOADS.md](IMPORTING-WORKLOADS.md) — Import into ConfigHub
- [examples/](../examples/) — Interactive demos
