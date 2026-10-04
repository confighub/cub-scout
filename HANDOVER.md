# cub-scout execution handover

**Current snapshot:** 2026-10-04. Verified merged baseline:
[`5aeebc48`](https://github.com/confighub/cub-scout/commit/5aeebc48a6f168865b79d5a228013b9e576695f7).
The published release is v2.12.4; main includes newer, unreleased work.
Issue [#645](https://github.com/confighub/cub-scout/issues/645) is the live
execution queue. The adopted 3.0 plan below remains authoritative for work
order, quality gates, budgets and decisions.

## Investigation quality direction — 2026-10-04

The maintainer wants Scout to lead on read-only investigation quality for people
and agents, with no competing-product names in product documentation. The
updated `docs/reference/explorer-comparison.md` records five priorities and
success criteria: workflow continuity, scope safety, least privilege, complete
evidence and cheap reuse/demo. It corrects the older polling-only assessment
with the existing opt-in watch-backed inventory boundary. Operator assessment
remains #519; context/parity and efficiency retain their existing issue owners.
The maintainer subsequently confirmed that the roadmap must deliver a winning
product. Its outcome scorecard now sets operator diagnosis/onboarding targets,
scope and reuse proof, and P3–P6 delivery responsibilities alongside the existing
agent savings gates. Benchmark failures determine the next corrective work.
Permission profiles and scope-bound investigation history are explicitly indexed
as design follow-ups in the roadmap checklist. No runtime behavior, frozen
benchmark, v2.13 scope or release gate changed. Documentation/link/name checks
and `go test ./scripts/ci` passed; no live or paid tests ran.

## Steps 1–2 offline checkpoint — 2026-10-04

The topic packet adds enriched Explain explicit-context selection using one
captured Kubernetes binding, shared CLI/MCP/TUI snapshot output, exact namespace
and denied/partial-evidence handling. Defaults are preserved; API-version,
refresh and expected-revision remain bounded-only. Six deterministic contracts,
an example and a separate authored opt-in agent case cover this behavior without
changing the frozen 24-case experiment. Genuine UI/CLI acceptance remains pending.

The terminal adapter binds a selected source/stage/arm/control to captured trace
hashes, grades only one final successful terminal result with the exact selected
regex, and retains reported usage/cost as unreconciled metadata. The overlay
builder prepares and verifies exact selected-stage skills and sanitized plugin
metadata in a new read-only directory. Thirteen launch-policy contracts and
three authored Explain controls pass offline; the overlay/adapter have independent
review. Neither preparation establishes runtime enforcement, official evaluator
execution, trusted attempt provenance, descendants or billing attribution.

Cached build, full offline `go test ./...`, vet, read-only/CLI parity/name guards
and three manual-CI condition contracts pass. The first full Go run exposed the
new case's missing dedicated scaffold validator; its failed log is retained and
the exact-byte/inventory repair passes the full suite. Reviewed implementation
`0a0ef0e5` is pushed. Unit-only CI
[37192588839](https://github.com/confighub/cub-scout/actions/runs/37192588839)
passed Unit and Proof Artifact at that exact SHA. Downloaded proof confirms
50.0% coverage against 25.0% minimum and all five nonunit tiers skipped.
Later documentation checkpoints do not change that tested implementation.

The remaining scope is explicit in `docs/releases/v2.13-readiness.md`: 13 recorded
bindings, pinned runtime assets/actual enforcement, governance captures and
delivery-health contracts are still open. Full controller-desired operands and
broader context identity remain open; storage/publication belongs to the later
phase. SDK #758 stays deferred. Steps 1–2 are advanced, not fully closed; final
live gates, a published paired baseline and release remain pending.

## Dispatch guard and ChangeOrder read checkpoint — 2026-10-04

The source-bound launch candidate now prepares a PreToolUse dispatch guard in
both arms. It binds the selected case/control, immutable policy and exact tool
names; malformed or mismatched events produce a generic denial. Seventeen
launch-policy contracts include authored DEL-03/DEL-04 controls. This is local
stdlib/harness proof, not actual Claude hook enforcement. Python/runtime asset
admission, hook loss/timeout behavior and descendant accounting remain open.
The thirteen remaining recorded bindings have an explicit source-shape inventory
in `evals/full24-launch-policy/recorded-binding-gaps.md`; none is newly admitted.

`history changeorder <slug-or-id> --space EXACT --format ascii|json|md [--tui]`
and connected MCP `confighub_changeorder_get` share one conservative read
projection. One exact-space GET preserves reported Stage/State and raw workflow
prerequisite declarations; evaluated outcomes remain unknown. Completed does not
establish approval or runtime health. Parser contracts are pinned to SDK v0.6.8
source; no SDK dependency or Go-version migration was made. Deterministic tests,
a separate authored example/eval and dedicated scaffold validator cover identity,
malformed/duplicate responses, cancellation and failed-command stdout refusal.
Genuine cub/server recordings and live CLI/TUI acceptance remain pending; #597
stays open. Independent review found and repaired one stale tool count.
The first full Go run caught the missing read-only subcommand classification;
the one-line test-policy repair passes focused ChangeOrder/skill controls.
The CLI parity guard also caught a subcommand row in the top-level docs table;
that row was moved and the guard passes. Both failed logs are retained.

The fresh full-24 preparation validates current skill metadata. Its retained
`evals/full24-launch-policy/source-refresh-proof.json` distinguishes the archived
preparation from the refreshed source: only two treatment skill metadata files
changed, while frozen questions, inputs, grants, budgets, graders and authored
controls remain identical. Historical container/stdio proof is not revalidated
at the new metadata by this source-only comparison. No model, cluster or new
container execution occurred. Cached build/vet, the repaired full offline Go
suite, 17 launch-policy contracts, three authored ChangeOrder controls, three
CI condition contracts and read-only/CLI parity/name/diff guards pass.

Unit-only CI `37194543835` at `dcbb4789` failed the historical combined-runtime
source-staging test because the current skill metadata no longer matches its v1
pin. Its runner/contract/receipt remain unchanged. The repair reconstructs the
original pinned tree from an exact archived skill fixture and separately tests
current-source drift refusal; all 19 offline controls pass. This preserves the
historical proof boundary rather than admitting new metadata under old evidence.

Reviewed implementation `e0611783` is pushed. Manual unit-only CI
[37194709007](https://github.com/confighub/cub-scout/actions/runs/37194709007)
passed Unit and Proof Artifact. Downloaded proof matches the exact SHA, coverage
50.2% against 25.0% minimum, and all five nonunit tiers skipped. Later docs-only
checkpoints do not change this tested implementation. No PR/main merge or release.

## Offline runtime checkpoint — 2026-10-03

The user now requests all remaining v2.13 work except live tests. Current topic
branch: `codex/v213-offline-runtime`. No new container, cluster, ConfigHub,
registry or paid-model execution is included. Ordinary PR/main CI runs live
jobs, so do not open/merge this branch while that exclusion applies. Manual
`level=unit` CI has been corrected to select only Unit and Proof Artifact;
three condition/graph tests retain the normal PR/main and live-level behavior.
Reviewed code head `df68de81` is pushed. Manual unit-only CI
[37150688436](https://github.com/confighub/cub-scout/actions/runs/37150688436)
passed Unit and Proof Artifact; Integration, GitOps, Demo, Connected and Full
Verification skipped. These are pending live gates. Later documentation-only
checkpoint commits do not change that tested implementation.

`evals/full24-launch-policy/` preserves all 24 frozen prompts, ordinary grants
and source budgets in both arms, including both authored controls. Six offline
contracts pass. Eleven treatment cases have exact recorded object/export
bindings; the remaining 13 explicitly block, with no fabricated tool response.
Independent review's Python/runtime-asset prerequisite is explicitly held:
the candidate binds the wrapper source digest but requires independently pinned
Python/dependencies and inspected immutable assets before execution.
This is candidate policy, not actual tool enforcement or runtime admission.

The explicit recorded-only host stdio probe passed for all eleven bindings with
the actual local Scout binary: exact tool inventory, map provenance/count,
explain identity/hash and unsupported live-tool refusal. Request/reply hashes
and scope are retained in `evals/full24-launch-policy/local-recorded-proof.json`.
This does not establish the pinned Linux runtime, model tools/skills or full-24
MCP usability. Offline build, `go test ./...` and `go vet ./...` pass with cached
dependencies and an explicit empty kubeconfig. A separate temporary consumer
imports `/v2/pkg/agent` with a local replacement and classifies a Helm-labelled
Deployment correctly. Existing self-import/distribution guards pass; public
proxy installation awaits a `/v2` release tag and is not proved by this check.
Genuine governance captures, full-runtime/evaluator/process/cost admission and
the final release gates remain pending. SDK #758 remains deferred.

## Full isolation matrix checkpoint — 2026-10-03

The same reviewed probe/source at `5ff09940` now passes for all 24 ordinary
cases and both unweighted authored controls in both arms: 26 serial pairs,
52 unique owned containers, all verified absent. The initial INV-01 and DEL-03
control pairs were reused, not rerun; 24 remaining pairs passed first attempt
within the bounded matrix driver. Independent data review verifies every pair,
stage receipt, staged file and captured command output hash, common-byte parity,
treatment-only delta, unique ID, and cleanup receipt. Full summary:
`evals/full24-case-isolation/matrix-proof.json`; raw records:
`evals/results/full24-case-isolation-20261003/matrix/`. This expanded proof is a
local follow-up checkpoint, not part of the already inspected #767 CI head.
It establishes fixed-probe case-mount coverage only. No model/provider/MCP/
official grader ran; ordinary grants and descendant/accounting/cost remain
unproved. The next runtime implementation must preserve each case's exact
source grant (none grants Bash), case budgets and prompt, and replace the inert
MCP marker with a case-scoped recorded binding. Do not reuse the earlier combined
diagnostic's broader Read/Bash grants as full-24 grants. Genuine governance
recordings and final paid-run/release acceptance gates remain explicit.

## Selected-case isolation runtime — 2026-10-03

The user authorised further implementation and offline validation, with final
acceptance gates before release. Success is defined in #645 comment 5969082934.
The new `evals/full24-case-isolation/` runner uses the existing cached image,
serial invocation-owned containers, one exact selected `model-stage/` mount per
arm, no network, read-only root and fixed unprivileged/resource bounds. A fixed
non-model probe checks complete file hashes and direct/symlink/root write denial;
actual inspection precedes start and verifies terminal state. All bounded raw
command outputs, failed receipts and owned cleanup results are retained. Source
and stage integrity are checked before, after the probe and after cleanup.
Ten deterministic offline tests pass, covering all 24 cases/both arms and both
controls plus profile/final-state drift, lost create output, failure/timeout/
interruption, corrupted input, cleanup uncertainty and cleanup-time mutation.
Independent review approved the implementation and repaired proof coverage.
The source/probe commit is `5ff09940`; local fixed-probe proof passed for INV-01 and the DEL-03 authored control
in both arms: four actual containers, all cleaned up with absence verified.
See `evals/full24-case-isolation/local-proof.json`; all-24 coverage remains
offline/fake-Docker only. PR #767 merged at `5aeebc48` after inspected exact-head CI `37122926570`
at `36f672bf`: Unit, Integration, GitOps E2E and Proof Artifact passed;
optional Connected, Demo and Full Verification skipped.
Ordinary tool grants remain unenforced, treatment MCP inert, and model/provider/
official grader/process-cost admission unproved. Paid evaluation stays stopped.

## All-24 offline answer controls — 2026-10-03

The remaining eleven-case packet is independently reviewed. Five focused tests
and the complete twenty-one-test source-preflight suite pass. Five additional
selected tests covering the nine existing cases pass with no skips. The first
combined custom loader failed before tests because of a package import path;
its failed log stays retained beside the successful normal package discovery.
All 24 selected grader bytes and log hashes are independently checked in
`evals/full24-pair-preflight/offline-grader-control-proof.json`; raw records:
`evals/results/all24-grader-controls-20261003/`. Seventeen cases have local
Python/Node vector checks; seven have Python controls only. CI now also selects
the existing strict scale grader test. PR #766 merged at `3abfdd7f` after
inspected exact-head CI `37106806069` at `a52c7bf7`: Unit, Integration, GitOps
E2E and Proof Artifact passed; optional Connected, Demo and Full Verification
skipped. No frozen questions, weights, graders or fixtures
changed; official evaluator/target integration, actual tool/runtime enforcement,
usable full-24 recorded MCP, process accounting and attributable cost remain
admission gates. Paid evaluation stays stopped; v2.13 remains unreleased.

## Offline delivery-grader controls — 2026-10-03

A bounded read-only audit found direct selected-answer controls for 9/24 frozen
cases; capture/source validation does not count as answer-grader validation.
Fifteen selected graders lack direct acceptance controls. Admit one DEL-01–04
family test-only packet, success defined in #645 comment 5966621013. It must
bind canonical answers to frozen input facts, reject per-field wrong evidence
and malformed output, and cross-check local Python/Node matching with the exact
selected flags and `last_message` target. Questions, weights, graders and
source fixtures stay frozen. Implementation and independent review pass; six
new delivery tests and the complete sixteen-test source-preflight suite pass.
Local control engines are Python 3.14.4 / Node v25.9.0; copied log and engine
metadata: `evals/results/delivery-grader-controls-20261003/`. Separate exact-head
CI `37105851902` at `cb7b670b` passed all enabled Unit, Integration, GitOps E2E
and Proof Artifact jobs; optional jobs skipped. #765 merged at `025f3a71`.
That family supplies four of the selected-case controls. This does not establish
the official plugin grader engine, model correctness or full-suite admission.
Paid runs stay stopped.

## Offline baseline case staging — 2026-10-03

#764 merged at `fbe886d4`, with inspected enabled exact-head CI `37104777953`
at `b452a419` passing Unit, Integration, GitOps E2E and Proof Artifact. Optional
Connected, Demo and Full Verification skipped. The bounded source-only packet
is independently reviewed, including one
consolidated receipt/type/special-file repair. Eight offline guards cover all
24 cases in both arms, both authored controls, exact treatment delta, source
and receipt corruption, path/symlink hazards and oracle/sibling injection.
Actual CLI prepare/stage/verify and wrong-case/malformed-receipt rejection pass;
raw command records are retained at
`/private/tmp/scout-full24-case-stage-cli-wuf_qhlz/` and repaired tests at
`/tmp/scout-full24-case-stage-tests-repaired-20261003.log`. Copied command/test
records and stage receipt are retained under
`evals/results/full24-case-stage-20261003/`; CI includes the eight guards. Only `model-stage/` is a future
mount candidate; the host receipt and source preparation stay outside it.
The MCP marker remains inert. Tool/sandbox enforcement, usable MCP, selected
grader controls, runtime/process accounting and attributable cost remain gates.
No model, container, grader or server was executed; paid evaluation stays stopped.

## Full comparison context packet — 2026-10-03

#762 merged at `cf60006b`, closing #755, after inspected exact-head CI
`37102952826` at `2441718d`. Unit, Integration, GitOps E2E and Proof Artifact
passed; optional Connected, Demo and Full Verification skipped. Initial CI
`37102822312` stopped at unused refactor wrappers/test assignment; independently
reviewed dead-code/test-only cleanup passed targeted checks before the rerun.
The existing worktree was fast-forwarded to the merge without changing the
primary checkout. SDK #758 remains deferred; v2.13 is still unreleased.

#763 merged at `9c7e9578`: exact-case live-status app/unit-slug joins preserve
space-ID precedence and no-match omissions. Success was defined in issue comment
5966342944 before implementation. Red tests reproduced both cross-joins and
the shared CLI/TUI leak; fixed tests pass. Independent review, cached build/full
offline Go, exact scaffold and Python guard checks pass. Inspected exact-head
CI `37103880991` at `2d815ad2` passed Unit, Integration, GitOps E2E and Proof
Artifact; Connected, Demo and Full Verification skipped. Broader #561 remains
open. Readiness and remaining genuine governance/baseline
gates are recorded in `docs/releases/v2.13-readiness.md`.

Full #755 product wiring is implemented and independently reviewed. The repaired
offline build and full Go suite pass. CLI/MCP selectors and the shared scoped TUI
bind discovery, LIVE/source/link and rollout reads to one captured session;
View membership uses exact UnitID or space ID plus slug, with separate ConfigHub
authority. Missing/denied evidence remains partial. Review repaired stale pane
content, terminal-control handling and shared View eligibility. The initial full
suite exposed an intentional footer golden and fixture-owned scaffold routing
update; the rerun passes. The denied-source behavioral regression is retained.

The separately reviewed live helper has eighteen offline controls plus actual
Go viewport artifact validation. Immutable product/helper/probe pins and the
accepted owned capture are recorded below. Synthetic auth, Unit-read failure
and inert Application state cannot close genuine server governance, real
controller reconciliation or paid baseline gates.

Product pin is `478b1095359c6325d38e89c02e837861fc0f4410`; initial helper/probe
pin is `ae4a4d49118e79308aab237118ed762dd2c09340`. Owned attempt 1 failed an
overstated helper request contract: the existing tracer uses the complete
Application LIST object without a second GET. CLI output retained the correct
anchor and partial comparison. Failure receipt/raw logs are retained under
`evals/results/three-way-context-20261003/attempt-1/` and
`/tmp/scout755-owned-kind-proof-1/`. Owned cleanup, empty pending traffic and
all configuration-integrity checks passed. A narrowed exact helper contract
and separately reviewed rerun were required; attempt 2 below provides acceptance
for that bounded proof. Attempt 1 remains failed.

Attempt 2 passes and independent final receipt/report review accepts its bounded
scope. Product remains `478b1095`; corrected helper/probe pin is
`dc74eb0f4487c75d7cd1b0e8048fa5502ec107e9`. Ten phases comprise the actual
old-MCP ambient-routing control plus nine CLI/MCP/TUI allowed/source-denied/
Pod-denied phases. Each TUI exercised initial collection, real refresh and
same-scope edit, resize/scroll/back and cancellation. All comparison agreement
remains partial. Cleanup/config integrity and raw-log/report hash checks pass.
Summary: `evals/three-way-context-live/live-proof-report.json`; retained raw
receipts: `evals/results/three-way-context-20261003/attempt-{1,2}/`. Eighteen
offline helper controls are also wired into CI. The exact-head CI and merge
record above closes #755; local acceptance alone did not establish closure.

User granted another 300 credits: total ceiling 1,200, original baseline retained,
hard floor 1,731.922436 and dispatch stop 1,756.922436. First balance 2,048.4852735
leaves 316.5628375 under that ceiling; weekly included usage has reset. Admit one
bounded full #755 implementation with merged rollout/LIVE reader foundations,
then independent review, full validation and separately admitted owned proof.
SDK remains deferred; paid baseline/governance release gates are not waived.

## Captured LIVE reader — merged checkpoint

#761 merged at `94e6edf3`, tested head `efbac5ba`. Independent review, targeted
TLS/factory contracts, offline build/full Go and exact-head CI run 37026828296
passed. Enabled CI: Unit Tests, Integration Tests, GitOps E2E, Proof Artifact.
Connected E2E, Demo Tests and Full Verification skipped. One test-only redundant
assignment was corrected after lint caught it; production code was unchanged.

User requests continued progress toward v2.13. Latest shared balance
2,054.6347935 leaves 22.7123575 under the total 900 ceiling and crosses the
2,056.922436 stop-new-dispatch threshold. No further implementation/review/proof
packet is admitted. A budget extension is pending; initial 216.6857800 unexplained
shared decline stays conservatively charged. The SDK stays deferred. Full #755
scope/session/report/MCP/CLI/TUI wiring and remaining v2.13 governance/baseline
release gates are pending; no paid eval or release permission is inferred.

## Captured LIVE reader — implementation checkpoint

The bounded #755 LIVE/Git-source session foundation is implemented and
independently reviewed; targeted tests, offline build and full Go suite pass;
exact-head CI/merge remain pending. The existing ambient loader is unchanged.
The captured path binds workload, source tracing and ConfigHub link discovery
to the supplied session; partial summaries retain errors. Argo ambiguity and
multi-source omissions remain explicit; Flux cleanup failure discards the
anchor. Synthetic TLS/factory contracts are not real controller/server proof.
No public selector or invocation wiring was added; #755 remains open.

Latest shared balance 2,091.9568535 leaves 60.0344175 under the total 900-credit
ceiling. Reserve at least 25 before further dispatch. The unexplained initial
216.6857800 shared decline remains charged conservatively. No full-#755
completion forecast, SDK migration, paid eval, release or savings claim.

## Budget extension — 2026-10-02

User granted 300 more credits: total ceiling 900, original baseline retained,
hard floor 2,031.922436 and dispatch-stop threshold 2,056.922436. First balance
2,133.4559585 leaves 101.5335225 below that ceiling. The unexpected 216.6857800
shared-account decline since the previous check is being charged conservatively
pending attribution; no new implementation had been dispatched. Continue with
a bounded #755 shared-reader packet only after admission. SDK #758 remains
tracked and deferred; v2.13 context completion comes first.

## Rollout foundation checkpoint — 2026-10-02

- #760 merged at `7181d992`, tested head `d0d9aa32`. Extracted
  `fetchRolloutDecisionFrom` accepts a caller-bound dynamic client for workload
  and related Pod reads; the ambient wrapper and existing decision behavior
  remain intact. Independent review, offline build/full Go suite and exact-head
  CI passed (Unit Tests, Integration Tests, GitOps E2E, Proof Artifact).
  Connected E2E, Demo Tests and Full Verification were skipped.
- Deterministic two-endpoint TLS contracts cover endpoint/credential binding,
  no ambient read/config mutation, missing/denied workload, denied Pods and
  cancellation. Existing live-delivery example references the contract.
- This is only a safe shared-reader foundation for #755. No public context
  selector or comparison binding was added. Discovery, LIVE/Git-source reads,
  report adapters and scoped TUI integration still need one captured session.
- Balance snapshot 2,354.1507185 leaves 22.2282825 under the unchanged
  600-credit ceiling; it crosses the 2,356.922436 stop-new-dispatch threshold.
  Finish bookkeeping and stop. No new implementation/review/proof packet;
  no ceiling reset, paid eval, release or savings claim.

## Resumed execution — 2026-10-02

- #754 merged at `f2e708ac`; #750/#751 are closed. Its independently reviewed
  accepted source-truth/rendered-diff proof and retained failures remain at
  `evals/trace-context-live/combined-report.json`. Do not rerun without cause.
- #757 now has an accepted owned-kind CLI/MCP/TUI proof on attempt 6.
  [The report](evals/gitops-status-context/live-proof-report.json) pins product
  `00e1375e` and helper/probe `e7757e13`, retaining all artifact hashes and five
  failed attempts. All nine phases made 35 selected-endpoint GETs; real
  ModelDeployment/Pod 403s remain visible, with controller health preserved.
  Exact auth stub calls are CLI 1/MCP 3/TUI 1/old 0. Actual viewport actions,
  shared/upstream/observation config integrity and owned cleanup passed.
  This is synthetic controller evidence on a real kind API/RBAC, with local
  unauthenticated ConfigHub stub; no real controllers/server/governance, paid
  evaluation, terminal UX/refresh navigation or savings claim.
- Independent helper reviews repaired ambient-equals-selected configurations,
  snapshot/log cleanup races, unbound Home, padded Markdown validation, slow
  startup and CRD storage initialization. Eighteen offline controls pass; the
  pinned actual viewport/renderer integration passes. Raw attempts 1–6 are
  retained under ignored `evals/results/gitops-status-context-20261002/`.
  Attempt 4's uncertain node was separately identity-reviewed and removed;
  its original receipt remains failed beside `ownership-cleanup.json`.
- Product `00e1375e` passed build/full offline Go
  (`/tmp/scout757-resume-full-go-test.txt`); main merge `f4c65ae7` passed focused
  GitOps/source-truth and exact scaffold checks. Independent product review
  found no blocker. Independent final proof/report review passed. Exact product head
  `f156ac6e` passed enabled Unit/Integration/GitOps E2E/Proof Artifact in run
  `36983931044`; Connected/Demo/Full Verification were skipped. Results were
  inspected before the separate match-head merge call. #757 merged at
  `fbc8c733`, closing #753. This worktree was fast-forwarded to that merge;
  any newer checkpoint commits contain only handover/budget bookkeeping.
- Budget baseline remains 2,931.922436; latest balance 2398.824320 leaves
  66.901884 under the unchanged original 600-credit ceiling. Conservative
  decline is 533.098116. New work would have only
  41.901884 credits before the original no-new-dispatch threshold.
  Stop here rather than dispatch #755 without a defensible total-cost forecast
  covering implementation/review/full tests/owned proof. This checkpoint does
  not reset or increase the cap. Effective speed telemetry
  remains unavailable; no speed override was requested.
- #755 three-way context binding remains unimplemented. #599/#746 remain open.
  SDK #758 is deferred, not a v2.13 whole-SDK migration mandate. Its recorded
  candidate is source-truth's exact-space Unit/head-revision read; SDK v0.8.0
  requires Go 1.25 while Scout declares 1.24.
- v2.13 is not released. The paired baseline and genuine #591/#597 governance
  fixtures remain missing; paid evaluations remain stopped. The SDK inventory
  does not establish a permitted GET prerequisite-evaluation endpoint. Empty
  scoped order lists do not prove ungoverned state; no POST/promote/dry-run
  action calls are authorized. v2.12.4 remains the published release.

No local proof/build/full-suite process remains running. The merged-main CI
may continue remotely; the inspected pre-merge gate is complete. Raw receipts
and source pins are retained. No paid eval or v2.13 release was run.

## Historical laptop-close checkpoint — 2026-10-01

The following records the saved state before the resumed execution above.

- Main is `23e1ec4227ecf06b9add1b3da6e5bc44e797bb98`: #756 merged after
  independent review, build/full Go and all enabled exact-head CI passed.
  ConfigHub unit/target slugs now match case-sensitively; ID precedence remains.
- Draft #754 (`codex/trace-rendered-diff`, readonly-auth-provider worktree)
  contains source-truth and caller-rendered Trace comparisons. Product pin
  `fb3c76b6fc28d72f6ccea99fd9f38040c5b58b65` passes build/full offline Go.
  TUI source-truth now refreshes the ConfigHub session at each deliberate
  observation; its expired/recovered-session regression failed before the fix.
- The combined CLI / real MCP subprocess / actual TUI proof **passed** at
  `/tmp/scout-source-truth-diff-proof-4`. All 13 action phases retained the
  selected endpoint and expected read outcomes. Cluster deletion, node absence,
  credential/source-worktree removal and shared-config integrity passed.
  [The derived report](evals/trace-context-live/combined-report.json) preserves
  source/artifact hashes and the three failed attempts. Raw receipts are also
  retained under ignored `evals/results/source-truth-diff-20261001/` in this
  worktree. This is synthetic Application evidence on real kind API/RBAC, with
  a deliberately failing ConfigHub stub; no controller reconciliation, real
  governance evidence, model run or cost-savings claim.
- Main was merged into #754 at `2b38101a`; the only conflict was eval fixture
  routing, resolved by retaining all cases. The exact scaffold check passes.
  Resume with independent derived-report review and final exact-head CI before
  making #754 ready or merging. The tested proof pin remains immutable.
- Draft #757 (`codex/gitops-status-context`, doctor-scan-context worktree)
  is saved at `00e1375e`, including runtime-omission repair `93fe6fb8` and main.
  Focused tests and before/after regressions pass; build/full Go, owned proof
  and final exact-head CI remain pending. The merge retained both eval routes.
- #755 is the next planned three-way comparison binding packet, not started.
  Its cheaper-model design audit covers discovery, LIVE reads, Git-source and
  rollout enrichment, plus the missing TUI entry. See the issue for budget.
- Updated integration findings from newer cub-argo/cub-flux and Sveltos sources
  are on #645 (comment 5932706485). Plugin status reporters write ConfigHub;
  Scout must read evidence rather than invoke those reporters.
- v2.13 is **not released**. Its baseline and genuine governance fixtures are
  still missing. Paid evals remain stopped; no new model/provider run is
  authorized. Keep shared clusters/configuration and retained failed evidence.

No local cluster proof or full-suite process remains running at this checkpoint.
GitHub CI may continue remotely after pushes. Closing the lid does not complete
any pending release gate. Resume from these branches and receipts, not a new run.

## Current work and evidence

- [#744](https://github.com/confighub/cub-scout/issues/744) merged after its
  required CI passed; #742 is closed. The reviewed baseline contains ten
  guards and the source-only full-24 preparation, with no paid admission.
- [#743](https://github.com/confighub/cub-scout/issues/743) is closed after
  doctor/live-scan context packet [#745](https://github.com/confighub/cub-scout/pull/745)
  merged under #599. Context selection is an
  invocation-local Kubernetes binding; its context-name label is not a stable
  observed cluster identity. Other surfaces still need their own complete
  bindings, and this slice does not close #599.
  [PR #745](https://github.com/confighub/cub-scout/pull/745) carries the implementation,
  denial/partial/unreachable and concurrent-binding tests, and TUI scan reopening
  against a retargeted kubeconfig. Doctor and scan preserve the selected label
  and coverage warnings across their outputs; normalized scan fields are additive.
  The [owned-cluster proof](evals/doctor-scan-context/README.md) passed on its
  second attempt: eight CLI observations plus actual TUI S/close/S after private
  config retargeting and a separately bound denied scan. Pod reads returned
  200/200/403; each TUI action made seven GETs. Cleanup, credential removal and
  shared/CLI-config integrity passed. The first validator-failure receipt remains
  preserved. Fixed product source `5b362975` passed local build/full Go validation;
  23 offline helper/recorded-case guards pass. The new file-only agent contract
  case has captured fixtures; paid agent execution remains unrun under the stop. CI's E2E package exhausted its 120-second limit
  as the final smoke test began, so the limit is now 180 seconds. Final head
  `07e22b94` passed Unit, Integration and GitOps E2E in run `36852331432`;
  optional Connected/Demo/Full Verification were skipped. Two
  behavior-specific regression probes caught missing shared Doctor context/human
  warnings and missing scan JSON/TUI context before the corresponding repairs.

- [#747](https://github.com/confighub/cub-scout/issues/747) is closed after
  [#748](https://github.com/confighub/cub-scout/pull/748) merged. The shared
  resolver no longer supplies an auth-provider config writer. A test-registered
  provider refreshed credentials for actual local TLS requests without changing
  the source kubeconfig; the old resolver rewrote the isolated fixture. Local
  full Go tests, independent review and exact-head CI (`36853834498`) passed.
  This removes a latent capability; the shipped binary links no legacy provider
  plugin. It does not sandbox exec-auth helpers.
- [#746](https://github.com/confighub/cub-scout/issues/746) is the current Trace
  context packet, **not complete**. The working branch has a shared captured
  observation engine, a writer-based full human renderer, direct bound Argo
  Application reads and private Flux child kubeconfig support. CLI normal Trace
  and TUI T/Enter actions use that model. Local two-server tests retain context A
  after the source kubeconfig points to B. File-backed TLS/token credentials are
  captured before clients are created; exec helpers retain their refresh behavior.
  Static proxy provenance comes from the same parsed configuration snapshot,
  never callback sampling. JSON now retains context labels and partial warnings.
  Explicit CLI/MCP selectors and reverse observations are now implemented,
  with partial timing/Secret/artifact evidence preserved. Reverse Secret saved
  manifests are omitted explicitly. Generic Argo source summaries preserve the
  declared target revision without fabricating ownership evidence; Flux artifact
  JSON retains its existing shape.
  The [MCP process proof](evals/trace-context-binding/README.md) passed its final
  local-fixture capture: four GETs, zero ambient Beta requests, explicit denied
  evidence and verified private cleanup. It exercises the real stdio server and
  child processes, not a live cluster or real RBAC. Binary/source association
  limits and earlier attempts remain recorded. Its three Python tests and the
  exact recorded-case scaffold guard pass; paid agent execution remains unrun.
  The first combined full-Go run found two Flux golden changes and a scaffold
  mismatch; both are repaired and targeted tests pass. The fresh build and full offline Go suite pass at `24074d85`. The
  [owned-kind proof](evals/trace-context-live/README.md) passed on attempt three:
  allowed/denied CLI reads and actual TUI open/reopen/denied actions, with
  six/six/one GETs and retained 403 evidence. Cleanup and config integrity pass.
  Attempt two exposed a real selected-workload identity bug, now fixed with
  a failing-before regression; both failed attempts remain retained. Full
  controller-desired diff remains open. The foundation merged in
  [PR #752](https://github.com/confighub/cub-scout/pull/752), after final review
  repaired reverse Application namespace resolution. Exact head `4d3f28e9`
  passed full local Go validation and enabled Unit/Integration/GitOps E2E/Proof
  Artifact CI in run `36865407426`; Connected/Demo/Full Verification were skipped.
  Explicit context with
  legacy delegated diff fails before reads. A context label is not a stable
  cluster ID; this packet does not close #599.

- [PR #749](https://github.com/confighub/cub-scout/pull/749) supplies the missing
  Trace Markdown unit-event rows under #561, preserving Result/Status and
  missing values. Independent diff review, offline build and full Go tests pass
  at `7c68ea66`. Enabled exact-head CI checks passed in run `36860479725`;
  optional Connected/Demo/Full Verification were skipped. PR #749 merged at
  `f05716a1`. Its example is typed fixture output,
  not a new server capture. The broader #561 scope stays open.

- Source-truth [#750](https://github.com/confighub/cub-scout/issues/750) and
  rendered-file diff [#751](https://github.com/confighub/cub-scout/issues/751)
  are integrated locally. Reviews repaired declared-versus-observed Argo
  revisions, mixed LIST/GET snapshots, the source-truth MCP executable route,
  scope-discovery cancellation, and failed-read timestamp rendering. Focused
  regressions pass. The combined full suite found only a stale Trace-picker
  snapshot; the correction at `d6e0e26b` passes the final build/full Go suite
  (command package 143.357 seconds). One fixed-source CLI/MCP/TUI owned proof is being prepared, with ConfigHub
  reads explicitly mocked as failures; it cannot establish genuine server
  behavior or controller reconciliation. Live proof and CI remain open.
- GitOps-status context binding [#753](https://github.com/confighub/cub-scout/issues/753)
  is the next bounded implementation, in a reused clean worktree. Its worker
  forecast is 30k input/6k output, ceiling 65k/10k, one implementation plus one
  repair. The separate combined-proof worker forecast is 40k/7k, ceiling
  85k/12k. These advisory token envelopes are not billing or credits.

- The fixed benchmark retains 24 questions and six equally weighted groups.
  This checkout has **0 planned, 5 refreshed fixtures, 2 prepared recorded
  bindings, 8 prepared projections, 7 prepared raw recordings and 2 prepared
  synthetic source replays**. These are preparation statuses, not model
  execution or admission. Older-case grader and runtime gates remain open.
- [PR #727](https://github.com/confighub/cub-scout/pull/727) integrates three
  independently reviewed cases in one final CI cycle and has merged. PRE-02's owned
  node-selector capture passed in 38.575 seconds with cleanup and unchanged
  shared configuration: scheduling is not application health.
  [RUL-01](evals/rul01-dated-snapshot/README.md) reuses that exact Pod response
  and its request receipt; authored clocks yield 300/600 seconds at one-second
  timestamp resolution, without renewing freshness. It is not a second capture.
  [RUL-02](evals/rul02-cache-replay/README.md) retains the actual 3.837-second
  local production-cache replay; authored inputs are distinct from served
  responses and returned results. Valid hits preserve observation times;
  refresh/expiry fetch changed identity/content; failed refresh evicts success.
  No push invalidation, live freshness or cost-saving claim follows.
- [PR #728](https://github.com/confighub/cub-scout/pull/728) passed required
  checks and merged the historical Helm-experiment Argo parent/child/Pod case.
  Parent health does not establish child health. Its source pins, sequential
  capture, missing response times and conflicting cleanup fields remain explicit.
  This does not establish Crossplane coverage or a complete/current graph.
- [PR #723](https://github.com/confighub/cub-scout/pull/723) merged the reviewed
  [HLT-04 synthetic producer replay](evals/sveltos-hlt-04-report-freshness/README.md).
  Eight controls passed at a pinned Sveltos integration source; a report renewal
  is not proof of a new health check, exact applied digest or live controller run.
- [PR #733](https://github.com/confighub/cub-scout/pull/733) merged the bounded
  PRE-01 API replay and actual Linux version/ABI proof. Four pinned binaries
  passed in 4.610 seconds inside an inspected network-none container; cleanup
  and unchanged inputs were verified. Both earlier failed attempts remain
  recorded. This proves version execution, not installation or tool parity.
- [PR #734](https://github.com/confighub/cub-scout/pull/734) merged opt-in strict
  answer contracts for five legacy cases, preserving historical defaults and
  results. [PR #736](https://github.com/confighub/cub-scout/pull/736) merged the
  bounded PRE-01 real-kubectl gate. [PR #737](https://github.com/confighub/cub-scout/pull/737)
  also merged after its required checks passed.
- [#732](https://github.com/confighub/cub-scout/issues/732) passed its first
  real-kubectl recorded-response gate at source 75ef04b9 in 3.503 seconds.
  Six exact reads: three captured 404s produced normal exit-1 errors; three
  captured 200s returned byte-exact responses. Both listeners and the owned
  container were cleaned up; configured isolation and input hashes verified.
  Eighteen offline guards passed, with old-source assertion failures. Proof
  packaging merged in #736. No general discovery, Helm, MCP, model or savings proof.
- [#735](https://github.com/confighub/cub-scout/issues/735) fixes cross-namespace
  watch-cache reuse. The owned-kind before/after probe passed on one unchanged
  fixture: the pinned old source showed exactly four expected false-empty/error
  mismatches, and the fixed source passed all six checks with matching direct
  identities and preserved denials. Cleanup and shared-config integrity were
  verified. The concise report binds the local raw archive in
  [the proof README](evals/watch-cache-namespace/README.md). Product fix and
  proof packaging merged in [PR #738](https://github.com/confighub/cub-scout/pull/738)
  after required checks passed; the private raw archive remains local. The Nodes case used an empty
  synthetic cluster-scope lister and observed a 403 fallback; it does not prove
  route validation or Nodes informer coverage. Stream freshness, store limits
  and reconnect/410 coverage remain under #539.
- [PR #739](https://github.com/confighub/cub-scout/pull/739) merged the
  [ordinary-tool query plan](evals/ordinary-tool-query-plan-v1.md). None of the
  frozen 24 questions requires installed Helm release history. Inventory
  questions concern supplied rows, with original-cluster completeness unknown.
  Equal meaningful tool access remains required; every tool need not be called.
- [#740](https://github.com/confighub/cub-scout/issues/740) now has a passed
  combined offline two-arm diagnostic merged in PR #741 at source 1b80b8c, in 9.091
  seconds. Both real CLIs completed required tools; treatment advertised 35
  skills and produced one complete validated recorded map result. Both owned
  containers were removed and input hashes verified. All four earlier failed
  attempts remain retained in the
  [diagnostic report](evals/reports/2026-10-01-combined-runtime.json). Eighteen
  offline tests pass. This is not full24 execution, skill-use value, billing,
  savings evidence or paid admission; those gates remain stopped.
- General ordinary-tool parity, API/Helm route coverage, complete descendant
  accounting and paid admission remain unresolved. External #591/#597/#600
  still require their own authoritative evidence and agreement.
- Earlier evidence remains bounded: [#715](https://github.com/confighub/cub-scout/pull/715)
  records distinct denied/readable contexts (403 is unknown, not empty);
  [#716](https://github.com/confighub/cub-scout/pull/716) separates one mock Read
  round from opaque `num_turns: 2`; [#721](https://github.com/confighub/cub-scout/pull/721)
  preserves the original failed container receipt and separate exact-ID absence
  proof. HLT-02/RUL-04 are sequential observations; runtime image identity does
  not supply missing intended identity. The named Helm matrix does not close
  #588's hooks, CRDs, rollback, conflicts or default-apply behavior.

## Evaluation status

Paid evals are stopped. The ledger reports **$4.38666035 estimated** inclusive
list-price spend (smoke $1.91745385, live-only $2.4692065, baseline $0); the
latest diagnostic's complete spend is unresolved, so this is not a certified
all-in bill. The $200 baseline tranche is unspent; account credits and
development cost are unmeasured. No savings result is established. Byte counts,
tokens and aggregate goal telemetry do not certify product savings.

Do not rerun or add paid evals until offline review admits the full Experiment A
arms: the same pinned Claude and equal ordinary read-only file, kubectl and
Helm access/evidence, plus the treatment's 35 Scout skills and recorded MCP
surface. File-only mocks are narrower and do not satisfy that baseline. Also
require actual tool inventory/use, safe runtime and process boundaries, turn
limits, descendant completion and complete retry/judge/subagent/mock accounting.
Preserve failed and partial attempts with all available costs. Binary answer
verification requires every mandatory positive-weight non-tool grader exactly
once and passing; missing or malformed evidence is unknown. Tool-use indicators
never determine answer correctness. The plan's quality and cost gates remain
fixed; prepared cases and reduced output bytes do not relax them.

## Operating boundaries

- cub-scout product workflows are deterministic and read-only. They do not
  mutate clusters or make ConfigHub/Pilot decisions. Standalone never publishes
  facts; connected publication has a separate explicit write boundary under
  the adopted D5/D6 gates. Owned
  disposable test/capture harnesses may bootstrap and clean up only their
  explicitly marked resources under the adopted packet. Do not use ConfigHub
  promotion/action commands; every promotion argv, including dry-run, remains
  prohibited. Pilot is the separate acceptance judge and ConfigHub is the
  authority. Product ownership/health detection is not model judgement. Keep
  the shared ConfigHub server, credentials, and context unchanged; no shared
  server write path is authorized.
- Parse explicit evidence. Missing data is unknown, not proof of absence,
  health, freshness, identity, ownership, execution, or success. Keep raw
  observations separate from derived conclusions, and preserve partial/error
  states.
- Select clusters with an explicit private kubeconfig and context. For offline
  Go tests use a verified empty kubeconfig **file**, e.g.
  `/tmp/scout-offline-validation.kubeconfig`; an empty `KUBECONFIG` value can
  select the shared default. Go tests that mutate clusters require the integration tag
  and a private kubeconfig. `kind create` may switch global kubectl context;
  owned capture scripts must use private config and explicit contexts and must
  verify shared config integrity. Never treat that shared context as disposable.
  Disposable test-cluster creation/cleanup is permitted only inside the owned
  capture boundary; it does not authorize product writes to a shared cluster.
- Preserve ignored evaluation archives and private traces; never inspect sealed
  run homes. Stop only processes created by your retained command/session, with
  verified ancestry and process-group ownership; never scan process names and
  kill a match. Prior process incidents and an unresolved ad-hoc recorded
  preflight session are documented in #654 and the relevant report. Do not
  assert a clean process environment without evidence.
- For Docker/kind wrappers, preserve the reviewed launcher path/argv0: resolving
  a symlink to a helper executable can change Docker behavior. Verify executable
  pins before invoking it. Capture scripts must refuse unowned resources and
  preserve failed evidence.
- Follow [AGENTS.md](AGENTS.md): use `./cub-scout`, read the current command
  contracts, build and test with an explicit offline config, and keep CLI/TUI
  semantics aligned. Check the installed `cub` CLI current help. The `cub gitops`
  command group, including `discover` and `import`, was removed in July 2026 with
  no replacement. cub-scout's `import --git-path` is a local preview and does not
  do SDK rendering. Do not claim unsupported commands or renderer behavior.
- Maintain source/input/output hashes, source and tool versions, scope,
  commands, timing, errors, omissions and cleanup receipts. Keep before/after
  observations distinct; never silently repair or overwrite failed evidence.
- Use one bounded normal-speed worker per settled packet, one implementation
  plus one repair, then independent review. No recursive delegation or maximum
  speed. Actual runtime limits are not established by requested settings alone.

The maintainer's standing authorization covers implementation, routine public
GitHub synchronization, and merging reviewed changes after required checks;
reviewed releases may proceed within the adopted release gates. It does not
authorize private ConfigHub publication or public/secret gists. Do not add
permission gates to already authorized routine work.

## Exact context frame checkpoint — 2026-10-04

The offline RUL-03 reader in `evals/recorded-api/context_frames.py` requires an
exact context, GET and raw path. It preserves the selected captured body,
403/200 status, endpoint/CA identity and source timestamps, with pinned metadata
and bounded regular-file checks. It does not default to the recorded readable
current context or fall back after a denied read. Five deterministic controls
cover exact responses, unavailable requests, missing/changed bytes, symlinks,
FIFO and oversized input; the combined recorded-api suite has 20 controls.
Independent review found no actionable defects. Full offline `go test ./...`
passed with the explicit empty kubeconfig. Implementation `bf728b93` is pushed;
unit-only CI [37199576055](https://github.com/confighub/cub-scout/actions/runs/37199576055)
passed at that exact SHA. Its downloaded proof confirms 50.2% coverage against
25.0% minimum, Unit and Proof Artifact success, and all five nonunit tiers
skipped. Later documentation checkpoints do not change that tested code.
This is source-selection preparation, not an MCP binding or model admission.
The full-24 count remains eleven candidates and thirteen blocked cases. No
frozen evidence, grants, budgets or product CLI behavior changed.

## Recorded-response MCP checkpoint — 2026-10-04

`evals/recorded-api/context_mcp.py` prepares an eval-only stdio evidence adapter
with one read-only `recorded_response` tool. Exact context/GET/raw path select
original response text plus capture provenance; a captured 403 remains source
data and does not become an empty inventory or a substituted readable context.
All four source files must pass pins before the tool catalog is served. Fixed
staged evidence paths, strict bounded JSON and message limits exclude dynamic
paths, live clients and executable dispatch. Five new transport controls bring
the recorded-api suite to 25. Independent review identified exponent-overflow
JSON numbers; finite-float parsing and bounded integers repair that gap, with
negative vectors. The product MCP catalog and map/explain semantics are unchanged.
The repaired 25-control suite and full offline `go test ./...` passed. Reviewed
implementation `387cee05` is pushed; unit-only CI
[37211787841](https://github.com/confighub/cub-scout/actions/runs/37211787841)
passed at that exact SHA. Its downloaded proof confirms 50.2% coverage against
25.0% minimum and all five nonunit tiers skipped. Unit Tests and Proof Artifact
both succeeded; later documentation checkpoints do not alter tested code.
The new tool is not granted by the frozen launch policy. RUL-03 remains blocked
pending tool/runtime admission; the count stays eleven candidates/thirteen
blocked. No live, container, model or provider execution is claimed.

## Source-bound context host preflight — 2026-10-04

`evals/full24-launch-policy/probe_context.py` binds only original RUL-03
treatment through the public stage/source verifier and reviewed adapter/evidence
pins. An explicit hash-selected host Python child runs the exact read-only
package with isolated Python imports and clean environment. Six actual stdio
requests passed initialization/catalog, exact 403/200 text/status/provenance,
implicit-context refusal and map-substitution refusal. The private attempt
retains all input/output bytes; `local-context-proof.json` records their hashes.
Source, package and interpreter revalidation passed after the child exited.
Independent audit verified the retained proof and all source/input/output pins.
Five pure controls pass, including failed/timed-out artifact retention, bringing
launch-policy coverage to 22 tests; full
offline Go tests passed (unit package 47.419s).
Implementation `2a498cfd` is pushed; unit-only CI
[37213160950](https://github.com/confighub/cub-scout/actions/runs/37213160950)
passed at that exact SHA. Downloaded proof confirms 50.2% coverage against 25.0%
minimum, Unit Tests and Proof Artifact success, and all five nonunit tiers
skipped. Later documentation checkpoints do not alter tested code.

The admission boundary is explicit: `recorded_response` currently transports
raw source evidence, not a product Scout inventory/diagnosis capability. It stays
outside the frozen tool grant, with RUL-03 still blocked and counts unchanged at
eleven candidates/thirteen blocked. Host stdio success does not prove runtime
dependency admission, OS/network containment, descendants, model execution or
savings. Those proof claims remain false. No live/container/model/provider run
occurred.

## Recorded DeploymentList product prerequisite — 2026-10-04

The shared recorded-object loader now accepts exact `apps/v1 DeploymentList`
responses, whose Kubernetes API declaration establishes Deployment item types.
Only omitted item type fields are supplied; explicit conflicts/null/blank types
refuse. Metadata, UID/resourceVersion/managedFields and raw source hashes remain
unchanged. Generic `v1/List` still requires explicit item types; denied Status
and empty recordings remain refusals. Optional input-wide
`typedListDerivedObjects` provenance flows through map/list/summary and explain
JSON, CLI/MCP and ASCII/Markdown/TUI. Zero-count outputs remain unchanged.

Deterministic Go controls exercise the actual pinned RUL-03 body and type/boundary
negatives, with independent review finding no actionable defect. The new
authored opt-in `evals/recorded-typed-list` has exact-byte/scaffold and strict
answer controls, runs in unit-only CI and leaves frozen cases unchanged. The
local binary passed recorded map/explain CLI and actual MCP stdio checks, retained
at `/private/tmp/scout-v213-typed-list-host-proof-20261004`; denial and unsupported
live-tool requests refused. Build, vet, focused controls and the repaired full
`go test ./...` pass offline. The first full suite found the new case missing
its dedicated scaffold registration; the repair pins original bytes and exact
output inventory, with independent review finding no actionable defect. Failed
and repaired logs are retained under `/tmp/scout-v213-typed-list-*20261004.log`.
Manual unit-only CI [37216359170](https://github.com/confighub/cub-scout/actions/runs/37216359170)
at implementation `aa841535691d6e01303c9f4b90fa1e44aa37f781` passed Unit and Proof
Artifact. Downloaded proof matches that exact SHA: coverage 50.2% / minimum 25.0%,
with Integration/GitOps/Demo/Connected/Full Verification skipped.
This product prerequisite adds no multi-context
joining or current-state claim. RUL-03 remains blocked and the Linux binary pin
has not been rebuilt/admitted for this new code; full24 stays eleven candidates
and thirteen blocked. No live/container/model/provider run occurred.

## Exact recorded-context inventory binding — 2026-10-04

`evals/recorded-api/context_inventory.py` now binds a full product recorded map
projection to one exact pinned RUL-03 context/GET/path response. It passes only
selected original bytes and exact namespaced Deployment scope to an injected
reader, then validates hash/size/count/type-derivation provenance, schema,
resource identities and owner histogram. Ownership remains reader output;
reader binary/runtime admission is separate. A recorded 403 returns unreadable
coverage and null inventory without invoking the reader or reading another
context's body. There is no current-context default, join, retry or fallback.

Success criteria were defined before implementation. Six deterministic controls
cover exact source/scope, denial without readable body, foreign report identity,
counts, malformed/bounded JSON, missing/changed source and reader failure. All
31 recorded-API controls, three workflow guards and full offline Go suite pass.
The existing recorded-loader example references these controls. Independent
source and retained host-proof review found no actionable defect. Actual local `./cub-scout` composition
with private empty HOME/kubeconfig passed; retained bytes/hashes live at
`/private/tmp/scout-v213-context-inventory-host-proof-20261004`. There was one
readable CLI call and zero denied calls.

Manual unit-only CI [37217238140](https://github.com/confighub/cub-scout/actions/runs/37217238140)
at implementation `c646c207eb504b122f50653e8a5fb8d67bc890dd` passed Unit and Proof
Artifact. Downloaded proof matches that exact SHA: coverage 50.2% / minimum 25.0%,
with Integration/GitOps/Demo/Connected/Full Verification skipped.

This is an eval-only binding, not a new product CLI/MCP/TUI surface or admitted
MCP tool. Existing transport module pins, frozen case inputs/grants/prompts and
budgets are unchanged. RUL-03 remains blocked on tool/runtime admission and the
new Linux binary pin; full24 remains eleven candidates and thirteen blocked.
Current-state/runtime admission flags stay false. No live, container, model or
provider run occurred; SDK #758 remains deferred and v2.13 is unreleased.

## Offline Linux arm64 build candidate — 2026-10-04

Two cached, download-disabled builds from a clean temporary local checkout of
`c646c207eb504b122f50653e8a5fb8d67bc890dd` produce byte-identical Linux/arm64
ELF binaries: 75,903,128 bytes, SHA-256
`feeb1ebc37d6ffed635588ecc30866b7a1027e79517cd4139a3a1f97c3aa0ba5`.
Go 1.26.2 build metadata reports CGO disabled, trimpath, exact Git revision and
`vcs.modified=false`. The embedded module pseudo-version is source metadata,
not a published version. [Build receipt](docs/releases/v2.13-linux-candidate.json)
and `/private/tmp/scout-v213-linux-candidate-20261004` retain verification,
metadata, logs and both binaries. Independent read-only audit found no defect.

The first managed-worktree build compiled but omitted VCS metadata because the
local Go VCS detector expects a `.git` directory; its unstamped artifacts remain
retained as rejected. Building the same commit in the temporary local checkout
provided the required source stamp. An initial verifier assumed `(devel)` module
metadata; it was corrected to validate the reported exact-commit pseudo-version.

The candidate contains recorded DeploymentList support but has not run on Linux
or been admitted by a runtime/tool gate. Historical runtime pins and accepted
proofs are unchanged. No target binary, Docker, live cluster, model, provider or
registry operation ran. Runtime execution remains deferred to the final gates;
this is a build candidate, not a release artifact or full24 admission.

## Prepared exact-context inventory MCP — 2026-10-04

The separate eval-only `context_inventory_mcp.py` presents one read-only map tool
with a mandatory exact recorded context and fixed request/scope. It reuses the
reviewed protocol envelope/initialization gate without enabling raw-response,
explain or live tools. Denial remains a successful historical evidence read with
HTTP 403, unreadable coverage and null inventory; the reader is never called.
Reader/source/report failure gives a generic refusal, and the whole outgoing
JSON-RPC envelope is bounded. Six pure protocol controls bring recorded-API tests
to 37; all pass, as does the full offline Go suite. Independent source and host
proof reviews found no actionable defect.

The actual host stdio/CLI composition retained at
`/private/tmp/scout-v213-context-map-mcp-host-20261004` has seven replies, map-only
catalog, one readable local Scout invocation and zero denied invocations. The
[public summary](evals/full24-launch-policy/local-context-map-proof.json) records
exact package/stream/interpreter/binary hashes and discloses modified source
state separately from its base revision. This is host composition, not runtime
containment or model-tool admission. Existing transport source pins, frozen
launch policy/questions/evidence/grants/budgets and product CLI/MCP/TUI surfaces
are unchanged. RUL-03 remains blocked; Linux Python/dependency/mount admission,
enforced reader wiring and descendant accounting remain open. Full24 remains
eleven candidates and thirteen blocked. No live/container/model/provider run.

## Sveltos controller reports — 2026-10-04

The #641 read-side packet preserves bounded ClusterSummary delivery features
and ClusterHealthCheck continuous-health conditions separately from the same
existing scoped list reads. Raw reported references/timestamps remain facts;
no name/time joins, release correlation, gate acceptance or check-execution
freshness is inferred. Workload health and check freshness stay unknown.
Missing identities/status, unsupported versions, invalid/truncated fields,
malformed arrays, source/entry caps and forbidden lists have deterministic
controls. Markdown/ASCII/TUI safely quote external metadata.

An authored [example](examples/sveltos-controller-facts/) and separate opt-in
answer case are outside the frozen 24. The local CLI/MCP/owned-PTY
[rendering proof](examples/sveltos-controller-facts/render-proof.json) uses an
authored summary with empty HOME/kubeconfig and offline mode; it establishes
propagation, not genuine controller reconciliation or live acceptance. No live,
container, model, provider or registry run occurred. Broader #641 server gates,
underlying check execution timestamps and later publication remain open.
Independent final source/rendering review found no remaining defects. Build,
repaired full offline Go tests, vet, 37 recorded-API controls, 22 launch-policy
controls, two authored-answer controls and three workflow guards pass. The
initial full-suite failure was the new scaffold using a nonstandard heredoc
delimiter; changing it to the shared validator delimiter repaired exact-byte
fixture checks. Initial failure logs remain retained.

## Static Linux Python asset candidate — 2026-10-04

A local cached-image export now binds OCI index, selected Linux arm64/v8
manifest/configuration, four compressed-layer hashes and diff IDs, effective
filesystem inventory, and Python 3.11 ELF/library/link hashes. The
[receipt](evals/full24-launch-policy/runtime-python-candidate.json) identifies
static evidence only; retained archive/audit/inventory live under
`/private/tmp/scout-v213-python-image-20261004`. No container or target asset was
executed and no registry/model/provider run occurred. Existing runtime admission
and historical pins stay unchanged. Python/dependency/mount wiring and actual
execution/enforcement remain final gates; the candidate reduces asset discovery
work, not acceptance scope. An independent read-only audit reconstructed the
6,215-entry inventory and verified all selected blobs/layers and Python assets
against the receipt without findings.

## Current implementation Linux candidate — 2026-10-04

Reviewed Sveltos source `5098762b0af1fea7bfea1e915dff90f2581db50b` has a new
[current Linux arm64 build receipt](docs/releases/v2.13-sveltos-linux-candidate.json).
Two offline CGO-disabled trimpath builds from a clean local checkout are
byte-identical: 75,977,288 bytes, SHA-256
`e59564148524cd0c131c26aeca591256d5a2f4490b692a0470e60a7cc782769a`.
Embedded Go 1.26.2 metadata names the exact revision and modified=false;
independent static audit accepted the binaries, clean checkout and receipt.
Retained files are under `/private/tmp/scout-v213-sveltos-linux-candidate-20261004`.
No target execution or runtime admission is claimed. Earlier build receipts and
historical runtime pins remain unchanged; this is not a release artifact.

## Verified implementation checkpoint — 2026-10-04

Implementation `5098762b0af1fea7bfea1e915dff90f2581db50b` includes the exact-context
map protocol and Sveltos facts. Exact-head manual unit-only CI
[37221782102](https://github.com/confighub/cub-scout/actions/runs/37221782102)
passed Unit and Proof Artifact. Downloaded
`/tmp/scout-v213-sveltos-ci-proof-37221782102/proof-matrix.json` matches the exact
revision, coverage 50.4% against minimum 25.0%, with Integration/GitOps/Demo/
Connected/Full Verification skipped. Full local offline Go tests, build, vet,
authored controls, recorded-API and launch-policy guards passed before push.
The source/rendering/static-asset/build receipts have independent acceptance.
No live gate, runtime/model admission, release or merge is claimed.

## Repeatable Python asset verification — 2026-10-04

`evals/full24-launch-policy/python_image.py` turns the retained one-off static
image audit into a repeatable, receipt-pinned offline verifier. Bounded no-follow
regular-file reads, exact OCI/platform/config/layer hashes and diff IDs,
whiteout-applied inventory, Python ELF/library/link comparison and metadata/
member/expanded-size controls refuse changed or unsupported assets. No archive
extraction, Docker, network or target execution occurs. Nine synthetic negative
and positive controls pass; independent source review accepted without findings.
The [host proof](evals/full24-launch-policy/runtime-python-verifier-proof.json)
retains actual historical-export success and truncated-copy refusal with exact
source/stream/interpreter pins. Its explicit runtimeAdmission/targetExecuted
flags are false. The initial directory-refusal test exposed fdopen preceding
fstat; fstat/regular-file checks now happen first, with the descriptor closed on
refusal. Initial exact-head CI `37224032996` caught personal checkout paths in
the public proof argv. Public paths now use explicit labels, with the exact
retained private proof digest and a source/path disclosure regression control;
the repository name guard passes locally. Historical candidate receipt and
launch-policy admission stay unchanged.

## Expanded maintainer execution authorization — 2026-10-04

The maintainer removed the agent credit-usage cap and instructed execution
through 2.14 or 3.0, releasing 2.13 on the way. Execute the adopted 3.0 plan with
incremental 2.13/2.14 releases once their gates pass. The removed cap concerns
agent execution spending; frozen per-case benchmark grants/budgets and quality/
publication gates remain unchanged. SDK #758 stays deferred. The earlier live-
test exclusion remains until the maintainer answers the pending clarification
about final live acceptance. No release, merge or paid-model admission is
inferred from static verification or spending authorization.

## PRE-03 declared-reference foundation — 2026-10-04

The eval-only `argo_child_reference.py` now projects the pinned parent target
reference separately from parent/child captured identities and controller
reports. One exact namespace/name reference and child tracking-id name the
parent; observed GVKs stay unknown because both captures omit top-level type
fields. Distinct UIDs do not establish a UID foreign key. Reported child resource
rows remain separate, bounded and historical, with no Pod/tree join or grader
answer. Bad/missing/drifted/ambiguous source or malformed identity/report refuses.
Seven controls bring recorded-API tests to 44; all pass. Independent source and
[host package/output proof](evals/recorded-api/local-argo-reference-proof.json)
audits accepted without findings. The read-only retained host package lives at
`/private/tmp/scout-v213-pre03-reference-host-final-20261004`. Initial temporary
fixture paths followed macOS `/var` symlinks and hit the shared safe-path refusal;
resolving owned temporary paths made negative tests exercise their intended
semantic failures. No frozen source bytes, questions, grants/budgets or launch
policy changed. PRE-03 product/MCP/runtime admission remains blocked; eleven
candidates/thirteen blocked remains the accepted count.

The [draft v2.13.0 notes](docs/releases/v2.13.0.md) describe candidate scope and
publication gates explicitly as unreleased; they do not announce a tag.

Unit-only CI `37224249514` at `362878c8` passed the corrected Python verifier
packet. CI `37224721435` at `f55dd11e` passed the PRE-03/draft-notes packet.
Both downloaded proof matrices bind their exact SHA, report 50.4% coverage
against 25.0% minimum and mark all five nonunit tiers skipped. Earlier failed
verifier CI remains history; neither passing run establishes live acceptance.

## P4 cluster identity/read-cost foundation — 2026-10-04

The shared `pkg/agent/cluster_identity.go` reader captures one copied REST
configuration and context label, then reads only the `kube-system` Namespace.
Verified identity requires its exact reported type/name and observed UID.
Denial, missing/malformed fields, timeout and unreachable API remain unverified
with bounded omissions; they do not establish ownership or health. Endpoint
projection omits user information, query and fragment fields. There is no
discovery, redirect, REST retry, identity cache or other-context fallback.

The transport meter counts attempts and consumed response-body bytes for this
reader, including errors. It excludes headers/wire bytes, authentication traffic
and unrelated clients; an opaque preexisting wrapper marks coverage partial.
Reads serialize with cancellable waiting for attributable deltas, wall duration
and reuse=false. The [loopback example](examples/cluster-identity-cost/README.md)
defines collision-name, copied-config, denied/malformed, cancellation, timeout,
retry/redirect, byte-count and concurrent controls without a live cluster.
Independent review identified the shared 2 MiB error-body limit bypassing the
identity reader's 64 KiB cap; an identity-specific transport cap now bounds all
statuses, with oversized 403/500 controls passing. Initial logs are retained.

This is a library foundation, not command output or whole-command cost reporting.
CLI/MCP/TUI integration, merge-safe references, connected Target alignment and
genuine live acceptance remain open in #599. Existing exact-object read budgets
are unchanged; adding identity reads to commands requires an explicit budget.

Ten deterministic identity/meter controls, their targeted race run, build, vet
and repository read-only/parity/name guards pass. The first full offline Go run
hit the existing 100 ms exec-auth helper-start/cleanup timeout under concurrent
build/race load; its log is retained. That test passed three isolated repeats,
then the final full `go test ./...` passed without overlapping build/race jobs.
Independent review accepted the cap repair and example without remaining
findings. Exact-head unit-only CI is still required for this new packet.

The follow-on `ObservedResourceIdentity` library constructor/key retains actual
API version and exact group/kind/namespace/name/object UID alongside observed
cluster-instance identity. Canonical keys omit served version but distinguish
clusters, API groups and recreated objects, with no context/server fallback or
GVK derivation. Unknown identity, missing timestamp, malformed metadata/scope and
invalid exported-key mutations refuse. The caller must supply served scope and
collect object/cluster evidence with the same captured client; the pure helper
cannot establish that association, atomicity or current state. Five authored
controls and the combined identity/meter/reference race run pass; independent
review accepted the source. The shared example explains collision, version and
degradation controls. CLI/MCP/TUI fields and connected identity remain open.
The follow-on build, vet, full offline Go suite and repository guards pass.
Initial identity implementation `9b9f68f3` is pushed; its exact-head manual
unit-only CI `37226008155` is in progress, not an acceptance claim.

## Resume here

Check #645 and the checkpoint above. Drive the adopted plan through 3.0,
releasing 2.13 and 2.14 as their gates pass. Continue current v2.13 work on
`codex/v213-offline-runtime`, preserving the current exclusion of live tests.
The eleven recorded host bindings do not admit full-24 model execution: remaining
case bindings, Python/runtime asset admission, actual grants, trusted official
evaluator terminal integration, descendant completion and cost attribution
still need bounded implementation and review. The terminal adapter and overlay
are prepared candidates; their runtime integration remains unproved. Do not invent Kubernetes
objects or evaluated server decisions to fill gaps. Context/Trace/GitOps packets
#750/#751/#753 and three-way #755 have merged; audit remaining parent scope
#599/#746 against current code before repeating completed work.
External #591/#597 still need genuine read-side governance recordings, and #600
needs storage/schema agreement. SDK #758 stays deferred; `/v2` is already in
source, with public proxy installation awaiting a release tag. Keep paid work
stopped until its explicit admission gates pass. Keep this branch unmerged while
normal PR/main CI would launch excluded live tests. Final acceptance and the
published paired baseline remain required before release; no gate is waived.

- [Adopted 3.0 execution plan](docs/roadmap-3.0-execution.md)
- [Tracker #645](https://github.com/confighub/cub-scout/issues/645)
- [Evaluation protocol and reports](evals/README.md)
- [v2.12.4 release record](docs/releases/v2.12.4.md)

## Historical record

The complete handover and plan as of source commit
[`9e3fa9b765170ad414801ffb5069c9eeb11b2fec`](https://github.com/confighub/cub-scout/tree/9e3fa9b765170ad414801ffb5069c9eeb11b2fec)
are immutable history, not current status:
[HANDOVER.md at that commit](https://github.com/confighub/cub-scout/blob/9e3fa9b765170ad414801ffb5069c9eeb11b2fec/HANDOVER.md) ·
[execution plan at that commit](https://github.com/confighub/cub-scout/blob/9e3fa9b765170ad414801ffb5069c9eeb11b2fec/docs/roadmap-3.0-execution.md).
Retrieve exact prior bytes offline with
`git show 9e3fa9b765170ad414801ffb5069c9eeb11b2fec:HANDOVER.md` or
`git show 9e3fa9b765170ad414801ffb5069c9eeb11b2fec:docs/roadmap-3.0-execution.md`.
