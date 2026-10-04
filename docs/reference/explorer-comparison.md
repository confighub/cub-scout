# Scout investigation quality and acceptance

Updated 2026-10-04. Published baseline: **v2.12.4**. The v2.13 topic branch
contains additional unreleased context and ChangeOrder read work; its current
status and release gates are in [HANDOVER.md](../../HANDOVER.md) and the
[v2.13 readiness checklist](../releases/v2.13-readiness.md).

## Product aim

Make Scout the fastest safe route to a correct, explainable Kubernetes and
GitOps diagnosis for both operators and agents. Every answer should expose the
selected scope, evidence, timestamps, omissions and useful next read.

A terminal interface, graphs, logs, read-only defaults and MCP are useful
capabilities. Leadership requires measured investigation outcomes. Documentation
research and operator comments inform priorities; they do not establish a
performance advantage or prove that another tool lacks a capability.

Scout's strongest opportunity is to join supported controller, source, release,
object and workload evidence into one reusable answer across mixed environments.
Ownership support is not equal semantic depth for every controller. Missing or
ambiguous joins must remain explainable unknowns.

## Current foundation and limits

| Foundation | Evidence and practical limit |
|---|---|
| Cross-controller ownership and provenance | [Ownership contract](../../AGENTS.md) and [command reference](commands.md). Use observed labels, annotations and owner references; do not infer a person from a field-manager name. |
| Scoped diagnosis | `doctor`, `trace`, `explain`, `gitops status` and comparison surfaces. Explicit context support is advancing per command; scope conformance is tracked in #599/#746, not presumed complete. |
| Delivery and running-image checks | [Image verification guide](../howto/is-this-image-deployed.md). Controller revision, live object agreement, rollout convergence and application success are separate claims; supported workload/digest limits remain visible. |
| Reusable evidence | JSON, MCP, bundles and fingerprinted receipts. A fingerprint supports artifact integrity; it does not independently prove source authenticity, approval or freshness. |
| Bounded reads and observation reuse | [Bounded resource example](../../examples/bounded-resource-read/) and [observation plan](../proposals/observation-efficiency.md). The narrow object path and opt-in watch-backed inventory have scoped request tests; broad commands and the default polling path do not inherit those guarantees. |
| Standalone operation | Cluster observation does not require ConfigHub. Connected governance is additional evidence with separate authentication and authority boundaries. Stored declarations and reported state are not evaluated approval. |

The historical September assessment is superseded by this checkpoint. Old
receipts retain their named source/version scope; they are not acceptance for
current binaries or an overall performance result.

## Five priorities and success criteria

These criteria define future work before implementation. They do not add gates
to v2.13 or replace the adopted release plan.

| Priority | Operator benefit | Required proof and tracking |
|---|---|---|
| 1. One coherent investigation | Start with a symptom, find the relevant workload, follow its source/controller, inspect failure evidence and export the result without losing the selection. | One identical supported scenario through CLI and TUI; facts, omissions and scope agree. Record completion, time to a supported diagnosis, navigation/actions, dead ends and export correctness. #519/#596. |
| 2. Scope safety | Always know which context, namespace and connected space produced the evidence; changing selection never shows another scope's late result. | Colliding names across two contexts/spaces, missing explicit context, cancellation and delayed responses. No ambient fallback, global-context mutation or merged history; scope labels are not stable cluster identity. #599/#746. Scope-bound investigation history remains a design proposal. |
| 3. Enforced least privilege | Use actual restricted credentials and explain unavailable coverage without pretending the cluster is empty or healthy. | Define required read permissions by operation; propose explicit core/controller profiles and separate sensitive reads. Test denied discovery/list/get, absent APIs and no credential escalation. Mocked controls first; reproducible genuine RBAC acceptance later. The broad wildcard read-role example in [SECURITY.md](../../SECURITY.md) is not proof of a least-privilege profile. |
| 4. Complete, honest answers | Explain source/revision, applied configuration, workload convergence, drift and evidence freshness together where contracts support the join. | Fixed same-name, wrong-revision, stale-report, deleted-object, missing-metadata and denied-read fixtures. No false delivery, health, orphan or approval claims. #561/#641/#591/#597. |
| 5. Cheap reuse and easy demonstration | Revisit an investigation cheaply, refresh explicitly, and try a useful scenario without a cluster or model provider. | Measure cold/warm/idle/change/denied/reconnect requests and bytes with cache age visible. Revisit must not renew observation time. Use an authored mixed-controller demo plus an unavailable-evidence case; separate it from genuine live proof. #539/#519/#604. |

The [roadmap outcome scorecard](../roadmap.md#winning-means-verified-investigation-outcomes)
sets measurable operator, agent, scope, onboarding and reuse targets and assigns
them to delivery stages. Passing is required for the corresponding product
claim; none is presented here as an achieved result.

## Measuring improvement fairly

Use the same supported question, object scale, evidence availability, effective
permissions and failure conditions for each evaluated workflow. Include an
ordinary command-line read workflow. Separate controller-specific tasks from
mixed-controller breadth; report unsupported tasks explicitly. Do not assign
"absent" to an unreviewed capability or use a wider evidence grant for Scout.

For operators, measure independently checked completion, time to a correct
explanation, navigation effort, incorrect conclusions, export usefulness and
API load. Fix the task set, scoring and performance thresholds before any run;
retain failures and report per-task results and uncertainty. No operator speed
result is currently claimed by this document.

For agents, retain the [adopted paired benchmark](../roadmap-3.0-execution.md#measuring-whether-cub-scout-saves-users-money):
verified-answer quality, cost per verified answer, attributable credits/tokens,
median/p95 time and the mechanism behind any saving. MCP availability or shorter
output does not prove lower cost. The frozen 24 cases remain unchanged; new
operator scenarios belong to a separately declared assessment.

## Execution order

Finish the current v2.13 evidence/runtime packets and preserve their release
gates. Then settle the operator workflow under #519, scope/parity under
#599/#596, and observation efficiency under #539. Define permission profiles
and scope-bound investigation history before implementing them. Keep all
standalone benefits useful without connected governance.

Observation remains read-only. Delivery, reconciliation, promotion and policy
acceptance stay with their existing authorities. Scout provides evidence and
safe next reads; adding mutation controls is not a requirement of this plan.

New ideas are indexed in the [roadmap backlog](../roadmap.md#untracked-backlog-checklist).
