# HLT-01 — Sveltos Provisioned versus workload health

This is a recorded-evidence case for benchmark-v1 HLT-01 (#603), informed by
Sveltos observations tracked in #641. It is prepared for review and has not been
run. It does not make the 24-case manifest executable and supports no product
or savings claim.

## Success criteria

Both arms receive the identical case-owned YAML transcript projection. A
verified answer must distinguish delivery-profile state from observed workload
health and return the case's exact five-field JSON contract. It must report
what the excerpt records for the named cluster, retain synchronized delivery
state beside degraded health, and mark release identity unknown. It must not
call `Provisioned` proof of current health, claim the excerpt is an atomic or
current-live snapshot, or infer an exact release from a truncated revision
string.

The required grader rejects a healthy conclusion, a missing or altered
controller/report field, any claimed exact release identity, additional
properties, and prose outside the JSON object. It checks the declared answer
shape only; it does not grade arbitrary explanations.

## Evidence and provenance

The fixture is a projection of **only Part B, source lines 109–131** from
`health-2026-09-30.log`; Part A's manually run probe is excluded. The pinned
source is `confighub/sveltos-confighub` revision
`8187910f9fe226e109e55c4d9c7c0e21297ff424`, as recorded in
[`docs/roadmap-3.0-execution.md`](../../docs/roadmap-3.0-execution.md#source-ledger).
The whole source-file SHA-256 is
`24ef8acd48ead05400604198bb0ea695c284c146ab68f27b10a6601f75a655a8`. The
source lines were selected with `sed -n '109,131p' health-2026-09-30.log`;
the fixture records that line range and the SHA-256 of those exact line bytes
joined with LF and a final LF. The source file itself is not copied into this
repository. The checked-in projection file SHA-256 is
`49e34b3bb6dc9adbe28c6be75ba97a51615149298a361a230efeb5aa5c18a3ef`.

The YAML is explicitly a transcript projection, **not a Kubernetes resource**.
Its embedded `cordon` and `delete pod` command lines are historical recorded
text, not instructions to execute. The case permits only file reading/search;
any future shell grant must be scoped to read-only query commands, never those
historical mutations.
It preserves the recorded sequence: the ClusterHealthCheck reports `False`
with degraded/progressing Deployment evidence at 07:40:43; the ClusterSummary
still reads `Provisioned`; the later status output shows `Synced` and `Degraded`,
and the Space live-status projection repeats those values. The source gives no
timezone and does not timestamp each later command result. These outputs are
sequential, not an atomic snapshot.

The status table contains only a shortened `sha256:` revision prefix. This
case makes no exact release/digest, release-freshness, or release-to-health
binding claim. It does not establish the state of the cluster now, reproduce
the external Sveltos/ConfigHub environment, or add synthetic Kubernetes object
metadata. There are no live reads, cluster mutations, or paid eval runs in this
packet.

## Equal evidence and scaffold

`scaffold.sh` writes the checked-in transcript projection unchanged to
`./cluster/sveltos-health.yaml` for each arm. It is marked
`FIXTURE-OWNED`; `evals/scripts/record.py --scaffolds-only` preserves it, while
the unit test verifies the scaffold bytes, fixture hash, source hash/range and
answer-contract negatives. Neither arm receives an MCP recording or treatment-
only evidence. Benchmark admission remains pending the frozen full-suite
protocol and the other cases.
