# How To: Query and Filter Resources

Map includes a powerful query language for filtering resources. This guide shows how to use queries effectively.

## The Problem

You have hundreds of resources. You need to find:
- All Flux-managed deployments in production
- Resources with specific labels
- Everything in a namespace pattern

**Question:** How do I filter to see only what I need?

## The Solution

### CLI: Use -q flag

```bash
cub-agent map list -q "owner=Flux AND namespace=prod*"
```

### TUI: Press ':'

In the TUI, press `:` to open the command palette, then type your query.

### TUI: Press 'Q'

Press `Q` to open saved queries, then select from common filters.

## Query Syntax

### Basic operators

| Operator | Meaning | Example |
|----------|---------|---------|
| `=` | Equals | `owner=Flux` |
| `!=` | Not equals | `owner!=Native` |
| `~=` | Regex match | `name~=^api-.*` |
| `*` | Wildcard | `namespace=prod*` |

### Multiple values

```bash
# IN list (any of these)
cub-agent map list -q "owner=Flux,ArgoCD,Helm"

# Equivalent to OR
cub-agent map list -q "owner=Flux OR owner=ArgoCD OR owner=Helm"
```

### Logical operators

```bash
# AND (both must match)
cub-agent map list -q "owner=Flux AND namespace=production"

# OR (either matches)
cub-agent map list -q "owner=Flux OR owner=ArgoCD"

# Combined
cub-agent map list -q "(owner=Flux OR owner=ArgoCD) AND namespace=prod*"
```

### Available fields

| Field | Description | Example |
|-------|-------------|---------|
| `kind` | Resource type | `kind=Deployment` |
| `namespace` | Namespace | `namespace=prod*` |
| `name` | Resource name | `name=payment-api` |
| `owner` | Detected owner | `owner=Flux` |
| `status` | Resource status | `status=Synced` |
| `cluster` | Cluster name | `cluster=prod-east` |
| `labels[key]` | Label value | `labels[app]=nginx` |

## Saved Queries

Press `Q` in the TUI to access built-in queries:

| Query Name | Filter |
|------------|--------|
| `all` | (no filter) |
| `orphans` | `owner=Native` |
| `gitops` | `owner!=Native` |
| `flux` | `owner=Flux` |
| `argo` | `owner=ArgoCD` |
| `helm` | `owner=Helm` |
| `prod` | `namespace=*-prod,prod-*,production` |
| `dev` | `namespace=*-dev,dev-*,development` |

## Common Queries

### Find all orphans in production

```bash
cub-agent map list -q "owner=Native AND namespace=prod*"
```

### Find all GitOps-managed resources

```bash
cub-agent map list -q "owner!=Native"
```

### Find resources by label

```bash
cub-agent map list -q "labels[app]=payment-api"
```

### Find deployments with issues

```bash
cub-agent map list -q "kind=Deployment AND status!=Ready"
```

### Find resources in multiple namespaces

```bash
cub-agent map list -q "namespace=team-a*,team-b*"
```

### Find Flux Kustomizations that are suspended

```bash
cub-agent map list -q "kind=Kustomization AND status=Suspended"
```

## Query in TUI

### Using command palette

1. Press `:` to open command palette
2. Type your query: `owner=Flux AND namespace=prod*`
3. Press Enter to apply

### Using saved queries

1. Press `Q` to open saved queries
2. Select a query with arrow keys
3. Press Enter to apply

### Clear filter

Press `Escape` or select "all" from saved queries.

## JSON Output

For scripting, combine queries with JSON output:

```bash
cub-agent map list -q "owner=Native" --json | jq '.[] | .name'
```

## Query Examples by Use Case

### Security audit
```bash
# Find all non-GitOps resources in production
cub-agent map list -q "owner=Native AND namespace=prod*"
```

### Flux audit
```bash
# Find suspended Flux resources
cub-agent map list -q "owner=Flux AND status=Suspended"
```

### ArgoCD audit
```bash
# Find out-of-sync ArgoCD apps
cub-agent map list -q "owner=ArgoCD AND status=OutOfSync"
```

### Resource inventory
```bash
# Count resources by owner
cub-agent map list --json | jq 'group_by(.owner) | map({owner: .[0].owner, count: length})'
```

## Next Steps

- [Trace Ownership](trace-ownership.md) - Trace a found resource
- [Import to ConfigHub](import-to-confighub.md) - Bring resources into ConfigHub
