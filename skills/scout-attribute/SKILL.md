---
name: scout-attribute
description: 'Use when the user wants manager evidence for a field value on a running Kubernetes resource — which recognized controller or interactive manager is recorded for a path, which git file or ConfigHub Link supplies desired state, or whether resource-level managedFields contains an interactive manager. Natural phrasing: "why is replicas: 1 in prod?", "does this resource show a kubectl manager?", "is Argo still reconciling or is an interactive manager recorded?", "what git commit set this image tag?", "which upstream unit feeds this field?", "where does this value come from?", "show me the provenance of this Deployment", "is this controller-drift or manual-edit?", "has an interactive manager changed this Deployment?", "which manager is recorded for this field?". Load whenever attribution / provenance / lineage / git-blame-for-runtime intent appears, especially on `compare` and `explain` output. For a known resource and exact canonical field path, `explain --field-path` can return only that path’s manager evidence; `explain` without the selector has a resource-level summary. Do NOT promise the latest writer or a human identity from manager evidence: exact-path and compare results do not order managers by time. Do NOT load for: pure ownership classification at the resource level (use scout-observe — that gives Owner=Argo/Flux/etc., not per-field), source-truth strategy verdicts (use scout-compare), or live-cluster mutation (cub-scout cannot mutate — that is `cub` or `kubectl` with user driving).'
phase: verify
allowed-tools: Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *) Bash(cub link list *) Bash(cub link get *) Bash(cub unit get *) Bash(cub unit list *)
---

# scout-attribute

The Attribute layer of cub-scout (#435). Reads `metadata.managedFields` + label-based owner detection + Argo/Flux tracer + ConfigHub Link bindings. It reports recognized manager evidence for a field path when decodable `FieldsV1` is available, otherwise a resource-level manager summary, alongside the field's desired-state source where available. These manager classifications do not form a time-ordered mutation history.

## When to use

Explicit phrasings:

- "Why is `replicas: 1` in prod when ConfigHub says `replicas: 3`?"
- "Did someone `kubectl edit` this?" / "is this manual or controller drift?"
- "Is Argo still reconciling, or has the GitOps loop been bypassed?"
- "What git commit set this image tag?" / "trace this field back to its source"
- "Which upstream unit feeds the `image` field on this Deployment?"
- "Show me the provenance of this resource" / "git-blame this Deployment"
- "Is this `controller-drift` or `manual-edit`?"
- "What does the `cause` / `managerHint` field mean in this JSON?"

Implicit intents:

- The user is *already looking at a divergence* from [`scout-compare`](../scout-compare/SKILL.md) and wants to know who caused it
- The user wants per-field detail, not resource-level rollup
- The user is investigating an audit / postmortem and needs evidence with citations

## Do not load for

- Resource-level ownership classification (Owner = Argo / Flux / Sveltos / Modelplane / Helm / Crossplane / kro / ConfigHub / Native) — [`scout-observe`](../scout-observe/SKILL.md) covers that via `cub-scout map` / `cub-scout trace`
- "Did the deploy land?" verdicts at the resource level — [`scout-compare`](../scout-compare/SKILL.md)
- Diagnosing the *symptom* (CrashLoop, OOM, etc.) — [`scout-diagnose`](../scout-diagnose/SKILL.md)
- Live-cluster mutation — cub-scout never mutates. Route to `cub` or `kubectl` (user driving)

## Standalone vs connected

- **Standalone (cluster only):** `cause` + `managerHint` (from managedFields), `gitSource.{repoUrl, revision, path}` (from controller tracer). Per-field attribution via `FieldsV1` decoding (A1.5) works standalone.
- **Standalone with local checkout (cluster + `--source-path <dir>`):** Stage B back-resolution adds `gitSource.file` + `gitSource.line` for raw-YAML manifests (#440).
- **Connected (cluster + `cub auth login`):** adds `incomingBindings[]` (the ConfigHub Links influencing this unit) and per-field `bindingSource` (which upstream unit + path feeds each field).

The attribution layer **always emits something honest** — if managedFields was stripped, `cause: unknown` and `managerHint` is omitted. If ConfigHub is unavailable in standalone mode, `bindingSource` is omitted from the JSON. The current JSON contract expresses missing evidence via `cause: unknown` + field omission; structured `omissions[]` entries are a forthcoming receipt-layer surface (in #446 batch 1, not today's attribution output). See [`references/kubernetes-managedfields.md`](../references/kubernetes-managedfields.md) § "What happens when evidence is missing" for the today-vs-future distinction.

## Tool boundary

- **Allowed (read-only):** `cub-scout compare *`, `cub-scout explain *`, `cub-scout trace *`, `kubectl get/describe`, `kubectl get --show-managed-fields`, `cub link list/get`, `cub unit get/list`.
- **Not allowed:** any path that writes — `kubectl edit/patch/apply/delete`, `argocd app sync`, `cub * create/update/delete`. Attribution reads the writer; it never becomes one.

## The output shape

Attribution evidence is **not its own verb** — it's *enrichment* on the output of [`scout-compare`](../scout-compare/SKILL.md)'s `compare three-way` and [`scout-diagnose`](../scout-diagnose/SKILL.md)'s `explain`. The fields:

| Field | Means | Source |
|---|---|---|
| `cause` | One of `controller-drift`, `manual-edit`, `unknown`. A classification of recognized manager evidence for the mapped path, or a resource-level rollup when path attribution is unavailable. It does not say who wrote last. | `metadata.managedFields` + owner co-signal |
| `managerHint` | A representative recognized manager string (e.g., `argocd-controller`, `kubectl-edit`) for transparency. Resource-level explain output aggregates manager strings across the object. | `metadata.managedFields[].manager` |
| `gitSource` | `{repoUrl, revision, path, file?, line?}` — where the field's desired value lives in git | Controller spec (Argo Application / Flux GitRepository) + optional `--source-path` back-resolution (#440) |
| `bindingSource` | `{linkId, linkSlug, upstreamUnitId, upstreamPath, transformExpr}` — the ConfigHub Link that supplies this field's value | `cub link list` for the owning unit |
| `incomingBindings[]` | All Links whose downstream is this resource's unit | `cub link list --where "FromUnitID = ..."` |

See [`references/verified-manager-strings.md`](../references/verified-manager-strings.md) for the full enumeration of recognized manager strings and [`references/kubernetes-managedfields.md`](../references/kubernetes-managedfields.md) for the data substrate.

## The loop

1. **Identify the divergence.** The user is asking about a specific field value (`replicas: 1`, `image: ghcr.io/.../v1.4`, `env[LOG_LEVEL]`) — figure out which resource and which field.
2. **Pick the surface:**
   - If the resource and exact canonical field path are known and the user asks which managers are recorded for that path, call `./cub-scout explain <kind>/<name> -n <ns> --field-path '<path>' --format json` directly (MCP: `explain` with `field_path`). Do not run `doctor`, `map`, `trace`, or a broad compare first just to rediscover manager evidence. The `fieldAttribution` block is scoped to the requested path; the normal resource summary remains. Absent, malformed, unrecognized-only, or ambiguous evidence remains `unknown` and never falls back to resource-level evidence. Do not guess a canonical path; if the path is not known, use the comparison/read needed to identify it or ask the user.
   - For a *resource-level first read*, use `./cub-scout explain <kind>/<name> -n <ns>`. Its `mutationCause` and representative `mutationManager` aggregate manager evidence across the resource; they do not identify a field or latest writer.
   - Use `./cub-scout compare three-way --scope resource:<kind>/<name>` or `--scope namespace/<ns>` when the user also needs intended/live values or a divergence identified. A mismatch can use the mapped path's `cause` and `managerHint` when decodable `FieldsV1` is available; otherwise it may fall back to the resource-level summary, which must not be stated as evidence for that field. This classification does not order managers by time.
3. **Read the `cause`:**
   - `controller-drift` → a recognized expected-controller manager claims the resource or mapped path; this does not establish that reconciliation is happening now
   - `manual-edit` → a recognized interactive manager is present in the resource-level rollup or on the mapped compare path; this does not establish that it wrote last or identify a person. On an exact `fieldAttribution` query, shared controller and interactive claims or any unrecognized co-claimant make the field cause `unknown`, while all observed managers remain visible.
   - `unknown` → manager evidence is missing, unrecognized, or cannot be mapped; never guess
4. **Read the `gitSource`:** when present, this is the git origin of the desired value. Add `--source-path <local-checkout>` to populate `file` + `line` (stage B back-resolution, raw YAML only — #440).
5. **Read the `bindingSource`** (connected only): which upstream ConfigHub unit + path supplies this field. `incomingBindings[]` shows the full Link graph for the unit.
6. **Hand off.** Treat `cause: manual-edit` as manager evidence, not proof of a person or latest write. If the user separately needs raw entry details or timestamps, inspect that field's `managedFields` path; timestamps still do not identify a person and may not establish the latest writer. If the record is incomplete or ambiguous, say unknown. Any correction remains user-driven; cub-scout does not mutate.

## Worked examples

### A: standalone attribution with file:line

The primary v1 path. No ConfigHub auth required.

```bash
$ cub-scout compare three-way Deployment/api -n prod --source-path /home/me/platform-config
Drift cause: controller-drift (manager: argocd-controller)
Git source: https://github.com/org/platform-config @abc123 path=apps/prod/api file=deployment.yaml:9

Diff Highlights
  - replicas: DRY=3 | WET=3 | LIVE=1
```

Standalone — no ConfigHub. Argo is the controller; the `gitSource.file:line` points at the exact line in the local checkout where `replicas` is set. Stage B (#440) — raw YAML only; Helm/Kustomize templated sources fall back to resource-level `gitSource` (no `file:line`).

Without `--source-path`, the resource-level `gitSource` still applies — `repoUrl`, `revision`, `path` from the controller's spec.

### B: classifying divergence as controller-drift vs manual-edit *(connected enrichment)*

```bash
$ cub-scout compare three-way Deployment/api -n prod
Compare Resource: Deployment/api (namespace: prod)
Mode: dry-live    Connection: connected

DRY (unit intent)
  replicas: 3
LIVE (cluster)
  replicas: 1

Drift cause: manual-edit (manager: kubectl-edit)

Diff Highlights
  - replicas: DRY=3 | WET=- | LIVE=1
      <- bound from unit:01HFK...XY path:.spec.scale.value via link:replicas-from-scale
```

There is no WET side: ConfigHub no longer exposes a unit's live data, so a connected compare shows DRY against LIVE (`Mode: dry-live`) and says so in its notes.

For this `.spec.replicas` mismatch, a path-specific `manual-edit` classification means a recognized interactive manager is recorded for that path (when decodable `FieldsV1` is available); a controller manager may also be recorded. It does not prove which manager wrote the value last or identify who ran a command. A resource-level fallback must not be stated as evidence about `replicas`. The `bound from` line tells the user the upstream ConfigHub unit + path that *should* be feeding `replicas` — that's the C2 connected enrichment on top of standalone's cause + gitSource.

Next read-only step (don't apply!): inspect the exact path's managed-field entries; use audit logs if the user needs write history or identity.

```bash
$ kubectl get deploy/api -n prod -o yaml --show-managed-fields | grep -A 3 "manager: kubectl-edit"
- manager: kubectl-edit
  operation: Update
  time: "2026-05-21T13:42:00Z"
  fieldsV1: { f:spec: { f:replicas: {} } }
```

So `replicas` was overwritten by `kubectl edit` at 13:42 UTC. The user can now decide: port back to ConfigHub (governed) or revert (also governed). The skill does not act.

### C: per-field binding (connected, C2)

```bash
$ cub-scout compare three-way Deployment/api -n prod --format json | \
    jq '.mismatches[] | {field, cause, managerHint, bindingSource}'
{
  "field": "replicas",
  "cause": "manual-edit",
  "managerHint": "kubectl-edit",
  "bindingSource": {
    "linkId": "01HFK...A1",
    "linkSlug": "replicas-from-scale",
    "upstreamUnitId": "01HFK...XY",
    "upstreamPath": ".spec.scale.value",
    "transformExpr": "to_int"
  }
}
```

The agent can say that the live `replicas` path is associated with the `kubectl-edit` manager while the intended value comes from upstream unit `01HFK...XY` at path `.spec.scale.value` via link `replicas-from-scale`. The manager string does not identify a human or prove it was the latest write.

## Output evidence

- Embedded in `compare three-way` JSON and `explain` JSON — there is no `cub-scout attribute` command. The Attribute layer is *enrichment*, not its own verb.
- See `docs/reference/json-contracts.md` § "Field Mutation Attribution Contract" for the full schema.
- Example fixtures: `examples/drift/mutation-cause-attribution/`.

## References

- Capability map: [`README.md`](../../README.md#capability-map) § Attribute
- Parent attribution issue: #435 (closed) — shipped via #437 / #438 / #439 / #440
- Manager-string enumeration: [`references/verified-manager-strings.md`](../references/verified-manager-strings.md)
- managedFields substrate + caveats: [`references/kubernetes-managedfields.md`](../references/kubernetes-managedfields.md)
- Receipt (forthcoming): the attribution evidence will be embedded in `cub-scout receipt verify` output once #446 batch 1 lands

## Constraints

- `cause: manual-edit` is a *manager-presence signal*, not proof of a person or the latest writer. managedFields records a manager string, not the human identity behind it. To attribute to a specific user or order writes, cross-reference cluster audit logs (out of scope for cub-scout).
- managedFields is **field-manager evidence, not complete or time-ordered mutation-history proof**. The classifier does not order managers by their timestamps. A controller that was overwritten and then re-reconciled may not leave a manual-edit trace if the controller subsequently wins. For stronger history, pair with a K8s admission audit log.
- The `kubectl-client-side-apply` manager string is *ambiguous* — Argo CD's CSA migration uses it as the default, and so does `kubectl apply` (client-side). The classifier disambiguates via the `argocd.argoproj.io/tracking-id` annotation (label co-signal). See [`references/verified-manager-strings.md`](../references/verified-manager-strings.md).
- Crossplane composed-resource manager strings carry a per-XR hash suffix (`apiextensions.crossplane.io/composed-<hash>`). The classifier matches by **prefix**, not exact string.
- For *templated sources* (Helm / Kustomize), `gitSource.file:line` is not populated (stage B handles raw YAML only). Resource-level `gitSource` (`repoUrl`, `revision`, `path`) still applies.
- Attribution evidence is read-only enrichment. The triad lock (#410 / #428) means no `cub-scout` path can act on the evidence. Always hand off mutation to the appropriate `cub` / `kubectl` skill with the user driving.
