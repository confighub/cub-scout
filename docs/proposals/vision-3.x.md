# cub-scout 3.x Vision & Roadmap (3.0 – 3.5)

**Status (2026-11-27):**

| Milestone | Status | Release / PRs | What it delivers |
|---|---|---|---|
| **3.0** | **Shipped** | [v3.0.0](https://github.com/confighub/cub-scout/releases/tag/v3.0.0) (`#989`–`#1023`) | Single-cluster agent witness: `landed --verify-pod-images`, Flux/Argo CD/Helm v3/v4/kro/Crossplane parity, `snapshot` + `--recording`, writer contention & revert prediction, `--if-unchanged` ~89B delta polls, CI-enforced byte ceilings (99.78% smaller). |
| **3.1** | **Shipped** | [v3.1.0](https://github.com/confighub/cub-scout/releases/tag/v3.1.0) (`#1025`–`#1033`) | Complete read-only agent write loop: `before-write` (CLI + MCP), offline Ed25519 `receipt keygen`/`sign`/`verify`, offline `trace --recording` + sanitized Pod/PDB snapshots, `watch`/`bot` `answer` envelopes, multi-arch GHCR bot deploy kit (`#520`), Argo Rollouts parity, offline `graph blast-radius` (82.98% smaller). |
| **3.2** | **Shipped** | [v3.2.0](https://github.com/confighub/cub-scout/releases/tag/v3.2.0) (`#1034`–`#1041`) | Real-cluster Argo Rollouts `v1.8.3` CI lane (`rollouts-lane.sh`), live-cluster bounded `graph blast-radius` (CLI + MCP `graph_blast_radius` + `before-write --include-blast-radius`), reusable GitHub Actions CI promotion gate, ops-bot consumer contract, offline multi-cluster `receipt verify-fleet` + 4-cluster example, offline in-toto v1 `receipt export-attestation`, and `cub mini` hook contract. |
| **3.3 – 3.4** | **Next (Ecosystem & Adoption)** | Maintainer / external | External Krew index PR + official MCP Registry submission (`server.json`), broad G5 `agent/<tool>[/<run>]` field-manager adoption across coding-agent skills, real-cluster Modelplane parity when gated, and standing 3.x release benchmarks. |
| **3.5** | **Gated on ConfigHub `D6`** | `#600`, `G4` | Connected multi-cluster fleet promotion proof: opt-in per-cluster observed-fact / `G4` `ChangeWorkflow` attestation publishing into ConfigHub so `dev` → `staging` → `prod-east` → `prod-west` promotion gates run live without a second fleet DB. |

This roadmap extends [agentic GitOps observability](../reference/agentic-gitops-observability.md)
from **single-change verification (3.0)** and the **read-only write loop &
offline fleet proof (3.1–3.2)** to **the independent, deterministic trust layer
for multi-agent, multi-cluster Kubernetes (3.5)**.

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
3.2  [shipped]  Live proof, CI gate & fleet     Real-cluster Rollouts CI lane, live graph blast-radius + MCP, before-write --include-blast-radius, CI promotion gate, ops-bot contract, offline receipt verify-fleet + 4-cluster kit, in-toto v1 export-attestation, cub mini contract
3.3  [next]     External distribution & skills  Krew index PR, MCP Registry listing (server.json), G5 agent/* adoption in coding-agent skills, real-cluster Modelplane parity when gated
3.5  [gated]    ConfigHub fleet promotion proof Upstream D6 (#600 observed facts, G4 ChangeWorkflow attestations) composing live dev -> staging -> prod promotion gates
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

### 3.2 — Real-Cluster Proof, CI Gate, Live Blast-Radius & Offline Fleet Verification *(shipped in `v3.2.0`, 2026-11-27)*
Shipped in `v3.2.0` (`#1034`–`#1041`):
- **Real-cluster Argo Rollouts `v1.8.3` CI lane (`#1037`):** `scripts/ci/rollouts-lane.sh` wired into `CI / GitOps E2E`, proving `landed ro/<name> --verify-pod-images` (`landed`, `converging`, `not-landed`) against real `argoproj.io/v1alpha1` Rollouts and Pods in `kind`.
- **Live-cluster `graph blast-radius` & MCP parity (`#1037`, `#1041`):** Bounded, sanitized live-cluster reads with `--kube-context`, `before-write --include-blast-radius` (CLI + MCP, capped at 5 workloads, `<= 700B` answer ceiling), and standalone read-only MCP tool `graph_blast_radius`.
- **Reusable CI promotion gate & ops-bot consumer contract (`#1034`):** Drop-in read-only GitHub Actions composite gate (`examples/ci-gates/verify-landed/`) and `examples/bot/consumer-contract.md` for `k8sbot`, Pilot, and incident bots.
- **Offline multi-cluster `receipt verify-fleet` & 4-cluster kit (`#1035`, `#1040`):** Deterministic, fail-closed Ed25519 aggregation across `dev` → `staging` → `prod-east` → `prod-west` (`examples/fleet-promotion-proof/`, `docs/howto/fleet-promotion-proof.md`).
- **Offline in-toto Statement v1 export (`#1038`):** `cub-scout receipt export-attestation` emitting `https://confighub.com/attestations/landed/v1` ready for ConfigHub `G4` / `ChangeWorkflow` / `cosign`.
- **`cub mini` hook contract & standing benchmarks (`#1036`, `#1039`):** Clean read-only witness vs write-owner contract (`docs/howto/cub-mini-integration.md`), Krew manifest updates, and 13-surface `answer-sizes.md` benchmarks.

---

### 3.3 – 3.4 — Ecosystem Distribution & Write-Path Adoption *(next)*
1. **External registry submissions:** Submit `dist/krew/cub-scout.yaml` to the upstream `kubernetes-sigs/krew-index` and publish `server.json` to the official MCP Registry.
2. **Broad G5 `agent/<tool>[/<run>]` adoption:** Standardize `--field-manager=agent/<tool>/<run>` across coding-agent skills and `cub mini` write flows so multi-agent contention is visible on every cluster.
3. **Real-cluster Modelplane parity (when gated):** Add a real-cluster CI lane when Modelplane control-plane fixtures are ready to run deterministically in CI.

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
