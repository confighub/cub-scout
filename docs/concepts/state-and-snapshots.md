# State and snapshots in cub-scout

> Status: Current (Deep Dive)
> Last reviewed: 2026-10-05
> Concepts index: [README.md](README.md)

Local UI state, exported observations, debug bundles and verification receipts
have different purposes and guarantees. They must not be treated as one artifact.

| Artifact | Existing purpose | Boundary |
|----------|------------------|----------|
| Local TUI snapshot | Restore a local inventory view from `~/.confighub/sessions/localcluster-snapshot.json` | Mutable local convenience state; not a portable receipt or complete investigation history. |
| `snapshot` output | Export GSF ownership/resource/relationship observations | JSON written to stdout or a file; no immutable-store or automatic cluster-name redaction guarantee. |
| Debug bundle | Retain available captured session, drift, events or logs for offline inspection | `bundle inspect`, `bundle replay` and `bundle diff` consume captured evidence; replay does not make it current. |
| Verification receipt | Record a scoped predicate, evidence, omissions and timestamp | Fingerprinted historical check; immutable local receipt storage is distinct from an ordinary export file. |

## Exporting observed state

```bash
./cub-scout snapshot --namespace prod -o state.json
./cub-scout bundle inspect ./captured-bundle
./cub-scout bundle replay ./captured-bundle
```

There is no `snapshot create` / `snapshot view` command pair. Snapshot JSON and
bundle replay are separate contracts. Consult [CLI reference](../reference/cli-reference.md)
and [JSON contracts](../reference/json-contracts.md) for their supported inputs.

The snapshot exporter retains resource identities and labels. It does not emit
Secret data values, but identities, labels and other captured artifact content
can still be sensitive. Logs and user-provided evidence are not universally
sanitized. Do not describe all exports as automatically safe to share or promise
that the cluster name is redacted.

## Freshness and reuse

A retained observation describes its capture, not today's state. Local caches
and bounded read reuse have their own expiry/refresh contracts. Reusing a
snapshot must preserve its evidence age. Missing capture time or completeness
must not be inferred. A failed refresh cannot justify presenting old success as
fresh. Connected intent and history add separate evidence sources; they do not
renew a retained Kubernetes observation.

[Receipts and proofs](receipts-and-proofs.md) defines historical integrity.
[Architecture](architecture.md) and the
[release continuity review](../reference/configuration-investigation-continuity.md)
map existing retention foundations to future history/store work. Scope-bound
investigation history remains a design follow-up, not a feature established by
this local TUI cache.
