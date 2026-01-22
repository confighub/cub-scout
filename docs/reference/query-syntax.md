# Query Syntax Reference

Filter and search resources across your cluster.

---

## Basic Syntax

```bash
cub-scout map list -q "FIELD=VALUE"
```

---

## Fields

| Field | Examples |
|-------|----------|
| `owner` | `Flux`, `ArgoCD`, `Helm`, `Native` |
| `namespace` | `default`, `flux-system`, `payments-*` |
| `kind` | `Deployment`, `Service`, `ConfigMap` |
| `status` | `Ready`, `Pending`, `Failed` |
| `labels[KEY]` | `labels[app]=nginx`, `labels[env]=prod` |

---

## Operators

| Operator | Meaning | Example |
|----------|---------|---------|
| `=` | Equals | `owner=Flux` |
| `!=` | Not equals | `owner!=Native` |
| `*` | Glob pattern | `namespace=prod-*` |

---

## Combining Conditions

```bash
# AND
cub-scout map list -q "owner=Flux AND status!=Ready"

# OR
cub-scout map list -q "owner=Flux OR owner=ArgoCD"

# Parentheses
cub-scout map list -q "labels[env]=prod AND (owner=Flux OR owner=ArgoCD)"
```

---

## Cheat Sheet

| Use Case | Query |
|----------|-------|
| Find orphans | `owner=Native` |
| GitOps-managed only | `owner!=Native` |
| Specific app | `labels[app]=payment-api` |
| Production only | `labels[env]=production` |
| Find problems | `status!=Ready` |
| Specific namespace | `namespace=payments-prod` |
| Multi-tool | `owner=Flux OR owner=ArgoCD` |
| Combined | `owner=Flux AND status!=Ready` |

---

## Output Formats

```bash
# Table (default)
cub-scout map list -q "owner=Flux"

# JSON (for scripting)
cub-scout map list -q "owner=Flux" --json

# Count only
cub-scout map list -q "owner=Flux" --count
```

---

## Real-World Examples

**"What's not managed by GitOps?"** (Security audit)
```bash
cub-scout map list -q "owner=Native"
```

**"What's broken right now?"** (Incident response)
```bash
cub-scout map list -q "status!=Ready"
```

**"Where is payment-api deployed?"**
```bash
cub-scout map list -q "labels[app]=payment-api"
```

**"Which Flux resources are unhealthy?"**
```bash
cub-scout map list -q "owner=Flux AND status!=Ready"
```

---

## See Also

- [howto/query-resources.md](../howto/query-resources.md) — Query guide
- [howto/find-orphans.md](../howto/find-orphans.md) — Finding shadow IT
