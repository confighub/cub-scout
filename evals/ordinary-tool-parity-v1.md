# Recorded ordinary-tool parity v1

**Status:** supplemental protocol clarification; not benchmark admission. This note is scoped to Experiment A under [#603](https://github.com/confighub/cub-scout/issues/603). It does not change the frozen 24 questions, six group weights, correctness or statistical rules, cost accounting, budgets, paid-run stop, or runtime/descendant gates in the [3.0 execution plan](../docs/roadmap-3.0-execution.md) and [benchmark manifest](benchmark-v1.json). Process containment/accounting remains separately tracked by [#709](https://github.com/confighub/cub-scout/issues/709).

## Equal evidence and transport

Experiment A requires both arms to receive the same complete raw workload evidence, including `managedFields`, and the manifest requires equal-information evidence packets to be byte-identical. These requirements apply to the immutable input packet. They do not require every ordinary-tool HTTP response to be a historically captured, byte-exact Kubernetes API response. A recorded ordinary-tool arm may use the real pinned kubectl (or Helm) against a transparent, deterministic read-only transport derived from the same packet, if each response is labeled as a source-captured response or an authored projection. This is a recorded comparison method, not live-cluster behavior.

The packet contains the original source files and a hash manifest. The transport must not rewrite those files. For every supported request, retain a mapping from the served response bytes to the exact source file(s), source rows and projection rule. Preserve all source fields losslessly; do not add Kubernetes defaults, IDs, resource versions, status, events, managed fields, timestamps, identities or other answer-bearing facts that are absent from source. Keep source bytes, deterministic projection, served response and actual client result as separate artifacts.

Use these provenance labels:

- `captured-response`: exact request method/path/query, status and response bytes are present in a pinned capture receipt.
- `authored-projection`: response bytes were deterministically produced from pinned source files using a versioned rule. The projection and any authored transport metadata are separately hashed and cannot be described as a captured API response.
- `unavailable`: the query is unsupported, the required source field is missing, or scope/completeness cannot be established. Return an explicit replay-unavailable error and fail query coverage; never turn a missing route into HTTP 404, an empty list, or an absence answer.

A source-derived list can support negative claims only when the source manifest establishes completeness for that exact kind, namespace/scope, selector and capture interval. A partial or denied list stays partial/unknown. Kubernetes discovery may be supplied as clearly labeled transport metadata for an explicitly supported GVK; discovery describes the adapter's supported query surface, not observed cluster resources. It must not claim that an API, CRD or object existed or was absent. Deny mutating requests. Record exact client argv, request, response provenance, status, output, exit code, timing and errors. A passing client command proves it consumed the stated recorded view, not that authored transport was observed on a cluster.

Both arms receive the same raw packet and may read it with ordinary file tools. Baseline ordinary tools must also be genuinely available and usable as declared; treatment adds only the declared cub-scout skills/MCP. Access to a file-only mock or a treatment-only source is not equal ordinary-tool parity. Actual tool grants, tool inventory/use, descendant completeness and attributable costs remain subject to the existing admission audit and gates.

## Full-suite coverage matrix

This matrix records the source basis and known parity gap for each frozen case. It is a preparation checklist, not a command plan, proof of complete evidence, or admission decision. `Projectable` means a deterministic adapter may expose facts represented in the named packet; the adapter still needs review and request-to-source tests. Cases with public projections or synthetic inputs do not become raw cluster evidence by being made queryable.

| Case | Current packet basis (per manifest) | Ordinary-tool evidence status / missing coverage |
|---|---|---|
| INV-01 | Seven-file Deployment export; 302 parsed, 300 selected under the recorded scope | The selected rows are file evidence. A read-only Deployment projection can cover only the declared scope; exact API discovery and query-to-row transport proof are pending. Do not claim whole-cluster counts. |
| INV-02 | Same seven-file Deployment export | Same scope limit as INV-01. Exact unmanaged conclusions require proven list completeness for the declared scope; omitted/denied rows remain unknown. |
| INV-03 | Refreshed direct ownership-label fixture | File-level object evidence is available. No source API receipt or ordinary-client parity receipt is bound in the manifest. |
| INV-04 | Raw readable/denied inventory capture | Captured readable response and 403/omissions can be replayed as such. Denied resources are unknown, not an empty or unmanaged inventory; wider list completeness is not established. |
| ATR-01 | Refreshed `changed-by-checkout` fixture | The case needs the exact changed object and `managedFields`; fixture bytes can be shared, but API request/response parity is not yet bound. |
| ATR-02 | Refreshed `changed-by-cart` fixture | Same limit as ATR-01; no claim from resource-level hints without field evidence. |
| ATR-03 | Refreshed `changed-by-payments` fixture | Same limit as ATR-01; controller-only attribution must be grounded in included manager fields. |
| ATR-04 | Refreshed Argo label/tracking-ID fixture | File fixture is available; broader Application discovery, scope and API receipt coverage are not established. |
| DEL-01 | Pinned Flux public source/receipt projection | Not a raw cluster packet; exact applied digest joins remain limited to the cited projection. Missing workload/API receipts cannot be filled by transport. |
| DEL-02 | Pinned Argo publication narrative projection | Exact release digests and replica values are absent; API/Helm access cannot invent them. |
| DEL-03 | Pinned Sveltos source/receipt projection | Missing/stale variants and complete applied-source joins remain absent. |
| DEL-04 | Pinned OCI publication/render/delivery receipts | No raw OCI/bundle bytes, runtime image ID, or Helm release storage/history. Helm release queries are unavailable until those source artifacts are prepared. |
| HLT-01 | Pinned Sveltos Part B projection | Not a raw workload/API snapshot; prerequisite state and workload health remain separate. |
| HLT-02 | Sequential raw Flux/Kubernetes capture | Named objects and their recorded fields can be exposed; this is not atomic/current, has no application-level check, and does not establish broader namespace inventory. |
| HLT-03 | Pinned receipts plus child Application capture | Supports only the recorded receipt/child facts. It is not a full raw Kubernetes snapshot or complete descendant query surface. |
| HLT-04 | Synthetic source replay | Authored clock/check inputs exercise deterministic reader behavior; not a live controller run or observed health-check execution. |
| PRE-01 | Sequential CRD/ServiceMonitor API observations and dependent apply outputs | Exact captured paths/status/body bytes support those routes only. General kubectl discovery or other GVR coverage is not implied. |
| PRE-02 | Owned-cluster node-selector capture | Raw named observations support the recorded scheduling facts; not application health or a complete cluster inventory. |
| PRE-03 | Historical Argo parent/child/Pod projection | Not raw Kubernetes responses; no Crossplane or current-state coverage. |
| PRE-04 | Pinned desired-state/configuration projection | No live observations. Placement intent is not proof of installation, health or ownership. |
| RUL-01 | PRE-02 Pod bytes plus request-time receipt and authored test clocks | Supports the dated snapshot question only; no product capture-time or current-freshness claim. |
| RUL-02 | Source-pinned synthetic cache replay | Authored local responses are not captured Kubernetes API traffic or general live cache behavior. |
| RUL-03 | Explicit denied/readable-context raw responses | Supports the named requests/context facts only. The denied cluster remains unknown; no full inventory or global-context claim. |
| RUL-04 | Sequential raw StatefulSet/Pod observations plus desired YAML | Runtime image ID is present but immutable intended identity/applied source proof is missing; do not invent that relationship. |

## Kubectl and Helm boundaries

A minimal kubectl transport for the Deployment export would retain the full seven source files unchanged, declare the exact supported `apps/v1` Deployment scope, and expose only query paths/filter semantics that can be tested against those rows. For a derived List, the response must be labeled `authored-projection`, preserve item fields, and publish the scope/completeness limit out of band. If the packet cannot prove completeness for a requested negative, that request is unavailable. PRE-01's captured API paths can be replayed byte-for-byte, but its six raw reads do not establish general discovery compatibility.

The present seven-file scale packet does not include Helm release storage/history. `helm template` renders desired state and is not a substitute for installed release state. To cover a frozen task that queries installed Helm releases, prepare source evidence for the specific read-only commands (release metadata/storage records and the required manifests, values and history), protect sensitive release data, and bind each served result to those source bytes. Without those artifacts, report Helm state as unavailable/unknown and leave that case's Helm baseline coverage open. Do not fabricate an installed release, a release history, or a missing release.

## Smallest full-baseline gate

Before a full Experiment A run, publish the exact read-only kubectl/Helm command set required by the frozen questions and a coverage row for every request: source file/receipt, capture scope, projection rule if any, served-response hash, expected client result, and failure behavior for missing evidence. Run real pinned clients in the approved isolated recorded environment; deny writes and make unsupported requests fail closed. Verify byte-identical raw packets in both arms, every projected fact against source bytes, and actual tool grants/inventory/use. Then complete the existing 24-case grader, retry/cost, descendant and review gates. A packet can pass a bounded path test while the overall Experiment A remains unready; no missing row is upgraded to evidence by the adapter.
