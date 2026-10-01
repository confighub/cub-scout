# Trace one selected Kubernetes context

This example demonstrates normal and reverse Trace using an explicit context,
including when the kubeconfig's current context points elsewhere. All Scout
commands below observe existing resources; no fixture installation is required.
Replace the illustrative context, namespace and resource names with objects you
are authorized to read. A context label is not a stable cluster identity.

Build the local binary from the repository root:

```bash
go build ./cmd/cub-scout
```

Read an existing Application from its exact Kubernetes namespace:

```bash
./cub-scout trace --app frontend -n delivery --kube-context staging --format json
```

The JSON `context` is `staging`; the chain describes the Application read from
that binding. An Argo server login is not used. Missing Kubernetes permissions
cannot be remedied by an unrelated Argo login. If namespace is omitted,
Application lookup requires a unique name across namespaces.

Walk an existing Pod's Kubernetes ownership chain:

```bash
./cub-scout trace pod/api-abc -n team-a --reverse --kube-context staging --format md
```

Open the same selected context in the standalone TUI, select a workload and
press `T`, then confirm the selection. Returning and tracing again reuses the
captured binding; it does not imply cached observations or zero new reads.

```bash
./cub-scout map --kube-context staging
```

For the MCP `trace` tool, the corresponding arguments are:

```json
{"resource":"application/frontend","namespace":"delivery","context":"staging"}
```

## Expected degradation

- Empty or missing named contexts fail without using the current context or
  in-cluster credentials. Explicit selection does not change `current-context`.
- Known chain evidence remains visible when supplementary timing, Events,
  Secret-reference or artifact reads fail. JSON warnings name omissions;
  missing evidence does not prove healthy or unmanaged state.
- Secret evidence exposes references/metadata, not Secret payloads.
- Explicit context plus fixture input or legacy delegated `--diff` is refused
  before observation. Full controller diff context/read-only support remains
  unfinished in #746; the local comparison primitive is not a public command.
- ConfigHub authentication is not required for these standalone examples.

## Deterministic proof

From the repository root, these fixtures use local HTTP test servers and no
cluster or ConfigHub credentials:

```bash
GOPROXY=off GOTOOLCHAIN=local go test ./cmd/cub-scout -run 'TestTraceCLI|TestMCPTraceContext|TestCapturedTraceTimingOmissions|TestTraceSessionNestedReaders' -count=1
```

The CLI tests place ambient context on Beta and explicitly select Alpha, verify
Alpha evidence and zero Beta requests, and check kubeconfig bytes are unchanged.
Additional fixture tests retarget the source kubeconfig after session capture.
These deterministic fixtures do not substitute for the pending disposable-cluster
live proof of the full #746 packet.
