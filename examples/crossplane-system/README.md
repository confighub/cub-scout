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

## How cub-scout Sees It

```
./cub-scout tree ownership

OWNERSHIP HIERARCHY
════════════════════════════════════════════════════════════════════

Flux (28 resources)
────────────────────────────────────────────────────────────────────
  ├── boutique/cart              Deployment   ✓ 2/2
  └── ... (27 more)

Crossplane (4 resources)
────────────────────────────────────────────────────────────────────
  Managed by: pkg.crossplane.io/* and apiextensions.crossplane.io/* API groups

  ├── crossplane-system/provider-aws               Provider          ✓ Healthy
  ├── crossplane-system/provider-aws-1234abcd       ProviderRevision  ✓ Active
  ├── crossplane-system/platform-config             Configuration     ✓ Healthy
  └── crossplane-system/xpostgresqlinstances.db.x   XRD               ✓ Offered

Native (0 resources)
────────────────────────────────────────────────────────────────────
  No orphans detected ✓

════════════════════════════════════════════════════════════════════
Ownership Distribution:

  Flux         ████████████████████████████████████░░░░  88%
  Crossplane   █████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  12%
  Native       ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   0%

→ Without Crossplane detection, those 4 would show as orphans
```

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


## Conservative lineage control (2.14 candidate)

`lineage-ambiguity.yaml` is an authored control with a labeled child and
same-named parent candidates in two namespaces and two API groups. The label
names the composite but does not identify its GVK, scope or UID. Legal parent
candidates are same-namespace or cluster-scoped, following Kubernetes
[owner locality](https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/).
Crossplane v2 supports
[namespaced composites](https://docs.crossplane.io/v2.2/guides/upgrade-to-crossplane-v2/);
this correction does not establish complete v2 support.

The resolver refuses multiple eligible parent or claim candidates instead of
choosing by input order. Unknown composite type stays `CompositeResource` with
`present: false` and `xr:ambiguous` evidence; claim ambiguity stays `Claim` with
`claim:ambiguous`. Foreign-namespace/missing parents remain unresolved with `xr:unresolved`. The
target itself is never joined as its own XR or Claim. Known-group
owner references match exact API version/Kind/name, a legal namespace and UID
when supplied. A nonempty supplied UID without an observed match is not a present
parent and is marked `xr:owner_uid_not_observed`. When no UID is supplied and no
eligible parent is found, evidence is `xr:unresolved`. Label-only joins remain name-based evidence,
without a UID identity guarantee. The same resolved XR object supplies claim
labels; no second weaker lookup enriches it.

Run the pure fixture controls without a cluster:

```bash
GOPROXY=off GOTOOLCHAIN=local go test ./pkg/agent -run '^TestCrossplaneLineage(Label|OwnerRef|Claim|Whole)' -count=1
GOPROXY=off GOTOOLCHAIN=local go test ./cmd/cub-scout -run '^TestCrossplaneAmbiguitySharedTraceRendering$' -count=1
GOPROXY=off GOTOOLCHAIN=local go test ./test/unit -run '^TestResolveCrossplaneLineage$' -count=1
```

The five resolver controls reverse input order and cover locality, conflicting
types/scope, duplicate candidates, UID replacement, ambiguous claims and whole-inventory self-join refusal. A sixth
control loads this example into the actual CLI reverse-trace renderer, a TUI trace
pane loaded with that rendered text, and composition index and verifies partial lineage without guessed parent
type. Existing cluster XR/legacy claim fixtures remain regression controls.
No new API/discovery calls, readiness verdicts or object/fleet joins are added.
Missing/partial inventory remains partial evidence rather than an orphan finding.
The loaded TUI rendering control does not establish automatic lineage collection
or reverse-trace routing for a TUI action. This is offline source/renderer coverage,
not genuine live CLI/TUI acceptance,
Crossplane graduation or completion of #601/#594.
