# Traefik Helm Worked Example: Base Unit + App Units with FluxCD and ArgoCD

Status: reference walkthrough (product semantics)  
Last updated: 2026-02-28

This is the Traefik/Helm equivalent of the dry/wet unit model used in the FluxCD
and ArgoCD rendered-pipeline examples.

It shows a practical pattern:

1. Platform team owns a Helm DRY base unit.
2. App teams own bounded DRY app units.
3. Renderer units produce WET deploy artifacts.
4. Flux/Argo reconcile unchanged.
5. Reusable app-level improvements can be promoted back into platform DRY.

---

## 1. What This Example Proves

For a Traefik-based ingress path, this walkthrough shows:

1. how upstream Helm chart defaults become a platform DRY base unit,
2. how app-specific ingress needs become app DRY units,
3. how renderer units compose `BaseUnit + AppUnit` into WET,
4. how Flux/Argo keep their normal reconcile role,
5. how app changes can be merge-approved and promoted upstream into base DRY.

No controller replacement is required.

---

## 2. Equivalent Mapping (Helm/Traefik vs Dry/Wet Model)

| Concept | Dry/Wet model | Traefik Helm equivalent |
|---|---|---|
| Shared platform intent | Platform DRY base | Base unit wrapping pinned Traefik chart/version |
| Team-specific intent | App DRY overlays | App units with bounded override fields |
| Render operation | Renderer unit | Helm template/render worker |
| Deployment contract | WET artifacts | Rendered manifests or chart+values artifact |
| Transport | OCI preferred | OCI artifact digest for controller pull |
| Reconciler | Flux/Argo | Flux/Argo (unchanged) |

---

## 3. DRY Inputs (Base Unit + App Unit)

### Platform-owned base unit (illustrative)

```yaml
kind: HelmBaseUnit
metadata:
  name: traefik-platform-base
spec:
  chart:
    repo: oci://ghcr.io/traefik/helm/traefik
    version: 29.0.0
  defaults:
    service:
      type: LoadBalancer
    ports:
      websecure:
        tls:
          enabled: true
    providers:
      kubernetesCRD:
        enabled: true
  guardrails:
    deny_override:
      - service.type
      - ports.websecure.tls.enabled
```

### App-owned unit (illustrative)

```yaml
kind: HelmAppUnit
metadata:
  name: checkout-edge-prod
spec:
  base_ref: traefik-platform-base
  target:
    app: checkout
    env: prod
  overrides:
    ingressRoute:
      enabled: true
      match: Host(`checkout.example.com`)
      middlewares:
        - checkout-rate-limit
    deployment:
      replicas: 3
```

This separation gives app autonomy without allowing silent guardrail bypass.

---

## 4. Step-by-Step Walkthrough

### Step 1: Platform defines Traefik base DRY

Platform team creates or updates `traefik-platform-base` (chart pin, default
values, non-overridable policy fields).

### Step 2: App team adds app DRY unit

App team proposes app-specific ingress behavior in `checkout-edge-prod`.

Entry can be either direction:

1. GitHub PR -> ConfigHub MR/card link, or
2. ConfigHub MR -> bot-created GitHub PR.

### Step 3: Renderer unit composes DRY -> WET and publishes OCI

Renderer unit binds:

1. base unit revision,
2. app unit revision,
3. chart/generator version,
4. input digest,
5. resulting WET artifact digest.

Expected output artifact (example):

```text
oci://ghcr.io/acme/platform-wet/traefik-checkout@sha256:ab44...771e
```

### Step 4: Keep controller wiring unchanged

Import/discover pipeline remains the same shape:

```bash
cub gitops discover --space "$SPACE" "$K8S_TARGET"
cub gitops import --space "$SPACE" "$K8S_TARGET" "$RENDERER_TARGET"
```

Flux and Argo still reconcile as before.

Flux-style OCI source (illustrative):

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: traefik-checkout-prod
  namespace: flux-system
spec:
  interval: 1m
  url: oci://ghcr.io/acme/platform-wet/traefik-checkout
  ref:
    digest: sha256:ab44...771e
```

Argo-style OCI source (illustrative):

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: traefik-checkout-prod
  namespace: argocd
spec:
  source:
    repoURL: oci://ghcr.io/acme/platform-wet/traefik-checkout
    targetRevision: sha256:ab44...771e
  destination:
    server: https://kubernetes.default.svc
    namespace: checkout-prod
```

### Step 5: App change -> platform merge approval -> upstream DRY promotion

This is the key promotion path:

1. App team adds a feature/config change in app DRY unit (for example new middleware rule).
2. Platform engineer reviews and merge-approves source PR in GitHub.
3. ConfigHub policy decides `ALLOW|ESCALATE|BLOCK` for deploy execution.
4. On allowed execution, rollout and verification complete.
5. If broadly useful, ConfigHub opens a separate promotion PR to platform base DRY.
6. Platform team merges upstream promotion and app unit override is minimized.

This avoids permanent app forks while preserving app-team velocity.

---

## 5. Provenance and Field-Origin Map (Extra Value)

### Provenance snapshot (illustrative)

```yaml
unit: traefik-checkout-prod-rendered
kind: wet
generator:
  name: helm-renderer
  version: helm@3.16.2
inputs:
  digest: sha256:2f3d...9a10
source_artifacts:
  - role: base_unit
    unit: traefik-platform-base
    revision: b41f3ad
  - role: app_unit
    unit: checkout-edge-prod
    revision: c92d10e
  - role: chart
    ref: oci://ghcr.io/traefik/helm/traefik@29.0.0
rendered:
  artifact: oci://ghcr.io/acme/platform-wet/traefik-checkout@sha256:ab44...771e
controller:
  type: flux
  object: OCIRepository/traefik-checkout-prod
```

### Field-origin map snapshot (illustrative)

```yaml
field_origin_map:
  - wet_path: spec.routes[0].match
    dry_source:
      unit: checkout-edge-prod
      path: overrides.ingressRoute.match
      editable_by: app-team

  - wet_path: spec.entryPoints[0]
    dry_source:
      unit: traefik-platform-base
      path: defaults.ports.websecure
      editable_by: platform-team

  - wet_path: Service.spec.type
    dry_source:
      unit: traefik-platform-base
      path: defaults.service.type
      editable_by: platform-team
```

Operational result:

1. teams can tell exactly which DRY layer owns each deployed field,
2. write-through edits can route to correct source unit,
3. policy can enforce ownership boundaries at field granularity.

---

## 6. Where Data Lives (Git vs ConfigHub vs OCI)

1. Git: DRY sources, review history, optional compact receipts.
2. OCI: WET deployment artifact transport (digest-pinned).
3. ConfigHub: dry/wet units, merge links, policy decisions, verification, attestation.

Git remains collaboration ingress. OCI is deployment transport. ConfigHub is the
governance and provenance control plane.

---

## 7. Equivalence Checklist

If all are true, this Traefik Helm flow matches the dry/wet model:

1. Platform base DRY and app DRY are explicitly separated.
2. Renderer unit composes DRY layers into WET artifact.
3. Flux/Argo controllers run unchanged.
4. WET transport is OCI digest (preferred).
5. Governance decisions are explicit (`ALLOW|ESCALATE|BLOCK`).
6. Verification and attestation are linked to the same change ID.
7. Reusable app changes can be promoted to platform DRY through separate PR.

---

## 8. Related Docs

1. `docs/reference/scoredev-dry-wet-unit-worked-example.md`
2. `docs/reference/dual-approval-gitops-gh-pr-and-ch-mr.md`
3. `docs/reference/stored-in-git-vs-confighub.md`
4. `docs/reference/next-gen-gitops-ai-era.md`
