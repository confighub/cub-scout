# Cluster identity and read-cost foundation

User question: "Can two contexts with the same label still be kept distinct,
and what did reading their identity cost?"

This library foundation for [#599](https://github.com/confighub/cub-scout/issues/599)
is not yet integrated into CLI, MCP or TUI output. Existing command request
budgets remain unchanged. It complements the wider
[observation request baseline](../observation-budget/).

From the repository root, reproduce the deterministic loopback controls:

```sh
GOPROXY=off GOTOOLCHAIN=local \
  KUBECONFIG=/tmp/scout-offline-validation.kubeconfig \
  go test ./pkg/agent -run 'TestClusterIdentity|TestKubernetesReadMeter' -count=1 -v
```

Two independently configured HTTP fixtures use the same context label and
return different `kube-system` Namespace UIDs. The reader captures a copied REST
configuration before making requests. Changing the original configuration does
not retarget it. Construction makes zero requests; every successful read makes
one `GET /api/v1/namespaces/kube-system`, with no discovery, redirect, REST retry,
workload read, identity cache or second-context fallback. Existing bounded,
noninteractive exec authentication remains in use; authentication refresh does
not retry the identity request.

Only an exact `v1/Namespace` named `kube-system` with a nonempty observed UID
establishes verified Namespace-instance identity. This distinguishes the two
fixture instances; it is not a universal infrastructure identifier or a
ConfigHub Target join. Recreating that Namespace can change the identity.
Missing UID, denial, malformed responses, timeout and unreachable endpoints
return unverified identity, an omission reason and no successful observation
timestamp. They do not establish ownership, orphan status or workload health.
API endpoint output omits user information, query and fragment fields.

The controls compare transport attempts and consumed response-body bytes with
the fixture, including error bodies, early body limits and read errors. The
meter excludes HTTP/TLS headers, wire compression, authentication traffic and
other clients. A preexisting opaque transport wrapper explicitly makes cost
coverage partial. Duration includes cancellable waiting for this reader;
separate calls are serialized for attributable meter deltas, and `reused` is
always false. Concurrent meter controls are suitable for the Go race detector.

This is standalone loopback proof, using no cluster or credentials. It does not
prove connected/fleet identity, whole-command costs, CLI/MCP/TUI conformance,
merge-safe object references or genuine live acceptance. Those remain open in
#599 and the [3.0 execution plan](../../docs/roadmap-3.0-execution.md).
