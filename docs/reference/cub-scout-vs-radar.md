# cub-scout vs Radar (Skyhook) for Kubernetes + GitOps Visibility

> Status: Living document. Last updated: 2026-11-08 (cub-scout v2.5.0; Radar / `skyhook-io/radar` facts as of 2026-11).

## Overview

[Radar](https://radarhq.io/) (open-source, `skyhook-io/radar`, Apache-2.0, Go,
one binary) is a general-purpose Kubernetes UI — a Lens / k9s / Headlamp / Freelens-class
console. It ships a live web UI, a desktop app, and an MCP server, runs from your
laptop with a kubeconfig or in-cluster via Helm, and — by default — **can act on the
cluster**: restart, scale, apply, drain, Helm lifecycle, GitOps sync, plus
AI-driven investigations that apply fixes on approval.

cub-scout is a cluster-wide, **read-only** observer that works across all GitOps
controllers and answers questions a live dashboard structurally does not: who owns
a resource, where each field's value came from, whether live state still matches the
source of truth, and whether you can *prove* it.

The two are complementary, not substitutes. Radar is the broad operator console —
and the thing you reach for to *act*. cub-scout is the read-only provenance and
evidence layer — the first stop to *diagnose* what diverged, *who* caused it, and
to produce a verifiable artifact, without touching the cluster. They overlap only
at the edges: cluster/GitOps visibility and a read-only MCP surface for agents.

## The one-line difference

Radar answers **"what's broken in my cluster right now — and do you want me to fix it?"**
cub-scout answers **"who owns this, where did each field's value come from, does live
still match the source of truth — and can I prove it, without touching anything?"**

## Current comparison

| Capability | Radar (Skyhook) | cub-scout |
|-----------|-----------------|-----------|
| Category | General-purpose K8s UI / operator console | Read-only provenance + evidence observer |
| Primary interface | Web UI (live via SSE), desktop app, kubectl plugin | CLI + TUI + JSON (no web UI) |
| Cluster writes | **Yes, by default** — restart, scale, set-image, apply, node drain; Helm upgrade/rollback/uninstall; GitOps sync/suspend/resume/reconcile/rollback | **None** — read-only by construction (`Get`/`List`/`Watch` only) |
| Read-only mode | Opt-in: `/mcp-readonly`, `--mcp-read-only`, `--disable-exec`, `--disable-helm-write` (UI writes still possible) | The only mode; enforced in CI + static grep + runtime filter |
| AI | AI investigations via your agent CLI (Claude Code / Codex / Cursor / OpenCode); applies fixes on approval | None — deterministic, no AI/ML in ownership logic |
| MCP | On by default; `/mcp` (full, incl. write tools) + `/mcp-readonly` | `mcp serve` — read-only-by-construction tool catalog |
| Per-field change cause | Not modeled | `manual-edit` vs `controller-drift` vs `unknown`, from `managedFields` |
| Provenance (where a value came from) | App grouping via Helm / Argo / Flux / label / name evidence | per-field `gitSource` (repo + `file:line`) and ConfigHub `bindingSource` |
| Source-of-truth verdict | GitOps sync state + field drift vs the GitOps source | `compare source-truth` → PASS / WATCH / BLOCK / ASK across 9 strategies |
| Intended vs rendered vs running | Resource compare + GitOps source-vs-live diff | `compare three-way` (DRY/WET/LIVE, connected) + `compare drift --file` (standalone) |
| Tamper-evident evidence | None — live dashboard (Radar Cloud retains ~30 days of history) | `receipt verify` → fingerprinted in-toto Statement v1; CI-gateable + chainable |
| Cluster audit / checks | 44 best-practice checks | `scan` — 46 patterns (+ confighub-scan 3,513 catalog) + `doctor` summary |
| Topology | Live resource graph with real-time traffic (Hubble / Istio / Caretta / Beyla) | `graph export` / `trace` ownership lineage (no live traffic) |
| Metrics / cost | Prometheus metrics, OpenCost / Kubecost, Karpenter capacity | Out of scope |
| Controllers covered | Argo CD, Flux, Helm (+ Argo Rollouts, Flagger, Argo Workflows) | Argo, Flux, Helm, Terraform, Crossplane, kro, ConfigHub, native |
| Pod logs / exec / terminals | Yes (`exec_pod`, pod terminals) | Out of scope (read-only metadata) |
| Multi-cluster / fleet | Radar Cloud (free ≤ 3 clusters; alerts, fleet views, 30-day history) | Connected mode via ConfigHub (`fleet outliers`, `history`) |
| Works with no account | Yes — OSS binary, kubeconfig only | Yes — fully offline, kubectl context only; ConfigHub optional |
| Distribution | `curl \| sh`, brew, krew, PowerShell, desktop app, Helm in-cluster | brew, krew, `go install`, docker, `cub` plugin, kubectl plugin |
| License / stack | Apache-2.0, Go, single binary (~4.1k★) | Open source, Go, single binary |
| Access requirements | kubeconfig (OSS); RBAC + OIDC/proxy for shared in-cluster | kubectl context only |

## Where cub-scout wins today

These are differentiators that are **structurally out of reach for a live
dashboard**, not just things cub-scout also happens to do:

1. **Who changed a field — human vs controller.** Radar shows live state and GitOps
   sync status; it does not attribute a divergent field to a *writer*. cub-scout reads
   `managedFields` and classifies each field as `manual-edit` (a `kubectl-*` writer),
   `controller-drift` (the controller itself), or `unknown`. That turns a noisy
   "out of sync" into "a human patched prod outside Git" vs "ignore it, the controller
   is mid-reconcile." (`pkg/agent/field_ownership.go`)

2. **Intended vs rendered vs running.** Radar can diff a GitOps *source* against live.
   It has no concept of a governed *intended* state that exists before rendering.
   `compare three-way` compares DRY (intended) / WET (rendered) / LIVE (running), so it
   catches render-time faults and drift that a source-vs-live diff cannot separate.

3. **A verdict stricter than "in sync."** "Synced" means "the target revision was
   applied" — it says nothing about a human editing on top afterward.
   `compare source-truth` returns PASS / WATCH / BLOCK / ASK across 9 delivery
   strategies (`git-argo`, `helm-flux`, `oci-flux`, …), accounting for manual edits,
   missing evidence, and strategy mismatch. (`pkg/agent/source_truth.go`)

4. **"Has anyone touched this since the freeze?"** The `no-manual-edits-since` receipt
   predicate reads `managedFields` timestamps to assert that no interactive writer
   touched a resource after a cutoff — a question a live view has no way to answer.

5. **Tamper-evident, portable evidence.** A dashboard view is true-right-now and then
   gone (Radar Cloud retains ~30 days of history, but as data, not a verifiable claim).
   `receipt verify` wraps the verdict + evidence into a fingerprinted in-toto Statement
   you hand to a CI gate, an auditor, or an AI agent a year later — and they recompute
   the SHA-256 (over RFC 8785 canonical JSON) to confirm nothing was edited. Receipts
   chain, so each link re-verifies the one before it.

6. **Read-only by guarantee, not by configuration.** Radar *can* be constrained
   (`/mcp-readonly`, `--mcp-read-only`, `--disable-exec`, `--disable-helm-write`), but
   its default surface — and its UI — write to the cluster. cub-scout *cannot* write at
   all: observation uses only `Get`/`List`/`Watch`, and even `suggest-remedy` only
   describes a fix. The guarantee is enforced three ways (a CI gate, a static grep over
   receipt sources, and a runtime filter that strips mutating next-steps).

7. **Cross-controller provenance in one model.** Terraform, Crossplane, kro, and
   ConfigHub are covered alongside Argo / Flux / Helm, with a single ownership model and
   a verified manager-string enumeration — so "is this Argo or Helm, and what owns the
   CRD under it?" is answered immediately in mixed-controller shops.
   (`pkg/agent/manager_strings.go`)

8. **Deterministic, citable automation.** `--format json` on every command plus a
   read-only MCP catalog whose answers carry attribution — so an agent gets
   provenance-attributed, receipt-backed reads it can *cite*, not just correlated reads.

## Where Radar still wins

cub-scout is read-only and narrow by design. For these, Radar (or another console) is
the right tool:

1. **A real web UI + desktop app** — a shared screen a whole team can look at. cub-scout
   is CLI / TUI / JSON only.
2. **Writes and remediation** — restart, scale, apply, drain, Helm lifecycle, and GitOps
   sync/suspend/resume/rollback from the UI or MCP. cub-scout will not act.
3. **AI investigations that apply fixes** — LLM root-cause plus apply-on-approval.
   cub-scout *describes* a remedy (`suggest-remedy`) but never runs it.
4. **Breadth of live observability** — real-time traffic topology (Hubble / Istio /
   Caretta / Beyla), Prometheus metrics, cost (OpenCost / Kubecost), Karpenter capacity,
   TLS-cert expiry, network-path diagnosis, image-filesystem inspection, RBAC views.
   All out of scope for cub-scout.
5. **Pod logs, exec, and terminals** — cub-scout reads metadata only.
6. **Maturity and adoption** — a polished, widely adopted project (~4.1k★), a desktop
   app, and a hosted Cloud with alerts, fleet views, and shared history.

## Gaps still open

| Gap | Why it matters | Status |
|-----|----------------|--------|
| Standalone three-way (git as DRY) | `compare three-way` still needs ConfigHub; Radar delivers source-vs-live drift with no account | Stage B back-resolution (#440) lays the groundwork |
| No shared / web view | Teams want one screen to look at together — a real draw of Radar's UI | Out of scope today (CLI / TUI / JSON) |
| Live metrics / traffic / cost | Breadth Radar has and cub-scout intentionally lacks | Out of scope — cub-scout is not an observability console |
| Receipt signing | Receipts are tamper-evident (SHA-256 / JCS) but not yet cryptographically signed | in-toto base makes DSSE / cosign a natural next step |

## When to use which

cub-scout is the read-only first stop for diagnosis, ownership, drift cause, and proof:

```
cub-scout doctor                         # cluster-wide health: what's broken?
cub-scout explain deploy/app -n ns       # why? (events, conditions, owner, sync state)
cub-scout trace   deploy/app -n ns       # where from? (Git source, owner chain, secrets)
cub-scout compare source-truth ... \     # does live still match the source of truth?
  --strategy git-argo
cub-scout receipt verify deploy/app \    # prove it — portable, tamper-evident
  -n ns --strategy git-argo
```

Reach for Radar when you want a **web console a team can share**, to **take an action**
(restart, scale, Helm, GitOps sync/rollback), **live metrics / traffic / cost**, or
**pod logs / exec**. Everything about *who owns this, where each field came from,
whether live matches the source of truth, and verifiable evidence of it* is answerable
from cub-scout — read-only, offline-capable, and across every controller.

The two compose: both read from the same kubeconfig and both expose a read-only MCP
surface, so an agent can take Radar's correlated reads and cub-scout's
provenance-attributed, receipt-backed reads of the same cluster side by side.
