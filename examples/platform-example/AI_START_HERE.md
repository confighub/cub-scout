# AI Start Here

Use this page when you want to drive `platform-example` safely with
Codex, Claude, Cursor, or another AI assistant.

## What This Example Is For

This is a realistic GitOps learning environment combining Flux-managed
workloads (podinfo) with orphan "shadow IT" resources. It requires a
live kind cluster.

It teaches: ownership detection, trace chains, orphan identification,
multi-layer Kustomize, and the "clobbering" problem.

## Read-Only First

Before creating a cluster, review what will happen:

```bash
cd examples/platform-example
cat setup.sh
cat orphans.yaml
```

`setup.sh` creates a kind cluster, installs Flux, creates a podinfo
GitRepository + Kustomization, and applies orphan fixtures.

## Recommended Path

```bash
# Create cluster and deploy everything
./setup.sh

# Explore with cub-scout
../../cub-scout map list
../../cub-scout map orphans
../../cub-scout trace deploy/podinfo -n podinfo
../../cub-scout gitops status

# When done
./cleanup.sh
```

## Important Boundaries

- `./setup.sh` creates a kind cluster named `platform-demo` and installs Flux
- `./setup.sh` applies orphan fixtures from `orphans.yaml`
- `./cleanup.sh` deletes the kind cluster
- All cub-scout commands are read-only — they never modify cluster state
- This example does not connect to ConfigHub

## What To Verify

After setup completes:

Cluster side:

```bash
kubectl get pods -n flux-system        # Flux controllers running
kubectl get kustomizations -A          # podinfo Kustomization reconciled
kubectl get deploy -n podinfo          # podinfo deployment ready
kubectl get deploy debug-nginx         # orphan resource present
```

cub-scout side:

```bash
../../cub-scout map list               # ownership classification
../../cub-scout map orphans            # Native resources without GitOps labels
../../cub-scout trace deploy/podinfo -n podinfo  # Git → Flux → Deployment chain
../../cub-scout gitops status          # pipeline health
```

## Learning Journeys

| Journey | Command | What it shows |
|---------|---------|---------------|
| What's in my cluster? | `map` (TUI) or `map list` | Ownership classification |
| Where did this come from? | `trace deploy/podinfo -n podinfo` | GitRepository → Kustomization → Deployment |
| What's NOT in Git? | `map orphans` | Resources without GitOps ownership |
| What would change? | `trace deploy/podinfo -n podinfo --diff` | Live vs desired state |

## Related Files

- [README.md](./README.md)
- [prompts.md](./prompts.md)
- [contracts.md](./contracts.md)
- [setup.sh](./setup.sh)
- [cleanup.sh](./cleanup.sh)
