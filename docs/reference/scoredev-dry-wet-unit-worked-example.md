# score.dev Worked Example: Dry/Wet Units with FluxCD and ArgoCD

Status: reference walkthrough (product semantics)
Last updated: 2026-02-28

This is a complete Score.dev example that matches the same dry/wet unit model used
in the FluxCD and ArgoCD rendered-pipeline demos.

It is equivalent in structure to Brian's Flux/Argo dry/wet unit solution:

1. DRY intent is authored and versioned.
2. Renderer units produce WET manifests.
3. GitOps controller reconciles WET to cluster.
4. ConfigHub stores dry units and wet units, with WET authoritative for deployment.
5. Dry/wet units are linked and auto-update through the renderer pipeline.

---

## 1. What This Example Proves

For a Score.dev app (`checkout`), this walkthrough shows:

1. how Score intent becomes a dry unit,
2. how rendered manifests become a wet unit,
3. how FluxCD or ArgoCD remains the deployment engine,
4. how `cub gitops import` creates equivalent dry/wet pairs for ongoing management,
5. how provenance and field-origin maps make review, editing, and governance practical.

No controller replacement is required.

---

## 2. Equivalent Mapping (Score vs Flux/Argo)

| Concept | Flux/Argo dry/wet pipeline | Score.dev equivalent |
|---|---|---|
| DRY authoring input | Kustomization/Application source intent | `score.yaml` + environment parameters |
| Renderer | Flux/Argo renderer worker | Score->K8s generator + controller renderer worker |
| WET deployment contract | Fully rendered manifests | Fully rendered manifests from Score intent |
| Reconciler | FluxCD or ArgoCD | FluxCD or ArgoCD (unchanged) |
| ConfigHub model | Dry/wet units linked by MergeUnits | Dry/wet units linked by MergeUnits |
| Update behavior | Git change -> re-render -> wet auto-updates | Score change -> re-render -> wet auto-updates |

---

## 3. Repository Layout (Example)

Use two repos (common enterprise split):

1. **App intent repo** (team-owned DRY ingress)
2. **GitOps deploy repo** (controller-facing WET contract)

Example:

```text
acme-checkout-intent/
  score.yaml
  env/
    staging.env
    prod.env

acme-platform-gitops/
  apps/
    checkout/
      staging/
        rendered.yaml
      prod/
        rendered.yaml
  flux/ or argo/ controller objects
```

Git remains the primary collaboration and ingress surface for most teams.
ConfigHub stores dry units and wet units as control-plane records.

---

## 4. Step-by-Step Walkthrough

### Step 1: Author Score intent (DRY)

`score.yaml` in `acme-checkout-intent`:

```yaml
apiVersion: score.dev/v1b1
kind: Workload
metadata:
  name: checkout
containers:
  main:
    image: ghcr.io/acme/checkout:1.3.1
    resources:
      requests:
        cpu: "250m"
        memory: "256Mi"
      limits:
        cpu: "500m"
        memory: "512Mi"
    livenessProbe:
      httpGet:
        path: /healthz
        port: 8080
      initialDelaySeconds: 10
```

Commit this as normal team workflow:

```bash
git add score.yaml env/
git commit -m "feat(score): define checkout workload intent"
git push
```

### Step 2: Render Score intent to WET manifests

Render with your Score pipeline (example using `score-k8s`):

```bash
# staging render
score-k8s generate score.yaml \
  --output-format yaml \
  --output acme-platform-gitops/apps/checkout/staging/rendered.yaml

# prod render
score-k8s generate score.yaml \
  --output-format yaml \
  --output acme-platform-gitops/apps/checkout/prod/rendered.yaml
```

Commit generated WET artifacts in the GitOps repo:

```bash
cd acme-platform-gitops
git add apps/checkout/staging/rendered.yaml apps/checkout/prod/rendered.yaml
git commit -m "chore(render): update checkout manifests from score intent"
git push
```

### Step 3: Keep controller config unchanged

### Flux variant

Flux Kustomization points at rendered path:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: checkout-prod
  namespace: flux-system
spec:
  path: ./apps/checkout/prod
  prune: true
  sourceRef:
    kind: GitRepository
    name: platform-config
```

### Argo variant

Argo Application points at rendered path:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: checkout-prod
  namespace: argocd
spec:
  source:
    repoURL: https://github.com/acme/platform-gitops
    targetRevision: main
    path: apps/checkout/prod
  destination:
    server: https://kubernetes.default.svc
    namespace: checkout-prod
```

In both cases, controllers reconcile exactly as before.

### Step 4: Build dry/wet unit pairs in ConfigHub

Create the live rendered pipeline (same model as existing Flux/Argo demos):

```bash
# discover deployers
cub gitops discover --space "$SPACE" "$K8S_TARGET"

# create dry/wet unit pairs through renderer target
cub gitops import --space "$SPACE" "$K8S_TARGET" "$RENDERER_TARGET"
```

Where:

1. For Flux: renderer target type is `fluxrenderer`.
2. For Argo: renderer target type is `argocdrenderer`.

Expected result:

1. dry unit for `checkout` deployment intent,
2. wet unit for rendered manifests,
3. MergeUnits linkage between dry and wet,
4. auto-update behavior when Git/render inputs change.

### Step 4a: Renderer units (explicit)

In connected mode, renderer workers create and refresh renderer-unit records that
bind:

1. the DRY input snapshot,
2. the render operation metadata (generator, version, digest, rendered timestamp),
3. the resulting WET unit revision.

This is what makes the dry->wet pipeline continuously explainable instead of
"render happened somewhere in CI."

### Step 5: Verify unit behavior

Operational expectation after import:

1. dry unit captures the declared deployer/render intent and source metadata,
2. wet unit captures fully rendered YAML the controller applies,
3. wet unit changes on new renders without manual re-import.

This is the same semantics proven in:

1. `examples/flux-import-confighub-demo/README.md`
2. `examples/argo-import-confighub-demo/README.md`

---

## 5. Provenance and Field-Origin Map (What Extra You Get)

This is the part teams do not get from a plain Git diff.

For each dry/wet pair, capture explicit provenance fields and a field-origin map.

### Provenance snapshot (illustrative)

```yaml
unit: checkout-prod-rendered
kind: wet
generator:
  name: score-generator
  version: score-k8s@0.5.0
inputs:
  digest: sha256:2f3d...9a10
rendered:
  at: "2026-02-28T17:12:43Z"
source_artifacts:
  - role: dry-intent
    repo: https://github.com/acme/acme-checkout-intent
    revision: 3e8c8fb
    path: score.yaml
  - role: wet-contract
    repo: https://github.com/acme/acme-platform-gitops
    revision: a12b77d
    path: apps/checkout/prod/rendered.yaml
controller:
  type: flux
  object: Kustomization/checkout-prod
merge_units:
  dry_unit: checkout-prod
  wet_unit: checkout-prod-rendered
```

The same structure works with `controller.type: argo` and the corresponding
`Application/checkout-prod` source object.

### Field-origin map snapshot (illustrative)

```yaml
field_origin_map:
  - wet_path: spec.template.spec.containers[name=main].image
    dry_source:
      file: score.yaml
      path: containers.main.image
      line: 9
      editable_by: app-team

  - wet_path: spec.template.spec.containers[name=main].resources.requests.cpu
    dry_source:
      file: score.yaml
      path: containers.main.resources.requests.cpu
      line: 12
      editable_by: app-team

  - wet_path: spec.template.spec.containers[name=main].resources.limits.memory
    dry_source:
      file: score.yaml
      path: containers.main.resources.limits.memory
      line: 16
      editable_by: app-team

  - wet_path: spec.template.spec.containers[name=main].livenessProbe.httpGet.path
    dry_source:
      file: score.yaml
      path: containers.main.livenessProbe.httpGet.path
      line: 19
      editable_by: app-team
```

### Why this matters in operations

1. You can prove exactly which generator version and input digest produced WET.
2. You can route edits to the right DRY source without Git archaeology.
3. You can detect drift classes:
   `inputs.digest` changed but WET unchanged -> stale render;
   WET changed with same `inputs.digest` -> overlay drift.

### Write-through editing flow using the field map

The field-origin map enables write-through editing (edit in control plane, commit
to DRY source):

1. User selects WET field `spec.template.spec.containers[name=main].resources.requests.cpu`.
2. ConfigHub resolves map entry -> `score.yaml` `containers.main.resources.requests.cpu`.
3. System prepares DRY-source change (PR/commit) in the intent repo.
4. Renderer pipeline re-runs and updates WET unit.
5. Flux/Argo reconciles updated WET as usual.

Developers get one editing surface without losing DRY-source integrity.

---

## 6. AI-Assisted Mutation Loop (Score-specific)

Now run an AI-assisted change and observe equivalence end-to-end.

### Change request

"Increase checkout resilience in prod and tighten resource bounds."

### Execution

1. AI updates `score.yaml` (probe settings/resources).
2. Team reviews and merges DRY change in intent repo.
3. Render pipeline updates `apps/checkout/prod/rendered.yaml`.
4. Flux/Argo reconciles updated WET manifests.
5. Renderer pipeline updates wet unit in ConfigHub.

Optional mutation governance overlay:

```bash
# in GitOps repo
cub track enable
git commit -m "chore: tune checkout probe and resources"
cub track explain --commit HEAD
cub track search --text "checkout probe" --agent codex
cub track search --decision ESCALATE
```

This adds intent/decision/outcome lineage on top of the same dry/wet controller flow.

### Trust-tier path (explicit `ALLOW|ESCALATE|BLOCK`)

Example governance outcomes for the same Score mutation:

1. **staging** tier-1 check -> `ALLOW`
2. **prod** tier-2 check -> `ESCALATE` (human approval required)
3. if policy fails hard -> `BLOCK`

After `ALLOW` (or approved `ESCALATE`):

1. system issues scoped execution token,
2. reconciler applies,
3. verification checks run,
4. attestation record is written and linked to mutation + dry/wet units.

Illustrative attestation summary:

```yaml
mutation_id: mut_9f29a5c
decision:
  environment: prod
  result: ESCALATE
  approved_by: sre-oncall
execution:
  token_scope: app=checkout,env=prod
  runtime: confighub-actions
verification:
  result: pass
  checks:
    - rollout_ready
    - policy_postconditions
attestation:
  digest: sha256:ab44...771e
  recorded_at: "2026-02-28T18:07:15Z"
```

### App-team change -> platform approval -> upstream DRY promotion

This is the recommended promotion path when an app-level change should become a
platform default:

1. App team edits app-owned DRY fields (`score.yaml` or app overlay unit).
2. ConfigHub renders/evaluates candidate WET, posts evidence, and opens/updates a ConfigHub MR (plus paired Git PR if mirror is enabled).
3. Platform engineer reviews and merge-approves the app change in ConfigHub.
4. ConfigHub enforces governed deploy decision (`ALLOW|ESCALATE|BLOCK`) for that merged change.
5. On `ALLOW` (or approved `ESCALATE`), execution runs with scoped token, then verification and attestation are recorded.
6. After successful rollout, ConfigHub opens a promotion PR/MR to upstream Platform DRY/Base Unit when reusable, if not done already.
7. After required upstream approvals, ConfigHub merges the promotion PR in Git.
8. App-specific override is reduced or removed to avoid long-lived drift.

This keeps team velocity high while still converging reusable behavior into the
platform's main DRY contract.

Guardrail:

1. Do not auto-write directly to platform main DRY without separate upstream review/merge.

Live-origin variant:

1. If observer tooling detects a live-only Score-related change, ConfigHub creates a proposal MR from live evidence.
2. Accepted proposals are converted into DRY source edits and follow the same promotion path above.

---

## 7. What Belongs in Git vs ConfigHub in This Example

### Git (primary ingress/collaboration)

1. `score.yaml` and env inputs (DRY authoring)
2. rendered manifests in GitOps repo (WET deployment contract in repo form)
3. commit history and review artifacts
4. optional compact mutation linkage receipts (`cub-track`)

### ConfigHub (control-plane store)

1. dry units and wet units
2. dry/wet linkage (`MergeUnits`)
3. provenance joins across repos/clusters
4. policy, verification, and attestation state (connected/governed modes)

WET in ConfigHub remains authoritative as deployment contract representation.

---

## 8. Equivalence Checklist

If all items are true, your Score setup is equivalent to the Flux/Argo dry/wet unit solution:

1. Flux/Argo controllers are unchanged.
2. Score intent is versioned as DRY input.
3. Rendered manifests are explicit WET artifacts.
4. `cub gitops import` creates dry/wet pairs for managed deployers.
5. dry/wet pairs auto-update when render input changes.
6. Teams can query both "what is deployed" and "what generated it."
7. Teams can trace any critical WET field back to a DRY source via field-origin map.
8. Teams can explain/search AI mutations and enforce `ALLOW|ESCALATE|BLOCK` with attested outcomes.
9. App-team changes can be promoted upstream into platform DRY defaults through a separate PR.

---

## 9. Explicit 12-Point Generator Capability Coverage

This Score example now explicitly covers the full 12-point generator value set:

1. DRY intent as first-class authoring input (`score.yaml` + env inputs).
2. Explicit WET contract generated from DRY input.
3. Renderer units that bind DRY snapshot -> render operation -> WET revision.
4. Dry/wet unit pairing with `MergeUnits` linkage in ConfigHub.
5. Provenance tuple (`generator`, `version`, `inputs.digest`, `rendered.at`).
6. Cross-repo source artifact references (intent repo + deploy repo revisions).
7. Field-origin map from WET path back to DRY source path/line.
8. Write-through editing from WET view back to DRY source.
9. Drift classification support (`stale render` vs `overlay drift`).
10. AI mutation ledger overlay (`cub track enable`, `explain`, `search`).
11. Trust-tier decision path (`ALLOW|ESCALATE|BLOCK`) with scoped execution.
12. Verification + attestation linked to mutation and dry/wet records.

Upstream DRY promotion flow is additionally shown in Section 6.

### Fast validation map

| Capability | Where shown |
|---|---|
| 1-4 | Sections 2, 4, 4a |
| 5-7 | Section 5 (`Provenance snapshot`, `Field-origin map`) |
| 8-9 | Section 5 (`Write-through editing`, drift classification notes) |
| 10 | Section 6 (`cub track` commands) |
| 11-12 | Section 6 (`Trust-tier path`, attestation summary) |

---

## 10. Related Docs

1. `examples/flux-import-confighub-demo/README.md`
2. `examples/argo-import-confighub-demo/README.md`
3. `docs/reference/next-gen-gitops-ai-era.md`
4. `docs/reference/stored-in-git-vs-confighub.md`
5. `docs/reference/dual-approval-gitops-gh-pr-and-ch-mr.md`
6. `docs/reference/traefik-helm-dry-wet-unit-worked-example.md`
