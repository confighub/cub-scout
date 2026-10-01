# cub-scout execution handover

**Current snapshot:** 2026-10-01 05:38 UTC. Verified main baseline:
[`8e39ef7`](https://github.com/confighub/cub-scout/commit/8e39ef78238e93768666e0ca6d65a3e7145cf34b).
The published release is v2.12.4; main includes newer, unreleased work.
Issue [#645](https://github.com/confighub/cub-scout/issues/645) is the live
execution queue. The adopted 3.0 plan below remains authoritative for work
order, quality gates, budgets and decisions.

## Current work and evidence

- The fixed benchmark has 24 questions in six equally weighted groups. Main
  currently has **5 planned, 5 refreshed, 2 prepared recorded bindings, 7
  prepared projections and 5 prepared raw recordings**.
  [PR #715](https://github.com/confighub/cub-scout/pull/715) merged as
  [`8e39ef7`](https://github.com/confighub/cub-scout/commit/8e39ef78238e93768666e0ca6d65a3e7145cf34b)
  with Unit, Integration, GitOps E2E and Proof Artifact checks passing; Full
  Verification, Connected E2E and Demo Tests were skipped. Its
  [RUL-03 capture report](evals/reports/2026-10-01-rul03-context.json) and
  [case README](evals/rul03-context/README.md) retain source pins and scope. The
  capture records distinct denied/readable contexts and sequential responses.
  A 403 means unknown, not empty. The fixture is prepared, not a model run.
  All counts are preparation statuses, never execution/admission.
- Issue [#717](https://github.com/confighub/cub-scout/issues/717) prepares an
  offline HLT-04 producer replay pinned to
  `confighub/sveltos-confighub@8187910f9fe226e109e55c4d9c7c0e21297ff424`.
  The first network-denied producer replay passed in 2.844 seconds at helper
  source `6a13aee`, with eight synthetic controls and verified owned cleanup.
  The retained evidence is in independent review; packaging remains pending.
  It is not raw live evidence, a real check-execution record, or a model run.
- [PR #716](https://github.com/confighub/cub-scout/pull/716) merged the separate
  [single-round analysis](evals/reports/2026-10-01-direct-cli-round-accounting.json):
  one observed Read round, with terminal `num_turns: 2` kept opaque. The original
  harness failure is preserved. Named Task/Agent refusals are mock evidence;
  full treatment and complete paid accounting remain unadmitted.
- Issue [#718](https://github.com/confighub/cub-scout/issues/718) prepares an
  owned offline container boundary for broader eval tools. It has not run.
- Recent raw evidence is linked from the [execution plan](docs/roadmap-3.0-execution.md)
  and #645. HLT-02 and RUL-04 observations are sequential, not an atomic
  snapshot or proof of current state. A Ready Pod runtime image ID does not
  prove immutable intended identity when the declared image is a tag. The
  pinned Helm 3.22/4.1 matrix covers named install/upgrade paths only; it does
  not close #588 or establish hook, CRD, rollback, conflict or default-apply
  behavior.

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

Check #645, then complete the bounded HLT-04 source replay and owned-container
isolation packets. External dependencies #591 (genuine attestations), #597
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
