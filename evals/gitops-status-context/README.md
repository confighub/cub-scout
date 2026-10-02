# GitOps status context interpretation case

This opt-in case contains synthetic, fixture-backed summaries for two
independent Kubernetes contexts, including an explicit Modelplane API denial.
It tests whether an agent reports the context label and partial coverage
without promoting either to stable cluster identity or complete health.

**Model execution has not run.** These records do not come from a live cluster
or ConfigHub service and establish no real RBAC or connected-mode behavior.
The scaffold only copies the two checked-in JSON files into a temporary
`recorded/` directory.

## Owned-kind live capture packet (#753)

`capture_live.py` is a draft proof harness for the CLI, MCP stdio `gitops_status`
tool, and the immutable GitOps status TUI summary. It shares the bounded command
runner/receipt primitives from `evals/doctor-scan-context/capture.py` and the
GET-only forwarding proxy from `evals/trace-context-live/api_proxy.py`.

**This helper is not admitted for live execution.** `--execute` currently exits
before reading the shared kubeconfig or creating output. Root review identified
remaining environment isolation and expected `cub` shim-call contract work. Do
not remove that refusal until those items are fixed and independently reviewed.
No owned-kind cluster was created for this draft.

The proposed synthetic fixture has an Argo Application on an inert
`example.invalid` source and two synthetic CRDs: Argo Application and
ModelDeployment. It does not install or claim to exercise either controller.
Three private context bindings are planned: admin allowed, a service account
allowed controller lists except ModelDeployment, and a service account allowed
the same controller lists but denied Pods. The separate pod denial is required
because a controller-list denial alone does not prove runtime Pod omission.
Expected evidence includes a visible Modelplane `unreadable`/`forbidden`
omission, a `runtimeOmission` for Pods while preserving Application health, and
the selected kubeconfig label in each output. The label does not identify a
stable cluster.

Run only the offline acceptance controls:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest evals/gitops-status-context/test_capture_live.py -v
```

The `gitops_status_tui_live_test.go.txt` probe runs the production status
collector and drives the actual viewport model's resize, scroll, and quit
messages. It is not a terminal-emulator test or interactive refresh proof.
