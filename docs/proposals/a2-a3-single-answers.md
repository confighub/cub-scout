# Proposal: Single-Answer Surfaces for A(2) and A(3)

**Status:** Draft proposal — awaiting maintainer approval before any code change  
**Date:** 2026-11-24  
**Tracks:** A(2) and A(3) in [docs/roadmap-3.0-execution.md](../roadmap-3.0-execution.md) (Refs #596, #645)

---

## 1. Context

Two agent-loop questions from [agentic-gitops-observability.md](../reference/agentic-gitops-observability.md) are partly answered today by existing commands, but not yet shaped as single, bounded agent answers:

- **A(2) — Look before writing:** *"What is this field right now, who last set it, and will a controller revert a direct change?"*
- **A(3) — Unrecorded changes:** *"What changed on the cluster that nobody recorded in Git or ConfigHub?"*

Both already have the underlying read logic in `cub-scout`:
- A(2): `cub scout explain <resource> --field <path>` already extracts `managedFields` attribution (`pkg/agent/field_ownership_paths.go`), and `gitops settings` already reads Argo CD `syncPolicy.automated.selfHeal`, `ignoreDifferences`, and Flux `spec.suspend` / `spec.force` (`cmd/cub-scout/gitops_settings.go`).
- A(3): `cub scout compare three-way` already computes live-only / unrecorded diffs between intended (DRY), recorded (WET), and live cluster state (`cmd/cub-scout/compare_three_way.go`).

This proposal defines the smallest single-answer shape for each so an agent gets a bounded, verifiable answer in one call without adding a sprawling new command family.

---

## 2. A(2): Field Value, Last Manager, and Revert Prediction

### Proposed surface
Extend the existing `cub scout explain <resource> -n <ns> --field <path> --format json` (and MCP `explain` with optional `field` parameter) rather than adding a separate top-level command.

### Structured evidence added when `--field <path>` is passed
Under the existing `fieldAttribution` object (omitted when `--field` is not passed):
- `path` (existing): normalized field path, e.g. `.spec.replicas`.
- `value` (new, quoted/JSON): the current value read from the live object at `path`, plus `valueFound: bool` so absence is never confused with `""` or `0`.
- `managers` / `cause` / `reason` (existing): observed `managedFields` manager(s) for `path`.
- `revertPrediction` (new):
  - `verdict`: `"likely-reverted"` | `"not-expected"` | `"unknown"` (never guesses when controller settings were not read).
  - `reason`: e.g. `"argo-self-heal-enabled"`, `"argo-self-heal-disabled"`, `"argo-field-ignored"`, `"flux-reconciling-active"`, `"flux-deployer-suspended"`, `"no-owner-marker"`, `"deployer-settings-not-read"`.
  - `restsOn`: `"controller-spec"` (from the deployer's declared spec, never a guarantee of runtime controller behavior) or `"object-metadata"`.
  - `deployerRef`: exact `kind/namespace/name` of the deployer whose spec was read.

### Rules
1. **Flux:** If the owning `Kustomization` or `HelmRelease` is read and `spec.suspend != true`, `verdict` is `"likely-reverted"` (`flux-reconciling-active`); if `spec.suspend == true`, `"not-expected"` (`flux-deployer-suspended`).
2. **Argo CD:** If the owning `Application` is read:
   - If `path` matches `spec.ignoreDifferences`, `"not-expected"` (`argo-field-ignored`).
   - Else if `spec.syncPolicy.automated.selfHeal == true`, `"likely-reverted"` (`argo-self-heal-enabled`).
   - Else `"not-expected"` (`argo-self-heal-disabled`).
3. **Anything else (no owner marker, unreadable deployer, unsupported controller):** `"unknown"` with the exact reason. Never claim `"not-expected"` on missing data.
4. **No new `answer.state` words required:** `explain`'s existing `answer` states (`explained`, `owner-only`, `no-owner-marker`, `not-observable`) remain unchanged; `answer.basis[]` adds one concise claim citing `revertPrediction.reason` and `restsOn: "controller-spec"`.

---

## 3. A(3): Unrecorded Changes Single Answer

### Proposed surface
Add `--view unrecorded` to `cub scout compare three-way --format json` (and MCP `compare_three_way` with `view: "unrecorded"`), mirroring the `--view summary` pattern already shipped on `gitops settings`, `gitops status`, and `doctor`.

### Behavior
- Runs the same read-only three-way collection (`--scope` or `--dry-from`), then filters the JSON payload to **only** resources/fields present in live state that are unrecorded in intended/recorded state (`unrecorded[]`), preserving:
  - `answer` block (exact same envelope contract, bounded under a declared CI ceiling),
  - `scope` / `context`,
  - `counts` (`total`, `unrecorded`, `omitted`),
  - `omissions[]` (so RBAC denials or unreadable kinds are never misread as "zero unrecorded changes").
- Drops the full matched/in-sync resource list so an agent pays only for the unrecorded set.

---

## 4. Decisions Needed from Maintainer

1. **Timing:** Land A(2) and A(3) in `v3.0.0` before tagging, or ship `v3.0.0` with what is on `main` now and land A(2) + A(3) in `v3.0.1` / `v3.1.0`?
2. **A(2) surface:** Approve extending `explain --field <path>` (with `value` + `revertPrediction`, zero new `answer.state` words), or prefer a separate command?
3. **A(3) surface:** Approve `compare three-way --view unrecorded` (reusing the `--view` convention), or prefer a standalone `compare unrecorded` subcommand?
