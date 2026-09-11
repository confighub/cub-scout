# cub-scout Bot Example

Run cub-scout continuously inside Kubernetes as a read-only observation bot.

The bot is the deployable form of `cub-scout watch`: it uses the same polling
engine, event types, queueing, and optional receipt emission, but reads
configuration from `CUB_SCOUT_BOT_*` environment variables and uses in-cluster
Kubernetes authentication.

## What This Covers

- `cub-scout bot` as an in-cluster observer
- read-only ServiceAccount, ClusterRole, and ClusterRoleBinding
- webhook delivery using `CUB_SCOUT_BOT_WEBHOOK_URL`
- warning/critical filtering and bounded receipt-build backpressure
- numeric nonroot image identity compatible with `runAsNonRoot` (fixed in v2.9.0)

## Quick Run

Choose a usable image first. Anonymous pulls from the documented registry are
currently unverified/failing (#520); the default manifest pins v2.10.1 but does
not fix access. The published registry image is amd64-only. The local-build
path below supports either Linux amd64 or arm64 without pulling that image.

Create the webhook Secret:

```bash
kubectl create namespace cub-scout
kubectl -n cub-scout create secret generic cub-scout-bot-webhook \
  --from-literal=url='https://events.example.com/cub-scout'
```

Apply the bot manifest:

```bash
kubectl apply -f examples/bot/deployment.yaml
```

Inspect the Pod:

```bash
kubectl -n cub-scout logs deploy/cub-scout-bot
```

## Build a Local Image From a Release

The helper is an unreleased installation addition for v2.11 and can package
the existing v2.10.1 release. It requires Bash, curl, tar, awk, sha256sum or
shasum, and Docker with BuildKit/`--load` support. It needs network access to
GitHub release assets and the runtime base image, but not GHCR access or
ConfigHub authentication.

Select the architecture of the **target Kubernetes nodes**, not necessarily
your laptop. For arm64:

```sh
bash examples/bot/build-from-release.sh v2.10.1 arm64
docker run --rm --platform linux/arm64 --read-only --cap-drop ALL \
  --security-opt no-new-privileges cub-scout-bot:v2.10.1-arm64 version
```

For amd64, use `amd64` in both commands. An optional third argument sets the
local image name. The helper downloads the exact Linux archive and release
checksum manifest, rejects missing/duplicate/mismatched checksums, extracts
only the binary, and uses this checkout's existing numeric-nonroot Dockerfile.
It removes its temporary build context on success or failure. No image is
pushed, kubeconfig read, registry permission changed, or cluster mutated.

Checksum verification establishes agreement with the published checksum
manifest, not a signature or bit-for-bit reproducibility of the runtime image:
the Dockerfile's base image may change independently of the Scout binary.

For a local kind cluster, load the built image into your chosen cluster:

```sh
kind load docker-image cub-scout-bot:v2.10.1-arm64 --name <cluster-name>
```

Set the Deployment's `containers[].image` in `deployment.yaml` to
`cub-scout-bot:v2.10.1-arm64` (or the amd64 tag), retaining
`imagePullPolicy: IfNotPresent`, before applying it. For other clusters, use
your normal image distribution process and set the corresponding image name.
Image loading/pushing and applying the manifests are separate operator actions;
the helper never performs them or provisions webhook credentials.

Offline helper tests: `go test ./test/unit -run '^TestBotBuildFromRelease$' -count=2`.
They use synthetic archives and fake curl/docker tools, including rejection
before build and exact image/platform arguments. Runtime-image proof is
separate from an in-cluster observation/sink smoke test or public pull access.

## Scope Tuning

The default manifest observes all namespaces. To scope it down, set:

```yaml
- name: CUB_SCOUT_BOT_NAMESPACE
  value: prod
```

The RBAC is intentionally read-only. If you remove access to a resource type,
cub-scout will omit that evidence or report partial observations rather than
mutating the cluster.

## Event Types

The bot emits the same event types as `watch`:

- `resource.discovered`
- `ownership.changed`
- `drift.detected`
- `scan.finding`

Use `CUB_SCOUT_BOT_EMIT_RECEIPT_ON=all` to attach inline receipts where
supported. `CUB_SCOUT_BOT_EMIT_RECEIPT_BATCH_CAP` bounds receipt-building per
poll so bursts do not turn into unbounded API pressure.
