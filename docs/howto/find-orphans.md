# Find Orphan Resources

Orphans are resources NOT managed by GitOps — they'll be lost on cluster rebuild.

---

## Quick Check

```bash
cub-scout map orphans
```

**Output:**

```
┌─ ORPHANS (Not in Git) ───────────────────────────────────────────────────────┐
│                                                                              │
│  ⚠ These resources will be LOST on cluster rebuild                          │
│                                                                              │
│  NAMESPACE     NAME                KIND          SCENARIO                    │
│  ─────────────────────────────────────────────────────────────────────────   │
│  default       debug-nginx         Deployment    Left from debugging        │
│  default       manual-config       ConfigMap     Applied during incident    │
│  kube-system   legacy-monitor      DaemonSet     Pre-GitOps installation    │
│                                                                              │
│  Found: 3 orphans across 2 namespaces                                       │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Using Queries

```bash
# All orphans
cub-scout map list -q "owner=Native"

# Orphans in specific namespace
cub-scout map list -q "owner=Native AND namespace=default"

# Count orphans
cub-scout map list -q "owner=Native" --count
```

---

## Why Orphans Are Risky

| Risk | Impact |
|------|--------|
| **Lost on rebuild** | Cluster recreation won't restore them |
| **No audit trail** | Who created it? When? Why? |
| **Conflicts with GitOps** | May be clobbered by reconciliation |
| **Security unknown** | Not reviewed through Git PR process |

---

## Common Orphan Sources

| Source | Example |
|--------|---------|
| Debugging | `kubectl run debug-pod --image=busybox` |
| Incident response | `kubectl apply -f hotfix.yaml` |
| Manual scaling | `kubectl scale deploy/app --replicas=5` |
| Pre-GitOps resources | Legacy monitoring, operators |
| Testing | Forgotten test deployments |

---

## What to Do with Orphans

### Option 1: Add to Git (recommended)

```bash
# Export the resource
kubectl get deploy debug-nginx -o yaml > apps/debug-nginx/deployment.yaml

# Remove runtime fields
# Add to Git, create Kustomization
```

### Option 2: Delete if not needed

```bash
kubectl delete deploy debug-nginx
```

### Option 3: Document the exception

Some resources are legitimately not GitOps-managed (cluster-critical CRDs, emergency configs).

---

## See Also

- [concepts/clobbering-problem.md](../concepts/clobbering-problem.md) — Why manual changes get reverted
- [reference/query-syntax.md](../reference/query-syntax.md) — Query syntax
