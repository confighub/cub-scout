# Watch cache namespace proof (#735)

This is an opt-in, read-only comparison harness for the namespace-reuse fix in
`cmd/cub-scout/observation_watch.go`. It has not been run against a cluster.
No live behavior or performance result is claimed.

## Machine acceptance contract

The source revisions are fixed: before `d7f081e88e1841a9557e086a8c0b479c8180008d`
and after `7611908afeb361b15a03d2687ccdaff6c504a811`. Both phases use one fresh
local kind cluster and the same three namespaces/ConfigMaps, sequentially and
without fixture changes. A namespaced service account has `get`, `list`, and
`watch` only for ConfigMaps in A and B. It has no binding in the denied
namespace and no cluster role.

The probe records exact dynamic-client HTTP method/path/watch/status tuples and
compares sorted `(namespace, name, UID)` identities. The direct API control
must return the same-name ConfigMap in both A and B, a Forbidden result for an
all-namespace list, a Forbidden result for the denied namespace, and an API
error (Forbidden, BadRequest, or NotFound) for a namespaced URL to
cluster-scoped Nodes. The fixed-source probe must preserve the exact observed
direct API error rather than assuming which authorization/routing layer
rejects that invalid request. The old source must fail exactly
these cache-reuse checks: B, all namespaces, denied namespace, and the
cluster-scoped request. The A cache hit remains correct. The fixed source must
pass every comparison, issue live fallback LISTs for B/all/denied/cluster
scope, return direct-identical identities for A/B, and preserve the direct API
errors for all/denied/cluster scope. Missing or unexpected evidence fails the
capture; a generic test timeout is never accepted as a regression.

## Offline checks

These checks do not call kind, kubectl, Docker, Kubernetes, or any remote
service:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest evals/watch-cache-namespace/test_capture.py -v
```

The Go probe source is copied only into two isolated temporary source
worktrees. The template is named `.go.txt`, so package discovery never compiles
it as a separate package. The old-source adapter alters its constructor call
and adds no scope-map field; it does not patch `cacheableList` or any old
product source. After independent review, the runner compiles both probe/source
pairs using `go -C <absolute-source-worktree> test ./cmd/cub-scout -run '^$'
-count=1` before creating a cluster. Each command has both an explicit
Go `-C` worktree, plus a private empty `KUBECONFIG`; it executes no tests and
has Go module/toolchain downloads disabled. Only after both compile
steps pass does it create the owned cluster and run the single live probe test
from each pinned worktree with the explicit private observer config.

## Reviewed invocation shape (do not run before independent review)

The runner refuses execution without `--execute`, a fresh `/tmp` output path,
and an explicitly supplied `--integrity-only-shared-kubeconfig`. That last
file is read only to hash it before and after; it is never parsed or passed to
any command. The actual test receives only the newly generated mode-0600
observer kubeconfig through explicit `KUBECONFIG` and
`SCOUT_WNS_PRIVATE_KUBECONFIG` environment variables. There is no default or
shared-config fallback. The helper requires local Docker, kind `v0.31.0`, the
existing digest-pinned kind node image, and existing kubectl/go executables; it
does not download or install tools. The API endpoint and current context are
checked as belonging to the invocation-generated loopback kind cluster.

After independent review, an operator may run:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/watch-cache-namespace/capture.py \
  --execute \
  --integrity-only-shared-kubeconfig /explicit/path/to/kubeconfig-to-hash-only \
  --output-dir /tmp/scout-watch-cache-namespace-proof
```

The fresh cluster, namespaces, ConfigMaps, role bindings, source worktrees, and
private kubeconfigs are invocation-owned. Cleanup is attempted only for the
unique cluster name in the invocation marker and is accepted only after `kind
get clusters` confirms absence. `provenance.json`, exact probe request/status
facts, phase outputs, source/probe/tool hashes, shared-config before/after
hashes, private-config hashes, timings, errors, and cleanup result are retained
in the mode-0700 output directory. Private kubeconfig files are removed after
cleanup. The helper has a 10-minute overall probe deadline and bounded
per-command/startup/cleanup limits.

The live proof is deliberately narrow: it does not measure cache freshness,
reconnect behavior, storage bounds, complete inventory coverage, or production
latency. The before/after phases are sequential, not an atomic snapshot. No
live cluster proof has yet been performed.
