# ConfigHub Initiatives integration: design (held pending backend surface)

This document is the design spec for cub-scout's planned integration with
ConfigHub Initiatives. It exists so the day ConfigHub exposes Initiative
as an addressable primitive, cub-scout has an agreed-on plan rather than
a fresh discussion.

**Status: held.** The prerequisite has not shipped on the ConfigHub side
as of 2026-05-08. See [Why this is deferred](#why-this-is-deferred) below
and [confighub/cub-scout#392](https://github.com/confighub/cub-scout/issues/392)
for the tracking issue.

## Background

ConfigHub Initiatives are operator-curated, prioritised policy goals
("Pin Model Versions", "GPU Resource Limits", "Guardrail Policy
Required") visible in the ConfigHub web UI under `/initiatives`.

Each Initiative is a UI composite of three backend primitives:

- A **Filter** — selects the unit set the policy applies to (`Where`,
  `WhereData`, `ResourceType` clauses)
- A **View** — display + a stable annotation `initiative-check-summary`
  carrying passing / failing / notApplicable counts (capped at 1024
  chars)
- A **Trigger** — embedded Kyverno policy YAML; `Trigger.Warn=false`
  means blocking ApplyGate, `Trigger.Warn=true` means non-blocking
  ApplyWarning. Per-unit results come from `FunctionInvocationsResponse`.

The UI assembles the Initiative shape via `useInitiatives()` from the
underlying `useGetTriggerQuery`, `useDeleteFilterMutation`, and
`useDeleteViewMutation` hooks. It is reconstructed at render time from
the three persistent primitives.

See [`ui/src/types/initiative.ts:13-84`](https://github.com/confighubai/confighub/blob/main/ui/src/types/initiative.ts) on the ConfigHub side for the canonical UI type shape.

## Why this is deferred

The council ruling on cub-scout #393 ("evidence provider, not judge")
explicitly forbade reverse-engineering the UI composite from cub-scout:

> Reverse-engineering the UI composite from cub-scout would create a
> parallel implementation that drifts the moment ConfigHub picks a
> canonical surface.

cub-scout could technically resolve an Initiative today by chaining
three calls — `cub view get`, `cub` filter resolve, and reading the
trigger annotation — and assembling the shape itself. We don't, because:

1. The drift risk is real. ConfigHub UI work is active (e.g. the recent
   `confighub#4244` "Track initiative runs as ChangeSets" added 12,000
   lines, all UI). When the team picks a canonical Initiative shape
   server-side, any cub-scout-side reconstruction will diverge in the
   first month.
2. The contract is acceptance-bearing. Pilot's compliance verdicts
   pivot on Initiative shape; a transitively-defined source of truth
   undermines the architectural triad we just locked in #393.

So cub-scout waits for one of two prerequisite paths:

### Path 1 (preferred): Initiative as a backend primitive

`internal/models/initiative.go` with the field shape mirroring the UI's
current `Initiative` type, plus standard CRUD HTTP endpoints (`GET
/api/space/{space}/initiative`, etc.). Followed by a corresponding
`sdk/cmd/cub/initiative_*.go` set so `cub initiative list|get|create`
exists as a first-class CLI surface.

### Path 2 (also acceptable): `cub initiative get --resolve` helper

Keep Initiative as a UI composite, but ship a `cub` CLI helper that
returns the resolved bundle (filter clauses + view metadata + trigger
+ Kyverno YAML + per-unit results) for a given Initiative ID. Cheaper
to build than Path 1, but every consumer reimplements its own composite
around it.

When either path lands, the integration described below is what
cub-scout ships.

## Planned implementation (when prerequisite is ready)

The shape mirrors cub-scout's existing Views integration pattern (#391)
so operators see a consistent CLI surface across ConfigHub primitives.

### CLI surface

```bash
# URL-as-positional, mirroring `views resolve`
cub-scout initiatives resolve <uuid-or-url>

# Concrete examples
cub-scout initiatives resolve 4f8d9c-2a1b-...
cub-scout initiatives resolve "https://hub.confighub.com/initiatives/<uuid>"
```

The same parser pattern from `pkg/agent/view_ref.go::ParseViewRef`
applies here. New file: `pkg/agent/initiative_ref.go` with a parallel
`ParseInitiativeRef`. URL parser accepts the canonical
`https://hub.confighub.com/initiatives/<uuid>` shape and any future
extras (`?priority=HIGH`, etc.).

### JSON contract

```json
{
  "uuid": "...",
  "source_form": "url" | "uuid",
  "original_url": "...",
  "extras": { ... },
  "initiative": {
    "name": "Pin Model Versions",
    "priority": "HIGH",
    "status": "in_progress",
    "filter": { ... },
    "kyverno_policy": "...",
    "enforced": true,
    "check_summary": {
      "passing": 8,
      "failing": 2,
      "notApplicable": 1,
      "total": 11,
      "checkedAt": "2026-05-08T12:34:56Z"
    }
  }
}
```

Shape mirrors the UI's `Initiative` type so cross-repo diffs are
trivial. cub-scout does not transform — it surfaces the resolved bundle
and lets Pilot judge.

### Compliance overlay (#391-style scope item)

Once the resolver lands, the compliance overlay composes with the
source-truth evidence already shipped in #393. For each unit selected
by an Initiative's filter, cub-scout joins:

- **ConfigHub policy verdict** — from `UnitCheckResult.success` /
  `violations` (read from the trigger's invocation results)
- **Live cluster presence** — applied? orphan? missing?
- **Enforcement reality** — was the trigger blocking
  (`Warn=false`) at apply time, and did the running resource come
  from a passing or failing check?

The overlay verdict is per-unit, with a stable JSON shape:

```json
{
  "unit": "rag-server",
  "initiative_compliance": "PASSING|FAILING|NOT_APPLICABLE",
  "live_presence": "applied|orphan|missing",
  "enforcement": "blocking|non-blocking|none",
  "reality_verdict": "consistent|gap|undetermined"
}
```

The `reality_verdict` field captures the council's framing: ConfigHub
says X, runtime says Y, here's whether they agree.

### Strict rules

The same rules locked into #393 apply here, in code:

1. **No silent fallback.** If `cub initiative get` (or whatever the
   prerequisite ships) returns an error, cub-scout emits `BLOCK` /
   `BLOCKED` evidence. It does not reverse-engineer the bundle from
   underlying primitives just because the resolver is missing.
2. **Missing proof never produces PASSING.** If a per-unit result is
   absent, the compliance verdict is `NOT_APPLICABLE` or
   `undetermined` — never silently passing.
3. **Connected-mode required.** Initiatives are inherently a
   ConfigHub surface; offline mode refuses.

### Out of scope (initial Initiative integration)

- Authoring or editing Initiatives from cub-scout. Read-only contract
  holds. Pilot does not author Initiatives either; ConfigHub UI does.
- Running Kyverno policies inside cub-scout. cub-scout consumes results,
  ConfigHub computes them.
- Multi-source / ApplicationSet child resolution. Handled by the View
  filter (since Initiative wraps a Filter).

## How to know when this design unblocks

The prerequisite has shipped when **all** of the following are true:

1. `find /Users/alexis/code/confighub/internal -iname '*initiative*' -path '*models*'` returns at least one Go file (Path 1)
   **— OR —**
   `find /Users/alexis/code/sdk/cmd/cub -iname 'initiative*'` returns CLI files (Path 2 via SDK)
2. `cub initiative` (or the equivalent helper) responds to `--help`
   without error
3. The shape returned by `cub initiative get <uuid> -o json` is
   documented or self-evidently stable (round-trips, has matching field
   names with the UI Initiative type)

When all three hold, this document graduates from "design held" to
"design in progress", and cub-scout #392 reopens for the
implementation PRs.

## Linked issues and references

- [confighub/cub-scout#392](https://github.com/confighub/cub-scout/issues/392) — tracking issue
- [confighub/cub-scout#391](https://github.com/confighub/cub-scout/issues/391) — Views integration that this mirrors
- [confighub/cub-scout#393](https://github.com/confighub/cub-scout/issues/393) — source-truth contract that compliance overlay composes with
- [confighubai/confighub-ai-demo#264](https://github.com/confighubai/confighub-ai-demo/issues/264) — Pilot consumer-side fixtures
- Council verdict on #393 capturing the evidence-vs-judge split: [#393#issuecomment-4407651183](https://github.com/confighub/cub-scout/issues/393#issuecomment-4407651183)
