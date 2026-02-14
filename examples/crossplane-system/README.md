# Crossplane System Ownership Example

## The Problem

Crossplane installs its own control-plane resources: Providers, ProviderRevisions,
Configurations, and CompositeResourceDefinitions. These don't have Flux or ArgoCD labels.

Without ownership detection, cub-scout would flag them as "orphans" — unmanaged resources
with no GitOps owner. That's a false alarm: Crossplane manages them itself.

**cub-scout recognizes Crossplane-managed resources:**

```
$ ./cub-scout map list -n crossplane-system

  STATUS  NAMESPACE          NAME                        OWNER       MANAGED-BY
  ✓       crossplane-system  provider-aws                Crossplane  system
  ✓       crossplane-system  provider-aws-1234abcd       Crossplane  system
  ✓       crossplane-system  platform-config             Crossplane  system
  ✓       crossplane-system  xpostgresqlinstances.db.x   Crossplane  system
```

No false orphans. No noise.

## What It Demonstrates

| What you'll see | Why it matters |
|-----------------|----------------|
| Crossplane resources detected as `owner=Crossplane` | No false orphan alerts |
| Sub-type `system` for control-plane resources | Distinguishes infra from app resources |
| Works alongside Flux/ArgoCD resources | Mixed clusters handled correctly |

## Resources in This Example

| Kind | Name | cub-scout Classification |
|------|------|--------------------------|
| Provider | `provider-aws` | Crossplane (system) |
| ProviderRevision | `provider-aws-1234abcd` | Crossplane (system) |
| Configuration | `platform-config` | Crossplane (system) |
| CompositeResourceDefinition | `xpostgresqlinstances.database.example.org` | Crossplane (system) |

## How Detection Works

cub-scout detects Crossplane ownership via:
- Resource API group: `pkg.crossplane.io/*` → Provider, Configuration
- Resource API group: `apiextensions.crossplane.io/*` → CompositeResourceDefinition
- Label: `crossplane.io/claim-name` → Crossplane-managed claims

This is *experimental* — the detection heuristics may evolve as Crossplane patterns mature.

## Quick Start

```bash
# Scan the fixture offline (no Crossplane CRDs needed)
./cub-scout scan --file examples/crossplane-system/crossplane-system.yaml

# On a cluster with Crossplane installed
./cub-scout map list -n crossplane-system
./cub-scout tree ownership  # Crossplane appears as its own category
```

## Offline Use

```bash
# No cluster or CRDs required
./cub-scout scan --file crossplane-system.yaml
```

## See Also

- [Platform Example](../platform-example/) — Mixed Flux + orphan ownership
- [Flux Boutique](../flux-boutique/) — Pure Flux ownership detection
