---
name: investigate-drift
description: 'Use when the user wants to compare live and desired state and inspect available manager and source evidence. Natural phrasing: "why is this Deployment different from the git manifest?", "which recognized manager is recorded for this path?", "controller-drift or interactive-manager evidence?", "compare three-way + attribution for deploy/api". Composes `compare three-way` + `compare drift` + the attribution layer (`cause` / `managerHint` / `gitSource` / `bindingSource`). Do NOT promise who changed the value or which manager wrote last: path-level manager evidence is not time-ordered, and resource-level `explain` evidence is not field-specific. Do NOT load for: a triage where the workload won''t start (use triage-unhealthy-workload), pure inventory (use scout-observe), generating a fingerprinted evidence artifact (use scout-verify and the no-manual-edits-since predicate), or any mutating fix (cub-scout never mutates).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *) Bash(cub space list *) Bash(cub unit get *) Bash(cub unit list *) Bash(cub link get *) Bash(cub link list *) Bash(cub view get *) Bash(cub view list *) Bash(cub unit-event list *) Bash(cub resource list *) Bash(cub release list *) Bash(cub changeset list *) Bash(argocd app get *) Bash(flux get *)
---

# investigate-drift

The drift-investigation loop. Live state does not match desired state, so compare the values and inspect available provenance. Composes `compare three-way` (DRY/WET/LIVE) + `compare drift` (file/bundle vs cluster) + attribution (`cause` / `managerHint` / `gitSource` / `bindingSource`). Manager evidence classifies recognized managers; it does not establish write order or human identity.

## When to use

Explicit phrasings:

- "Why is deploy/api different from the git manifest?"
- "Is a recognized interactive manager recorded for this path?"
- "Which recognized controller or interactive manager is recorded?"
- "controller-drift or manual-edit?"
- "Find the field that diverged and show its available manager evidence"
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
2. **Inspect manager evidence** on each `compareFieldMismatch`. Path-specific classifications require decodable `FieldsV1` and a mapped canonical path; otherwise the result can be a resource-level fallback, which must not be attributed to that field:
   - `cause`: `controller-drift` / `manual-edit` / `unknown`
   - `managerHint`: a representative recognized K8s `managedFields` manager string for the path classification or fallback
   - `gitSource`: where the desired value comes from (repo / revision / path / file:line if stage B back-resolution is wired)
   - `bindingSource` (connected): which ConfigHub Link supplied this field
3. **Interpret the manager string conservatively** using [`references/verified-manager-strings.md`](../references/verified-manager-strings.md):
   - `argocd-controller` / `kustomize-controller` / `helm-controller` → recognized controller-manager evidence; this does not show reconciliation is happening now
   - `kubectl-edit` / `kubectl-patch` / `kubectl-apply` → recognized interactive-manager evidence; it does not identify the person or prove write order
   - `kubectl-client-side-apply` + Argo owner → Argo CSA migration co-signal; manager strings alone do not identify a human
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
| `managerHint` | A representative recognized manager string; not proof of the latest writer or a person |
| `gitSource.{repoUrl, revision, path}` | Resource-level git anchor from the Argo / Flux tracer (A2) |
| `gitSource.{file, line}` | Stage B back-resolution (`--source-path <local-checkout>`) — the exact YAML file:line where the field was set |
| `bindingSource` (connected) | `{unit, path, link}` — which ConfigHub Link supplies this field's value (C2) |

The `cause` classifies recognized manager evidence. `managerHint` is a manager string, not a person or timestamped writer identity. The git/binding sources point to desired-state provenance.

### Step 3 — interpret manager evidence

Read the manager-string enumeration in [`references/verified-manager-strings.md`](../references/verified-manager-strings.md). These are manager classifications, not proof of a current reconciliation or human action:

| Writer | Owner context | Meaning |
|--------|--------------|---------|
| `argocd-controller` | Argo-owned | Recognized Argo controller-manager evidence. `controller-drift`; recency is not established. |
| `kubectl-client-side-apply` | Argo-owned | Argo CSA migration co-signal. `controller-drift`; does not establish recency. |
| `kubectl-client-side-apply` | Native (no Argo signal) | Classified as interactive-manager evidence. `manual-edit`; the human identity is unknown. |
| `kustomize-controller` / `helm-controller` | Flux-owned | Recognized Flux controller-manager evidence. `controller-drift`; recency is not established. |
| `kubectl-edit` / `kubectl-patch` | Any | Recognized interactive-manager evidence. `manual-edit`; person and write order are unknown. |
| `apiextensions.crossplane.io/composed-<hash>` | Crossplane-owned | Recognized Crossplane composed-resource manager evidence. `controller-drift`; recency is not established. |
| `helm` (bare) | Helm-direct | Helm manager evidence. `controller-drift` for OwnerHelm; it does not establish a person or write order. |
| Unrecognized | Any | `cause=unknown`. cub-scout parses; it does not guess. |

The owner co-signal is essential for the `kubectl-client-side-apply` disambiguation. See [`observe-argocd`](../observe-argocd/SKILL.md) for the full rule.

### Step 4 — decide the response

cub-scout produces the evidence; the operator picks the action.

| Cause | Common operator response (mutations are user-driven) |
|-------|----------------------------------------------------|
| `controller-drift` + independently observed recent sync | Compare live values again and inspect controller health; manager evidence alone does not show it is reconciling or that the diff is transient. |
| `controller-drift` + persistent | Investigate controller status (Argo `OutOfSync`, Flux `False`, kstatus failure); do not infer a broken reconciler from manager evidence alone. |
| `manual-edit` + change should be kept | Verify the live value and desired source, then port an approved change back to git. |
| `manual-edit` + change should be reverted | Verify policy and live value, then have the operator restore desired state; do not assume which value is newest. |
| `manual-edit` + change should be accepted as canonical | Verify and review the live value, then update the source of truth through the operator's normal process. |
| `unknown` | Cannot classify. Missing, incomplete, unmapped, or unrecognized evidence can all yield unknown; investigate the available metadata without guessing. |

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

**Reading:** If these are path-specific classifications, each path has recognized interactive-manager evidence; controller-manager evidence may also be present. The strings do not prove separate commands, identify a person, or establish that Argo's last sync predates these values. Check the actual live and desired values and the controller status independently.

**Response options:**

- If the live values should return to desired state, have the operator restore them through the approved workflow and verify reconciliation.
- If the live values should become canonical, have the operator review and update the source of truth through the normal workflow.
- Manager evidence alone does not establish that somebody bypassed GitOps. For time-bounded history, use a suitable audit source or the receipt predicate with its documented timestamp evidence.

cub-scout produced the evidence in one command. The decision is the operator's.

## Connected-mode enrichment

With `cub auth login`, `compare three-way` adds:

- **DRY column** — the ConfigHub Unit's intent (the canonical "what we asked for")
- **`bindingSource` per field** — which ConfigHub Link supplies this value (e.g., `link:env-shared.replicas`). Per-field provenance into the unit graph.
- **`confighubUrl`** — deep-link to the unit's revision in the ConfigHub GUI
- **`incomingBindings[]`** on the unit — which upstream units feed this unit

Standalone mode loses the DRY column and bindingSource; what remains is the WET (file) vs LIVE (cluster) diff. A path-specific manager classification is available only when decodable `FieldsV1` maps the mismatch; otherwise output may be resource-level or unknown. It still does not establish write order or a person.

## Tool boundary

- **Allowed:** the four Compare verbs; `explain`, `trace`; `kubectl get/describe/get --show-managed-fields`; `cub <entity> get / list` for the entities named in allowed-tools (connected); `argocd app get`, `flux get` (controller-side reads)
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
- `cause=unknown` is honest when manager evidence is missing, incomplete, unmapped, or unrecognized. Don't pressure-classify it.
- `controller-drift` and `manual-edit` describe recognized manager presence in the resource or mapped path. The classifier does not use manager timestamps to order writers or prove a person acted; inspect audit logs for identity/history.
- Stage B file:line back-resolution requires `--source-path <local-git-checkout>` and only covers raw YAML (no Helm / Kustomize templating yet — see #435 follow-ons).
