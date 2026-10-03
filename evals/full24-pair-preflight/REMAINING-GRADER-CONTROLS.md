# Remaining answer-grader controls

## Success criteria defined before implementation

This packet covers exactly HLT-01–04, INV-04, PRE-01/03/04 and RUL-02/03/04.
Each test prepares and validates a fresh frozen full24 packet, then reads the
exact selected positive-weight (`1`) answer grader from its oracle. Canonical
answer claims must follow the prepared recorded evidence and explicit source
limits; regex literals alone are not an evidence oracle. Existing source-package
answer or fixture validators are reused where available.

For every selected answer field, controls must reject a wrong evidence value,
missing or duplicate fields, extra fields, wrong types, partial JSON, and prose.
Canonical answers and declared whitespace variants must match the selected
pattern, flags and `last_message` target in both Python and existing local
Node.js. Node receives bounded JSON on stdin and executes only local regex
matching. Unknown, degraded and incomplete evidence must not become claims of
health, unmanaged/orphan resources, current state, or established correlation.

Only new files in this packet may change. Questions, weights, selected graders,
source fixtures, and existing tests remain frozen. If canonical evidence needs
new genuine capture or broader scope, stop and report that limitation.

## Limits

These are authored offline answer-vector controls. They do not execute the
official evaluator, model/provider, plugin, containers, servers, live APIs, or
credential paths. Agreement between two engines on these vectors is not
universal regex-dialect parity, model quality proof, a paid evaluation, or a
savings measurement. No installation or download is permitted or performed.

## Source binding

| Case | Canonical evidence checked before regex matching |
| --- | --- |
| HLT-01 | The focused transcript supplies Provisioned, False, Synced and Degraded. Its revisions are shortened; its report JSON carries no exact release identity. |
| HLT-02 | The pure capture validator checks the Deployment/ReplicaSet/Pod UID chain and current workload failure. Kustomization Ready generation, `wait: false`, both absent health-check fields, and source/applied revisions are read independently. The capture remains sequential and historical. |
| HLT-03 | The receipt's three runtime legs pass while its overall outcome is watch. The separate child records one Synced/Progressing Ingress without a cause; provenance denies current, atomic or full-object-snapshot claims. |
| HLT-04 | All eight recorded synthetic held/computed reports and write patches supply timestamp, health, sync and revision fields. Raw-input hashes bind every row to source metadata. The separate synthetic receipt is outside producer inputs; source limits distinguish report renewal, condition transitions and inferred release identity from check execution or digest proof. |
| INV-04 | The pure API validator distinguishes two readable scoped responses from a typed 403. Visible identities retain namespace, name, UID and resource version; denial and orphan status stay unknown. |
| PRE-01 | The pure raw-response validator checks exact typed CRD absence, route-level 404s, registration/discovery, typed object NotFound and later creation. Retained operation exit codes and stdout/stderr establish apply outcomes; registration proves no health. |
| PRE-03 | Parent/child identities, tracking ID and statuses come from separate Argo responses. StatefulSet/Pod tree and diagnostics bind image-pull failure. Scope and timing metadata deny Crossplane, current state and atomicity; conflicting cleanup fields remain unconfirmed. |
| PRE-04 | Current config enables cert-manager on the recorded hub and three spokes. Desired matrix rows supply selected version and placement but explicitly unknown observation fields; spoke Argo is hub-managed. No live reads are included. |
| RUL-02 | Nine recorded authored-clock steps supply cache results, returned and configured identities, digests and timestamps. A changed response is not served on a hit; failed refresh yields no old object. The replay does not establish automatic/push invalidation or general freshness. |
| RUL-03 | The pure observation validator checks typed denial and the readable Deployment identity. Context map and before/after metadata preserve the ambient context; denied inventory remains unknown. |
| RUL-04 | Pure StatefulSet/Pod validators check UID linkage and Ready/Running runtime evidence. Authored intent remains tag-only, so observed runtime imageID does not prove intended immutable identity or an applied-source binding. |

The exact oracle grader bytes are also compared to their selected source files.
HLT-04 and PRE-03 explicitly require compact JSON and reject padded/pretty
variants; the other nine selected patterns accept the tested whitespace variant.
Nested HLT-04 records receive per-leaf controls plus missing/duplicate record,
container type, duplicate container field, and nested extra-field checks. String
fields reject null, number, Boolean, list, and object values.

Run only this packet from the repository root:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/full24-pair-preflight -p 'test_remaining_graders.py'
```
