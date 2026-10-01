# Doctor and scan context proof (#743)

Prepared, opt-in, serial before/after CLI proof on one uniquely named disposable
kind cluster. **The live lane has not been run.** The fixed source pin is still
a placeholder, so execution currently fails before creating files or running tools.

Old source: `98fe0183a932e32be3cbc9c04aae8f1d7124740d`. Set the fixed source to the
reviewed implementation commit only after product review and offline tests pass.
The capture checkout must be clean and contain that source as an ancestor.

## What this lane proves

Two private contexts use the same owned cluster: an administrator and a short-lived
service-account token with no read grants. The ambient context stays allowed;
explicit denied requests must not fall back to it. The isolated namespace contains
a Deployment and ConfigMap. The Deployment has zero replicas, so no application
image is pulled. Doctor must report the fixture's two inventory resources for the
allowed context, and zero observed resources with exact denial evidence for the
denied context. Fixed Doctor JSON must name the selected context explicitly.

Scan must return structured state results or an explicit denial from the selected
service account. This deliberately does **not** prove nonempty scan findings or
cross-cluster identity: the fake-server/provider tests supply those proofs. An empty
allowed state scan alone is not endpoint-routing proof. Old binaries must reject the
unsupported selector; this is a CLI before/after demonstration, not a test-revert
proof for every nested-reader change.

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
failures. The lifecycle tests replace every external command; they are not live proof.

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
