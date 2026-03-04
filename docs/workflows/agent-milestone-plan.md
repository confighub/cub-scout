# Agent Milestone Delivery Plan

This plan defines how to use multiple delivery agents to execute remaining roadmap milestones while enforcing `CLAUDE.md` coding and testing rules.

## Scope Source (Authoritative)

- Roadmap: `docs/roadmap.md` (especially "What Is Left on the Roadmap")
- Principles and DoD gates: `CLAUDE.md`
- Testing recipe/cookbook: `docs/testing/BEST-PRACTICES.md`

## Non-Negotiable Delivery Rules (from CLAUDE.md)

Every milestone issue must include, before coding:

1. Deterministic success criteria (exact input -> exact output).
2. Required test plan:
   - Unit/contract tests (offline, deterministic).
   - Example coverage for user-visible behavior.
   - Integration/E2E proof (or explicit waiver + safe fallback).
3. Graceful degradation behavior:
   - Missing metadata behavior.
   - Partial result behavior.
   - How false unmanaged/orphan claims are avoided.
4. Definition of Done:
   - Tests pass.
   - Example/docs updated.
   - Output is correct and explainable.

## Agent Topology

Use a fixed 6-agent pipeline per milestone. One person can run multiple roles, but handoffs stay explicit.

1. `Milestone-Owner Agent`
   - Break milestone into issue slices with acceptance criteria.
   - Enforce scope boundaries from `docs/roadmap.md`.
2. `Contract Agent`
   - Defines CLI/JSON/schema impact.
   - Rejects silent contract expansion.
3. `Implementation Agent`
   - Writes code in small PR slices.
   - Preserves deterministic/read-only behavior.
4. `Example Agent`
   - Adds or updates `examples/` and demo proof paths.
   - Ensures examples are runnable and aligned with CLI.
5. `Test Agent`
   - Adds required tests (`unit`, `golden`, `integration`/`e2e`).
   - Runs required commands and records evidence.
6. `Docs/Release Agent`
   - Updates user docs, contracts, and release notes.
   - Verifies command/docs consistency.

## Mandatory Task Sequence (Per Agent, Per Task)

Every milestone slice must run the same proof-first sequence. No coding begins until Steps 1-4 are complete.

| Step | Agent | Required action | Required output |
|---|---|---|---|
| 1 | `Milestone-Owner Agent` | Define task boundary and non-goals from roadmap scope | Task brief with scope, out-of-scope, risk notes |
| 2 | `Contract Agent` | Define expected CLI/JSON/schema behavior and graceful degradation | Contract delta note (or "no contract change") |
| 3 | `Test Agent` | Define proof plan before implementation (unit/golden/integration/e2e as applicable) | Proof matrix with exact commands and expected assertions |
| 4 | `Example Agent` | Define example/fixture proof path before implementation | Example impact note (`existing` vs `new`, with file paths) |
| 5 | `Implementation Agent` | Implement smallest vertical slice that can satisfy the first proof | Minimal code change linked to proof step |
| 6 | `Test Agent` | Run proofs immediately after slice implementation | Proof run #1 evidence (pass/fail + logs summary) |
| 7 | `Implementation Agent` | Fix gaps found in run #1; no new scope | Patch set tied only to failed proofs |
| 8 | `Test Agent` | Re-run the same proofs until stable | Proof run #2 and #3 evidence (consecutive passes) |
| 9 | `Docs/Release Agent` | Update user docs/contracts/examples to match shipped behavior | Doc diff + command/docs consistency check |
| 10 | `Milestone-Owner Agent` | Approve completion only when all proof gates pass | Task completion note with evidence links |

## Proof-First Rule (Hard Gate)

For each task:

1. Proofs/tests are designed and written first (or at minimum stubbed as failing checks) before implementation.
2. Implementation starts only after the proof matrix exists and is reviewed.
3. Task cannot be marked complete on a single pass.
4. Completion requires repeated verification:
   - at least 2 consecutive passing runs of the task proof set;
   - plus 1 final pass after docs/examples updates.

If any verification run fails, task status returns to `in progress`.

## Iterative Verification Loop (Required)

Use this loop for each task until done:

1. `Plan`: confirm contract + proof matrix + fixture/example coverage.
2. `Implement`: small change only.
3. `Verify`: run scoped proofs immediately.
4. `Record`: capture results (command, pass/fail, short evidence note).
5. `Refine`: patch only what verification showed.
6. Repeat from step 3 until proof stability criteria are met.

Do not batch multiple unverified code changes together.

## Task Proof Matrix Template

Each task should include this matrix in its issue/PR description.

| Proof tier | Required? | Command(s) | Deterministic assertion |
|---|---|---|---|
| Unit | Yes | `go test <pkg>` | Exact ownership/logic output |
| Golden/ASCII | For user-visible output | `go test ./test/golden/...` or `./test/ascii/...` | Stable rendered output |
| Integration | For live-cluster behavior | `go test -tags=integration ./...` | Behavior against real K8s API |
| E2E | For internal + live state workflows | `go test -tags=e2e ./cmd/cub-scout/...` | End-to-end workflow evidence |
| Contract audit | For schema/contract-sensitive work | `./scripts/contract-audit.sh` | No contract drift |
| Example proof | For user-facing features | Example command/script path | Example output aligns with feature |

All rows marked "Required?" must be green before completion.

## Milestone Sequence

Prioritize by risk reduction and dependency order.

All milestone slices must follow the mandatory task sequence and iterative verification loop above.

## M1: Trace and Provenance Fidelity

Roadmap targets:
- Distinguish "ConfigHub via OCI" ownership in trace outputs.
- Source staleness/sync signals where evidence exists.
- Orphan detection for broken ApplicationSet generator links.
- Close known scan gap #200 (cluster export -> cub-scan flow).

Agent execution:
1. `Milestone-Owner Agent`: split into 3-4 PR slices (trace labels, staleness signals, appset orphan detection, scan-cluster bridge).
2. `Contract Agent`: define JSON/ASCII deltas and fallback semantics.
3. `Implementation Agent`: implement in order above.
4. `Example Agent`: add/refresh one Argo+OCI example and one broken-generator fixture.
5. `Test Agent`: add deterministic fixtures and goldens for each new signal.
6. `Docs Agent`: update trace/scan/howto docs.

Required proof:
- `go build ./cmd/cub-scout`
- `go test ./...`
- New `test/golden/trace` and `test/golden/scan-*` coverage
- Integration proof for ApplicationSet broken-link behavior

## M2: Connected Import Evolution

Roadmap targets:
- OCI-first source language alignment.
- Evidence contract integration with bundle/scan surfaces.
- Import from bundle artifacts (not only live cluster).

Agent execution:
1. `Milestone-Owner Agent`: split by import entrypoint (`cluster`, `bundle`), then evidence export.
2. `Contract Agent`: lock proposal JSON changes and CLI flag semantics.
3. `Implementation Agent`: ship `bundle` import path behind explicit command/flag.
4. `Example Agent`: add "import from bundle" worked example with expected output.
5. `Test Agent`: unit + golden + integration path (connected gated).
6. `Docs Agent`: update `docs/howto/import-to-confighub.md`, `commands.md`, and migration playbook references.

Required proof:
- Deterministic `--dry-run --json` outputs for cluster and bundle paths
- Connected-mode integration test with `skipIfNotConnected(t)` guard
- Example fixture plus expected output committed

## M3: Git as First-Class Source + Fleet Ergonomics

Roadmap targets:
- Git <-> cluster compare.
- Git <-> Git compare.
- Fleet query provenance readability and impact clarity.
- Pattern backlog: `gitops.flux_kustomization_paths`.

Agent execution:
1. `Milestone-Owner Agent`: two tracks in parallel:
   - Track A: compare primitives (Git/cluster/Git).
   - Track B: fleet readability/impact UX.
2. `Contract Agent`: define compare JSON model and text rendering invariants.
3. `Implementation Agents` (2 in parallel): Track A and B.
4. `Example Agent`: add one multi-env fleet example with compare outputs.
5. `Test Agent`: pattern fixtures + compare golden snapshots + scale smoke.
6. `Docs Agent`: update query/fleet/howto docs and command matrix.

Required proof:
- Offline deterministic compare tests (no network)
- Fleet example validates readability claims
- No mutation semantics introduced

## M4: Testing Gate Contract and Coverage Enforcement

Roadmap targets:
- Testing gate contract (coverage matrix, CI-enforced gate, per-run proof artifact).
- CI-enforced coverage metrics.

Agent execution:
1. `Milestone-Owner Agent`: define minimum test matrix by change type.
2. `Contract Agent`: codify gate policy in docs + CI checks.
3. `Implementation Agent`: add CI workflow checks and failing thresholds.
4. `Example Agent`: ensure each user-visible command change maps to example coverage.
5. `Test Agent`: verify gate catches intentionally missing test/docs cases.
6. `Docs Agent`: update `docs/testing/README.md` and `docs/testing/BEST-PRACTICES.md`.

Required proof:
- CI fails when required test tier is missing.
- CI emits a per-run proof artifact (test matrix + pass/fail summary).
- Local reproducibility documented in `scripts/`.

## Execution Cadence

Use repeating 1-week cycles per milestone slice:

1. Day 1: issue scoping + success criteria freeze.
2. Day 2-3: implementation + fixtures.
3. Day 4: test hardening + degradation validation.
4. Day 5: docs + release notes + PR merge.

At most one in-progress milestone should change user-facing contract at a time.

## PR Template for Agent Slices

Each PR must include:

1. `Scope` (roadmap item(s) addressed).
2. `Success Criteria` (deterministic statements).
3. `Graceful Degradation` behavior.
4. `Tests Added` (unit/golden/integration/e2e).
5. `Examples Updated`.
6. `Commands Run` (with output summary).

## Mandatory Validation Commands

Baseline for every slice:

```bash
go build ./cmd/cub-scout
go test ./...
```

When relevant:

```bash
go test -tags=integration ./...
go test -tags=e2e ./cmd/cub-scout/...
./scripts/contract-audit.sh
```

## Milestone Exit Checklist

A milestone is done only when:

- Roadmap checklist items are implemented (or explicitly deferred with waiver).
- No stale docs/commands remain for the changed surfaces.
- Required tests pass in CI and locally.
- Examples and expected outputs are committed.
- Follow-up issues are filed for any deferred work.
