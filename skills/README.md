# cub-scout skills

Repo-local Claude / Codex / agent skills for working with cub-scout — the read-only Kubernetes + GitOps observer.

Modeled on the format used in [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills) for the `cub` CLI, adapted for cub-scout's **read-only-triad-locked** position (cub-scout observes; `cub` and other tools act).

If your AI host supports repo-local skills, load these alongside `AI-README-FIRST.md` when working in this repo or against a real cluster.

## Skill map

Skills are grouped along the **eight verb groups** from [`README.md`](../README.md#capability-map) (Observe / Diagnose / Compare / Attribute / Govern / Adopt Existing Config / Integrate / Verify), plus controller-specific observer skills, workflow scenario skills, Pilot consumer scenarios, and shared references.

### Verb-grouped scenario skills

One skill per cub-scout verb group. The skill knows which commands belong to its group and how to pick between them. Most user prompts land here first.

| Skill | Verb group | Covers |
|---|---|---|
| [`scout-observe/`](scout-observe/SKILL.md) | Observe | `doctor` / `map` / `trace` / `tree` / `scan` / `graph` / `snapshot` / `watch` / `status` |
| [`scout-diagnose/`](scout-diagnose/SKILL.md) | Diagnose | `explain` / `debug` / `suggest-remedy` / `patterns` / `gitops status` |
| [`scout-compare/`](scout-compare/SKILL.md) | Compare | `compare` / `compare drift` / `compare three-way` / `compare source-truth` |
| [`scout-attribute/`](scout-attribute/SKILL.md) | Attribute | `cause` / `managerHint` / `gitSource` / `bindingSource` on `compare` + `explain` JSON |
| [`scout-govern/`](scout-govern/SKILL.md) | Govern | `history` / `impact` / `fleet` / `summary` / `views` / `audit` / `bundle` / `catalog` |
| [`scout-ingest/`](scout-ingest/SKILL.md) | Adopt Existing Config | `import --dry-run` / `import --from-bundle` / `import --git-path` / `import argocd` / `import cluster-aggregator` / `import parse-repo` / `app` (preview-first — `import apply` is intentionally out of band) |
| [`scout-mcp/`](scout-mcp/SKILL.md) | Integrate | `mcp serve` / `context-pack` (AI gateway) |
| [`scout-verify/`](scout-verify/SKILL.md) | Verify | `cub-scout receipt verify / show / validate / list` + `watch --emit-receipt-on` (typed, fingerprinted evidence — `#446` v1 + v2 feature-complete: `--fail-on`, `--input-attestation` chained, `--scope` aggregate-with-discovery, real-time emission) |

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

Each covers a real operator / agent workflow that spans multiple verbs. Compose verb-group skills (Observe / Diagnose / Compare / Attribute / Govern / Adopt Existing Config / Integrate / Verify) into a single situation-shaped loop.

- [`triage-unhealthy-workload/`](triage-unhealthy-workload/SKILL.md) — doctor → explain → trace under pager-time pressure; the 30-second loop
- [`investigate-drift/`](investigate-drift/SKILL.md) — compare three-way + attribution (controller-drift vs manual-edit), per-field cause classification
- [`audit-fleet-conformance/`](audit-fleet-conformance/SKILL.md) — compare three-way `--view` + source-truth + fleet outliers + optional receipt persistence
- [`prepare-for-confighub/`](prepare-for-confighub/SKILL.md) — live-cluster import preview + optional repo preview before ever invoking `import apply`
- [`migrate-from-kubectl/`](migrate-from-kubectl/SKILL.md) — find manual edits via attribution, risk-rank with scan, plan per-resource revert/port/accept, capture baseline receipts
- [`ai-agent-readonly-context/`](ai-agent-readonly-context/SKILL.md) — MCP gateway + context-pack + `--presentation ai` for Claude / Codex / Cursor / Continue integration with the read-only invariant
- [`operator-incident-evidence/`](operator-incident-evidence/SKILL.md) — postmortem evidence package: trace + compare + bundle + history + audit + fingerprinted receipts
- [`confighub-source-truth/`](confighub-source-truth/SKILL.md) — strategy-typed `compare source-truth` verdicts (one of 9 strategies; cub-scout never infers); Pilot-acceptance-shaped output

### Pilot–cub-scout integration scenarios (`#444`)

The consumer-side complement to the verb-grouped + workflow skills above. Same cub-scout verbs, framed around **Pilot** (the acceptance judge) reading cub-scout's evidence and rendering verdicts that downstream consumers (CI/CD, dashboards, audit tickets, postmortems) act on. cub-scout produces evidence with documented `omissions[]`; Pilot decides. Pilot mutates via `cub` / Argo / Flux / kubectl — never via cub-scout.

Batch A (CD / observability / event-driven; 5 scenarios — shipped in `#466` + Codex round-7 cleanup in `#467`):

- [`pilot-cd-gate/`](pilot-cd-gate/SKILL.md) — pre-deploy gate in a CD pipeline; consumes `compare source-truth --strategy <s>` (+ optional `receipt verify --fail-on any-non-pass`); renders PASS / WATCH / ASK / BLOCK on the release
- [`pilot-fleet-conformance/`](pilot-fleet-conformance/SKILL.md) — fleet-wide conformance verdict (View / namespace / cluster scope) composing `compare three-way --view` + per-resource `compare source-truth` + `fleet outliers`; judge-driven counterpart to `audit-fleet-conformance`
- [`pilot-patch-and-drift/`](pilot-patch-and-drift/SKILL.md) — drift-classification + revert / quarantine / accept-as-canonical / ASK decision; consumes attribution layer (`cause` / `managerHint` / `gitSource` / `bindingSource`); judge-driven counterpart to `investigate-drift`
- [`pilot-watch-alert-response/`](pilot-watch-alert-response/SKILL.md) — real-time event-driven response; consumes the `cub-scout watch` event stream with inline receipts via `--emit-receipt-on` (shipped in `#463`); the only event-driven skill in the pilot-* batch
- [`pilot-incident-evidence/`](pilot-incident-evidence/SKILL.md) — incident close-out evidence pack with **chained receipts** (`--input-attestation` from `#463`) for multi-stage incidents; judge-driven counterpart to `operator-incident-evidence`

Batch B (governance-shaped; 4 scenarios):

- [`pilot-rollback-decision/`](pilot-rollback-decision/SKILL.md) — choose the rollback target SHA + render a safety verdict; consumes `history` + `impact` + per-candidate `receipt verify --at-commit <sha>`; chains candidate receipts into a chosen-target receipt
- [`pilot-promotion-gate/`](pilot-promotion-gate/SKILL.md) — cross-variant promotion-safety verdict (staging → prod; canary 5% → 50%; blue → green; cell-A → cell-B); consumes `compare three-way` per variant + `bindingSource` graph diff; chains variant-A receipt into the promotion-target receipt
- [`pilot-compliance-audit/`](pilot-compliance-audit/SKILL.md) — periodic policy-conformance report (quarterly / monthly) with fingerprinted evidence inventory; consumes scope-wide `compare source-truth` + `scan` + `audit list`; compliance-vocabulary translation layer over receipt verdicts
- [`pilot-release-verification/`](pilot-release-verification/SKILL.md) — post-deploy validation gate (companion to `pilot-cd-gate`'s pre-deploy half); consumes `compare three-way` + `history --since <deploy-time>` + `receipt verify --at-commit <release-sha>`; `summary.agreement` field drives the convergence signal

### Shared references

| Reference | Purpose |
|---|---|
| [`references/kubernetes-managedfields.md`](references/kubernetes-managedfields.md) | The data substrate for attribution — what `metadata.managedFields` carries, how cub-scout reads it, what's lost in older clusters |
| [`references/verified-manager-strings.md`](references/verified-manager-strings.md) | The enumeration of known field-manager strings (Argo / Flux / Helm / Crossplane / kro / kubectl) with upstream citations |
| [`references/source-truth-strategies.md`](references/source-truth-strategies.md) | The 9 source-truth strategies (post-#418) — when to use which, the four-status / five-verdict contract, proof-gap catalog |
| [`references/standalone-vs-connected.md`](references/standalone-vs-connected.md) | The mode axis — what works without ConfigHub, what unlocks with `cub auth login`, the graceful-degradation rule |
| [`references/read-only-triad.md`](references/read-only-triad.md) | cub-scout / ConfigHub / Pilot role separation (#410 / #428); three-layer code-enforced invariant |
| [`references/plugin-vs-standalone.md`](references/plugin-vs-standalone.md) | `cub scout` (plugin) vs `cub-scout` (standalone binary) invocation parity; v2.0.0 switchover plan |
| [`references/argocd-applicationset.md`](references/argocd-applicationset.md) | ApplicationSet generators (git directories, list, clusters, matrix, merge); full-path slugs; exclude patterns |
| [`references/flux-source-types.md`](references/flux-source-types.md) | GitRepository / HelmRepository / OCIRepository / Bucket / HelmChart; the two-stage delivery chain; source-truth anchors |
| [`references/mcp-tool-catalog.md`](references/mcp-tool-catalog.md) | Complete MCP tool catalog — 5 standalone + 5 connected, per-tool parameters and return shape |

### Umbrella router

[`cub-scout/SKILL.md`](cub-scout/SKILL.md) is the original umbrella skill — kept as a slim router that points readers at the verb-grouped scenario skills above. Load that first for general capability-assistant questions ("can cub scout do X?").

## Authoring rules

All skills under `skills/` follow the read-only-triad invariant. Concretely:

- Every skill's `allowed-tools` line stays inside #410/#428 — see [`SKILL_TEMPLATE.md`](SKILL_TEMPLATE.md) for the canonical Read set and the explicit "never grant" list.
- Skills never recommend `kubectl apply/edit/patch/delete`, `argocd app sync` (as mutation), `cub * create/update/delete`, or any mutating pattern. If the user wants to act, hand off to a `cub` skill in [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills) or to direct `kubectl` with the user driving.
- Standalone-mode comes first in worked examples; connected-mode is the enrichment.
- CI-tool-neutral wording — no GitHub Actions / GitLab CI / Jenkins-specific syntax in committed examples; show the shell command and let users adapt.

## Status

**`#442` complete (`batch 5` shipped)**: the seven remaining shared references are in — `source-truth-strategies`, `standalone-vs-connected`, `read-only-triad`, `plugin-vs-standalone`, `argocd-applicationset`, `flux-source-types`, `mcp-tool-catalog`. Together with the 2 from batch 1, all 9 references are now available.

**Final tally:**
- **Scaffolding + 1 router + 1 umbrella** — `SKILL_TEMPLATE.md`, `skills/README.md`, `skills/cub-scout/SKILL.md`
- **Verb-group scenario skills (8)** — Observe, Diagnose, Compare, Attribute (batch 1) + Adopt Existing Config, Govern, Integrate, Verify (batch 2)
- **Controller-observer skills (7)** — argocd, flux, helm, crossplane, kro, confighub-managed, native (batch 3)
- **Workflow scenario skills (8)** — triage-unhealthy-workload, investigate-drift, audit-fleet-conformance, prepare-for-confighub, migrate-from-kubectl, ai-agent-readonly-context, operator-incident-evidence, confighub-source-truth (batch 4)
- **Shared references (9)** — kubernetes-managedfields, verified-manager-strings (batch 1) + source-truth-strategies, standalone-vs-connected, read-only-triad, plugin-vs-standalone, argocd-applicationset, flux-source-types, mcp-tool-catalog (batch 5)

That's **~33 skill files** plus 9 references — the full scope from `#442`.

**Five PRs shipped this set:**
- [`#452`](https://github.com/confighub/cub-scout/pull/452) — batch 1 scaffolding + 4 verb-group skills + 2 references
- [`#457`](https://github.com/confighub/cub-scout/pull/457) — batch 2 remaining 4 verb-group skills
- [`#458`](https://github.com/confighub/cub-scout/pull/458) — batch 3 controller-observer skills
- [`#459`](https://github.com/confighub/cub-scout/pull/459) — batch 4 workflow scenario skills
- batch 5 — this PR (closes `#442`)

**`#444` complete**: 9 Pilot–cub-scout integration scenario skills shipped across two batches. Each frames the same cub-scout verbs from Pilot's perspective (acceptance judge reading evidence, rendering verdicts), with worked CLI examples + verdict-mapping tables. The watch-alert-response skill consumes the v2 `--emit-receipt-on` surface (shipped in `#463`); the incident-evidence + rollback-decision + promotion-gate skills consume the v2 chained-receipts surface (`--input-attestation`, also `#463`).

PRs:
- [`#466`](https://github.com/confighub/cub-scout/pull/466) — batch A (5 scenarios: CD / observability / event-driven)
- [`#467`](https://github.com/confighub/cub-scout/pull/467) — batch A Codex round-7 fixes (enumerated `compare *` allowed-tools; snake_case source-truth JSON shape; strategy enum corrected; etc.)
- [`#468`](https://github.com/confighub/cub-scout/pull/468) — batch B (4 governance scenarios: rollback-decision / promotion-gate / compliance-audit / release-verification) — **closed `#444`**

Codex round-7 learnings applied upfront in batch B: enumerated `compare three-way / compare drift / compare source-truth` allowed-tools (no broad `compare *`); snake_case source-truth fields (`declared_strategy` / `source_truth` / `proof_gaps`); no invented Pilot CLI surfaces; abstract mutation paths; receipt verdicts only PASS/WATCH/BLOCK/INCONCLUSIVE (ASK is source-truth `status`, maps to receipt `WATCH` when wrapped).

## Related receipts work (sibling closures)

The receipts capability the `pilot-*` skills consume reached feature-complete state alongside `#444`:

- ~~`#448`~~ closed — chained half via `#463`, aggregate half via `#469` (`--scope namespace/<ns>` + `synthetic-aggregate://` subject + max-severity verdict synthesis)
- ~~`#449`~~ closed — v1 (drift.detected + ownership.changed) in `#463`; full 4-event-type set + per-poll `--emit-receipt-batch-cap` backpressure in `#470`
- ~~`#451`~~ closed via `#463` — `receipt verify --fail-on <verdict>` exit semantics

The `scout-verify` skill (verb-group) and the `pilot-*` skills (consumer-side) cover the operator-side and judge-side of this surface respectively. End-to-end tutorial at [`docs/howto/receipts-end-to-end.md`](../docs/howto/receipts-end-to-end.md); dedicated watch event reference at [`docs/reference/watch-events.md`](../docs/reference/watch-events.md).
