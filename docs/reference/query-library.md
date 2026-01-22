# Query Library Reference

Built-in and user-defined queries for filtering resources.

## Built-in Queries

| Name | Query | Description |
|------|-------|-------------|
| `unmanaged` | `owner=Native` | Resources with no GitOps owner |
| `gitops` | `owner=Flux OR owner=ArgoCD` | Resources managed by GitOps |
| `flux` | `owner=Flux` | All Flux-managed resources |
| `argo` | `owner=ArgoCD` | All Argo CD-managed resources |
| `helm-only` | `owner=Helm` | Helm-managed resources (no GitOps) |
| `confighub` | `owner=ConfigHub` | Resources managed by ConfigHub |
| `deployments` | `kind=Deployment` | All Deployments |
| `services` | `kind=Service` | All Services |
| `prod` | `namespace=prod* OR namespace=production*` | Production namespaces |

---

## Usage

### Run a saved query

```bash
cub-scout map list -q unmanaged
cub-scout map list -q gitops
cub-scout map list -q prod
```

### Combine queries with filters

```bash
# Unmanaged resources in production
cub-scout map list -q "unmanaged AND namespace=prod*"

# GitOps resources that aren't Flux
cub-scout map list -q "gitops AND owner!=Flux"

# Deployments managed by Argo
cub-scout map list -q "deployments AND argo"
```

### List all queries

```bash
cub-scout map queries
cub-scout map queries --json
```

---

## User Queries

Save custom queries to `~/.confighub/queries.yaml`:

```bash
# Save a new query
cub-scout map queries save my-team "labels[team]=payments" "Payment team resources"

# Use it
cub-scout map list -q my-team

# Delete it
cub-scout map queries delete my-team
```

### File Format

```yaml
# ~/.confighub/queries.yaml
queries:
  - name: my-team
    description: Payment team resources
    query: labels[team]=payments
  - name: critical-apps
    description: Production critical workloads
    query: "namespace=prod* AND labels[tier]=critical"
```

---

## Query Syntax

### Operators

| Operator | Example | Description |
|----------|---------|-------------|
| `=` | `owner=Flux` | Equals |
| `!=` | `owner!=Native` | Not equals |
| `=*` | `namespace=prod*` | Wildcard match |
| `AND` | `owner=Flux AND namespace=prod*` | Both conditions |
| `OR` | `owner=Flux OR owner=ArgoCD` | Either condition |

### Fields

| Field | Example | Description |
|-------|---------|-------------|
| `owner` | `owner=Flux` | Ownership (Flux, ArgoCD, Helm, Native, ConfigHub) |
| `kind` | `kind=Deployment` | Resource kind |
| `namespace` | `namespace=prod*` | Namespace (supports wildcards) |
| `name` | `name=payment*` | Resource name |
| `labels[key]` | `labels[team]=payments` | Label value |
| `variant` | `variant=prod` | Inferred variant |
| `status` | `status=Synced` | Sync status |

---

## Commands Reference

| Command | Description |
|---------|-------------|
| `cub-scout map queries` | List all saved queries |
| `cub-scout map queries --json` | List queries as JSON |
| `cub-scout map queries save <name> <query> [desc]` | Save a user query |
| `cub-scout map queries delete <name>` | Delete a user query |

---

## Example Queries

### Security and compliance

```bash
# Find unmanaged resources (potential security risk)
cub-scout map list -q unmanaged

# Find resources not in GitOps
cub-scout map list -q "owner=Native OR owner=Helm"
```

### By team/application

```bash
# Find payment team resources
cub-scout map list -q "labels[team]=payments"

# Find all resources for an app
cub-scout map list -q "labels[app]=order-service"
```

### By environment

```bash
# Production only
cub-scout map list -q prod

# Staging deployments
cub-scout map list -q "namespace=staging* AND kind=Deployment"
```

### By GitOps tool

```bash
# Flux only
cub-scout map list -q flux

# Argo only
cub-scout map list -q argo

# Either
cub-scout map list -q gitops
```

---

## See Also

- [Commands Reference](commands.md) — Full CLI reference
- [Views Reference](views.md) — TUI views and keybindings
