---
name: pilot-patch-and-drift
description: 'Use when Pilot is CLASSIFYING DRIFT and DECIDING WHAT TO DO ABOUT IT after a divergence between intended state and live state is detected. Natural phrasings: "Pilot, classify this drift", "is this manual edit or controller drift?", "should Pilot revert this patch?", "quarantine or accept-as-canonical?", "what does cub-scout say about who touched this field?", "render a drift verdict for the audit". Pilot consumes cub-scout''s attribution evidence (`cause: manual-edit` / `controller-drift` / `unknown` + `managerHint` + `gitSource` + `bindingSource`) and decides revert / quarantine / accept-as-canonical / ASK-human. The decision shape matches the attribution layer''s evidence shape — cub-scout produces deterministic classification; Pilot picks the policy action. Do NOT load for: a single-resource pre-deploy gate (use pilot-cd-gate), a real-time event response (use pilot-watch-alert-response), a fleet roll-up (use pilot-fleet-conformance), or any mutating action. Pilot mutates via `cub` / Argo / kubectl — never via cub-scout.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout history *) Bash(cub-scout history *) Bash(cub scout history *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *) Bash(cub * get) Bash(cub * list) Bash(cub link list *) Bash(cub unit get *) Bash(argocd app get *) Bash(flux get *)

---

# pilot-patch-and-drift

The drift-classification scenario from **Pilot's** side. A field has diverged from its intended value; Pilot needs to decide what to do — revert, quarantine, accept-as-canonical, or ASK a human. The decision depends on **who** changed the field and **what kind** of change it was, which is exactly what cub-scout's attribution layer (#435) produces.

cub-scout's attribution surface is deterministic: every `compareFieldMismatch` carries `cause` (`controller-drift` / `manual-edit` / `unknown`) + `managerHint` (the verified manager-string from `managedFields`) + `gitSource` (where the intended value came from) + `bindingSource` (which ConfigHub Link supplied it, when connected). Pilot reads that triple and picks an action.

## When to use

Explicit phrasings:

- "Pilot, classify this drift"
- "Is this manual edit or controller drift?"
- "Should Pilot revert this patch?"
- "Quarantine or accept-as-canonical?"
- "What does cub-scout say about who touched this field?"
- "Render a drift verdict for the audit"
- "Decision: is this an emergency manual fix we accept, or noise we revert?"
- "Argo and kubectl both wrote this field — who actually owns it?"

Implicit intents:

- A **divergence is already known** (an audit found it, a watch event surfaced it, a gate failed) — this isn't the discovery skill
- The user wants a **classification + action**, not just a description
- The action is the **operator's call** but informed by deterministic attribution evidence
- The result may persist as a **fingerprinted receipt** if the action needs an audit trail

## Do not load for

- A **pre-deploy gate** (drift hasn't happened yet) — [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md)
- A **real-time event-driven** drift response — [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md)
- A **fleet roll-up verdict** — [`pilot-fleet-conformance`](../pilot-fleet-conformance/SKILL.md)
- **Postmortem evidence assembly** — [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md)
- Pure **investigation** without a Pilot policy decision (operator wants to understand, not decide) — [`investigate-drift`](../investigate-drift/SKILL.md)
- The **first-pass discovery** of drift — [`scout-attribute`](../scout-attribute/SKILL.md) (the underlying capability) or [`scout-compare`](../scout-compare/SKILL.md)
- Any **mutating action**. cub-scout doesn't revert, quarantine, or apply. Pilot's mutation path is `cub` / Argo / Flux / kubectl.

## The Pilot ↔ cub-scout drift loop

```
   alert: drift detected on deploy/payments-api spec.replicas
        │
        ▼
   Pilot (acceptance judge)
        │  "classify this drift, decide an action"
        │
        ├──► cub-scout compare three-way deploy/payments-api -n prod
        │     --format json
        │     │
        │     │   per-field fieldMismatches[] with:
        │     │     cause          (controller-drift / manual-edit / unknown)
        │     │     managerHint    (verified manager string)
        │     │     gitSource      (repoUrl / revision / path / file / line)
        │     │     bindingSource  (upstream unit + binding path + link, connected)
        │     │     compareThreeWay (DRY/WET/LIVE values per field)
        │     ▼
        │
        ├──► (optional) cub-scout history deploy/payments-api -n prod
        │     # ConfigHub ChangeSet timeline for the past 24h
        │
        ├──► (optional) cub-scout explain deploy/payments-api -n prod --presentation ai
        │     # narrative ownership + lineage (AI-shaped output)
        │
        └──► (optional, for audit) cub-scout receipt verify deploy/payments-api -n prod
              --predicate no-manual-edits-since --since <timestamp> --save
        │
        ▼
   Pilot's drift verdict, by field:
        │
        │  cause: controller-drift   → ACCEPT (controller is reconciling; transient)
        │  cause: manual-edit + Argo-owned → INVESTIGATE → likely REVERT or QUARANTINE
        │  cause: manual-edit + Flux-owned → same
        │  cause: manual-edit + unmanaged   → ACCEPT-AS-CANONICAL or QUARANTINE
        │  cause: unknown                   → ASK (managedFields incomplete)
        │
        ▼
   downstream consumer reads the action
        │  ACCEPT             → no-op
        │  REVERT             → route to Pilot's REVERT path (controller sync or
        │                       intent re-apply; mutation lives outside cub-scout)
        │  QUARANTINE         → tag the resource, alert the owner
        │  ACCEPT-AS-CANONICAL → route to Pilot's intent-update path (Git PR / ConfigHub
        │                       unit update; mutation lives outside cub-scout)
        │  ASK                → page a human
        └────────────────────────────────────────────────────
```

## Five Pilot policy cases over three `cause` values + owner context

cub-scout's attribution layer produces one of **three** `cause` values per field (per `pkg/agent/field_ownership.go:10-30`): `controller-drift`, `manual-edit`, `unknown`. Pilot's policy decision is shaped by the (cause, owner, managerHint) triple — so the same `cause` can map to different actions depending on whether the resource has a controller owner and whether the writer's manager string is in the verified enumeration:

| `cause` | Owner / manager context | Typical Pilot action |
|---------|-------------------------|----------------------|
| `controller-drift` | Detected owner's controller wrote the field; managerHint is in the verified enumeration. The drift is the controller doing its job (HPA scaled replicas, etc.). | **ACCEPT** — no action; the controller is reconciling. |
| `manual-edit` | Argo / Flux / ConfigHub / kro / Crossplane owner; `kubectl-*` writer touched the field. The controller will fight the manual edit on next sync. | **INVESTIGATE** — read the gitSource / bindingSource; usually **REVERT** (let the controller win) or **QUARANTINE** if the manual edit was an emergency fix. |
| `manual-edit` | No controller owner (unmanaged resource); `kubectl-*` writer. There's no controller to fight back. | **ACCEPT-AS-CANONICAL** (update intent to match) or **QUARANTINE** if the resource shouldn't be unmanaged. |
| `controller-drift` | A controller wrote it, but the managerHint is **not** in the verified enumeration, OR it's from a different controller than the detected owner (e.g., Argo wrote on a Flux-owned resource). | **ASK** — possible controller-on-controller conflict; cub-scout doesn't guess which is canonical. |
| `unknown` | `managedFields` is missing (older K8s, stripped) or the field-path resolution failed. | **ASK** — Pilot doesn't have enough evidence. |

The five rows above are Pilot policy cases, not five distinct `cause` values from cub-scout. cub-scout's `cause` is the three-valued enum; the owner / manager / verified-or-not context disambiguates within the `manual-edit` and `controller-drift` cases. See [`verified-manager-strings`](../references/verified-manager-strings.md) for the canonical enumeration.

The verified manager-string enumeration ([`verified-manager-strings`](../references/verified-manager-strings.md)) covers Argo CD, Flux (kustomize/helm/source controllers), Helm direct, Crossplane (composite/composed/claim/MRD/refs), kro (applyset/parent/labeller), and `kubectl-*`. Strings not in the enumeration return `unknown` rather than misclassifying — same `parse, don't guess` rule used by ownership detection.

## Step-by-step

### Step 1 — get the structured evidence

```bash
$ cub-scout compare three-way deploy/payments-api -n prod --format json
```

The output's `fieldMismatches[]` is per-field. Each entry has:

```json
{
  "path": "spec.template.spec.containers[0].image",
  "compareThreeWay": {
    "dry": "ghcr.io/org/payments-api:v1.4.2",
    "wet": "ghcr.io/org/payments-api:v1.4.2",
    "live": "ghcr.io/org/payments-api:v1.4.3-hotfix"
  },
  "cause": "manual-edit",
  "managerHint": "kubectl-edit",
  "gitSource": {
    "repoUrl": "https://github.com/org/payments",
    "revision": "abc123",
    "path": "apps/prod/payments-api/deployment.yaml",
    "file": "apps/prod/payments-api/deployment.yaml",
    "line": 42
  },
  "bindingSource": null
}
```

This is the canonical evidence shape Pilot reads. The triple `(cause, managerHint, gitSource)` carries the deterministic classification; the four values (`dry`, `wet`, `live`, plus the resolved `gitSource`) carry the divergence shape.

### Step 2 — (connected) check the binding source

When connected, the same output includes a `bindingSource` per field that the field came from an upstream ConfigHub Link:

```json
{
  "bindingSource": {
    "unit": "payments-base",
    "path": ".spec.image",
    "link": "lnk-2024-prod-image-bump"
  }
}
```

Reading: the live `image` field was supposed to come from upstream unit `payments-base` via Link `lnk-2024-prod-image-bump`. Pilot can:

- `cub unit get payments-base` to see the canonical value
- `cub link list payments-api` to see all incoming bindings
- Decide whether the divergence is in the downstream resource or the upstream unit

When standalone or the resource isn't ConfigHub-linked, `bindingSource: null` and Pilot drops back to the `cause` + `managerHint` evidence.

### Step 3 — (optional) get the change timeline

```bash
$ cub-scout history deploy/payments-api -n prod --since 24h --format json
```

The history surface produces the ConfigHub ChangeSet timeline (connected only). Useful when:
- The drift might be a recent intentional change Pilot should reconcile to
- The audit needs a "who changed what when" entry

### Step 4 — render the verdict

Pilot's verdict per field is the action from the table above. For the worked-example field (manual-edit on an Argo-owned image field):

```json
{
  "resource": "Deployment/payments-api in prod",
  "field": "spec.template.spec.containers[0].image",
  "cause": "manual-edit",
  "managerHint": "kubectl-edit",
  "owner": "argo",
  "action": "INVESTIGATE",
  "recommendation": "Likely emergency hotfix — verify with on-call before reverting. If intentional, update intent (Git PR to apps/prod/payments-api/deployment.yaml line 42); if not, let Argo sync revert it.",
  "evidence_path": null
}
```

### Step 5 — (optional) persist a receipt

If the verdict needs an audit trail (e.g., for compliance review later):

```bash
# Predicate "no-manual-edits-since": BLOCK if any kubectl-* writer touched the resource
# after the cutoff. The cutoff is typically the last known intentional change.
$ cub-scout receipt verify deploy/payments-api -n prod \
    --predicate no-manual-edits-since \
    --since 2026-05-22T00:00:00Z \
    --save \
    --out drift.receipt.json
```

The receipt records what cub-scout observed at this moment + the verdict (BLOCK if a kubectl-* writer touched it after the cutoff). Pilot attaches `drift.receipt.json` to the drift ticket.

### Step 6 — invoke the action (Pilot's surface, not cub-scout's)

Pilot's mutation choices, by action (mutation implementation lives outside cub-scout — concrete CLI surfaces are intentionally abstracted here):

| Action | Pilot's mutation path |
|--------|----------------------|
| ACCEPT | none (no-op) |
| REVERT | Pilot's REVERT path — controller sync or intent re-apply, depending on which controller owns the resource. Implementation lives in Pilot / `cub` / Argo / Flux; this skill does not name a specific CLI. |
| QUARANTINE | Label/annotate the resource (out-of-scope for cub-scout); page the owner. |
| ACCEPT-AS-CANONICAL | Pilot's intent-update path — typically a Git PR amending intent, or a ConfigHub unit update. Implementation lives outside cub-scout. |
| ASK | Escalate to the on-call human. |

cub-scout participates **only in evidence**. The mutation paths above are all out-of-scope for this skill, and their concrete CLI surfaces (controller sync verbs, ConfigHub unit update verbs) are deliberately not enumerated here to keep cub-scout's read-only boundary loud.

## Worked example

A drift alert fires on `deploy/payments-api` in prod — `spec.template.spec.containers[0].image` has diverged.

```bash
$ cub-scout compare three-way deploy/payments-api -n prod --format json \
    | jq '.fieldMismatches[] | select(.path | contains("image"))'
{
  "path": "spec.template.spec.containers[0].image",
  "compareThreeWay": {
    "dry": "ghcr.io/org/payments-api:v1.4.2",
    "wet": "ghcr.io/org/payments-api:v1.4.2",
    "live": "ghcr.io/org/payments-api:v1.4.3-hotfix"
  },
  "cause": "manual-edit",
  "managerHint": "kubectl-edit",
  "gitSource": {
    "repoUrl": "https://github.com/org/payments",
    "revision": "abc123",
    "path": "apps/prod/payments-api/deployment.yaml",
    "file": "apps/prod/payments-api/deployment.yaml",
    "line": 42
  },
  "bindingSource": null
}
```

Pilot's reading:
- DRY=WET (`v1.4.2`) but LIVE=`v1.4.3-hotfix` — manual-edit
- `cause: manual-edit` + `managerHint: kubectl-edit` confirms a human ran `kubectl edit` or `kubectl set image`
- Owner (from `cub-scout explain`): Argo CD application `payments-api`
- `gitSource.file:line` points at the canonical declaration

Pilot's verdict:
- **Action**: INVESTIGATE
- **Recommendation**: Likely an emergency hotfix (suffix `-hotfix` is a strong hint). Verify with on-call:
  - If intentional → open Git PR amending `apps/prod/payments-api/deployment.yaml:42` to `v1.4.3-hotfix` and merge; Argo will sync.
  - If accidental → route to Pilot's REVERT path (a controller-sync verb in Pilot's stack); cub-scout doesn't perform the sync itself.

Pilot's downstream system records the verdict + the cub-scout evidence ID + a link to the on-call ticket.

## Edge cases the attribution layer handles

- **Argo CD CSA migration default and `kubectl apply --client-side` write the same manager string** — the classifier reads `pkg/agent/ownership.go` first; Argo-owned resources resolve as `controller-drift`, non-Argo as `manual-edit`. (See [`verified-manager-strings`](../references/verified-manager-strings.md) for the full enumeration.)
- **Crossplane composed children use prefix match** — `apiextensions.crossplane.io/composed-<hash>` varies per XR. The classifier matches by prefix, not exact string.
- **Per-field rollup degrades cleanly to resource-level** — when `FieldsV1` is missing (older K8s, stripped), classification is resource-level. When present, per-field. List-key fields (e.g., `spec.template.spec.containers[name="api"].image`) currently fall back to resource-level pending list-key selector support — `pilot-patch-and-drift` should treat resource-level `cause` as authoritative for those fields.
- **`gitSource.file:line` is opt-in via `--source-path <local-checkout>`** — the resource-level `gitSource` (repoUrl/revision/path) is automatic; raw-YAML back-resolution to `file:line` requires the CI pipeline (or Pilot's runtime) to have a local checkout available. Helm/Kustomize templating explicitly deferred.

## Tool boundary

- **Allowed (cub-scout):** Compare verbs (read-only attribution); `explain`, `trace`, `history`; `receipt verify` / `show` / `validate` / `list`. All read-only.
- **Allowed (cluster):** `kubectl get/describe`, `kubectl get --show-managed-fields` (Pilot may want to inspect the raw `managedFields` to verify cub-scout's classification). NOT `kubectl apply / edit / patch / delete`.
- **Allowed (ConfigHub):** `cub * get / list`, `cub unit get`, `cub link list`. NOT `cub * create / update / delete`.
- **Allowed (controllers):** `argocd app get`, `flux get`. NOT `argocd app sync`, `flux reconcile` (those are Pilot's REVERT path, not cub-scout's).
- **Pilot's mutation paths** (REVERT, QUARANTINE, ACCEPT-AS-CANONICAL) are out-of-scope for this skill. cub-scout produces the classification; Pilot decides; the mutation lives in `cub` / Argo / Flux.

## References

- [`scout-attribute`](../scout-attribute/SKILL.md) — the underlying attribution capability (cub-scout side)
- [`scout-compare`](../scout-compare/SKILL.md) — the Compare verbs that surface attribution
- [`scout-verify`](../scout-verify/SKILL.md) — receipt persistence
- [`investigate-drift`](../investigate-drift/SKILL.md) — operator-driven counterpart (same evidence, different framing)
- [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md) — pre-deploy companion
- [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md) — real-time companion
- [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md) — postmortem companion
- [`verified-manager-strings`](../references/verified-manager-strings.md) — the canonical manager-string list
- [`kubernetes-managedfields`](../references/kubernetes-managedfields.md) — `managedFields` mechanics
- Attribution layer: `#435` (parent), `#437` (A1+A1.5+A2 classifier + manager strings + git anchor), `#438` (C1 incoming binding), `#439` (C2 per-field binding source), `#440` (B raw-YAML file:line)

## Constraints

- Per-field `cause` classification requires `managedFields` present on the resource (K8s ≥ 1.18 default; some controllers strip it). When missing, classification degrades to resource-level.
- The verified manager-string enumeration is a **whitelist**, not a heuristic. New controllers' manager strings need to be added to `pkg/agent/manager_strings.go` (a `cub-scout` core change, not a Pilot-side patch).
- `bindingSource` requires connected mode AND the resource being ConfigHub-linked via `cub link create`. Without that, `bindingSource: null` and Pilot drops to `cause` + `managerHint` alone.
- `gitSource.file:line` requires `--source-path <local-checkout>` to be passed AND the source to be raw YAML (not Helm/Kustomize templated). Templated sources only get the resource-level `repoUrl/revision/path` anchor.
- Pilot's action policy (revert vs quarantine vs accept-as-canonical) is **Pilot's call**. cub-scout produces the classification; the action mapping above is illustrative, not normative.
