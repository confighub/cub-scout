# Trace a caller-rendered object against live state

This example compares one already-rendered Kubernetes object with one exact
live object. `cub-scout` does not render a chart or repository, ask Flux or
ArgoCD to render, or predict the next reconciliation.

Render with the tool already used by your workflow, then identify the exact
resource and API version:

```sh
helm template web ./chart --namespace demo > /tmp/web-rendered.yaml
./cub-scout trace deployment/web -n demo --diff \
  --desired-file /tmp/web-rendered.yaml --api-version apps/v1 --format json
```

The command reads only the selected live object (plus discovery when it must
resolve resource scope). Without `-n`, it uses the namespace in the selected
manifest. If that manifest omits namespace, pass `-n` for a namespaced object;
cluster-scoped resources reject `-n`. If kind/name/API version still selects
multiple documents, the command fails before reading the live object.

The JSON result labels the operand `local-rendered`, the comparison
`authored-fields-only`, and coverage `one-selected-object`. It includes bounded
live read evidence and distinguishes matched, changed, missing, and
inconclusive. Secret payloads are never compared or emitted, and the whole-file
digest is withheld if any Secret document is present. These results do not
establish the controller's rendered desired state or the completeness of the
resource set.

The manifest below is a tiny input fixture for the same command shape; replace
it with the output from your existing renderer before comparing a workload.
