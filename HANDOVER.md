# cub-scout execution handover

**Current snapshot:** 2026-10-02. Verified merged baseline:
[`f2e708ac`](https://github.com/confighub/cub-scout/commit/f2e708ac43c9fedf068569acc97aae351163c737).
The published release is v2.12.4; main includes newer, unreleased work.
Issue [#645](https://github.com/confighub/cub-scout/issues/645) is the live
execution queue. The adopted 3.0 plan below remains authoritative for work
order, quality gates, budgets and decisions.

## Resumed execution — 2026-10-02

- #754 merged at `f2e708ac`; #750 and #751 are closed. Independent final
  report review verified the immutable proof pins, request/receipt hashes,
  cleanup and all three retained failures. Exact-head CI `36871797848` passed.
  A redundant helper rerun timed out in two 2-second subprocess checks; the
  merge tool batch incorrectly continued before that result was inspected.
  The unchanged rerun passed all 30 tests in 2.028 seconds. Both outcomes and
  the sequencing error are recorded on #645 (comment 5946755724); the failed
  run does not replace or invalidate the separate accepted live receipt.
- #757 build and full offline Go suite passed at `00e1375e`, with log
  `/tmp/scout757-resume-full-go-test.txt`. That head's enabled CI also passed
  (`36871814280`). Main was merged at `f4c65ae7`, retaining all four relevant
  eval scaffold routes. The post-merge build, focused GitOps/source-truth tests
  and exact scaffold guard pass (`/tmp/scout757-main-merge-focused.txt` and
  `/tmp/scout757-main-merge-scaffolds.txt`). Owned-kind CLI/MCP/TUI proof,
  helper/report review and final-head CI remain open. No new live proof or paid
  evaluation is claimed.
  The new `evals/gitops-status-context/capture_live.py` is an explicitly disabled
  draft: `--execute` refuses before side effects, covered by an offline test.
  Seven offline acceptance controls pass. Before admission, repair private
  HOME/XDG/restricted PATH isolation and expected `cub` auth calls, then review
  exact API paths/query handling, MCP responses, cleanup and the TUI probe.
  The bounded cheaper-worker pass stopped here; do not remove the refusal as
  a shortcut or describe these draft controls as live evidence.
- SDK adoption is tracked in [#758](https://github.com/confighub/cub-scout/issues/758).
  A bounded cheaper-agent inventory selected source-truth's exact-space
  Unit/head-revision read as the first shared CLI/MCP/TUI candidate. SDK
  v0.8.0 requires Go 1.25, while Scout declares 1.24. Typed read-only adapters,
  deliberate auth refresh, immutable observation sessions, scope/cancellation
  tests and overhead measurements precede adoption. No SDK dependency or
  implementation has been added. This is not a wholesale v2.13 migration gate
  or proof of agent dollar/credit savings.
- #755 and the v2.13 paired baseline/genuine governance fixtures remain open.
  Paid evals remain stopped. #599 and #746 are not complete.

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

## Resume here

Check #645, then continue Trace [#746](https://github.com/confighub/cub-scout/issues/746)
and its explicit rendered-operand diff packet #751, followed by source-truth
context binding #750 under #599. #743 is merged. Source-truth is committed
locally with reviewed repairs and focused tests passing; combined validation
and live proof remain. Continue the bounded GitOps-status packet #753 next.
The immediate target is v2.13, subject to the adopted gates. The full-24
source preparation under #742 is integrated, but its blinded equal-evidence
packet and authored input controls do not admit paid execution: recorded MCP
binding, actual grants, descendants and accounting remain open. Product work
need not wait for paid benchmark admission. #735/#738 and #740/#741 are merged.
Continue actual tool parity and process/cost accounting under #709; six raw
kubectl reads are only one bounded prerequisite. External dependencies #591 (genuine attestations), #597
(current gate evidence), #600 (fact storage/schema agreement), and GHCR access
remain unresolved. Keep the benchmark non-executable and paid work stopped while
tool, evidence, process or cost gates remain unresolved. The adopted plan's
ordered packets and external dependencies control next steps. Replace this
current checkpoint in place at the next handover; retain run chronology in
issues, PRs, reports and Git history rather than prepending dated snapshots.

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
