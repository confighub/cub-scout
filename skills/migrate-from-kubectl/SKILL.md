---
name: migrate-from-kubectl
description: 'Use when the user is MIGRATING from ad-hoc `kubectl apply` toward GitOps (Argo / Flux / ConfigHub) — and needs to find the manual edits, decide what to do with each (revert / port to git / accept as canonical), and plan the transition. Natural phrasing: "find all the manual edits in this cluster", "we have years of kubectl-apply, help us move to GitOps", "which resources have been hand-edited?", "audit drift caused by humans not controllers", "port my cluster state to git", "show me everything kubectl-* has touched". Composes attribution layer (`cause: manual-edit`) + `map orphans` (resources without a controller owner) + `scan` (risk classification) + `snapshot` / `import --git-path` preview once a repo skeleton exists. Read-only throughout — the migration itself is user-driven. Do NOT load for: real-time triage (use triage-unhealthy-workload), single-resource drift question (use investigate-drift), a fleet conformance audit (use audit-fleet-conformance), or any mutating action.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout map orphans *) Bash(cub-scout map orphans *) Bash(cub scout map orphans *) Bash(./cub-scout scan *) Bash(cub-scout scan *) Bash(cub scout scan *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout import --git-path *) Bash(cub-scout import --git-path *) Bash(cub scout import --git-path *) Bash(./cub-scout snapshot *) Bash(cub-scout snapshot *) Bash(cub scout snapshot *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *)
---

# migrate-from-kubectl

The legacy-cluster migration loop. A cluster has accumulated `kubectl apply -f` over years; the team wants to adopt GitOps (Argo / Flux / ConfigHub). Step 1 of that migration is **finding what's been hand-touched** and deciding per-resource: revert, port to git, or accept as canonical.

This skill composes the attribution layer (`cause: manual-edit`), `map orphans`, `scan`, and `snapshot` / `import --git-path` preview once a repo skeleton exists. Read-only throughout — the actual migration is user-driven.

## When to use

Explicit phrasings:

- "Find all the manual edits in this cluster"
- "We have years of kubectl-apply, help us move to GitOps"
- "Which resources have been hand-edited?"
- "Audit drift caused by humans (not controllers)"
- "Port my cluster state to git" / "scaffold ConfigHub for our legacy cluster"
- "Show me everything `kubectl-*` has touched"
- "Triage the migration backlog — which resources to handle first?"

Implicit intents:

- The user is **before** GitOps adoption (no Argo/Flux/ConfigHub for most resources yet)
- The user wants a **per-resource plan** (revert / port / accept), not a single fix
- The user is gating on **risk** — what's safe to migrate first, what's risky
- The user may not know which resources even need migration ("show me what's manual")

## Do not load for

- Real-time triage of a broken workload — [`triage-unhealthy-workload`](../triage-unhealthy-workload/SKILL.md)
- Single-resource drift question (`why is THIS one different?`) — [`investigate-drift`](../investigate-drift/SKILL.md)
- Fleet conformance audit **once GitOps is in place** — [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md)
- Preview an import proposal from an existing repo — [`prepare-for-confighub`](../prepare-for-confighub/SKILL.md) (closely related; that skill assumes the repo exists; this one assumes it doesn't yet)
- Any mutating action. cub-scout never mutates. Every revert / port / accept is user-driven.

## The loop

1. **Inventory.** What workloads exist? Who (if anyone) owns each? Which are GitOps-managed already, which are `OwnerType=unknown`?
2. **Classify.** For each non-GitOps resource, read `managedFields` for the **last writer**. If a `kubectl-*` writer wrote it, this is a `manual-edit` candidate.
3. **Risk-rank.** `scan` produces severity-ranked findings. Use it to prioritize: high-severity / production-namespace resources first.
4. **Per-resource decision.** For each `manual-edit` resource, decide: revert (let a future controller reconcile), port to git (commit current state + plan controller adoption), or accept as canonical (and *then* port).
5. **Scaffold the git side.** Once decisions are made, snapshot the cluster (`snapshot` → GSF JSON). If you have already created a git layout, use `import --git-path --dry-run --json` to preview what ConfigHub would infer from it.
6. **Persist baseline receipts.** `receipt verify <kind>/<name> --since <migration-start-time>` per resource — captures the **pre-migration baseline** as fingerprinted evidence. Useful when the migration is contested ("did we really revert that?").

## Step-by-step

### Step 1 — inventory

```bash
$ cub-scout map --format json | jq 'group_by(.ownership.type) | map({type: .[0].ownership.type, count: length})'
[
  {"type": "argo",     "count": 12},
  {"type": "flux",     "count": 5},
  {"type": "helm",     "count": 8},
  {"type": "k8s",      "count": 47},       # native chains (ReplicaSets etc.) — fine
  {"type": "unknown",  "count": 23}        # ← the migration backlog
]
```

The 23 `OwnerType=unknown` resources are the candidates. Some are ad-hoc applies that should adopt GitOps; some might be orphans whose original owner is gone.

### Step 2 — classify by writer

For each unknown-owned resource, read the last writer:

```bash
$ for r in $(cub-scout map --format json | jq -r '.[] | select(.ownership.type == "unknown") | "\(.kind)/\(.name)|\(.namespace)"'); do
    kind=$(echo "$r" | cut -d/ -f1); rest=${r#*/}; name=${rest%|*}; ns=${rest#*|}
    last=$(kubectl get "$kind" "$name" -n "$ns" -o jsonpath='{.metadata.managedFields[-1:].manager}')
    echo "$kind/$name in $ns  ← $last"
  done
```

Group by writer:

| Writer | Population | Plan |
|--------|-----------|------|
| `kubectl-edit` | High-touch operator resources | High-risk migration; investigate per-resource |
| `kubectl-apply` / `kubectl-client-side-apply` | Bulk applies | Likely the legacy `kubectl apply -f *.yaml` pattern; group by similar paths |
| `kubectl-patch` / `kubectl-create` / `kubectl-replace` | Ad-hoc surgery | Audit; many of these are emergency fixes that should be in git |
| `kubectl-last-applied` | CSA migration artifacts | Usually safe to ignore; the resource has the `last-applied-configuration` annotation |
| Anything else (custom Helm wrappers, in-house controllers) | Unknown | File an issue with the writer string; cub-scout's verified enumeration may not include it yet |

`cub-scout explain <kind>/<name> -n <ns>` gives the same writer info in human form per resource.

### Step 3 — risk-rank

```bash
$ cub-scout scan --format json | jq '.findings | sort_by(.severity) | reverse | .[:10]'
```

`scan` returns risk findings: severity (error / warning / info), the rule that triggered, and the resource. Use it to prioritize the migration backlog:

- `severity=error` + production namespace → migrate FIRST
- `severity=warning` + multi-replica deployment → next
- `severity=info` or single-pod jobs → defer

`scan` complements the writer-classification: a `kubectl-apply` to a production deployment is much higher priority than the same writer on a one-off CronJob.

### Step 4 — per-resource decision

For each high-priority resource:

| Decision | When to pick it | Migration step (user-driven) |
|----------|----------------|------------------------------|
| **Revert** | The manual edit was wrong (emergency hack, mistaken) | Find the original spec (git history? snapshot?). `kubectl rollout undo` or `kubectl apply -f original.yaml`. Eventually adopt GitOps. |
| **Port to git** | The manual edit is correct AND should be reproducible | `kubectl get <kind>/<name> -o yaml > apps/.../resource.yaml`; commit; eventually adopt Argo/Flux/ConfigHub for this directory. |
| **Accept as canonical** | The manual edit IS the source of truth; nothing in git matches | Snapshot the resource (`kubectl get` or `cub-scout snapshot`); commit the snapshot; treat the file as authoritative going forward. |

cub-scout doesn't pick. The user does — but cub-scout's evidence makes the decision well-informed (you know exactly which fields were edited, which writer touched them, and whether a controller would have set them differently).

### Step 5 — scaffold the git side

For workloads adopting Argo / Flux / ConfigHub:

```bash
# Snapshot the current state as a starting point
$ cub-scout snapshot -n payments --format gsf > snapshot-payments.json

# Or, if you already have a repo skeleton, preview what ConfigHub would propose
$ cub-scout import --git-path ./proposed-repo --dry-run --json > proposed-units.json
```

See [`prepare-for-confighub`](../prepare-for-confighub/SKILL.md) for the import-preview flow once you have the repo skeleton.

### Step 6 — persist baseline receipts

```bash
$ MIGRATION_START="2026-05-22T00:00:00Z"
$ for r in $(cub-scout map --format json | jq -r '.[] | select(.ownership.type == "unknown") | "\(.kind)/\(.name) -n \(.namespace)"'); do
    cub-scout receipt verify $r --since $MIGRATION_START --save
  done
$ cub-scout receipt list --format json > migration-baseline.json
```

Each receipt captures "no `kubectl-*` writer touched this resource after `$MIGRATION_START`" — a verifiable baseline. If a stakeholder later disputes the migration (`we agreed not to touch X`), the receipts are the evidence.

See [`scout-verify`](../scout-verify/SKILL.md) for the receipt surface.

## Worked example

A team has a small `payments` namespace with 8 deployments, no GitOps. They want to adopt Argo CD.

```bash
$ cub-scout map -n payments --format json | jq '[.[] | {kind, name, owner: .ownership.type}]'
[
  {"kind": "Deployment", "name": "api",        "owner": "unknown"},
  {"kind": "Deployment", "name": "worker",     "owner": "unknown"},
  {"kind": "Deployment", "name": "scheduler",  "owner": "unknown"},
  ... (8 total) ...
]

$ for d in api worker scheduler; do
    cub-scout explain deploy/$d -n payments --presentation ai | grep -E "(Owner|Last writer|Mutation cause)"
  done
Owner:    Unknown (no GitOps labels, no ownerReferences)
Last writer: kubectl (manager: kubectl-client-side-apply)
Mutation cause: manual-edit

Owner:    Unknown (...)
Last writer: kubectl-edit
Mutation cause: manual-edit

Owner:    Unknown (...)
Last writer: kubectl-patch
Mutation cause: manual-edit

$ cub-scout scan -n payments --format json | jq '.findings[] | select(.severity == "error")'
{"resource": "Deployment/api", "rule": "no-resource-requests", "severity": "error"}

# Migration plan emerges:
# - api: revert the manual patches (probably emergency tuning), then commit a clean version to git
# - worker: port to git as-is (the manual edits look intentional)
# - scheduler: accept as canonical, document the policy exception

# Scaffold the proposed repo:
$ mkdir -p apps/prod/payments
$ for d in api worker scheduler; do
    kubectl get deploy/$d -n payments -o yaml > apps/prod/payments/$d.yaml
  done

# Capture baseline receipts:
$ MIGRATION_START="2026-05-22T00:00:00Z"
$ for d in api worker scheduler; do
    cub-scout receipt verify deploy/$d -n payments --since $MIGRATION_START --save
  done
```

The user now has: a snapshot of current state in `./apps/prod/payments/`, a per-resource migration decision, and 3 baseline receipts confirming the pre-migration state. The actual revert / commit / Argo Application creation is user-driven from here.

## Tool boundary

- **Allowed:** map / map orphans / scan / explain / trace / compare drift / compare three-way (for resources that DO have a controller); snapshot; import --git-path (preview-only — apply is out of band); receipt verify (specifically `no-manual-edits-since` for baselines); kubectl get / describe / managedFields reads.
- **Not allowed:** `kubectl apply/edit/patch/delete` (the actual port / revert is user-driven); `cub-scout import apply`; `argocd app create` (creating the Argo Application is user-driven); `cub` mutations. The migration *itself* is mutation; this skill stops at evidence + scaffolding.

## References

- [`scout-observe`](../scout-observe/SKILL.md) — map / scan / snapshot
- [`scout-attribute`](../scout-attribute/SKILL.md) — `cause` field reading
- [`scout-verify`](../scout-verify/SKILL.md) — baseline receipts
- [`prepare-for-confighub`](../prepare-for-confighub/SKILL.md) — once you have the repo skeleton
- [`observe-native`](../observe-native/SKILL.md) — the `OwnerType=unknown` population
- [`references/verified-manager-strings.md`](../references/verified-manager-strings.md) — writer enumeration
- `map orphans` is the verb for stale-label orphans (different from `OwnerType=unknown`); see scout-observe

## Constraints

- The actual port / revert / accept-as-canonical is **always user-driven**. cub-scout produces the evidence; the operator runs the mutation.
- `cause=unknown` (no `kubectl-*` writer, but no controller signal either) is a separate category from `manual-edit`. The migration plan for `unknown` resources is "investigate why no managedFields signal" — typically an admission webhook stripped them.
- Baseline receipts are most useful when captured **before** the migration starts. Capture them as Step 6, not after the fact.
- For Helm / Kustomize / templated sources, snapshot + `import --git-path` produces low-confidence proposals; the rendering reconstruction is out of scope. Plan for additional manual review of the proposal in those cases.
