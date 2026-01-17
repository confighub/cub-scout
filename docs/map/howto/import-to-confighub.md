# How To: Import Workloads to ConfigHub

The import wizard brings existing Kubernetes workloads into ConfigHub management. This guide shows how to import from various sources.

## The Problem

You have workloads deployed by Flux, ArgoCD, or Helm. You want to:
- Manage them centrally in ConfigHub
- Get DRY → WET → Live visibility
- Use the Hub/AppSpace model for platform + app team collaboration

**Question:** How do I bring my existing GitOps workloads into ConfigHub?

## Prerequisites

Before importing:
1. ConfigHub account (sign up at app.confighub.com)
2. `cub` CLI installed and authenticated
3. Worker connected to ConfigHub

```bash
# Check authentication
cub context get

# Check worker status
cub worker list
```

## The Solution

### CLI: Run import

```bash
cub-agent import
```

This launches an interactive wizard.

### TUI: Press 'i'

In the TUI (with `--hub` mode), press `i` to start the import wizard.

## Import Wizard Steps

### Step 1: Choose Import Source

```
What do you want to import?

> [1] Kubernetes namespace
  [2] ArgoCD Application
  [3] Flux Kustomization
  [4] Helm Release
```

### Step 2: Select Target

For namespace import:
```
Select namespace:
  > production
    staging
    development
```

For ArgoCD:
```
Select Application:
  > frontend (argocd)
    backend (argocd)
    payment-api (argocd)
```

### Step 3: Discover Workloads

The wizard shows what will be imported:
```
Discovered workloads in 'production':

  Deployments: 5
  Services: 8
  ConfigMaps: 12
  Secrets: 3

Continue? [Y/n]
```

### Step 4: Configure Space/Unit

```
ConfigHub Space: [production]
Unit name: [frontend]
```

### Step 5: Extract Configuration

The wizard extracts manifests and creates ConfigHub Units:
```
Extracting configuration...
  ✓ frontend/deployment.yaml
  ✓ frontend/service.yaml
  ✓ frontend/configmap.yaml

Creating ConfigHub Unit: frontend
  ✓ Unit created
  ✓ Revision pushed
```

### Step 6: Handle GitOps Controller (Optional)

For ArgoCD imports:
```
The ArgoCD Application 'frontend' currently manages these resources.

What would you like to do?
  > [1] Leave ArgoCD Application unchanged
    [2] Disable ArgoCD auto-sync
    [3] Delete ArgoCD Application (ConfigHub will manage)
```

### Step 7: Test Pipeline

```
Testing ConfigHub pipeline...
  ✓ Unit deployed via OCI
  ✓ Resources match expected state
  ✓ Import complete!
```

## Import Scenarios

### Scenario 1: Import from Namespace

Best for: Starting fresh, importing kubectl-applied resources

```bash
cub-agent import
# Select: Kubernetes namespace
# Select: your-namespace
```

### Scenario 2: Import ArgoCD Application

Best for: Migrating from ArgoCD to ConfigHub

```bash
cub-agent import
# Select: ArgoCD Application
# Select: your-app
# Choose: Delete ArgoCD Application (ConfigHub will manage)
```

### Scenario 3: Import Flux Kustomization

Best for: Migrating from Flux to ConfigHub

```bash
cub-agent import
# Select: Flux Kustomization
# Select: your-kustomization
```

## After Import

### Verify in ConfigHub

```bash
# Check unit was created
cub unit list

# Check deployment pipeline
cub unit get YOUR-UNIT
```

### Verify in TUI

```bash
cub-agent map --hub
# Navigate to your space/unit
```

### Check Ownership Changed

```bash
cub-agent map list -q "namespace=YOUR-NS"
# Should show owner=ConfigHub now
```

## Handling App of Apps

**Warning:** ArgoCD App of Apps patterns manage Application CRs, not workloads directly.

When importing an App of Apps:
```
Warning: This Application manages other Applications, not workloads.

Recommendation: Import the child Applications instead.

Child Applications found:
  - frontend
  - backend
  - payment-api

Import children instead? [Y/n]
```

## Rollback

If import fails or you want to undo:

```bash
# Delete ConfigHub Unit
cub unit delete YOUR-UNIT

# If ArgoCD Application was deleted, recreate it
kubectl apply -f your-argocd-app.yaml

# If Flux Kustomization was deleted, recreate it
kubectl apply -f your-kustomization.yaml
```

## Best Practices

1. **Import one at a time**: Start with a single app to test the flow
2. **Keep GitOps controller first**: Use option [1] to leave Flux/ArgoCD unchanged during testing
3. **Verify pipeline**: Use `cub unit apply` to test the ConfigHub pipeline works
4. **Switch gradually**: Only delete GitOps controller after ConfigHub is proven working

## Demo

Try the import demo:

```bash
# First, run the quick demo to create test resources
# DEPRECATED: ./test/atk/demo quick

# Then run import
cub-agent import
```

## Next Steps

- [Business Outcomes](../../outcomes/README.md) - Why ConfigHub import matters
- [ConfigHub Documentation](https://docs.confighub.com) - Full ConfigHub guide
