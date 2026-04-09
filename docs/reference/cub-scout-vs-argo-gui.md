# cub-scout vs Argo CD GUI for GitOps Troubleshooting

> Status: Living document. Last updated: 2026-04-09.

## Overview

Argo CD ships a purpose-built GUI for managing ArgoCD Applications.
cub-scout is a cluster-wide observer that works across all GitOps controllers.

Today they are complementary. The goal is for cub-scout to become the
better first stop for GitOps troubleshooting, even for Argo-managed resources.

## Current comparison

| Capability | Argo CD GUI | cub-scout today |
|-----------|------------|-----------------|
| Scope | ArgoCD resources only | All controllers: Argo, Flux, Helm, Terraform, Crossplane, native |
| Sync status | Detailed: phase, message, retry count | Basic reconciliation state via `gitops status` |
| Resource tree | Visual tree per Application with health bubbles | `trace` and `graph export` show ownership lineage |
| Live vs desired diff | Side-by-side manifest diff | `compare three-way` (connected, Git/CH/cluster) |
| Health checks | Argo custom health assessments | `scan` (46 config patterns) + `doctor` (cluster summary) |
| K8s events | Inline per resource | Recent events in `explain` and `trace` (v1.10+) |
| Pod logs | Inline | Out of scope (read-only metadata) |
| Sync history | Full history with rollback | Not available |
| App grouping | ApplicationSets, app-of-apps | Flat — no Argo Application grouping |
| Secret references | Shows sync errors from missing secrets | Planned (#328) |
| Offline | Requires Argo server | Fully offline, just kubectl context |
| Cross-controller | Blind to non-Argo resources | Sees everything |
| Automation | API + `argocd` CLI | `--format json` on every command |
| Access requirements | Argo RBAC + port-forward/ingress | kubectl context only |

## Where Argo CD GUI wins today

1. **Sync error detail** — exact error messages, retry history, parameter overrides
2. **Live vs desired diff** — purpose-built manifest comparison
3. **Pod-level debugging** — logs, events, exec
4. **Application topology** — visual resource tree per Application
5. **Rollback** — one-click revert to previous sync

## Where cub-scout wins today

1. **Cluster-wide visibility** — "what else is here?" across all controllers
2. **Ownership clarity** — "is this Argo or Helm?" answered immediately
3. **Offline/disconnected** — no server dependency
4. **Config scanning** — finds issues Argo doesn't look for
5. **Automation-first** — JSON output for every command

## Gaps to close

To beat the Argo GUI as the first stop for GitOps troubleshooting,
cub-scout needs to close these gaps:

### P0 — Must have

| Gap | Why | Issue |
|-----|-----|-------|
| Sync status detail | Operators need the actual error, not just "out of sync" | TBD |
| ~~K8s events per resource~~ | ~~Events are the first thing operators check after status~~ | ✅ v1.10 |
| Resource conditions | `status.conditions` are critical for diagnosis — currently stripped (#348) | #348 |
| Application-level grouping | Argo users think in Applications, not flat resource lists | TBD |

### P1 — Strong differentiators

| Gap | Why | Issue |
|-----|-----|-------|
| Desired vs live diff (standalone) | Should not require connected mode to see drift | TBD |
| Secret health (#328) | Missing secrets are a top Argo troubleshooting issue | #328 |
| Cross-controller correlation | "This Argo app depends on a Helm-managed CRD" — only cub-scout can show this | TBD |
| Guided troubleshooting flow | `cub-scout explain` should walk the operator from symptom to root cause | TBD |

### P2 — Nice to have

| Gap | Why |
|-----|-----|
| Sync history timeline | Useful but Argo GUI will always have deeper Argo-specific history |
| Rollback support | Out of scope for read-only tool; recommend `argocd` CLI instead |

## Target state

A typical troubleshooting flow should be:

```
cub-scout doctor                    # cluster-wide health: what's broken?
cub-scout explain deploy/app -n ns  # why is this broken? (events, conditions, sync state, owner)
cub-scout trace deploy/app -n ns    # where did this come from? (Git source, owner chain)
cub-scout scan -n ns                # any config issues?
```

The operator should only need to switch to Argo GUI for:
- Pod logs and exec (out of scope)
- Argo-specific rollback (write operation)
- Parameter override management (Argo-specific feature)

Everything else — status, events, conditions, ownership, drift, config issues —
should be answerable from cub-scout, faster and with broader context than the
Argo GUI provides.
