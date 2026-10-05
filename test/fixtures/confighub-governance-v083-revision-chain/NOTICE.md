# Genuine revision-chain recordings — ConfigHub v0.8.3

Captured 2026-10-05 from the existing isolated acceptance server. cub and server
versions/commits, exact commands, UTC command-start times, statuses and raw stdout/
stderr hashes are in `capture-manifest.json`. Responses are unedited; no
credentials are retained. Authentication is the isolated local administrator,
not a read-only viewer. Setup writes are acceptance-harness operations outside
Scout, confined to `scout-v213-contract/revision-coverage-20261005`.

## Observed cases and boundaries

- Initial revision 2 has two direct claims: Approval and SecurityReview, both
  Pass, with exact subject Unit/Revision identities.
- A metadata-only label update retains revision 2 and its DataHash. It is not
  an inherited-coverage or new-revision case.
- Changed configuration creates revision 3 with a different DataHash. Its GET
  omits `Attestations`; omission is not a proven empty effective-coverage set.
- Restoring revision 2's configuration creates revision 4 with the same DataHash
  as revision 2 but a distinct RevisionID and no returned `Attestations` field.
  This does not establish approval inheritance or absence of effective claims.
- The explicit SecurityReview-filtered attestation list returns one claim but
  omits the initial revision's Approval reference. That claim still exists in
  the original create response. A filtered list cannot prove that a referenced
  claim is absent. This is query-limited coverage, not RBAC-denied evidence.
- Exact served revision-data bytes hash to each reported DataHash in these
  fixtures. This is observed consistency on these inputs, not a general server
  normalization specification or Scout canonical-object digest equivalence.

## Reproduction

Use a disposable server with matching cub/server versions and a unique test
unit in an explicitly selected test space. Follow the retained command order:
create a ConfigMap with `data.generation: first`; attach Approval and
SecurityReview to that head revision; GET the revision and its data; update a
metadata label; GET again; change generation to `second`; GET again; restore
the original revision; GET again. Finally list revisions and query attestations
with `Type = 'SecurityReview'`. Capture raw command output/status/command-start-time/hash at
each step. The original temporary input path was reused for the changed file;
its two values are specified here and in the served-data recordings. Setup-file
paths are command provenance, not retained immutable input-file identities.

Run `python3 scripts/ci/test_governance_revision_chain.py` to verify this packet.
Do not use it to close viewer RBAC, evaluated-stage gates, effective coverage,
production adapter or CLI/TUI/MCP acceptance. Those gates remain open in #591,
#597 and the release readiness checklist.
