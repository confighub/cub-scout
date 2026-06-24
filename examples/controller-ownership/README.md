# Controller Ownership Fixtures

Static fixtures for controller ownership detection. These manifests document the
metadata cub-scout parses for controllers beyond the original Flux / Argo CD /
Helm set.

## Sveltos

`sveltos-modelplane.yaml` includes a Deployment with:

- `projectsveltos.io/owner-kind: ClusterProfile`
- `projectsveltos.io/owner-name: config-to-production`
- `projectsveltos.io/reference-*` annotations identifying the ConfigMap source
- Helm labels, proving Sveltos provenance wins over generic Helm metadata

Expected owner:

```text
Sveltos / clusterprofile / config-to-production
```

## Modelplane

The same fixture includes:

- a `ModelEndpoint` served from the `modelplane.ai` API group
- a Crossplane Helm `Release` labeled `modelplane.ai/release`

Expected owners:

```text
Modelplane / modelendpoint / qwen-demo-0
Modelplane / release / traefik
```

When equivalent objects are present in a cluster, inspect them with:

```bash
./cub-scout map list --owner Sveltos
./cub-scout map list --owner Modelplane
./cub-scout tree ownership --owner Sveltos
./cub-scout tree ownership --owner Modelplane
```
