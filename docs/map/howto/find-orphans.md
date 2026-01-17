# How To: Find Orphan Resources

Orphan resources (also called "shadow IT") are Kubernetes resources deployed outside your GitOps pipeline. This guide shows how to find and manage them.

## The Problem

Someone ran `kubectl apply` in production. A debug pod was left behind. A configuration was patched directly. Your GitOps tools say everything is synced, but there are resources you don't know about.

**Question:** What resources exist outside GitOps?

## The Solution

### Quick: Use the orphans command

```bash
cub-agent map orphans
```

Shows all resources with `owner=Native` (no GitOps ownership detected).

### TUI: Press 'o'

In the interactive TUI, press `o` to switch to the Orphans view.

### Query: Filter explicitly

```bash
cub-agent map list -q "owner=Native"
```

## Understanding Orphans

A resource is marked as **Native** when:
1. It has no Flux toolkit labels
2. It has no ArgoCD instance labels (or only one of the two required)
3. It has no Helm managed-by label
4. It has no ConfigHub unit slug

This usually means:
- Someone ran `kubectl apply` directly
- A CRD controller created the resource
- Labels were accidentally removed
- A job or cronjob created pods

## Common Orphan Types

### Intentional Orphans
- Debug pods for troubleshooting
- Temporary jobs
- Resources managed by controllers (not GitOps)

### Unintentional Orphans
- Forgotten manual deployments
- Failed cleanup after testing
- Bypassed CI/CD pipeline

## Investigating Orphans

### See who created it

```bash
kubectl get deploy ORPHAN-NAME -n NS -o jsonpath='{.metadata.annotations}'
```

Look for `kubectl.kubernetes.io/last-applied-configuration` annotation.

### Check when it was created

```bash
kubectl get deploy ORPHAN-NAME -n NS -o jsonpath='{.metadata.creationTimestamp}'
```

### Trace its history

```bash
cub-agent map trace deploy/ORPHAN-NAME -n NS
```

For Native resources, this shows there's no GitOps owner.

## Filtering Orphans

### By namespace
```bash
cub-agent map list -q "owner=Native AND namespace=production"
```

### By kind
```bash
cub-agent map list -q "owner=Native AND kind=Deployment"
```

### Exclude system namespaces
```bash
cub-agent map list -q "owner=Native AND namespace!=kube-system AND namespace!=kube-public"
```

## What To Do With Orphans

### Option 1: Adopt into GitOps
Add the resource to your Git repository and let Flux/ArgoCD manage it.

### Option 2: Delete
If it's not needed:
```bash
kubectl delete deploy ORPHAN-NAME -n NS
```

### Option 3: Import to ConfigHub
Use the import wizard to bring it under ConfigHub management:
```bash
cub-agent import
```

### Option 4: Document and Accept
Some orphans are intentional (controller-created resources). Document them as exceptions.

## Try It

```bash
# List all orphan resources
cub-agent map list -q "owner=Native"

# Filter to production only
cub-agent map list -q "owner=Native AND namespace=prod*"

# Use the TUI and press 'o' for orphans view
cub-agent map
```

## Best Practices

1. **Regular audits**: Run `map orphans` weekly to catch drift
2. **Alert on new orphans**: Integrate with monitoring
3. **Namespace policies**: Require GitOps labels in production namespaces
4. **Document exceptions**: Keep a list of intentional orphans

## Next Steps

- [Trace Ownership](trace-ownership.md) - Understand the full chain
- [Import to ConfigHub](import-to-confighub.md) - Adopt orphans into ConfigHub
