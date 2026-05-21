# cub-scout skills

Repo-local Claude / Codex / agent skills for working with cub-scout — the read-only Kubernetes + GitOps observer.

Modeled on the format used in [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills) for the `cub` CLI, adapted for cub-scout's **read-only-triad-locked** position (cub-scout observes; `cub` and other tools act).

If your AI host supports repo-local skills, load these alongside `AI-README-FIRST.md` when working in this repo or against a real cluster.

## Skill map

Skills are grouped along the **seven verb groups** from [`README.md`](../README.md#capability-map), plus controller-specific observer skills, workflow scenario skills, and shared references.

### Verb-grouped scenario skills

One skill per cub-scout verb group. The skill knows which commands belong to its group and how to pick between them. Most user prompts land here first.

| Skill | Verb group | Covers |
|---|---|---|
| [`scout-observe/`](scout-observe/SKILL.md) | Observe | `doctor` / `map` / `trace` / `tree` / `scan` / `graph` / `snapshot` / `watch` / `status` |
| [`scout-diagnose/`](scout-diagnose/SKILL.md) | Diagnose | `explain` / `debug` / `suggest-remedy` / `patterns` / `gitops status` |
| [`scout-compare/`](scout-compare/SKILL.md) | Compare | `compare` / `compare drift` / `compare three-way` / `compare source-truth` |
| [`scout-attribute/`](scout-attribute/SKILL.md) | Attribute | `cause` / `managerHint` / `gitSource` / `bindingSource` on `compare` + `explain` JSON |
| `scout-ingest/` *(planned, #442 batch 2)* | Ingest | `import --git-path` / `import argocd` / `import cluster-aggregator` / `import apply` / `app` |
| `scout-govern/` *(planned, #442 batch 2)* | Govern | `history` / `impact` / `fleet` / `summary` / `views` / `audit` / `bundle` / `catalog` |
| `scout-mcp/` *(planned, #442 batch 2)* | Integrate | `mcp serve` / `context-pack` (AI gateway) |
| `scout-verify/` *(planned once #446 batch 1 lands)* | Verify | `cub-scout receipt verify / show / validate / list` |

### Controller observer skills *(planned, #442 batch 3)*

One skill per controller cub-scout detects. Each documents the specific labels / annotations / manager strings the controller writes, the ownership classification cub-scout produces, and the edge cases.

- `observe-argocd/` — Applications, ApplicationSets, tracking-id annotation, CSA migration default
- `observe-flux/` — GitRepository / HelmRelease / Kustomization / OCIRepository / Bucket / source-controller
- `observe-helm/` — direct Helm vs Flux-helm-controller vs Argo-helm-renderer (managed-by + chart label disambiguation)
- `observe-crossplane/` — XR / composed / claim / MRD / provider managed-resources, ProviderConfig secrets
- `observe-kro/` — applyset / parent / labeller
- `observe-confighub-managed/` — UnitSlug label, delivered-via-Argo vs delivered-via-Flux
- `observe-native/` — no controller, OwnerReferences only, orphan detection

### Workflow scenario skills *(planned, #442 batch 4)*

Each covers a real operator / agent workflow that spans multiple verbs.

- `triage-unhealthy-workload/` — doctor → explain → trace under time pressure
- `investigate-drift/` — compare three-way + attribution (controller-drift vs manual-edit)
- `audit-fleet-conformance/` — compare three-way `--view` + source-truth across scope
- `prepare-for-confighub/` — import --git-path preview + propose units before connecting
- `migrate-from-kubectl/` — find manual edits via attribution, plan revert / port
- `ai-agent-readonly-context/` — mcp serve + context-pack patterns for Claude / Codex / agents
- `operator-incident-evidence/` — gather evidence for incident postmortem without mutating
- `confighub-source-truth/` — `compare source-truth` strategy verdicts for upstream acceptance

### Shared references

| Reference | Purpose |
|---|---|
| [`references/kubernetes-managedfields.md`](references/kubernetes-managedfields.md) | The data substrate for attribution — what `metadata.managedFields` carries, how cub-scout reads it, what's lost in older clusters |
| [`references/verified-manager-strings.md`](references/verified-manager-strings.md) | The enumeration of known field-manager strings (Argo / Flux / Helm / Crossplane / kro / kubectl) with upstream citations |
| `references/source-truth-strategies.md` *(planned, batch 5)* | The 9 source-truth strategies (post-#418) — when to use which |
| `references/standalone-vs-connected.md` *(planned, batch 5)* | The mode axis — what works without ConfigHub, what unlocks with `cub auth login` |
| `references/read-only-triad.md` *(planned, batch 5)* | cub-scout / Pilot / ConfigHub role separation (#410 / #428) |
| `references/plugin-vs-standalone.md` *(planned, batch 5)* | `cub-scout` vs `cub scout` invocation parity |
| `references/argocd-applicationset.md` *(planned, batch 5)* | ApplicationSet-managed resource handling |
| `references/flux-source-types.md` *(planned, batch 5)* | GitRepository / HelmRepository / OCIRepository / Bucket |
| `references/mcp-tool-catalog.md` *(planned, batch 5)* | Every MCP tool, parameters, return shape |

### Umbrella router

[`cub-scout/SKILL.md`](cub-scout/SKILL.md) is the original umbrella skill — kept as a slim router that points readers at the verb-grouped scenario skills above. Load that first for general capability-assistant questions ("can cub scout do X?").

## Authoring rules

All skills under `skills/` follow the read-only-triad invariant. Concretely:

- Every skill's `allowed-tools` line stays inside #410/#428 — see [`SKILL_TEMPLATE.md`](SKILL_TEMPLATE.md) for the canonical Read set and the explicit "never grant" list.
- Skills never recommend `kubectl apply/edit/patch/delete`, `argocd app sync` (as mutation), `cub * create/update/delete`, or any mutating pattern. If the user wants to act, hand off to a `cub` skill in [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills) or to direct `kubectl` with the user driving.
- Standalone-mode comes first in worked examples; connected-mode is the enrichment.
- CI-tool-neutral wording — no GitHub Actions / GitLab CI / Jenkins-specific syntax in committed examples; show the shell command and let users adapt.

## Status

This batch (#442 batch 1) lands the scaffolding plus the first four verb-grouped scenario skills (Observe / Diagnose / Compare / Attribute) and two references (managedFields, verified-manager-strings). Batches 2–5 add the remaining verb groups, controller observer skills, workflow scenarios, and references. See [`docs/roadmap.md`](../docs/roadmap.md) § "AI Agent Skills" for the full plan.
