# GitOps status context interpretation case

This opt-in case contains synthetic, fixture-backed summaries for two
independent Kubernetes contexts, including an explicit Modelplane API denial.
It tests whether an agent reports the context label and partial coverage
without promoting either to stable cluster identity or complete health.

**Model execution has not run.** These records do not come from a live cluster
or ConfigHub service and establish no real RBAC or connected-mode behavior.
The scaffold only copies the two checked-in JSON files into a temporary
`recorded/` directory.

## Accepted owned-kind proof (#753)

[The derived report](live-proof-report.json) records accepted attempt 6 on
2026-10-02. The product source is pinned to
`00e1375e0380ccf809ee4ef6f22abd47ca3119d1`; the helper/probe source is
`e7757e13d3c67893d358091c35f3e0d6e27438e2`. Source, binary, fixture,
RBAC, request-log, TUI-artifact and raw-receipt hashes are retained.

All nine observations passed: CLI, actual MCP stdio subprocess and actual
summary viewport for allowed, controller-denied and Pod-denied selections.
Each action made 35 exact GETs through its selected proxy, with no traffic
on the other endpoints. The ModelDeployment and Pod denials were actual
upstream 403s on their expected paths. Application controller health remains
visible beside the separate runtime omission. Every observation config
contained all three private bindings and a different ambient current context;
immutable config hashes matched before and after the reads.

The TUI probe drives production viewport resize, PgDown/PgUp offset and view
changes, and q yielding `tea.QuitMsg`. An offline Go control emits actual
production-rendered content for Python validator integration. This establishes
viewport-model actions, not terminal-emulator UX or refresh navigation.

The fixture uses an inert `example.invalid` Argo source and synthetic Argo
Application/ModelDeployment CRDs on a real owned kind API/RBAC. It installs no
Argo or Modelplane controller and proves no reconciliation. Private HOME/XDG
and a PATH containing only the auth shim exclude ambient credentials, kube
settings, proxies, plugin mode and the GitOps fixture hook. The shim accepts
only `cub auth status`, returns synthetic unauthenticated exit 73, and
records exact per-action counts: CLI 1, MCP 3, TUI 1, old-selector control 0.
Unexpected argv is redacted and rejected. This is local stub evidence, not
real ConfigHub authentication, service availability or governance evidence.

The GET-only proxy accepts exact fixture-specific paths and no query
parameters. Non-GETs, discovery, watches, selectors, other namespaces and
arbitrary object names fail closed. Newly created CRD storage readiness is
checked only during bounded setup; observations still require strict HTTP
status and exact denial evidence. Existing pinned tools/image are required,
no installs or downloads are allowed, and the entire capture has a 600-second
bound (kind subprocess 240 seconds, Kubernetes readiness wait 90 seconds).

Accepted cleanup verified owned node absence, source-worktree/private-directory
removal, unchanged shared kubeconfig, unchanged upstream and observation
configs during reads, and no pending API requests or cleanup errors. Raw
artifacts are retained under ignored
`evals/results/gitops-status-context-20261002/attempt-6/`; the checked-in
report preserves their hashes and all five failed attempts:

1. Probe used unbound Home; config integrity was checked after kind cleanup.
2. Probe actions passed, but the helper expected an unpadded Markdown span.
3. Clean-checkout preflight rejected generated review bytecode; no cluster.
4. Kind control-plane startup timed out; uncertain node retained for explicit
   ownership review. A separate `ownership-cleanup.json` verifies exact
   node identity, pre-create absence, removal and shared config integrity.
5. ModelDeployment storage returned HTTP 429 during initialization; Scout
   surfaced a `list_failed` omission and strict validation rejected it.

Every failed receipt remains failed. The repairs, product context/runtime
changes and admission steps were independently reviewed. Eighteen offline
controls pass; run them without creating a cluster:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest evals/gitops-status-context/test_capture_live.py -v
```

No paid model evaluation or measured agent dollar/credit savings is claimed.
Context labels are not stable cluster identities. This proof does not close
#599 or the v2.13 release gates.
