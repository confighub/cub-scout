# Query Syntax Reference

Complete reference for the map query language.

## Quick Reference

```bash
# Basic
cub-agent map list -q "owner=Flux"
cub-agent map list -q "namespace=prod*"

# Logical
cub-agent map list -q "owner=Flux AND namespace=production"
cub-agent map list -q "owner=Flux OR owner=ArgoCD"

# Labels
cub-agent map list -q "labels[app]=nginx"
```

---

## Operators

### Equality

| Operator | Meaning | Example |
|----------|---------|---------|
| `=` | Equals | `owner=Flux` |
| `!=` | Not equals | `owner!=Native` |

### Pattern Matching

| Operator | Meaning | Example |
|----------|---------|---------|
| `~=` | Regex match | `name~=^api-.*` |
| `*` | Wildcard | `namespace=prod*` |

### Multiple Values

```bash
# IN list (comma-separated)
owner=Flux,ArgoCD,Helm

# Equivalent to OR
owner=Flux OR owner=ArgoCD OR owner=Helm
```

---

## Logical Operators

### AND

Both conditions must match:

```bash
cub-agent map list -q "owner=Flux AND namespace=production"
```

### OR

Either condition matches:

```bash
cub-agent map list -q "owner=Flux OR owner=ArgoCD"
```

### Grouping

Use parentheses for complex expressions:

```bash
cub-agent map list -q "(owner=Flux OR owner=ArgoCD) AND namespace=prod*"
```

---

## Available Fields

| Field | Description | Values |
|-------|-------------|--------|
| `kind` | Resource type | `Deployment`, `Service`, `Pod`, etc. |
| `namespace` | Namespace | Any namespace name |
| `name` | Resource name | Any resource name |
| `owner` | Detected owner | `Flux`, `ArgoCD`, `Helm`, `ConfigHub`, `Native` |
| `status` | Resource status | `Ready`, `Synced`, `Applied`, `Failed`, etc. |
| `cluster` | Cluster name | Cluster identifier (fleet mode) |
| `labels[key]` | Label value | Any label key/value |

---

## Field Details

### kind

Resource type (singular, capitalized):

```bash
kind=Deployment
kind=Service
kind=ConfigMap
kind=Pod
kind=Kustomization
kind=HelmRelease
kind=Application
```

### namespace

Kubernetes namespace:

```bash
namespace=default
namespace=production
namespace=prod*           # Wildcard: starts with "prod"
namespace=*-prod          # Wildcard: ends with "-prod"
```

### name

Resource name:

```bash
name=nginx
name=api-*                # Wildcard
name~=^payment-.*         # Regex
```

### owner

Detected GitOps owner:

| Value | Meaning |
|-------|---------|
| `Flux` | Managed by Flux |
| `ArgoCD` | Managed by ArgoCD |
| `Helm` | Managed by Helm |
| `ConfigHub` | Managed by ConfigHub |
| `Native` | No GitOps owner (unmanaged) |

```bash
owner=Flux
owner=ArgoCD
owner!=Native             # All GitOps-managed
owner=Flux,ArgoCD,Helm    # Any GitOps
```

### status

Resource status (varies by resource type):

```bash
status=Ready
status=Synced
status=Applied
status=Failed
status=Suspended
status=OutOfSync
```

### labels[key]

Access label values:

```bash
labels[app]=nginx
labels[team]=platform
labels[env]=production
labels[app.kubernetes.io/name]=frontend
```

Note: Label keys with special characters need quoting in some shells:

```bash
# Shell-safe
cub-agent map list -q 'labels[app.kubernetes.io/name]=frontend'
```

---

## Wildcards

The `*` character matches any string:

| Pattern | Matches |
|---------|---------|
| `prod*` | `prod`, `production`, `prod-east` |
| `*-prod` | `api-prod`, `web-prod` |
| `*prod*` | `production`, `preprod`, `prod-test` |

```bash
cub-agent map list -q "namespace=prod*"
cub-agent map list -q "name=api-*"
```

---

## Regular Expressions

Use `~=` for regex matching:

```bash
# Names starting with "api-"
cub-agent map list -q "name~=^api-.*"

# Names containing "payment"
cub-agent map list -q "name~=.*payment.*"

# Namespaces matching pattern
cub-agent map list -q "namespace~=^(prod|staging)-.*"
```

Regex syntax follows Go's `regexp` package (RE2).

---

## Common Query Patterns

### Security/Compliance

```bash
# All unmanaged resources
owner=Native

# All GitOps-managed resources
owner!=Native

# Unmanaged in production
owner=Native AND namespace=prod*
```

### Flux-specific

```bash
# All Flux resources
owner=Flux

# Suspended Kustomizations
kind=Kustomization AND status=Suspended

# Failed HelmReleases
kind=HelmRelease AND status=Failed
```

### ArgoCD-specific

```bash
# All ArgoCD resources
owner=ArgoCD

# Out-of-sync Applications
kind=Application AND status=OutOfSync
```

### By label

```bash
# By app name
labels[app]=payment-api

# By team
labels[team]=platform

# By environment
labels[env]=production
```

### Namespace patterns

```bash
# All production namespaces
namespace=prod*,*-prod,production

# All non-system namespaces
namespace!=kube-system AND namespace!=kube-public
```

---

## Saved Queries

Built-in saved queries (access with `Q` in TUI):

| Name | Query |
|------|-------|
| `all` | (no filter) |
| `orphans` | `owner=Native` |
| `gitops` | `owner!=Native` |
| `flux` | `owner=Flux` |
| `argo` | `owner=ArgoCD` |
| `helm` | `owner=Helm` |
| `confighub` | `owner=ConfigHub` |
| `prod` | `namespace=*-prod,prod-*,production` |
| `dev` | `namespace=*-dev,dev-*,development` |

---

## Query in TUI

### Command Palette

Press `:` and type query:

```
:owner=Flux AND namespace=production
```

### Saved Queries

Press `Q` to select from saved queries.

### Search vs Query

- `/` = Search (text match in displayed content)
- `:` = Query (structured field filtering)

---

## JSON Output

Combine queries with JSON for scripting:

```bash
# Get all orphan names
cub-agent map list -q "owner=Native" --json | jq '.[].name'

# Count by owner
cub-agent map list --json | jq 'group_by(.owner) | map({owner: .[0].owner, count: length})'

# Filter in jq
cub-agent map list --json | jq '[.[] | select(.namespace | startswith("prod"))]'
```

---

## Troubleshooting

### Query returns no results

- Check field names are correct
- Check values match exactly (case-sensitive)
- Try wildcards: `namespace=*prod*`

### Regex not matching

- Use `~=` not `=` for regex
- Escape special characters
- Test regex with simple pattern first

### Shell escaping issues

```bash
# Use single quotes for complex queries
cub-agent map list -q 'labels[app.kubernetes.io/name]=frontend'

# Or escape special characters
cub-agent map list -q "labels[app.kubernetes.io\/name]=frontend"
```

## See Also

- [How To: Query Resources](../howto/query-resources.md) - Query guide with examples
- [Commands Reference](commands.md) - CLI flags
