# Pipeline Source Resolution

This reference defines how `cub-scout map` pipeline views resolve the displayed source.

## Resolution order

1. Flux `Kustomization`: `spec.sourceRef.name`
2. Flux `HelmRelease`: `spec.chart.spec.chart`
3. Argo CD `Application`: `spec.source.repoURL`
4. Fallback: `unknown`

## Meaning of `unknown`

`unknown` means the source field is missing or unreadable in the deployer spec.

It does **not** imply an error by itself. It means cub-scout cannot extract source metadata from the object fields above.
