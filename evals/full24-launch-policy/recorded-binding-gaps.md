# Remaining recorded MCP binding contracts

Offline inventory on 2026-10-04, against the archived verified preparation report SHA-256
`5bbb24f55457068050cc19427b84c53da8cdd113f5dd87f67692d45941e7fa71`.
This is a contract inventory, not execution or new evidence. The eleven existing
single-object/export bindings remain candidates; these thirteen stay blocked.
Frozen prompts, evidence source bytes and tool grants are unchanged. The
[source refresh proof](source-refresh-proof.json) records the later treatment
skill metadata update separately; historical runtime proofs retain their original
source binding.

| Case | Existing source shape | Contract needed before recorded MCP admission |
|---|---|---|
| DEL-01 | Pinned Flux run-log excerpt, explicitly not raw API output | Typed excerpt evidence preserving source lines and applied-digest claims; never create a Kubernetes snapshot from prose. |
| DEL-02 | Pinned Argo publication excerpt | Keep publication and later consumption observations distinct; expose source scope rather than infer atomic reconciliation. |
| DEL-03 | Sveltos delivery proof plus onboarding/known-behaviour excerpts | Preserve timestamp-inferred release binding and missing exact proof. Authored control has different evidence and must bind separately. |
| DEL-04 | OCI installer/render/catalog receipts under `evidence/oci-identity-lifecycle/` | Typed receipt identities and digest joins; no fabricated workload objects. Authored control remains separately scoped. |
| HLT-01 | Sveltos transcript projection, explicitly not a Kubernetes resource | Delivery Provisioned and continuous health observations remain separate, with original source lines and non-atomic timing. |
| HLT-03 | Captured child Application fields, receipt and source provenance | Child JSON omits `apiVersion` and `kind`; establish exact GVK through pinned capture provenance before adding an explicit identity overlay. Do not infer it from filename or managedFields. Preserve named residual Ingress separately. |
| HLT-04 | Scenario metadata and check-execution receipt | Preserve report time versus older underlying check time and source provenance; refreshed reporting does not refresh checks. |
| INV-04 | Populated, empty and denied Deployment responses | Exact named capture/scope selection; unreadable coverage is distinct from empty, and contexts are never merged. |
| PRE-01 | Before/after discovery, CRD/object responses and apply stdout/stderr | Read-only frame selector for each recorded request; preserve absent/present sequences and failures without running apply. |
| PRE-03 | Parent/child Application fields, tree and pod/event text | Captured Application JSON omits GVK; bind identity from pinned capture provenance. Parent sync does not replace child or pod/event evidence. |
| PRE-04 | Desired placement matrix and config provenance | Desired component/cluster placement is not observed live inventory; disabled/unobserved states remain distinct. |
| RUL-02 | Timestamped cache-replay sequence | Explicit frame/cache timing semantics, never collapse replay frames into one current snapshot. |
| RUL-03 | Context map, readable and denied API response bodies | Exact context/request source reader is prepared in `../recorded-api/context_frames.py`, with pinned bytes and denial/no-fallback controls. MCP adapter/runtime integration remains blocked; denied evidence stays unknown. |

All of these source files remain equally readable in both experiment arms.
A future adapter must expose only evidence already in the selected case, retain
its exact hashes/provenance and add negative scope/identity/denial tests. It must
not answer the grader question directly, copy an oracle, change source inputs or
claim that this inventory supplies the missing runtime implementation.
