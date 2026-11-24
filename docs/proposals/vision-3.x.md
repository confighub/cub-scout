# Proposal: cub-scout 3.x Vision (3.2 and beyond)

**Status:** Draft proposal for maintainer review — **not approved work, not a
roadmap commitment.** Nothing in this document starts implementation without
explicit approval and an eval plan.

This draft extends [agentic GitOps observability](../reference/agentic-gitops-observability.md)
beyond 3.0 (the agent's single-change loop) and 3.1 (facts in ConfigHub `#600`,
delivery attestations `G4`, signed receipts `#889`, pullable bot image `#520`).

---

## 1. Unchanged boundaries (what 3.x will NOT become)

Every rule from `CLAUDE.md` and the `not-planned` list in
[docs/roadmap-3.0-execution.md](../roadmap-3.0-execution.md#trimmed-plan--2026-11-10)
remains binding in 3.x:

- **No cluster writes or remediation.** cub-scout stays the witness; Pilot is
  the judge; ConfigHub is the authority.
- **No AI/ML inference.** Every fact is parsed deterministically from real
  objects, labels, annotations, `managedFields`, and controller reports.
- **No metrics, logs, or network traffic.** Not an APM or Prometheus/Loki
  replacement.
- **No UI of its own** beyond the CLI and TUI renderings of the same model.
- **No background watch service or second fact store.** `#539` (long-lived
  watch cache/service) stays `not-planned`; durable cross-session facts belong
  in ConfigHub (`#600`), never in a competing cub-scout database.
- **No unmeasured agent claims.** Every 3.x capability must pass Principle 9
  (measured against ordinary read-only tools on recorded fixtures) and
  Principle 10 (the six questions) before any benefit is claimed.

---

## 2. Where 3.0 and 3.1 leave the agent's loop

By the end of 3.1, an agent can:
1. **Look before writing** (`explain`, `gitops settings`, field attribution).
2. **Witness after writing** (`did my change land` with `converging` + bounded
   wait, `compare source-truth`, `release check`).
3. **Leave proof that outlives the session** (signed receipts `#889`, observed
   facts in ConfigHub `#600`, ChangeWorkflow delivery attestations `G4`).

What remains hard for an agent in a **multi-agent, multi-controller, or
multi-cluster** environment after 3.1?

---

## 3. Candidate 3.2+ themes (for maintainer selection)

### Theme 1: Ephemeral local graph query layer for multi-hop questions

**The agent's question:** *"If I change this ConfigMap / HelmRelease /
ApplicationSet / Crossplane Composition, which workloads and namespaces are
downstream of it — and what is the shortest broken path?"*

- **Shape:** Load a point-in-time `cub scout graph export --format json`
  snapshot into an **ephemeral, read-only, in-memory** (or single-command
  embedded) graph query layer exposed via CLI and MCP (e.g. bounded queries:
  `ancestors`, `descendants`, `broken-paths`, `shared-dependencies`).
- **Hard guardrails:**
  - **Never a second fact store beside ConfigHub.** The graph is strictly
    ephemeral per snapshot/invocation (carrying `observedAt` and `cluster`),
    never persisted as an authoritative inventory.
  - **Strict size ceilings.** Every query returns bounded JSON carrying the
    `answer` envelope, with declared byte ceilings.
  - **Eval gate first.** Must demonstrate on recorded multi-controller graphs
    (e.g. `PRE-03`, `PRE-04`, Crossplane + Argo + Flux) that graph queries
    reduce evidence bytes and turns compared to repeated `trace` / `kubectl`
    calls — otherwise do not ship.

### Theme 2: Multi-writer & agent-identity attribution (building on G5)

**The agent's question:** *"Which agent, controller, or human last modified
this field, and are two writers fighting over it?"*

- **Shape:** Standardise on `managedFields` field-manager identity conventions
  proposed in G5 (reporting exactly what Kubernetes records, never guessing),
  and surface **writer contention** (e.g. a controller and an external writer
  touching the same field path across two point-in-time snapshots or observed
  reconcile events).
- **Guardrail:** Deterministic field-path comparison only; unknown managers
  stay `unknown`.

### Theme 3: Cross-controller delivery parity (real-cluster proven)

**The agent's question:** *"Did my change land?"* regardless of whether the
cluster runs Flux, Argo CD, Sveltos, Helm, Crossplane, kro, or Modelplane.

- **Shape:** Extend the 3.0 landing check (`A(1)`) and `release check` beyond
  Flux + Argo CD to Sveltos, Crossplane, kro, and Modelplane — **only** where
  a real-cluster CI lane proves what the controller actually writes (following
  the 2026-11-13 lesson: *"Record from a real cluster before claiming a read
  works"*).
- **Guardrail:** Where a controller writes no digest or revision anchor (such
  as Sveltos today), report `not-observable` / `not-compared` with the exact
  reason rather than inferring agreement.

### Theme 4: Zero-cluster / air-gapped & CI-gate agent workflows

**The agent's question:** *"Before I open a PR or promote a release, can I
verify a recorded bundle or receipt chain offline?"*

- **Shape:** Make every core `answer`-carrying query runnable against a
  recorded `cub scout snapshot` / debug bundle (building on existing offline
  `--from-bundle` / recorded-map contracts) so CI pipelines and sandboxed
  agents with no live kubeconfig can inspect captured cluster state and verify
  signed receipt chains offline.
- **Guardrail:** Always surface `observedAt` and staleness explicitly so an
  offline bundle is never mistaken for live cluster state.

### Theme 5: Ecosystem & agent-host distribution (Package F follow-through)

- First-class listings and version-verified installs across MCP registries,
  `cub plugin`, Homebrew, `kubectl krew` (once verified), and signed multi-arch
  container images (`#520`).
- Regular standing checks against upstream controllers and sibling MCP servers
  so cub-scout's witness role stays sharp and non-redundant.

---

## 4. What stays explicitly out of 3.x

- Rebuilding `fleet outliers` without stable ConfigHub cross-cluster lineage (`#562`).
- Fallback live-status publication (`#641`), long-lived watch cache (`#539`),
  bespoke UI/Commander work (`#519`), and speculative controller adapters
  without real-cluster CI lanes (`#601`, `#602`, `#584`).

---

## 5. Questions for the maintainer

1. Which of Themes 1–5 belong in a 3.2+ roadmap, and which should be dropped
   or deferred?
2. For Theme 1 (ephemeral local graph query layer), is a small prototype +
   recorded bytes-and-tokens comparison worth scoping after 3.0 ships?
3. Are there any ConfigHub 3.1+ primitives (Channels, Attestations, Resource
   graph) that should anchor 3.2 instead of local graph snapshots?
