# Quickstart: Map in 5 Minutes

Get from zero to ownership visibility in 5 minutes.

## Prerequisites

- Go 1.21+ installed
- kubectl configured with cluster access
- (Optional) Flux, ArgoCD, or Helm deployments in your cluster

## Step 1: Build (30 seconds)

```bash
git clone https://github.com/confighub/cub-scout
cd cub-scout
go build ./cmd/cub-scout
```

## Step 2: Run Map (10 seconds)

```bash
./cub-scout map
```

You'll see the interactive TUI showing all resources grouped by owner.

## Step 3: Explore (2 minutes)

### Navigate views
Press these keys to switch views:

| Key | View |
|-----|------|
| `s` | Status dashboard |
| `w` | Workloads |
| `p` | Pipelines (GitOps deployers) |
| `o` | Orphans (Native resources) |

### Find orphans
Press `o` to see all unmanaged resources. These are resources deployed via `kubectl apply` or other non-GitOps methods.

### Search
Press `/` to search, then type a resource name.

### Get help
Press `?` to see all keyboard shortcuts.

### Quit
Press `q` to exit.

## Step 4: Try Subcommands (2 minutes)

### List all resources
```bash
./cub-scout map list
```

### Show only orphans
```bash
./cub-scout map orphans
```

### Trace a deployment's ownership
```bash
./cub-scout map trace deploy/YOUR-DEPLOYMENT -n YOUR-NAMESPACE
```

### Scan for configuration issues
```bash
./cub-scout scan
```

## Step 5: Query (Optional)

Filter resources with queries:

```bash
# All Flux-managed resources
./cub-scout map list -q "owner=Flux"

# All production namespaces
./cub-scout map list -q "namespace=prod*"

# All non-GitOps resources (shadow IT)
./cub-scout map list -q "owner=Native"
```

## What's Next?

### Try more features
```bash
# Find orphan resources (shadow IT)
./cub-scout map list -q "owner=Native"

# Scan for configuration issues
./cub-scout scan

# Trace a specific deployment
./cub-scout map trace deploy/YOUR-DEPLOYMENT -n YOUR-NAMESPACE
```

### Connect to ConfigHub
For multi-cluster visibility:
```bash
./cub-scout map --hub    # Requires cub CLI + authentication
```

### Learn more
- [How-To Guides](howto/) - Task-based guides
- [Reference](reference/) - All commands, views, and shortcuts
- [Business Outcomes](../outcomes/) - Why this matters

## Troubleshooting

### "no resources found"
- Check kubectl access: `kubectl get pods -A`
- Ensure you have workloads in your cluster

### "build failed"
- Ensure Go 1.21+: `go version`
- Run `go mod download` first

### "permission denied"
- Check kubectl context: `kubectl config current-context`
- Ensure you have read access to the cluster
