# Complete Trace diff observation contract

Execution dependency: [#746](https://github.com/confighub/cub-scout/issues/746),
under #599 and the adopted 3.0 plan. This proposal records the unresolved diff
portion; it does not declare that packet complete or alter the roadmap gates.

## Current implementation and evidence

Normal/reverse Trace and the public local-rendered diff use a captured
Kubernetes binding. `trace --diff` without `--desired-file` now fails before
reads with migration guidance; it no longer invokes Flux, Argo, or Helm diff
delegates. `trace --diff --desired-file FILE` compares one already-rendered
object with one exact live object. It is not controller desired state.

The upstream source audit used Flux revision
`04acaec6161ac4fb1a82ffafa88901c03271d34f` and Argo revision
`7ae7d2cc723f5408b080a31263e705198af08613`. Installed versions reported modified
builds, so those pins do not establish binary equivalence.

- Flux `diff kustomization` requires a local path and performs server-side dry
  run. Scout's delegate supplies no path. Its exit status is not sufficient
  evidence that live state matches Git, and it is not a GET-only observer.
- Argo `app diff` can use server-side diff based on an Application annotation.
  Even disabling that option does not bind all Argo services or an Application's
  destination cluster to the selected Kubernetes endpoint.
- Helm's existing diff path gives plugin guidance, not a computed desired/live
  comparison. Sveltos/Modelplane give observation guidance.

Source links are recorded in the #746 audit comment. No actual external diff
request was needed to establish these gaps.

## Required operands

Every comparison needs explicit desired and observed operands:

1. Desired: exact API version/kind/namespace/name, source role, capture time when
   known, and revision/digest only when supplied by that source. Local rendered
   files are caller-supplied rendered configuration, not automatically Git or
   the controller's current desired state.
2. Observed: one captured cluster binding, exact object identity, observed UID
   and resource version when available, observation time and read evidence.
3. Join: exact identity and scope. No matching Application names across unrelated
   Argo server/cluster sessions; no inferred target mapping or digest-role swaps.

Comparisons report authored-field deltas. They must not promise exactly what a
future reconcile will do: admission, defaulting, controllers and hooks may affect
that outcome. Missing objects and unreadable objects remain distinct. Secret
payloads and their content digests are omitted; omission is never a clean result.

## Implementation sequence and compatibility

1. The local-rendered-vs-live primitive, CLI/MCP/TUI projection, safe
   no-operand migration behavior, and deterministic endpoint-binding proof are
   now implemented. Remaining final integration review and release gates do
   not change its scope: it remains authored-field-only for one object.
2. Define additional desired-operand selectors/providers without changing the
   meaning of the local-rendered mode. Validate every operand before any
   provider/child call; do not route a supplied context into ambient external
   delegates.
3. Establish each controller's desired-observation provider independently:
   exact source/destination binding, authenticated provenance where available,
   no refresh/reconcile/server-dry-run side effects, and named unavailable
   states when the provider cannot supply its rendered operand. Scout does not
   acquire a hidden rendering engine to fill this gap.
4. The unsafe public delegate route has been removed with compatibility
   guidance and a rendered-input example. Continue to keep unavailable
   controller-desired providers explicit under #746; the local comparison does
   not close the complete diff request.

## Success proof before public integration

- Local file and directory examples: changed/matched/missing/unreadable objects,
  ambiguous identities, malformed manifests and Secret payload omission.
- Two API endpoints with colliding names, source kubeconfig retarget after
  capture, unchanged shared config, exact GET paths and no mutation verbs.
- Source-specific desired identity/revision tests, including wrong cluster,
  wrong namespace, stale desired capture and revision/digest role mismatches.
- Actual CLI/MCP/TUI action paths and format parity, cancellation, omitted
  coverage and source/read costs. No unsupported automatic follow-up commands.
- Before/fixed behavior proof, disposable owned-cluster capture, independent
  review, build/full suite and exact-head CI before packet completion.

The current public comparison proves only local operand selection and bound
live reads. Controller-desired providers, installed-Helm operands, and final
release/live proof remain missing; their absence must stay visible in status
and release claims.
