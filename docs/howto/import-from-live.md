# Import from Live Cluster

Discover workloads from a running cluster and propose an App structure for ConfigHub — without needing Git access.

## When to Use This

- You have workloads running and want to understand what's there
- Git repos aren't organized or immediately available
- You want a quick read-only assessment before planning a migration
- You're onboarding an existing cluster into ConfigHub

## How It Works

cub-scout reads the cluster (Deployments, StatefulSets, DaemonSets), detects who manages each workload, and proposes how to organize them as Apps in ConfigHub.

It reads labels and annotations — it never modifies anything.

## Quick Start

```bash
# See everything (all namespaces, dry-run)
./cub-scout import --dry-run

# Focus on one namespace
./cub-scout import -n payment-prod --dry-run

# Get structured JSON output
./cub-scout import --dry-run --json
```

## What cub-scout Detects

| Owner | How It's Detected |
|-------|-------------------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| **ArgoCD** | `argocd.argoproj.io/instance` label or tracking-id annotation |
| **Helm** | `app.kubernetes.io/managed-by: Helm` |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | None of the above — unmanaged |

## What cub-scout Proposes

Given the detected workloads, cub-scout suggests:

- **App Space** — a team workspace name (inferred from namespace patterns)
- **Units** — one per workload variant, with `app` and `variant` labels
- **Variant** — inferred from GitOps paths (`envs/prod` → `prod`) or namespace patterns (`myapp-prod` → `prod`)

The proposal uses the current ConfigHub API (Space/Unit). In practice, think of it as:

| cub-scout proposes | You should read as |
|--------------------|--------------------|
| App Space: `myapp-team` | Team workspace for the myapp service |
| Unit: `api-prod` | The api component, production deployment |
| Unit: `api-dev` | The api component, dev deployment |

## Variant Inference Priority

cub-scout infers the environment variant in this order:

1. **Flux Kustomization path** — `spec.path: ./overlays/prod` → `prod`
2. **ArgoCD Application path** — `spec.source.path: envs/staging` → `staging`
3. **Kubernetes labels** — `app.kubernetes.io/instance: api-prod` → `prod`
4. **Namespace pattern** — `myapp-staging` → `staging`
5. **Workload name** — fallback

## What Happens Next

cub-scout's job stops at the proposal. It discovers and explains — it doesn't create anything in ConfigHub.

The next steps are ConfigHub's responsibility:
- Create Units from the proposal (via `cub unit create` or the ConfigHub UI)
- Set up bridge workers and targets
- Connect the OCI pipeline (ConfigHub renders → Flux/Argo deploys)

See [Import to ConfigHub](import-to-confighub.md) for the full migration path.

## Worked Example

See [examples/import-from-live/](../../examples/import-from-live/) for a complete worked example with fixture manifests and expected JSON output.

## Related Docs

- [Import to ConfigHub](import-to-confighub.md) — Full migration path (includes ConfigHub steps)
- [ConfigHub Glossary](../reference/glossary.md) — Terminology reference
- [Hub/AppSpace Examples](../reference/hub-appspace-examples.md) — Real-world mapping patterns
