# Modelplane on Crossplane Evidence

Use this example when the question is:

> Is this Modelplane-owned resource backed by Crossplane composition?

cub-scout keeps Modelplane as the higher-level owner when Modelplane ownership
signals are present, and surfaces Crossplane as substrate evidence only when
explicit Crossplane metadata is also present.

## Fixture

[`workload.yaml`](./workload.yaml) is an authorable Deployment with:

- `modelplane.ai/deployment`
- `crossplane.io/composite`
- `crossplane.io/claim-name`
- `crossplane.io/claim-namespace`
- `crossplane.io/composition-resource-name`

[`observed-workload.json`](./observed-workload.json) shows the same live object
shape after Kubernetes has recorded a verified Crossplane composed field
manager in `metadata.managedFields`.

## Surfaces

```bash
./cub-scout map list --owner Modelplane --format json
./cub-scout trace deployment/qwen-engine -n models
./cub-scout receipt verify deploy/qwen-engine -n models --format json
./cub-scout watch --once --output-file events.jsonl --emit-receipt-on resource.discovered
```

Expected JSON field paths:

- `map list`: `ownerEvidence`
- `watch` / `bot`: `owner.evidence`
- `receipt verify`: `predicate.evidence.platformSubstrate`

Representative payload:

```json
{
  "platform": "modelplane",
  "substrate": "crossplane",
  "composite": "x-qwen",
  "claim": {
    "name": "qwen-claim",
    "namespace": "models"
  },
  "compositionResource": "engine",
  "fieldManager": "apiextensions.crossplane.io/composed-qwen",
  "sources": [
    "label:crossplane.io/composite",
    "label:crossplane.io/claim-name",
    "label:crossplane.io/claim-namespace",
    "label:crossplane.io/composition-resource-name",
    "managedFields:apiextensions.crossplane.io/composed-qwen"
  ]
}
```

This does not mean Crossplane becomes the owner. It means the Modelplane-owned
resource exposes Crossplane substrate facts that cub-scout can show without
guessing.
