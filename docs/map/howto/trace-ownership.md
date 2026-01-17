# How To: Trace Ownership Chains

Tracing shows you the complete path from a Kubernetes resource back to its source. This guide explains how to trace ownership chains.

## The Problem

You see a deployment with issues. You need to know:
- Which GitOps controller manages it?
- Which Git repository is the source?
- What's the full chain from source to resource?

**Question:** Where did this deployment come from?

## The Solution

### CLI: Use trace command

```bash
cub-agent map trace deploy/payment-api -n prod
```

Output:
```
Deployment: payment-api
  └── HelmRelease: payment-api (flux-system)
      └── GitRepository: main-repo (flux-system)
          └── git@github.com:myorg/platform-config.git
```

### TUI: Press 'T'

In the interactive TUI:
1. Navigate to a resource
2. Press `T` to trace it
3. See the ownership chain in the details pane

## Understanding the Chain

### Flux Chain

```
Deployment
  └── Kustomization or HelmRelease
      └── GitRepository or OCIRepository
          └── Git URL or OCI registry
```

### ArgoCD Chain

```
Deployment
  └── Application
      └── Git repository (in Application spec)
```

### Helm Chain

```
Deployment
  └── Helm release (via labels)
      └── No further chain (direct install)
```

### Native Chain

```
Deployment
  └── (No GitOps owner detected)
```

## Trace Output Details

### Flux Kustomization

```bash
cub-agent map trace deploy/nginx -n web
```

```
Deployment: nginx (web)
  Owner: Flux
  └── Kustomization: web-apps (flux-system)
      Status: Applied revision: main@sha1:abc123
      └── GitRepository: platform-repo (flux-system)
          URL: ssh://git@github.com/myorg/platform-config
          Status: Fetched revision: main@sha1:abc123
```

### ArgoCD Application

```bash
cub-agent map trace deploy/frontend -n app
```

```
Deployment: frontend (app)
  Owner: ArgoCD
  └── Application: frontend (argocd)
      Project: default
      Sync: Synced
      Source: https://github.com/myorg/app-config
      Path: apps/frontend
      Revision: HEAD
```

### Native (No Owner)

```bash
cub-agent map trace deploy/debug-pod -n prod
```

```
Deployment: debug-pod (prod)
  Owner: Native
  └── No GitOps controller detected
      Created: 2026-01-10T14:30:00Z
      Annotations: kubectl.kubernetes.io/last-applied-configuration present
```

## Trace for Debugging

### Find why a resource isn't syncing

```bash
cub-agent map trace deploy/broken-app -n prod
```

Look for:
- Status messages on each link
- Error states on GitRepository/Application
- Suspended Kustomizations/HelmReleases

### Find the source to fix

The trace shows you exactly which Git repo and path to edit:

```
└── GitRepository: main-repo (flux-system)
    URL: git@github.com:myorg/platform-config.git
```

Go to that repo, find the manifest, fix it, push.

## Trace in JSON

For scripting:

```bash
cub-agent map trace deploy/payment-api -n prod --json
```

```json
{
  "resource": {
    "kind": "Deployment",
    "name": "payment-api",
    "namespace": "prod"
  },
  "owner": "Flux",
  "chain": [
    {
      "kind": "HelmRelease",
      "name": "payment-api",
      "namespace": "flux-system",
      "status": "Applied"
    },
    {
      "kind": "GitRepository",
      "name": "main-repo",
      "namespace": "flux-system",
      "url": "git@github.com:myorg/platform-config.git"
    }
  ]
}
```

## Try It

```bash
# Trace any deployment
cub-agent map trace deploy/nginx -n default

# Or use the TUI
cub-agent map
# Navigate to a resource and press T
```

## Next Steps

- [Scan for CCVEs](scan-for-ccves.md) - Find configuration issues along the chain
- [Query Resources](query-resources.md) - Filter before tracing
