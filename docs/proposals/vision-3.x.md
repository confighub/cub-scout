# cub-scout 3.x Vision & Roadmap (3.1 – 3.5)

**Status (2026-11-26):**
- **`v3.0.0` (published 2026-11-25):** Shipped at https://github.com/confighub/cub-scout/releases/tag/v3.0.0 (`03a79769`) — `/v3` contract (`#991`–`#995`, `#1000`), `cub scout landed` with independent Pod/image-digest witness (`--verify-pod-images`) and multi-controller parity (Flux, Argo CD, Helm v3/v4, kro, Crossplane, honest `not-observable` for Sveltos; `Deployment`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob`), zero-cluster `cub scout snapshot` + `--recording` on `landed`/`gitops status`/`doctor`, field writer-contention + revert prediction (`explain --field-path`), `--unrecorded-only`, G5 `agent/<tool>[/<run>]`, and Package C byte ceilings + `--view summary` + `--if-unchanged` delta answers (`#989`–`#1023`).
- **`v3.1.0` (published 2026-11-26):** Shipped at https://github.com/confighub/cub-scout/releases/tag/v3.1.0 (`084a9e5f`, `#1025`–`#1033`) — complete strictly read-only agent write-loop:
  - Pre-write safety sensor `cub scout before-write` + MCP `before_write` (`#1027`, 672B compact answer `<= 700B` ceiling).
  - Offline Ed25519 `receipt keygen`/`sign`/`verify` (`signed-receipt/v1`, `#1026`, Refs `#889`).
  - Full offline `trace --recording` + sanitized `Pod`/`PodDisruptionBudget` capture in `snapshot` (`#1028`).
  - Standard `answer` envelopes and `<= 700B` ceilings on `watch` and `bot` streaming events (`#1025`, Refs `#596`).
  - Multi-arch (`linux/amd64`, `linux/arm64`) GHCR `cub-scout bot` image config and hardened read-only deploy kit (`#1029`, Refs `#520`).
  - Argo Rollouts (`argoproj.io/v1alpha1` `Rollout`) parity across `landed`, `before-write`, `trace`, and `snapshot` (`#1031`).
  - Eval-gated ephemeral offline `graph blast-radius` (`#1032`; 82.98% overall byte reduction, 89.65% on 4-hop Flux, 1,084B `<= 1,200B` ceiling).
  - Front-door agent write-loop guide and `v3.1.0` release notes (`#1030`, `#1033`).
- **Remaining in 3.2 – 3.5:**
  - **3.2 (Real-Cluster Controller & Live Graph Proof):** Real-cluster Argo Rollouts CI lane (and Modelplane when real-cluster gated), plus live-cluster `graph blast-radius` parity with bounded API reads.
  - **3.3 – 3.4 (Write-Path Adoption & Fleet Receipt Verification):** CI promotion gate and ops-bot consumer contract (`k8sbot`, Pilot), offline multi-cluster `receipt verify-fleet` aggregation, standing 3.x release benchmarks (`docs/reference/answer-sizes.md`), and Krew/MCP registry distribution.
  - **3.5 (Connected Multi-Cluster Fleet Promotion Proof):** Upstream ConfigHub `D6` storage agreement (`#600` observed facts and `G4` `ChangeWorkflow` attestations) and connected `dev` → `staging` → `prod` fleet promotion proof.

This roadmap extends [agentic GitOps observability](../reference/agentic-gitops-observability.md)
from **single-change verification (3.0) and the read-only write loop (3.1)** to
**the independent, deterministic trust layer for multi-agent, multi-cluster
Kubernetes (3.5)**.

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
3.0  [shipped]  Single-cluster agent witness    landed (--verify-pod-images), Flux/Argo/Helm/kro/Crossplane, snapshot/--recording, contention, 99.78% smaller answers
3.1  [shipped]  Read-only write loop & offline  before-write, Ed25519 receipt sign/verify (#889), trace --recording + Pod/PDB snapshot, watch/bot envelopes (#596), multi-arch bot kit (#520), Argo Rollouts parity, offline graph blast-radius
3.2  [next]     Real-cluster & live graph       Real-cluster Argo Rollouts CI lane (& Modelplane when gated), live-cluster bounded graph blast-radius
3.3  [next]     Write-path CI & bot adoption    CI promotion gate & ops-bot consumer contract (k8sbot/Pilot), standing 3.x release benchmarks, krew/MCP registry
3.4  [next]     Offline fleet verification      receipt verify-fleet multi-cluster aggregation with per-cluster observedAt & staleness boundaries
3.5  [gated]    ConfigHub fleet promotion proof Upstream D6 (#600 observed facts, G4 ChangeWorkflow attestations) composing dev -> staging -> prod promotion gates
```

---

## 3. Detailed Release Themes

### 3.1 — Read-Only Write Loop, Offline Signing & Bot Packaging *(shipped in `v3.1.0`, 2026-11-26)*
Shipped in `v3.1.0` (`#1025`–`#1033`):
- **Pre-write safety sensor (`cub scout before-write` & MCP `before_write`, `#1027`):**
  Composes `explain --field-path` (live value, revert prediction, writer
  contention) with live Argo `syncWindows`, Flux `suspend`, and
  `PodDisruptionBudget` checks into one sub-KB read-only answer (672B `<= 700B`
  ceiling), with offline `--recording` support.
- **Offline Ed25519 signed receipts (`#1026`, Refs `#889`):** `receipt keygen`,
  `receipt sign`, and `receipt verify` (`signed-receipt/v1`) using Go stdlib
  cryptography only.
- **Complete offline bundle parity (`#1028`):** `trace --recording` across
  Flux, Argo CD, Helm, kro, Crossplane, and Argo Rollouts, plus sanitized
  `Pod` and `PodDisruptionBudget` capture in `cub scout snapshot`.
- **Envelope conformance on streaming surfaces (`#1025`, Refs `#596`):**
  Standard `answer` envelopes on `watch` (609B) and `bot` (624B) events, both
  CI-enforced at `<= 700B`.
- **Multi-arch GHCR `cub-scout bot` image & hardened deploy kit (`#1029`, Refs `#520`):**
  Non-root, read-only `linux/amd64` + `linux/arm64` release config and
  `examples/bot/` manifests.
- **Argo Rollouts parity (`#1031`):** `argoproj.io/v1alpha1` `Rollout` support
  across `landed`, `before-write`, `trace`, and `snapshot`.
- **Eval-gated ephemeral offline `graph blast-radius` (`#1032`):** Bounded
  in-memory dependency fan-out (`ancestors`/`descendants`/`affectedWorkloads`)
  with 82.98% overall byte reduction (89.65% on 4-hop Flux; 1,084B `<= 1,200B`
  ceiling) and zero persistent second fact store.

---

### 3.2 — Real-Cluster Controller & Live Graph Proof *(remaining)*
Already shipped in `v3.0.0` / `v3.1.0`: Flux, Argo CD, Helm v3/v4, kro,
Crossplane, honest `not-observable` for Sveltos, Argo Rollouts
(fixture/contract-tested), `Job`/`CronJob`, `--verify-pod-images`,
`cub scout snapshot` (including sanitized Pods/PDBs), offline `trace`, offline
`graph blast-radius`, and Ed25519 `receipt sign`/`verify`.

Remaining in 3.2:
1. **Real-cluster Argo Rollouts CI lane (and Modelplane when real-cluster gated):**
   Prove what the live Argo Rollouts controller writes on a disposable kind
   cluster in CI so Rollouts parity carries the same live-cluster guarantee as
   Flux and Argo CD.
2. **Live-cluster `graph blast-radius` parity:** Bounded live-cluster reads for
   `graph blast-radius` matching the offline `--recording` output schema and
   truncation bounds without introducing a persistent graph database.

---

### 3.3 – 3.4 — Write-Path Adoption Flywheel & Offline Fleet Verification *(remaining)*
**Clean boundary with `cub mini` and the `cub` plugin:**
- `cub` / `cub mini` / plugin owns **intended state and writes** (spaces,
  units, targets, edits, mutations).
- `cub-scout` owns **read-only live-cluster sensing** that `cub mini` and
  agent skills call immediately *before* writing (`before-write`) and *after*
  writing (`landed --verify-pod-images`).

Remaining in 3.3 – 3.4:
1. **CI promotion gate & ops-bot consumer contract:** Reference GitHub Actions
   / CI gate and ops-bot (`k8sbot`, Pilot) integration consuming `before-write`,
   `landed --verify-pod-images`, `bot`/`watch` `answer` envelopes, and
   `signed-receipt/v1` artifacts.
2. **Offline multi-cluster `receipt verify-fleet`:** Verify and aggregate a set
   of signed per-cluster receipts offline, surfacing per-cluster `observedAt`,
   staleness, and missing/invalid signatures without requiring a fleet DB.
3. **Writer self-identification (`G5`):** Broad adoption of
   `agent/<tool>[/<run>]` in `--field-manager` so multi-agent contention is
   immediately visible on every cluster.
4. **Standing release benchmarks & distribution:** Maintain the free 24-case
   bytes-and-tokens benchmark, 13-surface `answer` ceilings, and
   `graph blast-radius` benchmark (`docs/reference/answer-sizes.md`) as a
   standing gate across every 3.x minor release, alongside Krew and MCP
   Registry distribution.

---

### 3.5 — Connected Multi-Cluster Fleet Promotion Proof *(waits on ConfigHub `D6` agreement)*
- **Why multi-cluster (`>1` cluster) lives here:** Standalone `cub-scout` stays
  strictly single-cluster. In connected mode, each cluster runs its own
  read-only `cub-scout` (CLI or `cub-scout bot`) and publishes per-cluster
  observed facts tagged with `(cluster/Target, Release/Revision, observedAt)`
  into ConfigHub — giving multi-cluster visibility without a second fleet DB.
- **How facts are saved in ConfigHub (`#600`, Package D):**
  - `cub-scout` never writes to `Release.LiveStatus` (which `argobot` owns;
    `#641` stays rejected to prevent writer conflict).
  - Pending upstream ConfigHub `D6` agreement, `cub-scout` writes opt-in,
    off-by-default observed facts as either dedicated `observed`-labelled
    ConfigHub Units/objects (new revision only when observation changes),
    **ConfigHub Attestations (`G4`)** on Releases that a `ChangeWorkflow` can
    require as a promotion gate, or both.
- **Fleet-scale promotion proof:** Compose per-cluster `cub-scout`
  receipts/attestations stored in ConfigHub (`#600`, `G4`) to answer *"Did
  revision `R` land across `dev` → `staging` → `prod-east` → `prod-west`?"*
  with exact per-cluster `observedAt` and staleness boundaries — turning
  standalone `cub-scout` adoption into a seamless ConfigHub upgrade path.

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
   Units, Release Attestations, or both — this unlocks 3.5 connected fleet
   promotion proof.
2. **`cub mini` integration:** Confirm the exact pre-write / post-write hook
   contract between `cub mini` (writes) and `cub scout` (`before-write` /
   `landed --verify-pod-images` read-only witness).
3. **3.2 real-cluster controller gate:** Confirm sequencing between the
   real-cluster Argo Rollouts CI lane (backing the `v3.1.0` Rollouts parity)
   and real-cluster Modelplane parity.

---

## 6. The 3.5 Value Proposition (Positioning Anchor)

> **`cub-scout` is the independent, deterministic witness that lets AI agents,
> CI gates, and platform teams *prove* a change actually landed across
> Kubernetes clusters — at 99.8% lower token cost than `kubectl`, with zero
> hallucination and zero cluster write permissions.**

- **Before writing (`before-write` / `explain --field-path`):** One sub-KB
  read-only call for `cub mini` and agent skills: live field value, last
  manager (`agent/*` included), Flux/Argo revert risk, writer contention,
  `syncWindows`, and PDBs.
- **After writing (`landed --verify-pod-images`):** One uniform,
  revision-anchored answer across Flux, Argo CD, Helm v3/v4, kro, Crossplane,
  and Argo Rollouts — verifying running Pod `imageID` digests independently of
  controller self-report.
- **Agent economics & safety:** 99.78% fewer bytes than raw K8s objects
  (CI-enforced ceilings + 89-byte `--if-unchanged` delta polls), zero LLM in
  the read path, all cluster text quoted, air-gapped via `snapshot` +
  `--recording`.
- **Standalone → ConfigHub graduation (3.5):** Start in 30 seconds on any
  cluster with zero lock-in; add ConfigHub for per-cluster observed facts
  (`#600`), signed delivery attestations (`G4`, `#889`), and multi-cluster
  `ChangeWorkflow` promotion gates (`dev` → `staging` → `prod`).
- **Core line:** *"Don't let your deploy controller or your coding agent grade
  its own rollout. `cub-scout` is the independent witness that makes autonomous
  GitOps safe in production."*
