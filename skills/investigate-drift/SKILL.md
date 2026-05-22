---
name: investigate-drift
description: 'Use when the user wants to find out WHY live state diverges from desired state — and specifically whether the drift is the controller still reconciling vs. someone bypassing the GitOps loop. Natural phrasing: "why is this Deployment different from the git manifest?", "did someone kubectl-edit this?", "is Argo still syncing or did a human change this?", "controller-drift or manual-edit?", "find the field that diverged and the writer", "compare three-way + attribution for deploy/api". Composes `compare three-way` + `compare drift` + the attribution layer (per-field cause / managerHint / gitSource / bindingSource). Do NOT load for: a triage where the workload won''t start (use triage-unhealthy-workload), pure inventory (use scout-observe), generating a fingerprinted evidence artifact (use scout-verify and the no-manual-edits-since predicate), or any mutating fix (cub-scout never mutates).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *) Bash(cub * get) Bash(cub * list) Bash(cub unit get *) Bash(cub link list *) Bash(argocd app get *) Bash(flux get *)
---

# investigate-drift

The drift-detective loop. Live state doesn't match desired state — and the operator needs to know WHY. Composes `compare three-way` (DRY/WET/LIVE) + `compare drift` (file/bundle vs cluster) + the attribution layer (`cause` / `managerHint` / `gitSource` / `bindingSource` per field) into a structured "find the diff + identify the writer" investigation.

## When to use

Explicit phrasings:

- "Why is deploy/api different from the git manifest?"
- "Did someone `kubectl edit` this?"
- "Is Argo still syncing or did a human change this?"
- "controller-drift or manual-edit?"
- "Find the field that diverged and tell me who wrote it"
- "Compare three-way + attribution for deploy/api in prod"
- "The cluster says replicas=1, ConfigHub says 3 — what happened?"

Implicit intents:

- The user knows there's a divergence; they need the **per-field** detail
- The user wants the **cause** classified, not just the diff
- The user is planning a *response* (revert to controller / port edit to git / accept manual change)
- The user may need to attach the result to a release-gate decision

## Do not load for

- A pager-pressed triage where the workload is broken — [`triage-unhealthy-workload`](../triage-unhealthy-workload/SKILL.md)
- Pure inventory ("what's running") — [`scout-observe`](../scout-observe/SKILL.md)
- Generating a fingerprinted evidence receipt — [`scout-verify`](../scout-verify/SKILL.md), specifically the `no-manual-edits-since` predicate
- Audit-fleet-wide conformance ("which clusters are outliers?") — [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md)
- Any mutating fix — cub-scout never applies. Route to `kubectl rollout undo`, `cub` for ConfigHub updates, or a git commit.

## The loop

1. **Detect the diff** with `compare three-way` (connected) or `compare drift --file <yaml>` (standalone).
2. **Identify the writer per field** by reading the `attribution` block on each `compareFieldMismatch`:
   - `cause`: `controller-drift` / `manual-edit` / `unknown`
   - `managerHint`: the K8s `managedFields` manager string for this field
   - `gitSource`: where the desired value comes from (repo / revision / path / file:line if stage B back-resolution is wired)
   - `bindingSource` (connected): which ConfigHub Link supplied this field
3. **Resolve the writer** to a real human/CI action:
   - `argocd-controller` / `kustomize-controller` / `helm-controller` → controller is reconciling
   - `kubectl-edit` / `kubectl-patch` / `kubectl-apply` (no GitOps owner) → human writer
   - `kubectl-client-side-apply` + Argo owner → Argo CSA migration (controller, not human)
4. **Decide the response** based on cause + the operator's policy. cub-scout doesn't pick; the user does.

## Step-by-step

### Step 1 — detect the diff

**Connected mode (preferred):** DRY (ConfigHub intent) vs WET (rendered) vs LIVE (cluster).

```bash
$ cub-scout compare three-way deploy/api -n prod --format json
{
  "agreement": "diverged",
  "summary": { "agreed": 5, "diverged": 2 },
  "fieldMismatches": [
    {
      "path": ".spec.replicas",
      "dry": 3, "wet": 3, "live": 1,
      "attribution": {
        "cause": "manual-edit",
        "managerHint": "kubectl-edit",
        "gitSource": { "repoUrl": "https://github.com/org/platform-config", "revision": "abc123", "path": "apps/prod/api" }
      }
    }
  ]
}
```

**Standalone mode:** `compare drift --file desired.yaml -n prod` for a file-vs-live diff. Or `compare three-way --source-path <local-git-checkout>` for stage B back-resolution (file:line per field).

### Step 2 — read the attribution

Each `compareFieldMismatch.attribution` block carries the per-field provenance. The fields:

| Field | Meaning |
|-------|---------|
| `cause` | One of `controller-drift` / `manual-edit` / `unknown` (verified manager-string enumeration in `pkg/agent/manager_strings.go`) |
| `managerHint` | The specific writer name that produced this classification |
| `gitSource.{repoUrl, revision, path}` | Resource-level git anchor from the Argo / Flux tracer (A2) |
| `gitSource.{file, line}` | Stage B back-resolution (`--source-path <local-checkout>`) — the exact YAML file:line where the field was set |
| `bindingSource` (connected) | `{unit, path, link}` — which ConfigHub Link supplies this field's value (C2) |

The `cause` is the headline. `managerHint` is the verbatim writer string. The git/binding sources are the breadcrumb back to source-of-truth.

### Step 3 — resolve the writer

Read the manager-string enumeration in [`references/verified-manager-strings.md`](../references/verified-manager-strings.md). Common writers and what they mean:

| Writer | Owner context | Meaning |
|--------|--------------|---------|
| `argocd-controller` | Argo-owned | Argo reconciled the field. `controller-drift`. |
| `kubectl-client-side-apply` | Argo-owned | Argo's CSA migration default. `controller-drift`. |
| `kubectl-client-side-apply` | Native (no Argo signal) | Human ran `kubectl apply`. `manual-edit`. |
| `kustomize-controller` / `helm-controller` | Flux-owned | Flux reconciled the field. `controller-drift`. |
| `kubectl-edit` / `kubectl-patch` | Any | Human edit. `manual-edit`. |
| `apiextensions.crossplane.io/composed-<hash>` | Crossplane-owned | Crossplane composed-resource writer. `controller-drift`. |
| `helm` (bare) | Helm-direct | Direct `helm install/upgrade`. `controller-drift` for OwnerHelm. |
| Unrecognized | Any | `cause=unknown`. cub-scout parses; it does not guess. |

The owner co-signal is essential for the `kubectl-client-side-apply` disambiguation. See [`observe-argocd`](../observe-argocd/SKILL.md) for the full rule.

### Step 4 — decide the response

cub-scout produces the evidence; the operator picks the action.

| Cause | Common operator response (mutations are user-driven) |
|-------|----------------------------------------------------|
| `controller-drift` + recent sync | Wait. The controller is reconciling; the diff is transient. |
| `controller-drift` + persistent | Investigate the controller (Argo `OutOfSync`, Flux `False`, kstatus failure). The reconciler is broken. |
| `manual-edit` + change should be kept | Port the edit back to git → controller reconciles → drift resolves. |
| `manual-edit` + change should be reverted | `kubectl rollout undo` OR wait for controller to revert. The original git state wins. |
| `manual-edit` + change should be accepted as canonical | Commit the live state to git → drift resolves; document the policy exception. |
| `unknown` | Cannot classify. Likely `managedFields` was stripped (admission webhook?) or the writer isn't in the verified enumeration. File an issue with the manager string. |

## Worked example

Production `deploy/api` has 1 replica; ConfigHub says 3. Argo CD is in `Synced` state. The operator's question: "did Argo do this, or did someone?"

```bash
$ cub-scout compare three-way deploy/api -n prod
Three-way compare for Deployment/api in prod

Agreement: DIVERGED — 2 of 7 fields disagree

Field mismatches:
  .spec.replicas
    DRY (ConfigHub unit payments-api rev=42):    3
    WET (rendered):                              3
    LIVE (cluster):                              1
    Cause:        manual-edit (manager: kubectl-edit)
    Git source:   https://github.com/org/platform-config @abc123 path=apps/prod/api
  .spec.template.spec.containers[0].resources.requests.cpu
    DRY:  500m
    WET:  500m
    LIVE: 250m
    Cause:        manual-edit (manager: kubectl-patch)
    Git source:   (same)
```

**Reading:** Two fields, both `manual-edit`. The writers are `kubectl-edit` (replicas) and `kubectl-patch` (cpu request). Someone ran two separate manual commands. Argo CD's `Synced` state is misleading because Argo's last sync wasn't the most recent write — the `managedFields` co-signal proves it.

**Response options:**

- If the edit was emergency cost-control (CPU + replicas down for a spike), revert: `kubectl rollout undo deploy/api -n prod` and let Argo re-apply the manifest version.
- If the edit was correct (replicas 1 IS what prod needs), commit the change back to `apps/prod/api/deployment.yaml`, push, and Argo will reconcile.
- Either way, somebody bypassed GitOps. Surface this in the team's drift dashboard (or via `cub-scout receipt verify ... --predicate no-manual-edits-since`, which captures it as a fingerprinted artifact for the postmortem).

cub-scout produced the evidence in one command. The decision is the operator's.

## Connected-mode enrichment

With `cub auth login`, `compare three-way` adds:

- **DRY column** — the ConfigHub Unit's intent (the canonical "what we asked for")
- **`bindingSource` per field** — which ConfigHub Link supplies this value (e.g., `link:env-shared.replicas`). Per-field provenance into the unit graph.
- **`confighubUrl`** — deep-link to the unit's revision in the ConfigHub GUI
- **`incomingBindings[]`** on the unit — which upstream units feed this unit

Standalone mode loses the DRY column and bindingSource; what remains is the WET (file) vs LIVE (cluster) diff plus the per-field cause from `managedFields`. Still useful — just narrower.

## Tool boundary

- **Allowed:** the four Compare verbs; `explain`, `trace`; `kubectl get/describe/get --show-managed-fields`; `cub * get/list`, `cub unit get`, `cub link list` (connected); `argocd app get`, `flux get` (controller-side reads)
- **Not allowed:** `kubectl rollout undo`, `kubectl edit`, `kubectl patch`, `argocd app sync`, `flux reconcile`, `cub * update/create/delete`. The investigation produces evidence; the response is the operator's. cub-scout's `compare three-way --fail-on` and `--suggest` flags work for CI gating but do not write.

## References

- [`scout-compare`](../scout-compare/SKILL.md) — the verb group this skill composes
- [`scout-attribute`](../scout-attribute/SKILL.md) — the attribution-layer evidence surface
- [`references/kubernetes-managedfields.md`](../references/kubernetes-managedfields.md) — the data substrate
- [`references/verified-manager-strings.md`](../references/verified-manager-strings.md) — the writer enumeration
- [`observe-argocd`](../observe-argocd/SKILL.md), [`observe-flux`](../observe-flux/SKILL.md) — owner co-signals for disambiguation
- [`scout-verify`](../scout-verify/SKILL.md) — the `no-manual-edits-since` predicate captures the same evidence as a fingerprinted receipt
- Attribution layer issues: `#435` (parent), `#437` (A1+A1.5+A2), `#438` (C1), `#439` (C2), `#440` (stage B)
- Example: [`examples/drift/mutation-cause-attribution/`](../../examples/drift/mutation-cause-attribution/)

## Constraints

- This skill is **investigation**, not remediation. cub-scout categorically never applies a fix. Every revert / port / accept is the operator running another tool.
- `cause=unknown` is an honest answer when `managedFields` is missing or stripped. Don't pressure-classify it.
- Stage B file:line back-resolution requires `--source-path <local-git-checkout>` and only covers raw YAML (no Helm / Kustomize templating yet — see #435 follow-ons).
