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

cub-scout commands fall into seven groups. Each group's **inputs** column tells you whether a command needs only a cluster (standalone), needs a local git checkout, or needs ConfigHub auth (connected). See [README.md § Capability Map](README.md#capability-map) for the full per-command table — this section is the navigation aid.

| Group | What it answers | Notable commands |
|---|---|---|
| **Observe** | What's running, who owns it | `doctor`, `map`, `trace`, `tree`, `scan`, `graph`, `snapshot`, `watch`, `status` |
| **Diagnose** | What's wrong, what to do next | `explain`, `debug`, `suggest-remedy`, `patterns`, `gitops status` |
| **Compare** | Intended vs actual | `compare`, `compare drift`, `compare three-way`, `compare source-truth` |
| **Attribute** | Where each field's value came from | per-field `cause`/`managerHint`/`gitSource`/`bindingSource` on `compare` + `explain` |
| **Ingest** | How to bring config into ConfigHub | `import --git-path`, `import parse-repo`, `import argocd`, `import cluster-aggregator`, `import apply`, `app` |
| **Govern** | Connected history, fleet, views | `history`, `impact`, `fleet outliers`, `summary`, `views`, `audit`, `bundle`, `catalog` |
| **Integrate** | Setup + AI gateway | `setup`, `quickstart`, `mcp serve`, `context-pack`, `version` |

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

As of 2026-05-21, these areas are fully or materially shipped:

- **Attribution layer** (`#435` — A1, A1.5, A2, B, C1, C2)
  - `cause` + `managerHint` on every field mismatch — classifies controller-drift vs manual-edit via K8s `managedFields` co-signaled with detected owner
  - Verified manager-string enumeration (`pkg/agent/manager_strings.go`) covers Argo CD, Flux (kustomize/helm/source), Helm direct, Crossplane (composite/composed/claim/MRD/refs), kro (applyset/parent/labeller), `kubectl-*` — strings not in the enumeration return `unknown`
  - Per-field-path resolution via `FieldsV1` decoding (`pkg/agent/field_ownership_paths.go`)
  - `gitSource{repoUrl, revision, path}` resource-level anchor via existing Argo/Flux tracers
  - `gitSource{file, line}` raw-YAML back-resolution via `--source-path <local-checkout>` flag (`pkg/agent/source_back_resolution.go`)
  - Connected `incomingBindings[]` from `cub link list` — surfaces upstream units feeding this unit
  - Connected per-field `bindingSource` — answers "this field's value came from upstream unit X at path Y via link Z"
  - Documented in `docs/reference/json-contracts.md` § Field Mutation Attribution Contract
  - Example: `examples/drift/mutation-cause-attribution/`
- **Source-truth contract** (`#393`)
  - `compare source-truth <kind>/<name> -n <ns> --strategy <s>` emits the structured JSON Pilot's acceptance kernel consumes
  - 4 strategies today: `confighub-oci-argo`, `confighub-oci-flux`, `git-argo`, `git-flux`
  - Strategy-relative correctness + missing-proof rule enforced in tests
  - 6-fixture producer suite with byte-equal goldens at `test/fixtures/source-truth/`
- **Architectural triad locked**
  - cub-scout = read-only evidence provider
  - Pilot = acceptance judge
  - ConfigHub = authority and workflow engine
  - cub-scout never mutates, repairs, approves, or infers authority
  - `suggest-remedy` (was `remedy`) is categorically read-only — describes but never applies; the executor was removed in `#428`
- **kstatus migration complete** (`#394`)
  - All readiness derivation flows through `sigs.k8s.io/cli-utils/pkg/kstatus`
  - Same library Argo CD and Flux use upstream
- **Views integration** (`#391`)
  - `views resolve` accepts UUIDs or View Explorer URLs (`#403`)
  - `compare three-way --view <uuid-or-url>` scopes to a ConfigHub View (`#414`)
  - `views project --with-reality` composes View columns with source-truth verdicts (`#420`)
- `doctor` / `explain` with `--presentation human|ai|paired` and `--hint-mode default|beginner|operator`
- Argo truth-and-guidance track — truthful `explain` ownership for ApplicationSet-managed resources, connected three-way disagreement surfacing, phase-aware next-step hints
- `compare three-way` — connected DRY/WET/LIVE comparison, `--fail-on` conformance exit codes, agreement/convergence summary in CLI + JSON, `--source-path` raw-YAML back-resolution (`#440`)
- MCP gateway — `mcp serve` exposes `doctor` / `explain` / `map` / `scan` / `trace` standalone; connected mode adds read-only ConfigHub query tools
- Secrets track — trace secret evidence, Crossplane `ProviderConfig` secrets, `map issues` findings, TUI secret panel
- Git import track — `import --git-path` local preview, ApplicationSet git-generator support, path-centric duplicate-safe proposal identifiers

## Current Open Queue

Verify live state before acting. As of 2026-05-21 the open follow-ons are:

- **`#409`** — source-truth v0.2 cross-surface revision equality. Phase 1 (existing four strategies) shipped; Phase 2 (enum expansion to `helm-flux`, `helm-argo`, `kustomize-flux`, `oci-flux`, `oci-argo`) shipped in `#418`. Phase 3 (multi-source Argo) remains. Verify ConfigHub-side rendered-digest exposure before extending further.
- **`#391`** — Views integration tail. Scope #1 (`--view` on `compare three-way`) shipped in `#414`. Scope #2 (TUI Hub View column projection) shipped in `#419`/`#420`. Scope #3 (reality overlay) is the active follow-on.
- **`#392`** — Initiatives compliance overlay; **deferred** until ConfigHub exposes Initiative as a backend primitive. Design doc at `docs/howto/initiatives-integration-when-ready.md`.
- **Attribution layer next-up** (no separate issues yet; tracked in [README § What's coming next](README.md#whats-coming-next)):
  - Helm / Kustomize back-resolution to populate `gitSource.file:line` for templated sources
  - List-key selectors (e.g., `[name="api"]` for container images) in `compareFieldToPath`
  - Standalone `--source-path` as DRY source for `compare three-way`
  - `import --git-path --output-dir` to emit proposals to disk
  - Hierarchy-aware ingest (preserve ApplicationSet / app-of-apps / Flux composition)
  - Additional manager-string writers (Tekton, Argo Workflows, Cluster API, OIDC-based CD)
- `confighubai/confighub#4356` — cross-repo dependency for ArgoCDOCI Helm-source shape symptom classifier.
- `confighub-ai-demo#264` — Pilot consumer-side fixtures (paired with cub-scout `#395` + future `#409` fixtures).

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
