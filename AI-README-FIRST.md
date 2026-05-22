# AI Read Me First

This is the repo-specific cold-start guide for Claude, Codex, and other AI coding agents.

Preferred wording in AI/product prose: `cub scout`.
When showing exact local repo commands in this repo, use `./cub-scout ...`.

If your AI host supports repo-local skills, load [skills/cub-scout/SKILL.md](skills/cub-scout/SKILL.md) after this file.

If you are starting work in this repo, read files in this order:

1. `AI-README-FIRST.md`
2. `HANDOVER.md`
3. `CLAUDE.md`
4. `docs/reference/commands.md`
5. `docs/reference/cli-contract.md`
6. `docs/reference/json-contracts.md`

Use these for different AI scenarios:
- `skills/cub-scout/references/capability-assistant.md` — capability-assistant profile (answering "can cub scout do X?")
- `docs/ai/cub-scout-tasks.md` — task skill for *using* cub scout to investigate a real cluster
- `docs/howto/using-cub-scout-from-ai-tool.md` — demo/operator flows

## Capability Map at a Glance

cub-scout commands fall into eight groups. Each group's **inputs** column tells you whether a command needs only a cluster (standalone), needs a local git checkout, or needs ConfigHub auth (connected). See [README.md § Capability Map](README.md#capability-map) for the full per-command table — this section is the navigation aid.

| Group | What it answers | Notable commands |
|---|---|---|
| **Observe** | What's running, who owns it | `doctor`, `map`, `trace`, `tree`, `scan`, `graph`, `snapshot`, `watch`, `status` |
| **Diagnose** | What's wrong, what to do next | `explain`, `debug`, `suggest-remedy`, `patterns`, `gitops status` |
| **Compare** | Intended vs actual | `compare`, `compare drift`, `compare three-way`, `compare source-truth` |
| **Attribute** | Where each field's value came from | per-field `cause`/`managerHint`/`gitSource`/`bindingSource` on `compare` + `explain` |
| **Ingest** | How to bring config into ConfigHub | `import --git-path`, `import parse-repo`, `import argocd`, `import cluster-aggregator`, `import apply`, `app` |
| **Govern** | Connected history, fleet, views | `history`, `impact`, `fleet outliers`, `summary`, `views`, `audit`, `bundle`, `catalog` |
| **Integrate** | Setup + AI gateway | `setup`, `quickstart`, `mcp serve`, `context-pack`, `version` |
| **Verify** | Typed, fingerprinted evidence artifacts | `receipt verify`, `receipt show`, `receipt validate`, `receipt list` |

When a user asks "can cub scout do X?", first locate X in this map, then verify the exact flag surface with local `--help` (see [Quick Reality Checks](#quick-reality-checks)).

## Tool Boundaries

### `cub scout` / `cub-scout`

**`cub scout` observes and explains; it never decides.** It is the read-only Kubernetes and GitOps observer — it surfaces evidence, never mutates the cluster, and never makes authority calls about what *should* be true.

`cub scout` is the preferred documented form. In this repo, local command examples still use `./cub-scout ...`.

Use it for:
- Observe: `doctor`, `map`, `trace`, `tree`, `scan`, `graph`, `snapshot`, `watch`, `status`
- Diagnose: `explain`, `debug`, `suggest-remedy`, `patterns`, `gitops status`
- Compare: `compare three-way`, `compare source-truth`, `compare drift`, `compare` (resource mode)
- Attribute (read-only enrichment surfacing on `compare` + `explain` JSON)
- Local Git structure discovery with `import --git-path` and `import parse-repo`
- Govern (connected): `history`, `impact`, `fleet outliers`, `summary`, `views resolve`, `audit list`, `bundle inspect/diff/timeline`, `catalog list`
- Verify: `receipt verify / show / validate / list` (typed, fingerprinted evidence — v1 complete)
- Model Context Protocol (MCP) serving via `mcp serve`

Important:
- `cub scout` is cluster read-only by default
- connected imports write inventory/state to ConfigHub, not cluster manifests
- `import --git-path` is a structure/import-preview path, not a manifest renderer
- attribution evidence (`cause`, `managerHint`, `gitSource`, `bindingSource`) is enrichment of existing read-only evidence; it never implies a write

### `cub`

`cub` is the ConfigHub CLI for intended-state workflows.

Use it for:
- spaces, units, targets, workers, and ConfigHub state
- `cub gitops discover`
- `cub gitops import`
- `cub link list / get` (the data feed for cub-scout's connected attribution layer)

Current local CLI truth:
- `cub gitops discover --space <space> <target-slug>`
- `cub gitops import --space <space> <target-slug> <render-target-slug>`

Important:
- `cub gitops import` is target + render-target based
- it is not a local `--git-path` renderer

### `confighub/sdk`

The SDK contains renderer and bridge implementation details used by `cub`.

Do not claim that `cub scout` can do SDK/renderer work locally unless the current repo code and CLI help actually expose that path.

## Current High-Signal Shipped Capabilities

Current release: **v2.2.1**. As of 2026-05-22, these areas are fully or materially shipped:

- **Receipts v1** (`#446` — closed; PRs `#454` + `#455` + `#456` + Codex round-5 `#461`)
  - Typed, fingerprinted, immutable evidence artifacts wrapping the existing attribution + source-truth + gitSource evidence
  - In-toto Statement v1 envelope wrapping `https://cub-scout.dev/receipt/v1`; SHA-256 over RFC 8785 canonical JSON via `gowebpki/jcs`; dual subjects (`k8s-live://` + `confighub-unit://` when connected)
  - Three v1 predicates: `applied-matches-spec` (Argo/Flux/ConfigHub anchor matches LIVE), `source-truth-pass` (with explicit `--strategy`), `no-manual-edits-since` (with explicit `--since` RFC 3339 cutoff)
  - Four verdicts: PASS / WATCH / BLOCK / INCONCLUSIVE
  - CLI: `receipt verify / show / validate / list`; `--save` writes to immutable local store at `$CUB_SCOUT_RECEIPTS_DIR → $XDG_DATA_HOME/cub-scout/receipts → $HOME/.local/share/cub-scout/receipts`
  - Store immutability: `O_EXCL` atomic create; `--out` rejects paths under the store; receipt validate exit codes 0/1/2 via `errors.As` dispatch
  - Read-only-triad guards: `TestReceiptPackageReadOnlyClient` static-greps for mutating K8s client methods; `FilterNextSteps` drops mutating actionType / nextCommand at emit
  - Documented in `docs/reference/json-contracts.md` § Receipt Contract; examples at `examples/receipts/`
- **AI-agent skill catalog** (`#442` — closed; 5 PRs `#452` + `#457` + `#458` + `#459` + `#460`)
  - ~35 skill files + 9 references modeled on `confighub/confighub-skills` for `cub`
  - 8 verb-group skills (`scout-observe`, `scout-diagnose`, `scout-compare`, `scout-attribute`, `scout-ingest`, `scout-govern`, `scout-mcp`, `scout-verify`)
  - 7 controller-observer skills (`observe-argocd`, `observe-flux`, `observe-helm`, `observe-crossplane`, `observe-kro`, `observe-confighub-managed`, `observe-native`) — each verified against `pkg/agent/ownership.go` + `pkg/agent/manager_strings.go`
  - 8 workflow scenario skills (`triage-unhealthy-workload`, `investigate-drift`, `audit-fleet-conformance`, `prepare-for-confighub`, `migrate-from-kubectl`, `ai-agent-readonly-context`, `operator-incident-evidence`, `confighub-source-truth`)
  - 9 shared references (`kubernetes-managedfields`, `verified-manager-strings`, `source-truth-strategies`, `standalone-vs-connected`, `read-only-triad`, `plugin-vs-standalone`, `argocd-applicationset`, `flux-source-types`, `mcp-tool-catalog`)
  - Every skill's `allowed-tools` enumerates read-only verbs only — no broad wildcards
- **Attribution layer** (`#435` — A1, A1.5, A2, B, C1, C2; all stages shipped)
  - `cause` + `managerHint` on every field mismatch — classifies controller-drift vs manual-edit via K8s `managedFields` co-signaled with detected owner
  - Verified manager-string enumeration (`pkg/agent/manager_strings.go`) covers Argo CD, Flux (kustomize/helm/source), Helm direct, Crossplane (composite/composed/claim/MRD/refs), kro (applyset/parent/labeller), `kubectl-*` — strings not in the enumeration return `unknown`
  - Per-field-path resolution via `FieldsV1` decoding (`pkg/agent/field_ownership_paths.go`)
  - `gitSource{repoUrl, revision, path}` resource-level anchor via existing Argo/Flux tracers
  - `gitSource{file, line}` raw-YAML back-resolution via `--source-path <local-checkout>` flag (`pkg/agent/source_back_resolution.go`)
  - Connected `incomingBindings[]` from `cub link list` — surfaces upstream units feeding this unit
  - Connected per-field `bindingSource` — answers "this field's value came from upstream unit X at path Y via link Z"
  - Documented in `docs/reference/json-contracts.md` § Field Mutation Attribution Contract; example at `examples/drift/mutation-cause-attribution/`
- **Source-truth contract** (`#393` Phase 1 + `#418` Phase 2)
  - `compare source-truth <kind>/<name> -n <ns> --strategy <s>` emits the structured JSON Pilot's acceptance kernel consumes
  - **9 strategies**: `confighub-oci-argo`, `confighub-oci-flux`, `git-argo`, `git-flux`, `helm-flux`, `helm-argo`, `kustomize-flux`, `oci-flux`, `oci-argo`
  - Strategy-relative correctness + missing-proof rule enforced in tests
  - 4-status (PASS/WATCH/BLOCK/ASK) × 5-verdict (AGREED/MISMATCH/INCOMPLETE/BLOCKED/UNKNOWN) enum split — these are SEPARATE axes
  - 6-fixture producer suite with byte-equal goldens at `test/fixtures/source-truth/`
- **Architectural triad locked** (`#410` / `#428`)
  - cub-scout = read-only evidence provider
  - Pilot = acceptance judge
  - ConfigHub = authority and workflow engine
  - cub-scout never mutates, repairs, approves, or infers authority
  - `suggest-remedy` (was `remedy`) is categorically read-only — describes but never applies; the executor was removed in `#428`
  - Three layers of enforcement: `scripts/check-readonly.sh` CI gate + `TestReceiptPackageReadOnlyClient` static grep + `FilterNextSteps` runtime filter
- **kstatus migration complete** (`#394`)
  - All readiness derivation flows through `sigs.k8s.io/cli-utils/pkg/kstatus`
  - Same library Argo CD and Flux use upstream
- **Views integration** (`#391`)
  - `views resolve` accepts UUIDs or View Explorer URLs (`#403`)
  - `compare three-way --view <uuid-or-url>` scopes to a ConfigHub View (`#414`)
  - `views project --with-reality` composes View columns with source-truth verdicts (`#420`)
- **MCP gateway** — `mcp serve` exposes a closed, read-only-by-construction tool catalog:
  - 5 standalone tools: `doctor`, `map`, `scan`, `trace`, `explain`
  - 5 connected tools: `compare_three_way`, `compare_source_truth`, `confighub_changesets`, `confighub_units`, `confighub_unit_get`
  - Verified by `cmd/cub-scout/mcp_test.go`; full per-tool reference at `skills/references/mcp-tool-catalog.md`
- `doctor` / `explain` with `--presentation human|ai|paired` and `--hint-mode default|beginner|operator`
- Argo truth-and-guidance track — truthful `explain` ownership for ApplicationSet-managed resources, connected three-way disagreement surfacing, phase-aware next-step hints
- `compare three-way` — connected DRY/WET/LIVE comparison, `--fail-on` conformance exit codes, agreement/convergence summary in CLI + JSON, `--source-path` raw-YAML back-resolution (`#440`)
- Secrets track — trace secret evidence, Crossplane `ProviderConfig` secrets, `map issues` findings, TUI secret panel
- Git import track — `import --git-path` local preview, ApplicationSet git-generator support, path-centric duplicate-safe proposal identifiers
- `cub scout` is the preferred invocation form (v2.0.0 plugin switchover shipped); `cub-scout` standalone binary still works identically

## Current Open Queue

Verify live state before acting. As of 2026-05-22 the open follow-ons are:

### Receipts v2 + small follow-ons

- **`#448`** — Aggregate / chained receipts via `inputAttestations[]` composition. v1 envelope already emits `inputAttestations: []`; v2 wires the cross-receipt digest semantics.
- **`#449`** — `cub-scout watch --emit-receipt-on <event-type>`: event-driven receipt emission so receipts can land in real time. Becomes the real-time channel into Pilot.
- **`#451`** — `--fail-on RECEIPT_VERDICT` exit semantics extension for CI gating.
- **MCP `compare_source_truth` strategy-enum drift** — schema enum lists 4 strategies (Phase 1); CLI supports 9 (Phase 2). One-file fix in `cmd/cub-scout/mcp.go`.
- **Codex round-5 P3** — source-truth receipt precedence tests (`StatusBLOCK + VerdictBLOCKED`, `StatusWATCH + VerdictINCOMPLETE`). Not blocking; nice-to-have.

### Pilot integration

- **`#444`** — Pilot–cub-scout integration skills: 9 consumer-side scenarios (CD gate / fleet conformance / patch+drift / rollback / promotion / incident evidence / compliance / release verification / watch alert). Closes the trust-triad loop on the consumer side.

### Adjacent open issues

- **`#432`** — Grafana collector / data-source path using existing cub-scout outputs.
- **`#427`** — Watch kstatus migration may flip `Ready=true → false` for stalled workloads in v2.1.0+ (behavior-change design).
- **`#422`** — Views project: TUI Hub view integration (`#391` scope #2 follow-up). Scopes #1 (`#414`) and #3 (`#420`) already shipped.
- **`#421`** — Views project: CEL + JSONPath column evaluators.
- **`#409` Phase 3** — source-truth multi-source Argo (`spec.sources[]` len > 1). Phases 1 + 2 shipped (9 strategies).
- **`#392`** — Initiatives compliance overlay; **deferred** until ConfigHub exposes Initiative as a backend primitive. Design doc at `docs/howto/initiatives-integration-when-ready.md`.
- **`#386`** — `preferInvocationForm` lint extension to non-hint legacy string leaks.

### Attribution-layer next-up (no separate issues yet; tracked in README § What's coming next)

- Helm / Kustomize back-resolution to populate `gitSource.file:line` for templated sources
- List-key selectors (e.g., `[name="api"]` for container images) in `compareFieldToPath`
- Standalone `--source-path` as DRY source for `compare three-way`
- `import --git-path --output-dir` polish (disk-PR proposal flow)
- Hierarchy-aware ingest (preserve ApplicationSet / app-of-apps / Flux composition)
- Additional manager-string writers (Tekton, Argo Workflows, Cluster API, OIDC-based CD)

### Cross-repo dependencies

- `confighub/confighub#4356` — ArgoCDOCI Helm-source shape symptom classifier.
- Pilot consumer-side fixtures (tracked in a separate non-public repo).

## Non-Negotiables

1. Do not invent command surfaces.
2. Verify current behavior from local help before claiming support.
3. Prefer `./cub-scout` in local repo workflows.
4. Keep cluster read-only behavior separate from ConfigHub writes.
5. Treat `cub scout` and `cub gitops import` as complementary, not interchangeable.
6. Preserve deterministic facts over optimistic guidance — when attribution can't classify confidently, return `unknown` rather than guessing.

## Quick Reality Checks

Use these before answering capability or workflow questions:

```bash
./cub-scout version
./cub-scout --help
./cub-scout doctor --help
./cub-scout explain --help
./cub-scout compare three-way --help
./cub-scout compare source-truth --help
./cub-scout import --help
./cub-scout views --help
./cub-scout mcp serve --help
```

When the question crosses into ConfigHub or renderer workflows:

```bash
cub version
cub gitops --help
cub gitops import --help
cub link --help
cub link list --help
```

## Best Next Read Based On Intent

- implementing or reviewing code: `HANDOVER.md`
- checking exact flags or stable command surfaces: `docs/reference/commands.md` and `docs/reference/cli-contract.md`
- checking JSON outputs or MCP-adjacent facts: `docs/reference/json-contracts.md`
- attribution layer (cause / managerHint / gitSource / bindingSource): `docs/reference/json-contracts.md` § Field Mutation Attribution Contract, plus `examples/drift/mutation-cause-attribution/`
- capability-assistant or demo flow: `docs/howto/using-cub-scout-from-ai-tool.md`
