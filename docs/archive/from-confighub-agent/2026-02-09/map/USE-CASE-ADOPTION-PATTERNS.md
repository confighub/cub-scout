# Use Case: Adoption Patterns for Flux/Argo Organizations

> **Archive status:** Historical planning context (non-canonical for current releases).
> Use these maintained docs for current behavior and scope:
> - `docs/roadmap.md`
> - `docs/reference/import-docs-crosswalk.md`
> - `docs/reference/gitops-repo-structures.md`

**Goal:** Make ConfigHub easier to adopt for organizations already using Flux or Argo CD.

**Status:** Research + reference gathering

---

## Context (Slack Thread 2026-01-14)

**Jesper Joergensen:**
> As part of discovering how we can make ConfigHub easier to adopt for organizations already using Flux or Argo, we want to "nail" a couple of examples of typical repo structures for both Flux and Argo.

---

## Reference Architectures

### Flux CD

**Recommended:** Flux Fluxy Architectural Reference
- **URL:** https://fluxcd.control-plane.io/guides/d2-architecture-reference/
- **Status:** Well-documented, 3 linked repos

The Fluxy architecture is the recommended pattern for enterprise Flux deployments.

#### Fluxy-Fleet Repository

```
fluxy-fleet/
├── .github/workflows/
│   └── push-artifact.yaml       # Publish OCI Artifacts to GHCR
├── clusters/
│   └── staging/
│       └── flux-system/
│           └── flux-instance.yaml   # FluxInstance manifest per cluster
├── tenants/
│   └── *.yaml                   # ResourceSet definitions for delivery
└── terraform/
    └── ...                      # Bootstrap clusters with Terraform/OpenTofu
```

**Purpose:** Platform team's control plane for multi-cluster orchestration.

#### Fluxy-Infra Repository

```
fluxy-infra/
├── .github/workflows/
├── components/
│   ├── cert-manager/
│   │   ├── controllers/
│   │   │   ├── base/
│   │   │   ├── production/
│   │   │   └── staging/
│   │   └── configs/
│   │       ├── base/
│   │       ├── production/
│   │       └── staging/
│   ├── monitoring/
│   │   └── ...
│   └── ingress/
│       └── ...
└── update-policies/
    └── *.yaml                   # Automate OCI chart updates
```

**Purpose:** Cluster add-ons and monitoring stack (infrastructure layer).

#### Fluxy-Apps Repository

```
fluxy-apps/
├── .github/workflows/
├── components/
│   └── {namespace}/
│       ├── base/
│       │   └── release.yaml     # HelmRelease definition
│       ├── production/
│       │   └── values.yaml      # Production overrides
│       └── staging/
│           └── values.yaml      # Staging overrides
└── update-policies/
    └── *.yaml                   # Automate app version updates
```

**Purpose:** Application deployments organized by namespace and environment.

#### Fluxy Pattern Summary

| Repo | Layer | Owned By |
|------|-------|----------|
| `fluxy-fleet` | Fleet orchestration | Platform team |
| `fluxy-infra` | Infrastructure add-ons | Platform team |
| `fluxy-apps` | Applications | Dev teams |

**Key insight:** Uses OCI Artifacts instead of direct Git references ("Gitless GitOps").

### Argo CD

**Sources identified:**

1. **Web Environment Promotion Pattern**
   - **Repo:** https://github.com/kostis-codefresh/gitops-environment-promotion
   - **Blog:** https://codefresh.io/blog/how-to-model-your-gitops-environments-and-promote-releases-between-them/
   - **Pattern:** Folders for environments, propagate by moving files, single branch
   - **Stars:** 325

2. **Akuity/Kargo Promotion Pattern**
   - **URL:** https://akuity.io/blog/promotion-made-easy-with-kargo
   - **Pattern:** GitOps-compliant branches, propagate changes from cluster back to Git

3. **Web Certification Course** (confidential, do not share)
   - Screenshots saved: `arnie-web-screenshots/00.png` - `08.png`
   - PDF saved: `arnie-web-screenshots/Use folders for environments.pdf`
   - **Includes Helm example** (image 08)

---

## Web Course Summary (from Arnie's screenshots)

### Image 00: The Environment-per-Folder Approach

**Promotion flow:** Source Code → Build → QA → Stage → Prod

**4 categories of configuration:**
1. **Application version** — container image tag (most important for promotion)
2. **Kubernetes specific settings** — replicas, resource limits, affinity
3. **Mostly static business settings** — env-specific URLs, DB credentials (don't promote)
4. **Non-static business settings** — settings you DO want to promote (global VAT, cache sizes)

### Image 01: Example with 11 Environments

```
gitops-repo/
├── base/                    # Common to all environments
├── envs/
│   ├── integration-gpu/
│   ├── integration-non-gpu/
│   ├── load-gpu/
│   ├── load-non-gpu/
│   ├── prod-asia/
│   ├── prod-eu/
│   ├── prod-us/
│   ├── qa/
│   ├── staging-asia/
│   ├── staging-eu/
│   └── staging-us/
└── variants/                # Mixins/components
    ├── asia/
    ├── eu/
    ├── non-prod/
    ├── prod/
    └── us/
```

### Image 02-03: Kustomization + Patches

**staging-asia/kustomization.yaml:**
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: staging
namePrefix: staging-asia-
resources:
  - ../../base
components:
  - ../../variants/non-prod
  - ../../variants/asia
patchesStrategicMerge:
  - deployment.yml
  - version.yml      # Image tag (promotable)
  - replicas.yml     # Replica count
  - settings.yml     # Business settings (promotable)
```

### Image 04: Comparing Environments

```bash
# Diff settings between two environments
vimdiff envs/integration-gpu/settings.yml envs/integration-non-gpu/settings.yml

# Build and compare full manifests
kustomize build envs/qa/ > /tmp/qa.yml
kustomize build envs/staging-us/ > /tmp/staging-us.yml
kustomize build envs/prod-us/ > /tmp/prod-us.yml
vimdiff /tmp/staging-us.yml /tmp/qa.yml /tmp/prod-us.yml
```

### Image 05: Promotion Scenarios (File Copy)

| Scenario | Command |
|----------|---------|
| QA → staging-us | `cp envs/qa/version.yml envs/staging-us/version.yml` |
| integration-gpu → load-gpu → qa | 2-step: copy version.yml through the chain |
| prod-eu → prod-us + settings | `cp envs/prod-eu/version.yml envs/prod-us/` + `cp envs/prod-eu/settings.yml envs/prod-us/` |
| Global change to all non-prod | Edit `variants/non-prod/non-prod.yml` |
| Add config to all US envs | Add to `variants/us/`, update kustomization.yaml |

**Key insight:** All promotions are just `cp` operations. No git cherry-pick, no branch merges.

### Image 06: Safe Multi-Environment Changes

**3-step approach for EU-wide change:**
1. Make change in `envs/staging-eu` (test in staging)
2. Make same change in `envs/prod-eu` (apply to prod)
3. Delete from both, add to `variants/eu` (make permanent)

**Anti-pattern:** Never change `base/` directly. Always promote through envs first.

### Image 07: Why Folders > Branches

| Problem with Branches | Solved by Folders |
|-----------------------|-------------------|
| Commit order matters | Copy files, order irrelevant |
| Cherry-pick complexity | Just `cp` |
| Merge brings unwanted changes | Copy only what you need |
| N environments = N branches | N environments = 1 branch |
| Drift during merge | No drift, files are explicit |

### Image 08: Helm with GitOps Environments

```
helm-repo/
├── chart/
│   └── [...chart files...]
├── common/
│   └── values-common.yml
├── variants/
│   ├── prod/
│   │   └── values-prod.yml
│   ├── non-prod/
│   │   └── values-non-prod.yml
│   └── [...other variants...]
└── envs/
    └── prod-eu/
        ├── values-env-default.yaml
        ├── values-replicas.yaml
        ├── values-version.yaml      # Promotable
        └── values-settings.yaml     # Promotable
```

**Preview command:**
```bash
helm template chart/ --values common/values-common.yaml --values variants/prod/values-prod.yml --values envs/prod-eu/values-version.yaml
```

---

## Pattern: Folders for Environments (Web/Kostis)

```
gitops-repo/
├── envs/
│   ├── dev/
│   │   └── deployment.yaml
│   ├── staging/
│   │   └── deployment.yaml
│   └── prod/
│       └── deployment.yaml
└── base/
    └── kustomization.yaml
```

**How it works:**
1. Create env per folder
2. Propagate by moving/copying files
3. Everything on one branch

**Limitations:**
- Usually not enough for complex scenarios
- Need additional tools:
  - Argo CD Pull Request Generator (trunk-based, feature branch → environment)
  - Kargo (creates GitOps branches, propagates changes)

---

## ConfigHub Examples Using This Pattern

**Already implemented:**
- https://github.com/confighub/examples/tree/main/global-app
  - Deliberately reuses the Web environment layout
  - Shows ConfigHub working with folder-per-env structure

---

## What We Need

### For Flux
- [x] Fluxy Architecture Reference — well documented
- [ ] Map TUI demo against Fluxy repos

### For Argo CD
- [ ] Solid blueprint/sample repo for Kustomize (Web pattern works)
- [ ] Solid blueprint/sample repo for **Helm** (Arnie to provide)
- [ ] Map TUI demo against Argo repos

### Map Integration Points

The `cub-agent map` TUI should:

1. **Detect** both patterns automatically
   - Flux: `kustomize.toolkit.fluxcd.io/*` labels
   - Argo: `argocd.argoproj.io/instance` labels

2. **Show** the folder structure visually
   - Which env folders exist
   - What's deployed where

3. **Trace** from deployed resource → folder → Git
   - `cub-agent trace` already does this

4. **Import** respecting existing structure
   - Don't break their folder-per-env layout
   - Create ConfigHub Units that mirror their existing org

---

## Next Steps

1. [ ] Get Helm example from Arnie
2. [ ] Test `cub-agent map` against Web environment-promotion repo
3. [ ] Test `cub-agent map` against Fluxy reference repos
4. [ ] Document any gaps in detection

---

---

## Real-World Flux Structure (Banko, 2026-01-14)

**Context:** Banko shared their actual Flux repo structure used in production.

### Directory Structure

```
├── clusters/
│   ├── cluster-1.example.com/
│   │   ├── component-a/
│   │   │   └── kustomization.yaml
│   │   ├── component-b/
│   │   │   └── kustomization.yaml
│   │   └── component-c/
│   │       └── kustomization.yaml
│   ├── cluster-2.example.com/
│   │   ├── component-a/
│   │   │   └── kustomization.yaml
│   │   └── component-b/
│   │       └── kustomization.yaml
│   └── cluster-3.example.com/
│       └── component-a/
│           └── kustomization.yaml
│
├── platform/                          # Off-the-shelf components (versioned)
│   ├── component-a/
│   │   └── v1.0.0/
│   │       ├── sync.yaml              # CRDs, Charts, etc.
│   │       └── values.yaml
│   ├── component-b/
│   │   └── v2.1.0/
│   │       ├── sync.yaml
│   │       └── namespace.yaml
│   └── component-c/
│       └── v3.0.0/
│           └── sync.yaml
│
└── apps/                              # Internal apps
    ├── app-1/
    │   └── v1.0.0/
    │       └── manifests.yaml
    └── app-2/
        └── v2.0.0/
            └── manifests.yaml
```

### How It Works

**Flux Kustomization** references `./clusters/{clustername}` to bring in everything for that cluster.

**Example: `cluster-1.example.com/component-a/kustomization.yaml`:**
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../../platform/example/namespace.yaml
  - ../../../platform/example/2022.4.8/sync.yaml
  - sync.yaml
```

### Key Patterns

| Pattern | Description |
|---------|-------------|
| **Cluster per directory** | Each cluster has its own dir under `clusters/` |
| **Version per directory** | Platform components versioned: `platform/grafana/v1.0.0/` |
| **Apps separate from platform** | Internal apps in `apps/`, external in `platform/` |
| **Avoid Helm where possible** | They "de-helmed" cert-manager, still use grafana chart |
| **sync.yaml** | Contains resources to apply directly (CRDs, charts, namespaces) |

### Repo Organization

| Question | Answer |
|----------|--------|
| Single monorepo? | **No** — used to be monolith, now broken apart |
| Multi-team? | Some repos require PRs from other teams |
| Version strategy | External products versioned (`v1.0.0/`), internal apps use `/prod`, `/dev`, `/base` |

### Map Integration Opportunity

`cub-agent map` should detect:
1. `clusters/*/` structure → one Flux Kustomization per cluster
2. `platform/*/v*/` structure → versioned platform components
3. `apps/*/` structure → internal applications

This maps to ConfigHub:
- Each `clusters/{name}` → Space or Target
- Each `platform/{component}` → Hub (shared infrastructure)
- Each `apps/{app}` → AppSpace (team-owned)

---

## Related

- [08-CASE-STUDIES-IITS.md](08-CASE-STUDIES-IITS.md) — Enterprise fleet patterns
- [VIEW-TIERS.md](../VIEW-TIERS.md) — TUI view tiers and proof matrix
- `confighub/examples/global-app` — Our example using this pattern
