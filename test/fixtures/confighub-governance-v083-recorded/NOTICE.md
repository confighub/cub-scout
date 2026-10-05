# Genuine server read capture candidate

Captured on 2026-10-05 from a disposable ConfigHub v0.8.3 instance with cub
v0.8.3. Entity identifiers and timestamps are server responses, not authored
examples. The capture manifest records commands, exit statuses, retrieval times
and hashes of the exact command-output bytes. No credentials are included and
no response fields were edited. Local administrator authentication is explicit;
this packet does not prove read-only viewer RBAC.

Fixture writes were performed by the acceptance harness, outside cub-scout.
The missing-ID read names an intentionally absent UUID and returns an actual
server refusal; that UUID does not identify a claimed existing entity.

The exact revision-data stdout hashes to its Revision.DataHash for this fixture.
The packet includes direct revision attestation references, Pass/Fail claims,
short expiry, and a separate revocation entity. A revoked claim's own GET remains
immutable. The ChangeOrder here has no ChangeWorkflow and therefore does not
prove prerequisite evaluation or stage-gate coverage. Further effective/inherited
coverage, access-denied list coverage and user-facing acceptance remain required.
