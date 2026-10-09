# Independent scopes, one Scout process per call

Question: “Can I combine these observations without confusing same-name objects
or losing a denied cluster?” This external serial caller reuses the opt-in map
contract and [instance evidence](../cluster-identity-cost/). Scout itself still
observes a single selected cluster per call. No ConfigHub access is required;
this example explicitly disables connected reads.

Build the local binary and supply **explicit** scopes in a private JSON file:

```sh
go build ./cmd/cub-scout
python3 examples/fleet-by-orchestration/observe.py /private/scopes.json > observations.json
```

Example input (replace paths/contexts with your own; do not commit credentials):

```json
[
  {"label":"test","context":"same-label","kubeconfig":"/private/test.yaml","namespace":"team-a"},
  {"label":"production","context":"same-label","kubeconfig":"/private/production.yaml","namespace":"team-a"}
]
```

`label` is a unique caller selection, never a cluster identity. Context names may
match because each call uses its supplied kubeconfig. The script calls
`./cub-scout map list --cluster-identity --format json --kube-context … --kind
Deployment`, with an optional namespace, once per scope, serially. It never
switches context, writes to a cluster, renders manifests or executes a model.
`--scout-directory` selects another directory containing the local binary.

The output retains each original map envelope and adds an index of verified
instance keys pointing back to its rows. It checks cluster binding, row identity,
explicit scope and canonical key components; context/server/name/legacy IDs never
fill gaps. Duplicate keys retain every observation. A denied scope retains its
rows without indexing them; command failure, timeout or unsupported response is
an explicit omission. Malformed output and subprocess stderr are not echoed.
No observation replaces another, and no denied scope is dropped.

`status: complete` means these collection/identity checks had no omission. It does
not mean health, freshness, an atomic snapshot, verified Target binding or fleet
membership. These are sequential dated observations. Original identity-reader
cost fields keep their limited scope; this example does not report whole-command
costs, transport authentication costs or savings. Check `status` and per-scope
`omissions` even when the script exits successfully. Input/configuration errors
fail before any call. Calls have a 30-second timeout; input is limited to 32 scopes.

Run offline controls without Kubernetes, credentials or network:

```sh
python3 -m unittest discover -s examples/fleet-by-orchestration -v
```

Executable authored fixtures assert exact argv/private-config selection, two
same-label clusters, denied inventory retention, failed/invalid output, malformed
or conflicting references, duplicate-key retention and order-independent index.
These are authored controls, not real cluster captures. The underlying product's
CLI/MCP/TUI proof remains in the linked instance example; broader #599/#596 and
v2.14 publication remain open.

## Genuine external-process acceptance

The [live receipt](live-proof.json) and [actual output](live-observations.json)
come from clean source `446e7d7d`, an isolated candidate build and two owned
Kubernetes 1.35 clusters. Both contexts were named `same-label` and both workloads
were `team-a/api`. The actual external script called Scout once for each cluster
and once for a restricted reader, produced two distinct verified instance keys,
and retained the denied selection/row without indexing it. A direct restricted
Namespace GET returned Forbidden. Shared kubeconfig stayed unchanged; both
owned clusters were removed. No paid model calls were made.

Reproduce from a clean committed checkout with Docker, kind, kubectl and the Go toolchain `go.mod` selects:

```sh
python3 examples/fleet-by-orchestration/verify-live.py
```

The harness creates only uniquely named owned clusters, builds into a private
temporary directory, and uses private kubeconfigs. Raw credentials/captures stay
in that mode 0700 directory; committed output contains no token/kubeconfig.
This proves this external workflow, not whole-command cost accounting, fleet
membership, Target binding, current application health or all command surfaces.
