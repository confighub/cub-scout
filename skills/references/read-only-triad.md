# Reference: the read-only triad

The architectural separation cub-scout was rebuilt around in `#410` / `#428`. Three roles, three tools, one invariant:

- **cub-scout** is the **evidence provider** — it observes; it never mutates.
- **ConfigHub** (via `cub`) is the **authority** — it owns intended state and mutations of that state.
- **Pilot** is the **judge** — it consumes cub-scout's evidence and ConfigHub's authority to produce accept/wait/block verdicts.

cub-scout's read-only invariant is **code-enforced**, not convention. This doc explains what that means concretely.

## The three roles

| Role | Tool | What it does | What it does NOT do |
|---|---|---|---|
| Evidence provider | `cub-scout` (`cub scout` plugin) | Reads cluster + controllers + ConfigHub; produces typed structured evidence (attribution, source-truth, receipts, etc.) | Apply, patch, edit, delete cluster state. Create / update / delete ConfigHub units. Decide whether evidence is "approved" or "acceptable". |
| Authority | `cub` (the ConfigHub CLI) | Authors intended state in ConfigHub (units, spaces, targets, links). Applies units to clusters. Manages governance workflows. | Read the cluster to produce evidence about it — that's cub-scout's job. |
| Judge | Pilot (tracked in `#444`) | Consumes cub-scout's source-truth evidence + receipts; produces acceptance verdicts that gate CD / promotion / rollback. Acts on the verdict via its own connectors. | Generate evidence — Pilot is a consumer, not a producer. |

The separation is **structural**: each tool owns one phase of the loop. Combining them in one CLI would conflate provenance, authority, and acceptance — and conflating any two collapses the trust model. The triad keeps them honest.

## Why this matters

Before the triad work, cub-scout had a `remedy` command that executed mutations (`kubectl apply / patch / delete` via an `executor.Execute` path). The architectural-triad audit (`#410`) found that this was an evidence tool acting as an authority — exactly the conflation the triad forbids.

`#428` resolved it: `remedy` was renamed to `suggest-remedy` and the executor path was deleted. cub-scout is now categorically read-only, by code, by tests, by static guard.

This unlocked the receipt capability (`#446`): receipts are evidence artifacts, and the same read-only invariant lets them be safely consumed by Pilot or any other acceptance kernel without trust assumptions about cub-scout's intent.

## Code-enforced invariant

The invariant has three layers of enforcement.

### Layer 1 — `scripts/check-readonly.sh`

Runs in CI on every PR. Greps the `cmd/` and `pkg/` trees for Kubernetes client write operations:

```bash
$ ./scripts/check-readonly.sh
Checking for Kubernetes write operations outside allowed files...

PASSED: No write operations found outside allowed files.

Allowed files: remedy.go import.go import_wizard.go import_argocd.go _test.go mock fake
```

The allowlist is intentionally narrow:
- `remedy.go` is now `suggest-remedy.go` (read-only); the script tracks the old name for legacy reasons but the file no longer writes
- `import.go` / `import_wizard.go` / `import_argocd.go` are the `cub-scout import apply` paths — mutating, but tightly scoped, and the `apply` path is excluded from the LLM-loadable skills' allowed-tools
- `_test.go` and `mock` / `fake` are test surfaces that can simulate mutating behavior for testing

Any new mutating client call outside these files fails the script → fails CI → blocks merge.

### Layer 2 — receipt-package static grep

`cmd/cub-scout/receipt_readonly_test.go` (specifically `TestReceiptPackageReadOnlyClient`) scans all `receipt*.go` source files in `cmd/cub-scout/` and `pkg/agent/` and fails the build if any of these forbidden tokens appear:

```
.Create(  .Update(  .UpdateStatus(  .Patch(
.Apply(   .ApplyStatus(  .Delete(  .DeleteCollection(
```

Future receipt code (batch 4+, v2 signing, chained receipts) inherits the guard automatically — no opt-in required.

### Layer 3 — `FilterNextSteps` runtime filter

`pkg/agent/receipt_predicates.go` `FilterNextSteps` is called on every receipt before the fingerprint is stamped. It drops:

- Any `nextStep` with `actionType` outside `{read-only, waiting, human-decision}`
- Any `nextStep` whose `nextCommand` contains a mutating verb fragment: `apply`, `edit`, `patch`, `delete`, `sync` (with trailing space — so `synced` doesn't false-flag), `create`, `update`, `replace`, `scale`, `rollout`, `reconcile`, `annotate`, `label`, `set`, `exec`, `debug`, `helm install`, `helm upgrade`

Dropped steps are recorded in the receipt's `omissions[]` array with `Missing` set to `next-step-allowed-action` or `next-step-allowed-command` — so downstream consumers can see *which* mutating hint was rejected.

This is **defense in depth**: predicate evaluators are not supposed to produce mutating next steps in the first place, but the filter catches the case where one slips through.

## Skill-level invariant

Every skill in this repo's `skills/` directory follows the same invariant in its `allowed-tools` line. From `SKILL_TEMPLATE.md`:

> DO NOT use broad `Bash(cub-scout *)` / `Bash(./cub-scout *)` / `Bash(cub scout *)` wildcards — those grant `cub-scout demo` (kubectl-applies to set up demo state), `cub-scout import apply` (creates ConfigHub resources), and `cub-scout compare --suggest --apply` (creates ConfigHub Apps + Deployments). The architectural triad is read-only-by-default but specific verbs and flag combinations mutate; allowed-tools must enumerate the read-only verbs only.

Every shipped skill follows this — no broad wildcards, every verb enumerated. The route-mutations-to-cub-or-user handoff is the convention; the static guards + filter are the code-level backup.

## The boundary with `cub`

`cub` is mutation-capable by design. The boundary is bright:

| Operation | Tool |
|---|---|
| Read cluster state | `cub-scout` (or `kubectl get / describe / logs`) |
| Read ConfigHub state | `cub * get`, `cub * list`, `cub unit get`, `cub link list` |
| Author a ConfigHub unit | `cub unit create / update` |
| Apply a unit to a cluster | `cub gitops import` (target + render-target based) OR direct `cub apply` |
| Sync an Argo Application | `argocd app sync` (user-driven, not via cub-scout) |
| Force a Flux reconcile | `flux reconcile` (user-driven) |
| Patch a cluster resource | `kubectl patch / edit / apply` (user-driven) |
| Approve a CD promotion | Pilot (via its connectors) |

cub-scout never invokes the mutating side. If a workflow needs mutation, the skill routes the user to `cub`-skills upstream (in [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills)) or to direct CLI with the user driving.

## Suggest-remedy specifically

`cub-scout suggest-remedy` is **named** to make the boundary obvious. It describes a remediation in JSON; it does not apply one. The legacy alias `remedy` is hidden but still accepts the command (for backward compatibility); `suggest-remedy` is the canonical name.

The output's `actionType` field is part of the `nextSteps[]` contract — same as receipts. A `suggest-remedy` finding with `actionType: mutating` would be a bug; the same `FilterNextSteps` rules apply.

## What about `import apply`?

`cub-scout import apply` writes to **ConfigHub**, not to the cluster. It's the one cub-scout verb that mutates anything — and it mutates the ConfigHub side. The architectural-triad audit found this is acceptable because:

1. ConfigHub is the authority side; writing to it is congruent with cub-scout being the evidence provider for the cluster side
2. The mutation is **user-driven** — `import apply` is explicitly outside every skill's `allowed-tools` line; agents call only preview paths such as `import --dry-run`, `import --from-bundle`, and `import --git-path`
3. The preview flow recommends saving a deterministic proposal for review ([`prepare-for-confighub`](../prepare-for-confighub/SKILL.md)) — the proposal is reviewed first, and only after approval does anyone run `import apply`

Lower-severity findings in `#410` (e.g., `import apply` wording in some user-facing strings) remain open as follow-ups. The high-severity finding (`remedy` executing mutations) is resolved.

## Receipts and the triad

Receipts (`#446`) embody the triad in a single artifact. A receipt is:

- **Produced** by cub-scout (the evidence provider)
- **Anchored** to ConfigHub state via the `confighub-unit://` subject (in connected mode) — connected to the authority's identity
- **Consumed** by Pilot (or another acceptance kernel) to produce a verdict

The receipt's `nextSteps[]` field is filtered to read-only / waiting / human-decision actionTypes (by `FilterNextSteps` above). The receipt itself is **immutable** — once stamped with a fingerprint, the artifact is the artifact, and the local store refuses to overwrite. This matches the triad: evidence is recorded once, then consumed; it doesn't get edited downstream.

## Skills that consume this reference

Every skill in this repo, implicitly — the read-only invariant is the contract. Specific skills that lean on it explicitly:

- [`scout-attribute`](../scout-attribute/SKILL.md) — the attribution layer's evidence-vs-authority boundary
- [`scout-verify`](../scout-verify/SKILL.md) — receipts as the trifold artifact
- [`ai-agent-readonly-context`](../ai-agent-readonly-context/SKILL.md) — the safety-by-construction value prop for AI integration
- [`scout-mcp`](../scout-mcp/SKILL.md) — MCP gateway exposes only the read-only tool catalog
- [`investigate-drift`](../investigate-drift/SKILL.md) — produces evidence; never reverts
- [`operator-incident-evidence`](../operator-incident-evidence/SKILL.md) — captures the state for postmortem; never restores
- [`migrate-from-kubectl`](../migrate-from-kubectl/SKILL.md) — finds manual edits, plans migration; user drives revert/port

## References

- Code:
  - `scripts/check-readonly.sh` (CI gate)
  - `cmd/cub-scout/receipt_readonly_test.go` `TestReceiptPackageReadOnlyClient` (static grep)
  - `pkg/agent/receipt_predicates.go` `FilterNextSteps` + `isMutatingCommand` (runtime filter)
  - `cmd/cub-scout/suggest_remedy.go` (the renamed read-only command)
- Architectural-triad work: `#410` (audit), `#428` (suggest-remedy refactor + executor deletion)
- Receipt invariant: `#446` (parent), `docs/proposals/receipts-way-forward.md` § "read-only-triad lock"
- Skill-level allowed-tools convention: [`SKILL_TEMPLATE.md`](../SKILL_TEMPLATE.md)
- Related projects: Pilot (the judge side; integration tracked in `#444`); [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills) (cub authoring skills — the authority side)
