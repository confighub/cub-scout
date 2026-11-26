# cub-scout Bot Example

Run `cub-scout` continuously inside Kubernetes as a strictly read-only observation bot.

The bot is the deployable form of `cub-scout watch`: it runs the same deterministic
observation engine, event types, queueing, and optional receipt emission, reading
configuration from `CUB_SCOUT_BOT_*` environment variables and using in-cluster
Kubernetes ServiceAccount authentication.

## What This Deploy Kit Includes

- [`rbac-readonly.yaml`](rbac-readonly.yaml): `Namespace`, `ServiceAccount`, `ClusterRole` with strictly `get`, `list`, and `watch` verbs only (zero write verbs), and `ClusterRoleBinding`.
- [`deployment.yaml`](deployment.yaml): Hardened non-root `Deployment` (`runAsNonRoot: true`, `readOnlyRootFilesystem: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: ["ALL"]`, `seccompProfile: RuntimeDefault`) pinned to `ghcr.io/confighub/cub-scout:v3.0.0`, running `cub-scout bot --watch-backed`.
- [`build-from-release.sh`](build-from-release.sh): Checksum-verified local image builder for `linux/amd64` and `linux/arm64` if your cluster cannot pull from GHCR.

## 30-Second Deploy & Verification

1. Apply the strictly read-only RBAC manifest:

```bash
kubectl apply -f examples/bot/rbac-readonly.yaml
```

2. Create the webhook destination Secret:

```bash
kubectl -n cub-scout create secret generic cub-scout-bot-webhook \
  --from-literal=url='https://events.example.com/cub-scout'
```

3. Deploy the hardened bot Deployment:

```bash
kubectl apply -f examples/bot/deployment.yaml
```

4. Verify rollout and inspect bot logs:

```bash
kubectl -n cub-scout rollout status deploy/cub-scout-bot
kubectl -n cub-scout logs deploy/cub-scout-bot
```

5. Verify the ServiceAccount has zero write access:

```bash
kubectl auth can-i --as=system:serviceaccount:cub-scout:cub-scout-bot get deployments -A   # yes
kubectl auth can-i --as=system:serviceaccount:cub-scout:cub-scout-bot patch deployments -A # no
kubectl auth can-i --as=system:serviceaccount:cub-scout:cub-scout-bot delete pods -A       # no
```

## Configuration

| Environment Variable | Default in Manifest | Purpose |
|---|---|---|
| `CUB_SCOUT_BOT_WEBHOOK_URL` | Secret `cub-scout-bot-webhook/url` | Webhook URL receiving JSON events (required unless `CUB_SCOUT_BOT_OUTPUT_FILE` is set) |
| `CUB_SCOUT_BOT_INTERVAL` | `30s` | Observation cycle interval (Go duration) |
| `CUB_SCOUT_BOT_NAMESPACE` | `""` (all namespaces) | Scope observation to a single namespace (for example `prod`) |
| `CUB_SCOUT_BOT_SEVERITY` | `warning,critical` | Filter finding/drift events by severity |
| `CUB_SCOUT_BOT_MAX_QUEUED_EVENTS` | `1000` | Maximum buffered events when the webhook is unavailable |
| `CUB_SCOUT_BOT_EMIT_RECEIPT_ON` | `all` | Event types that attach an inline receipt |
| `CUB_SCOUT_BOT_EMIT_RECEIPT_BATCH_CAP` | `10` | Per-cycle cap on receipt-build attempts |

The Deployment passes `args: ["bot", "--watch-backed"]` to the container entrypoint
(`/cub-scout`), backing inventory and state-scan reads with Kubernetes watch informers
so idle cycles read from cache instead of re-listing the API server.

The RBAC is intentionally read-only. If you remove access to a resource type,
`cub-scout` omits that evidence and reports partial observations rather than
mutating the cluster.

## Event Types & Standard `answer` Envelope

The bot emits the same event types as `cub-scout watch`:

- `resource.discovered`
- `ownership.changed`
- `drift.detected`
- `scan.finding`

Each emitted event carries the standard read-only `answer` envelope (`#1025`),
reusing the approved `answer.state` vocabulary (`issues-flagged`, `nothing-flagged`,
`incomplete`, `not-observable`) with quoted cluster text in `basis[].quoted` and
`unknowns[].quoted`:

```json
{
  "timestamp": "2026-11-26T00:00:00Z",
  "type": "scan.finding",
  "severity": "warning",
  "resource": {
    "kind": "Deployment",
    "namespace": "prod",
    "name": "payments",
    "owner": "Flux"
  },
  "details": {
    "message": "container runs as root"
  },
  "answer": {
    "cluster": {
      "context": "in-cluster",
      "idSource": "not-read"
    },
    "observedAt": "2026-11-26T00:00:00Z",
    "state": "issues-flagged",
    "reason": "finding-flagged",
    "summary": "Watch flagged a scan finding on Deployment/payments in prod.",
    "basis": [
      {
        "claim": "state scan reported a finding",
        "restsOn": "cub-scout-comparison",
        "ref": "Deployment/prod/payments",
        "quoted": "container runs as root"
      }
    ],
    "unknowns": [],
    "next": [
      {
        "command": "cub-scout explain deploy/payments -n prod",
        "why": "inspect owner and recent field managers"
      }
    ],
    "cost": {
      "coverage": "not-measured"
    }
  }
}
```

## Build a Local Image From a Release

If your cluster cannot pull `ghcr.io/confighub/cub-scout` directly (for example in an
air-gapped environment or before a release tag is published), you can build a local
image from a published GitHub release archive. The helper requires Bash, `curl`, `tar`,
`awk`, `sha256sum` or `shasum`, and Docker with BuildKit/`--load` support. It needs
network access to GitHub release assets and the runtime base image, but not GHCR access
or ConfigHub authentication.

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
before build, custom image names, failed download/build cleanup, and exact
image/platform arguments. Runtime-image proof is
separate from an in-cluster observation/sink smoke test or public pull access.

### Recorded Runtime Proof

On 2026-09-11, the helper built local Linux arm64 and amd64 images from the
published v2.10.1 archives. Both images reported
`2.10.1 (built 2026-09-11T17:07:18Z)` under the read-only filesystem,
dropped-capability and no-new-privileges options above. Image inspection showed
the requested architecture, `/cub-scout` entrypoint and `65532:65532` user.
The amd64 run used the local arm64 host's emulation, not an amd64 Kubernetes node.

Verified archive SHA-256 values:

| Archive | SHA-256 |
|---|---|
| `cub-scout_2.10.1_linux_arm64.tar.gz` | `37513aad6e39527e495b5ce39a64977b2b9bb49a127c2723deaa032402ad2890` |
| `cub-scout_2.10.1_linux_amd64.tar.gz` | `6ebddf50abcd3ad7aeb8a1652b2eea59eb165b9f06c2a7d40a1c16815f9e07d1` |

This proves the fallback's download/build/runtime path for those archives.
No cluster manifests were applied, no images pushed, and no registry settings,
credentials or installed plugins changed.
