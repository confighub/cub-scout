# Recorded delivery-settings objects

`applications.json`, `kustomizations.json` and `helmreleases.json` are
`kubectl get <kind> -A -o json` from the owned kind cluster created by
`examples/delivery-settings/verify-live.py` (Argo CD v3.5.3, Flux installed by
flux CLI 2.8.6, Kubernetes v1.35.0), unedited. They are what a real API server
returned, including fields the CRDs default: the Flux Kustomization CRD writes
`force: false` on an object that never mentioned it.

`expected-declared.json` was written by hand from the specs in these files, not
generated from the code under test.
