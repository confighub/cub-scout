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
| [`scout-ingest/`](scout-ingest/SKILL.md) | Ingest | `import --git-path` / `import argocd` / `import cluster-aggregator` / `import parse-repo` / `app` (preview-only — `import apply` is intentionally out of band) |
| [`scout-govern/`](scout-govern/SKILL.md) | Govern | `history` / `impact` / `fleet` / `summary` / `views` / `audit` / `bundle` / `catalog` |
| [`scout-mcp/`](scout-mcp/SKILL.md) | Integrate | `mcp serve` / `context-pack` (AI gateway) |
| [`scout-verify/`](scout-verify/SKILL.md) | Verify | `cub-scout receipt verify / show / validate / list` (typed, fingerprinted evidence — `#446` v1 complete) |

### Controller observer skills

One skill per controller cub-scout detects. Each documents the specific labels / annotations / manager strings the controller writes, the ownership classification cub-scout produces, and the edge cases. Verified against `pkg/agent/ownership.go` and `pkg/agent/manager_strings.go` — the constants are the single source of truth.

- [`observe-argocd/`](observe-argocd/SKILL.md) — Applications, ApplicationSets, tracking-id annotation, CSA migration default
- [`observe-flux/`](observe-flux/SKILL.md) — GitRepository / HelmRelease / Kustomization / OCIRepository / Bucket / source-controller
- [`observe-helm/`](observe-helm/SKILL.md) — direct Helm vs Flux-helm-controller vs Argo-helm-renderer (managed-by + chart label disambiguation matrix)
- [`observe-crossplane/`](observe-crossplane/SKILL.md) — XR / composed / claim / MRD / provider managed-resources, ProviderConfig secrets, control-plane subset
- [`observe-kro/`](observe-kro/SKILL.md) — applyset / parent / labeller, kro.run API group
- [`observe-confighub-managed/`](observe-confighub-managed/SKILL.md) — UnitSlug label, delivered-via-Argo vs delivered-via-Flux, ConfigHub-priority detection
- [`observe-native/`](observe-native/SKILL.md) — OwnerType=k8s (OwnerReferences chain) vs OwnerType=unknown (terminal fallthrough), orphan detection

### Workflow scenario skills

Each covers a real operator / agent workflow that spans multiple verbs. Compose verb-group skills (Observe / Diagnose / Compare / Attribute / Ingest / Govern / Integrate / Verify) into a single situation-shaped loop.

- [`triage-unhealthy-workload/`](triage-unhealthy-workload/SKILL.md) — doctor → explain → trace under pager-time pressure; the 30-second loop
- [`investigate-drift/`](investigate-drift/SKILL.md) — compare three-way + attribution (controller-drift vs manual-edit), per-field cause classification
- [`audit-fleet-conformance/`](audit-fleet-conformance/SKILL.md) — compare three-way `--view` + source-truth + fleet outliers + optional receipt persistence
- [`prepare-for-confighub/`](prepare-for-confighub/SKILL.md) — import --git-path preview + disk-PR proposal flow before ever invoking `import apply`
- [`migrate-from-kubectl/`](migrate-from-kubectl/SKILL.md) — find manual edits via attribution, risk-rank with scan, plan per-resource revert/port/accept, capture baseline receipts
- [`ai-agent-readonly-context/`](ai-agent-readonly-context/SKILL.md) — MCP gateway + context-pack + `--presentation ai` for Claude / Codex / Cursor / Continue integration with the read-only invariant
- [`operator-incident-evidence/`](operator-incident-evidence/SKILL.md) — postmortem evidence package: trace + compare + bundle + history + audit + fingerprinted receipts
- [`confighub-source-truth/`](confighub-source-truth/SKILL.md) — strategy-typed `compare source-truth` verdicts (one of 9 strategies; cub-scout never infers); Pilot-acceptance-shaped output

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

**Batch 4 shipped (`#442` batch 4)**: the eight workflow scenario skills are in — `triage-unhealthy-workload`, `investigate-drift`, `audit-fleet-conformance`, `prepare-for-confighub`, `migrate-from-kubectl`, `ai-agent-readonly-context`, `operator-incident-evidence`, `confighub-source-truth`. Each composes verb-group skills + controller-observer skills + references into a single situation-shaped loop with worked examples.

**Earlier:**
- Batch 3 landed the seven controller-observer skills (argocd / flux / helm / crossplane / kro / confighub-managed / native), verified against the Go enumeration
- Batch 2 landed the remaining four verb-grouped scenario skills (Ingest / Govern / Integrate / Verify, with `scout-verify` consuming the receipt capability shipped in `#446`)
- Batch 1 landed the scaffolding plus the first four scenario skills (Observe / Diagnose / Compare / Attribute) and two references (managedFields, verified-manager-strings)

**Remaining work:** batch 5 — the remaining 7 shared references (`source-truth-strategies`, `standalone-vs-connected`, `read-only-triad`, `plugin-vs-standalone`, `argocd-applicationset`, `flux-source-types`, `mcp-tool-catalog`). See [`docs/roadmap.md`](../docs/roadmap.md) § "AI Agent Skills" for the full plan.
