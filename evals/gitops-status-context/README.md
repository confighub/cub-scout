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

**Admitted for one serial owned-kind lane on 2026-10-02**, after root semantic
review, independent Luna review and pinned-source TUI probe compilation.
Review found and repaired selected-equals-ambient configurations and a final
request/log race during cleanup. Seventeen offline controls pass. Execution
still requires `--execute`, existing pinned tools/image and a clean checkout.
No live acceptance is claimed by this admission checkpoint.

The synthetic fixture has an Argo Application on an inert
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
Each observation config contains all three proxy bindings and sets its current
context to a different endpoint from the explicit selector. Cross-endpoint
requests fail the action receipt, so ambient fallback cannot pass.
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
checks a changed viewport YOffset/view, PgUp return-to-top state, actual `tea.QuitMsg`,
and unchanged summary/config. Pinned-source compilation passed; owned-kind execution remains
pending; no terminal-emulator, refresh navigation, controller reconciliation,
real ConfigHub service, or paid model evaluation is claimed.

The first owned-kind attempt is retained at `/tmp/scout757-owned-kind-proof-1`.
Six CLI/MCP observations passed; the TUI probe failed because Home is not a
viewport binding. Its private admin config hash was also incorrectly checked
after kind deletion edited that setup config. Both are harness failures; the
receipt remains failed. Cluster/node/private-directory removal and shared
config integrity passed. The repair uses PgUp and tests the same viewport
actions offline, and seals upstream read integrity before kind cleanup.

Attempt 2 is retained under ignored
`evals/results/gitops-status-context-20261002/attempt-2/`. Its actual TUI
viewport actions passed, but the helper expected an unpadded Markdown code
span while production renders a padded span. Shared and upstream read config
integrity and all owned cleanup passed. The repaired validator is now tested
against an actual offline production viewport artifact and the retained live
artifact; neither changes the failed attempt's acceptance. Independent review
admits a new bounded run. Product context/runtime/MCP review found no blocker.
