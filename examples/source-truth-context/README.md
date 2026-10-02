# Source-truth with an explicit Kubernetes context

`compare source-truth` accepts an exact kubeconfig context for its runtime and
controller Kubernetes reads. ConfigHub remains a separate connected service;
the kubeconfig context does not select a ConfigHub account, space, or server.

```bash
./cub-scout compare source-truth Deployment/api -n team-a \
  --strategy git-argo --kube-context production-a --format md
```

The selected context label is included in the evidence for review. It is not a
stable identity for the cluster. If `production-a` is absent from the kubeconfig,
the command returns an error before reading Kubernetes; it does not fall back
to `current-context`. With no explicit option, the normal default kubeconfig
loader is used.

For Flux strategies, the CLI child receives a private kubeconfig captured from
the selected context. Argo Application resources are queried through the same
Kubernetes binding. An Argo CD server context is a different identity and is
not used to resolve the Kubernetes context.

The TUI offers the same operation on a selected Deployment, StatefulSet, or
DaemonSet: press `Y`, choose the declared delivery strategy, then press Enter.
It does not infer a strategy. Both renderings use the source-truth evidence
model. The deterministic local-HTTP tests in
`cmd/cub-scout/source_truth_binding_test.go` validate explicit-context binding,
retarget isolation, denied-read behavior, Flux child binding, and the TUI event
path without contacting a cluster.
