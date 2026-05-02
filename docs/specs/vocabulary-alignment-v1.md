# Vocabulary Alignment v1: Component / Deployable Variant / Base Variant / Target

**Status:** Specification, not implemented
**Owner (proposed):** Brian (cub-scout import path)
**Tracks:** Track 1 of the [import plan](cubscout-import-plan.md)
**Related:** [pattern-1-takeover-v1.md](pattern-1-takeover-v1.md)

---

## What this is

The cub-scout import path emits ConfigHub proposals using App-centric
language: `App`, `Deployment`, `Unit`, `Space`. ConfigHub PR #104 (merged
2026-04-30) and issue confighubai/confighub#2985 (closed 2026-05-01) settle
the user-visible model on **Component → Deployable Variant → Target**, with
**Base Variant** for non-deployable templates and **Connection** for typed
inter-Component contracts.

This spec describes what cub-scout has to change so its output matches the
new doctrine. It is a specification engineering reads to write the code.

---

## Doctrinal mapping

| Old (cub-scout today) | New (doctrine) | Notes |
|-----------------------|----------------|-------|
| `App` | **Component** | The logical software family. VariantSet in API. |
| `App's base template` | **Base Variant** | Non-deployable, holds placeholders. |
| `Deployment` | **Deployable Variant** | A Variant bound to a Target. |
| `Unit` | (no direct rename) | Unit remains the API primitive; Variants are Spaces with `Labels.Variant` containing Units. |
| `Space` (per-team workspace) | (deprecated framing) | Spaces still exist as the API primitive, but they are now per-Variant, not per-team. |
| `variant=prod` label | `Labels.Variant` | Variant is now a first-class Space label, not a Unit overlay tag. |
| `Target` | **Target** | Unchanged. |
| (new) | **Connection** | Typed contract for inter-Component dependencies. v1 produces a draft from discovery (see [Track 3](cubscout-import-plan.md#track-3--connection-draft-from-import)). |

The mapping is mostly a rename plus one structural shift: variant moves
from a Unit label to a Space label (`Labels.Variant`) per the v0.1.36
implementation.

---

## What changes in cub-scout's output

### 1. JSON schema (`cub-scout import --json`)

Today's `ImportResult` (cmd/cub-scout/import.go:101-136) produces:

```json
{
  "namespace": "payments",
  "model": "app-centric",
  "workloads": [...],
  "suggestion": {
    "app": "payments",
    "units": [
      {"slug": "api-prod", "app": "payments", "variant": "prod", "workloads": [...]}
    ]
  }
}
```

After v1, the same input emits:

```json
{
  "namespace": "payments",
  "model": "component-variant",
  "workloads": [...],
  "suggestion": {
    "component": "payments",
    "baseVariant": {
      "slug": "payments-base",
      "labels": {"Variant": "base"}
    },
    "deployableVariants": [
      {
        "slug": "payments-prod",
        "component": "payments",
        "labels": {"Variant": "prod"},
        "target": {"slug": "prod-east-cluster", "providerType": "kubernetes"},
        "units": [
          {"slug": "api", "workloads": ["Deployment/api"]},
          {"slug": "worker", "workloads": ["Deployment/worker"]}
        ]
      }
    ],
    "connections": {
      "draft": [
        {"kind": "Secret", "name": "db-creds", "needsTyping": true, "discoveredVia": "trace"},
        {"kind": "ConfigMap", "name": "feature-flags", "needsTyping": true, "discoveredVia": "selects"}
      ]
    }
  }
}
```

Field-level changes:

- `model`: `"app-centric"` → `"component-variant"`.
- `suggestion.app` → `suggestion.component`.
- `suggestion.units` (flat list) → `suggestion.baseVariant` + `suggestion.deployableVariants[]`, each Variant carrying its own units.
- New: `suggestion.connections.draft[]` listing discovered dependencies.
  Each entry has `kind`, `name`, `discoveredVia` (one of `trace`, `selects`,
  `mounts`, `references`, `owns`), and `needsTyping: true` until the
  Connection v1 spec lands.
- `WorkloadJSON.connected` and `WorkloadJSON.unitSlug` retain their meaning.

The Go types in `cmd/cub-scout/import.go` that need to change:

- `ImportResult` (line 101)
- `SuggestionJSON` (line 124) — rename `App` to `Component`, replace `Units` with `BaseVariant` + `DeployableVariants`.
- `UnitJSON` (line 130) — split: keep a leaner `UnitJSON` for workload-to-unit mapping; introduce `VariantJSON` and `BaseVariantJSON` types.

### 2. Human-readable output

Where today the wizard and `--dry-run` output say:

```
Suggested App: payments
  Deployment: payments-prod (variant=prod)
    Unit: api  ← Deployment/api
    Unit: worker ← Deployment/worker
```

After v1:

```
Suggested Component: payments
  Base Variant: payments-base (Labels.Variant=base, non-deployable)
  Deployable Variant: payments-prod (Labels.Variant=prod) → Target: prod-east-cluster
    Unit: api ← Deployment/api
    Unit: worker ← Deployment/worker
  Draft Connections (3):
    Secret/db-creds (via trace, needs typing)
    ConfigMap/feature-flags (via selects, needs typing)
    ServiceAccount/payments-sa (via references, needs typing)
```

The wizard prompts at `cmd/cub-scout/import_wizard.go` need parallel
updates: any prompt that says "App" becomes "Component"; "Deployment"
becomes "Deployable Variant"; "base template" becomes "Base Variant".

### 3. ConfigHub state created on `--connect`

Today, `--connect` delegates to `cub gitops import` and the resulting
ConfigHub state uses Space + Unit primitives with `app=<name>` and
`variant=<env>` Unit labels.

After v1, the same path creates:

- One Space per Variant, with `Labels.Variant` set on the Space, not the Unit.
- A Base Variant Space with `Labels.Variant=base` and `Labels.Component=<name>`.
- One Deployable Variant Space per discovered environment, with the
  Variant's Units inside and the Target binding on the Space.
- `Labels.Component=<name>` consistent across the family so a Component
  query (`cub space list -l Component=<name>`) returns all Variants.

This is a `cub` CLI capability question, not a cub-scout one — cub-scout
emits the desired state and `cub gitops import` (or the eventual
`cub component create` / `cub variant create` commands) creates it.
Coordinate with the ConfigHub side (Track 1, Brian) before shipping.

### 4. Trust URLs (the v1.13 canonical URL set)

`cmd/cub-scout/confighub_urls.go` currently emits:

```
https://hub.confighub.com/units/<spaceID>/<unitID>
https://hub.confighub.com/units/<spaceID>/<unitID>?tab=2
```

These are Unit-detail URLs and remain valid for Unit-level navigation.
The Promotions UI in v0.1.36 introduces Component- and Variant-level
views. Add (do not replace):

- `configHubComponentURL(orgID, componentSlug)` →
  `${WebBaseURL}/components/<orgID>/<componentSlug>`
- `configHubVariantURL(spaceID, variantSlug)` →
  `${WebBaseURL}/variants/<spaceID>/<variantSlug>`
- `configHubBaseVariantURL(spaceID, variantSlug)` → same path with a
  `?kind=base` query string (or whatever the Promotions UI chose).

The exact path strings have to be confirmed against the live Promotions
UI before code lands. Mark this as an open question (Q-URL-1 below).

Trust-URL emission already happens in `compare`, `trace`, `explain`,
`history`, MCP, and the import next-step hints. All six surfaces need
to learn the new URLs alongside the existing Unit URLs.

### 5. MCP tool output

The `compare_three_way`, `import`, and `explain` MCP tool wrappers should
emit the `component`, `baseVariant`, `deployableVariants`, and `connections`
fields in the same shape as the JSON output above. Update the MCP schema
files so AI clients consuming cub-scout get the new vocabulary
out-of-the-box.

---

## Migration path for users with existing scripts

There are users with scripts that parse `cub-scout import --json` output
keyed on `suggestion.app` and `suggestion.units[]`. A flag-day rename
will break them silently (the field will simply be missing).

**Proposal: dual-emit for two minor releases, then deprecate.**

- v1.14: emit *both* the new fields (`component`, `baseVariant`,
  `deployableVariants`, `connections`) and the legacy fields
  (`app`, `units`). Set `model: "component-variant"`. Document the
  legacy fields as deprecated.
- v1.15: emit only the new fields. `model: "component-variant"`.
  A `--legacy-app-output` flag (off by default) re-enables dual-emit
  for one more release as an escape hatch.
- v1.16: remove `--legacy-app-output`. Old scripts must migrate.

Human-readable output flips immediately at v1.14. Only the JSON contract
gets the deprecation runway, since human output is not parsed.

---

## Code locations that change

| File | Change |
|------|--------|
| `cmd/cub-scout/import.go:101` | `ImportResult.Model` value to `"component-variant"`. |
| `cmd/cub-scout/import.go:124` | `SuggestionJSON` rename + restructure. |
| `cmd/cub-scout/import.go:130` | `UnitJSON` slimmed; new `VariantJSON`, `BaseVariantJSON`, `ConnectionDraftJSON`. |
| `cmd/cub-scout/import.go` (emit) | All printf statements on the human path. Search for `"App:"`, `"Deployment:"`, `"Unit:"`. |
| `cmd/cub-scout/import_wizard.go` | Prompt strings + confirmation text. |
| `cmd/cub-scout/import_argocd.go` | Argo-specific labels emitted onto ConfigHub Spaces/Units; ensure `Labels.Component` and `Labels.Variant` appear on the Space. |
| `cmd/cub-scout/confighub_urls.go` | Add `configHubComponentURL`, `configHubVariantURL`, `configHubBaseVariantURL`. |
| `cmd/cub-scout/import.go` (hint emission) | Include Component / Variant URLs in next-step hints. |
| `cmd/cub-scout/compare_three_way.go`, `cmd/cub-scout/trace*.go`, `cmd/cub-scout/explain*.go`, `cmd/cub-scout/history*.go`, MCP tool wrappers | Trust URL emission. |
| `cmd/cub-scout/import_*_test.go`, `import_suggest_golden_test.go` | Golden files updated. |
| `docs/reference/json-contracts.md` | Document the new schema; mark legacy fields deprecated. |
| `docs/reference/glossary.md` | Vocabulary swap (covered separately in Pass 3). |

The `pkg/agent/` discovery layer does not need vocabulary changes —
it surfaces workloads and ownership. Vocabulary lives in the import
output layer only.

---

## Open questions

- **Q-URL-1.** What are the exact path strings for Component, Variant,
  and Base Variant detail pages in the Promotions UI? Owner: Brian
  (cross-repo coordination with the GUI team).
- **Q-LABELS-1.** When a Component has many Variants, do the labels
  on each Space repeat the Component name, or is there a parent-Space
  primitive? `cub space list -l Component=foo` works either way; the
  question is whether to also emit a `Component` Space. Owner: Jesper.
- **Q-CONN-1.** Does the Connection v1 spec land before this work
  ships? If yes, Track 3 produces typed Connections. If no, Track 3
  produces `connections.draft[]` with `needsTyping: true` and the
  typing pass is added later. Owner: Alexis (drives the spec ask).
- **Q-MIGRATION-1.** Two minor-release deprecation runway is the
  proposal. Is two enough, or does the customer install base have
  enough import-output-parsing scripts that we want three? Owner:
  Brian, with input from product.
- **Q-BASE-1.** Today the import path infers a single `App` per
  namespace. Does it also infer a Base Variant? In the simplest case
  (one namespace, one prod environment), there is no obvious Base —
  the Deployable Variant *is* the only render.

  Per Jesper (2026-05-02), the system distinguishes Base from
  Deployable by *presence of a Target*: no Target = Base. The spec
  therefore proposes emitting a Base Variant for every Component
  anyway, populated from the discovered render with literal values
  in place of placeholders, so a future clone-adapt-apply flow can
  promote against it. Confirm shape with Brian; the underlying model
  is settled.

  Open downstream question: when a customer eventually clones an
  imported Deployable Variant to a new environment, the "adapting"
  step (replace placeholders, clear `vet-placeholders` trigger) is
  not yet a first-class flow. Out of v1 onboarding scope. Brian and
  Jesper own the adaptation question on a separate track; this plan
  does not absorb it.

---

## Out of scope for this spec

- Writing the implementation code.
- The `cub`-side commands that materialise Components and Variants.
  This spec lists what cub-scout *emits*; what `cub` does with that
  emission is a ConfigHub product question.
- Changing the GSF schema. GSF v1 relations (`owns`, `selects`,
  `mounts`, `references`) feed the Connection draft but the schema
  itself is unaffected.
- AI Variant emission. Onboarding produces Deployable Variants, not
  AI Variants.

---

## Done means

- `cub-scout import -n <ns> --dry-run --json` emits the new schema.
- Human output uses Component / Base Variant / Deployable Variant
  consistently.
- `--connect` creates ConfigHub state under the new doctrine, with
  `Labels.Component` and `Labels.Variant` set on Spaces.
- Trust URLs cover Component, Variant, and Base Variant detail pages.
- Golden tests updated.
- Two-release deprecation runway for legacy JSON fields documented in
  `docs/reference/json-contracts.md` and the release notes.
- The glossary, import-to-confighub, and migration-playbook docs no
  longer carry "transitioning" caveats (Pass 3 of this session).
