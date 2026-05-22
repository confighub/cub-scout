# Reference: Flux source types

cub-scout's coverage of the **Flux source-controller** family — the five CRDs that produce content for Flux's other controllers (kustomize-controller, helm-controller) to apply. This reference documents each source kind's shape, the field-manager identity, the trace path back from a workload, and the source-truth strategy mapping.

Source of truth: `pkg/agent/flux_trace.go` (the tracer), `pkg/agent/manager_strings.go` `ManagerFluxSource`, and the Flux Operator Interop Slice (`v0.20.0` per the roadmap).

## The five source kinds

| Kind | API group | Provides | Anchor for source-truth |
|---|---|---|---|
| `GitRepository` | `source.toolkit.fluxcd.io/v1` | Cloned Git repo at a tag/branch/SHA | Git SHA (most common) |
| `OCIRepository` | `source.toolkit.fluxcd.io/v1beta2` | OCI artifact (any registry — could be ConfigHub-rendered) | OCI digest |
| `HelmRepository` | `source.toolkit.fluxcd.io/v1` | Index of Helm charts at a HTTP/OCI source | Resolved chart version |
| `Bucket` | `source.toolkit.fluxcd.io/v1` | S3 / GCS / generic blob store | Revision (provider-specific) |
| `HelmChart` | `source.toolkit.fluxcd.io/v1` | A specific chart from a HelmRepository (usually managed by helm-controller internally; rarely user-authored) | Chart version |

All five expose a common contract on their `status`:

| `status` field | Meaning |
|---|---|
| `artifact.url` | Where Flux cached the fetched artifact (local cluster URL) |
| `artifact.revision` | The version Flux observed (SHA / digest / chart-version) |
| `artifact.lastUpdateTime` | When Flux last refreshed the artifact |
| `conditions[]` | `Ready` / `Reconciling` / `Failed` |

cub-scout's tracer reads `artifact.revision` as the anchor when walking back from a workload. That's the value that goes into `gitSource.revision` on attribution evidence.

## Field-manager identity

All five source kinds are written by the **source-controller**:

```
manager: source-controller   (pkg/agent/manager_strings.go ManagerFluxSource)
operation: Apply
fieldsType: FieldsV1
```

When cub-scout observes a `GitRepository` / `OCIRepository` / `HelmRepository` / `Bucket` / `HelmChart` resource directly, the writer is `source-controller`. cub-scout's `controllerManagersForOwner(OwnerFlux, ownerSubType)` narrows the match when `ownerSubType` is one of `source / gitrepository / ocirepository / helmrepository / bucket` — only `source-controller` counts as a controller co-signal for these. When subtype is unknown, all three Flux managers (kustomize / helm / source) are accepted.

## The two-stage delivery chain

Flux delivery is structurally **two stages**:

```
[Stage 1 — source-controller fetches]
GitRepository / OCIRepository / HelmRepository / Bucket
              ↓ (artifact cached, revision pinned)
              ↓ exposes status.artifact.url + .revision

[Stage 2 — kustomize-controller / helm-controller applies]
       Kustomization or HelmRelease
       (spec.sourceRef → the source from Stage 1)
              ↓ (applied to cluster)
        Deployment / Service / ConfigMap …
        (managedFields manager: kustomize-controller OR helm-controller)
```

cub-scout's `flux_trace.go` walks **backwards** from a workload:

1. Read the workload's labels → find the owning `Kustomization` or `HelmRelease` (via `kustomize.toolkit.fluxcd.io/name` / `helm.toolkit.fluxcd.io/name` labels)
2. Read that Kustomization/HelmRelease spec → extract `spec.sourceRef.{kind,name,namespace}`
3. Read the source CRD → resolve `status.artifact.revision` + `spec.{url,interval,ref,...}` → produce the `gitSource{repoUrl, revision, path}` evidence

The chain hop count is fixed at 2 for raw workloads, 1 for a Kustomization/HelmRelease, 0 for a source CRD itself.

## Source-truth strategy mapping

Each Flux strategy in the source-truth contract names a specific source type:

| Strategy (`pkg/agent/source_truth.go`) | Expected source kind | Anchor |
|---|---|---|
| `git-flux` | `GitRepository` | Git SHA equality |
| `helm-flux` | `HelmRepository` (consumed by a `HelmRelease`) | Helm chart version equality — runtime extractor is partial (`runtime.helm_chart_anchor` proof gap) |
| `kustomize-flux` | Any source kind (typically `GitRepository`, also valid against `OCIRepository`) | Git SHA equality (when source is Git); else revision-equal |
| `confighub-oci-flux` | `OCIRepository` pointing at a ConfigHub-rendered artifact | OCI digest — currently a proof gap (ConfigHub doesn't yet expose per-revision rendered digest) |
| `oci-flux` | `OCIRepository` (non-ConfigHub) | OCI digest equality |

`ExpectsArgoController()` on the strategy type returns `false` for these five; the source-truth derivation refuses to silently fall back across controllers, so cub-scout will produce a `BLOCK` if it observes Argo writers under a Flux strategy.

## Worked examples per source kind

### GitRepository → Kustomization → Deployment

```yaml
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: platform-prod
  namespace: flux-system
spec:
  url: https://github.com/org/platform-config
  ref: { branch: main }
status:
  artifact:
    revision: main@sha1:abc123def456
    url: http://source-controller.flux-system/gitrepository/flux-system/platform-prod/abc123def456.tar.gz
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: platform-prod
  namespace: flux-system
spec:
  sourceRef: { kind: GitRepository, name: platform-prod }
  path: ./clusters/prod
  prune: true
```

A `Deployment` produced by this chain has labels:

```
kustomize.toolkit.fluxcd.io/name: platform-prod
kustomize.toolkit.fluxcd.io/namespace: flux-system
```

cub-scout trace:

```
$ cub-scout trace deploy/api -n prod
Flux Kustomization platform-prod in flux-system
  Source:    GitRepository platform-prod in flux-system
    Revision: abc123def456 (status.artifact.revision)
    URL:      https://github.com/org/platform-config @main
    Path:     clusters/prod
```

### OCIRepository (ConfigHub-rendered)

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
spec:
  url: oci://ghcr.io/confighub-managed/payments-api
  ref: { tag: 42 }   # the ConfigHub unit revision number
status:
  artifact:
    revision: 42@sha256:0123abcd...
```

cub-scout recognizes the OCI source pattern and pairs it with the ConfigHub-side unit (via the `confighub.com/UnitSlug` label on the workload). Source-truth strategy: `confighub-oci-flux`. See [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md) for the dual-label case.

### HelmRepository → HelmRelease → Deployment

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
spec:
  url: https://charts.example.com
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
spec:
  chart:
    spec:
      chart: payments
      version: 1.2.3
      sourceRef: { kind: HelmRepository, name: charts }
```

A `Deployment` produced by this chain has labels:

```
helm.toolkit.fluxcd.io/name: payments-api
helm.toolkit.fluxcd.io/namespace: flux-system
app.kubernetes.io/managed-by: Helm        # the chart itself sets this
helm.sh/chart: payments-1.2.3
```

cub-scout reads this as `OwnerFlux` `SubType=helmrelease` (Flux labels take detection priority over the Helm labels) with `helm-controller` as the controller co-signal. See [`observe-helm`](../observe-helm/SKILL.md) for the three-ways-to-be-Helm disambiguation matrix.

### Bucket → Kustomization

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: Bucket
spec:
  provider: aws        # or "gcp", "generic"
  bucketName: my-flux-state
  region: us-east-2
```

Used the same way as `GitRepository` (consumed by a Kustomization). The `artifact.revision` is provider-specific (S3 object version, GCS generation number, etc.). cub-scout records it as the source revision but does not interpret the provider-specific format.

### HelmChart (internal)

Rarely user-authored. helm-controller creates `HelmChart` resources on-the-fly when it materializes a `HelmRelease`. cub-scout's tracer walks through the HelmChart silently — it's an implementation detail of the helm-controller, not part of the trace output.

## Multi-source-type tracer

The tracer in `pkg/agent/flux_trace.go` handles all five source kinds via a kind-dispatch switch:

```
switch sourceRef.Kind {
case "GitRepository":   resolve via GitRepository GVK
case "OCIRepository":   resolve via OCIRepository GVK
case "HelmRepository":  resolve via HelmRepository GVK
case "Bucket":          resolve via Bucket GVK
default:                produce a proof gap + best-effort
}
```

Unknown source kinds (a future Flux release adds a new source type, or a CRD-extension introduces a custom source kind) produce a structured proof gap rather than a panic.

The Flux Operator Interop Slice (`v0.20.0`) explicitly covers all five types — locked by test coverage in `pkg/agent/flux_trace_test.go`.

## Edge cases

### Source not yet `Ready`

A GitRepository whose initial reconcile hasn't completed has empty `status.artifact`. cub-scout records a proof gap (`source.revision` missing) and emits INCONCLUSIVE / WATCH depending on the verdict path. Never a panic.

### Cross-namespace source references

Flux supports cross-namespace `sourceRef`. cub-scout's tracer reads the namespace explicitly from the `sourceRef.namespace` field; it doesn't assume the source lives in the same namespace as the Kustomization.

### Source name collisions

Two GitRepositories with the same name in different namespaces are fine — Kubernetes' namespace scope handles it. cub-scout's trace output names the namespace explicitly to avoid ambiguity.

### Helm chart-version equality (Phase 2 gap)

The `helm-flux` and `helm-argo` source-truth strategies emit a `runtime.helm_chart_anchor` proof gap because the runtime chart-version extractor isn't fully wired. The chart version is readable from `app.kubernetes.io/version` or `helm.sh/chart` labels on the live resources, but the field-equality check is partial. Phase 3 work lands the full equality.

### `confighub-oci-flux` digest equality (still deferred)

ConfigHub does not yet expose a per-revision rendered OCI digest. cub-scout records this as a proof gap on `confighub-oci-flux` source-truth verdicts: status reflects what we *can* check; the per-field digest equality lands once the ConfigHub-side field exists. See the source-truth-strategies reference for the broader context.

## Skills that consume this reference

- [`observe-flux`](../observe-flux/SKILL.md) — the controller-observer skill for Flux
- [`scout-compare`](../scout-compare/SKILL.md) — `compare source-truth` strategy selection
- [`confighub-source-truth`](../confighub-source-truth/SKILL.md) — Flux strategies
- [`scout-ingest`](../scout-ingest/SKILL.md) — Flux import targets the same source/Kustomization shape
- [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md) — Flux as a delivery path for ConfigHub units

## References

- Code:
  - `pkg/agent/flux_trace.go` — the multi-source-type tracer
  - `pkg/agent/manager_strings.go` `ManagerFluxSource` — the source-controller manager identity
  - `pkg/agent/source_truth.go` — strategy definitions touching Flux sources
- Upstream:
  - [fluxcd/source-controller](https://github.com/fluxcd/source-controller) — the controller and its CRD definitions
  - [Flux source API v1 reference](https://fluxcd.io/flux/components/source/)
- Flux Operator Interop Slice: `v0.20.0` in `docs/roadmap.md`
- Examples: [`examples/flux-boutique/`](../../examples/flux-boutique/), [`examples/flux-import-confighub-demo/`](../../examples/flux-import-confighub-demo/)
- Related: `#427` — kstatus migration may flip `Ready=true → false` for stalled Flux workloads in v2.1.0 (not source-controller-specific, but adjacent)
