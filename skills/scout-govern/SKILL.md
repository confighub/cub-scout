---
name: scout-govern
description: 'Use when the user wants to consume CONNECTED ConfigHub governance signals from cub-scout — the Govern verb group. Natural phrasing: "show me the ChangeSet history for this unit", "what is the blast radius if I change this unit?", "which clusters in my fleet are outliers?", "show me yesterday''s connected summary", "render this view across the fleet", "audit who changed this", "where did the last apply come from?", "list ConfigHub bundles I have", "show me the catalog of past snapshots". Load whenever intent is history / impact / blast-radius / fleet-outliers / summary / view / audit / bundle / catalog / connected-context. Do NOT load for: live cluster comparison (use scout-compare), receipts (use scout-verify), inventory of running pods (use scout-observe), or any mutation via cub. Most of this skill requires `cub auth login` — say so upfront.'
phase: verify
allowed-tools: Bash(./cub-scout history *) Bash(cub-scout history *) Bash(cub scout history *) Bash(./cub-scout impact *) Bash(cub-scout impact *) Bash(cub scout impact *) Bash(./cub-scout fleet *) Bash(cub-scout fleet *) Bash(cub scout fleet *) Bash(./cub-scout summary *) Bash(cub-scout summary *) Bash(cub scout summary *) Bash(./cub-scout views *) Bash(cub-scout views *) Bash(cub scout views *) Bash(./cub-scout audit *) Bash(cub-scout audit *) Bash(cub scout audit *) Bash(./cub-scout bundle *) Bash(cub-scout bundle *) Bash(cub scout bundle *) Bash(./cub-scout catalog *) Bash(cub-scout catalog *) Bash(cub scout catalog *) Bash(./cub-scout status) Bash(cub-scout status) Bash(cub scout status) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub unit get *) Bash(cub link list *) Bash(cub history *) Bash(cub auth status)
---

# scout-govern

The Govern verb group of cub-scout. Read-only consumption of connected ConfigHub governance signals — ChangeSet history, blast-radius impact analysis, fleet conformance, connected summaries, Views, audit trails, and bundle / catalog state.

Most of this skill requires **connected mode** (`cub auth login` or `CONFIGHUB_API_KEY`). Standalone-mode users get `bundle` / `catalog` / `audit` against local artifacts only.

## When to use

Explicit phrasings:

- "Show me the ChangeSet history for unit `payments-api`"
- "What's the blast radius if I change this unit?" / "what depends on this?"
- "Which clusters in my fleet are outliers?" / "show fleet conformance"
- "Show me yesterday's connected summary for cluster X"
- "Render Hub View `monthly-spend-by-team` across the fleet"
- "Audit who changed deploy/api recently"
- "Where did the last apply come from?"
- "List the ConfigHub bundles I have locally"
- "Show me the catalog of past snapshots"
- "What ConfigHub units / spaces does this resource belong to?"

Implicit intents:

- The user is investigating a change *retroactively* (history / audit / postmortem framing)
- The user is gating a change *prospectively* (impact / blast-radius / promotion-safety framing)
- The user is reasoning across the fleet (multi-cluster outliers, conformance)
- The user wants a structured connected-mode snapshot rather than ad-hoc reads

## Do not load for

- Live cluster comparison (DRY/WET/LIVE) — [`scout-compare`](../scout-compare/SKILL.md)
- Typed, fingerprinted evidence artifacts (receipts) — [`scout-verify`](../scout-verify/SKILL.md)
- Inventory of running pods / status — [`scout-observe`](../scout-observe/SKILL.md)
- Interpreting *why* something is broken — [`scout-diagnose`](../scout-diagnose/SKILL.md)
- ConfigHub mutations (create / update / delete units) — that's `cub`, not cub-scout. Route to [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills).

## Standalone vs connected

- **Connected (cluster + `cub auth login`):** `history`, `impact`, `fleet outliers`, `summary list/slack`, `views resolve`, `audit list` — all require ConfigHub auth.
- **Standalone (cluster only):** `audit list` against local audit logs; `bundle inspect/replay/diff/summarize/timeline` and `catalog list` against on-disk artifacts. No ConfigHub needed.
- **Offline (artifact only):** `bundle inspect`, `bundle replay`, `bundle diff`, `bundle timeline`, `catalog list` work against `.bundle.tar` / `.catalog.json` files with no cluster and no ConfigHub.

`cub-scout status` is the entry point that says which mode you're in.

## Tool boundary

- **Allowed (read-only):** all eight Govern verbs; `kubectl get/describe` for cross-reference; `cub * get / list`, `cub unit get`, `cub link list`, `cub history` (connected); `cub auth status` to confirm mode.
- **Not allowed:** `cub * create/update/delete`, `cub gitops import` (mutating), `kubectl apply/edit/patch/delete`, `argocd app sync`, `flux reconcile --with-source`. Governance verbs *read* the connected surface; they never write to it.
- **`fleet` and `summary`:** read across multiple clusters via ConfigHub's connected snapshots. They do NOT shell out to each cluster's kubeconfig directly — the connected surface is the source of truth.

## The verb menu

| Question | Verb | Notes |
|---|---|---|
| "Who changed this unit, when?" | `cub-scout history <kind>/<name> -n <ns>` | ChangeSet timeline for the unit tied to this resource. Requires connected. |
| "What depends on this unit?" | `cub-scout impact <unit-slug>` | Blast-radius preview: which downstream units / clusters / Apps would be affected. Requires connected. |
| "Which clusters are outliers?" | `cub-scout fleet outliers --view <view>` | Fleet-wide conformance against a View. Requires connected. |
| "Show me the connected summary" | `cub-scout summary list --cluster <name>` | List stored connected summaries. `summary slack` formats for Slack output. |
| "Render a Hub View" | `cub-scout views resolve <url-or-uuid>` | Resolve a ConfigHub Hub View URL to its structured columns. `--scope cluster` for fleet-wide. |
| "Audit who changed what" | `cub-scout audit list` | Audit log entries. Read-only access to ConfigHub's audit trail. |
| "Inspect a debug bundle" | `cub-scout bundle inspect <path>` | `bundle replay / diff / summarize / timeline` for richer per-bundle reads. |
| "List my bundle catalogs" | `cub-scout catalog list` | Multi-bundle catalog view — useful for time-series / per-environment archives. |

## The loop

1. **Check the mode.** Run `cub-scout status` to confirm whether you're connected. If not and the user needs connected data, say so explicitly — don't half-answer with stale local data.
2. **Pick the verb.** Most user prompts map cleanly to one of the eight verbs above.
3. **Invoke.** Use `--format json` for agent / CI consumption; the default ASCII output is human-friendly.
4. **Read the evidence.** Each verb has a structured shape (ChangeSet timeline, impact graph, fleet outlier list, View column matrix). They feed Pilot's acceptance kernel and similar tooling directly.
5. **Hand off.** If the user wants to *act* on a governance finding ("revert this change", "promote to staging", "apply the missing unit"), route to a `cub` skill. cub-scout never mutates ConfigHub.

## Worked examples

### A: ChangeSet history (connected)

```bash
$ cub-scout history deploy/api -n prod
ChangeSet timeline for unit payments-api:
  2026-05-22T10:30Z  Apply by ci-bot     rev=42  "promote v2.3.0"
  2026-05-21T14:00Z  Apply by alice      rev=41  "config tuning"
  2026-05-20T09:15Z  Create by alice     rev=40  "initial unit"

ConfigHub URL: https://hub.confighub.com/p/payments/u/payments-api
```

### B: blast-radius impact (connected)

```bash
$ cub-scout impact payments-api --format json
{
  "unit": "payments-api",
  "downstream": [
    {"kind": "App", "name": "checkout", "via": "link:env-shared"},
    {"kind": "Unit", "name": "payments-worker", "via": "link:image-shared"}
  ],
  "clusters": ["prod-use2", "prod-euw1", "staging-use2"]
}
```

### C: fleet outliers against a View (connected)

```bash
$ cub-scout fleet outliers --view monthly-spend-by-team
View: monthly-spend-by-team
Clusters: 12

  payments  EXPECTED 3 deploys / cluster   prod-euw1: 2 (outlier: missing payments-worker)
  checkout  EXPECTED 5 deploys / cluster   ALL match
  search    EXPECTED 4 deploys / cluster   staging-use2: 3 (outlier: missing search-replica)
```

### D: bundle inspect (offline, no cluster needed)

```bash
$ cub-scout bundle inspect ./prod-snapshot.bundle.tar
Bundle: prod-snapshot.bundle.tar
  capturedAt: 2026-05-22T10:30:00Z
  cluster: prod-use2
  resources: 247
  topNamespaces: payments(48), checkout(32), platform(20)
```

`bundle diff` / `bundle timeline` work the same way for multi-bundle comparisons.

## Output evidence

| Verb | Returns |
|---|---|
| `history` | ChangeSet timeline + ConfigHub deep-link |
| `impact` | Downstream unit / cluster / App graph |
| `fleet outliers` | Per-View matrix with outlier rows highlighted |
| `summary` | Stored connected summaries indexed by cluster / timestamp |
| `views resolve` | Structured columns from a Hub View URL |
| `audit` | Audit log entries (read-only ConfigHub audit trail) |
| `bundle inspect/diff/summarize/timeline` | Per-bundle structured reads |
| `catalog list` | Multi-bundle catalog metadata |

## Connected-mode trust surface

Where `history` / `impact` / `fleet outliers` return guidance about whether to act, those hints are **trust signals** for Pilot and similar acceptance kernels. They are NEVER action recommendations from cub-scout itself. Per the read-only triad (#410 / #428):

- cub-scout = evidence (this skill)
- ConfigHub = authority (where the state lives)
- Pilot = judge (decides accept / wait / block)

If a user is reading impact output and asks "should I promote?", route them to Pilot or to a human acceptance step — never let cub-scout produce an "approved" answer.

## References

- Command reference: [`docs/reference/commands.md`](../../docs/reference/commands.md) § history, § impact, § fleet, § summary, § views, § audit, § bundle, § catalog
- Bundle format: [`docs/bundle-diff.md`](../../docs/bundle-diff.md), [`docs/bundle-timeline.md`](../../docs/bundle-timeline.md)
- Catalog format: [`docs/catalog.md`](../../docs/catalog.md)
- Views: [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md), `cub-scout/#391`
- Read-only triad: `cub-scout/#410`, `cub-scout/#428`

## Constraints

- Connected verbs (history / impact / fleet / summary / views / audit) refuse to run without `cub auth login`. The error message points at auth, not at the verb.
- `bundle` / `catalog` work in standalone mode but cannot resolve ConfigHub-specific identifiers (unit slugs, space names) without connected context. They surface the unresolved references explicitly.
- This skill does not generate receipts — receipts are [`scout-verify`](../scout-verify/SKILL.md)'s job. Govern returns *governance evidence*; Verify returns *fingerprinted artifacts*.
