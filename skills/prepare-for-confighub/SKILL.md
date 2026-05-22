---
name: prepare-for-confighub
description: 'Use when the user is ADOPTING ConfigHub — they have existing K8s + GitOps state (a git repo of manifests, live Argo ApplicationSets, an existing cluster, a Flux Kustomization tree) and want to PREVIEW what a ConfigHub import would look like before committing. Natural phrasing: "preview what would land in ConfigHub if I imported this repo", "show me the proposed units before I apply", "I want to adopt ConfigHub for this app", "what would ConfigHub see in our argocd setup?", "scaffold ConfigHub units from this git repo", "disk-PR my ConfigHub import proposal". Composes `cub-scout import --git-path` + `import argocd` + `import cluster-aggregator` + `import parse-repo` + `app` for the **preview-only** flow. Do NOT load for: actually applying the proposal (that''s `cub-scout import apply`, user-driven), live conformance audits (use audit-fleet-conformance), or comparing live to ConfigHub once units exist (use scout-compare). The whole skill is read-only and refuses to invoke `import apply`.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout import --git-path *) Bash(cub-scout import --git-path *) Bash(cub scout import --git-path *) Bash(./cub-scout import parse-repo *) Bash(cub-scout import parse-repo *) Bash(cub scout import parse-repo *) Bash(./cub-scout import argocd *) Bash(cub-scout import argocd *) Bash(cub scout import argocd *) Bash(./cub-scout import cluster-aggregator *) Bash(cub-scout import cluster-aggregator *) Bash(cub scout import cluster-aggregator *) Bash(./cub-scout app *) Bash(cub-scout app *) Bash(cub scout app *) Bash(./cub-scout map workloads *) Bash(cub-scout map workloads *) Bash(cub scout map workloads *) Bash(./cub-scout scan *) Bash(cub-scout scan *) Bash(cub scout scan *) Bash(./cub-scout status) Bash(cub-scout status) Bash(cub scout status) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub unit list *) Bash(cub gitops discover *) Bash(argocd app get *) Bash(argocd appset get *) Bash(argocd appset list *) Bash(flux get *)
---

# prepare-for-confighub

The adoption-preview loop. The user has existing K8s + GitOps state and wants to KNOW what a ConfigHub import would produce **before** committing. This skill is the **preview** flow — every verb is read-only, and `import apply` (the mutating write) is **explicitly out of band**. The user runs that themselves after reviewing the proposal.

## When to use

Explicit phrasings:

- "Preview what would land in ConfigHub if I imported this repo"
- "Show me the proposed units before I apply"
- "I want to adopt ConfigHub for this app — what would it look like?"
- "What would ConfigHub see in our Argo CD setup?"
- "Scaffold ConfigHub units from this git repo"
- "Disk-PR my ConfigHub import proposal"
- "Walk me through ConfigHub adoption for `payments-api`"
- "Compare ApplicationSet generator import vs a raw `--git-path` import for this repo"

Implicit intents:

- The user is **before** ConfigHub adoption, not in steady state
- The user wants **PR-review-able** output, not a live mutation
- The user is comparing options ("what if I use ApplicationSet vs raw YAML import?")
- The user is gating on team-review or compliance approval before the actual write

## Do not load for

- Actually applying the proposal (`cub-scout import apply`) — that's user-driven, not in this skill's allowed-tools
- Live conformance audits **once** units exist — [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md)
- Comparing live state to existing ConfigHub units — [`scout-compare`](../scout-compare/SKILL.md) (specifically `compare three-way`)
- Cluster-side `cub gitops import` (target + render-target based) — that's `cub`, not cub-scout. Route to [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills).
- Triage / drift investigation — the corresponding workflow skills

## The loop

1. **Identify the source.** Where is the structure today? Local git repo (`--git-path <dir>`)? Cluster Argo Applications (`import argocd`)? Multi-cluster snapshot (`import cluster-aggregator`)? Pick the verb.
2. **Run the preview verb.** ASCII for review-by-eye; `--format json` if the output feeds a PR-bot or a CI check.
3. **Land a disk PR (optional but recommended).** `import --git-path --output-dir <path>` writes proposed unit YAMLs to disk so the PR review happens in git, not in ConfigHub.
4. **Iterate.** Adjust the source structure (rename, regroup, split namespaces) until the proposal looks right. cub-scout doesn't change anything; you just re-run the preview.
5. **Hand off the mutation.** Once the proposal is approved, the user runs `cub-scout import apply --space <s>` themselves. cub-scout's `prepare-for-confighub` skill **never** invokes apply.

## Step-by-step

### Step 1 — pick the source verb

| Source | Verb | Notes |
|--------|------|-------|
| Local git checkout | `cub-scout import --git-path <repo>` | The most flexible. Classifies kustomize / helm / raw manifests; emits proposed slugs + spaces + targets. Supports `--output-dir` for disk-PR. |
| Existing Argo Applications | `cub-scout import argocd --space <s>` | Reads Argo state from the cluster, proposes one ConfigHub unit per Application. Preserves ApplicationSet generator origin as hierarchy. |
| Multi-cluster bundle | `cub-scout import cluster-aggregator --space <s>` | Aggregator-pattern. Aggregates per-cluster snapshots into a single ConfigHub proposal. |
| Just structure parsing | `cub-scout import parse-repo <repo>` | Lower-level. Extracts ApplicationSet directories, helm chart paths, kustomization roots. Used as a primitive. |

The verbs are **preview** by default. None of them write to ConfigHub.

### Step 2 — run the preview

**Git-path preview (most common):**

```bash
$ cub-scout import --git-path ./platform-config --format json | jq '.'
{
  "units": [
    {"slug": "apps-prod-api", "path": "apps/prod/api", "kind": "Deployment", "rendererHint": "kustomize", "confidence": "high"},
    {"slug": "apps-prod-worker", "path": "apps/prod/worker", "kind": "Deployment", "rendererHint": "kustomize", "confidence": "high"}
  ],
  "spaces": ["platform-prod"],
  "targets": ["Kubernetes:prod-use2"],
  "unsupported": []
}
```

**Argo-import preview:**

```bash
$ cub-scout import argocd --space payments --format json
```

The output structure is per-Application: each gets a proposed slug, the source path the Application reads, and the renderer hint (kustomize / helm / raw). ApplicationSet generators are preserved — see [`observe-argocd`](../observe-argocd/SKILL.md) for the generator support.

### Step 3 — land a disk PR

The **safest** workflow: write proposed unit YAMLs to disk; open a PR in git; review there; merge; only THEN does anyone run `import apply`.

```bash
$ cub-scout import --git-path ./platform-config --output-dir ./proposed-units
$ git checkout -b adopt-confighub
$ git add proposed-units/
$ git commit -m "propose ConfigHub units for platform-config"
$ git push origin adopt-confighub
$ gh pr create ...
```

The PR is reviewable in git, with all the usual diff tooling. ConfigHub itself sees nothing until someone runs `import apply` post-merge.

### Step 4 — iterate

If the proposal isn't right (wrong slugs, wrong space grouping, missing workloads), adjust the source structure and re-run. cub-scout's import verbs are deterministic — same source produces same proposal, so the PR diff narrows iteration-by-iteration.

Common adjustments:

- **Rename** repo directories to control proposed slugs (`apps/prod/api` → slug `apps-prod-api`)
- **Split** monolithic kustomize roots into per-workload directories
- **Add** README or NOTES files to explain non-obvious structure
- **Tag** ApplicationSet templates so generator-derived units get distinguishable slugs

### Step 5 — hand off the apply

Once the proposal is approved:

```bash
# THE USER runs this — not cub-scout, not this skill
$ cub-scout import apply --space platform-prod --output-dir ./proposed-units
```

`import apply` writes to ConfigHub. It IS a mutation. It is intentionally NOT in this skill's allowed-tools. Recommend the user run it themselves and verify the result with `cub unit list --space platform-prod`.

## Worked example

A team has a git repo `platform-config/` with three apps under `apps/prod/`. They want to adopt ConfigHub.

```bash
$ cub-scout import --git-path ./platform-config --output-dir ./proposed-units
Proposal:
  Units (3):
    apps-prod-api       — apps/prod/api/        kustomize  high
    apps-prod-worker    — apps/prod/worker/     kustomize  high
    apps-prod-frontend  — apps/prod/frontend/   raw        medium
  Spaces (1):
    platform-prod
  Targets (1):
    Kubernetes:prod-use2
  Unsupported (0): —

Wrote 3 unit YAMLs to ./proposed-units/

$ ls ./proposed-units/
apps-prod-api.yaml      apps-prod-worker.yaml   apps-prod-frontend.yaml

$ git checkout -b adopt-confighub
$ git add proposed-units/
$ git commit -m "propose ConfigHub units for platform-prod"
$ # ...PR review...
$ # ...merge...

# After PR merges, the operator runs:
$ cub-scout import apply --space platform-prod --output-dir ./proposed-units
```

cub-scout produced the proposal; git carried the review; the operator drove the apply. Three tools, three responsibilities.

## Boundaries

### cub-scout vs cub gitops import

This is **the** distinction to make clear:

- `cub-scout import --git-path <repo>` is **structure-and-preview** focused. Walks a local git checkout, classifies kustomize/helm/raw, emits a proposal. Optional `--output-dir` for disk-PR. No cluster, no ConfigHub required for the preview itself.
- `cub gitops import` (in `cub`, not cub-scout) is **cluster discovery + render-target based**. Requires a ConfigHub target + render-target already configured. Renders the discovered state and writes to ConfigHub.

They solve different problems. cub-scout is for **adoption** (first-time ingest); `cub gitops import` is for **on-going ingest** through a configured render pipeline. Don't claim they overlap.

### apply boundary

`cub-scout import apply` IS the mutating write. The user runs it directly; this skill describes its shape but does not invoke it. Defense in depth: not in `allowed-tools`.

## Tool boundary

- **Allowed (preview):** all four import verbs in their non-`apply` form; `app` (read-only manipulations); `map workloads` for inventory cross-reference; `scan` for risk classification before adoption; `cub * get/list` for connected-mode context.
- **Not allowed:** `cub-scout import apply` (writes ConfigHub units); `cub * create/update/delete`; `cub gitops import` (different tool, write-capable); `kubectl apply/edit/patch/delete`. The skill is preview-only by construction.

## References

- [`scout-ingest`](../scout-ingest/SKILL.md) — the verb group this skill composes
- [`observe-argocd`](../observe-argocd/SKILL.md) — ApplicationSet generator detail
- [`observe-flux`](../observe-flux/SKILL.md) — Kustomization / source structure
- [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md) — the result-side view of ConfigHub-managed resources
- ApplicationSet generator support: `#363`
- Render integration investigation: `#364` (verdict: keep tools separate)
- Examples: [`examples/argo-import-confighub-demo/`](../../examples/argo-import-confighub-demo/), [`examples/flux-import-confighub-demo/`](../../examples/flux-import-confighub-demo/), [`examples/fleet-import/`](../../examples/fleet-import/)

## Constraints

- This skill is **preview-only**. `import apply` is explicitly out of band — recommend the user runs it themselves after PR review.
- `--output-dir` is the safest adoption path: review in git, mutate post-merge.
- For deeply-nested Helm or templated structure, the parser produces lower-confidence proposals; surface those explicitly. cub-scout's import is for **adoption**, not for renderer reconstruction.
- Helm / Kustomize back-resolution at file:line precision is currently raw-YAML only (#440 stage B); templated sources fall back to resource-level anchors.
