# Doctor and scan context proof (#743)

Prepared, opt-in, serial before/after CLI and fixed-source TUI proof on one uniquely named disposable
kind cluster. **The first live attempt failed a validator expectation; no complete
accepted proof exists yet.** The retained [failure report](../reports/2026-10-01-doctor-scan-context.json)
records successful cleanup and unchanged shared/private configs. The validator
expected a raw service-account name in scan warnings, while the pinned scanner
projects typed Forbidden errors as scoped `Access denied` warnings. Separate
offline revalidation matches the corrected exact warning contract and preserves
the original failed receipt. The prepared live TUI probe is still required before acceptance.

Old source: `98fe0183a932e32be3cbc9c04aae8f1d7124740d`.
Reviewed fixed product source: `5b3629752f354b257f7e667aa2c4f0126b35562b`.
The local build and full Go suite passed at that revision before pinning it.
The capture checkout must be clean and contain that source as an ancestor.

## What this lane proves

Two private contexts use the same owned cluster: an administrator and a short-lived
service-account token with no read grants. The ambient context stays allowed;
explicit denied requests must not fall back to it. The isolated namespace contains
a Deployment and ConfigMap. The Deployment has zero replicas, so no application
image is pulled. The helper waits for the namespace controller's `kube-root-ca.crt` ConfigMap
and records all three exact Deployment/ConfigMap identities and UIDs. Doctor must
report that directly verified inventory count for the allowed context, and zero observed resources with exact denial evidence for the
denied context. Fixed Doctor and scan JSON must name the selected context explicitly.
Doctor retains the raw denied principal; scan retains the exact scoped warning
multiset produced by `formatScanWarning`. An unchanged private-config hash binds
the paired observations to the same context configuration. Arbitrary warning
strings do not satisfy the denial control.

Scan must return structured state results or an explicit denial from the selected
service account. This deliberately does **not** prove nonempty scan findings or
cross-cluster identity: the fake-server/provider tests supply those proofs. An empty
allowed state scan alone is not endpoint-routing proof. Old binaries must reject the
unsupported selector; this is a CLI before/after demonstration, not a test-revert
proof for every nested-reader change.

The fixed-source Go probe drives the actual model's `S`, close and `S` events
against the owned API. It records GET paths/statuses and rendered views. After the
first allowed scan it changes only a separate private config copy to select denied
credentials; the captured model must still read Pods successfully. A newly bound
denied model must receive HTTP 403 and render missing coverage with its context.
Both configs are removed after the attempt. This is model-event and rendering
proof against a live API, not terminal-emulator or full interactive navigation proof.
The helper compiles the probe before cluster creation and records its template hash.

Context names remain selection labels, not stable cluster IDs. This lane does not
prove ConfigHub joins, fleet behavior, Commander integration or model-cost savings.

## Isolation and evidence

- Existing kind `v0.31.0` and cached digest-pinned Kubernetes `v1.35.0` node image;
  existing git, Go, kubectl and a local Unix-socket Docker engine. No installs or
  downloads. The helper records executable hashes and rejects a remote engine.
- Compile each pinned source with downloads disabled and an explicit private empty
  kubeconfig before creating the cluster. Scout uses the legacy scan provider; a
  local `cub` shim prevents ConfigHub access. No model or paid evaluation runs.
- Shared kubeconfig is read only for an integrity hash. All fixture writes, context
  changes and cleanup target the newly owned cluster/private kubeconfig.
- Work is bounded by a 600-second overall deadline, individual
  command deadlines and a separate 120-second cleanup budget. Command process
  groups belong to this invocation; output is bounded to 2 MiB per command.
- Retain command argv, exit/failure status and partial stdout/stderr even on timeout
  or output overflow. Credential operations redact argv/output from the receipt; the token is written
  directly into the private config and never passed in process arguments.
  Private credential files are removed after cleanup; other temporary proof files
  remain available. Check `cleanupErrors` before treating cleanup as complete.
- Delete only after successful creation, then check that no owned node containers
  remain. If creation fails, retain exact-name container inventory and flag any
  possible partial resources for ownership review before removal; an intent marker
  alone never authorizes deletion. Hash the private config around observation reads and the
  shared config around the entire attempt. Any integrity or cleanup failure makes
  acceptance fail. Failures remain evidence, never successful observations.

## Offline guard validation

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s evals/doctor-scan-context -p test_capture.py -v
```

These tests run only synthetic local Python children and in-memory observation
fixtures. They cover command failure retention, timeout/output limits, zero execution
after deadline, credential redaction, missing executables, complete structured CLI
controls, wrong context/count, false-empty denial, unrelated warnings, timeout
masquerading as denial, missing/duplicate observations, and mocked setup/create/cleanup
failures, plus TUI request/status/rendering acceptance controls. The lifecycle tests replace every external command; they are not live proof.

## Live invocation after review

Review the finalized source pin, helper, fixture acceptance and cleanup paths before
running under the adopted owned-cluster authorization. No fresh user approval is
required for the already authorized packet; an independent code/evidence review is
still required. The next live step is pending that review, not user permission.

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/doctor-scan-context/capture.py \
  --execute \
  --integrity-only-shared-kubeconfig /explicit/path/to/kubeconfig-to-hash-only \
  --output-dir /tmp/scout-doctor-scan-context-proof
```

Use a fresh output directory for every attempt. Inspect the retained receipt and
cleanup status on failure before retrying; do not overwrite failed evidence.
