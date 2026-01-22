# Enterprise Case Studies: Real GitOps Problems Solved

Real-world enterprise GitOps challenges documented by [IITS Consulting](https://www.iits-consulting.de) and how Map solves them.

**Source:** Artem Lajko's research with ~25-30 enterprise teams running multi-cluster GitOps at scale.

---

## The 47-Namespace Problem

> **IITS Pain Point:** "What you see in the Git repository isn't what actually gets deployed... you need to mentally compile all these layers."

**The scenario:** Platform team manages 47 namespaces across 12 clusters. Each cluster runs Flux + ArgoCD + some Helm releases. An incident occurs at 2am.

**Before Map:**
```
1. SSH into cluster-prod-east
2. kubectl get deploy -A | wc -l  → 847 deployments
3. Check ArgoCD dashboard → 127 applications, mostly green
4. Check Flux dashboard → 89 kustomizations, some yellow
5. "Which deployment is the problem?"
6. Start grepping...
```
**Time:** 30-45 minutes to get situational awareness

**With Map:**
```bash
$ cub-scout map
```
```
 5 FAILURE(S)   prod-east

  Deployers  23/27
  Workloads  841/847

  PROBLEMS
  ────────────────────────────────────────────────
  ✗ HelmRelease/redis-cache  SourceNotReady
  ✗ Application/frontend     OutOfSync
  ✗ Kustomization/monitoring suspended
  ✗ payments/payment-api     0/3 pods
  ✗ orders/order-processor   0/2 pods

  OWNERSHIP
  ────────────────────────────────────────────────
  Flux(89) ArgoCD(127) ConfigHub(12) Helm(45) Native(574)
```
**Time:** 2 seconds

---

## The Monday Morning Hunt

> **IITS Pain Point:** "Multi-tool chaos — Flux + Argo + Helm + kubectl in same cluster"

**The scenario:** You arrive Monday morning. Over the weekend, three different people made changes:
- DevOps ran a hotfix via kubectl
- Platform team upgraded monitoring via Helm
- App team deployed a new feature via ArgoCD

**The questions:**
1. What changed?
2. Who changed it?
3. Is it still there?

**Before Map:** Check Git history, check each tool's dashboard, hope nobody forgot to commit.

**With Map:**
```bash
# What's NOT in GitOps (the weekend hotfixes)?
$ cub-scout map list -q "owner=Native"
```
```
NAMESPACE     KIND        NAME              CREATED
prod          Deployment  debug-pod         Sat 14:30
prod          ConfigMap   temp-override     Sun 02:15
prod          Secret      api-hotfix        Sun 03:00

Total: 3 orphan resources
By Owner: Native(3)
```

**Found them.** Now decide: adopt to GitOps or delete.

---

## The 50-Cluster Version Query

> **IITS Pain Point:** "Can't query fleet — What version of redis across 50 clusters?"

**The scenario:** Security reports a CVE in Redis 6.2.x. You need to know: which clusters are affected?

**Before Map:**
```bash
# For each of 50 clusters...
for cluster in $(cat clusters.txt); do
  kubectl --context=$cluster get deploy -A -o json | \
    jq '.items[] | select(.spec.template.spec.containers[].image | contains("redis"))' | \
    jq -r '.metadata.namespace + "/" + .metadata.name + " " + .spec.template.spec.containers[].image'
done
# Hope you have credentials to all clusters...
# Hope jq syntax is correct...
# Hope redis isn't named something else...
```
**Time:** 20-30 minutes per cluster = hours for full fleet

**With Map + ConfigHub:**
```bash
$ cub unit list --where "image~=redis" --show-version
```
```
SPACE         UNIT            IMAGE              VERSION   CLUSTERS
prod-east     cache-primary   redis:6.2.8        Affected  3
prod-west     cache-primary   redis:6.2.8        Affected  3
staging       cache-test      redis:7.0.5        Safe      1
dev           cache-local     redis:7.0.5        Safe      2

Affected: 6 clusters
```
**Time:** 2 seconds

---

## Problem → Solution Summary

From [IITS fleet architecture research](https://www.iits-consulting.de):

| Enterprise Problem | Before | With Map/ConfigHub |
|--------------------|--------|-------------------|
| "What you see isn't what deploys" | Mentally compile Git layers | WET manifests visible in Units |
| Umbrella chart divergence | Teams fork, configs drift | Clone from Hub with tracked overrides |
| Per-cluster values sprawl | 50 clusters × N apps = explosion | Labels replace folder hierarchy |
| Silent patch breakage | Discover in production | Structural validation at import |
| Multi-tool chaos | Check each dashboard | Single view across all deployers |
| Can't query fleet | Manual per-cluster scripts | `cub-scout map list -q "..."` |
| Hotfix → Git hell | Drift accumulates | `cub drift accept` reconciles |
| No ownership boundaries | Tribal knowledge | Hub = platform, AppSpace = team |

---

## The Shadow IT Discovery

> **IITS Pain Point:** "Per-cluster sprawl — 50 clusters × N apps = explosion of config files"

**The scenario:** Quarterly security audit asks: "Show me everything running in production that isn't in Git."

**Before Map:** "We... we don't know. We'd have to audit each cluster manually."

**With Map:**
```bash
# Production namespaces, unmanaged resources
$ cub-scout map list -q "namespace=prod* AND owner=Native"
```
```
NAMESPACE     KIND           NAME              CREATED           SOURCE
prod-east     Deployment     debug-utils       2026-01-05        kubectl
prod-east     ConfigMap      override-config   2026-01-08        kubectl
prod-west     Secret         api-key-temp      2025-12-28        kubectl
prod-west     Deployment     test-runner       2026-01-10        kubectl

Total: 4 shadow IT resources
Risk: High (secrets, deployments outside GitOps)
```

**Now you can answer:** "We have 4 unmanaged resources in production. Here they are, here's when they were created, and here's our remediation plan."

---

## The BIGBANK Incident

**Reference:** [BIGBANK 4-hour outage](https://www.youtube.com/watch?v=VJiuu-GqfXk) — Grafana sidecar whitespace bug

**The problem:** A YAML whitespace error in a Grafana sidecar annotation caused a 4-hour production outage. The config looked correct in Git but deployed incorrectly.

**CCVE-2025-0027** now detects this pattern:
```bash
$ cub-scout scan
```
```
CONFIG CVE SCAN: prod-east

CRITICAL (1)
────────────────────────────────────────────────────────────────
[CCVE-2025-0027] monitoring/grafana
  Grafana sidecar whitespace in annotation
  Risk: Dashboard loading failure, 4-hour outage pattern
  Fix: Remove trailing whitespace from grafana.ini annotation

Summary: 1 critical, 0 warning, 0 info
```

**Pattern detection prevents outages before they happen.**

---

## Query Language Reference

The query language solves "needle in haystack" fleet problems:

| Use Case | Query | Time |
|----------|-------|------|
| GitOps-managed only | `-q "owner!=Native"` | 2 sec |
| Orphan hunting | `-q "owner=Native"` | 2 sec |
| Production filter | `-q "namespace=prod*"` | 2 sec |
| Multi-tool view | `-q "owner=Flux OR owner=ArgoCD"` | 2 sec |
| Label cross-cut | `-q "labels[app]=payment"` | 2 sec |
| Specific team | `-q "labels[team]=platform"` | 2 sec |
| Image search | `-q "image~=nginx"` | 2 sec |
| Behind detection | `-q "status=BEHIND"` | 2 sec |

### Query Syntax

| Pattern | Description | Example |
|---------|-------------|---------|
| `field=value` | Exact match | `owner=Flux` |
| `field!=value` | Not equal | `owner!=Native` |
| `field~=pattern` | Regex match | `name~=.*-api` |
| `field=a,b,c` | IN list | `owner=Flux,ArgoCD` |
| `field=prefix*` | Wildcard | `namespace=prod*` |
| `AND` / `OR` | Logical operators | `owner=Flux AND namespace=prod*` |

**Available fields:** `kind`, `namespace`, `name`, `owner`, `cluster`, `status`, `image`, `labels[key]`

---

## Time Savings Summary

| Task | Before Map | With Map |
|------|------------|----------|
| Cluster overview | 30-45 min | 2 sec |
| Find orphans | Unknown (might never) | 2 sec |
| Trace ownership | 10-20 min | 2 sec |
| Fleet version query | Hours | 2 sec |
| Security audit prep | Days | Minutes |
| Incident triage | 30 min | 2 sec |

**ROI:** One incident avoided pays for months of adoption time.

---

## See Also

- [Ownership Visibility](ownership-visibility.md) — The Native bucket insight
- [ConfigHub Integration](confighub-integration.md) — DRY → WET → Live journey
- [IITS ArgoCD Fleet Patterns](https://www.iits-consulting.de) — Original research
- [Query Reference](../map/howto/query-resources.md) — Full query documentation
