# `map list --kind` request scope

To inspect Deployments in one namespace, run:

```sh
./cub-scout map list --kind Deployment --namespace team-a --format json
```

Previously, the live collector listed every configured built-in and first-class
controller collection, then discarded rows whose Kind did not match. It now
uses explicit canonical Kind-to-GVR mappings to omit only collections known
not to contain the requested Kind. A custom GVR remains in the request set
because its configuration has no Kind field and it could contain a
`Deployment`. The separate ApplicationSet lookup remains for ownership
lineage. Empty and unrecognized Kind filters keep the unfiltered request set.

The deterministic HTTP test `TestMapListKindFilterLimitsLiveRequestsAndRetainsUnknownGVR`
checks the exact request paths and visible ownership result; its fixture issues
three GETs for this filtered example (the Deployment collection, an unknown
custom GVR, and the ApplicationSet dependency). This is a request-count test,
not a measurement of time, bytes, or cost on a live cluster.
