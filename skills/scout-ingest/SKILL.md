---
name: scout-ingest
description: 'Use when the user wants to MOVE existing Kubernetes / GitOps workload structure INTO ConfigHub — the Ingest verb group of cub-scout. Natural phrasing: "import this Argo Application set", "bring my Flux Kustomizations into ConfigHub", "discover workloads in this cluster and propose ConfigHub units", "preview what would land in ConfigHub if I imported this repo", "parse my git repo as a ConfigHub import bundle", "import a cluster aggregator", "show me the import proposal before I apply", "what would --git-path produce for this repo?". Load whenever intent is import / ingest / preview / propose / parse-repo / discovery / aggregator / bring-into-confighub / what-would-confighub-see. Do NOT load for: comparing live state to ConfigHub (use scout-compare), inventory of what is currently running (use scout-observe), the actual write to ConfigHub via `cub gitops import` (that is `cub`, not cub-scout — route to cub skills). Important: `cub-scout import apply` IS the mutating write into ConfigHub — this skill teaches the read-only preview path and explicitly refuses to invoke `import apply` itself.'
phase: verify
allowed-tools: Bash(./cub-scout import --git-path *) Bash(cub-scout import --git-path *) Bash(cub scout import --git-path *) Bash(./cub-scout import parse-repo *) Bash(cub-scout import parse-repo *) Bash(cub scout import parse-repo *) Bash(./cub-scout import cluster-aggregator *) Bash(cub-scout import cluster-aggregator *) Bash(cub scout import cluster-aggregator *) Bash(./cub-scout app list *) Bash(cub-scout app list *) Bash(cub scout app list *) Bash(./cub-scout app list) Bash(cub-scout app list) Bash(cub scout app list) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub unit list *) Bash(cub gitops discover *) Bash(argocd app get *) Bash(argocd appset get *) Bash(flux get *)
---

# scout-ingest

The Ingest verb group of cub-scout. Reads cluster + git-repo + controller structure and **proposes** the corresponding ConfigHub unit/space/target tree. Every Ingest verb is **preview-by-default**; the actual write into ConfigHub uses `import apply` and is intentionally NOT in this skill's allowed-tools.

## When to use

Explicit phrasings:

- "Import this Argo Application set into ConfigHub"
- "Bring my Flux Kustomizations into ConfigHub"
- "Discover the workloads in this cluster and propose ConfigHub units"
- "Preview what would land in ConfigHub if I imported this repo"
- "Parse my git repo as a ConfigHub import bundle"
- "Import a cluster aggregator" / "aggregator-pattern multi-cluster import"
- "Show me the import proposal before I apply"
- "What would `--git-path` produce for this repo layout?"
- "Generate a proposed unit YAML I can put in a PR"

Implicit intents:

- The user has *existing* GitOps state (cluster, Argo ApplicationSets, Flux Kustomizations, raw git repo) and wants ConfigHub adoption
- The user is gating on review-before-apply (PR-flow, dry-run mindset)
- The user is migrating off a different tool and wants ConfigHub to mirror current state
- The user is exploring "what would ConfigHub see here" without committing

## Do not load for

- Comparing live state to ConfigHub once units exist — [`scout-compare`](../scout-compare/SKILL.md)
- Inventory of what is currently running (without ConfigHub) — [`scout-observe`](../scout-observe/SKILL.md)
- The actual cluster-side `cub gitops import` (target + render-target based) — that's `cub`, not cub-scout. Route to [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills).
- `cub-scout import apply` (mutating; writes to ConfigHub) — this skill refuses to invoke it directly. Recommend the user run it themselves once they've reviewed the preview.

## Standalone vs connected

- **Standalone (cluster only):** `import argocd` and `import cluster-aggregator` read controller state from the cluster + (optionally) the controller CLI; they produce a proposal document. No ConfigHub auth needed for the preview itself.
- **Standalone (git only):** `import --git-path <repo>` and `import parse-repo <repo>` read a local git checkout and produce a proposal. No cluster, no ConfigHub.
- **Connected (cluster + `cub auth login`):** the proposal is rendered against the connected ConfigHub instance (existing spaces, naming conflicts, etc. are surfaced) and `import apply` (mutating; not in this skill) becomes available to write the proposal.

## Tool boundary

- **Allowed (read-only, agent-callable):**
  - `cub-scout import --git-path *` — local-only flow; the `--git-path` flag forces `--dry-run` in `cmd/cub-scout/import.go`, so this is preview-only by construction
  - `cub-scout import parse-repo *` — pure parser; read-only
  - `cub-scout import cluster-aggregator *` — emits the proposal as JSON / YAML; `import apply` is the separate mutating verb
  - `cub-scout app list *` — read-only ConfigHub App inventory
  - `cub * get/list`, `cub unit list`, `cub gitops discover` for connected-mode context
  - `argocd app get`, `argocd appset get`, `flux get` for source-side structure reads
- **Not allowed (mutating; user-driven):**
  - `cub-scout import apply *` — writes ConfigHub units / spaces / targets
  - `cub-scout import argocd *` — without `--dry-run` it calls `createUnitWithConfigAndLabels` (line 465 in `cmd/cub-scout/import_argocd.go`) and can also `--disable-sync` or `--delete-app`. Because the wildcard cannot enforce `--dry-run`, the verb is **out of allowed-tools entirely**; recommend the user run `cub-scout import argocd <app> --dry-run` themselves and pipe the output back to the agent for review.
  - `cub-scout app create *` — creates ConfigHub Apps; `app list` is the read-only form in allowed-tools
  - `cub * create/update/delete`, `cub gitops import` (cluster-side; different tool boundary — `cub`, not cub-scout), any `kubectl apply/edit/patch/delete`
- **`import apply` boundary:** the skill **describes** `import apply`'s shape and what it would do, but does NOT run it. If the user wants to apply, the skill says "you run `cub-scout import apply --space <s>` yourself after review" — the user is the actor for any mutation.

## The verb menu

| Question | Verb | Notes |
|---|---|---|
| "Preview ConfigHub units from a local git repo" | `cub-scout import --git-path <repo>` | Walks the repo, classifies kustomize / helm / raw manifests, emits a proposal. Use `--output-dir <path>` to land proposed unit YAMLs on disk for PR review. |
| "Parse the repo structure deterministically" | `cub-scout import parse-repo <repo>` | Lower-level parser. Extracts ApplicationSet generator directories, helm chart paths, kustomization roots. Used as a primitive by other verbs. |
| "Import existing Argo Applications / ApplicationSets" | `cub-scout import argocd --space <s>` | Reads Argo state from the cluster and proposes one ConfigHub unit per Application. ApplicationSet generators (git directories, lists, clusters) preserved as hierarchy. |
| "Import via a cluster aggregator" | `cub-scout import cluster-aggregator --space <s>` | Multi-cluster bundle-based import. Aggregates per-cluster snapshots into a single ConfigHub proposal. |
| "Manage ConfigHub Apps (existing units → App)" | `cub-scout app *` | Read-only manipulations of the App/Deployment/Target metadata in ConfigHub. |

## The loop

1. **Identify the source of truth.** Where does the structure come from? A local git checkout? Argo on the cluster? An aggregator snapshot? This determines the verb (see menu above).
2. **Run the preview verb.** Default to `--format json` if the proposal is going into a script or a PR-bot; ASCII if the user is reading it.
3. **Read the proposal.** Each proposed unit has a slug, a space, a path (for ApplicationSet generator origin), and a confidence signal. Naming conflicts with existing ConfigHub state are flagged (connected mode).
4. **Optionally land a disk PR.** `import --git-path --output-dir <path>` writes the proposed unit YAMLs to disk so the user can open a PR for review without ever touching ConfigHub.
5. **Hand off the mutation.** If the user approves the proposal, they run `cub-scout import apply --space <s>` themselves. The skill **does not** invoke it. The mutation is the user's decision, not the agent's.

## Worked examples

### A: preview a local git repo (standalone, no cluster, no ConfigHub)

```bash
$ cub-scout import --git-path ./platform-config --format json
{
  "units": [
    {
      "slug": "apps-prod-api",
      "path": "apps/prod/api",
      "kind": "Deployment",
      "rendererHint": "kustomize",
      "confidence": "high"
    },
    {
      "slug": "apps-prod-worker",
      "path": "apps/prod/worker",
      "kind": "Deployment",
      "rendererHint": "kustomize",
      "confidence": "high"
    }
  ],
  "spaces": ["platform-prod"],
  "targets": ["Kubernetes:prod-use2"]
}
```

### B: preview Argo Application import (connected mode)

```bash
$ cub-scout import argocd --space payments --format json
$ # proposal has one unit per Argo Application,
$ # with ApplicationSet origin preserved as hierarchy
```

### C: write a disk-PR proposal (standalone, no ConfigHub)

```bash
$ cub-scout import --git-path ./platform-config --output-dir ./proposed-units
$ git checkout -b adopt-confighub
$ git add proposed-units/
$ git commit -m "propose ConfigHub units for platform-config"
$ # ...PR review happens in git, not in ConfigHub
```

This is the **safest** workflow: the proposal lives in git, gets reviewed, and only after merge does someone run `cub-scout import apply` to write the units to ConfigHub.

## Output evidence

Every preview returns a structured proposal Pilot or a CI bot can consume:

| Field | Meaning |
|---|---|
| `units[]` | Proposed ConfigHub units, one per workload. Each carries slug, path, kind, rendererHint, confidence. |
| `spaces[]` | Proposed ConfigHub spaces (typically grouped by environment / cluster) |
| `targets[]` | Proposed ConfigHub targets (the K8s clusters / contexts the units would deploy to) |
| `conflicts[]` | (Connected mode) existing ConfigHub names that would clash with the proposal |
| `unsupported[]` | Workloads / resource shapes the parser can't classify — surface as gaps, not silent skips |

## Boundary with `cub gitops import`

cub-scout's `import` verbs are **structure-and-preview** focused:

- `cub-scout import --git-path <repo>` reads a local checkout, classifies it, emits a proposal
- `cub-scout import argocd` reads Argo Applications, emits a proposal
- `cub-scout import apply` writes the proposal to ConfigHub (mutation; out of this skill)

`cub gitops import` (in the `cub` CLI) is **cluster discovery + render-target based**:

- Requires a ConfigHub target + render-target already configured
- Renders the discovered state against the render-target's renderer
- Writes the rendered output to ConfigHub

The two paths solve different problems. cub-scout's import is for **adoption** (existing state → ConfigHub for the first time); `cub gitops import` is for **on-going ingest** (cluster state → ConfigHub via a configured render pipeline). Don't claim they overlap.

## References

- Command reference: [`docs/reference/commands.md`](../../docs/reference/commands.md) § import, § app
- Argo import flow: [`examples/argo-import-confighub-demo/`](../../examples/argo-import-confighub-demo/)
- Flux import flow: [`examples/flux-import-confighub-demo/`](../../examples/flux-import-confighub-demo/)
- Cluster aggregator pattern: [`examples/fleet-import/`](../../examples/fleet-import/)
- ApplicationSet generator support: PR #363 (`cub-scout/#363`)
- `cub` side of the boundary: [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills)

## Constraints

- Receipts are NOT this skill's surface — [`scout-verify`](../scout-verify/SKILL.md) handles typed evidence about a *single resource*. Ingest is about *structural adoption*; the output is a proposal document, not an evidence artifact.
- `import apply` is intentionally not in the allowed-tools. If a downstream agent is wired to apply automatically, that's an agent-policy decision; this skill says "preview, never apply."
- For deeply-nested Helm or templated paths, the parser produces lower-confidence proposals — surface those explicitly, never silently drop them.
