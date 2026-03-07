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
