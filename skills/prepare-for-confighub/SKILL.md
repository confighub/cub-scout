---
name: prepare-for-confighub
description: 'Use when the user is ADOPTING ConfigHub — they have existing live K8s + GitOps state (an existing cluster, live Argo ApplicationSets, Flux Kustomizations, or secondarily a git repo of manifests) and want to PREVIEW what a ConfigHub import would look like before committing. Natural phrasing: "show me the proposed units before I apply", "I want to adopt ConfigHub for this app", "what would ConfigHub see in our cluster?", "what would ConfigHub see in our argocd setup?", "preview what would land in ConfigHub if I imported this repo". Composes `cub-scout import --dry-run` + `import --git-path` + `import argocd` + `import cluster-aggregator` + `import parse-repo` + `app` for the **preview-only** flow. Do NOT load for: actually applying the proposal (that''s `cub-scout import apply`, user-driven), live conformance audits (use audit-fleet-conformance), or comparing live to ConfigHub once units exist (use scout-compare). The whole skill is read-only and refuses to invoke `import apply`.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout import --dry-run *) Bash(cub-scout import --dry-run *) Bash(cub scout import --dry-run *) Bash(./cub-scout import --git-path *) Bash(cub-scout import --git-path *) Bash(cub scout import --git-path *) Bash(./cub-scout import parse-repo *) Bash(cub-scout import parse-repo *) Bash(cub scout import parse-repo *) Bash(./cub-scout import cluster-aggregator *) Bash(cub-scout import cluster-aggregator *) Bash(cub scout import cluster-aggregator *) Bash(./cub-scout app list *) Bash(cub-scout app list *) Bash(cub scout app list *) Bash(./cub-scout app list) Bash(cub-scout app list) Bash(cub scout app list) Bash(./cub-scout map workloads *) Bash(cub-scout map workloads *) Bash(cub scout map workloads *) Bash(./cub-scout scan *) Bash(cub-scout scan *) Bash(cub scout scan *) Bash(./cub-scout status) Bash(cub-scout status) Bash(cub scout status) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub unit list *) Bash(cub gitops discover *) Bash(argocd app get *) Bash(argocd appset get *) Bash(argocd appset list *) Bash(flux get *)
---

# prepare-for-confighub

The adoption-preview loop. The user has existing live K8s + GitOps state and wants to KNOW what a ConfigHub import would produce **before** committing. Start from the live cluster when possible; use local repo parsing only when the manifest repository itself is under review. This skill is the **preview** flow — every verb is read-only, and `import apply` (the mutating write) is **explicitly out of band**. The user runs that themselves after reviewing the proposal.

## When to use

Explicit phrasings:

- "Show me the proposed units before I apply"
- "I want to adopt ConfigHub for this app — what would it look like?"
- "What would ConfigHub see in this live cluster?"
- "What would ConfigHub see in our Argo CD setup?"
- "Preview what would land in ConfigHub if I imported this repo"
- "Walk me through ConfigHub adoption for `payments-api`"
- "Compare ApplicationSet generator import vs a raw `--git-path` import for this repo"

Implicit intents:

- The user is **before** ConfigHub adoption, not in steady state
- The user wants **reviewable** output, not a live mutation
- The user is comparing options ("what if I use ApplicationSet vs raw YAML import?")
- The user is gating on team-review or compliance approval before the actual write

## Do not load for

- Actually applying the proposal (`cub-scout import apply`) — that's user-driven, not in this skill's allowed-tools
- Live conformance audits **once** units exist — [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md)
- Comparing live state to existing ConfigHub units — [`scout-compare`](../scout-compare/SKILL.md) (specifically `compare three-way`)
- Cluster-side `cub gitops import` (target + render-target based) — that's `cub`, not cub-scout. Route to [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills).
- Triage / drift investigation — the corresponding workflow skills

## The loop

1. **Identify the source.** Where is the structure today? Live namespace (`import --dry-run -n <ns>`)? Cluster Argo Applications (`import argocd`)? Multi-cluster snapshot (`import cluster-aggregator`)? Local git repo (`--git-path <dir>`)? Pick the verb.
2. **Run the preview verb.** ASCII for review-by-eye; `--json` if the output feeds a PR-bot or a CI check.
3. **Save a review artifact when needed.** `cub-scout import ... --json > proposal.json` gives reviewers a deterministic proposal without writing ConfigHub state.
4. **Iterate.** Adjust the source structure (rename, regroup, split namespaces) until the proposal looks right. cub-scout doesn't change anything; you just re-run the preview.
5. **Hand off the mutation.** Once the proposal is approved, the user runs `cub-scout import apply --space <s>` themselves. cub-scout's `prepare-for-confighub` skill **never** invokes apply.

## Step-by-step

### Step 1 — pick the source verb

| Source | Verb | Notes |
|--------|------|-------|
| Live namespace or cluster | `cub-scout import --dry-run -n <namespace> --json` | First adoption path. Reads current workloads, ownership, and labels before any write. |
| Local git checkout | `cub-scout import --git-path <repo> --dry-run --json` | Advanced path. Classifies kustomize / helm / raw manifests; emits proposed slugs + spaces + targets without upload or render. |
| Existing Argo Applications | `cub-scout import argocd --space <s>` | Reads Argo state from the cluster, proposes one ConfigHub unit per Application. Preserves ApplicationSet generator origin as hierarchy. |
| Multi-cluster bundle | `cub-scout import cluster-aggregator --space <s>` | Aggregator-pattern. Aggregates per-cluster snapshots into a single ConfigHub proposal. |
| Just structure parsing | `cub-scout import parse-repo <repo>` | Lower-level. Extracts ApplicationSet directories, helm chart paths, kustomization roots. Used as a primitive. |

The verbs are **preview** by default. None of them write to ConfigHub.

### Step 2 — run the preview

**Git-path preview (most common):**

```bash
$ cub-scout import --git-path ./platform-config --dry-run --json | jq '.'
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
$ cub-scout import argocd guestbook --space payments --dry-run
```

The output structure is per-Application: each gets a proposed slug, the source path the Application reads, and the renderer hint (kustomize / helm / raw). ApplicationSet generators are preserved — see [`observe-argocd`](../observe-argocd/SKILL.md) for the generator support.

### Step 3 — save a reviewable proposal

The **safest** workflow today: save the proposal JSON; review it in git, a ticket, or the PR that is coordinating the adoption; only THEN does anyone run `import apply`.

```bash
$ cub-scout import --git-path ./platform-config --dry-run --json > proposal.json
$ git checkout -b adopt-confighub
$ git add proposal.json
$ git commit -m "preview ConfigHub units for platform-config"
$ git push origin adopt-confighub
$ gh pr create ...
```

The proposal is reviewable in git, with all the usual diff tooling. ConfigHub itself sees nothing until someone runs `import apply` post-review.

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
$ cub-scout import apply proposal.json
```

`import apply` writes to ConfigHub. It IS a mutation. It is intentionally NOT in this skill's allowed-tools. Recommend the user run it themselves and verify the result with `cub unit list --space platform-prod`.

## Worked example

A team has a live `payments` namespace and a git repo `platform-config/` with three apps under `apps/prod/`. They want to adopt ConfigHub, so they preview the live cluster first and use the repo preview as a secondary check.

```bash
$ cub-scout import --dry-run -n payments --json > proposal-live.json
$ cub-scout import --git-path ./platform-config --dry-run --json > proposal-repo.json
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

$ git checkout -b adopt-confighub
$ git add proposal-live.json proposal-repo.json
$ git commit -m "preview ConfigHub units for platform-prod"
$ # ...PR review...
$ # ...merge...

# After PR merges, the operator runs:
$ cub-scout import apply proposal-live.json
```

cub-scout produced the proposals; git carried the review; the operator drove the apply. Three tools, three responsibilities.

## Boundaries

### cub-scout vs cub gitops import

This is **the** distinction to make clear:

- `cub-scout import --git-path <repo>` is **structure-and-preview** focused. Walks a local git checkout, classifies kustomize/helm/raw, emits a proposal. No cluster, no ConfigHub required for the preview itself.
- `cub gitops import` (in `cub`, not cub-scout) is **cluster discovery + render-target based**. Requires a ConfigHub target + render-target already configured. Renders the discovered state and writes to ConfigHub.

They solve different problems. cub-scout is for **adoption** (first-time preview/import); `cub gitops import` is for **ongoing import** through a configured render pipeline. Don't claim they overlap.

### apply boundary

`cub-scout import apply` IS the mutating write. The user runs it directly; this skill describes its shape but does not invoke it. Defense in depth: not in `allowed-tools`.

## Tool boundary

- **Allowed (agent-callable preview):**
  - `cub-scout import --git-path *` — preview-only by construction (`--git-path` forces `--dry-run` in `cmd/cub-scout/import.go`)
  - `cub-scout import parse-repo *` — pure parser
  - `cub-scout import cluster-aggregator *` — emits proposal as JSON
  - `cub-scout app list *` — read-only ConfigHub App inventory
  - `map workloads`, `scan`, `status`; `cub * get/list`, `cub unit list`, `cub gitops discover`; `argocd app get`, `argocd appset get/list`, `flux get`
- **Not allowed (user-driven only):**
  - `cub-scout import apply *` — writes ConfigHub units
  - `cub-scout import argocd *` — without `--dry-run` it creates units, can disable Argo auto-sync, can delete the Argo Application (`cmd/cub-scout/import_argocd.go:465-488`). The wildcard cannot enforce `--dry-run`, so the verb is out of allowed-tools entirely; recommend the user run `cub-scout import argocd <app> --dry-run` themselves and feed the proposal to the agent for review.
  - `cub-scout app create *` — `app list` is the read-only form in allowed-tools
  - `cub * create/update/delete`, `cub gitops import` (different tool), `kubectl apply/edit/patch/delete`

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
- Save proposal JSON for review today. A future `import --git-path --output-dir` flow is tracked separately for emitting proposed unit YAMLs directly.
- For deeply-nested Helm or templated structure, the parser produces lower-confidence proposals; surface those explicitly. cub-scout's import is for **adoption**, not for renderer reconstruction.
- Helm / Kustomize back-resolution at file:line precision is currently raw-YAML only (#440 stage B); templated sources fall back to resource-level anchors.
