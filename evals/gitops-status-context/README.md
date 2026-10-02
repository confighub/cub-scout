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
before reading the shared kubeconfig or creating output. The bounded repair adds private observation HOME/XDG and restricted PATH,
exact synthetic auth-call controls, and retained failure/cleanup evidence. Do
not remove that refusal until the helper and compiled probe are independently
reviewed. Offline controls are preparation, not live acceptance.
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

The observation environment inherits no ambient tokens, plugin mode, proxies,
GitOps test hook, kube settings or host tool PATH. Setup/build uses a separate
environment and absolute tools; observation PATH contains only the `cub` shim.
It accepts exactly `cub auth status`, returns a synthetic unauthenticated result
(exit 73), and records a fixed JSON argv. Any other arguments return 97 with a
redacted rejection marker. Expected calls per process are CLI 1, MCP 3 (startup,
pre-call session, child status), TUI 1, old unsupported-selector control 0.
This proves a local stub contract, not real ConfigHub authentication.

The Kubernetes proxy accepts only exact fixture-specific status GET routes
with no query parameters. Discovery, watches, selectors, other namespaces,
mutations, and arbitrary object names fail closed. All endpoint snapshots are
validated together, including unexpected endpoint traffic. CLI/MCP/TUI require
exact Application-list, Application-object and Pod-read evidence; denial must
occur on its expected exact route and unexpected forbidden reads fail.
Credential-free observation configs are mode 0400 and hashed before/after;
owned upstream credentials stay outside product environments and command logs.
The receipt hashes source/binaries, helper dependencies, fixture/CRD/RBAC,
TUI artifacts and logs. API event logs and partial commands remain available on
failure; invocation-private files are removed during finalization. Uncertain
cluster creation retains an inventory and fails instead of deleting resources
without established ownership.

The offline tests cover environment exclusion, exact auth argv/counts and secret
redaction, actual loopback proxy refusal/closure, wrong-context and hidden-denial
controls, real MCP result-envelope dispatch, immutable-config tampering, and
failure/interrupt receipt retention with cleanup exceptions. The TUI template
checks a changed viewport YOffset/view, return-to-top state, actual `tea.QuitMsg`,
and unchanged summary/config. Compilation and owned-kind execution remain
pending; no terminal-emulator, refresh navigation, controller reconciliation,
real ConfigHub service, or paid model evaluation is claimed.
