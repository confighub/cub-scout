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

## Tool Boundaries

### `cub scout` / `cub-scout`

**`cub scout` observes and explains; it never decides.** It is the
read-only Kubernetes and GitOps observer — it surfaces evidence, never
mutates the cluster, and never makes authority calls about what *should* be
true.

`cub scout` is the preferred documented form. In this repo, local command
examples still use `./cub-scout ...`.

Use it for:
- `doctor`, `explain`, `trace`, `map`, `scan`
- connected comparison such as `compare three-way`
- local Git structure discovery with `import --git-path` and `parse-repo`
- Model Context Protocol (MCP) serving via `mcp serve`

Important:
- `cub scout` is cluster read-only by default
- connected imports write inventory/state to ConfigHub, not cluster manifests
- `import --git-path` is a structure/import-preview path, not a manifest renderer

### `cub`

`cub` is the ConfigHub CLI for intended-state workflows.

Use it for:
- spaces, units, targets, workers, and ConfigHub state
- `cub gitops discover`
- `cub gitops import`

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

As of 2026-05-08, these areas are fully or materially shipped:

- **Source-truth contract** (`#393`, council-prescribed)
  - `compare source-truth <kind>/<name> -n <ns> --strategy <s>` emits the structured JSON Pilot's acceptance kernel consumes
  - 4 strategies: `confighub-oci-argo`, `confighub-oci-flux`, `git-argo`, `git-flux`
  - Strategy-relative correctness + missing-proof rule enforced in tests, not just intent
  - 6-fixture producer suite with byte-equal goldens at `test/fixtures/source-truth/`
- **Architectural triad locked**
  - cub-scout = read-only evidence provider
  - Pilot = acceptance judge
  - ConfigHub = authority and workflow engine
  - cub-scout never mutates, repairs, approves, or infers authority
- **kstatus migration complete** (`#394`)
  - All readiness derivation flows through `sigs.k8s.io/cli-utils/pkg/kstatus`
  - Same library Argo CD and Flux use upstream — operator expectations match
  - Stalled / generation-lagging workloads correctly report `Ready=false`
- **Views integration** (`#391`)
  - `cub-scout views resolve <uuid-or-url>` — accepts both bare UUIDs and View Explorer URLs
  - `cub-scout views open <uuid-or-url>` — browser handoff helper
  - `cub-scout compare three-way --view <uuid-or-url>` — scope a three-way comparison to the cluster resources whose ConfigHub units match a View's filter (#408, shipped 2026-05-09)
  - URL-as-positional convention for ConfigHub primitives — paste from browser address bar
  - On-prem ConfigHub deployments work (host not pinned)
  - `cubRunner` subprocess-injection seam in `views.go` — all future `cub`-shelling code in the views layer should use `viewCubRunner` for testability
- `doctor` / `explain`
  - `--presentation human|ai|paired`
  - `--hint-mode default|beginner|operator`
- Argo truth-and-guidance track
  - truthful `explain` ownership for ApplicationSet-managed resources
  - connected three-way disagreement surfacing
  - phase-aware next-step hints
- `compare three-way`
  - connected DRY/WET/LIVE comparison
  - `--fail-on` conformance exit codes
  - agreement/convergence summary in CLI + JSON
- MCP (Model Context Protocol) gateway
  - `mcp serve` exposes `doctor` as the first standalone troubleshooting tool, including when the problem might be local access uncertainty such as wrong context, stale kubeconfig, or API reachability
  - standalone tool set: `doctor`, `explain`, `map`, `scan`, `trace`
  - connected mode adds read-only ConfigHub query tools
- Secrets track
  - trace secret evidence
  - Crossplane `ProviderConfig` secret evidence
  - `map issues` secret findings
  - TUI secret panel
- Git import track
  - `import --git-path` local preview
  - ApplicationSet git-generator support in parser
  - path-centric duplicate-safe proposal identifiers

## Current Open Queue

Verify live state before acting, but the current open follow-ons are (2026-05-09):

- `#391` — Views integration. Scope #1 (`--view` on `compare three-way`) shipped in #414. Remaining: scope #2 (View column projection in TUI Hub view), scope #3 (reality overlay composing View columns with source-truth verdicts — now unblocked).
- `#409` — source-truth v0.2 cross-surface revision equality. Design pre-baked, plus a [strategy-shape comment](https://github.com/confighub/cub-scout/issues/409#issuecomment-4411862418) with phased Phase 1/2/3 plan. Phase 1 = four existing strategies; Phase 2 = enum expansion; Phase 3 = multi-source Argo. Prerequisite to verify: does `cub unit get -o json` expose the rendered digest per unit revision?
- `#410` — Triad-compliance audit (HIGH). Real violation: `cmd/cub-scout/remedy.go` actually executes `kubectl apply`/`delete` via `executor.Execute`. Discussion ticket — decide remove / rename / hide-behind-flag before code change.
- `#392` — Initiatives compliance overlay; **deferred** until ConfigHub side exposes Initiative as an addressable backend primitive. Design doc at `docs/howto/initiatives-integration-when-ready.md` is ready to consume the day the prerequisite lands.
- `confighubai/confighub#4356` — cross-repo dependency for the ArgoCDOCI Helm-source shape symptom classifier in `compare source-truth`.
- `confighub-ai-demo#264` — Pilot consumer-side fixtures (paired with cub-scout #395 + future #409 fixtures).

## Non-Negotiables

1. Do not invent command surfaces.
2. Verify current behavior from local help before claiming support.
3. Prefer `./cub-scout` in local repo workflows.
4. Keep cluster read-only behavior separate from ConfigHub writes.
5. Treat `cub scout` and `cub gitops import` as complementary, not interchangeable.
6. Preserve deterministic facts over optimistic guidance.

## Quick Reality Checks

Use these before answering capability or workflow questions:

```bash
./cub-scout version
./cub-scout --help
./cub-scout doctor --help
./cub-scout explain --help
./cub-scout compare three-way --help
./cub-scout import --help
./cub-scout parse-repo --help
```

When the question crosses into ConfigHub or renderer workflows:

```bash
cub version
cub gitops --help
cub gitops import --help
```

## Best Next Read Based On Intent

- implementing or reviewing code: `HANDOVER.md`
- checking exact flags or stable command surfaces: `docs/reference/commands.md` and `docs/reference/cli-contract.md`
- checking JSON outputs or MCP-adjacent facts: `docs/reference/json-contracts.md`
- capability-assistant or demo flow: `docs/howto/using-cub-scout-from-ai-tool.md`
