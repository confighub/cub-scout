# cub-scout 3.x Vision & Roadmap (3.1 – 3.4+)

**Status:**
- **3.0 (`main`, not yet tagged):** Complete across the `/v3` transition (`#991`–`#995`, `#1000`), agent loop (`A(1)`–`A(4)`, `G1`, `G2`, `G5`), and Package C (`#996`, `#997`, `#1005`, `#1007`).
- **3.1 (Authority & Durable Proof):** Scoped, waiting on ConfigHub agreement (`#600`, `G4`, `#889`, `#520`).
- **3.2 (Universal Witness — approved 2026-11-24):** Focuses on **Theme 3** (Cross-controller delivery parity, gated by real-cluster CI lanes — Helm v3/v4 slice shipped in `#1016`) and **Theme 4** (Offline / bundle-based verification — `landed --recording` slice shipped in `#1015`).
- **3.3 – 3.4 (Proposed creative direction below):** Multi-agent safety, independent pod-digest witness, blast-radius pre-write checks (eval-gated), and an open agentic GitOps witness standard.

This roadmap extends [agentic GitOps observability](../reference/agentic-gitops-observability.md)
from **single-change observation (3.0)** to **the universal deterministic trust
layer for multi-agent, multi-controller Kubernetes (3.x)**.

---

## 1. Unchanged Boundaries (What 3.x Will NOT Become)

Every rule from `CLAUDE.md` and the `not-planned` list in
[docs/roadmap-3.0-execution.md](../roadmap-3.0-execution.md#trimmed-plan--2026-11-10)
remains binding across all of 3.x:

- **No cluster writes or remediation.** `cub-scout` stays the witness; Pilot or
  a gate is the judge; ConfigHub is the authority.
- **No AI/ML inference.** Every fact is parsed deterministically from real
  objects, labels, annotations, `managedFields`, and controller reports.
- **No metrics, logs, or network traffic.** Not an APM or Prometheus/Loki
  replacement.
- **No bespoke UI** beyond the CLI and TUI renderings of one shared model.
- **No background watch service or second fact store.** `#539` stays
  `not-planned`; durable cross-session facts belong in ConfigHub (`#600`),
  never in a competing `cub-scout` database.
- **No unmeasured agent claims.** Every 3.x capability must pass Principle 9
  (measured against ordinary read-only tools on recorded fixtures) and
  Principle 10 (the six questions) before any benefit is claimed.

---

## 2. Milestone Arc: 3.0 → 3.4

```text
3.0  Single-change agent loop        "Did my change land? Who owns this field? Will it revert?"
3.1  Proof that outlives the session Signed receipts, ConfigHub facts (#600), ChangeWorkflow gates (G4), bot image (#520)
3.2  Universal & offline witness     Every major controller (real-cluster proven), independent pod-digest proof, zero-cluster snapshots
3.3  Multi-agent safety & blast-rad  Writer-contention detection, pre-write safety check (eval-gated graph), delta/fingerprint answers
3.4  Fleet witness & open standard   Cross-cluster promotion proof via ConfigHub, versioned witness spec + conformance suite
```

---

## 3. Detailed Release Themes

### 3.1 — Proof That Outlives the Session *(waits on ConfigHub agreement)*
- **Facts in ConfigHub (`#600`, Package D):** `cub-scout` as the sole opt-in author of observed cluster facts.
- **Delivery attestations (`G4`):** Signed witness receipt format that a ConfigHub `ChangeWorkflow` can require before promoting a revision.
- **Signed receipts (`#889`)** and **pullable multi-arch `cub-scout bot` image (`#520`)**.
- **Envelope conformance (`#596` remainder):** bring `bot` and `watch` streaming surfaces onto the same `answer` envelope contract as the 10 CLI/MCP commands.

---

### 3.2 — Universal Witness & Zero-Cluster Verification *(approved; first slices landed)*

#### Theme 3: Cross-Controller Delivery Parity (Real-Cluster Proven)
**The agent's question:** *"Did my change land?"* regardless of whether the cluster runs Flux, Argo CD, Helm, Sveltos, Crossplane, kro, or Modelplane — and without trusting only the deployer's self-report.

- **Shipped on `main`:**
  - Flux (`Kustomization`, `HelmRelease`, `GitRepository`, `OCIRepository`) + Argo CD (`Application`) proven in `scripts/ci/landed-lane.sh` (`#1001`, `#1002`).
  - Helm v3 & v4 release Secrets (`sh.helm.release.v1.<release>.v<N>`) proven in `scripts/ci/helm-lane.sh` (`#1016`).
- **Next in 3.2:**
  1. **Independent Pod/Image-Digest Witness (`landed --verify-pod-images`):** Close the remaining "deployer grading its own homework" gap by comparing running Pod `status.containerStatuses[].imageID` digests against the expected image/OCI digest, reporting mismatches as `not-landed` even when the controller claims `Synced`/`Ready`.
  2. **Sveltos, kro, Crossplane, and Modelplane parity:** Add adapters **only** where a real-cluster CI lane proves what the controller actually writes. Where a controller writes no revision/digest anchor (such as Sveltos today), return `not-observable` with the exact reason rather than inferring agreement.
  3. **Workload breadth:** Extend `landed` beyond `Deployment`/`StatefulSet`/`DaemonSet` to `Job` and `CronJob` (and Argo Rollouts where installed).

#### Theme 4: Zero-Cluster / Air-Gapped & CI-Gate Verification
**The agent's question:** *"Before I open a PR or promote a release, can I verify a recorded cluster snapshot or receipt chain offline?"*

- **Shipped on `main`:**
  - `cub scout landed --recording <file>` evaluates multi-object Flux, Argo CD, and Helm snapshots offline with zero cluster or network access (`#1015`, `#1016`).
- **Next in 3.2:**
  1. **`cub scout snapshot` bounded capture:** One read-only command to capture a minimal, sanitized, size-bounded cluster snapshot for CI artifacts, incident tickets, or sandboxed agents.
  2. **Universal `--recording` / `--from-bundle` parity:** Run `doctor`, `gitops status`, and `trace` against the same offline snapshot format, always surfacing `observedAt` and staleness explicitly so offline evidence is never mistaken for live state.
  3. **Offline receipt-chain verification:** Verify signed `receipt/v1` / `G4` attestations in air-gapped CI without cluster or ConfigHub network access.

---

### 3.3 — Multi-Agent Safety & Blast-Radius Intelligence *(proposed for maintainer selection)*

When multiple coding agents, ops bots (`k8sbot`, Pilot), controllers, and humans share a cluster, three failure modes dominate:

1. **Writer Contention / "Reconcile Fighting" Detection (building on G5 `#1010` + A(2) `#1011`):**
   - **The agent's question:** *"Are two writers (e.g. `agent/remediator` vs `argocd-controller`, or HPA vs Flux) fighting over this field?"*
   - **Shape:** Deterministically compare `managedFields` co-claimants, `resourceVersion` churn, and controller reconcile events across two point-in-time reads (or a recorded bundle) to flag active field-level contention without guessing writer intent.
2. **One-Call Pre-Write Safety Check (`cub scout before-write` + eval-gated Theme 1 graph):**
   - **The agent's question:** *"If I change this resource/field right now, who owns it, will a controller revert it, what downstream workloads depend on it, and are any freeze/sync windows active?"*
   - **Shape:** Compose `explain --field-path` (`A(2)`), `gitops settings` (Argo `syncWindows`, Flux `suspend`), PodDisruptionBudget presence, and an **ephemeral, in-memory** dependency fan-out (`ancestors`/`descendants` from a single `graph export` snapshot).
   - **Hard eval gate:** Ship the ephemeral graph fan-out **only** if a recorded-fixture benchmark (`PRE-03`, `PRE-04`, multi-controller graphs) proves measurable byte and turn reduction over repeated `trace`/`kubectl` calls. Never persist the graph as a second fact store.
3. **Delta / Fingerprint Answers (`--if-unchanged <hash>`):**
   - **The agent's question:** *"Has anything changed since my last check 30 seconds ago?"*
   - **Shape:** Allow an agent or bot to pass the fingerprint of a prior `answer` block; if the live observation produces the identical answer, return a ~120-byte `unchanged` envelope instead of re-paying for the full payload.

---

### 3.4 — Fleet-Scale Witness & Open Agentic GitOps Standard *(proposed)*

1. **Cross-Cluster Promotion Proof (ConfigHub-backed, zero second DB):**
   - Compose per-cluster `landed` receipts stored in ConfigHub (`#600`) to answer *"Did revision `R` land across `dev` → `staging` → `prod-east` → `prod-west`?"* with exact per-cluster `observedAt` and staleness boundaries.
2. **Open "Agentic GitOps Witness" Spec & Conformance Suite:**
   - Publish the `answer` envelope, the 4-state `landed` contract, the `agent/<tool>[/<run>]` field-manager convention (`G5`), and the quoted-cluster-text prompt-injection safety rules as a versioned spec + fixture suite that ops bots (`k8sbot`, Pilot, CI gates) and MCP hosts can test against.
3. **Ecosystem & Standing Benchmarks (Package F follow-through):**
   - First-class listings across MCP directories, `cub plugin`, Homebrew, `kubectl krew`, and signed multi-arch container images (`#520`).
   - Expand the zero-credit 24-case bytes-and-tokens benchmark (`docs/reference/answer-sizes.md`) into a standing release gate tracking bytes, turns, and zero-false-positive invariants across every 3.x minor release.

---

## 4. What Stays Explicitly Out of 3.x

- Rebuilding `fleet outliers` without stable ConfigHub cross-cluster lineage (`#562`).
- Fallback live-status publication (`#641`), long-lived watch cache (`#539`), bespoke UI/Commander work (`#519`), and speculative controller adapters without real-cluster CI lanes (`#601`, `#602`, `#584`).

---

## 5. Open Decisions for the Maintainer

1. **3.2 priority:** Should **Independent Pod/Image-Digest Witness** (`landed --verify-pod-images`) be the next 3.2 slice after Helm parity (`#1016`), or should Sveltos/kro/Crossplane adapters come first?
2. **3.3 proposals:** Which of the three 3.3 ideas (writer-contention detection, one-call `before-write` + eval-gated ephemeral graph, `--if-unchanged` delta answers) should be scoped next?
3. **3.4 standard:** Worth publishing the `answer` envelope + `agent/<tool>[/<run>]` + quoted-cluster-text rules as a standalone versioned spec for external bots and MCP hosts?
