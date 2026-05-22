---
name: scout-compare
description: 'Use when the user wants to compare INTENDED vs ACTUAL Kubernetes state — the Compare verb group of cub-scout. Natural phrasing: "did this release land?", "does the cluster match Git?", "is there drift?", "compare DRY / WET / LIVE for this unit", "what does ConfigHub say should be running here?", "give me a source-truth verdict for this resource", "did Argo apply what was in the commit?", "compare this manifest to live state". Load whenever intent is compare / drift / sync-status / verify-matching / DRY-WET-LIVE / source-truth / has-this-landed. Do NOT load for: a list of what is running (use scout-observe), interpreting why something is wrong (use scout-diagnose), per-field provenance (use scout-attribute), or actually applying / syncing (cub-scout never mutates — route to cub, argocd, flux, or kubectl with the user driving).'
phase: verify
allowed-tools: Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub unit get *) Bash(cub unit list *) Bash(cub link list *) Bash(argocd app get *) Bash(flux get *)
---

# scout-compare

The Compare verb group of cub-scout. Four ways to ask "does what's running match what was intended?", with deterministic verdicts and structured evidence.

## When to use

Explicit phrasings:

- "Did this release land in prod?"
- "Does the cluster match what Git says?" / "is the manifest in the file actually deployed?"
- "Is there drift between ConfigHub and the cluster?"
- "Compare DRY / WET / LIVE for unit X" / "show me the three-way state"
- "Give me a source-truth verdict for deploy/api"
- "Did Argo apply what was in commit abc123?"
- "Compare this YAML file to what's running in prod"
- "Is namespace prod conformant to ConfigHub?"

Implicit intents:

- The user has *desired state* somewhere (Git, ConfigHub, a local manifest, a debug bundle) and wants to verify it's reflected in the cluster
- The user wants a *verdict* (PASS / WATCH / BLOCK / INCONCLUSIVE), not just a diff
- The user is gating a CI step on conformance

## Do not load for

- Pure inventory ("show me what's running") — [`scout-observe`](../scout-observe/SKILL.md)
- Interpreting *why* something is wrong (CrashLoop / events / patterns) — [`scout-diagnose`](../scout-diagnose/SKILL.md)
- Per-field provenance (which controller / which git file produced this value) — [`scout-attribute`](../scout-attribute/SKILL.md)
- Forcing a sync / apply / reconcile — cub-scout cannot. Route to `cub`, `argocd app sync` (with the user driving), `flux reconcile`, or `kubectl apply`

## Standalone vs connected

- **Standalone (cluster + file/bundle):** `compare drift --file <yaml>` and `compare --git-path <repo>` work without ConfigHub. Compares the file or repo against the live cluster.
- **Connected (cluster + `cub auth login`):** `compare three-way` adds the DRY (ConfigHub intent) layer; `compare source-truth` runs a strategy-relative verdict over DRY / WET / LIVE.
- **Offline (file or bundle vs file):** `compare` between a manifest file and a debug-bundle snapshot, no cluster needed.

## Tool boundary

- **Allowed (read-only):** all four Compare verbs; `kubectl get/describe`; `cub * get / list`, `cub unit get`, `cub link list` (connected); `argocd app get`, `flux get` (controller-side reads).
- **Not allowed:** `argocd app sync` *as a mutation*, `flux reconcile`, `kubectl apply/edit/patch/delete`, `cub * create/update/delete`. The compare commands never apply a fix.
- **`--fail-on`:** the existing `compare three-way --fail-on info|warning` flag makes the command a CI gate (exit code 2 on violations). It still doesn't mutate — the gate just fails the pipeline.

## The verb menu

| Question | Verb | Notes |
|---|---|---|
| "Compare a manifest file to the cluster" | `cub-scout compare drift --file <yaml>` | File-vs-live drift. Works standalone, no ConfigHub. |
| "Compare this resource end-to-end" | `cub-scout compare <kind>/<name> -n <ns>` | Single-resource compare. Connected: full DRY/WET/LIVE. Standalone: LIVE only with notes. |
| "Compare a scope" | `cub-scout compare three-way --scope namespace/<ns>` | Multi-resource DRY/WET/LIVE with agreement summary. Requires connected mode for DRY/WET. Supports `--scope`, `--view <uuid-or-url>`, `--source-path <local-checkout>` (stage B back-resolution), `--fail-on`. |
| "Strategy-relative verdict" | `cub-scout compare source-truth <kind>/<name> -n <ns> --strategy <s>` | One of 9 strategies (`confighub-oci-argo`, `confighub-oci-flux`, `git-argo`, `git-flux`, `helm-flux`, `helm-argo`, `kustomize-flux`, `oci-flux`, `oci-argo`). Returns a structured pair `status` ∈ {PASS, WATCH, BLOCK, ASK} + `source_truth` ∈ {AGREED, MISMATCH, INCOMPLETE, BLOCKED, UNKNOWN}. Pilot's acceptance kernel consumes both axes. |

All four verbs emit `--format json` for agents and the MCP gateway.

## The loop

1. **Identify the desired state.** Where is "intent" living? A YAML file? A ConfigHub unit? A git repo? A local checkout? This determines the verb:
   - File → `compare drift --file`
   - ConfigHub unit (connected) → `compare three-way`
   - Strategy-typed (connected) → `compare source-truth --strategy <s>`
   - Local git checkout → `compare three-way --source-path <dir>` (stage B back-resolution, #440)
2. **Pick the scope.** Single resource (`<kind>/<name> -n <ns>`), namespace (`--scope namespace/<ns>`), view (`--view <uuid-or-url>`), or cluster (`--scope cluster`).
3. **Invoke.** Use `--format json` if the output is going to an agent or CI; ASCII otherwise.
4. **Read the verdict + evidence.** For `compare source-truth`: the structured pair `status` ∈ {PASS, WATCH, BLOCK, ASK} + `source_truth` ∈ {AGREED, MISMATCH, INCOMPLETE, BLOCKED, UNKNOWN}. For `compare three-way` / `compare drift`: per-field diffs with attribution evidence (`cause`, `managerHint`, `gitSource`, `bindingSource`) — see [`scout-attribute`](../scout-attribute/SKILL.md) for how to read those. `INCONCLUSIVE` is the receipt-level verdict — it shows up only when these results are wrapped into a `cub-scout receipt verify` artifact, not in raw `compare source-truth` output.
5. **Hand off.** If the verdict is `WATCH` or `BLOCK` and the user wants to *fix* it, refuse to mutate and route them: drift produced by `kubectl edit` (per `cause: manual-edit`) typically goes to the original-author path (commit the change back); drift produced by a controller (`cause: controller-drift`) typically resolves on its own — wait or re-run.

## Worked examples

### A: file vs live drift, standalone

The primary v1 path. No ConfigHub auth required.

```bash
$ cub-scout compare drift --file desired.yaml -n prod
Drift Report
════════════
  Cluster: prod-cluster
  Source:  desired.yaml

  [Config] Configuration Drift
  └─ [WARNING] Deployment:prod/api
        path: spec.template.spec.containers[name=app].env[name=LOG_LEVEL]
        desired: info
        observed: debug

Summary: 1 finding
```

Pair with [`scout-attribute`](../scout-attribute/SKILL.md) when you want to know *who* changed `LOG_LEVEL` (controller? human? agent?). cub-scout reads `metadata.managedFields` on the live object to produce that classification — works without ConfigHub.

### B: did Argo land commit abc123? *(connected enrichment)*

```bash
$ cub-scout compare three-way --scope namespace/prod --fail-on warning
Three-Way Compare:
  Scope: namespace/prod
  Resources: 12  Connected: 12  DRY/WET/LIVE: 12  Mismatched: 0
  Agreement: ✓ AGREED — All 12 resources agree
  Conformance: PASS (threshold: warning, max: info, 0 issues)

exit 0
```

CI pipeline can gate on `exit 0`. Mismatch produces exit 2 + a structured `compareFieldMismatch[]` array with per-field `cause` and `managerHint` from the attribution layer (#435). Requires `cub auth login` — adds the DRY (ConfigHub-intended) layer that the standalone case doesn't have.

### C: strategy-relative verdict for Pilot *(connected, strategy-declared)*

```bash
$ cub-scout compare source-truth Deployment/api -n prod --strategy git-argo --format json
{
  "subject": {"kind": "Deployment", "name": "api", "namespace": "prod"},
  "strategy": "git-argo",
  "status": "PASS",
  "verdict": "AGREED",
  "evidence": {
    "git": {"repoUrl": "https://github.com/org/repo", "revision": "abc123"},
    "controller": {"argocd": {"lastSync": "abc123", "sync": "Synced"}},
    "cluster": {"resourceVersion": "1234567"}
  }
}
```

Pilot's acceptance kernel consumes this JSON directly. The strategy must be declared explicitly — `compare source-truth` does not auto-detect the strategy (Pilot needs the operator's stated assumption, not cub-scout's guess).

## Output evidence

- **`compare three-way` agreement:** `summary.agreement.state` ∈ {`agreed`, `converging`, `diverged`, `partial`}; the canonical "did the cluster end up matching ConfigHub intent?" rollup.
- **`compare source-truth` (two separate enums):**
  - `status` ∈ {`PASS`, `WATCH`, `BLOCK`, `ASK`} — cub-scout's evidence-quality verdict
  - `source_truth` ∈ {`AGREED`, `MISMATCH`, `INCOMPLETE`, `BLOCKED`, `UNKNOWN`} — cross-surface agreement under the declared strategy
  - Status and verdict are **separate axes** — a verdict can be `MISMATCH` while status is `BLOCK` (strategy violation) OR `WATCH` (soft mismatch with proof gaps). See [`references/source-truth-strategies.md`](../references/source-truth-strategies.md) for the full status×verdict matrix.
- **Receipt-level verdict** (only when wrapped via `cub-scout receipt verify --strategy <s>`): `PASS` / `WATCH` / `BLOCK` / `INCONCLUSIVE`. `INCONCLUSIVE` is the receipt-only fourth state; it does NOT appear in raw `compare source-truth` output.
- **Per-field mismatches** (`compareFieldMismatch[]`) with attribution: `cause`, `managerHint`, `gitSource`, `bindingSource`.
- **Agreement summary** (three-way): `summary.agreement.state` ∈ {`agreed`, `converging`, `diverged`, `partial`}.
- **CI exit code:** `--fail-on info|warning` produces exit 2 on violations.
- **`nextSteps[]`** structured hints — every `actionType` ∈ {`read-only`, `waiting`, `human-decision`}; mutating is rejected at emit.

## References

- Capability map: [`README.md`](../../README.md#capability-map) § Compare
- Source-truth contract: #393 (v0.1, 4 strategies); #418 (Phase 2 expansion to 9 strategies)
- Three-way agreement summary: #371
- `--view` scope (connected): #391 / #414
- Stage B back-resolution (`--source-path`): #440
- Attribution layer (the evidence inside each mismatch): #435 — and [`scout-attribute`](../scout-attribute/SKILL.md)

## Constraints

- `compare source-truth` does not auto-detect the strategy. The strategy is the operator's declared assumption (which authority produced this artifact?) and Pilot needs it stated, not guessed.
- Standalone mode cannot produce a DRY verdict (no ConfigHub). For pure standalone, prefer `compare drift --file` or `compare --git-path`. Missing connected-mode evidence is recorded explicitly, not silenced.
- For a *durable, fingerprinted* receipt of the comparison result, use the planned `cub-scout receipt verify` once #446 batch 1 lands. Until then, `--format json` output is the contract — it's stable, but ephemeral.
- Never advise the user to bypass a `BLOCK` verdict by mutating directly. If the user wants to accept the live state as the new intent, route them to a `cub` skill for governed write-back.
