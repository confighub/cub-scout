# M1 Task Cards: Trace and Provenance Fidelity

This document operationalizes M1 from `docs/workflows/agent-milestone-plan.md` into executable task cards.
Use this as the day-to-day delivery queue.

Authoritative inputs:
- `docs/roadmap.md` (M1 targets)
- `CLAUDE.md` ("Pre-Coding Test & Success Proof Requirements")
- `docs/testing/BEST-PRACTICES.md`

## Immediate Needs

M1 is complete only when these four outcomes are delivered with proof:

1. Trace clearly distinguishes "ConfigHub via OCI" ownership.
2. Trace surfaces source staleness/sync signals when evidence exists.
3. Broken ApplicationSet generator links are surfaced as orphan lineage.
4. Scan cluster flow closes gap #200 (`cluster export -> cub-scan`) with safe fallback.

## Non-Negotiable Gates (Per Task)

Before any implementation code:

1. Define contract deltas and graceful degradation behavior.
2. Create/modify tests first (unit + user-visible golden + integration/e2e if applicable).
3. Prepare fixture/example proof path.
4. Record proof matrix with exact commands and expected assertions.

A task is not complete until:

1. The task proof set passes 2 consecutive reruns after fixes.
2. `go build ./cmd/cub-scout` and `go test ./...` pass.
3. Required docs/examples are updated.
4. Evidence is logged in `/tmp/cub-scout-regression/m1/<task-id>/`.

## Per-Agent Sequence (Strict Order)

### Milestone-Owner Agent Queue

1. `M1-00`: Freeze scope and acceptance criteria for `M1-T1..M1-T4`.
2. `M1-T1`: Approve proof matrix and contract notes before coding starts.
3. `M1-T2`: Approve proof matrix and contract notes before coding starts.
4. `M1-T3`: Approve proof matrix and contract notes before coding starts.
5. `M1-T4`: Approve proof matrix and contract notes before coding starts.
6. Gate close for each task only after proof rerun stability and docs updates.

### Contract Agent Queue

1. `M1-T1`: Lock trace wording/JSON expectations for ConfigHub OCI distinction.
2. `M1-T2`: Lock staleness/sync signal semantics and missing-metadata fallback.
3. `M1-T3`: Lock orphan-generator semantics (what is "broken", what is "unknown").
4. `M1-T4`: Lock scan cluster merge semantics (legacy runtime + cub-scan static).

### Test Agent Queue

1. `M1-T1`: Add failing/placeholder tests first for OCI distinction.
2. `M1-T2`: Add failing/placeholder tests first for staleness/sync signals.
3. `M1-T3`: Add failing/placeholder tests first for appset orphan behavior.
4. `M1-T4`: Add failing/placeholder tests first for cluster-export cub-scan path.
5. For every task: run proof loop (`run #1`, fix, `run #2`, `run #3`) and archive logs.

### Example Agent Queue

1. `M1-T1`: Prepare/update `bridge-confighub-oci` fixture/example path.
2. `M1-T2`: Prepare stale/out-of-sync Argo fixture and expected output sample.
3. `M1-T3`: Prepare broken ApplicationSet generator fixture.
4. `M1-T4`: Prepare representative cluster-export manifest fixture for static pass.

### Implementation Agent Queue

1. Implement smallest slice for `M1-T1`, then stop for verification.
2. Implement smallest slice for `M1-T2`, then stop for verification.
3. Implement smallest slice for `M1-T3`, then stop for verification.
4. Implement smallest slice for `M1-T4`, then stop for verification.
5. No scope expansion in fix cycles; only proof-failure remediation.

### Docs Agent Queue

1. `M1-T1`: Update trace docs for ConfigHub OCI distinction.
2. `M1-T2`: Update trace docs for staleness/sync semantics and fallback behavior.
3. `M1-T3`: Update lineage docs for ApplicationSet orphan behavior.
4. `M1-T4`: Update scan docs/roadmap notes for #200 closure status and fallback.

## Iterative Proof Loop (Mandatory for Every Task)

1. `Plan`: publish proof matrix and degradation notes.
2. `Red`: create tests first; capture initial failing/coverage-gap run.
3. `Slice`: implement minimal code for first failing proof.
4. `Verify #1`: run scoped proofs, capture pass/fail.
5. `Fix`: patch only failing proof areas.
6. `Verify #2`: rerun same proof set.
7. `Verify #3`: rerun same proof set again (stability check).
8. `Close`: run baseline (`go build ./cmd/cub-scout`, `go test ./...`), then docs/example checks.

---

## Task Card `M1-T1`: ConfigHub via OCI Trace Distinction

Scope targets:
- `pkg/agent/argo_trace.go`
- `cmd/cub-scout/trace.go`
- `pkg/agent/argo_oci_test.go`
- `test/ascii/trace/*`
- `test/fixtures/patterns/bridge-confighub-oci/*`

Proofs/tests to create before implementation:

| Tier | Create/Update First | Command | Expected Proof |
|---|---|---|---|
| Unit | `pkg/agent/argo_oci_test.go` | `go test ./pkg/agent -run TestArgoTracerConfigHubOCIDetection -count=1` | ConfigHub OCI classified distinctly from generic OCI |
| ASCII golden | `test/ascii/trace/testdata/argo*.json`, `test/ascii/trace/*.txt` | `go test ./test/ascii -run TestTrace_Argo -count=1` | User output clearly shows ConfigHub OCI provenance |
| JSON contract | `test/ascii/trace_json_test.go` + goldens | `go test ./test/ascii -run TestTraceArgo_JSON -count=1` | `deliveryStage`/`renderedFrom`/`originalSource` semantics preserved |
| Integration (if cluster available) | `test/integration/trace_lineage_test.go` (extension) | `go test -tags=integration ./test/integration/... -run TestTraceLineage -count=1` | Trace JSON remains parseable with lineage fields |

Agent handoff sequence:

1. `Contract Agent`: finalize wording and fallback for missing OCI metadata.
2. `Test Agent`: land failing/placeholder tests and golden fixtures.
3. `Example Agent`: stage bridge fixture updates.
4. `Implementation Agent`: patch trace classification/rendering.
5. `Test Agent`: run proof loop until stable.
6. `Docs Agent`: update trace references/examples.
7. `Milestone-Owner Agent`: approve only with proof logs + 2 stable reruns.

## Task Card `M1-T2`: Source Staleness and Sync Signals

Scope targets:
- `pkg/agent/argo_trace.go`
- `pkg/agent/flux_trace.go` (if shared semantics are adjusted)
- `pkg/agent/argo_trace_test.go`
- `test/ascii/trace/*`
- `docs/reference/health-failure-states.md` (if user-facing semantics change)

Proofs/tests to create before implementation:

| Tier | Create/Update First | Command | Expected Proof |
|---|---|---|---|
| Unit | `pkg/agent/argo_trace_test.go` cases for out-of-sync/stale evidence | `go test ./pkg/agent -run TestArgoTrace -count=1` | Status/signal mapping is deterministic |
| ASCII golden | stale/out-of-sync trace fixture + golden | `go test ./test/ascii -run TestTrace_Argo -count=1` | Output explains staleness/sync without ambiguity |
| JSON contract | trace JSON golden updates | `go test ./test/ascii -run TestTraceArgo_JSON -count=1` | Signal fields remain compatible and explainable |
| Integration | extend lineage/trace integration coverage | `go test -tags=integration ./test/integration/... -run TestTraceLineage -count=1` | Live-cluster trace does not regress |

Agent handoff sequence:

1. `Contract Agent`: lock signal vocabulary and partial-evidence fallback.
2. `Test Agent`: add tests first (including missing-metadata behavior).
3. `Implementation Agent`: implement minimal signal extraction/output.
4. `Test Agent`: run proof loop with consecutive reruns.
5. `Docs Agent`: update operator-facing status interpretation docs.
6. `Milestone-Owner Agent`: close on stable proof + docs parity.

## Task Card `M1-T3`: ApplicationSet Broken-Generator Orphan Detection

Scope targets:
- `cmd/cub-scout/tree.go`
- `cmd/cub-scout/tree_git_test.go`
- `pkg/agent/argo_trace.go` (if lineage confidence/fallback needs alignment)
- `test/fixtures/patterns/appset-multi-gen/*`
- `test/integration/trace_lineage_test.go` (or new integration file)

Proofs/tests to create before implementation:

| Tier | Create/Update First | Command | Expected Proof |
|---|---|---|---|
| Unit | orphan generator relationship tests in `cmd/cub-scout/tree_git_test.go` | `go test ./cmd/cub-scout -run TestBuildTreeGitJSON -count=1` | Broken generator link is surfaced deterministically |
| Pattern fixture | broken-link fixture under `test/fixtures/patterns/appset-multi-gen/` | `go test ./test/fixtures/patterns/... -count=1` | Fixture remains valid and catches orphan scenario |
| Integration | lineage integration test for appset orphan behavior | `go test -tags=integration ./test/integration/... -run TestTraceLineage_ArgoAppFromApplicationSet -count=1` | Runtime behavior aligns with unit semantics |
| Golden/user output | tree/trace output snapshots where applicable | `go test ./test/ascii/... -run 'TestTrace_Argo|TestTree' -count=1` | User-visible orphan message is stable |

Agent handoff sequence:

1. `Contract Agent`: define orphan vs unknown behavior boundary.
2. `Test Agent`: create failing tests and fixture first.
3. `Implementation Agent`: implement orphan detection in smallest viable slice.
4. `Test Agent`: iterative reruns to stability.
5. `Docs Agent`: update lineage/orphan explanation docs.
6. `Milestone-Owner Agent`: close with proof logs and fallback behavior confirmed.

## Task Card `M1-T4`: Close Scan Gap #200 (`cluster export -> cub-scan`)

Scope targets:
- `internal/scan/confighub_provider.go`
- `internal/scan/confighub_provider_test.go`
- `cmd/cub-scout/scan.go`
- `test/integration/scan_provider_test.go`
- `test/prove-it-works.sh` (scan provider section)
- `docs/roadmap.md` and scan reference docs for status/fallback text

Proofs/tests to create before implementation:

| Tier | Create/Update First | Command | Expected Proof |
|---|---|---|---|
| Unit | cluster path + fallback tests in `internal/scan/confighub_provider_test.go` | `go test ./internal/scan -run TestConfighubScanProvider_ScanCluster -count=1` | Legacy runtime findings preserved; static findings added only when valid |
| Integration | provider behavior tests in `test/integration/scan_provider_test.go` | `go test -tags=integration ./test/integration/... -run TestScanProvider -count=1` | CLI/provider behavior remains correct with/without cub-scan |
| Golden/contract | scan JSON/normalized output checks | `go test ./test/golden/scan-file/... ./test/golden/scan-normalized/... -count=1` | Output shape unchanged except intentional additions |
| Example/proof script | provider checks in `test/prove-it-works.sh` | `./test/prove-it-works.sh --level=integration` | Documented proof path exercises provider detection safely |

Agent handoff sequence:

1. `Contract Agent`: finalize merge/fallback semantics and exit-code expectations.
2. `Test Agent`: add tests first, including export failure and binary failure cases.
3. `Implementation Agent`: patch cluster export + cub-scan invocation/merge path.
4. `Test Agent`: rerun scoped proofs to stability.
5. `Docs Agent`: update roadmap/docs to reflect actual status and limitation text.
6. `Milestone-Owner Agent`: close only with stable proof logs and docs sync.

---

## M1 Completion Checklist

1. All task cards (`M1-T1..M1-T4`) marked complete by Milestone-Owner.
2. Each task has proof logs under `/tmp/cub-scout-regression/m1/<task-id>/`.
3. Each task shows at least 2 consecutive passing reruns of its scoped proof set.
4. `go build ./cmd/cub-scout` and `go test ./...` pass after final merge order.
5. No stale docs remain for trace/scan behavior changed in M1.
