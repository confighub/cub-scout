# Proposal: Agent Field-Manager Identity Convention (G5)

**Status:** Draft proposal — awaiting maintainer approval before any code change  
**Date:** 2026-11-24  
**Tracks:** G5 in [docs/roadmap-3.0-execution.md](../roadmap-3.0-execution.md) (Refs #596, #645)

---

## 1. Problem

When multiple writers touch a cluster — GitOps controllers, operators running `kubectl`, CI jobs, and coding/ops agents — `metadata.managedFields[].manager` is the only per-field writer record Kubernetes keeps by default.

Today, `cub-scout` reads `managedFields` in two places:
- `pkg/agent/field_ownership.go` (`AttributeFieldMutation`) and `pkg/agent/field_ownership_paths.go` (`AttributeFieldPath`): classifies a resource or field path as `controller-drift`, `manual-edit`, or `unknown`, using the verified constant table in `pkg/agent/manager_strings.go`.
- `cub scout explain <resource> --field <path>`: surfaces the observed manager string(s) for that exact field path, falling back to `unknown` when a manager string is not in the verified table.

When an agent writes directly to a cluster (for example via `kubectl apply --server-side`, `kubectl patch`, or a client-go write):
1. If the agent does not set `--field-manager`, Kubernetes records `kubectl-client-side-apply`, `kubectl-patch`, or `Go-http-client`, which is indistinguishable from a human operator or a generic script.
2. If the agent invents its own ad-hoc manager string (`my-bot`, `remediator-v2`), `cub-scout` already surfaces the raw string verbatim in `managers[]` and classifies the cause as `unknown`, because it never guesses.

G5 asks for a **documented convention** for agents that choose to identify themselves in `metadata.managedFields[].manager`, and a clear rule for how `cub-scout` reports what it reads without ever overclaiming.

---

## 2. Non-Negotiable Rules

1. **Report what is read, never guess.** `cub-scout` never infers that a `kubectl-patch` or `Go-http-client` write came from an AI agent.
2. **Self-report is not proof.** Any Kubernetes client with write RBAC can pass any `--field-manager` string (up to Kubernetes' 128-character printable-ASCII limit). `cub-scout` must label any recognized convention as *self-reported field-manager metadata* (`rests_on: "object-metadata"`), never as cryptographic or authenticated identity.
3. **Quoted data.** Because `manager` is arbitrary cluster text, any agent-facing `answer` block must emit it in `quoted` (as `explain_answer.go` already does with `"manager: " + summary.MutationManager`), never as unquoted prose.
4. **No new ownership type.** An agent field-manager string is *mutation evidence* (like `HashiCorp` / `Terraform` in `KnownManagerTool`), never a GitOps owner. A Deployment with `kustomize.toolkit.fluxcd.io/*` labels that was patched by an agent is still Flux-owned.

---

## 3. Proposed Field-Manager Convention

Agents and agent-driven automation that write to Kubernetes (via Server-Side Apply `--field-manager` or client-go `FieldManager`) should use a structured, slash-delimited prefix:

```text
agent/<tool>
agent/<tool>/<run-or-session>
```

Examples:
- `agent/claude-code`
- `agent/codex`
- `agent/antigravity`
- `agent/pilot/run-9f2a1c`

### Constraints
- **Prefix:** literal `agent/` (lowercase).
- **Tool segment (`<tool>`):** `[a-z0-9][a-z0-9._-]{0,31}` — required, identifies the agent or automation runtime.
- **Optional session/run segment (`<run-or-session>`):** `[a-zA-Z0-9._-]{1,64}` — optional, identifies a specific run, PR, or workflow execution without leaking secrets or PII.
- **Total length:** at most 128 characters (Kubernetes `managedFields[].manager` validation limit).
- **No secrets or personal names** in the manager string.

### Why `agent/<tool>[/<run>]`?
- Mirrors existing structured Kubernetes manager conventions already in `pkg/agent/manager_strings.go` (for example `apiextensions.crossplane.io/composite`, `apiextensions.crossplane.io/composed-<hash>`).
- Works with stock `kubectl apply --server-side --field-manager=agent/<tool>` and `kubectl patch --field-manager=agent/<tool>` with zero cluster CRDs or webhooks required.
- Keeps the tool name parseable without regex guesswork while allowing an optional run identifier.
- Distinct from GitOps controllers (`kustomize-controller`, `argocd-controller`, `helm-controller`) and interactive CLI defaults (`kubectl-*`).

---

## 4. What `cub-scout` Would Do (If Approved)

1. **Parser helper in `pkg/agent/manager_strings.go`:**
   - Add `ManagerAgentPrefix = "agent/"` and a strict parser `ParseAgentFieldManager(manager string) (tool, run string, ok bool)` that validates the prefix and character set above.
   - Extend `KnownManagerTool(manager string)` so `agent/<tool>[/<run>]` returns a neutral, honest description (e.g. `"self-reported agent manager (<tool>)"`) — matching the existing `KnownManagerTool` contract ("evidence of who wrote fields, never an ownership claim: a manager name is free text any client can set").
2. **Classification in `pkg/agent/field_ownership.go`:**
   - **Option A (recommended — zero new state words):** Keep `FieldMutationCause` unchanged (`controller-drift`, `manual-edit`, `unknown`). Classify `agent/*` as `unknown` (or keep it out of `manual-edit` so human `kubectl` edits stay distinct), while surfacing the parsed tool name in `KnownManagerTool` and keeping the raw string in `Managers` / `ManagerHint`.
   - **Option B (new cause value — requires explicit approval):** Add `CauseAgentWrite FieldMutationCause = "agent-write"` for `agent/*` managers. This adds a new enum value to `FieldMutationAttribution.Cause` and requires updating `docs/reference/json-contracts.md` and receipt consumers.
3. **Surface in `explain` and `explain --field`:**
   - Continue quoting the raw `manager` string in `answer.basis[].quoted`.
   - In ASCII/Markdown `explain`, display the self-reported agent manager label alongside the raw manager string, with the caveat that `managedFields.manager` is client-supplied metadata.
4. **Never write, never enforce:** `cub-scout` stays strictly read-only. Adoption by writers (skills, plugins, CI jobs, Pilot) is opt-in and documented in the convention section of `docs/reference/`.

---

## 5. Out of Scope

- Cryptographic attestation of who wrote a field (requires Kubernetes audit logs or an admission webhook; outside read-only cluster observation).
- Guessing whether `kubectl-patch` or `Go-http-client` was invoked by a human or an LLM.
- Replacing GitOps delivery: agents should still write through Git or ConfigHub whenever a controller owns the resource; this convention makes direct writes visible when they happen.

---

## 6. Decisions Needed from Maintainer

1. **Prefix format:** Is `agent/<tool>[/<run>]` the right convention, or do you prefer a domain-qualified prefix (e.g. `confighub.com/agent/<tool>`)?
2. **Classification (Option A vs Option B):** Should `agent/*` stay `CauseUnknown` with `KnownManagerTool` labeling (Option A, no schema/enum change), or introduce `CauseAgentWrite = "agent-write"` (Option B)?
3. **Timing:** Land the parser + `KnownManagerTool` recognition in 3.0, or keep G5 as a documented convention for 3.0 and add `CauseAgentWrite` in 3.1/3.2 once writers adopt it?
