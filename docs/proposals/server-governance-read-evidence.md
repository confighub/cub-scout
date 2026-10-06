# Server governance read evidence — proposed 2.13 dependency

**Draft, not an implemented API or acceptance result.** This packet makes the
remaining [#591](https://github.com/confighub/cub-scout/issues/591) and
[#597](https://github.com/confighub/cub-scout/issues/597) requirements reviewable.
It does not change their scope, the read-only boundary, or the release gates.
The [2.13 checklist](../releases/v2.13-readiness.md) remains authoritative.

## What is available and what is missing

The implemented exact-space ChangeOrder GET projection preserves identified
order Stage/State and prerequisite declarations, with explicit unknown evaluation.
Its parser contract pins [SDK v0.6.8 source](https://github.com/confighub/sdk/blob/4c8d2fc3885fed0d7af6835f2aac0a24387b6221/core/openapi/goclient-new/models.gen.go).
That source pin describes a client shape, not the deployed server's behavior.
The [recorded contract investigation](https://github.com/confighub/cub-scout/issues/597#issuecomment-5947655272)
found evaluated gates attached to a promotion response, outside Scout's accepted
GET boundary. A declaration, `Completed` state or an empty scoped order list
cannot establish evaluated approval, absence of governance or runtime health.

Genuine v0.8.3 recordings and the production adapter now establish direct
revision-reference claims, observed revocations, expiry and filtered-list
omissions. [Live adapter acceptance](../../examples/receipts/attestations/README.md)
also verifies each served revision's bytes against its reported DataHash before
retaining that digest in a receipt subject. This is a per-read comparison, not
a global normalization specification. Effective/inherited coverage, query
completeness and viewer RBAC remain unsupported by this accepted read shape.
Authored fixtures cannot establish those server authority facts.

## Required authority contract

These are requirements for a server-supported read contract, **not proposed
wire field names or claims that an endpoint exists**. An existing GET may satisfy
them, or the server may need an additional read-side projection. Scout must not
substitute a promotion, dry-run POST, local policy evaluator or write action.

| Read | Identity and coverage required | Facts required | Missing/partial behavior |
|---|---|---|---|
| Attestations covering one revision | Exact space, unit ID/slug, revision ID/number; direct versus effective/inherited coverage reported separately; query completeness and permissions. | Original attestation ID/type/result/subjects, attester, creation/expiry/revocation and evidence links; any effective coverage must be an explicit server result. | Unreadable referenced IDs and unsupported coverage are omissions; expired/revoked claims remain visible. No local approval decision. |
| Revision DataHash | Exact revision binding, algorithm/encoding, and a documented statement of the hashed bytes, including any normalization. | Server-declared hash; recomputed comparison only when the byte contract is confirmed and those bytes are read. | A copied digest remains server-declared; no automatic equivalence to Scout's canonical-object fingerprint or served-data hash. |
| Evaluated stage prerequisites | Exact ChangeOrder, workflow snapshot/version and stage; evaluated subjects/revisions; evaluation coverage and freshness. | Server outcome for each identified prerequisite, raw message and counts if supplied; evaluated time distinct from retrieval time. | Declared-only, stale, unsupported, denied or partial evaluations stay unknown. An aggregate clear/blocked result needs explicit authoritative scope coverage; Scout does not manufacture it. |

A returned observation must remain tied to the same queried identities even if
an order advances or a revision changes between reads. If the server cannot
provide an atomic snapshot, responses must expose that limitation and Scout must
retain it; matching names or close timestamps do not repair a mixed snapshot.
RBAC must enforce the observer permission independently of Scout's command policy.

## Genuine recording packet required

For each response, retain a credential-free read command/request description,
exact selection and response
bytes/hash, UTC retrieval time, cub version, deployed server version/identity and
permission scope. Authentication credentials—authorization headers, cookies,
bearer tokens, CLI secret arguments and credential-bearing environment—must
never enter retained command/request descriptions or public/private proof
bundles. Redaction must be documented without removing the join fields
or changing evaluated outcomes. Keep an access-controlled original if public
redaction changes bytes, and label each hash's operand. Authored controls remain
separate from server captures. Existing captures may be inspected offline; new live captures are now authorized by the maintainer’s 2026-10-05 instruction.

Required attestation cases from #591: no covering claim, Pass, Fail, revoked,
expired, multiple types, and referenced IDs unavailable in the list/read. Include
exact unit/revision data and direct/effective coverage when supported. Verify the
DataHash byte question rather than inferring it from matching values.

Required governance cases from #597: scoped absence of orders, satisfied and
unsatisfied prerequisites, blocked publish, promoted-stage activity, RBAC denial,
partial coverage and declared-only evidence. Record the authority source for each
evaluated outcome. If the deployed GET contract cannot expose evaluation, retain
that absence and leave the gate open.

## Scout implementation and acceptance after contract resolution

1. Review the authoritative GET/read shape and genuine packet before adding an
   evaluated projection. State bounded request/page/output limits and omissions.
2. Define deterministic success tests from the recordings plus authored identity,
   coverage, time, ambiguity and malformed-response negatives. Keep server claims
   distinct from Scout health and receipt verdicts.
3. Project the same evidence through the required CLI formats, loaded TUI, MCP
   and receipt paths. Preserve existing un-enriched schemas and verdict meaning.
4. Verify read-only guards, integration contracts and required genuine acceptance,
   then update #591/#597 and the release checklist with exact source/proof pins.

This packet settles the questions to answer; it supplies neither server outcomes
nor a waiver. Required live tests are now authorized by the maintainer on 2026-10-05. SDK
renderer adoption remains deferred, and the published baseline remains v2.12.4.
