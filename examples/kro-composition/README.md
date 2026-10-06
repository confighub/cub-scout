# kro Composition Example

Minimal fixture showing kro ownership and composition lineage.

This example demonstrates:
- `kro` ownership detection (`kro.run/*` + owner references)
- `cub-scout trace` platform lineage rendering for kro
- `cub-scout tree composition` grouping under kro instance roots

## Files

- `chain.yaml` — ResourceGraphDefinition, instance CR, and generated Deployment.

## Use

> Requires kro CRDs in the target cluster (`kro.run/v1alpha1` and your instance CRD).
> If those CRDs are not installed, use this fixture as a reference/test artifact instead of applying it.

```bash
kubectl apply -f examples/kro-composition/chain.yaml

cub-scout map list -q "owner=kro"
cub-scout tree composition
cub-scout trace deployment/checkout-api -n app
```

Cleanup:

```bash
kubectl delete -f examples/kro-composition/chain.yaml
```

## Conservative owner-reference control (2.14 candidate)

`owner-reference-scope.yaml` is an authored offline example. The child refers
to an old owner UID; that UID exists only in another namespace, while the
same-namespace parent has a replacement UID. The result stays partial rather
than joining either parent.

Owner-reference joins now require a unique exact API version, Kind and name,
legal same-namespace/cluster locality, and matching UID when supplied. Definition
owner references only match cluster-scoped observations. Duplicate or conflicting
candidates remain partial. Self references are excluded. Missing parent scope
stays unknown; a blank namespace on a partial reference does not prove cluster
scope. Evidence reports `instance:` or `definition:` with `unresolved`,
`ambiguous` or `owner_uid_not_observed`.

Three resolver controls and one shared reverse-trace rendering control cover
namespace/type/version/UID conflicts, duplicates, legacy missing-UID behavior,
cluster and definition cases, reversed inputs, JSON, human output and the
existing loaded TUI trace pane. No added API reads or genuine controller/TUI
process proof. Metadata-only definition lookup and ownership group recognition
are separate paths; this packet does not certify those paths or full controller
coverage, health, UID/cluster joins or connected/fleet acceptance.
