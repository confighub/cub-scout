# cub-scout vs Argo CD GUI for GitOps Troubleshooting

> Status: Living document. Last updated: 2026-06-19 (reflects v2.5.0).

## Overview

Argo CD ships a purpose-built GUI for managing ArgoCD Applications.
cub-scout is a cluster-wide, read-only observer that works across all GitOps
controllers — and, for Argo-managed resources specifically, answers questions
the Argo GUI structurally cannot.

The two are complementary. Argo's GUI is the authority for *acting* on Argo
Applications (sync, rollback, parameter overrides). cub-scout is the better
first stop for *diagnosing* what diverged, who caused it, and whether you can
prove the live state still matches the source of truth.

## The one-line difference

Argo answers **"did I apply what Git said?"**
cub-scout answers **"does the live state still match the source of truth, who
changed it if not, and can I prove it?"**

## Current comparison

| Capability | Argo CD GUI | cub-scout today |
|-----------|------------|-----------------|
| Scope | ArgoCD resources only | All controllers: Argo, Flux, Helm, Terraform, Crossplane, kro, native |
| Sync status | Detailed: phase, message, retry count | Reconciliation state + `operationState` message via `trace` / `gitops status` |
| Per-field change cause | Not modeled — OutOfSync is cause-blind | `manual-edit` vs `controller-drift` vs `unknown`, from `managedFields` |
| Source-of-truth verdict | "Synced" = applied target revision | `compare source-truth` → PASS / WATCH / BLOCK / ASK across 9 strategies |
| Tamper-evident evidence | Sync history lives inside Argo | `receipt verify` → fingerprinted in-toto Statement v1, portable |
| Resource tree | Visual tree per Application with health bubbles | `trace` / `graph export` ownership lineage; `map` ApplicationSet grouping |
| Live vs desired diff | Side-by-side manifest diff | `compare drift --file` (standalone) + `compare three-way` (connected) |
| Health checks | Argo custom health assessments | `scan` (46 config patterns) + `doctor` (cluster summary) |
| Resource conditions | Inline per resource | `status.conditions` surfaced in `trace` / failure details |
| K8s events | Inline per resource | Recent events in `explain` and `trace` |
| Secret references | Shows sync errors from missing secrets | `trace` secret evidence (refs + presence, never secret data) |
| Pod logs | Inline | Out of scope (read-only metadata) |
| Sync history | Full history with rollback | Not available |
| Works when Argo server is down | No — needs Argo API/repo-server | Yes — reads Applications via kubectl |
| Offline | Requires Argo server | Fully offline, just kubectl context |
| Cross-controller | Blind to non-Argo resources | Sees everything |
| Automation | API + `argocd` CLI | `--format json` on every command + MCP gateway |
| Access requirements | Argo RBAC + port-forward/ingress | kubectl context only |

## Where cub-scout wins today

These are the differentiators that are **structurally out of reach for Argo**,
not just things cub-scout also happens to do:

1. **Who changed a field — human vs controller.** Argo's diff shows you *that*
   a field differs from Git; it cannot tell you *who* moved it. cub-scout reads
   `managedFields` and attributes each divergent field to `manual-edit` (a
   `kubectl-*` writer), `controller-drift` (the controller itself), or
   `unknown`. This turns a noisy OutOfSync into "a human patched prod outside
   Git" vs "ignore it, Argo's mid-reconcile."
   (`pkg/agent/field_ownership.go`)

2. **Diagnose the Argo app when Argo itself is down.** cub-scout reads the
   `Application` object straight from the cluster via kubectl — target
   revision, `reconciledAt`, owner chain, source — so it still works when the
   Argo API server / repo-server / dex is unhealthy, which is exactly when you
   need to debug. No Argo server, no port-forward, no Argo RBAC.
   (`pkg/agent/argo_trace.go`, `traceApplicationViaKubectl`)

3. **A verdict stricter than "Synced."** Argo's Synced means "I applied the
   target revision" — it says nothing about a human editing on top afterward.
   `compare source-truth --strategy git-argo` (and `oci-argo` / `helm-argo` /
   `confighub-oci-argo`) returns PASS / WATCH / BLOCK / ASK accounting for
   manual edits, missing evidence, and strategy mismatch.
   (`pkg/agent/source_truth.go`)

4. **"Has anyone touched this since the freeze?"** The `no-manual-edits-since`
   receipt predicate reads `managedFields` timestamps to assert no interactive
   writer touched the resource after a cutoff — a question Argo has no concept
   of.

5. **Tamper-evident, portable evidence.** Argo's sync history is meaningless
   once you leave the Argo UI. `receipt verify` wraps the verdict + evidence
   into a fingerprinted in-toto Statement you hand to a CI gate, an auditor, or
   an AI agent a year later — and they recompute the fingerprint to confirm
   nothing was edited.

6. **Cluster-wide + cross-controller visibility.** "What else is here, and is
   this Argo or Helm?" answered immediately — including an Argo-managed
   Deployment depending on a CRD actually owned by Flux or a raw Helm release.
   (Only matters in mixed-controller shops.)

7. **Automation-first and offline.** Deterministic `--format json` on every
   command plus a read-only MCP gateway, with no server dependency.

## Where Argo CD GUI still wins

cub-scout is read-only and will not do these — hand back to the `argocd` CLI or
GUI:

1. **Full sync history + retry timeline** — cub-scout surfaces the current
   `operationState` message, not the deep per-sync history.
2. **Purpose-built visual manifest diff** — side-by-side rendered diff.
3. **Pod-level debugging** — logs, exec.
4. **Per-Application visual topology** — the health-bubble resource tree.
5. **Rollback / sync / parameter overrides** — write operations, out of scope
   for a read-only observer.

## Gaps closed since the last revision

The previous version of this doc (April 2026) listed these as gaps; they have
since shipped:

- ✅ **K8s events per resource** (v1.10) — `explain` / `trace`
- ✅ **Resource conditions** — `status.conditions` in `trace` / failure details
- ✅ **Secret health** — `trace` secret evidence (refs + presence)
- ✅ **Sync error message** — `operationState.Message` surfaced in `trace`
- ✅ **Application-level grouping** — ApplicationSet lookup in `map`
- ✅ **Per-field drift cause + source-truth verdict + receipts** — the
  attribution layer, `compare source-truth`, and the receipt surface did not
  exist in April and are now the headline differentiators above.

## Gaps still open

| Gap | Why it matters | Status |
|-----|----------------|--------|
| Standalone three-way (git as DRY) | `compare three-way` still needs ConfigHub; `compare drift --file` covers the simpler file-vs-live case | Stage B back-resolution (#440) lays the groundwork |
| Deep sync retry history | Argo will always own its own sync timeline | Out of scope — recommend Argo GUI |
| Visual topology / rollback / logs | Write ops + rich visuals are Argo-GUI territory | Out of scope (read-only) |

## Target troubleshooting flow

```
cub-scout doctor                       # cluster-wide health: what's broken?
cub-scout explain deploy/app -n ns     # why? (events, conditions, owner, sync state)
cub-scout trace deploy/app -n ns       # where from? (Git source, owner chain, secrets)
cub-scout compare source-truth ... \   # does live still match source of truth?
  --strategy git-argo
cub-scout receipt verify deploy/app \  # prove it — portable, tamper-evident
  -n ns --strategy git-argo
```

Switch to the Argo GUI only for: pod logs/exec, Argo-specific rollback, and
parameter-override management. Everything else — status, events, conditions,
ownership, *who* caused drift, source-of-truth conformance, and verifiable
evidence — is answerable from cub-scout, faster and with broader context than
the Argo GUI provides.
