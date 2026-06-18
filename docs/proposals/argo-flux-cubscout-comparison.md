# Argo CD vs Flux vs cub-scout — Diff / Dry-Run / Drift (comparison for AI review)

**Author:** Claude (helm-expt/cub-scout work). **Date:** 2026-06-18.
**Purpose:** a reviewable, claim-by-claim comparison. Hand this to another model
and ask it to (a) falsify any claim, (b) re-rate confidence, (c) fill the open
questions in §9.

## 0. How to review this
- Every claim has an ID `[Cn]`, a **confidence** `H/M/L`, and a **source**.
- `H` = verified against a doc or source I read; `M` = inferred or
  version-dependent; `L` = uncertain.
- **Reviewer: start with §8** (the claims most likely to be wrong). Then
  challenge any `H` you doubt.
- Behaviour is **version-specific** — see §1. A claim can be "true for v2.13,
  false for v3.1." Call that out.

## 1. Scope, versions, legend
- **Scope:** the *observe / diff / dry-run / drift* capability, where all three
  tools overlap. Delivery/reconciliation is covered only for category context
  (§2). This is **not** a general "which GitOps tool should I use" doc.
- **Versions (as of 2026-06):** Argo CD ~v2.13–v3.x; Flux v2
  (`kustomize-controller` + `helm-controller`); **cub-scout v2.5.0**
  (source-verified in local checkout `v2.5.0-2-ge03b67c`,
  branch `claude/object-set-diff-proposal`).
- **Confidence legend:** H verified · M inferred/version-dependent · L uncertain.

## 2. Category — the framing that must be correct first
- `[C1, H]` Argo CD and Flux are **continuous-delivery controllers**: they render
  desired manifests from a source, **apply** them to the cluster, and
  **continuously reconcile** (including drift correction).
- `[C2, H, cub-scout source]` cub-scout is a **read-only observer/differ**. It
  does **not** apply, sync, or correct. It observes live state and compares it to
  a desired set it is *given*.
- `[C3, H]` Therefore cub-scout **complements** Argo/Flux; it does not replace
  them. The only fair head-to-head is the **diff/dry-run/drift** axis below. Any
  claim that "cub-scout beats Argo/Flux" must be read as *on that axis only*.

## 3. Comparison matrix (claim-by-claim)

| Dimension | Argo CD | Flux v2 | cub-scout v2.5.0 |
|---|---|---|---|
| Deploys + reconciles | yes `[C4,H]` | yes `[C5,H]` | **no — read-only** `[C2,H]` |
| Renders the desired side itself | yes, `helm template` / `kustomize build` `[C6,H]` | yes, **real helm install/upgrade** + `kustomize build` `[C7,H]` | **no — consumes a supplied render** (`--dry-from` file/dir, or ConfigHub intent) `[C8,H]` |
| Helm hooks at desired-build time | not run during template; handled as Argo *resource hooks* (sync phases) `[C9,H]` | **run** (real install/upgrade; `.disableHooks` to disable) `[C10,H]` | n/a — diffs whatever set you supply `[C8,H]` |
| Helm `lookup` (reads live objects) | **returns empty** (no cluster contact during render) `[C11,H]` | **expected to work during real install/upgrade** because Helm `lookup` uses the active Kubernetes connection; DNS `getHostByName` is separately feature-gated `[C12,M]` | n/a (no render) `[C8,H]` |
| Live-side diff technique | server-side diff (SSA dry-run) stable since v3.1 but still documented as controller/app opt-in; legacy/client-side diff otherwise `[C13,H]` | **server-side apply dry-run** (`flux diff kustomization` = build + SSA dry-run) `[C14,H]` | reads live objects; `compare three-way` DRY/WET/LIVE `[C15,H]` |
| Mutating-webhook effects in the diff | **opt-in** (`IncludeMutationWebhook=true` + ServerSideDiff; 3.x), off by default `[C16,H]` | for SSA dry-run paths, admission/defaulting participates as Kubernetes server-side dry-run permits; webhook behaviour depends on dry-run-compatible webhooks, not Flux managed-fields `[C17,M]` | not modelled by dry-run; runtime mutation is witnessed as live-state residue `[C18,M]` |
| Field-level diff | yes `[C19,H]` | yes `[C20,H]` | yes (`compareResourceResult.Mismatches`) `[C21,H]` |
| Maps a diff back to the input **value** (provenance) | no (source-agnostic field diff) `[C22,M]` | no `[C23,M]` | `--source-path` → gitSource file:line; **value-level** (helm value) is *proposed* (#496/#481), not shipped `[C24,H]` |
| Drift detection | OutOfSync status; optional auto-sync + self-heal `[C25,H]` | on by default for Kustomizations (SSA dry-run each interval) + auto-correct; HelmRelease drift detection is available but `mode` defaults disabled unless configured (`warn`/`enabled`) `[C26,H]` | observes drift + field-manager attribution (`ManagerHint`); **does not correct** `[C27,H]` |
| Scope of one diff | per **Application** (1 source→1 dest); ApplicationSet fans out apps `[C28,H]` | per **Kustomization / HelmRelease** `[C29,H]` | resource / namespace / cluster / View `[C30,H]`; **fleet roll-up proposed** (#496) `[C31,H]` |
| Cross-environment ("fleet") blast-radius in one view | no `[C32,M]` | no `[C33,M]` | proposed, not shipped `[C31,H]` |
| Tool coupling of the diff | Argo-managed apps `[C34,H]` | Flux-managed sets `[C35,H]` | **tool-agnostic** (reads cluster + supplied desired, not the reconciler) `[C36,H]` |
| Determinism of the desired side | Argo compares by running/caching manifest generation; random Helm functions can still regenerate and cause OutOfSync `[C37,H]` | Kustomizations rebuild on interval; HelmReleases install/upgrade on chart/value change and drift-check release state, so do **not** summarize as "re-installs each reconcile" `[C38,M]` | **as deterministic as the supplied render** — not a cub-scout property; it inherits determinism from a held render-once source `[C39,H]` |

## 4. Per-tool detail

### Argo CD
- `[C40,H]` Repo-server renders manifests; for Helm it runs `helm template` (no
  cluster contact), so `[C11]` lookup is empty and `[C9]` hooks are not in the
  rendered set (Argo runs them as PreSync/Sync/PostSync resource hooks).
- `[C13,H]` / `[C16,H]` Server-side diff (SSA dry-run) is stable since v3.1 and
  reduces false diffs from defaulting/admission; current docs still present it
  as controller/app opt-in (`controller.diff.server.side: "true"` or
  `ServerSideDiff=true`). Mutating-webhook effects are separately **opt-in**
  and off by default (`IncludeMutationWebhook=true`).
- `[C41,M]` Consequence: charts that use `lookup` to *reuse* a generated secret,
  or generate values per render, can show permanent OutOfSync unless masked with
  `ignoreDifferences`. (This is the well-known "OutOfSync forever" failure mode.)

### Flux v2
- `[C7,H]` `helm-controller` performs **real** Helm install/upgrade via Helm
  actions; `[C10]` hooks run by default unless `.spec.install.disableHooks` /
  `.spec.upgrade.disableHooks` is set. `[C12,M]` follows from Helm semantics:
  `lookup` uses Helm's Kubernetes connection during non-dry-run actions, but
  this draft did not live-test a Flux HelmRelease using `lookup`.
- `[C14,H]` `kustomize-controller` applies with SSA; `flux diff kustomization`
  does build + SSA dry-run + prints the diff; drift detection is on by default
  (`[C26]`).
- `[C42,H]` `helm-controller` cluster-state drift detection is available, but
  unlike Kustomization drift detection it is configured per HelmRelease and
  `mode` defaults to disabled unless set to `warn` or `enabled`.

### cub-scout v2.5.0 (source-verified)
- `[C8,H]` Does not render; `compare three-way --dry-from <file|dir>` supplies the
  DRY (desired) side from a local rendered set — so it runs with **no ConfigHub**
  and no reconciler.
- `[C21,H]` Field-level `Mismatches` per resource; `[C27]` drift attributed to the
  writing field manager (`Live.Attribution.ManagerHint`); `[C24]` `--source-path`
  resolves gitSource file:line.
- `[C36,H]` Tool-agnostic: it reads *cluster + supplied desired*, so Argo-,
  Flux-, and cub-direct-delivered objects diff identically.
- `[C43,H]` Also ships `object-set-matches` (boolean set match),
  `workloads-converged`, `prerequisites-met`, receipt freshness, normalization
  profiles, canonical receipt digests, chain walking (v2.5.0).
- `[C44,H]` **Gaps (proposed, not shipped):** a *set-level* `ObjectSetDiffReceipt`
  (vs per-resource / boolean); **value**-level provenance (#481 epic, Phase 2);
  a **fleet** roll-up (#496).

## 5. Where cub-scout genuinely differs (honest, on the diff axis)
- `[C36]` **Tool-agnostic** one differ for Argo/Flux/cub-direct, vs each tool's
  own diff for its own managed objects.
- `[C39]` Can diff a **deterministic held desired set** (render-once) instead of
  a fresh re-render — *when paired with such a source* (e.g. helm-expt).
- `[C24]/[C44]` Designed to carry **value-provenance** and a **fleet** view
  (proposed) that Argo/Flux diffs do not have.
- `[C45,H]` Read-only + receipt-based (fingerprinted/tamper-evident and
  chain-walkable, **not cryptographically signed** in current shipping code) —
  an evidence artifact, not a controller action.

## 6. Where it does NOT differ / Argo+Flux strengths (honest)
- `[C46,H]` Argo and Flux **deploy and reconcile**; cub-scout does not. For
  delivery there is no contest — cub-scout is not in that category.
- `[C47,H]` Argo/Flux server-side live-diff is mature; cub-scout should *adopt*
  the same SSA technique, not claim to beat it on the **live** side.
- `[C48,H]` cub-scout's determinism is **inherited from its input**, not magic —
  feeding it a fresh `helm template` gives the same nondeterminism as Argo.
- `[C49,H]` The set-level receipt, value-provenance, and fleet view are **not yet
  implemented** in cub-scout (they are a proposal, confighub/cub-scout#496).

## 7. Net claim (scoped, defensible)
> On the **diff/dry-run/drift axis**, cub-scout can be a single **tool-agnostic**
> differ that (when fed a held render-once desired set) is deterministic, and
> that is designed to add **value-provenance** and a **fleet** view — capabilities
> Argo's and Flux's per-tool diffs lack. It does **not** deploy or reconcile, and
> several of those advantages are proposed, not shipped.

## 8. Claims most likely to be wrong — reviewer, focus here
- `[C12,M]` Helm `lookup` works under Flux's real install. This is grounded in
  Helm + Flux docs by composition, but still deserves a live HelmRelease
  fixture before upgrading to H.
- `[C13,H]/[C16,H]` Argo server-side diff is stable since v3.1 but still
  documented as opt-in; webhook inclusion is separately opt-in.
- `[C26,H]/[C42,H]` Flux **helm-controller** drift detection is available but
  defaults disabled unless `.spec.driftDetection.mode` is set.
- `[C38,M]` Flux desired-side determinism needs controller-specific wording:
  Kustomizations rebuild each interval; HelmReleases should not be described as
  blindly re-installing/re-rendering every reconcile.
- `[C22,C23]` Argo/Flux have **no** value-level provenance — confirm no plugin
  provides it.
- `[C17,C18]` mutating-webhook handling in Flux SSA + cub-scout residue framing.

## 9. Open questions for the reviewer
1. Does any Argo CD release after v3.1 enable server-side diff by default? The
   current stable docs still describe explicit controller/app enablement.
2. Under Flux `helm-controller`, does the Helm `lookup` function resolve live
   objects at install (so the rendered set differs from `helm template`)?
3. Is there any Argo/Flux feature (plugin, config-management-plugin) that maps a
   field diff back to the source **value** (not just file:line)?
4. Does any Argo/Flux surface aggregate a diff **across environments** in one
   view (beyond ApplicationSet listing per-app status)?
5. Is the category framing (§2) correct — is cub-scout ever positioned as a CD
   controller, which would change the comparison?

## 10. Sources
- Argo CD diff strategies: https://argo-cd.readthedocs.io/en/stable/user-guide/diff-strategies/
- Argo CD diff customization: https://argo-cd.readthedocs.io/en/stable/user-guide/diffing/
- Argo CD helm `lookup` (unsupported) issues: https://github.com/argoproj/argo-cd/issues/5202 , https://github.com/argoproj/argo-cd/issues/21745
- Flux helm-controller: https://github.com/fluxcd/helm-controller , https://fluxcd.io/flux/components/helm/helmreleases/
- Flux helm DNS lookups (feature gate): https://fluxcd.io/flux/installation/configuration/helm-dns-lookup/
- Flux `flux diff kustomization`: https://fluxcd.io/flux/cmd/flux_diff_kustomization/
- Flux Kustomization drift detection: https://fluxcd.io/flux/components/kustomize/kustomizations/
- Flux HelmRelease API / drift default: https://fluxcd.io/flux/components/helm/api/v2/
- Flux helm-controller drift issue (historical announcement/limitations): https://github.com/fluxcd/helm-controller/issues/643
- cub-scout v2.5.0: `confighub/cub-scout` source (`cmd/cub-scout/compare_three_way.go`, `compare_resource.go`, `receipt_object_set.go`); proposal `confighub/cub-scout#496`.
- helm-expt schema/spec: `confighub/helm-expt#992` (ObjectSetDiffReceipt), value-source-map, fleet.yaml.

## 11. Empirical addendum — `lookup` template-vs-install (2026-06-18, kind)

Live test of the load-bearing claim `[C11]`/`[C12]`. Chart template:
`secret-found: "{{ if (lookup "v1" "Secret" "default" "preexisting") }}true{{ else }}false{{ end }}"`,
with the `preexisting` Secret created in the cluster first.

| Path | Result |
|---|---|
| `helm template` (Argo's render path — no cluster connect) | `secret-found: "false"` (lookup empty) |
| `helm install` (the Helm action Flux `helm-controller` invokes) | `secret-found = true` (lookup resolved) |

- **Confirms `[C11]`** (Argo `helm template` → `lookup` empty) → confidence **H, empirical**.
- **Confirms the Helm semantic under `[C12]`** (real install → `lookup` resolves).
- **Does NOT yet** run a full Flux **HelmRelease** under `helm-controller`, so it
  does not rule out helm-controller-specific sandboxing of `lookup`. That remains
  the **M→H** step. Friction: supplying a `lookup` chart via a cluster-reachable
  source (in-kind OCI registry, or push to ghcr). Plan tracked before promotion.
