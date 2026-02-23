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

Given the detected workloads, cub-scout suggests an App structure:

- **App** — the logical application (inferred from namespace/workload patterns)
- **Components** — individual workload pieces (api, worker, db) with `app` and `variant` labels
- **Variant** — inferred from GitOps paths (`envs/prod` → `prod`) or namespace patterns (`myapp-prod` → `prod`)

> **API note:** The proposal currently uses Space/Unit terminology in CLI output.
> Read "Space" as where the App lives, "Unit" as a component of the App.

| cub-scout proposes | You should read as |
|--------------------|--------------------|
| Space: `myapp-team` | Team workspace for the myapp service |
| Unit: `api-prod` | The api component, production Deployment |
| Unit: `api-dev` | The api component, dev Deployment |

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
- Create the App and its Deployments from the proposal
- Set up bridge workers and Targets
- Connect the OCI pipeline (ConfigHub renders → Flux/Argo deploys)

See [Import to ConfigHub](import-to-confighub.md) for the full migration path, or [Migration Playbook](migration-playbook.md) for the comprehensive guide.

## Worked Example

See [examples/import-from-live/](../../examples/import-from-live/) for a complete worked example with fixture manifests and expected JSON output.

## Related Docs

- [Migration Playbook](migration-playbook.md) — Comprehensive guide with assessment, planning, validation, and rollback
- [Import to ConfigHub](import-to-confighub.md) — Full migration path (includes ConfigHub steps)
- [ConfigHub Glossary](../reference/glossary.md) — Terminology reference
