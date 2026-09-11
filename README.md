# cub-scout

**A read-only Kubernetes and GitOps explorer for people, scripts, and AI agents.**

[v2.10.0 release](https://github.com/confighub/cub-scout/releases/tag/v2.10.0)
| [Start here](docs/getting-started/start-here.md)
| [Command guide](CLI-GUIDE.md)

## Why this exists

"Is this deployed?" usually means several questions: did the controller consume
the right revision, did the objects reach the cluster, and is the application
ready? Those answers live in different resources and tools.

cub-scout brings that evidence together without taking over delivery. Start from
a live cluster, follow ownership and source chains, inspect failures, and compare
against supplied or ConfigHub-managed configuration. Missing evidence stays
visible; a healthy Pod is not proof that the intended release was delivered.

## What you get

- **An ownership map:** explore resources across Argo CD, Flux, Helm, Sveltos,
  Modelplane, Crossplane, kro, ConfigHub, and native Kubernetes. Evidence depth
  depends on the controller and available metadata.
- **An explanation, not just a status color:** controller progress, workload
  symptoms, source revisions, field differences, and the next read-only checks.
- **Evidence you can reuse:** JSON for scripts and MCP tools for agents, plus
  saved bundles and fingerprinted receipts for review. No model inference is
  required to classify ownership or compute the reported evidence.
- **A small read when that is enough:** v2.10.0 bounded explain reads one exact
  object; MCP/TUI sessions can reuse the observation with visible freshness.

No ConfigHub account or in-cluster Scout installation is needed for standalone
inspection. Live checks need Kubernetes API access; saved-file and bundle
workflows can run offline. Observation does not modify cluster state, but read
permissions, sensitive output, and API load still matter.

## Start in Two Minutes

```sh
brew install confighub/tap/cub-scout
cub-scout doctor          # What needs attention?
cub-scout gitops status   # What do delivery controllers report?
cub-scout map             # Explore interactively
```

Already using `cub`? Run `cub plugin install confighub/cub-scout@v2.10.0`, then
`cub scout doctor`. [Installation and verified downloads](docs/getting-started/install.md)
cover macOS, Linux, Windows, and tagged source builds.

## Five ways to run cub-scout

| Run mode | Command | Best for | Notes |
|---|---|---|---|
| Standalone client | `cub-scout doctor`, `cub-scout map`, `cub-scout trace ...` | CLI, interactive TUI, or JSON in a script | Uses the current kube context; no ConfigHub dependency. `kubectl cub-scout` is another invocation of this client, not another service. |
| ConfigHub plugin | `cub scout doctor`, `cub scout compare ...` | ConfigHub users who want the same observer inside their existing `cub` workflow | Same read-only scout behavior, with plugin-aware help text and inherited `cub` context. |
| MCP server | `cub-scout mcp serve` | Agents calling structured read-only tools | The MCP host starts a local stdio process. Nothing must already be running. Agents with shell access can also call the CLI directly. |
| Watch stream | `cub-scout watch --output-file ./events.jsonl` | A long-running local observer feeding incident tooling | Polling-based discovery, ownership-change, drift, and scan events; webhook and receipt output are also available. |
| In-cluster bot | `cub-scout bot --webhook <url>` | One observer Pod feeding an approved sink | Same polling engine, with ServiceAccount auth and `CUB_SCOUT_BOT_*` configuration. It does not deploy or trigger sync. See [bot setup](examples/bot/) and [container availability limits](docs/getting-started/install.md#container-and-bot). |

Prefer browsing intended configuration in a connected TUI? The separately
installed [companion explorer](https://github.com/confighub/cub-commander) uses
Scout v2.10.0 in its Resource Evidence tab. Commander v0.3.0 requires an explicit
Target-ID/kube-context binding. Scout's standalone TUI remains available.

## User questions

Cub-scout helps users answer questions about k8s and GitOps clusters in one place.

| User question | cub-scout surface | What cub-scout provides |
|---|---|---|
| What is running, and who owns it? | `doctor`, `map`, `trace`, `explain` | Live inventory, ownership, source chain, recent events, and next read-only checks across supported controllers and platforms. |
| Can I inspect one exact resource without running a broad diagnostic sweep? | `explain Deployment/api -n prod --bounded --api-version apps/v1 --kube-context <context> --format json`; MCP `explain` with `bounded: true`; TUI `Ctrl+e` | **v2.10.0:** explicit context and API identity; one discovery document and one object GET on a cold read. No controller CLI, ConfigHub, pod/event fan-out, or object LIST. The TUI requires an inventory-bound kube context; unbound/in-cluster inventory, unsupported APIs, RBAC failures, and identity mismatches remain unavailable evidence. |
| Can repeated explorer or agent reads reuse the same observation, and can I force a fresh check? | Bounded MCP `explain`; TUI `Ctrl+e` and `r`; CLI `explain --bounded --refresh` | **v2.10.0:** a maximum of 16 object observations reused for less than 15 seconds within one MCP/TUI session. `resourceRead` exposes availability, scope, timestamps, cache state, and discovery/object read counts. Refresh, expiry, and context/credential-configuration changes invalidate reuse. Leaving the TUI read cancels pending work and rejects late results. Separate CLI processes do not share this cache. |
| What did this narrow check deliberately leave unverified? | Bounded `explain` JSON, ASCII/Markdown, MCP, and TUI | **v2.10.0:** structured `omissions` distinguish source/controller evidence, related pods/events, and desired/live comparison from object-local ownership and readiness. Readiness is not delivery completion or application success. No Secret payload reads. |
| Which source space, unit, and revision does this live resource identify? | `explain Deployment/api -n prod --bounded --api-version apps/v1 --kube-context <context> --format json`; bounded MCP `explain`; TUI `Ctrl+e` | **v2.10.0:** `configHubOrigin` parses the already-read `confighub.com/origin` annotation, preserving space/unit IDs, slugs, and an optional exact integer revision. No additional requests or ConfigHub authentication. This is observed source metadata, not independently verified provenance, a release/target/component/variant join, or proof of delivery. [Fixture and proof](examples/bounded-resource-read/#observed-origin). |
| Could incomplete or conflicting source metadata silently link me to the wrong configuration? | Bounded `explain` JSON, ASCII/Markdown, MCP, and TUI | **v2.10.0:** missing origin stays an information omission; malformed, duplicate, unsupported, or legacy-conflicting identity becomes a warning omission. No guessed IDs, legacy fallback, browser links, or target-to-cluster mapping; controller ownership and health stay unchanged. |
| Can I inspect a live object while browsing its intended configuration in a companion explorer? | [Companion panel](https://github.com/confighub/cub-commander/pull/1): `cub commander --scout-binding '<target-id>=<kube-context>'`, Resource detail `3 Evidence` | **Commander v0.3.0 + Scout v2.10.0:** calls the bounded CLI JSON provider with exact Resource API/kind/namespace/name and an explicit Target-ID/context binding. Missing identity/binding prevents execution; mismatched or incompatible responses stay unavailable. The panel displays scope, a shell-safe invocation, origin evidence and omissions. Standalone Scout and its TUI remain available; this is not a desired/live diff or delivery-success verdict. |
| Can I revisit that companion panel without re-reading the cluster, and see when its evidence is old? | Companion Resource `3 Evidence`, `1` / `2` to switch tabs, `r` to refresh | **Commander v0.3.0 + Scout v2.10.0:** one captured snapshot with observation/expiry times and a `STALE` label. Tab revisits make no provider calls, even after expiry; only explicit refresh reads again and discards old success first. Navigation cancels pending work; changed identities and late responses cannot reuse old evidence. Unlike Scout's MCP/TUI cache, viewing this retained snapshot does not revalidate credentials. [Contract and proof](https://github.com/confighub/cub-commander/blob/main/docs/resource-evidence.md). |
| Is delegated delivery healthy? | `doctor --with-confighub`, `gitops status`, MCP `gitops_status`, `map deployers`, `map activity --with-confighub`, `trace` | Controller-reported backend, transport, source/build/apply/sync stages, first-class aggregate delivery resources, scope-level delivery rollups, ConfigHub delivery timeline rows, exact live-status joins on matching Argo Application activity rows, reason/message, and explicit evidence gaps when status is incomplete. |
| Which controller families did cub-scout actually inspect? | `gitops status`, MCP `gitops_status` | `controllerCoverage[]` records Flux, Argo CD, ConfigHub, Sveltos, and Modelplane coverage as `found`, `not_found`, `partial`, or `unreadable`, including checked kinds, observed kinds, counts, and RBAC/list omissions. |
| Is this Modelplane resource backed by Crossplane composition? | `map list --format json`, `trace`, `watch`, `bot`, `receipt verify`, attribution JSON | Modelplane remains the higher-level owner when its signals are stronger, while `ownerEvidence`, `owner.evidence`, trace messages, and `predicate.evidence.platformSubstrate` surface Crossplane composite, claim, composition-resource, and verified field-manager evidence as substrate context. |
| What Kubernetes config does ConfigHub already hold for this space or target? | MCP `confighub_k8s_types`, MCP `confighub_k8s_resources` | Read-only, Resource-backed intended-config reads through `cub k8s types/get`: type surveys, stored resource rows or YAML, custom-resource discovery, explicit space/target scoping, and `where` / `where_resource` filters before touching live cluster APIs. |
| Which indexed resources match a fleet-wide predicate? | MCP `confighub_resources` | Read-only ConfigHub Resource entity queries through `cub resource list`: server-side `ResourceType`, `ResourceName`, `TargetID`, `Unit.*`, `Space.*`, and `Data.*` predicates, optional views/selects/raw data, and explicit space scoping for broad explorer questions. |
| Could a same-name application inherit another application's delivery status? | `map activity --with-confighub --format json` | Joins require an observed Application Space ID matching the reported Space ID plus a unique, exact Application name in the selected non-wildcard space. Missing/conflicting metadata and ambiguous names/statuses remain correlation omissions; the Argo-owned result is preserved. |
| What release or event triggered this delivery attempt? | `map activity --with-confighub`, `doctor --with-confighub`, `gitops status --with-confighub`, `trace --with-confighub`, `explain --with-confighub`, `history`, MCP `confighub_releases`, MCP `confighub_unit_events` | Bounded ConfigHub release and unit-event evidence for a known space/time window, including release ids, digests, targets, unit events, timestamps, exact resource-correlation keys when available, activity timeline rows, and structured omissions when history cannot be joined safely. |
| Did evented delivery feedback report back, and is the report fresh? | `map activity --with-confighub`, `doctor --with-confighub`, `gitops status --with-confighub`, `trace --with-confighub`, `explain --with-confighub`, MCP `confighub_live_status` | Read-only parsing of ConfigHub live-status writeback, with `observedAt`, freshness, sync status, operation phase, observed revision, delivery verdict, separate application-health verdict, scope-level rollups, timeline rows, exact joins onto matching Argo Application activity rows, and object-level matches only when exact space plus app/unit identity is present. **v2.10 limit:** unknown or future timestamps can still accompany `PASS`; inspect timestamps, not the verdict alone. [Known freshness gap](docs/reference/explorer-comparison.md#current-feedback-verdict-limit). |
| Is the in-cluster event consumer present and healthy? | `map activity --with-confighub`, `doctor --with-confighub`, `gitops status --with-confighub`, `trace --with-confighub`, `explain --with-confighub`, `map deployers` | Conservative, label-selected detection of the known event-consumer Deployment shape across namespaces when allowed, with ready/desired replicas, activity timeline rows, and `doctor` top issues when an observed consumer is unhealthy; absence or RBAC narrowing becomes an omission, not a claim that no eventing exists. |
| Can I trust a reported success or failure when its timestamp is missing, future, or old? | Connected `gitops status`, `doctor`, `map activity`, `trace`, `explain`, MCP `confighub_live_status`, single-resource receipts | **Post-v2.10.0 fix (unreleased):** missing/invalid/zero/future timestamps yield `INCONCLUSIVE`; stale reports yield `WATCH`, including old failures. The original report, timestamp and a freshness omission stay visible. Re-reading an unchanged report does not renew it; the next step is a current controller/workload read. [Cases and proof](examples/live-delivery-observability/#trusting-feedback-freshness). |
| Has intended configuration reached the cluster? | `compare three-way` | DRY/WET/LIVE agreement for a resource, namespace, cluster, or governed view, with states like `agreed`, `converging`, `diverged`, and `partial`. |
| Did the controller consume the expected source revision or OCI digest? | `trace`, `trace --with-confighub`, `gitops status`, `compare source-truth`, `gitops status --with-confighub` | Controller-owned source status, revision/digest evidence, object-correlated ConfigHub release digest context when exact space+target joins exist, and proof gaps when controller/source identifiers cannot be joined safely. |
| Did this rendered install set land as a set? | `compare object-set`, `receipt verify --file` | Set-level desired-vs-live evidence from rendered YAML: authored-field deltas, optional added/removed object closure, object-set receipts, normalization profiles, freshness TTL, and CI-gate verdicts. |
| Is this rollout still progressing, complete, or stuck? | `receipt verify --predicate workloads-converged`, `doctor`, `explain`, `compare three-way` | Generation-aware workload evidence: `metadata.generation`, `status.observedGeneration`, kstatus, progress clock, pod failure signals, and `PASS` / `WATCH` / `BLOCK` / `INCONCLUSIVE` verdicts. |
| Can I move to the next task, wait, or retry delivery? | `receipt verify --with-confighub`, `compare three-way`, `doctor --with-confighub`, `map activity --with-confighub` | A read-only decision frame that separates "not applied yet", "still converging", "runtime failure", stale/failed delivery feedback, recent delivery events, and missing evidence, with optional fingerprinted receipts for the exact observation. |
| Is live state drifting from desired state? | `compare drift`, `compare three-way`, `compare source-truth` | Field-level differences, strategy-relative source-truth evidence, conformance exit codes, and explicit proof gaps when evidence is missing. |
| Is this a delivery problem or an application/runtime problem? | `doctor --with-confighub`, `explain --with-confighub`, `gitops status --with-confighub`, `map activity --with-confighub`, `trace --with-confighub`, `patterns`, `scan` | kstatus health, Kubernetes events, live-status delivery vs application-health verdicts, audited action metadata when present, workload symptoms, delivery timeline rows, known-pattern matches, and phase-aware hints so operators can separate sync/convergence issues from runtime failures. |
| What triggered this reconcile, check, or action? | `map activity --with-confighub`, `trace`, `explain` | Audited Kubernetes action event metadata when present, plus optional ConfigHub release/unit-event rows: action, actor, subject, groups, timestamps, release digest/target, and preserved raw annotation evidence. |
| Who changed this field, and where did the value come from? | `compare`, `explain`, attribution JSON | `cause`, `managerHint`, `gitSource`, `bindingSource`, audit/event evidence where available, and optional file:line back-resolution from live `managedFields`, controller metadata, local source files, and governed links. |
| Can I answer broad or repeated questions without hammering live APIs? | `snapshot`, `watch`, `bot`, `summary`, `receipt verify --with-confighub`, `receipt list --format json`, `map list --format json`, `map activity --with-confighub --confighub-space <space>`, `doctor --with-confighub --confighub-space <space>`, `gitops status --with-confighub --confighub-space <space> --confighub-since <window>`, MCP `confighub_resources`, MCP `confighub_k8s_types`, MCP `confighub_k8s_resources` | Existing captured state, low-cardinality watch/bot events, connected summaries, bounded current-space/time-window ConfigHub reads, Resource-backed intended-config and Resource-index surveys, `observation.source/mode/observedAt/freshness` metadata on live inventory/event snapshots and stored summary records, and immutable receipts with list-time freshness status (`fresh`, `stale`, `not-declared`, `invalid`) so repeated reviewers can inspect frozen evidence instead of re-querying live APIs. |
| Can I centralize ongoing observation instead of starting a poller for every user? | `bot`, `watch`, `snapshot` | One read-only event producer can feed a shared sink using in-cluster auth, webhook/JSONL output, bounded queues, receipt-build caps, and filters. Consumers must read that sink to avoid their own cluster queries; this is not a shared Scout query service or a fleet-wide API budget. |
| Does Scout's bot replace a release-event or sync bot? | `bot`; connected delivery evidence | No. The observer produces evidence; the delivery bot triggers its controller and owns status writeback. Scout can consume that feedback with identity and freshness checks, then add independent live observations. [Integration boundaries](docs/reference/explorer-comparison.md#scout-bot-and-delivery-bot). |
| Can I keep auditable evidence of the check? | `receipt verify`, `receipt verify --with-confighub`, `receipt list`, `receipt validate`, `watch --emit-receipt-on`, `bot --emit-receipt-on` | Typed, fingerprinted, immutable evidence receipts for gates, incident closeout, audits, chained checks, live delivery-status snapshots, and real-time watch/bot events; `receipt list` shows whether saved TTL-backed receipts are still fresh, stale, undeclared, or malformed. |
| Can I onboard an existing Argo app or app-of-apps safely? | `import argocd`, `import parse-repo`, `import --git-path`, `compare three-way`, `trace` | Source-backed discovery and import proposals, app-of-apps topology warnings, and pre-handover comparison evidence; actual ConfigHub loading, release publishing, and controller handover stay with the governing toolchain. |

The main path starts from a **live cluster**. It works **standalone** with your current kube context, or **connected** to [ConfigHub](https://confighub.com) for governed comparison, history, import, fleet queries, and AI-friendly read-only workflows. Local repo and manifest inputs are available later for adoption, import-preview, and source-file enrichment, but they are not the first mental model.

Resource MCP reads and Application-row joins are v2.9.0 additions. See
[release notes](docs/releases/v2.9.0.md) for scope and validation. Scoped Resource
queries use ConfigHub rather than Kubernetes, but
each repeated query still runs a fresh `cub` command. Watch/bot remain polling
observers; output caps and freshness stamps are not API rate limits. Reuse saved
evidence when appropriate. [Request-cost proof](examples/mcp-gateway/README.md#request-cost-and-context-proof).

**v2.10.0 adds bounded explorer evidence.** See the
[release notes and upgrade order](docs/releases/v2.10.0.md) for Scout/companion
compatibility, request budgets, and explicit validation limits. The companion
is separately packaged; install Scout v2.10.0 before Commander v0.3.0.

### Who reaches for cub-scout?

- **SREs and on-call** — when a workload is unhealthy and you need to know *why* and *who owns it* in seconds, not by clicking across Argo or Flux UIs.
- **Platform engineers** — when you inherit a cluster and need a trustworthy ownership map across Flux, ArgoCD, Sveltos, Modelplane, Helm, Crossplane, kro, ConfigHub, and naked YAML.
- **AI agents and automation** — when you need stable, deterministic JSON or a Model Context Protocol (MCP) gateway for read-only cluster facts.

### `cub-scout` vs `cub`: the boundary

| | `cub-scout` | `cub` |
|---|---|---|
| Role | **Observe and explain** | **Act and govern** |
| Cluster state | Read-only observation | Publishes desired releases for deployment controllers to reconcile |
| ConfigHub state | Read-only evidence; explicit import commands can write inventory | Authoring, import, promotion |
| Best for | Diagnosis, ownership, drift, attribution, AI tools | Intended-state authoring, GitOps pipelines |

cub-scout is the **read-only witness** for cluster state. ConfigHub (driven by `cub`) is the **authority**. The binaries ship separately; explicit inventory-import commands are distinguished from read-only observation.

### Where Scout fits

Use Scout when you need cross-controller evidence that works in a terminal,
automation, and an agent workflow. Use your delivery controller or approved
deployment tooling to change desired state and reconcile it. Dedicated cluster
and GitOps explorers remain useful for live graphs, logs, and interactive
operations; Scout does not claim to replace every one of their views.

An event-driven delivery bot has a different job: trigger delivery and report
controller status back. Scout reads that feedback and checks it alongside live
evidence. Its `bot` mode is an observation producer, not a replacement syncer.
See the [version-pinned capability comparison](docs/reference/explorer-comparison.md)
for verified strengths, competing capabilities, and the gaps still on the roadmap.

---

## Capability Map

cub-scout's commands fall into eight groups. Read them as a live-cluster journey first: **observe** what is running, **diagnose** one thing, **compare** live state to governed intent when connected, then use adoption/import paths only when you are ready to bring existing config into ConfigHub.

Each command's **Inputs** column tells you exactly what it needs — cluster only (standalone), cluster + ConfigHub auth (connected), bundle/file inputs, or a local git checkout for advanced adoption and source-enrichment flows.

### Observe — see what's running

| Command | What you get | Inputs |
|---|---|---|
| `doctor` | One-screen cluster health summary with concrete next steps and optional bounded ConfigHub delivery evidence | cluster; optional ConfigHub auth for `--with-confighub` |
| `map` (TUI / `list` / `hooks` / `orphans` / `meaning`) | Ownership inventory, lifecycle hooks, orphan detection, meaning-first grouping | cluster |
| `trace` | Full ownership chain from K8s object → controller/source evidence, with optional object-correlated ConfigHub delivery evidence | cluster; optional ConfigHub auth for `--with-confighub` |
| `tree` | Runtime, ownership, git, and composition hierarchies | cluster |
| `scan` | Audit live cluster or manifest files against 46 built-in risk patterns | cluster *or* file |
| `graph export` | Resource graph as DOT/JSON | cluster |
| `snapshot` | Dump cluster state as GSF JSON | cluster |
| `watch` | Stream observation events to webhook/file sinks | cluster |
| `bot` | Run the watch engine continuously as an in-cluster observer | cluster |
| `status` | Connection mode and cluster context info | cluster |

### Diagnose — interpret what you observe

| Command | What you get | Inputs |
|---|---|---|
| `explain` | Plain-English ownership and lineage for one resource, with phase-aware next-step hints and optional object-correlated ConfigHub delivery evidence | cluster; optional ConfigHub auth for `--with-confighub` |
| `debug` | Guided GitOps debugging wizard | cluster |
| `suggest-remedy` | Read-only description of a remediation that *would* resolve a finding — never applies it | cluster |
| `patterns` (`detect` / `explain` / `list`) | Pattern-engine catalogue + matched findings | cluster *or* file |
| `gitops status` | GitOps/controller pipeline health across Flux, Argo, Sveltos, and Modelplane controllers | cluster |

### Compare — intended vs actual

| Command | What you get | Inputs |
|---|---|---|
| `compare` (resource mode) | Single-resource DRY/WET/LIVE picture when connected; LIVE-only when standalone | cluster (+ConfigHub for DRY/WET) |
| `compare drift` | Desired (file) vs live drift detection | cluster + file |
| `compare three-way` | Scope-wide DRY/WET/LIVE with agreement summary and conformance verdict; `--dry-from` supports standalone rendered YAML as DRY | cluster + ConfigHub or rendered YAML |
| `compare object-set` | Set-level desired-vs-live ObjectSetDiffReceipt from rendered YAML, with optional closed-world added/removed object diff | cluster + rendered YAML |
| `compare source-truth` | Strategy-relative `PASS` / `WATCH` / `ASK` / `BLOCK` evidence for downstream acceptance tools (#393) | cluster + ConfigHub |

`compare source-truth` still requires connected source-truth evidence. `compare three-way --dry-from` and `compare object-set --dry-from` support standalone rendered YAML as desired-state input.

### Verify — typed, fingerprinted, immutable evidence artifacts

cub-scout receipts (#446) are the **persistence** sibling of `compare`: where `compare` produces an ephemeral live picture, `receipt verify` wraps the same evidence into an in-toto Statement v1 envelope that CI/CD gates, audit trails, postmortems, and acceptance-judge tooling can attach to a decision and later re-check for tampering.

| Command | What you get | Inputs |
|---|---|---|
| `receipt verify <kind>/<name>` | Build a typed, fingerprinted receipt asserting a resource predicate. Predicates include `applied-matches-spec`, `source-truth-pass`, `no-manual-edits-since`, `object-set-matches`, `workloads-converged`, and `prerequisites-met`. Add `--with-confighub` to attach bounded, object-correlated delivery evidence as supporting evidence; it does not change which system owns delivery or application-health truth. Verdicts: PASS / WATCH / BLOCK / INCONCLUSIVE. | cluster (+ ConfigHub for `source-truth-pass` and optional `--with-confighub`) |
| `receipt verify --file <path> --scope namespace/<ns>` | Build an install/object-set receipt (`object-set-matches`) proving every desired object in rendered YAML is present live and every authored field still matches. Add `--predicate workloads-converged` to prove rendered workloads reached a ready runtime state, with `--grace-window` measured from current-generation progress signals. | cluster + rendered YAML |
| `receipt verify --fail-on <verdict>` | Same, plus CI-gate exit semantics: exit 2 when the receipt's verdict matches the listed set (`WATCH` / `BLOCK` / `INCONCLUSIVE` / `any-non-pass`). Artifact is preserved on fail. | cluster |
| `receipt verify --input-attestation <path>` | Chain receipts: reference a prior receipt via `inputAttestations[]`. Each referenced receipt's fingerprint is verified before chaining; tampered receipts are refused. | cluster + prior receipt file |
| `receipt show <path>` | Render a saved receipt (ASCII or JSON). Does NOT verify the fingerprint — works on tampered receipts for forensic inspection. | receipt file |
| `receipt validate <path>` | Recompute and compare the receipt's fingerprint. Exit 0 OK / 1 mismatch / 2 I/O. | receipt file |
| `receipt list` | Walk the local store (`$CUB_SCOUT_RECEIPTS_DIR → $XDG_DATA_HOME/cub-scout/receipts → $HOME/.local/share/cub-scout/receipts`) newest first, with freshness status for saved TTL-backed receipts. | local store |
| `watch --emit-receipt-on <event-types>` | Real-time receipt emission: each matching watch event carries a receipt inline. All four known event types build receipts (`drift.detected`, `ownership.changed`, `resource.discovered`, `scan.finding`); per-poll backpressure controlled by `--emit-receipt-batch-cap` (default 10). | cluster |

Wire format: in-toto Statement v1 (`_type = "https://in-toto.io/Statement/v1"`) wrapping `https://cub-scout.dev/receipt/v1`. SHA-256 fingerprint over RFC 8785 canonical JSON of the full Statement minus only `predicate.fingerprint`. Read-only by construction — receipts emit artifacts, never mutate.

### Attribute — where each value came from

cub-scout's **attribution layer** (#435) annotates every field mismatch with provenance evidence:

| Signal | Surfaces on | Inputs |
|---|---|---|
| `cause` + `managerHint` (controller-drift / manual-edit / unknown) — from K8s `managedFields` | `compare` + `explain` | cluster |
| `gitSource.{repoUrl, revision, path}` — from Argo Application / Flux GitRepository spec | `compare` + `explain` | cluster (needs `argocd` / `flux` CLI for tracer) |
| `gitSource.file` + `gitSource.line` — raw-YAML back-resolution | `compare` | cluster + `--source-path <local-checkout>` |
| `incomingBindings[]` — ConfigHub Links influencing this unit | `compare` | cluster + ConfigHub |
| `bindingSource` — per-field upstream unit + binding path | `compare` | cluster + ConfigHub |

The verified manager-string enumeration covers Argo CD, Flux (kustomize / helm / source controllers), Helm direct, Crossplane (composite / composed / claim / MRD / refs), kro (applyset / parent / labeller), Sveltos (`application/apply-patch`), Modelplane via Crossplane composition managers, and `kubectl-*` interactive paths — strings not in the enumeration fall through to `unknown` rather than being guessed.

### Govern — connected fleet and history

| Command | What you get | Inputs |
|---|---|---|
| `history` | Connected ChangeSet timeline for one resource | ConfigHub |
| `impact` | Connected blast-radius preview for one unit | ConfigHub |
| `fleet outliers` | Cluster divergence report across many clusters | ConfigHub |
| `summary` (`list` / `slack`) | Connected summary storage + Slack delivery | ConfigHub |
| `views` (`resolve` / `open` / `project`) | Resolve, open, and project ConfigHub Views (#391) | ConfigHub |
| `audit list` | Break-glass accept/reject audit trail | ConfigHub |
| `bundle` / `catalog` | Inspect, replay, diff, and summarize debug bundles + manage catalogs | bundle artifact |

### Adopt Existing Config — preview imports after observing live state

Use this when you are bringing an existing cluster, Argo/Flux setup, debug bundle, or manifest repo toward ConfigHub. This is an adoption path, not the first-run troubleshooting path.

| Command | What you get | Inputs |
|---|---|---|
| `import --dry-run` | Live-cluster ConfigHub adoption proposal | cluster |
| `import --from-bundle` | Offline adoption proposal from captured bundle facts | bundle artifact |
| `import --git-path` | Advanced local Git-structure preview, no upload or render | git checkout |
| `import parse-repo` | Lower-level repo structure JSON for downstream tools | git checkout or remote URL |
| `import argocd` | Proposal/import for one Argo CD Application's units | cluster + ConfigHub |
| `import cluster-aggregator` | Aggregate import proposals across many clusters/controllers | cluster/bundle proposal files |
| `import apply` | Apply an import proposal JSON to ConfigHub | ConfigHub |
| `app` | Manage ConfigHub Apps | ConfigHub |

Today standalone `import --git-path` is preview-only. A `--output-dir` mode that emits proposed unit YAMLs to disk for review/PR/upload is in the [next-up](#whats-coming-next) list.

### Integrate — AI, setup, and infrastructure

| Command | What you get | Inputs |
|---|---|---|
| `setup connect` | Import or create a kubeconfig context | none |
| `setup completion` | Shell completion script | none |
| `quickstart demo` | Fixture-backed first-run tour | none |
| `mcp serve` | Read-only Model Context Protocol (MCP) server over stdio for Claude, Codex, agents | cluster (any subset of the above based on the tool invoked) |
| `bot` | In-cluster read-only observation bot using the watch event stream | cluster |
| `context-pack` | Deterministic AI context JSON export | cluster |
| `version` | Build/version info | none |

Read commands expose structured output through `--format json` or a documented
`--json` flag; consult each command's help. Ownership and evidence logic are
deterministic for the same inputs. New live observations have new timestamps.

---

## Fast Path (2 Minutes)

**No ConfigHub signup required.** Live inspection reads your Kubernetes API;
offline analysis uses saved files or bundles.

```bash
brew install confighub/tap/cub-scout

cub-scout doctor                                  # cluster health summary
cub-scout explain deploy/api -n prod              # who owns this resource?
cub-scout trace   deploy/api -n prod              # where did it come from?
cub-scout map                                     # interactive TUI
```

If `cub-scout: command not found` but you have `cub` installed, try `cub scout doctor` instead — see [How to invoke](#how-to-invoke).

---

## Install

```bash
# cub plugin (preferred for ConfigHub users — one install, shared auth)
cub plugin install confighub/cub-scout
cub scout version

# Homebrew
brew install confighub/tap/cub-scout
```

For direct downloads and tagged source builds, use the
[install guide](docs/getting-started/install.md). Do not use
`go install github.com/confighub/cub-scout/cmd/cub-scout@latest` for v2.10.0:
the current Go module path resolves an older major. Container command
`docker run ghcr.io/confighub/cub-scout:v2.10.0 version` still needs registry
access verification (#520); the published image is Linux amd64 only.
`kubectl krew install cub-scout` is not a verified distribution path; use the
`kubectl-cub_scout` binary included in the archives or Homebrew instead.

`make build-kubectl-plugin` also produces a `kubectl cub-scout ...` wrapper. See [docs/howto/plugin-install.md](docs/howto/plugin-install.md) for pinned-version, direct-URL, and offline installs.

---

## How to invoke

The same Scout commands, JSON contracts, exit codes, and MCP tool names are
available through three invocation forms:

```bash
cub-scout doctor          # 1. Standalone — installed directly
cub scout    doctor       # 2. cub plugin — after `cub plugin install confighub/cub-scout`
kubectl cub-scout doctor  # 3. kubectl plugin — from `make build-kubectl-plugin`
```

Plugin form (`cub scout ...`) inherits `cub`'s auth automatically — useful when you also use ConfigHub. Standalone form is the path of least resistance and the default for AI/MCP scenarios. Parity is enforced by a release-gate test (`TestPluginParity_StandaloneMatchesPlugin`).

Host-global flags can differ: `cub --context` selects a ConfigHub context, while
Scout's bounded `--kube-context` selects the Kubernetes context to inspect.

---

## Standalone vs Connected

Standalone does not mean disconnected from Kubernetes: live checks need API
access. Saved-file and bundle analysis can run offline. ConfigHub connected
mode is optional and adds intended-state, history, and fleet context.

**Standalone (no signup):** ownership detection, tracing, health diagnosis, drift hints, risk scanning, JSON, MCP gateway, attribution via `managedFields` + Flux/Argo/Helm/Sveltos/Modelplane/Crossplane trace resolvers + (with `--source-path`) raw-YAML file:line resolution.

**Connected (`cub auth login`):** adds the historical and cross-cluster context the Kubernetes API doesn't have:

- **DRY vs WET vs LIVE** — compare what ConfigHub *intended*, what the renderer *produced*, and what's actually *running*. Catches drift the live cluster can't tell you about.
- **Per-field binding source** — answer "this field's value came from upstream unit X at path Y via link Z."
- **Change history** — `kubectl events` only goes so far; `history` walks the ConfigHub governed timeline.
- **Delivery evidence** — `gitops status --with-confighub`, `doctor --with-confighub`, `trace --with-confighub`, `explain --with-confighub`, `map activity --with-confighub`, and single-resource `receipt verify --with-confighub` add bounded release history, unit events, live-status writeback, event-consumer health, rollups, object-correlated snapshots, timeline rows, fingerprinted evidence, and omissions for a known space/time window.
- **Fleet queries** — "is this version running everywhere it should?" across many clusters.
- **Live-cluster adoption preview** — propose how current workloads should be modeled in ConfigHub before writing anything.

**Important:** connected import writes ConfigHub records, not cluster manifests. cub-scout never modifies the cluster.

Local repository parsing (`import --git-path`, `import parse-repo`) is a second-order adoption path: use it when a manifest repo is the thing you are reviewing or when you need source structure to enrich the live-cluster story.

---

## Signature Example

`trace` is the headline feature — it answers "what created this?" without making you stitch Deployments, Applications, Helm releases, Sveltos profiles, Modelplane model resources, labels, annotations, and controller state by hand:

```text
$ cub-scout trace deploy/frontend -n boutique

TRACE: Deployment/frontend in boutique
Owner: Flux
Source: GitRepository/flux-system/platform-config
Path: clusters/prod/apps/boutique
Status: Ready

Recent events:
  Warning BackOff (3x, 5m) kubelet: Back-off restarting failed container

Next:
  cub-scout explain deploy/frontend -n boutique
```

For more, see [docs/reference/commands.md#trace](docs/reference/commands.md#trace).

---

## Interfaces

| Interface | Best for | Start here |
|---|---|---|
| TUI | Interactive exploration, keyboard-driven debugging | `cub-scout map` |
| CLI | One-off triage, shell use, pipelines | `doctor`, `explain`, `trace` |
| JSON | Automation, AI, MCP, downstream tooling | `--format json` or `--json` |

Press `?` inside the TUI for shortcuts.

![cub-scout map dashboard](docs/images/map-dashboard.png)

### Scan Variants

`scan` is the fastest way to audit a cluster or manifest set for known risk patterns. The built-in scanner covers **46 patterns**, and the broader [confighub-scan](https://github.com/confighubai/confighub-scan) catalog tracks **3,513 risk patterns** for deeper policy and detection work.

Common entrypoints:

- `cub-scout scan --state`
- `cub-scout scan --kyverno`
- `cub-scout scan --lifecycle-hazards`
- `cub-scout scan --timing-bombs`
- `cub-scout scan --dangling`
- `cub-scout scan --file manifest.yaml`
- `cub-scout scan --json`
- `cub-scout scan --normalized-json`

Detailed scanner behavior and output shape: [docs/reference/commands.md#scan](docs/reference/commands.md#scan) and [docs/reference/json-contracts.md](docs/reference/json-contracts.md).

---

## AI integration

For Claude, Codex, and other AI agents:
- start with [AI-README-FIRST.md](AI-README-FIRST.md)
- then load [skills/cub-scout/SKILL.md](skills/cub-scout/SKILL.md) if repo-local skills are supported
- for AI tool setup, see [docs/howto/using-cub-scout-from-ai-tool.md](docs/howto/using-cub-scout-from-ai-tool.md)

`cub-scout mcp serve` is a read-only MCP server over stdio backed by the same CLI JSON surfaces — same answers, agent-shaped. It's not a Kubernetes write tool, not a multi-gateway router, and not the ConfigHub authority layer (that role belongs to `cub`).

---

## What's coming next

Honest gaps in the current capability map, with the leverage on filling them:

- **Helm / Kustomize provenance back-resolution** — [#481](https://github.com/confighub/cub-scout/issues/481) extends raw-YAML field attribution from stage B (#440) to templated sources while preserving honesty markers when exact file:line evidence is unavailable.
- **Live delivery observability follow-ups** — aggregate controller-resource failures with generated-artifact lineage as top-level `doctor` findings, audited action events as history/receipt evidence, deeper object-level correlation for release/event/live-status evidence ([#502](https://github.com/confighub/cub-scout/issues/502)), and parity omissions for controllers without status/source/event/generation evidence.
- **`import --git-path --output-dir`** — emit proposed unit YAMLs to disk for PR review, then upload via Installer's `--merge-external-source` once connected. One bundle, two workflows.
- **Hierarchy-aware adoption/import** — preserve ApplicationSet / app-of-apps / Flux Kustomization composition in import proposals so imported ConfigHub state is navigable, not flat.
- **Additional manager-string writers** — Tekton, Argo Workflows, Cluster API, OIDC-based CD systems — gated on whether the variant-management story demands them.

---

## Documentation Map

| Need | Doc |
|---|---|
| Workflow-first CLI tour | [CLI-GUIDE.md](CLI-GUIDE.md) |
| Full command catalog (A–Z) | [docs/reference/cli-reference.md](docs/reference/cli-reference.md) |
| Command usage and examples | [docs/reference/commands.md](docs/reference/commands.md) |
| Stable flags and schemas | [docs/reference/cli-contract.md](docs/reference/cli-contract.md) |
| JSON fields and output model | [docs/reference/json-contracts.md](docs/reference/json-contracts.md) |
| Getting started checklist | [docs/getting-started/checklist.md](docs/getting-started/checklist.md) |
| Import and migration path | [docs/howto/import-to-confighub.md](docs/howto/import-to-confighub.md) |
| Delivery readiness decision and live-delivery fixture | [docs/howto/delivery-readiness-decision.md](docs/howto/delivery-readiness-decision.md) + [examples/live-delivery-observability/](examples/live-delivery-observability/) |
| AI tool integration | [docs/howto/using-cub-scout-from-ai-tool.md](docs/howto/using-cub-scout-from-ai-tool.md) |
| Examples and demos | [examples/README.md](examples/README.md) |
| Receipts (typed evidence artifacts) | [examples/receipts/README.md](examples/receipts/README.md) + [docs/reference/json-contracts.md § Receipt Contract](docs/reference/json-contracts.md) |
| Receipt and proof terminology (vs log / journal / record / ledger / provenance) | [docs/concepts/receipts-and-proofs.md](docs/concepts/receipts-and-proofs.md) |
| Receipts end-to-end how-to (pre-deploy gate → audit chain → namespace aggregate → real-time emission) | [docs/howto/receipts-end-to-end.md](docs/howto/receipts-end-to-end.md) |
| Watch event types + inline receipts + backpressure | [docs/reference/watch-events.md](docs/reference/watch-events.md) |
| Security model | [SECURITY.md](SECURITY.md) |

---

## Build From Source

```bash
git clone https://github.com/confighub/cub-scout.git
cd cub-scout
go build ./cmd/cub-scout
./cub-scout version
```

---

## Principles

- **Read-only, by design** — observation uses only `Get`, `List`, `Watch`. No `apply`, no `delete`, no admission webhook. Even `suggest-remedy` only *describes* a fix — it does not run it.
- **Deterministic** — same inputs, same outputs. No AI/ML inference in the ownership logic.
- **Parse, don't guess** — ownership and attribution come from real labels, annotations, owner references, controller facts, and verified manager-string enumerations. Unknown is preferred over wrong.
- **Complement GitOps, don't replace it** — cub-scout helps you understand Flux, ArgoCD, Sveltos, Modelplane, Helm, Crossplane, kro, and ConfigHub state. It's not another reconciler.
- **Graceful degradation** — works without ConfigHub, without internet, and many flows work without a live cluster (debug bundles, manifest scans, Git-path import preview).
- **Evidence, not authority** — cub-scout is the read-only witness. ConfigHub (via `cub`) is the authority for intended state.

For more on the security model, see [SECURITY.md](SECURITY.md).

---

## Contributing

Contributions welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

- **Found a bug?** [Open an issue](https://github.com/confighub/cub-scout/issues)
- **Have an idea?** Start a discussion
- **Want to contribute?** PRs welcome

---

## Community

- **Discord:** [discord.gg/confighub](https://discord-auth.confighub.net/discord/join)
- **Issues:** [GitHub Issues](https://github.com/confighub/cub-scout/issues)
- **Website:** [confighub.com](https://confighub.com)
