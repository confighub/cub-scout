# OCI in ConfigHub

When and how OCI (Open Container Initiative) artifacts are used.

---

## What OCI Artifacts Are

OCI artifacts are container-registry-stored blobs. Originally for container images, now used for:

- **Helm charts** (`oci://registry.example.com/charts/myapp`)
- **Kustomize bases** (`oci://registry.example.com/kustomize/platform`)
- **ConfigHub Units** (WET manifests)

**Key benefit:** Same distribution infrastructure as container images. Same auth, same caching, same replication.

---

## Where OCI Fits in ConfigHub

### Three Deployment Paths

```
ConfigHub Unit (WET)
        │
        ├───→ Path A: Worker deploys directly to cluster
        │
        ├───→ Path B: Push to OCI → Flux/Argo deploys from OCI
        │
        └───→ Path C: Sync-back to Git → Flux/Argo deploys from Git
```

**OCI is Path B** — ConfigHub pushes WET manifests to OCI registry, your existing GitOps tool deploys from there.

---

## When to Use OCI

| Scenario | Use OCI? | Why |
|----------|----------|-----|
| Already using Flux/Argo | Yes | Keep existing GitOps tool, add ConfigHub for visibility |
| New deployment, no GitOps tool | No | Worker deploys directly |
| Git repo is source of truth | No | Use sync-back to Git (Path C) |
| Air-gapped environments | Yes | OCI registries work offline |
| Need artifact signing | Yes | cosign/notation work with OCI |

---

## OCI as Source

ConfigHub can **pull** from OCI sources:

```yaml
Hub: acme-platform
  sources:
    - name: platform-charts
      type: oci
      url: oci://ghcr.io/acme/platform-charts
      auth: ghcr-creds

    - name: kustomize-bases
      type: oci
      url: oci://ecr.aws/123456789/kustomize
      auth: ecr-creds
```

**Flow:**
```
OCI Registry (Helm charts, Kustomize)
        │
        ↓ pull
ConfigHub renders DRY → WET
        │
        ↓ store
Unit in App Space
```

---

## OCI as Target

ConfigHub can **push** WET manifests to OCI:

```yaml
App Space: payments-team
  deployer: flux-oci

  oci_target:
    registry: oci://ghcr.io/acme/rendered
    path: "{{ .AppSpace.Name }}/{{ .Unit.Labels.app }}/{{ .Unit.Labels.variant }}"
```

**Flow:**
```
Unit in App Space (WET)
        │
        ↓ push
OCI Registry
        │
        ↓ Flux/Argo pull
Cluster
```

**Result:** `oci://ghcr.io/acme/rendered/payments-team/payment-api/prod`

---

## Flux with OCI

### Traditional Flux (Git source)

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: GitRepository
metadata:
  name: my-app
spec:
  url: https://github.com/org/app
  ref:
    branch: main
```

### Flux with OCI (ConfigHub as source)

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
metadata:
  name: payment-api-prod
spec:
  url: oci://ghcr.io/acme/rendered/payments-team/payment-api/prod
  ref:
    tag: latest  # or semver
  interval: 5m
```

**Benefit:** Flux deploys WET manifests. No rendering at deploy time.

---

## Argo CD with OCI

### Argo Application with OCI

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: payment-api-prod
spec:
  source:
    repoURL: ghcr.io/acme/rendered
    path: payments-team/payment-api/prod
    targetRevision: v1.2.3
  destination:
    server: https://kubernetes.default.svc
    namespace: payments
```

**Benefit:** Argo deploys from OCI, ConfigHub handles rendering.

---

## OCI Authentication

### Registry Credentials in Hub

```yaml
Hub: acme-platform
  credentials:
    - name: ghcr-creds
      type: oci
      registry: ghcr.io
      username: ${GITHUB_USER}
      password: ${GITHUB_TOKEN}

    - name: ecr-creds
      type: oci
      registry: 123456789.dkr.ecr.us-east-1.amazonaws.com
      aws_role: arn:aws:iam::123456789:role/configub-ecr
```

### Workers Use Hub Credentials

Workers automatically use Hub credentials to push/pull OCI artifacts.

---

## OCI vs Git Sync-Back

| Aspect | OCI | Git Sync-Back |
|--------|-----|---------------|
| **Speed** | Faster (no commit cycle) | Slower (commit + push + webhook) |
| **Audit** | Registry logs | Git history |
| **Review** | No PR workflow | PR workflow possible |
| **Signing** | cosign/notation | Git commit signing |
| **Air-gap** | Works offline (mirror registry) | Needs Git access |

**Choose OCI when:** Speed matters, you already use OCI for Helm, air-gapped environments.

**Choose Git when:** Audit trail in Git matters, you want PR review workflow.

---

## OCI Artifact Structure

ConfigHub stores WET manifests as OCI artifacts:

```
oci://ghcr.io/acme/rendered/
├── payments-team/
│   ├── payment-api/
│   │   ├── dev/           ← variant
│   │   │   ├── :latest    ← always latest
│   │   │   ├── :v1.2.3    ← specific version
│   │   │   └── :rev-127   ← ConfigHub revision
│   │   ├── staging/
│   │   └── prod/
│   └── payment-worker/
│       └── ...
└── orders-team/
    └── ...
```

**Tags:**
- `:latest` — Always points to newest
- `:v1.2.3` — Semantic version (if configured)
- `:rev-127` — ConfigHub revision number

---

## Example: Full OCI Flow

```
┌─────────────────┐
│  Git (DRY)      │  ← Helm chart / Kustomize base
│  Source         │
└────────┬────────┘
         │
         ↓ ConfigHub pulls and renders
┌─────────────────────────────────────────┐
│              ConfigHub                   │
│                                          │
│  Unit: payment-api (variant=prod)        │
│  WET manifests stored                    │
└────────┬────────────────────────────────┘
         │
         ↓ Push to OCI
┌─────────────────┐
│  OCI Registry   │  oci://ghcr.io/acme/rendered/...
│  (WET)          │
└────────┬────────┘
         │
         ↓ Flux/Argo pulls
┌─────────────────┐
│    Cluster      │  ← Deploys WET manifests
└─────────────────┘
```

---

## See Also

- [05-THREE-SOURCES-OF-TRUTH.md](05-THREE-SOURCES-OF-TRUTH.md) — Git, ConfigHub, Cluster
- [06-MERGES-AND-WRITE-FLOWS.md](06-MERGES-AND-WRITE-FLOWS.md) — All deployment paths
- [02-HUB-APPSPACE-MODEL.md](02-HUB-APPSPACE-MODEL.md) — Where sources are registered

---

**Next:** [11-FEATURE-MATRIX.md](11-FEATURE-MATRIX.md) — 41 concepts GitOps leaves implicit

Return to [README.md](README.md) for the full index.
