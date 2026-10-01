# Complete Trace diff observation contract

Execution dependency: [#746](https://github.com/confighub/cub-scout/issues/746),
under #599 and the adopted 3.0 plan. This proposal records the unresolved diff
portion; it does not declare that packet complete or alter the roadmap gates.

## Current implementation and evidence

Normal/reverse Trace use a captured Kubernetes binding. Explicit context with
legacy delegated `--diff` is currently rejected before reads. With no selector,
the old diff delegates still exist; they are not accepted proof of read-only,
context-bound observation and have not been run against shared infrastructure.

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

1. Finish the existing local-rendered-vs-live primitive, shared rendering and
   deterministic proof. It must remain clearly labeled; this alone cannot
   satisfy full controller-desired diff functionality.
2. Define the public desired-operand selector and its CLI/MCP/TUI projections,
   including validation before any provider/child call. Do not route a supplied
   context into the current ambient external delegates.
3. Establish each controller's desired-observation provider independently:
   exact source/destination binding, authenticated provenance where available,
   no refresh/reconcile/server-dry-run side effects, and named unavailable
   states when the provider cannot supply its rendered operand. Scout does not
   acquire a hidden rendering engine to fill this gap.
4. Remove the unsafe delegate route only with an explicit compatibility note,
   migration examples and independent review. Preserve a useful comparison
   capability; do not silently relabel local input as controller desired state.
   Any temporarily unavailable controller-desired provider stays an explicit
   unresolved dependency under #746, not a closed feature claim.

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

The current helper proves only local operand selection and bound live reads.
Controller-desired providers, public comparison UI and the final live proof are
still missing; their absence must stay visible in status and release claims.
