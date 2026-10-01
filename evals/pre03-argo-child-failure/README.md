# PRE-03 Argo-only child-chain failure evidence

This prepared recorded-projection case uses retained Argo/Kubernetes runtime
observations from `confighub/helm-expt` at
`9ab4c753a888dc305a3c07956c9f8f5a19eb70a0`, under
`runs/live-helm-confighub-compare/bitnami-spark-ha/`. The six observation files
are byte-preserved. `capture-scope.json` pins their source paths, SHA-256 and
Git blob IDs, plus the source/run scope. Both arms receive exactly the same
seven files from the fixture-owned scaffold.

The retained root Argo Application is Synced/Healthy and tracks the
`argocd/spark-parity` child by GVK/namespace/name. The child Application's
tracking annotation names that parent; it is Synced/Progressing and its tree
shows the `spark-worker` StatefulSet with `spark-worker-0` below it. The Pod
describe/events retain `ImagePullBackOff`, `ErrImagePull`, and the exact
registry NotFound message for the Spark image. The two Argo Application UIDs
are separate object identities; the observations do not provide a UID
foreign-key between them. The recorded tree establishes the displayed
StatefulSet-to-Pod path, not a complete Kubernetes object graph.

This satisfies the frozen PRE-03 reference/control for an explicitly
**Argo-only** branch: a Synced/Healthy parent does not substitute for inspecting
the failing descendant. The supplied files contain no Crossplane Claim, XR, or
composed managed-resource chain. No Crossplane coverage is claimed, and this
case does not close #601 or assert Crossplane behavior.

## Time, receipt, and cleanup limits

The data are historical, sequential observations, not an atomic or current
snapshot. Object creation, health transition, and `reconciledAt` values are
object timestamps, not exact per-file response capture times. The event ages
are relative. The source parity receipt has `observedAt` before the Application
object times, so it is not used as their capture timestamp and is excluded
from model-facing files because it aggregates run summaries. The receipt
records cleanup as blocked while a separate lifecycle field says cleaned up;
that conflict does not confirm cleanup. See the [evidence report](../reports/2026-10-01-pre03-argo-child-failure.json)
for exact pins and limitations.

The case is prepared, not model-run or benchmark-admitted. It does not claim a
current immutable image identity, full child inventory, Crossplane coverage,
model quality, provider cost, account credits, or savings.

Pure fixture contract checks run as part of `go test ./test/unit` with an
explicit offline kubeconfig. No capture, cluster, provider, or model is run by
the fixture or tests.
