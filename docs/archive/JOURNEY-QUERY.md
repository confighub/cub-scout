# Journey: Fleet Queries

**Time:** 5 minutes
**Goal:** Query resources across your fleet using powerful filters

**Prerequisites:** Have workloads imported into ConfigHub (see [JOURNEY-IMPORT.md](JOURNEY-IMPORT.md)).

---

## What Are Fleet Queries?

Queries let you filter and search across all your resources:
- Across namespaces
- Across clusters (when connected to ConfigHub)
- By owner, labels, status, and more

---

## Step 1: Basic Query Syntax

```bash
./cub-scout map list -q "FIELD=VALUE"
```

**Example:**
```bash
./cub-scout map list -q "owner=Flux"
```

**Expected output:**

```
NAMESPACE        NAME                    KIND           OWNER    STATUS
flux-system      source-controller       Deployment     Flux     Ready
flux-system      kustomize-controller    Deployment     Flux     Ready
payments-prod    payment-api             Deployment     Flux     Ready
payments-prod    payment-worker          Deployment     Flux     Ready
orders-prod      order-service           Deployment     Flux     Ready
```

---

## Step 2: Query by Owner

| Query | What It Finds |
|-------|---------------|
| `owner=Flux` | Flux-managed resources |
| `owner=ArgoCD` | Argo CD-managed resources |
| `owner=Helm` | Helm-managed resources |
| `owner=Native` | No GitOps owner (orphans) |
| `owner!=Native` | All GitOps-managed resources |

**Find orphans (security/rebuild risk):**
```bash
./cub-scout map list -q "owner=Native"
```

**Find all GitOps-managed:**
```bash
./cub-scout map list -q "owner!=Native"
```

---

## Step 3: Query by Namespace

```bash
# Exact match
./cub-scout map list -q "namespace=payments-prod"

# Pattern match (glob)
./cub-scout map list -q "namespace=payments-*"

# Multiple namespaces
./cub-scout map list -q "namespace=payments-prod OR namespace=orders-prod"
```

---

## Step 4: Query by Labels

```bash
# By app label
./cub-scout map list -q "labels[app]=payment-api"

# By team label
./cub-scout map list -q "labels[team]=platform"

# By environment
./cub-scout map list -q "labels[env]=production"

# Combined
./cub-scout map list -q "labels[app]=payment-api AND labels[env]=production"
```

---

## Step 5: Query by Status

```bash
# Only healthy resources
./cub-scout map list -q "status=Ready"

# Find problems
./cub-scout map list -q "status!=Ready"

# Find specific issues
./cub-scout map list -q "status=Pending"
./cub-scout map list -q "status=Failed"
```

---

## Step 6: Combine Conditions

Use `AND`, `OR`, and parentheses:

```bash
# Flux resources that aren't ready
./cub-scout map list -q "owner=Flux AND status!=Ready"

# Payment services across all environments
./cub-scout map list -q "labels[app]=payment-api"

# Production resources from either team
./cub-scout map list -q "labels[env]=production AND (labels[team]=payments OR labels[team]=orders)"
```

---

## Step 7: Fleet Queries (Connected Mode)

When connected to ConfigHub, query across all clusters:

```bash
# All payment-api instances across fleet
cub unit list --where "Labels.app='payment-api'"

# All prod variants
cub unit list --where "Labels.variant='prod'"

# Find units behind on revision
cub unit list --where "Revision < 127"
```

**Expected output:**

```
SPACE           UNIT              VARIANT    TARGET         REVISION
payments-prod   payment-api       prod       k8s-east       127
payments-prod   payment-api       prod       k8s-west       127
payments-prod   payment-api       prod       k8s-eu         124  ← behind!
payments-staging payment-api      staging    k8s-staging    130
```

---

## Step 8: Save Queries

Save frequently used queries:

```bash
# Save a query
./cub-scout map list -q "owner=Native" --save orphans

# Run saved query
./cub-scout map list --query orphans

# List saved queries
./cub-scout map list --list-queries
```

See [TUI-SAVED-QUERIES.md](TUI-SAVED-QUERIES.md) for the full guide.

---

## Query Cheat Sheet

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
./cub-scout map list -q "owner=Flux"

# JSON (for scripting)
./cub-scout map list -q "owner=Flux" --json

# Count only
./cub-scout map list -q "owner=Flux" --count

# Names only
./cub-scout map list -q "owner=Flux" --names-only
```

---

## Real-World Examples

### "What's not managed by GitOps?"

```bash
./cub-scout map list -q "owner=Native"
```

Security audit: These resources aren't tracked in Git.

### "What's broken right now?"

```bash
./cub-scout map list -q "status!=Ready"
```

Incident response: Focus on unhealthy resources.

### "Where is payment-api deployed?"

```bash
./cub-scout map list -q "labels[app]=payment-api"
```

Or with ConfigHub connected:
```bash
cub unit list --where "Labels.app='payment-api'"
```

### "Which clusters are behind?"

```bash
cub unit list --where "Labels.app='payment-api'" | grep -v "127"
```

Find units not at latest revision.

---

## Next Steps

| Journey | What You'll Learn |
|---------|-------------------|
| [**TUI-SAVED-QUERIES.md**](TUI-SAVED-QUERIES.md) | Save and share queries |
| [**JOURNEY-SCAN.md**](JOURNEY-SCAN.md) | Find configuration issues |
| [**CLI-REFERENCE.md**](CLI-REFERENCE.md) | Full query syntax reference |

---

**Previous:** [JOURNEY-SCAN.md](JOURNEY-SCAN.md) — Find configuration issues

---

## See Also

- [TUI-SAVED-QUERIES.md](TUI-SAVED-QUERIES.md) — Saved query management
- [GLOSSARY-OF-CONCEPTS.md](GLOSSARY-OF-CONCEPTS.md) — Glossary of terms
- [ARCHITECTURE.md](ARCHITECTURE.md) — GSF format details
