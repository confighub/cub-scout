# cub-scout execution handover

**Current snapshot:** 2026-10-01. Verified merged baseline:
[`572bc71`](https://github.com/confighub/cub-scout/commit/572bc71e40b6f5f229844f25922c6df04fe0d0ef).
The published release is v2.12.4; main includes newer, unreleased work.
Issue [#645](https://github.com/confighub/cub-scout/issues/645) is the live
execution queue. The adopted 3.0 plan below remains authoritative for work
order, quality gates, budgets and decisions.

## Current work and evidence

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
  combined offline two-arm diagnostic in PR #741 at source 1b80b8c, in 9.091
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

Check #645, then complete #735 exact-head CI and merge its reviewed behavior
fix and already-passing owned before/after proof package.
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
