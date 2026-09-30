# P1 verified-answer benchmark v1

This freezes the 24 questions and six equally weighted groups from the P1
section of [the 3.0 execution plan](../docs/roadmap-3.0-execution.md#balanced-first-suite-24-cases-six-equally-weighted-groups).
It is a design manifest, not a runnable suite or evidence of savings. The
machine-readable source is [benchmark-v1.json](benchmark-v1.json).

Existing case directories are mapped only where their current prompt covers
the frozen question. The September 30 refresh now includes managedFields in
both arms' exports and validates the four attribution manager/cause pairs.
Main-scenario mappings are `existing_refreshed_fixture`; the scale scout arm
still reads live data, so those mappings are `existing_needs_snapshot_binding`.
Resource-level mutation manager/cause parity does not prove a particular field's
latest writer; exact-field counterexamples are tracked in [#649](https://github.com/confighub/cub-scout/issues/649).
Neither status means the controlled benchmark ran. Unmapped entries remain
`planned`; the three live-only scale cases remain a separate experiment.

| Group | ID | Frozen question | Current mapping/status |
|---|---|---|---|
| Inventory | INV-01 | Owner counts at scale | `scale/scale-ownership-counts` — snapshot binding pending |
| Inventory | INV-02 | Exact unmanaged list at scale | `scale/scale-unmanaged` — snapshot binding pending |
| Inventory | INV-03 | Simple direct ownership label lookup | `owner-confighub` — fixtures refreshed; benchmark run pending |
| Inventory | INV-04 | Partial/RBAC inventory without false orphan claims | Planned |
| Attribution | ATR-01 | Manual set-image attribution | `changed-by-checkout` — fixtures refreshed; benchmark run pending |
| Attribution | ATR-02 | Manual scale attribution | `changed-by-cart` — fixtures refreshed; benchmark run pending |
| Attribution | ATR-03 | Controller-only change | `changed-by-payments` — fixtures refreshed; benchmark run pending |
| Attribution | ATR-04 | Copied Argo instance label versus tracking identity | `argo-label-vs-tracking-id` — fixtures refreshed; benchmark run pending |
| Delivery identity | DEL-01 | Flux applied digest | Planned; identity is digest-based, never timestamp-derived |
| Delivery identity | DEL-02 | Argo publication not consumed | Planned |
| Delivery identity | DEL-03 | Sveltos inferred revision versus exact proof | Planned; include missing and stale identity |
| Delivery identity | DEL-04 | OCI identity mismatch and Helm lifecycle boundary | Planned; render parity does not prove hooks executed |
| Health | HLT-01 | Sveltos Provisioned after workload failure | `sveltos-hlt-01-health` — recorded Part B projection prepared, not run; prerequisite state is separate from health |
| Health | HLT-02 | Flux Ready without workload checks | Planned |
| Health | HLT-03 | Consul convergence with Ingress residue | Planned |
| Health | HLT-04 | Missing, old, or renewed report timestamps | Planned; report freshness, release identity, and health are separate facts |
| Prerequisites / graph | PRE-01 | Missing CRD prerequisite | Planned |
| Prerequisites / graph | PRE-02 | Kubernetes topology/cloud prerequisite | Planned |
| Prerequisites / graph | PRE-03 | Argo/Crossplane child-chain failure | Planned |
| Prerequisites / graph | PRE-04 | Hub/spoke intended placement with missing live observations | Planned |
| Reuse / limits | RUL-01 | Repeated question using a dated snapshot | Planned; snapshot age remains visible |
| Reuse / limits | RUL-02 | Cache invalidation after identity change | Planned; missing/stale identity stays unresolved |
| Reuse / limits | RUL-03 | Denied cluster beside readable cluster, explicit context | Planned; context cannot change implicitly |
| Reuse / limits | RUL-04 | Unsupported workload/image proof | Planned; unknown where proof is unsupported |

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
