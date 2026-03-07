# Break-Glass to Managed

Step-by-step guide for handling emergency `kubectl apply` resources and transitioning them back under GitOps management.

---

## When You Need This

Someone ran `kubectl apply` during an incident, bypassing GitOps. Now the cluster has resources that:

- Are not tracked in Git
- Will be lost on cluster rebuild
- May conflict with the next GitOps reconciliation
- Have no audit trail

This guide walks through detection, assessment, and resolution.

---

## Prerequisites

```bash
# Build cub-scout
go build ./cmd/cub-scout

# Verify cluster access
kubectl cluster-info
```

For the import step (optional): ConfigHub connected mode (`cub auth login`).

---

## Step 1: Detect Orphan Resources

Orphan resources have no GitOps owner — cub-scout classifies them as **Native**.

```bash
# Quick orphan check
./cub-scout map orphans

# Or use query syntax
./cub-scout map list -q "owner=Native"

# Filter to a specific namespace
./cub-scout map list -q "owner=Native AND namespace=production"
```

**What to look for:** Any resource with `Owner: Native` that wasn't intentionally left unmanaged. Common break-glass indicators:

- Resources with `break-glass/*` annotations
- Resources without standard GitOps labels (`kustomize.toolkit.fluxcd.io/*`, `argocd.argoproj.io/*`, `app.kubernetes.io/managed-by`)
- Resources created recently during an incident window

---

## Step 2: Inspect the Resource

Before deciding what to do, understand what was deployed and why.

```bash
# Check annotations for incident context
kubectl get deploy/<name> -n <namespace> -o yaml | grep -A5 "annotations:"

# Look for break-glass markers
kubectl get deploy/<name> -n <namespace> \
  -o jsonpath='{.metadata.annotations}' | jq .
```

Common break-glass annotations to look for:

| Annotation | Meaning |
|------------|---------|
| `break-glass/incident` | Incident ticket ID |
| `break-glass/applied-by` | Who applied the hotfix |
| `break-glass/applied-at` | When it was applied |
| `break-glass/reason` | Why it was needed |

---

## Step 3: Trace Ownership

Use `trace` to confirm the resource is truly unmanaged and see what cub-scout knows about it.

```bash
./cub-scout trace deploy/<name> -n <namespace>
```

**Expected output for a break-glass resource:**
- Tool: none (not Flux, ArgoCD, or Helm)
- No ownership chain
- Resource appears as standalone

If trace shows an unexpected owner (e.g., Flux), the resource may have been reconciled already — check GitOps status before proceeding.

---

## Step 4: Decide

Three outcomes are available. Choose based on your team's incident review.

### Option A: Accept — Bring Under Management

The break-glass change was correct and should be permanent.

**With ConfigHub (connected mode):**

```bash
# Preview what would be imported
./cub-scout import -n <namespace> --dry-run

# Import into ConfigHub and record decision reason
./cub-scout import -n <namespace> \
  --audit-reason "approved by sre lead for Q1 migration"

# Review break-glass decision history
./cub-scout audit list -n <namespace> --since 7d
```

This creates a ConfigHub Unit, publishes an OCI artifact, and Flux/ArgoCD reconciles from ConfigHub. The resource is now versioned and repeatable.

**Without ConfigHub (standalone):**

```bash
# Export the resource manifest
kubectl get deploy/<name> -n <namespace> -o yaml > manifests/<name>.yaml

# Remove runtime fields (status, managedFields, resourceVersion, uid, etc.)
# Add to your Git repo under the appropriate Kustomization/HelmRelease path
# Commit, push, and let GitOps reconcile
```

### Option B: Reject — Remove the Hotfix

The break-glass change was temporary or the proper fix is now in Git.

```bash
# Delete the orphan resource
kubectl delete deploy/<name> -n <namespace>

# Verify GitOps reconciles the correct state
flux reconcile kustomization <name> --with-source
# or
argocd app sync <app-name>
```

### Option C: Defer — Track for Later

You need more time to decide, but want visibility.

```bash
# Add a tracking label
kubectl label deploy/<name> -n <namespace> \
  break-glass/tracked="true" \
  break-glass/review-by="2026-03-01"

# Find tracked break-glass resources later
./cub-scout map list -q "owner=Native"
kubectl get deploy -l break-glass/tracked=true --all-namespaces
```

---

## Step 5: Verify

After taking action, confirm the cluster is in the expected state.

```bash
# Re-check for orphans
./cub-scout map orphans

# Verify GitOps status
./cub-scout gitops status

# If using ConfigHub, verify import
cub unit list --space <space>
```

---

## Guided Testing: Break-Glass Demo

cub-scout includes a break-glass demo scenario with pre-built fixtures.

### Run the Demo

```bash
# Apply break-glass fixtures
./cub-scout demo scenario break-glass

# What it creates:
#   break-glass-demo namespace with:
#   - payment-api (Flux-managed, with Kustomization labels)
#   - hotfix-cache (Native/orphan, with break-glass annotations)
```

### Verify Detection

```bash
# payment-api should show as Flux-owned
./cub-scout map list -n break-glass-demo

# hotfix-cache should appear as orphan
./cub-scout map orphans
```

### Validate Ownership

```bash
# Flux-managed resource
./cub-scout trace deploy/payment-api -n break-glass-demo
# Expected: Owner=Flux, Kustomization=payment-api

# Break-glass resource
./cub-scout trace deploy/hotfix-cache -n break-glass-demo
# Expected: Owner=Native (no GitOps chain)
```

### Clean Up

```bash
./cub-scout demo scenario break-glass --cleanup
```

---

## Graceful Degradation

| Scenario | Behavior |
|----------|----------|
| No GitOps CRDs installed | Orphan detection still works (all resources show as Native) |
| ConfigHub not connected | Detection and assessment work; import requires connected mode |
| Cluster unreachable | cub-scout reports connection error with remediation steps |
| Break-glass annotations missing | Resource still detected as orphan; less context available |

---

## Weekly Audit

Establish a recurring check for break-glass resources:

```bash
# Monday morning orphan check
./cub-scout map orphans

# Alert if orphans exist in production
if [ "$(./cub-scout map list -q 'owner=Native AND namespace=production' --count 2>/dev/null)" -gt 0 ]; then
  echo "WARNING: Orphan resources in production"
fi
```

---

## See Also

- [Find Orphans](find-orphans.md) — Orphan detection reference
- [Import to ConfigHub](import-to-confighub.md) — Full import path
- [Import from Live](import-from-live.md) — Cluster-only discovery
- [Trace Ownership](trace-ownership.md) — Ownership chain tracing
- [Break-Glass Scenarios](../outcomes/break-glass-scenarios.md) — Conceptual overview
- [Trace Context Troubleshooting](trace-context-troubleshooting.md) — When trace fails
