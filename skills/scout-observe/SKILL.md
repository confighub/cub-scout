---
name: scout-observe
description: 'Use when the user wants to SEE what is running in a Kubernetes cluster, who owns it, and how it is structured — the read-only Observe verb group of cub-scout. Natural phrasing: "what is running in my cluster?", "give me a map of resources", "who owns this Deployment?", "trace this back to its source", "show me the ownership tree", "scan this cluster for risk", "dump the cluster as JSON", "stream cluster events", "what is the connection status?", "is this cluster healthy?". Load when intent is observe / inventory / map / show / list / trace / scan / dump / stream / health-check over a Kubernetes cluster. Do NOT load for: explaining WHY something is broken (use scout-diagnose), comparing cluster state to intended state (use scout-compare), provenance of individual field values (use scout-attribute), or any action that mutates the cluster (cub-scout never mutates — route to cub or kubectl with the user driving).'
phase: verify
allowed-tools: Bash(./cub-scout doctor) Bash(./cub-scout doctor *) Bash(cub-scout doctor) Bash(cub-scout doctor *) Bash(cub scout doctor) Bash(cub scout doctor *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout tree *) Bash(cub-scout tree *) Bash(cub scout tree *) Bash(./cub-scout scan *) Bash(cub-scout scan *) Bash(cub scout scan *) Bash(./cub-scout graph *) Bash(cub-scout graph *) Bash(cub scout graph *) Bash(./cub-scout snapshot *) Bash(cub-scout snapshot *) Bash(cub scout snapshot *) Bash(./cub-scout watch *) Bash(cub-scout watch *) Bash(cub scout watch *) Bash(./cub-scout status) Bash(./cub-scout status *) Bash(cub-scout status) Bash(cub-scout status *) Bash(cub scout status) Bash(cub scout status *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl logs *) Bash(kubectl version) Bash(kubectl config current-context) Bash(kubectl config view *)
---

# scout-observe

The Observe verb group of cub-scout. Nine commands that show what is running in a cluster and who owns it — without modifying anything.

## When to use

Explicit phrasings:

- "What is running in my cluster?" / "show me the Deployments in namespace prod"
- "Give me a map of ownership" / "who manages this resource?"
- "Trace deploy/api back to its Git source" / "what created this?"
- "Show me the ownership tree" / "render the resource hierarchy"
- "Scan this cluster for risk" / "scan this manifest file"
- "Dump cluster state as JSON" / "snapshot the namespace"
- "Stream cluster events to a webhook"
- "What's my connection status?" / "am I connected to ConfigHub?"
- "Is this cluster healthy?" / "give me a one-screen status"

Implicit intents the skill should catch:

- The user wants a *picture* of the cluster, not a diagnosis or remediation
- The user wants a stable JSON / ASCII artifact (for an agent, an audit, or a pipeline)
- The user is starting a triage and needs to know what they're looking at before going deeper

## Do not load for

- "Why is this broken?" — load [`scout-diagnose`](../scout-diagnose/SKILL.md) instead
- "Does the cluster match what Git says?" — load [`scout-compare`](../scout-compare/SKILL.md)
- "Where did this field's value come from?" — load [`scout-attribute`](../scout-attribute/SKILL.md)
- "Fix this" / "apply this" / "delete this" — cub-scout cannot mutate; route to `cub`, `kubectl` (user driven), or another tool

## Standalone vs connected

- **Standalone (cluster only):** all nine Observe verbs work. `doctor`, `map`, `trace`, `tree`, `scan`, `graph`, `snapshot`, `watch`, `status` need only your current kubectl context.
- **Connected (cluster + `cub auth login`):** `trace` and `map` gain ConfigHub unit linkage; `status` shows ConfigHub session detail; `scan` can correlate findings with units.
- **Offline (file / bundle only):** `scan --file <path>` audits manifests without a cluster; `bundle inspect` reads recorded debug bundles.

## Tool boundary

- **Allowed (read-only):** all `cub-scout *` / `cub scout *` Observe verbs, `kubectl get/describe/logs`, `kubectl version`, `kubectl config current-context/view`.
- **Not allowed:** `kubectl apply/edit/patch/delete/scale/rollout/exec`, `kubectl debug` (privileged debug pods), any `cub * create/update/delete`.
- **Boundary with [`scout-diagnose`](../scout-diagnose/SKILL.md):** Observe shows *what* is there; Diagnose interprets *why* it looks the way it does and recommends *next steps*. The two often chain: Observe to map, then Diagnose to interpret.

## The verb menu

Pick by the question being asked:

| Question | Verb | Notes |
|---|---|---|
| "Is this cluster healthy at a glance?" | `cub-scout doctor` | One-screen health summary, with `--presentation human\|ai\|paired` and `--hint-mode default\|beginner\|operator` |
| "What's in this cluster, grouped by ownership?" | `cub-scout map` (interactive TUI) | Sub-commands: `map list`, `map hooks`, `map orphans`, `map meaning` |
| "Trace this workload back to its Git / OCI source" | `cub-scout trace <kind>/<name> -n <ns>` | Walks Deployment → Application → Git or Deployment → HelmRelease → GitRepository |
| "Show me the runtime + ownership + git hierarchy" | `cub-scout tree` | Multi-axis composition view |
| "Scan for risk patterns" | `cub-scout scan` | 46 built-in patterns. Variants: `--state`, `--kyverno`, `--lifecycle-hazards`, `--timing-bombs`, `--dangling`, `--file <yaml>` |
| "Export the resource graph" | `cub-scout graph export` | DOT / JSON for downstream tools |
| "Dump the cluster as GSF JSON" | `cub-scout snapshot` | Stable snapshot for AI / agent / audit |
| "Stream observation events" | `cub-scout watch -n <ns> [--webhook url]` | Webhook / file sinks, namespace / owner / severity filters |
| "What mode am I in? Connection / cluster info" | `cub-scout status` | Standalone vs connected, context, cluster details |

All nine verbs emit deterministic JSON via `--format json` for agents.

## The loop

1. **Identify intent.** Translate the user's question into one of the rows in the verb menu above. If multiple verbs could answer, prefer the most specific (`trace` over `map` if the question is about one resource's lineage).
2. **Pick the format.** Humans: ASCII (the default). Agents / CI / MCP: `--format json`.
3. **Invoke.** Use `./cub-scout ...` in the local repo; `cub-scout ...` standalone; `cub scout ...` plugin-form. All three accept the same flags.
4. **Interpret.** Read the structured output:
   - `doctor` → top-level health, top issues, next-step hints
   - `map` → resource list with owner + namespace + health columns
   - `trace` → ownership chain (controller → source → repository → commit)
   - `scan` → findings list with severity + pattern + resource
   - `snapshot` / `graph export` → bulk JSON for downstream tooling
5. **Hand off.** If the user needs a *why* or a *fix*, route to [`scout-diagnose`](../scout-diagnose/SKILL.md). If they need to compare against intent, route to [`scout-compare`](../scout-compare/SKILL.md). If they need to act, refuse and route to `cub` skills or `kubectl` with the user driving.

## Worked example

User: *"Map the boutique namespace and show me which Deployments are managed by Argo."*

```bash
$ cub-scout map list -n boutique --format ascii
NAME              KIND          OWNER     SOURCE                                      HEALTH
frontend          Deployment    Argo CD   apps/prod/boutique/frontend                Ready
catalog           Deployment    Argo CD   apps/prod/boutique/catalog                 Ready
checkout          Deployment    Flux      flux-system/Kustomization/boutique-checkout Ready
redis             StatefulSet   Native    -                                           Ready
```

Read off: 2 Argo-managed Deployments, 1 Flux-managed, 1 native (no GitOps controller). The output is enough for the user to drill into any single resource with `trace deploy/frontend -n boutique` (which is still in scope for this skill) or jump to `scout-diagnose` if they want to know *why* a status is what it is.

For agents:

```bash
$ cub-scout map list -n boutique --format json | jq '[.[] | {name, owner, source}]'
[
  {"name": "frontend", "owner": "Argo CD", "source": "apps/prod/boutique/frontend"},
  {"name": "catalog", "owner": "Argo CD", "source": "apps/prod/boutique/catalog"},
  {"name": "checkout", "owner": "Flux", "source": "flux-system/Kustomization/boutique-checkout"},
  {"name": "redis", "owner": "Native", "source": null}
]
```

## Output evidence

- **Primary artifact:** ASCII or JSON output from the chosen verb.
- **MCP gateway:** all nine Observe verbs are exposed as MCP tools via `cub-scout mcp serve` (standalone tools — see [`references/mcp-tool-catalog.md`](../references/mcp-tool-catalog.md) once that reference ships).
- **`nextSteps[]`** structured hints (where supported): every entry has `actionType: read-only` — mutating actionType is rejected at every cub-scout emit point.

## References

- Capability map: [`README.md`](../../README.md#capability-map) § Observe
- Command details: [`docs/reference/commands.md`](../../docs/reference/commands.md)
- JSON contract: [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md)
- Read-only triad: planned at `references/read-only-triad.md` (batch 5)
- Standalone vs connected: planned at `references/standalone-vs-connected.md` (batch 5)

## Constraints

- All output is point-in-time. For a durable, fingerprinted record of "what cub-scout observed at time T", use the Verify verb group (`cub-scout receipt verify`) once #446 batch 1 lands — see [`scout-verify`](../scout-verify/SKILL.md) (planned).
- If `managedFields` is missing or stripped from the live object, ownership detection still works (via labels + annotations) but per-field attribution falls back to resource-level. See [`references/kubernetes-managedfields.md`](../references/kubernetes-managedfields.md) for the data substrate notes.
- The `watch` verb runs indefinitely until interrupted; agents should pass `--once` or a time bound. Streaming receipts via `--emit-receipt-on` is a separate planned feature (#449).
- If a user asks the skill to act (apply / edit / sync / patch / delete), refuse and route to the appropriate `cub` skill in [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills) or to direct `kubectl` with the user's hands on the keyboard.
