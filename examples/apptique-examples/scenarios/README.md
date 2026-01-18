# Apptique RM Scenarios — Real Working Demos

These scenarios demonstrate the **Rendered Manifest (RM) pattern goals** using real Kubernetes resources.

> **Unlike the simulation demos in `rm-demos-argocd/`**, these are **working deployments** that you can apply to your cluster and test with the cub-scout.

---

## The Three RM Goals

| Goal | Pain Point | Demo |
|------|------------|------|
| **Monday Panic** | "47 clusters, where's the problem?" | Find broken deployment in 30 seconds |
| **Drift Detection** | "Someone edited prod directly" | Detect kubectl changes |
| **Security Patch** | "CVE affects 847 services" | Find and fix CCVEs |

---

## Quick Start

```bash
# 1. Deploy base apptique (pick one pattern)
kubectl apply -f ../flux-monorepo/apps/apptique/overlays/dev/
# OR
kubectl apply -f ../argo-applicationset/apps/apptique/dev/

# 2. Deploy a scenario
kubectl apply -f monday-panic/

# 3. Find the problem with cub-scout
./test/atk/map problems
./test/atk/scan
```

---

## Scenario 1: Monday Panic (Fleet Visibility)

**The story:** It's Monday morning. Alerts are firing. You have multiple deployments across namespaces (simulating clusters). One has a problem. Find it fast.

```bash
# Deploy the scenario
kubectl apply -f monday-panic/

# This creates:
# - apptique-east (healthy)
# - apptique-west (healthy)
# - apptique-eu   (BROKEN - wrong image, crashlooping)
```

**Find the problem:**
```bash
# See all deployments across namespaces
./test/atk/map workloads

# Expected output shows the broken deployment:
# STATUS  NAMESPACE      NAME      OWNER  IMAGE
# ✓       apptique-east  frontend  Flux   frontend:v0.10.3
# ✓       apptique-west  frontend  Flux   frontend:v0.10.3
# ✗       apptique-eu    frontend  Flux   frontend:v0.10.3-broken  ← PROBLEM!

# Or find problems directly
./test/atk/map problems

# Expected:
# ✗ Deployment/frontend in apptique-eu: 0/1 ready (CrashLoopBackOff)
```

**Cleanup:**
```bash
kubectl delete -f monday-panic/
```

---

## Scenario 2: Drift Detection

**The story:** It's 2am. Someone `kubectl edit`ed a production deployment directly. ArgoCD still shows "Synced" but the actual state has drifted from Git.

```bash
# Deploy base apptique
kubectl apply -f ../flux-monorepo/apps/apptique/overlays/prod/

# Simulate drift - someone edits directly
kubectl patch deployment frontend -n apptique-prod \
  -p '{"spec":{"replicas":10}}'  # Changed from 3 to 10

kubectl patch deployment frontend -n apptique-prod \
  -p '{"spec":{"template":{"spec":{"containers":[{"name":"frontend","env":[{"name":"DEBUG","value":"true"}]}]}}}}'
```

**Detect the drift:**
```bash
# cub-scout detects drift by comparing to Git source
./cub-scout trace deployment/frontend -n apptique-prod

# Shows:
# DRIFT DETECTED:
# - spec.replicas: expected 3, actual 10
# - env.DEBUG: not in source, present in cluster
```

**Remediate:**
```bash
# Force sync from Git (Flux)
flux reconcile kustomization apptique-prod --with-source

# Or (Argo CD)
argocd app sync apptique-prod --force
```

---

## Scenario 3: Security Patch (CCVE Scanning)

**The story:** A security vulnerability affects multiple services. You need to find all affected deployments and patch them.

```bash
# Deploy the scenario
kubectl apply -f security-patch/

# This creates deployments with known CCVEs:
# - CCVE-2025-0027: Grafana sidecar whitespace bug
# - CCVE-2025-0001: Missing resource limits
# - CCVE-2025-0003: Latest tag usage
```

**Find all CCVEs:**
```bash
./test/atk/scan

# Expected output:
# CONFIG CVE SCAN
# ════════════════════════════════════════════════════════════════════
# CRITICAL (1)
# [CCVE-2025-0027] apptique-monitoring/grafana-ccve
#   Grafana dashboard sidecar label selector whitespace bug
#
# HIGH (2)
# [CCVE-2025-0001] apptique-vulnerable/no-limits
#   Missing resource limits - can cause node exhaustion
# [CCVE-2025-0003] apptique-vulnerable/latest-tag
#   Using :latest tag - unpredictable deployments
```

**Bulk remediation:**
```bash
# Show what needs fixing
./cub-scout scan --json | jq '.findings[] | select(.severity == "critical")'

# The fix: update manifests in Git, then sync
# (This is where the RM pattern shines - one PR fixes all affected clusters)
```

**Cleanup:**
```bash
kubectl delete -f security-patch/
```

---

## Multi-Cluster Simulation

These scenarios use **namespaces to simulate clusters**. In a real fleet:

| Scenario | Namespaces (Demo) | Clusters (Real) |
|----------|-------------------|-----------------|
| Monday Panic | apptique-{east,west,eu} | prod-{east,west,eu} |
| Drift Detection | apptique-prod | Any production cluster |
| Security Patch | apptique-{monitoring,vulnerable} | All clusters |

**The cub-scout works the same way** — it detects ownership, traces sources, and scans for CCVEs regardless of whether you're testing locally or running across 100 clusters.

---

## See Also

- [../README.md](../README.md) — Apptique examples overview
- [../../rm-demos-argocd/](../../rm-demos-argocd/) — Simulation demos (sales presentations)
- [../../../docs/planning/RENDERED-MANIFEST-PATTERN.md](../../../docs/planning/RENDERED-MANIFEST-PATTERN.md) — Full RM pattern documentation
