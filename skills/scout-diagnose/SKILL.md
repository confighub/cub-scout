---
name: scout-diagnose
description: 'Use when the user wants cub-scout to INTERPRET what it sees and suggest a next step — the Diagnose verb group. Natural phrasing: "explain this Deployment", "why is this broken?", "what should I do about this?", "is this drift or just slow sync?", "what does this error mean?", "is there a known pattern for this?", "is my GitOps pipeline healthy?", "what events are firing on this Pod?", "give me a suggested remedy for this finding". Load whenever intent is explain / why / what-does-it-mean / what-should-I-do / interpret / diagnose / recommend / suggest-fix over a Kubernetes cluster or a scan finding. Do NOT load for: pure cluster inventory (use scout-observe), comparing intended vs running state at the field level (use scout-compare), field-level provenance (use scout-attribute), or actually applying a fix (cub-scout never mutates — route to cub or kubectl with the user driving).'
phase: verify
allowed-tools: Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout debug *) Bash(cub-scout debug *) Bash(cub scout debug *) Bash(./cub-scout suggest-remedy *) Bash(cub-scout suggest-remedy *) Bash(cub scout suggest-remedy *) Bash(./cub-scout patterns *) Bash(cub-scout patterns *) Bash(cub scout patterns *) Bash(./cub-scout gitops status *) Bash(cub-scout gitops status *) Bash(cub scout gitops status *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl logs *) Bash(kubectl get events *) Bash(argocd app get *) Bash(flux get *)
---

# scout-diagnose

The Diagnose verb group of cub-scout. Five commands that interpret what's in the cluster and produce next-step guidance — without applying any fix.

## When to use

Explicit phrasings:

- "Explain this Deployment" / "what does this resource mean?"
- "Why is my Pod CrashLooping?" / "what's wrong with this StatefulSet?"
- "What should I do about this?" / "give me a remediation"
- "Is this drift, or just controller still syncing?"
- "Is there a known pattern that matches this?"
- "Is my GitOps pipeline healthy?" / "what's the state of Flux / Argo?"
- "Show me the recent events on this resource"
- "Suggest a remedy for this scan finding"

Implicit intents:

- The user has *already seen* something (via [`scout-observe`](../scout-observe/SKILL.md)) and wants to know *why* it looks that way
- The user wants a concrete next command to run, not just facts
- The user wants pattern-matching against known failure modes, not just raw cluster output

## Do not load for

- Pure inventory ("show me what's running") — load [`scout-observe`](../scout-observe/SKILL.md)
- Comparing intended vs running state at the field level — load [`scout-compare`](../scout-compare/SKILL.md)
- Per-field provenance (which controller / which git file set this value) — load [`scout-attribute`](../scout-attribute/SKILL.md)
- "Apply this fix" / "edit / patch / sync" — cub-scout never mutates. `suggest-remedy` describes a fix; it does not execute it. Route to `cub`, `kubectl` (user driven), or another tool

## Standalone vs connected

- **Standalone (cluster only):** `explain`, `debug`, `suggest-remedy`, `patterns`, `gitops status` all work against your current kubectl context.
- **Connected (cluster + `cub auth login`):** `explain` adds ConfigHub deep-links to the unit / revision URLs the resource came from; three-way disagreement gets a dedicated section.
- **Offline (file / bundle only):** `patterns detect --file <yaml>` runs the pattern engine against a manifest; `suggest-remedy` works against scan findings without a live cluster.

## Tool boundary

- **Allowed (read-only):** all five Diagnose verbs, plus `kubectl get/describe/logs/get events`, `argocd app get`, `flux get`. These are the read-only diagnostic surfaces the skill may invoke for context.
- **Not allowed:** `kubectl apply/edit/patch/delete/exec`, `argocd app sync/refresh` (as mutation), `flux reconcile` (mutating), `cub * create/update/delete`, any `--dry-run=false` carve-out.
- **`suggest-remedy` is read-only:** the original command was renamed from `remedy` in #428; the mutating executor was deleted. The command **describes** a fix that would resolve the finding — it does not run it. cub-scout no longer has a non-dry-run path.

## The verb menu

| Question | Verb | Notes |
|---|---|---|
| "Explain this one resource" | `cub-scout explain <kind>/<name> -n <ns>` | Plain-English ownership + lineage + health + drift + next steps. Supports `--presentation human\|ai\|paired` and `--hint-mode default\|beginner\|operator`. |
| "Walk me through a GitOps debug" | `cub-scout debug` | Guided wizard for the common failure modes |
| "Suggest a fix for this scan finding" | `cub-scout suggest-remedy <finding-id>` | **Read-only** — describes the patch, does not apply. Legacy `remedy` accepted as an alias. |
| "Match the cluster against known patterns" | `cub-scout patterns detect` / `patterns explain` / `patterns list` | The pattern engine. Works on cluster, file, or bundle. |
| "Is GitOps healthy?" | `cub-scout gitops status` | Pipeline health for Flux + Argo: stuck reconciles, source errors, applyset drift |

All five verbs emit `--format json` for agents.

## The loop

1. **Identify intent.** The user is starting from a *symptom* (broken resource, scan finding, slow sync) and wants an interpretation + a recommended next step.
2. **Gather context.** If the user names a specific resource, lead with `cub-scout explain <kind>/<name> -n <ns>`. If they have a scan finding ID, lead with `cub-scout suggest-remedy <id>`. If they're at "something feels off across the cluster", lead with `cub-scout doctor` (from [`scout-observe`](../scout-observe/SKILL.md)) and then drill in.
3. **Read the structured hints.** `explain` and `doctor` both emit `nextSteps[]` — each hint has an `actionType` (read-only / waiting / human-decision; mutating is rejected on emit) and a `nextCommand`. Use those hints to drive the next invocation.
4. **Pattern-match if needed.** If the symptom is well-known (CrashLoop, image pull error, kustomization unhealthy), `cub-scout patterns detect` produces a structured match against the 46 built-in patterns.
5. **Hand off.** When the diagnosis points at a *change* the user wants to make, refuse to act and route them: ConfigHub-authored fixes go to `cub` skills in [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills); direct manifest fixes go to `kubectl edit` with the user driving; if the divergence might be drift vs reconciliation, route to [`scout-compare`](../scout-compare/SKILL.md) for the connected three-way view.

## Worked example

User: *"deploy/api in prod is CrashLooping. What's going on?"*

### Standalone mode (cluster only)

```bash
$ cub-scout explain deploy/api -n prod --hint-mode operator
Deployment/api in prod:
  Owner:        Argo CD (Application: payments-api, source: https://github.com/org/repo @ apps/prod/api)
  Source:       Git
  Deployed via: argocd-controller
  Health:       Degraded (1/3 replicas Ready)
  Risks:        CrashLoopBackOff on container "api" (3 restarts in last 5m)
  Drift:        Not checked (standalone mode)
  Notes:
    - Recent events: Warning BackOff (3x, 5m) kubectl: Back-off restarting failed container

Try next (read-only):
  → cub-scout patterns detect --resource Deployment/api -n prod
  → kubectl logs -n prod deploy/api --previous --tail=100
```

The user can follow the structured hints — both read-only — without ever needing to apply, edit, or sync.

### Connected mode adds three-way disagreement detail

When `cub auth login` is set up, the same `explain` command also includes a `THREE-WAY STATUS` block:

```text
THREE-WAY STATUS (connected)
  ConfigHub revision 42 → controller revision 42 → cluster revision 42 (agreed)
  Conclusion: not a drift problem. Investigate the workload itself.
```

This disambiguates "is this CrashLoop a workload bug, or is the controller still rolling out an old revision?" — connected mode tells you the revisions agree, so the failure is in the workload, not the GitOps loop.

For agents:

```bash
$ cub-scout explain deploy/api -n prod --format json | jq '.nextSteps[]'
{
  "actionType": "read-only",
  "reason": "CrashLoop suggests application-level failure, not drift; inspect logs",
  "nextCommand": "kubectl logs -n prod deploy/api --previous --tail=100",
  "nextSurface": "kubectl"
}
```

## Output evidence

- **Primary artifact:** structured explanation JSON or human-readable ASCII
- **`nextSteps[]`** with `actionType` ∈ {`read-only`, `waiting`, `human-decision`}. Mutating actionType is rejected at every cub-scout emit point.
- **Three-way disagreement section** (connected mode only) when ConfigHub / controller / cluster disagree

## References

- Capability map: [`README.md`](../../README.md#capability-map) § Diagnose
- Command details: [`docs/reference/commands.md`](../../docs/reference/commands.md)
- Phase-aware hint design: #367 (Argo phase hints), #370 (structured `nextSteps`)
- Triad lock (read-only `suggest-remedy`): #410 / #428

## Constraints

- `suggest-remedy` describes; it does not apply. If a user asks to *run* the suggested fix, refuse and route them to the appropriate skill (`cub` for ConfigHub-authored changes; `kubectl` with the user driving for direct edits). The original `remedy` executor was deleted in #428.
- Hint `actionType: mutating` is structurally impossible — the emitter rejects it. Any "next step" from this skill is safe to read and follow without risk of accidental mutation.
- If the diagnosis is *evidence-missing* (no managedFields, no controller spec, no ConfigHub link), `explain` returns the evidence gap honestly rather than guessing. Don't synthesize a recommendation from absence.
