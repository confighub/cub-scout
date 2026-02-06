# Crossplane Walkthrough

This guide shows how cub-scout works with Crossplane-managed resources.

**Time:** ~10 minutes

---

## What You'll Learn

1. **Trace** — Follow the chain from Managed Resource → XR → Claim
2. **Tree** — See XR-grouped composition views
3. **Map** — See Crossplane resources counted distinctly (no false orphans)

---

## Prerequisites

- A cluster with Crossplane installed
- At least one Composite Resource (XR) with managed resources
- cub-scout installed (`brew install confighub/tap/cub-scout`)

---

## 1. Trace: Following the Lineage Chain

Crossplane creates a hierarchy:

```
Claim (user intent)
  └── Composite Resource (XR)
        └── Managed Resource (cloud resource)
```

### Example: Trace a Managed Resource

```bash
./cub-scout trace instance/staging-db -n default
```

**Output:**
```
REVERSE TRACE: Instance/staging-db in default

Instance/staging-db
  Owner: Crossplane
  Detected via: label:crossplane.io/composite

Crossplane lineage:
  managed:   Instance/staging-db
  xr:        XPostgreSQLInstance/xpostgresqlinstance-abc123
  claim:     PostgreSQLClaim/ecommerce-db (ecommerce)
  evidence:  label:crossplane.io/composite, label:crossplane.io/claim-*
```

### What This Shows

| Field | Meaning |
|-------|---------|
| `managed` | The cloud resource (RDS Instance, S3 Bucket, etc.) |
| `xr` | The Composite Resource that owns the managed resource |
| `claim` | The user-facing Claim that requested the XR |
| `evidence` | How cub-scout found these relationships |

### Partial Lineage

If the XR or Claim object isn't in the cluster (e.g., different namespace permissions), cub-scout shows partial lineage:

```
Crossplane lineage:
  managed:   Instance/staging-db
  xr:        XPostgreSQLInstance/xpostgresqlinstance-abc123 (partial lineage)
  evidence:  label:crossplane.io/composite
```

This means the relationship is known but the referenced object wasn't found.

---

## 2. Tree Composition: XR-Grouped View

See all managed resources grouped by their Composite Resource:

```bash
./cub-scout tree composition
```

**Output:**
```
CROSSPLANE COMPOSITION TREE
════════════════════════════════════════════════════════════════════

XPostgreSQLInstance/xpostgresqlinstance-abc123
  Claim: PostgreSQLClaim/ecommerce-db (ecommerce)
  ├── Instance/staging-db
  ├── SecurityGroup/staging-db-sg
  ├── SubnetGroup/staging-db-subnets
  └── ParameterGroup/staging-db-params

XPostgreSQLInstance/xpostgresqlinstance-def456
  Claim: PostgreSQLClaim/analytics-db (analytics)
  ├── Instance/analytics-db
  ├── SecurityGroup/analytics-db-sg
  └── SubnetGroup/analytics-db-subnets

────────────────────────────────────────────────────────────────────
Summary: 2 XRs, 7 managed resources
```

### What This Shows

- Each XR with its composed managed resources
- The Claim that requested each XR (if present)
- A flat list of all resources under each composition

### JSON Output

For scripting:

```bash
./cub-scout tree composition --json
```

```json
{
  "compositions": [
    {
      "xr": {
        "ref": {"kind": "XPostgreSQLInstance", "name": "xpostgresqlinstance-abc123"},
        "present": true
      },
      "claim": {
        "ref": {"kind": "PostgreSQLClaim", "name": "ecommerce-db", "namespace": "ecommerce"},
        "present": true
      },
      "managed": [
        {"ref": {"kind": "Instance", "name": "staging-db"}, "present": true},
        {"ref": {"kind": "SecurityGroup", "name": "staging-db-sg"}, "present": true}
      ]
    }
  ]
}
```

---

## 3. Map: Crossplane in the Ownership View

```bash
./cub-scout map list
```

**Output:**
```
NAMESPACE   KIND            NAME                    OWNER
default     Instance        staging-db              Crossplane
default     SecurityGroup   staging-db-sg           Crossplane
default     SubnetGroup     staging-db-subnets      Crossplane
ecommerce   PostgreSQLClaim ecommerce-db            Crossplane
...

────────────────────────────────────────────────────────────────────
Summary: 47 resources
  Flux(28) Crossplane(12) ArgoCD(5) Native(2)
```

### Key Points

1. **Crossplane resources are counted distinctly** — They appear under "Crossplane", not "Native"
2. **No false orphans** — Managed resources with `crossplane.io/composite` labels are correctly attributed
3. **Claims included** — Both managed resources and Claims show as Crossplane-owned

### Filter to Crossplane Only

```bash
./cub-scout map list --owner Crossplane
```

---

## How Ownership Is Detected

cub-scout detects Crossplane ownership via:

| Signal | Priority |
|--------|----------|
| `crossplane.io/composite` label | Primary |
| `crossplane.io/claim-name` label | Enrichment |
| OwnerReference to `*.crossplane.io` | Fallback |
| OwnerReference to `*.upbound.io` | Fallback |

See [reference/resolver-pattern.md](../reference/resolver-pattern.md) for implementation details.

---

## Common Scenarios

### Scenario: "Why is my managed resource unhealthy?"

```bash
./cub-scout trace instance/staging-db -n default
```

This shows:
- The XR that owns it (check XR status)
- The Claim that requested it (check Claim events)
- Evidence of how the lineage was resolved

### Scenario: "What resources does this XR compose?"

```bash
./cub-scout tree composition --json | jq '.compositions[] | select(.xr.ref.name == "xpostgresqlinstance-abc123")'
```

### Scenario: "Find all Crossplane-managed resources"

```bash
./cub-scout map list --owner Crossplane --json
```

---

## Troubleshooting

### "Partial lineage" shown

The XR or Claim object wasn't found. This can happen if:
- The XR is in a different namespace you don't have access to
- The Claim was deleted but managed resources remain
- RBAC restricts access to XR/Claim resources

cub-scout still shows the relationship via labels.

### Managed resource shows as "Native"

Check if the resource has Crossplane labels:

```bash
kubectl get instance/staging-db -o jsonpath='{.metadata.labels}'
```

Expected: `crossplane.io/composite` label present

### XR not showing in tree

The `tree composition` command only shows XRs that have at least one managed resource with the composite label. Orphaned XRs or XRs with no composed resources won't appear.

---

## Next Steps

- [Resolver Pattern](../reference/resolver-pattern.md) — How lineage resolution works internally
- [Ownership Detection](ownership-detection.md) — How cub-scout detects all owner types
- [Tree Hierarchies](tree-hierarchies.md) — Other tree views (runtime, git, config)
