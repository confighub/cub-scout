# Operator investigation assessment protocol

Status: **candidate protocol; no participant results or win claim**.
Owner: [#519](https://github.com/confighub/cub-scout/issues/519), with shared
facts/scope controls in #596/#599 and load/reuse in #539/#604. This protocol
implements the [roadmap outcome scorecard](roadmap.md#winning-means-verified-investigation-outcomes).
It neither replaces the standalone TUI nor admits live or paid execution.

## Question and supported scope

Does the released Scout workflow help an operator reach an independently
verified Kubernetes/GitOps diagnosis faster, with equally correct evidence and
safe scope, than an ordinary read-only command workflow?

Use the same immutable dataset, permissions, controller versions and observation
age in both arms. Register the exact release/commit/binary hashes, dependency
versions, task/answer manifests, fixture provenance, commands, training guides,
randomization seed and analysis code before recruitment or runs. Any missing
recording, metadata or context binding blocks that task cell. Authored regression
controls are labeled authored; they cannot substitute for genuine recordings.
The frozen paired-agent benchmark remains a separate experiment and is unchanged.

No connected/fleet workflow is admitted from standalone evidence. A connected
panel is assessed only after its #519 adapter gates pass. The standalone arm
must work without ConfigHub credentials or another explorer installed.

## Fixed task families

Each family has matched A/B variants with different names and equivalent
complexity. Assign variants across arms; do not let participants repeat the same
answer from memory. Each admitted cell includes exact expected facts, supported
commands, omissions, relevant identity/revision and a reviewer-verifiable answer.

| Family | Required answer and controls | Admission evidence |
|---|---|---|
| Locate affected workload | Correct kind/namespace/name, owner evidence and affected scope; same-name resources in another scope do not contaminate selection | Exact recorded object identities and scope controls; #599 |
| Follow source and revision | Distinguish declared source, observed consumption and unresolved joins; a parent status cannot replace child evidence | Recorded source/controller/object chain and wrong-revision negative; #561/#596 |
| Explain incomplete rollout | Name the incomplete stage and residual object; delivery readiness cannot imply unmeasured application health | Recorded rollout and missing-check evidence; #620/#641 |
| Assess manual field change | Identify the exact changed field and its evidence; absent or ambiguous provenance remains unknown | Pinned desired/observed comparison and missing metadata; #596 |
| Assess stale or wrong release | Reject stale, future, missing-time and wrong-unit/revision facts as current success | Genuine timestamp/revision cases and mandatory negatives; #591/#597 |
| Export scoped explanation | Export facts, age, scope and omissions that a reviewer can validate without the original session | Shared CLI/TUI output contracts and denial/empty distinctions; #596/#599 |

Controller families are separate strata. Report admitted and blocked cells;
do not hide weak depth behind a broad aggregate or score unsupported tasks as
failed supported diagnoses. Negative-control answers may correctly be unknown,
but must state the precise missing evidence and safe next read.

## Participants, training and assignment

Candidate design: 24 participants, 12 less experienced and 12 experienced
operators, classified with a fixed pre-study questionnaire. No recruitment is
performed by preparing this document. A pilot may estimate variance and test
instructions; pilot participants/results are excluded from confirmatory scoring.
Set the final sample size using the pilot variance and a documented power
calculation before freezing the protocol. Do not increase it after inspecting
confirmatory outcomes. If uncertainty remains too large, report inconclusive.

Both arms receive equally detailed guides, equal training time and the same
practice task outside the scored set. Counterbalance arm order within each
experience stratum, then use a registered Latin-square task order. Alternate
matched variants across arms. Record prior tool familiarity. Fix terminal size,
hardware, network/fixture access, caches and read permissions; reset each scored
session. Cold and warm navigation are separately labeled rather than pooled.

## Scoring and retention

Two independent reviewers score answer and evidence using the frozen rubric,
with disagreements adjudicated by a third reviewer. Reviewers do not see arm
labels in normalized answer exports. Preserve original streams and tool versions
for audit. Score exact scope, supported facts, required omissions and absence of
false health/approval/ownership/orphan claims. A faster incorrect answer fails.

Every supported attempt has a 15-minute task deadline. Retain errors, abandonments,
timeouts and unusable exports. An unverified answer or timeout counts as failure
and receives the full 15-minute time for the primary analysis. Report completion
and time separately; also publish successful-only timing as a secondary analysis,
never as the headline result. Missing telemetry is unknown, not zero cost.

Record setup time separately from task time. Task time runs from displaying the
question to submission of the diagnosis/export and includes navigation and tool
waiting. Verification is independent; invalid submissions retain their full time.
API read attempts and consumed response bytes need matched transport coverage;
identity-reader-only costs cannot be compared with whole-command totals. Record
wire/header/auth exclusions, refreshes, cache reuse and failed reads explicitly.
No agent dollar/credit savings is inferred from human timing.

For first use, run a separate guided offline scenario with new participants,
authored immutable inputs and an independent correct-export rubric. Record setup
and time to the first useful explanation separately, retaining failed attempts.
Both guides and any installation prerequisites are pinned before the run.

## Fixed analysis and decision rules

Before runs, register the final sample size and analysis script/hash. Use paired
participant-level summaries across matched supported tasks, with controller and
experience strata reported separately. The primary speed ratio is the ratio of
median per-participant task times (Scout / ordinary read-only workflow), including
failure penalties. Estimate a two-sided 95% interval by resampling participants
with their paired observations together, 10,000 bootstrap draws and fixed seed
1042026. Do not resample individual trials as independent participants.

A scoped speed claim requires at least 20% lower median time and an interval
supporting that improvement (upper ratio bound at most 0.80). Report p95 with
uncertainty; investigate any point-estimate regression greater than 10% before
recommendation. Supported-answer success must not regress. All mandatory scope,
denial, stale and wrong-revision negative controls must pass. A failed correctness
control blocks the claim even if timing improves. The separate first-use target
is median five minutes to a correct exported explanation; show its interval,
setup time and completion rate. The agent quality/cost gate remains separate.

Publish every registered stratum and exclusion. Record any protocol deviation
before analysis, explain its impact and label exploratory analyses. Narrow the
claim to admitted workflows where the thresholds pass; do not generalize across
controllers, connected/fleet mode or unmeasured tasks. A release tag alone is
not evidence of a product win.

## Report template

- Registered protocol/task/source/analysis hashes and dates; deviations.
- Admitted/blocked task cells, controller/permission/transport scope and provenance.
- Participant experience/familiarity, assignment, training and attrition.
- Correct supported answers and every mandatory negative control, by arm/stratum.
- Median ratio and paired interval, p95 and uncertainty, failures/timeouts retained.
- Setup and first-use explanation/export times and completion rate.
- Cold/warm/denied/refresh read counts and bytes with precise coverage exclusions.
- Examples of correct unknowns, misleading outputs and unsupported joins.
- Passed targets, failed targets and inconclusive cells; bounded recommendation.
- Redacted replayable evidence package and independent review sign-off.

Until this report exists, the roadmap targets remain **pending measurement**.
