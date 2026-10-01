# P1 verified-answer benchmark v1

This freezes the 24 questions and six equally weighted groups from the P1
section of [the 3.0 execution plan](../docs/roadmap-3.0-execution.md#balanced-first-suite-24-cases-six-equally-weighted-groups).
It is a design manifest, not a runnable suite or evidence of savings. The
machine-readable source is [benchmark-v1.json](benchmark-v1.json).
The supplemental [recorded ordinary-tool parity v1 clarification](ordinary-tool-parity-v1.md)
distinguishes identical raw input packets from authored deterministic
transports and lists current per-case evidence gaps. It does not alter cases,
weights, gates, or paid-run authorization; full Experiment A remains
non-executable until its existing admission requirements pass.
The [ordinary-tool query plan](ordinary-tool-query-plan-v1.md) maps the minimum
case evidence reads and distinguishes optional kubectl/Helm availability from
queries a frozen task actually requires; it is not an admission decision.

Existing case directories are mapped only where their current prompt covers
the frozen question. The September 30 refresh now includes managedFields in
both arms' exports and validates the four attribution manager/cause pairs.
Main-scenario mappings are `existing_refreshed_fixture`. INV-01/02 now have
`recorded_snapshot_binding_prepared_not_run` mappings via the explicit
[recorded-scale preparation packet](recorded-scale/README.md). Historical scale
cases remain unchanged; their recorded variants are generated outside the repo.
Resource-level mutation manager/cause parity does not prove a particular field's
latest writer; exact-field counterexamples are tracked in [#649](https://github.com/confighub/cub-scout/issues/649).
None of these statuses means the controlled benchmark ran. DEL-01 and DEL-02 are pinned
public-source projections, not raw cluster snapshots; both are prepared and
unrun, and their evidence limits are recorded per case. The 24 cases currently
comprise 5 refreshed fixtures, 2 recorded scale bindings prepared but not run, 8
recorded projections prepared but not run, 7 raw recordings prepared but not
run, 2 synthetic source replay fixtures prepared but not run, and 0 planned
cases. HLT-03 is a receipt-backed pair of public recorded
files, not a full Kubernetes snapshot; its receipt-level workload pass and
child Application Ingress residue are separate evidence facts. HLT-02, INV-04, PRE-01,
RUL-03 and RUL-04 are raw recordings. Unmapped entries remain `planned`; the three live-only scale
cases remain a separate experiment.

| Group | ID | Frozen question | Current mapping/status |
|---|---|---|---|
| Inventory | INV-01 | Owner counts at scale | `scale/scale-ownership-counts` — recorded binding prepared, not run; tool/grader admission pending |
| Inventory | INV-02 | Exact unmanaged list at scale | `scale/scale-unmanaged` — recorded binding prepared, not run; tool/grader admission pending |
| Inventory | INV-03 | Simple direct ownership label lookup | `owner-confighub` — fixtures refreshed; benchmark run pending |
| Inventory | INV-04 | Partial/RBAC inventory without false orphan claims | `inv04-rbac` — actual scoped populated/empty/403 responses prepared, not run; denied inventory stays unknown |
| Attribution | ATR-01 | Manual set-image attribution | `changed-by-checkout` — fixtures refreshed; benchmark run pending |
| Attribution | ATR-02 | Manual scale attribution | `changed-by-cart` — fixtures refreshed; benchmark run pending |
| Attribution | ATR-03 | Controller-only change | `changed-by-payments` — fixtures refreshed; benchmark run pending |
| Attribution | ATR-04 | Copied Argo instance label versus tracking identity | `argo-label-vs-tracking-id` — fixtures refreshed; benchmark run pending |
| Delivery identity | DEL-01 | Flux applied digest | `flux-applied-digest` — pinned public run-log projection prepared, not run; exact apps digest and Git revision shown |
| Delivery identity | DEL-02 | Argo publication not consumed | `argo-published-release-lag` — pinned narrative projection prepared, not run; exact old/new digests and replica counts absent |
| Delivery identity | DEL-03 | Sveltos inferred revision versus exact proof | `sveltos-inferred-revision` — source/receipt projection prepared, not run; missing/stale input variants pending |
| Delivery identity | DEL-04 | OCI identity mismatch and Helm lifecycle boundary | `oci-identity-lifecycle` — receipt projection prepared, not run; divergent-identity and mutable-tag inputs pending |
| Health | HLT-01 | Sveltos Provisioned after workload failure | `sveltos-hlt-01-health` — recorded Part B projection prepared, not run; prerequisite state is separate from health |
| Health | HLT-02 | Flux Ready without workload checks | `flux-ready-without-health` — raw 2026-09-30 capture prepared, not run; Ready, wait/check configuration, Deployment availability, UID chain, and exact source/applied revision kept separate |
| Health | HLT-03 | Consul convergence with Ingress residue | `consul-ingress-residue` — pinned receipt + child Application capture prepared, not run; workload pass does not override watched Ingress |
| Health | HLT-04 | Missing, old, or renewed report timestamps | `sveltos-hlt-04-report-freshness` — synthetic producer replay prepared, not run as a model case; no actual check-execution or applied-digest proof |
| Prerequisites / graph | PRE-01 | Missing CRD prerequisite | `pre01-crd` — actual sequential CRD/ServiceMonitor API reads and dependent apply outputs prepared, not run; route 404 bodies are preserved as untyped responses; registration is not health |
| Prerequisites / graph | PRE-02 | Kubernetes topology and cloud prerequisite | `pre02-node-selector` — first independently reviewed owned-kind selector recording prepared, not run; scheduling is not application health |
| Prerequisites / graph | PRE-03 | Argo/Crossplane child-chain failure | `pre03-argo-child-failure` — retained historical Argo-only parent→child→Pod failure chain prepared, not run; no Crossplane claim, non-atomic and not current |
| Prerequisites / graph | PRE-04 | Hub/spoke intended placement with missing live observations | `kubara-hub-spoke-placement` — pinned desired/config projection prepared, not run; no live observations consumed |
| Reuse / limits | RUL-01 | Repeated question using a dated snapshot | `rul01-dated-snapshot` — PRE-02 Pod response reused with its one-second request-time receipt and authored test clocks; prepared, not run; not live freshness or product capture-time support |
| Reuse / limits | RUL-02 | Cache invalidation after identity change | `rul02-cache-replay` — source-pinned synthetic reader replay prepared, not run as a model evaluation; no automatic invalidation or general freshness claim |
| Reuse / limits | RUL-03 | Denied cluster beside readable cluster, explicit context | `rul03-context` — raw explicit-context responses and strict answer contract prepared, not run; denied inventory stays unknown |
| Reuse / limits | RUL-04 | Unsupported workload/image proof | Raw capture prepared, not run: Ready Pod runtime imageID and UID linkage preserved; tag-only authored intent does not establish immutable intended image identity. No applied-source proof. [Case](rul04-image-identity/README.md) |

## Recorded scale binding limits

PRs #692–#694 prepare equal seven-file inputs and verify full/summary/Native/
Native-summary CLI/MCP parity for 300 selected Deployments out of 302 parsed.
The manifest pins the source recording, preparation revision and tested binary.
This is no-model preparation proof. The generated variants retain the original
questions and ordinary tool declarations, remove tool-use correctness graders,
and bind recorded MCP to the Deployment export. Both arms receive all seven
raw files. The historical live comparison and its results are unchanged.

Actual harness MCP grants and ordinary-tool parity remain unverified. This
file-tools-only setup is narrower than Experiment A's kubectl/Helm baseline.
The legacy final-line graders can miss contradictory prose; an opt-in strict
answer contract (`recorded-scale-answer-line.v1`, #697) is now explicitly selectable,
with generated prompt/grader hashes and a passed no-model preflight recorded in
the manifest. It preserves historical results; harness admission remains pending.
Product capture time/completeness stay unknown. These mappings do not admit the cases to paid execution, make the full
suite executable, or establish cost savings.

## Frozen comparison rules

Use the same pinned Claude model and Claude Code version in both arms. Both
arms receive byte-identical full raw evidence, including managedFields, object
metadata, status, relevant events, controller configuration, and source
identity. Record fixture hashes. The comparison is paired; randomize within-pair
arm order with a recorded seed. Keep each group at one sixth of task weight so
easy inventory prompts cannot hide an expensive or inaccurate group.

Answer success is binary. Every mandatory positive-weight grader other than
`tool_used` must appear exactly once and pass with a boolean `true`. Missing or
malformed correctness metadata is unknown; an errored run is unverified.
`tool_used` remains an indicator and does not affect answer correctness under
either ablation. Keep the old fractional harness score only as a diagnostic.

Count the full cost of every attempt, including failed runs, retries, judges,
mocks, and orchestration where attributable. Report provider dollars, harness
estimates, account credits, and token usage separately. If actual credits cannot
be attributed, say **not measured**; do not infer them from remaining balances.
Cost per verified answer is total arm spend divided by binary verified answers;
with zero successes it is undefined/infinite. Keep interrupted runs as failed attempts and include their costs in every
aggregate estimate. A partial campaign cannot support a complete-suite headline.

The execution plan budgets 3 paired repeats for diagnosis, followed by 10 per
arm as the starting point for release evidence. It sets a $20 smoke cap and a
$200 baseline cap; its $1.25 per arm-run is historical planning arithmetic, not
a current price quote. Before the full paid baseline, pin and date model pricing, complete
all 24 prompts and references, prove equal evidence, check grader failures
offline, and preflight authentication, provider limits, and cost accounting.
This packet authorizes no paid command.

The release gates remain the plan's gates: no lower observed verified-success
rate and a paired 95% interval not extending below -2 percentage points; at
least 20% lower task-balanced cost per verified answer with the paired
task-level bootstrap interval excluding no saving; 20% lower attributable
credits or an explicit “not measured” limitation with token/list-price evidence;
20% lower median time, with p95 reported and regressions above 10% investigated.
Any passing result applies only to the named workloads, evidence condition,
model/version, and date.
