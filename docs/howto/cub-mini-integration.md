# `cub mini` & Agent Write-Skill Integration Contract

This document defines the read-only hook contract between **`cub mini`** (or any
agent write skill / ConfigHub CLI workflow) and **`cub-scout`**.

`cub mini` and agent skills own intended state and writes. `cub-scout` is the
independent, strictly read-only live-cluster witness called immediately **before**
a write, **after** a write, and when producing durable delivery proof.

---

## Responsibility Boundary (Zero Overlap)

| Concern | `cub mini` / Agent Write Skill | `cub-scout` (`cub scout`) |
|---|---|---|
| Intended state (spaces, units, targets, GitOps manifests) | **Owns** | Never modifies |
| Cluster or Git writes (`apply`, `patch`, `commit`, `push`, `cub` mutations) | **Owns** (sets `--field-manager agent/<tool>[/<run>]`) | **Zero cluster writes** (`get`, `list`, `watch` only) |
| Sync triggers, rollbacks, remediation, approvals | **Owns** (or delegates to GitOps controller / `ChangeWorkflow`) | Never triggers sync or remediation |
| Pre-write live-cluster safety sensing | Calls `cub scout before-write` | **Owns** (`before-write` / MCP `before_write`) |
| Post-write landing & running Pod `imageID` verification | Calls `cub scout landed --verify-pod-images` | **Owns** (`landed` / MCP `landed`) |
| Tamper-evident offline delivery proof & fleet aggregation | Stores/transports signed artifacts | **Owns** (`receipt sign`, `receipt verify`, `receipt verify-fleet`) |
| AI / LLM reasoning | Agent host / skill | **Zero LLM** — 100% deterministic parsing |

---

## Hook 1 — Before Write: `cub scout before-write`

Invoke immediately before editing a GitOps manifest, updating a ConfigHub Unit,
or applying a direct patch:

```bash
cub scout before-write <kind/name> -n <ns> \
  --field-path <path> \
  --format json
```

Example:

```bash
cub scout before-write deploy/api -n prod \
  --field-path .spec.replicas \
  --format json
```

MCP equivalent (over `cub-scout mcp serve --tools core`): tool `before_write`
with `{ "resource": "deploy/api", "namespace": "prod", "field_path": ".spec.replicas" }`.

### What the hook returns (`< 1 KB` budget)

The `answer` block is CI-enforced at `<= 700` bytes (measured at 672 bytes in
[answer-sizes.md](../reference/answer-sizes.md)) and includes:

- **Owner (`owner`)**: Flux, Argo CD, Helm, kro, Crossplane, Sveltos,
  Modelplane, ConfigHub, or Native.
- **Field attribution (`fieldAttribution`)**:
  - Current live `value` at `--field-path`.
  - `managedFields` writers and **writer contention** across multiple managers,
    including G5 self-reported managers (`agent/<tool>[/<run>]`, e.g.
    `agent/cub-mini/run-42`).
  - `revertPrediction`: `likely-reverted`, `not-expected`, or `unknown`.
- **Deployer guardrails (`deployer`)**: Flux `spec.suspend` or Argo CD
  `autoSync`, `selfHeal`, `prune`, `ignoreDiffCount`, and `AppProject`
  `syncWindows`.
- **Disruption budgets (`pdbs`)**: matching `policy/v1` `PodDisruptionBudget`s
  in the target namespace (`minAvailable`, `maxUnavailable`,
  `disruptionsAllowed`).
- **Standard `answer` envelope**: `answer.state` is `explained` or
  `not-observable`, with quoted cluster text in `basis[]` / `unknowns[]`.

### Required caller actions

| Observed signal | Required `cub mini` / skill behavior |
|---|---|
| `fieldAttribution.revertPrediction == "likely-reverted"` | Do **not** patch live cluster state; route the change through the GitOps source or ConfigHub Unit instead. |
| Writer contention (`managedFields` shows another active `agent/<tool>[/<run>]` or controller on `--field-path`) | Surface the competing manager and coordinate or abort before overwriting the same field path. |
| Active Argo `deny` `syncWindow` or `pdbs[].disruptionsAllowed == 0` | Warn the operator or defer the rollout until the window/budget allows disruption. |
| `answer.state == "not-observable"` | Treat safety state as unknown (do not assume safe); check RBAC scope or `--kube-context`. |

> **G5 writer self-identification:** Whenever `cub mini` or an agent skill
> performs a direct Kubernetes server-side apply, pass
> `--field-manager agent/<tool>/<run>` (for example `agent/cub-mini/run-17`).
> `cub scout before-write` and `explain` parse this format deterministically so
> concurrent agents immediately see contention.

---

## Hook 2 — After Write: `cub scout landed`

Invoke after pushing a Git commit, publishing an OCI digest, or updating a
ConfigHub Unit to prove the exact anchored revision reached running Pods:

```bash
cub scout landed <kind/name> -n <ns> \
  --revision <sha> \
  --verify-pod-images \
  --wait 60s \
  --format json
```

Supply exactly one anchor: `--revision <sha>`, `--digest sha256:...`, or
`--unit-revision <rev>`. Add `--image <ref-or-sha256>` when pinning an explicit
expected container image or digest alongside `--verify-pod-images`.

MCP equivalent: tool `landed` (bounded `--wait` is capped at `60s` on MCP and
`5m` on CLI; poll interval defaults to `5s`, minimum `2s`).

### Exit and `answer.state` handling

`cub scout landed --format json` exits `0` whenever it emits a valid structured
JSON answer, and exits non-zero only on CLI/usage/validation errors (for
example, invalid flags or conflicting `--recording` + `--kube-context`).
Callers **must** branch on `.answer.state` rather than exit code alone:

| `answer.state` | Meaning | Required caller action |
|---|---|---|
| `landed` | Controller applied the exact anchor, workload rolled out, and (with `--verify-pod-images`) running Pod `status.containerStatuses[].imageID` digests match. | Mark rollout verified and optionally sign the receipt. |
| `converging` | Source fetch, controller reconcile, or workload rollout is still in progress (`convergence.recheckAfterSeconds` hints the next poll). | Keep polling (using `--if-unchanged`) or wait until `--wait` completes. |
| `not-landed` | Broken delivery link (`brokenLink.stage`), wrong settled revision, failed workload, or running Pod `imageID` mismatch (even if the controller claims `Synced`/`Ready`). | Fail closed; surface `brokenLink` and `answer.reason`. Never treat as success. |
| `not-observable` | Missing anchor, unreadable RBAC scope, unreachable cluster, or unsupported controller. | Fail closed or escalate; never assume landed. |

### Low-token delta polling (`--if-unchanged`)

When polling in a loop outside `--wait`, pass the prior response's
`.fingerprint` to `--if-unchanged`:

```bash
fp="$(jq -r '.fingerprint' landed.json)"
cub scout landed deploy/api -n prod \
  --revision "$SHA" \
  --verify-pod-images \
  --if-unchanged "$fp" \
  --format json
```

If nothing changed since the previous poll, `cub-scout` returns an ~89-byte
`unchanged` envelope instead of repeating the full JSON payload.

---

## Hook 3 — Durable Proof: `receipt sign` → `receipt verify-fleet`

Once a cluster reaches `answer.state == "landed"`, sign the JSON output offline
with Ed25519 and verify promotion across `dev` → `staging` → `prod` with zero
cluster or network access:

```bash
# On each environment after landed succeeds (offline Ed25519 signing)
cub scout landed deploy/api -n dev  --revision "$SHA" --verify-pod-images --format json \
  | cub scout receipt sign --input - --key "$SIGNING_KEY" -o receipts/dev.signed.json
cub scout landed deploy/api -n staging --revision "$SHA" --verify-pod-images --format json \
  | cub scout receipt sign --input - --key "$SIGNING_KEY" -o receipts/staging.signed.json
cub scout landed deploy/api -n prod --revision "$SHA" --verify-pod-images --format json \
  | cub scout receipt sign --input - --key "$SIGNING_KEY" -o receipts/prod.signed.json

# Verify the entire fleet offline before promotion or audit sign-off
cub scout receipt verify-fleet \
  --inputs ./receipts \
  --pubkey "$VERIFY_PUBKEY" \
  --revision "$SHA" \
  --require-clusters dev,staging,prod \
  --max-age 30m \
  --format json
```

`cub scout receipt verify-fleet` fails closed:
- Exits `0` only when every signature is valid, all required clusters are
  present, all revisions match `--revision`, and every cluster landed.
- Exits `1` on any tampered payload, wrong key, malformed JSON, revision
  mismatch, missing required cluster, or non-landed cluster state (and exits `2`
  if `--pubkey` cannot be read).
- Surfacing per-cluster `observedAt` and `--max-age` staleness flags keeps
  offline evidence honest without needing a fleet database.

---

## Offline / Air-Gapped Evaluation

Both `before-write` and `landed` accept `--recording <snapshot.yaml>` produced
by `cub scout snapshot --format yaml -o snapshot.yaml`, running with zero
Kubernetes API calls and adding an explicit `kind: stale` unknown with the
snapshot's `observedAt` timestamp.

---

## Related Docs

- [Agent Write Loop](agent-write-loop.md) — End-to-end walkthrough for coding agents, ops bots, and CI gates
- [Reusable CI Promotion Gate (`verify-landed`)](../../examples/ci-gates/verify-landed/README.md) — Drop-in GitHub Actions composite action
- [Ops-Bot Consumer Contract](../../examples/bot/consumer-contract.md) — Wire contract for `k8sbot` / Pilot consuming `bot` and `watch` envelopes
- [Answer Sizes & Standing Benchmarks](../reference/answer-sizes.md) — CI-enforced byte ceilings across all 13 surfaces
- [JSON Contracts](../reference/json-contracts.md) — Exact schemas and approved `answer.state` vocabulary
