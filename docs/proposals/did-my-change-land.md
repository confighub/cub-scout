# Proposal: "Did my change land?" (A(1) + G1 + G2)

**Status:** Approved by the maintainer on 2026-11-24; first slice implemented
as `cub scout landed` (CLI) and `landed` (MCP) with state words `landed`,
`converging`, `not-landed`, `not-observable` and G2 wait caps (`0s` default,
`5m` CLI max, `60s` MCP max, `5s` poll interval / `2s` min) for Flux and
Argo CD workloads (`Deployment`, `StatefulSet`, `DaemonSet`).

This proposal specifies Package A(1) ("did my change land"), Addition G1
(`converging` as a state), and Addition G2 (a bounded, read-only wait) from
[docs/roadmap-3.0-execution.md](../roadmap-3.0-execution.md#agent-plan--2026-11-13)
and [docs/reference/agentic-gitops-observability.md](../reference/agentic-gitops-observability.md).

---

## 1. The agent's question

After an agent writes configuration (through Git or ConfigHub), it must close
its own change loop before reporting success or handing off to a gate:

> **"Did revision `R` (or OCI digest `D` / ConfigHub unit revision `N`) reach
> `<kind>/<name>` in `<namespace>` on cluster `<context>` — or is delivery
> still converging, or did a link break?"**

Today an agent must stitch together `trace`, `gitops status`, `release check`,
and `compare source-truth` across multiple turns, and still cannot tell
*"still converging"* from *"failed"* in one call, or wait without burning turns
in a polling loop.

### Reused foundations

This builds directly on shipped cub-scout components rather than inventing a
parallel engine:
- **Ownership & delivery chain:** `trace` (`pkg/agent/*_trace.go`)
- **Revision / digest comparison:** `release check` (`pkg/agent/release_check*.go`) and `compare source-truth` (`pkg/agent/source_truth*.go`)
- **Rollout / workload readiness:** `kstatus` (`sigs.k8s.io/cli-utils/pkg/kstatus`)
- **Evidence envelope:** `mapsvc.Answer` (`internal/mapsvc/answer.go`)
- **Explicit cluster binding:** `--kube-context` (`cmd/cub-scout/command_context_registry_test.go`)

---

## 2. Candidate command and MCP surface (for approval)

- **CLI (plugin form first):**
  ```sh
  cub scout verify landing deploy/api -n prod \
    --revision <git-sha-or-oci-digest-or-unit-rev> \
    --kube-context <ctx> \
    --wait 60s \
    --format json
  ```
  *(Alternative command names for maintainer choice: `cub scout check landing`,
  `cub scout landing`, or `cub scout release landed`.)*
- **MCP tool:** `check_landing` (or matching the chosen CLI verb), taking
  `target`, `namespace`, `context`, one explicit anchor (`revision`, `digest`,
  or `unit_revision`), and optional `wait` (default `"0s"`).
- **TUI parity:** Renders the same structured result in a single-target
  convergence view (and ASCII/Markdown for CLI human use).

---

## 3. Candidate state words and reason keys (for maintainer choice)

The maintainer chooses all state words. Proposed closed set:

| `answer.state` | Meaning | Candidate `answer.reason` keys |
|---|---|---|
| `landed` | The anchored revision/digest was observed on the controller **and** the target workload has converged on that exact revision. | `revision-and-workload-converged`, `digest-and-workload-converged` |
| `converging` | Delivery or rollout is in progress and no terminal failure was observed (G1). Includes `recheckAfterSeconds` and `waitingOn`. | `source-fetching`, `controller-reconciling`, `controller-on-prior-revision`, `workload-rolling-out` |
| `not-landed` | A link in the delivery chain reported a failure or a settled mismatch against the requested revision/digest. Names `brokenLink`. | `source-error`, `controller-reconcile-failed`, `controller-settled-other-revision`, `workload-failed`, `strategy-source-mismatch` |
| `not-observable` | Required evidence could not be read (RBAC denied, CRD/object missing, no owner marker, or no anchor supplied). Never collapses into `landed` or `not-landed`. | `no-anchor-supplied`, `target-not-found`, `no-owner-marker-recognised`, `read-denied`, `cluster-unreachable` |

### Additional structured fields beside `answer`

- `anchor`: `{ "kind": "git-sha" | "oci-digest" | "unit-revision", "value": "..." }`
- `brokenLink` (when `not-landed`): `{ "stage": "source" | "deployer" | "workload" | "confighub-release", "ref": { ... } }`
- `convergence` (when `converging`): `{ "waitingOn": "source" | "deployer" | "workload", "recheckAfterSeconds": 10, "timedOut": false }`
- `wait` (when `--wait > 0`): `{ "requestedSeconds": 60, "elapsedMillis": 14200, "polls": 3, "settled": true }`

---

## 4. G2: Bounded, read-only wait contract

- **Opt-in:** Default `--wait 0s` performs one point-in-time check.
- **Hard ceiling:** `--wait` is capped (proposed max `5m` for CLI, `60s` for
  MCP tool calls to stay inside client tool-call timeouts) and `--poll-interval`
  defaults to `5s` (min `2s`).
- **Early exit:** Returns immediately on the first poll whose state is
  `landed`, `not-landed`, or terminal `not-observable` (e.g., `read-denied`).
- **Deadline expiry:** If `--wait` elapses while still `converging`, returns
  `state: "converging"` with `convergence.timedOut: true`,
  `convergence.recheckAfterSeconds`, and a read-only `next[]` command to check
  again.
- **Not a watch service:** No background daemon, no persistent cache, no
  webhooks, no cursor (`#539` stays `not-planned`). Every poll uses the same
  read-only session client and accumulates into `answer.cost`.

---

## 5. Answered by the six questions (Principle 10)

1. **Question:** Answers *"did my revision/digest `R` land on `<target>`?"*
   Always anchored to an explicit caller-supplied revision, digest, or unit
   revision — a healthy Pod on the old revision is never `landed`.
2. **Who says so:** Every claim in `answer.basis[]` carries `restsOn`
   (`controller-report`, `deployer-report`, `object-metadata`,
   `confighub-record`, or `cub-scout-comparison`) and a fetchable `ref`.
   Controller `Ready=True` is explicitly labelled `controller-report`, not
   independent proof of workload convergence.
3. **Unknowns:** Missing permissions, absent CRDs, unreadable Pods, or
   uncompared digests appear in `answer.unknowns[]` with one of the standard
   kinds (`not-observable`, `denied`, `stale`, `not-requested`, `not-compared`).
   Missing data never yields `landed` (zero false-landed invariant).
4. **Time:** `converging` is a first-class state distinct from `not-landed`
   (G1), carrying `recheckAfterSeconds` and `waitingOn`. `--wait` (G2) absorbs
   short convergence windows into one tool call.
5. **Cost:** One CLI/MCP call, with a declared `answer` block byte ceiling in
   `cmd/cub-scout/answer_size_ceilings_test.go` (target `<= 1200` bytes).
   `answer.cost` reports total `requestsMade`, `responseBodyBytes`, and
   `durationMillis` across all polls. Turns saved vs raw `kubectl` polling are
   countable deterministically without a paid run.
6. **Safety:** Strictly read-only (`scripts/check-readonly.sh`). Every
   cluster-sourced message, condition reason, or annotation appears only in
   `basis[].quoted` / `unknowns[].quoted`, never in `summary` or `next[]`.
   `next[]` contains only exact read-only commands.

---

## 6. Pre-coding test and recorded fixture plan

Before implementation is considered done (CLAUDE.md Pre-Coding rules):

1. **Deterministic recorded fixtures (zero live cluster required):**
   - `01-flux-oci-landed`: OCIRepository + Kustomization + Deployment all on target digest, `kstatus` Current → `landed`.
   - `02-flux-oci-converging-prior-rev`: OCIRepository on new digest, Kustomization still reconciling prior digest → `converging` (`waitingOn: "deployer"`).
   - `03-flux-kustomization-build-failed`: Kustomization `Ready=False` (`BuildFailed`) → `not-landed` (`brokenLink.stage: "deployer"`).
   - `04-argo-settled-wrong-revision`: Argo CD Application `Synced`/`Healthy` on an *older* Git SHA than requested → `not-landed` (`controller-settled-other-revision`), **never** `landed`.
   - `05-workload-rolling-out`: Controller applied target revision, Deployment `kstatus` InProgress (old Pods still terminating) → `converging` (`waitingOn: "workload"`).
   - `06-rbac-denied-and-no-owner`: Denied Deployment read or unlabelled resource → `not-observable`, **never** `landed` or `not-landed`.
   - `07-bounded-wait-settles`: Fake clock + sequence of 3 recorded snapshots (`converging` → `converging` → `landed`) proving early exit and accumulated `answer.cost`.
   - `08-bounded-wait-timeout`: Fake clock reaching deadline while `converging` → returns `converging` with `timedOut: true`.
2. **Real-cluster CI proof:** Exercise in the existing `GitOps E2E` lane on real Flux and Argo CD installs (both `converging` and `landed` transitions).
3. **Agent eval case:** Add a recorded eval case under `evals/` asking *"did my change land?"* (scored on recordings, no paid run required to merge).
4. **Zero false-landed invariant:** A dedicated unit test asserts that no negative control (wrong revision, stale report, denied read, missing anchor, progressing rollout) ever produces `state == "landed"`.

---

## 7. Decisions needed from the maintainer

1. **Command & MCP tool name:** e.g. `cub scout verify landing` / `check_landing` vs `cub scout landing` vs another name?
2. **State words:** Approve `landed`, `converging`, `not-landed`, `not-observable` (or specify replacements)?
3. **G2 wait caps:** Approve default `0s` (single-shot), CLI max `5m`, MCP max `60s`, default poll interval `5s`?
4. **Scope of v3.0 first slice:** Flux + Argo CD + Deployment/StatefulSet/DaemonSet first (matching `release check`), with Sveltos/Crossplane/kro reporting `not-observable` with explicit reason until their adapters land?
