# cub-scout 3.x Vision & Roadmap (3.1 – 3.5)

**Status (2026-11-25):**
- **`v3.0.0` (published):** Shipped at https://github.com/confighub/cub-scout/releases/tag/v3.0.0 (`03a79769`) — `/v3` contract (`#991`–`#995`, `#1000`), `cub scout landed` with independent Pod/image-digest witness (`--verify-pod-images`) and multi-controller parity (Flux, Argo CD, Helm v3/v4, kro, Crossplane, honest `not-observable` for Sveltos; `Deployment`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob`), zero-cluster `cub scout snapshot` + `--recording` on `landed`/`gitops status`/`doctor`, field writer-contention + revert prediction (`explain --field-path`), `--unrecorded-only`, G5 `agent/<tool>[/<run>]`, and Package C byte ceilings + `--view summary` + `--if-unchanged` delta answers (`#989`–`#1023`).
- **3.1 (Multi-Cluster Proof That Outlives the Session):** Waits on ConfigHub storage agreement (`D6`: `#600` observed facts, `G4` ChangeWorkflow attestations, `#889` signed receipts, `#520` pullable bot image).
- **3.2 (Remaining Universal & Offline Witness Polish):** Argo Rollouts / Modelplane parity (real-cluster gated) and offline `trace` + signed receipt-chain verification.
- **3.3 (Pre-Write Safety Sensor for `cub mini` & Agent Skills):** Strictly read-only live-cluster pre-write check (`before-write`) with zero overlap with `cub mini` / plugin write paths; eval-gated ephemeral dependency fan-out.
- **3.4 – 3.5 (Write-Path Adoption Flywheel & Fleet Promotion Proof):** `cub-scout` embedded on the write path of agent skills, CI gates, and ops bots (`k8sbot`, Pilot), graduating naturally into ConfigHub multi-cluster promotion gates.

This roadmap extends [agentic GitOps observability](../reference/agentic-gitops-observability.md)
from **single-change verification (3.0)** to **the independent, deterministic
trust layer for multi-agent, multi-cluster Kubernetes (3.5)**.

---

## 1. Unchanged Boundaries (What 3.x Will NOT Become)

Every rule from `CLAUDE.md` and the `not-planned` list in
[docs/roadmap-3.0-execution.md](../roadmap-3.0-execution.md#trimmed-plan--2026-11-10)
remains binding across all of 3.x:

- **No cluster writes or remediation.** `cub-scout` stays the read-only
  witness; `cub` / `cub mini` / plugins own intended state and writes; Pilot
  or a `ChangeWorkflow` gate is the judge; ConfigHub is the authority.
- **No AI/ML inference.** Every fact is parsed deterministically from real
  objects, labels, annotations, `managedFields`, and controller reports.
- **No metrics, logs, or network traffic.** Not an APM or Prometheus/Loki
  replacement.
- **No bespoke UI** beyond the CLI and TUI renderings of one shared model.
- **No background watch service or second fleet database.** `#539` stays
  `not-planned`. Multi-cluster state lives in ConfigHub (`#600`), never in a
  competing `cub-scout` database.
- **No unmeasured agent claims.** Every 3.x capability must pass Principle 9
  (measured against ordinary read-only tools on recorded fixtures) and
  Principle 10 (the six questions) before any benefit is claimed.

---

## 2. Milestone Arc: 3.0 → 3.5

```text
3.0  Single-cluster agent witness    landed (--verify-pod-images), Flux/Argo/Helm/kro/Crossplane, snapshot/--recording, contention, 99.78% smaller answers
3.1  Multi-cluster durable proof     Per-cluster facts in ConfigHub (#600), ChangeWorkflow attestations (G4), signed receipts (#889), pullable bot (#520)
3.2  Universal & air-gapped witness  Argo Rollouts & Modelplane (real-cluster gated), offline trace & signed receipt verification
3.3  Pre-write sensor for cub mini   Read-only before-write check (syncWindows, PDBs, contention, revert risk, eval-gated ephemeral graph)
3.4  Write-path adoption flywheel    First-class in agent skills, CI gates, ops bots (k8sbot/Pilot), MCP registries, krew, G5 agent/* adoption
3.5  Fleet promotion witness         Cross-cluster promotion proof (dev -> staging -> prod) composed from ConfigHub facts & signed attestations
```

---

## 3. Detailed Release Themes

### 3.1 — Multi-Cluster Proof That Outlives the Session *(waits on ConfigHub D6 agreement)*
- **Why multi-cluster (`>1` cluster) lives here:** Standalone `cub-scout` stays
  strictly single-cluster. In connected mode, each cluster runs its own
  read-only `cub-scout` (CLI or `cub-scout bot`) and publishes per-cluster
  observed facts tagged with `(cluster/Target, Release/Revision, observedAt)`
  into ConfigHub — giving multi-cluster visibility without a second fleet DB.
- **How facts are saved in ConfigHub (`#600`, Package D):**
  - `cub-scout` never writes to `Release.LiveStatus` (which `argobot` owns;
    `#641` stays rejected to prevent writer conflict).
  - Pending ConfigHub agreement (`D6`), `cub-scout` writes opt-in, off-by-default
    observed facts as either dedicated `observed`-labelled ConfigHub
    Units/objects (new revision only when observation changes), **ConfigHub
    Attestations (`G4`)** on Releases that a `ChangeWorkflow` can require as a
    promotion gate, or both.
- **Signed receipts (`#889`)** (verifiable offline first) and **pullable
  multi-arch `cub-scout bot` image (`#520`)**.
- **Envelope conformance (`#596` remainder):** bring `bot` and `watch`
  streaming surfaces onto the same `answer` envelope contract as the 10
  CLI/MCP commands.

---

### 3.2 — Remaining Universal & Offline Witness Polish
Already shipped in `v3.0.0`: Flux, Argo CD, Helm v3/v4, kro, Crossplane,
honest `not-observable` for Sveltos, `Job`/`CronJob`, `--verify-pod-images`,
`cub scout snapshot`, and `--recording` on `landed`, `gitops status`, and
`doctor`.

Remaining in 3.2:
1. **Additional controllers (real-cluster gated only):** Argo Rollouts and
   Modelplane landing parity, only where a real-cluster CI lane proves what
   the controller writes.
2. **Complete offline bundle parity:** `--recording` / `--from-bundle` on
   `trace` and offline verification of signed `receipt/v1` / `G4` attestations
   with explicit `observedAt` / staleness markers.

---

### 3.3 — Pre-Write Safety Sensor for `cub mini` & Agent Skills
**Clean boundary with `cub mini` and the `cub` plugin:**
- `cub` / `cub mini` / plugin owns **intended state and writes** (spaces,
  units, targets, edits, mutations).
- `cub-scout` owns **read-only live-cluster sensing** that `cub mini` and
  agent skills call immediately *before* writing and *after* writing.

Capabilities:
1. **One-Call Pre-Write Safety Check (`cub scout before-write`):** Composes
   already-shipped `explain --field-path` (live value, revert prediction,
   writer contention) with live Argo `syncWindows`, Flux `suspend`, and
   PodDisruptionBudget checks into one <1 KB read-only answer.
2. **Eval-Gated Ephemeral Dependency Fan-Out (Theme 1):** Optional in-memory
   `ancestors`/`descendants` from a single `graph export` snapshot — shipped
   **only** if recorded-fixture benchmarks prove measurable byte/turn
   reduction over repeated `trace`/`kubectl` calls, and never persisted as a
   second fact store (if ConfigHub owns cross-resource graphs natively,
   delegate to ConfigHub instead).

---

### 3.4 – 3.5 — Write-Path Adoption Flywheel & Fleet Promotion Proof
Rather than publishing a spec for its own sake, the goal is **making
`cub-scout` mandatory on the agent write path**:
1. **Embedded in agent skills & CI gates:** Default post-apply check
   (`cub scout landed --verify-pod-images`) and pre-write check
   (`explain --field-path` / `before-write`) across Claude Code, Codex,
   Cursor, Antigravity, and CI promotion pipelines.
2. **Deterministic sensor for ops bots:** `cub-scout` (CLI/MCP + pullable
   `cub-scout bot` `#520`) serves as the zero-LLM evidence layer beneath
   Slack/incident bots (`k8sbot`, Pilot) so bots never hallucinate ownership
   or delivery state.
3. **Writer self-identification (`G5`):** Broad adoption of
   `agent/<tool>[/<run>]` in `--field-manager` so multi-agent contention is
   immediately visible on every cluster.
4. **Fleet-Scale Promotion Proof (3.5, ConfigHub-backed):** Compose
   per-cluster `cub-scout` receipts/attestations stored in ConfigHub (`#600`,
   `G4`) to answer *"Did revision `R` land across `dev` → `staging` →
   `prod-east` → `prod-west`?"* with exact per-cluster `observedAt` and
   staleness boundaries — turning standalone `cub-scout` adoption into a
   seamless ConfigHub upgrade path.
5. **Standing release benchmarks:** Expand the free 24-case bytes-and-tokens
   benchmark (`docs/reference/answer-sizes.md`) into a standing gate across
   every 3.x minor release.

---

## 4. What Stays Explicitly Out of 3.x

- Rebuilding `fleet outliers` without stable ConfigHub cross-cluster lineage (`#562`).
- Fallback live-status publication (`#641`), long-lived watch cache (`#539`),
  bespoke UI/Commander work (`#519`), and speculative controller adapters
  without real-cluster CI lanes (`#601`, `#602`, `#584`).

---

## 5. Open Decisions for the Maintainer

1. **ConfigHub `D6` (`#600` + `G4`):** Confirm with the ConfigHub team
   whether `cub-scout` observed facts should be stored as `observed`-labelled
   Units, Release Attestations, or both — this unlocks 3.1 and 3.5 fleet proof.
2. **`cub mini` integration:** Confirm the exact pre-write / post-write hook
   contract between `cub mini` (writes) and `cub scout` (read-only witness).
3. **3.2 next controller:** Prioritize Argo Rollouts or Modelplane first for
   real-cluster `landed` parity?
