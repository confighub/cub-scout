---
name: cub-scout
description: Use when working in the cub-scout repo or when answering capability-assistant questions ("can cub-scout do X?", "should I use cub-scout or kubectl here?", "how does cub-scout differ from cub?"). This is the umbrella router — it points you at the verb-grouped scenario skills under `skills/scout-*/` and the workflow / observer / reference skills planned for batches 2–5. For a specific verb-group task (observe / diagnose / compare / attribute / adopt existing config / govern / integrate / verify) load the corresponding `scout-*` skill directly.
phase: cross-cutting
allowed-tools: Bash(./cub-scout --help) Bash(./cub-scout * --help) Bash(cub-scout --help) Bash(cub-scout * --help) Bash(cub scout --help) Bash(cub scout * --help)
---

# cub-scout (umbrella router)

This skill is the cross-cutting entry point. It picks the right verb-grouped skill for a user's question and explains the cub-scout / `cub` boundary. For specific tasks, load the scenario skill directly.

## When to use

- "Can cub-scout do X?" / "what can cub-scout observe / diagnose / compare?"
- "How does cub-scout differ from cub / kubectl / Argo / Flux?"
- "Should I use cub-scout or [other tool] for this?"
- Anything in this repo where the verb group isn't obvious from the prompt
- General capability-assistant or demo conversations

## Do not load for

- A specific verb-grouped task — load the corresponding `scout-*` skill directly:
  - [`scout-observe`](../scout-observe/SKILL.md) — see what's running
  - [`scout-diagnose`](../scout-diagnose/SKILL.md) — interpret + recommend
  - [`scout-compare`](../scout-compare/SKILL.md) — intended vs actual
  - [`scout-attribute`](../scout-attribute/SKILL.md) — provenance of field values
  - [`scout-ingest`](../scout-ingest/SKILL.md) — adopt existing config into ConfigHub with preview first
  - [`scout-govern`](../scout-govern/SKILL.md) — connected governance signals
  - [`scout-mcp`](../scout-mcp/SKILL.md) — MCP gateway + context-pack
  - [`scout-verify`](../scout-verify/SKILL.md) — typed, fingerprinted evidence receipts
- ConfigHub authoring or any mutating workflow — load [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills) skills (`cub-mutate`, `cub-apply`, etc.)
- Live cluster mutation — cub-scout never mutates; route to `kubectl` (user driven) or a `cub` skill

## Capability map (verb groups → skills)

| Group | Skill | Status |
|---|---|---|
| Observe | [`scout-observe`](../scout-observe/SKILL.md) | shipped (batch 1) |
| Diagnose | [`scout-diagnose`](../scout-diagnose/SKILL.md) | shipped (batch 1) |
| Compare | [`scout-compare`](../scout-compare/SKILL.md) | shipped (batch 1) |
| Attribute | [`scout-attribute`](../scout-attribute/SKILL.md) | shipped (batch 1) |
| Govern | [`scout-govern`](../scout-govern/SKILL.md) | shipped (batch 2) |
| Adopt Existing Config | [`scout-ingest`](../scout-ingest/SKILL.md) | shipped (batch 2) |
| Integrate | [`scout-mcp`](../scout-mcp/SKILL.md) | shipped (batch 2) |
| Verify | [`scout-verify`](../scout-verify/SKILL.md) | shipped (batch 2, consuming the `#446` receipt capability) |

See [`skills/README.md`](../README.md) for the full plan including controller observer skills (`observe-argocd`, `observe-flux`, `observe-helm`, `observe-crossplane`, `observe-kro`), workflow scenario skills (`triage-unhealthy-workload`, `investigate-drift`, `audit-fleet-conformance`, etc.), and shared references.

### Pilot–cub-scout integration scenarios (`#444`)

The consumer-side complement: same cub-scout verbs framed around **Pilot** (the acceptance judge) rendering verdicts from cub-scout's evidence. Load one of these instead of a `scout-*` skill when the user's prompt is shaped as "Pilot, decide ..." or "render the verdict ...". `#444` is complete: 9 scenarios shipped across batches A and B.

| Scenario | Skill | Pilot consumes | Pilot decides |
|---|---|---|---|
| Pre-deploy CD gate | [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md) | `compare source-truth --strategy <s>` + optional `receipt verify --fail-on any-non-pass` | PASS / WATCH / ASK / BLOCK on the release |
| Fleet-wide conformance | [`pilot-fleet-conformance`](../pilot-fleet-conformance/SKILL.md) | `compare three-way --view` + per-resource source-truth + `fleet outliers` | Fleet verdict + per-resource breakdown |
| Drift classification | [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md) | attribution evidence (`cause` / `managerHint` / `gitSource` / `bindingSource`) | revert / quarantine / accept-as-canonical / ASK |
| Real-time watch response | [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md) | `cub-scout watch` event stream with `--emit-receipt-on` inline receipts (v2 #463) | Per-event PASS / WATCH / ASK / BLOCK |
| Incident close-out | [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md) | trace + explain + compare + bundle + history + audit + chained receipts (`--input-attestation`, v2 #463) | Incident verdict + immutable evidence pack |
| Rollback decision | [`pilot-rollback-decision`](../pilot-rollback-decision/SKILL.md) | `history` + `impact` + per-candidate `receipt verify --at-commit <sha>` chained into chosen-target receipt | Rollback target SHA + safety verdict |
| Promotion gate | [`pilot-promotion-gate`](../pilot-promotion-gate/SKILL.md) | `compare three-way` per variant + `bindingSource` graph diff + chained variant-A receipt | Cross-variant PASS / WATCH / ASK / BLOCK with diff reasons |
| Compliance audit | [`pilot-compliance-audit`](../pilot-compliance-audit/SKILL.md) | scope-wide source-truth + `scan` + `audit list` + per-resource saved receipts | Compliance report (Compliant / with-caveats / Non-compliant / Insufficient-evidence) + fingerprinted evidence inventory |
| Release verification | [`pilot-release-verification`](../pilot-release-verification/SKILL.md) | `compare three-way` + `history --since <deploy-time>` + `receipt verify --at-commit <release-sha>`; `summary.agreement` drives convergence signal | Post-deploy PASS / WATCH (wait) / BLOCK / ASK |

## Product value in one breath

**cub scout observes and explains; it never decides.** It is the read-only Kubernetes and GitOps observer — it surfaces evidence about ownership, health, drift, and provenance, but it never mutates the cluster and never makes authority calls about what *should* be true. ConfigHub (driven by `cub`) is the authority; cub-scout is the witness.

`cub scout` is the preferred documented form. In this repo, use `./cub-scout ...` for exact local commands.

## Tool boundaries

### Use `cub scout` for

- **Observe** — `doctor`, `map`, `trace`, `tree`, `scan`, `graph`, `snapshot`, `watch`, `status`
- **Diagnose** — `explain`, `debug`, `suggest-remedy`, `patterns`, `gitops status`
- **Compare** — `compare drift`, `compare three-way`, `compare source-truth`, `compare <kind>/<name>`
- **Attribute** — read `cause` / `managerHint` / `gitSource` / `bindingSource` on `compare` and `explain` JSON
- **Adopt Existing Config** (preview first) — `import --dry-run`, `import --from-bundle`, `import --git-path`, `import parse-repo`, `import argocd`, `import cluster-aggregator`
- **Govern** (connected) — `history`, `impact`, `fleet outliers`, `summary`, `views resolve`, `audit list`, `bundle inspect/diff/timeline`, `catalog list`
- **Integrate** — `mcp serve`, `context-pack`
- **Verify** — `receipt verify / show / validate / list` (typed, fingerprinted evidence; `#446` v1 complete)

### Use `cub` for

- ConfigHub intended-state workflows
- spaces, units, targets, workers
- `cub gitops discover` / `cub gitops import`
- `cub link list / get` (the data feed for cub-scout's connected attribution layer)
- Any mutation — `cub` is the writer side of the triad

### Do not blur these

- `cub-scout import --git-path` is a local structure / import-preview flow — it does *not* render manifests or upload to ConfigHub
- `cub gitops import` is target + render-target based — it does both
- SDK renderers are an implementation detail for `cub`, not an implied cub-scout feature
- Attribution evidence on `compare` / `explain` JSON is read-only enrichment — never implies a write

## High-signal shipped capabilities

- **Attribution layer** (#435 — A1+A1.5+A2 in #437, C1 in #438, C2 in #439, stage B in #440): `cause` + `managerHint` + `gitSource{repoUrl,revision,path,file,line}` + `bindingSource` on every field mismatch. See [`scout-attribute`](../scout-attribute/SKILL.md).
- **Source-truth contract** (#393 + #418): `compare source-truth` with Phase 1 + Phase 2 strategies (9 total).
- **Views integration** (#391): `views resolve`, `views open`, `views project --with-reality`, `compare three-way --view`.
- **`doctor` / `explain`** with `--presentation` and `--hint-mode`.
- **MCP gateway** (`mcp serve`): standalone + connected tool sets.
- **Stage B back-resolution** (#440): `compare three-way --source-path <local-checkout>` populates `gitSource.file:line` for raw YAML manifests.
- **Receipt capability** (#446 — v1 complete; #454 + #455 + #456): typed, fingerprinted, immutable evidence artifacts wrapping cub-scout evidence into an in-toto Statement v1 envelope. Three predicates: `applied-matches-spec`, `source-truth-pass`, `no-manual-edits-since`. `verify` / `show` / `validate` / `list` + local store with immutable canonical filenames. See [`scout-verify`](../scout-verify/SKILL.md).

## Read first (for capability-assistant work)

1. [`AI-README-FIRST.md`](../../AI-README-FIRST.md) — cold-start guide, current shipped capabilities, current open queue
2. [`HANDOVER.md`](../../HANDOVER.md) — latest execution snapshot
3. [`README.md`](../../README.md) § "Capability Map" — the eight verb groups
4. [`docs/reference/commands.md`](../../docs/reference/commands.md) — exact command surface
5. [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md) — JSON shapes
6. `references/capability-assistant.md` (in this directory) — the original capability-assistant profile, kept for back-compatibility

## Verification rule

Do not invent command surfaces.

Verify from local help before claiming capability:

```bash
./cub-scout --help
./cub-scout doctor --help
./cub-scout explain --help
./cub-scout compare three-way --help
./cub-scout compare source-truth --help
./cub-scout import --help
./cub-scout views --help
./cub-scout mcp serve --help
```

When the workflow crosses into ConfigHub:

```bash
cub gitops --help
cub gitops import --help
cub link --help
cub link list --help
```

## Safety rule (read-only triad lock — #410 / #428)

- `cub-scout` is cluster read-only by default
- connected import writes inventory / state to ConfigHub, **not** cluster manifests
- prefer preview or dry-run paths first
- attribution evidence is enrichment, never mutation
- `suggest-remedy` describes a fix; it does not apply it (the executor was removed in #428)

## Authoring guidance for new skills

When adding skills under `skills/`, follow:

- [`skills/SKILL_TEMPLATE.md`](../SKILL_TEMPLATE.md) — the canonical template (cub-scout, read-only variant)
- The read-only-triad invariant — no mutating patterns in `allowed-tools`, ever
- Standalone-first worked examples; connected-mode is the enrichment
- CI-tool-neutral wording — no GitHub Actions / GitLab CI / Jenkins-specific syntax committed
