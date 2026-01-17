# Drift Detection Scenario

**The story:** It's 2am. Someone `kubectl edit`ed a production deployment directly. ArgoCD still shows "Synced" but the actual state has drifted from Git.

---

## The Problem

ArgoCD's "Synced" status means "the live state matches what I last applied."
It does NOT mean "the live state matches Git."

If someone does `kubectl edit`, `kubectl scale`, or `kubectl patch`:
- ArgoCD may still show "Synced"
- The actual config has drifted from Git source
- Next sync will overwrite the manual change (or not, depending on settings)

---

## Demo Steps

### 1. Deploy the Base Configuration

```bash
# Apply the base deployment (this is what Git says)
kubectl apply -f base-deployment.yaml

# Verify it's running
kubectl get deployment frontend -n apptique-drift
# Expected: 3 replicas
```

### 2. Create Drift (Simulate 2am kubectl)

```bash
# Run the drift script
./create-drift.sh

# Or manually:
kubectl patch deployment frontend -n apptique-drift \
  -p '{"spec":{"replicas":10}}'

kubectl patch deployment frontend -n apptique-drift \
  -p '{"spec":{"template":{"spec":{"containers":[{"name":"frontend","env":[{"name":"DEBUG","value":"true"}]}]}}}}'
```

### 3. Notice ArgoCD Still Shows "Synced"

If using ArgoCD:
```bash
argocd app get apptique-drift
# Shows: Synced, Healthy
# But... the config has drifted!
```

### 4. Detect Drift with ConfigHub Agent

```bash
# Trace shows the drift
./cub-agent trace deployment/frontend -n apptique-drift

# Expected output:
# DRIFT DETECTED
# ┌──────────────────────────────────────────────────────┐
# │ deployment/frontend                                   │
# │ namespace: apptique-drift                             │
# │ owner: ArgoCD                                         │
# │                                                       │
# │ ⚠ DRIFT from Git source:                             │
# │   - spec.replicas: expected 3, actual 10             │
# │   - env.DEBUG: not in source, present in cluster     │
# └──────────────────────────────────────────────────────┘

# Or check all drifted resources
./test/atk/map problems
```

### 5. Remediate

```bash
# Force sync from Git
# Flux:
flux reconcile kustomization apptique-drift --with-source

# ArgoCD:
argocd app sync apptique-drift --force

# Or just reapply:
kubectl apply -f base-deployment.yaml
```

### 6. Cleanup

```bash
kubectl delete -f base-deployment.yaml
kubectl delete namespace apptique-drift
```

---

## What Drifted?

| Field | Git Source | After Drift |
|-------|------------|-------------|
| `spec.replicas` | 3 | 10 |
| `env.DEBUG` | (not present) | "true" |

---

## Why This Matters

**Without drift detection:**
- Manual changes accumulate silently
- "Works on my cluster" syndrome
- Outages when next sync overwrites changes
- No audit trail of who changed what

**With ConfigHub Agent:**
- Immediate drift visibility
- Compare live state vs Git source
- Audit trail via trace command
- Bulk detection across fleet

---

## See Also

- [../README.md](../README.md) — All scenarios overview
- [../../../docs/planning/RENDERED-MANIFEST-PATTERN.md](../../../docs/planning/RENDERED-MANIFEST-PATTERN.md) — RM pattern documentation
