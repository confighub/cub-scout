# Ownership Visibility: The Native Bucket Insight

The most powerful insight from map is the **Native bucket** — resources that exist in your cluster but aren't managed by any GitOps tool.

## The Problem

Your GitOps tools all say "Synced" or "Applied":
- Flux shows green
- ArgoCD shows green
- Helm releases are deployed

But there are deployments in production nobody knows about.

**How?**
- Someone ran `kubectl apply` to fix an incident
- A debug pod was left behind
- A test configuration was never cleaned up
- Someone bypassed CI/CD "just this once"

**The risk:**
- These resources aren't in Git — no audit trail
- They won't survive a cluster rebuild
- They might have security or compliance issues
- Nobody knows who's responsible

## The Solution

```bash
cub-scout map orphans
```

Output:
```
RESOURCE            NAMESPACE    CREATED              SOURCE
deploy/debug-pod    prod         2026-01-10 14:30     kubectl applied
cm/temp-config      staging      2026-01-08 09:15     kubectl applied
secret/api-key      prod         2026-01-05 11:00     kubectl applied

Total: 3 orphan resources
```

These are your **Native** resources — the shadow IT bucket.

## Why This Matters

### Before Map

- You don't know what you don't know
- Production incidents reveal mystery resources
- Security audits find unauthorized deployments
- Cluster rebuilds lose undocumented config

### After Map

- One command shows all unmanaged resources
- Weekly audits catch drift early
- Security gets a clear inventory
- Nothing hides in production

## The Detection Logic

Map checks every resource for GitOps ownership labels:

| Check | Looking For |
|-------|-------------|
| Flux? | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` |
| ArgoCD? | Both `app.kubernetes.io/instance` AND `argocd.argoproj.io/instance` |
| Helm? | `app.kubernetes.io/managed-by: Helm` |
| ConfigHub? | `confighub.com/UnitSlug` |

If none match → **Native** (orphan)

## Common Native Resource Sources

### 1. Incident Response
```bash
# Someone ran this during an outage
kubectl scale deploy/api --replicas=10 -n prod
kubectl apply -f hotfix.yaml -n prod
```

### 2. Debugging
```bash
# Debug pod left behind
kubectl run debug --image=busybox -n prod -- sleep infinity
```

### 3. Testing
```bash
# Test config that should have been cleaned up
kubectl apply -f test-config.yaml -n staging
```

### 4. Bypassed CI/CD
```bash
# "I'll add it to Git later"
kubectl apply -f new-feature.yaml -n prod
```

## Taking Action

### Option 1: Adopt into GitOps

Add the resource to your Git repository:

```bash
# Export the resource
kubectl get deploy/debug-pod -n prod -o yaml > manifests/debug-pod.yaml

# Add to Git
git add manifests/debug-pod.yaml
git commit -m "Adopt debug-pod into GitOps"
git push

# Flux/ArgoCD will now manage it
```

### Option 2: Delete

If it's not needed:

```bash
kubectl delete deploy/debug-pod -n prod
```

### Option 3: Import to ConfigHub

```bash
cub-scout import
# Select the orphan resources
# Wizard creates ConfigHub Units
```

### Option 4: Document as Exception

Some Native resources are intentional:
- Controller-created resources
- Operator-managed CRDs
- System components

Document these as known exceptions.

## Ongoing Monitoring

### Weekly Audit

```bash
# Run weekly to catch drift
cub-scout map orphans -n prod
```

### Alert on New Orphans

```bash
# In CI/CD or cron
ORPHAN_COUNT=$(cub-scout map list -q "owner=Native AND namespace=prod*" --json | jq length)
if [ "$ORPHAN_COUNT" -gt 0 ]; then
  echo "WARNING: $ORPHAN_COUNT orphan resources in production"
  # Send alert
fi
```

### Namespace Policies

Consider admission controllers that require GitOps labels in production namespaces.

## Demo

Try the orphan-hunt scenario:

```bash
# DEPRECATED: ./test/atk/demo scenario orphan-hunt
```

This creates orphan resources and walks through finding and handling them.

## The Business Impact

| Metric | Before | After |
|--------|--------|-------|
| Time to find orphans | Unknown (might never find) | 30 seconds |
| Audit coverage | Incomplete | Complete |
| Compliance risk | Unknown | Quantified |
| Cluster rebuild confidence | Low | High |

## Summary

The Native bucket is map's killer feature:
- Shows what your GitOps tools can't see
- Identifies shadow IT and drift
- Enables complete cluster audits
- Turns "we don't know what we don't know" into "we know exactly what's there"
