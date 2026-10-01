# Minimal ordinary-tool evidence/query plan for the frozen 24

This is a coverage plan for the frozen manifest on main `572bc71e`; it does not
change a question, reference, control, weight, or admission rule. The governing
requirements are Experiment A in `docs/roadmap-3.0-execution.md` and
`evals/ordinary-tool-parity-v1.md`: same pinned Claude, same complete raw
packet (including `managedFields`) byte-for-byte, and baseline access to usable
ordinary read-only kubectl/Helm/file tools. The model need not invoke every
advertised tool in every case. Preserve explicit case tool grants and restrictions
on live access or replaying historical commands. “Only supplied files” limits
the evidence; it does not by itself forbid an allowed local tool from parsing
those same bytes through the documented transport. The table identifies source
evidence and required facts, not a mandatory tool sequence or an instruction to
read every available file. Kubectl queries are conditional; installed Helm
release queries are not required by any frozen question.

`authored-projection` is permissible only under the v1 clarification: retain the
unchanged source packet and map every response field/row to exact source bytes
and a versioned projection rule. Never fill defaults, synthesize missing
objects or turn an unsupported path into 404/empty. A source list supports an
absence claim only when capture scope and completeness for that exact query are
established. A client success against a projection proves consumption of that
projection, not cluster observation. For all cases keep raw inputs, projection,
served response, and client output separately identifiable.

## Per-question minimum

| ID | Required ordinary evidence read | kubectl/Helm query required by this question? |
|---|---|---|
| INV-01 | The identical seven raw exports: `evals/fixtures/scale/cluster/{deployments,namespaces,replicasets,pods,services,configmaps,events}.yaml`, plus `recording-2026-09-30.json`. Deployment `managedFields` is essential. | No command is mandated; prompt gives the exports. Optional scoped client probe is one read-only `kubectl get deployments -A -o yaml --show-managed-fields`, then the declared `team-*` selection. Do not call the file row set a complete cluster inventory without a completeness receipt. |
| INV-02 | Same seven files/provenance as INV-01; exact identities come from the Deployment rows and ownership metadata. | No. Same optional query as INV-01; exact unmanaged/Native negatives remain gated on completeness for that scope. |
| INV-03 | The case scaffold's seven `cluster/` exports; inspect the `inventory`/`inventory` Deployment labels and relevant provenance. | No. Optional exact `kubectl get deployment inventory -n inventory -o yaml --show-managed-fields` must use the same recorded evidence, not a live context. |
| INV-04 | Four files only: `fixtures/{capture-scope.json,readable-populated-deployments.json,readable-empty-deployments.json,denied-deployments.json}`. | No; prompt limits evidence to these files. The capture itself preserves three sequential responses; do not retry/fill gaps. |
| ATR-01 | `changed-by-checkout` scaffold exports; exact `shop/checkout` Deployment `managedFields` at image path. | No. Optional exact Deployment GET with `--show-managed-fields`; no Git or actor lookup is asked for. |
| ATR-02 | `changed-by-cart` scaffold exports; exact `shop/cart` Deployment `managedFields` at replica path. | No. Optional exact Deployment GET with managed fields; do not infer a person/command beyond supported manager evidence. |
| ATR-03 | `changed-by-payments` scaffold exports; exact `payments/payments-api` Deployment managed fields. | No. Optional exact Deployment GET with managed fields; do not infer a manual change absent field evidence. |
| ATR-04 | `argo-label-vs-tracking-id` scaffold exports: target Deployment plus Argo Application/tracking annotation/config evidence. | No. Optional read-only Deployment and Application GETs only if represented by the same packet; no broad live Application discovery. |
| DEL-01 | `fixtures/cluster/flux-applied-digest.yaml` only. | No kubectl/Helm query: the prompt explicitly limits input to the recorded excerpt. |
| DEL-02 | `fixtures/cluster/argo-publication-lag.yaml` only. | No; publication narrative only, and absent exact digests/counts stay unknown. |
| DEL-03 | The three named staged source/receipt files: Sveltos status doc, known-behaviours excerpt, separate delivery receipt. | No; keep their separate provenance and do not join to invented runtime state. |
| DEL-04 | The five committed public receipts under `fixtures/evidence/oci-identity-lifecycle/` (catalog delivery, installer publication, OCI chain, render intent, render receipt). | No. Prompt explicitly says receipts only and bars historical commands, live cluster, registry and ConfigHub service. It asks lifecycle limits, not a Helm release lookup. No frozen case requires installed Helm storage/history; `helm template` would answer a different question. |
| HLT-01 | `fixtures/cluster/sveltos-health.yaml` transcript projection only. | No; printed commands are historical, not executable instructions. |
| HLT-02 | All fixed files in `fixtures/2026-09-30/`, plus `gitrepository.yaml` and `kustomization.yaml`. This includes Kustomization/GitRepository, Deployment, ReplicaSet, Pods, manifests, capture binding/provenance and controller evidence. | No; prompt says only these files and bars cluster contact/historical commands. The exact joins are already in the packet; no extra list query. |
| HLT-03 | `fixtures/{source-provenance.json,receipt.yaml,argocd-child.json}` only. | No; receipt and child capture are separate recorded sources, not a live join. |
| HLT-04 | The three authored inputs/outputs in `fixtures/{scenario-evidence.json,source-metadata.json,synthetic-check-execution-receipt.json}`. | No; explicitly synthetic, file-only; no live check. |
| PRE-01 | All files in `fixtures/`: exact sequential CRD, discovery and ServiceMonitor bodies/statuses, apply stdout/stderr, capture scope and normalized input. | No; prompt is file-only and historical applies must not be rerun. Exact recorded API calls are not permission to issue a new apply. |
| PRE-02 | Six before/after Node, Pod and Event JSON responses plus `capture-scope.json`. | No; file-only sequential capture. No need to query a cluster again to answer the recorded scheduling transition. |
| PRE-03 | Seven captured files: parent/child Application JSON and trees, Pod describe, events, and capture scope. | No; prompt bars live access and limits the evidence to the recorded Argo branch. |
| PRE-04 | `fixtures/{config.yaml,desired-matrix.json,source-provenance.json}`. | No; intent projection only; no live observations to query. |
| RUL-01 | `fixtures/{after-pod.json,capture-time-receipt.json,test-clocks.json}`. | No; raw Pod bytes and two authored clocks are the question. A new GET would change the dated snapshot. |
| RUL-02 | `fixtures/{rul02-cache-replay.json,capture-scope.json}`. | No; authored local API replay and fixed clock, not live-cluster behavior. |
| RUL-03 | Four files: scope/context map and the exact denied/readable raw response bodies. | No; file-only, explicit contexts, no cross-context gap filling or default-context query. |
| RUL-04 | Four files: desired StatefulSet YAML, raw StatefulSet GET, raw PodList and capture scope. | No; file-only sequential evidence. Do not retrieve current image identity to fill the stated gap. |

The generic “exported cluster state” attribution/label prompts do not require
extra cross-product calls: their staged raw exports are the evidence. If an
optional kubectl comparison is actually used, it must be a read-only GET over a
declared source-backed route and the trace must show the exact argv, request,
response provenance/status, output and exit code. Never silently replace an
old fixture with a live observation.

## Minimum query surface and remaining coverage

The smallest useful client surface beyond ordinary file reads is:

1. A real pinned kubectl binary and a versioned read-only recorded transport
   for the *specific* object reads that the case permits. The strongest
   reusable captured routes are PRE-01's exact CRD/discovery/ServiceMonitor
   responses, PRE-02's named Node/Pod/Event responses, INV-04/RUL-03's exact
   success/empty/denied lists, and RUL-01/RUL-04's exact Pod/workload reads.
   These remain `captured-response` only for their recorded request/path/body.
2. For scale and attribution fixtures, if kubectl query execution is selected,
   one versioned Deployment List/Get projection can be built from the actual
   staged YAML including all `managedFields`; ATR-04 additionally needs the
   exact staged Application/tracking input. Bind each row to source file and
   identity, and fail unsupported GVK, selector, namespace or incomplete
   scope as unavailable. The two frozen prompts define the target as “these
   Deployments” in the supplied export. The immutable file set has 302 parsed
   Deployment documents, with 300 selected by the declared `team-*` scope and
   two excluded. The dataset-relative reading of “these Deployments” permits
   exact counts and classification of those supplied rows without a new
   capture. Preserve the frozen Native/unmanaged control by checking the
   completeness of the *provided dataset*: before admission, verify exact
   immutable source hashes; account for all 302 parsed / 300 selected / 2
   excluded rows; reject malformed, truncated, duplicate, or silently
   dropped selected rows; preserve all supplied fields; and state the exact
   `apps/v1` Deployment `team-*` scope in both arms. This is completeness of
   the immutable input set only, not proof that the source API acquisition
   included every matching Deployment in the original cluster. The source
   manifest says “scale raw exports only,” and product provenance intentionally
   says capture completeness is unknown. Do not expand the answer into an
   all-cluster or current-inventory claim. This interpretation keeps the
   question, reference, and control unchanged; it does not assert runtime or
   paid-run admission.
3. No case-specific Helm command is needed. Keep Helm available in the
   declared ordinary-tool environment if Experiment A protocol requires it,
   but do not force a call to every case. DEL-04's five receipts explicitly
   lack OCI/bundle bytes and runtime ID and direct the model to mark lifecycle
   and policy facts unknown. They do not ask whether a release is installed or
   require Helm release storage/history. Adding storage/history would be
   needless sensitive evidence, not a minimum query. If a future prompt asks
   for installed-release state, it needs its own source-backed storage/history
   packet; that is not this frozen suite.

This is a query plan, not evidence admission. Before a 24-case run, a compact
case-to-file/request manifest should enumerate all staged file hashes and any
actually enabled query routes, with source label, exact supported scope,
completeness basis, projection rule/hash, expected result boundary, and
fail-closed behavior. Apply the dataset-integrity checks above for INV-01/02
and preserve the distinction from unknown original-cluster completeness; also
ensure equal ordinary tool grants and treatment delta, all
case input controls, process/descendant accounting, graders, retries, costs,
credits and ordering gates. No paid admission follows from this memo.
