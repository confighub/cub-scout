# Query Resources

Filter and search across your cluster using powerful queries.

---

## Basic Query

```bash
cub-scout map list -q "owner=Flux"
```

**Output:**

```
NAMESPACE        NAME                    KIND           OWNER    STATUS
flux-system      source-controller       Deployment     Flux     Ready
flux-system      kustomize-controller    Deployment     Flux     Ready
podinfo          podinfo                 Deployment     Flux     Ready
```

---

## Query by Owner

```bash
# Find all Flux resources
cub-scout map list -q "owner=Flux"

# Find all ArgoCD resources
cub-scout map list -q "owner=ArgoCD"

# Find all Helm resources
cub-scout map list -q "owner=Helm"

# Find orphans (not managed by GitOps)
cub-scout map list -q "owner=Native"

# Find all GitOps-managed resources
cub-scout map list -q "owner!=Native"
```

---

## Query by Namespace

```bash
# Exact match
cub-scout map list -q "namespace=podinfo"

# Glob pattern
cub-scout map list -q "namespace=prod-*"
```

---

## Query by Labels

```bash
# By app label
cub-scout map list -q "labels[app]=podinfo"

# By environment
cub-scout map list -q "labels[env]=production"

# By team
cub-scout map list -q "labels[team]=platform"
```

---

## Query by Status

```bash
# Only healthy resources
cub-scout map list -q "status=Ready"

# Find problems
cub-scout map list -q "status!=Ready"
```

---

## Combine Conditions

```bash
# AND
cub-scout map list -q "owner=Flux AND status!=Ready"

# OR
cub-scout map list -q "owner=Flux OR owner=ArgoCD"

# Parentheses
cub-scout map list -q "labels[env]=prod AND (owner=Flux OR owner=ArgoCD)"
```

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

## Common Use Cases

**Security audit: What's not in Git?**
```bash
cub-scout map list -q "owner=Native"
```

**Incident response: What's broken?**
```bash
cub-scout map list -q "status!=Ready"
```

**Find specific app across environments:**
```bash
cub-scout map list -q "labels[app]=payment-api"
```

**Show all GitOps resources:**
```bash
cub-scout map list -q "owner!=Native"
```

---

## See Also

- [reference/query-syntax.md](../reference/query-syntax.md) — Full syntax reference
- [howto/find-orphans.md](find-orphans.md) — Finding shadow IT
