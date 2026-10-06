# Recorded Kubernetes API evidence helpers

## RUL-03 exact context response reader

`context_frames.py` exposes `select_frame(context, method, path)` for the
existing [RUL-03 case](../rul03-context/README.md). All three selectors are
mandatory and exact. It returns the byte-preserved response and its captured
HTTP status, endpoint/CA identity, source row and response times. A request for
`rul03-denied` returns the retained 403 even though the observer's recorded
current context is `rul03-readable`. There is no current-context default or
fallback. Unknown contexts, methods, paths and query variants refuse.

The helper pins the complete scope/context map and selected response body,
checks bounded regular files, and rejects symlinks, missing files and changed
bytes. It never reads the unselected response body. The two observations remain
historical and non-atomic; no present inventory, empty-list or ownership claim
is generated from a denied response. This pure reader opens no listener and
launches no executable. Its five deterministic controls run with the existing
offline test command below and in unit-only CI.

This prepares source selection for a future adapter. RUL-03 remains blocked in
the full-24 launch policy: the reader is not an MCP tool, runtime enforcement,
model execution or benchmark admission. Frozen case bytes and tool grants are
unchanged.

### Prepared MCP evidence transport

`context_mcp.py` wraps the reader in an eval-only newline-delimited JSON-RPC
stdio server. Its sole read-only tool, `recorded_response`, requires exact
`context`, `method` and `path` arguments. It returns `responseText` unchanged
alongside capture provenance. Retrieving a recorded 403 succeeds as an evidence
read (`isError: false`); its provenance still says `httpStatus: 403`. An
unsupported request is a tool error and never substitutes another context.
This tool does not present a denied HTTP response as a Kubernetes object or
imitate the product's `map` and `explain` contracts.

The command-line entry point accepts no overrides and reads only the fixed
staged `/evidence/cluster` files. Both contexts and metadata must pass source
pins before initialization publishes any tools. The imported test interface
accepts a fixture directory for offline validation. Input messages are limited
to 128 KiB, nesting to 32 levels, integers to 128 digits and each invocation to
64 messages. Duplicate keys, non-finite values (including exponent overflow),
malformed requests and unknown tool arguments refuse. No listener, executable,
live client or oracle is used.

Five transport controls extend the suite to 25 tests, exercising initialization,
catalog, exact denied/readable responses, strict input bounds and startup
refusal through byte streams. These are offline MCP protocol checks; they do
not establish official client interoperability or isolated runtime enforcement.
The new tool is not granted by the frozen full-24 launch policy. Its source
mounts, runtime and tool admission still need review before that policy can use
it. RUL-03 remains blocked; no prompts, grants, budgets or source evidence have
changed.

### Recorded inventory binding (offline, eval-only)

`context_inventory.py` adds `bind_inventory(context, method, path, map_reader)`.
It selects one exact pinned response before calling the injected recorded Scout
map reader. The reader receives only original response bytes and exact
`apps/v1 Deployment` scope in `rul03-proof`; it returns full recorded-map JSON.
The binding checks schema, source hash/size/counts, supplied-type count, exact
scope/resource identities and owner histogram before retaining that projection.
Ownership remains product output; this helper does not independently re-detect
it or establish the reader's binary/runtime admission.

A captured 403 yields `coverage: "unreadable"` and `inventory: null`. It never
calls the reader or loads the readable context's response, even when that
context was the observer's default. Successful historical evidence yields
`coverage: "recorded-response"`, never current state or complete cluster
coverage. Both retain exact capture endpoint, request, row, status and source
provenance, with `currentStateEstablished` and `runtimeAdmission` false. There
is no cross-context join, retry, default or fallback.

Success controls use the original [RUL-03 recording](../rul03-context/README.md):
exact readable bytes/scope/provenance; denied callback suppression with the
other body absent; foreign hash/scope/identity/count refusal; changed or missing
source refusal before the reader; malformed/duplicate/non-finite/oversized/deep
report refusal; and reader failure without retry or private error leakage.
Seven deterministic tests extend this directory's suite to 31 tests, automatically
included by its existing unit-only CI step. No product CLI/MCP/TUI surface,
MCP transport source pin, frozen prompt, evidence or grant changes.

The retained actual host CLI/binding smoke at
`/private/tmp/scout-v213-context-inventory-host-proof-20261004` used the local
`./cub-scout`, a private empty HOME/kubeconfig and selected original response.
It validates only host recorded CLI composition: one readable invocation and
zero denied invocations. It establishes no Linux runtime, MCP/model tool
admission, live test, containment or savings. RUL-03 remains blocked in full24.

### Prepared exact-context map protocol

`context_inventory_mcp.py` adds a separate eval-only bounded stdio protocol
adapter. Its sole read-only `map` tool requires exact `context`; GET/path and
namespaced Deployment scope are fixed by the selected capture. The catalog does
not expose raw responses, explain, doctor or live tools. Unknown contexts,
extra method/path/scope arguments, malformed initialization/envelopes and
notifications cannot execute the reader. The adapter checks the entire outgoing
JSON-RPC envelope against the byte bound and returns a fixed tool error when
source/reader/report validation fails, without echoing private inputs.

Startup validates both original pinned contexts before publishing a catalog.
Each subsequent call binds exactly one request. A captured denial is a successful
historical evidence read (`isError: false`) with HTTP 403, unreadable coverage and
null inventory; it never runs the inventory reader. The readable call retains
validated product map output and exact selected capture provenance. No source
files or contexts are combined into a new snapshot.

Six pure protocol controls bring this directory's suite to 37 tests. The actual
host stdio/CLI composition at
`/private/tmp/scout-v213-context-map-mcp-host-20261004` checks initialization,
map-only catalog, denied/readable selection, implicit-context refusal and
unsupported doctor/explain refusal. It records seven replies and one readable
local `./cub-scout` CLI call, with private empty HOME/kubeconfig, reviewed package
hashes and interpreter/binary hashes checked before/after. Source base revision
and modified-tree state are separate from exact adapter hashes. This is a host
composition check, not filesystem/network containment or trusted runtime mounts.

The module accepts an injected reader through its Python API; it has no executable
entrypoint, arbitrary binary flag or live fallback. Using the name `map` does not
by itself admit this eval adapter under a frozen model tool grant. Original
`context_frames.py`/`context_mcp.py` source pins, source questions/evidence/budgets,
launch-policy candidates and ordinary grants remain unchanged. Linux Python
bundle/mount admission, an enforced reader, exact model-tool wiring and complete
process accounting remain open; RUL-03 is still blocked. The product's
CLI/MCP/TUI surfaces remain unchanged.

## PRE-01 recorded API replay

This helper serves a small, fixed set of byte-pinned PRE-01 API responses on an
owned IPv4 loopback listener. It reads only
`evals/reports/2026-10-01-pre01-crd.json` and its listed fixture bytes. It does
not contact a cluster or upstream service, load kubeconfig, consume
credentials, invoke kubectl/Helm, or use a container/model/provider. The local
plain-HTTP transport is plumbing for a future network-none container gate; it
does not claim Kubernetes TLS or API-server equivalence.

The helper requires an explicit `--phase absent|present`. The absent replay
collapses repeated source observations only when the exact method/path/status
and response bytes agree. The final-present replay intentionally selects only
the final `present` observations; the earlier `present-before-apply` 404 is
excluded because the same path later has a captured 200 and a static replay
cannot represent both times. Phases never advance automatically. All routes
retain the original source row indexes, phases, and timestamps. The source
capture was sequential and non-atomic, not a current snapshot.

Only an exact GET method and raw path/query returns a captured body and status.
Other methods receive 405; unsupported paths and query variants receive an
explicit 503 replay-unavailable response, never a fabricated Kubernetes 404 or
empty discovery/list. Known captured 404s are returned byte-for-byte, including
the plain-text unregistered-route bodies. `Content-Type` and `Content-Length`
are generated for local transport and are not historical response-header
claims. Incoming headers (including authorization) and request bodies are
never written to the trace. Unknown request targets are represented by hash
and byte length rather than persisted as raw text.

Source report/scope hashes and the source revision are fixed in `replay.py`;
each selected fixture's exact size and SHA-256 is rechecked before the socket
opens. The listener binds only `127.0.0.1`, has no forwarding path, and closes
connections. Request-line/header/body/response sizes, request count, per-read
timeout and total wall time are bounded. `ready.json` and `trace.json` are
created mode 0600 in a new mode 0700 temporary output directory; existing or
symlinked outputs are rejected. The trace distinguishes generated transport
metadata from source observations and records listener closure. It deliberately
sets `coverageComplete: false`: this packet does not declare an external
kubectl command's expected route set, so a clean trace alone is not proof of
complete command coverage.

## Offline tests

The suite uses only Python's standard library and the local loopback listener:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/recorded-api -v
```

It checks source/report/body pins, phase selection, exact 200/404 bytes,
unavailable route behavior, denied methods and query variants, malformed and
oversized requests, read timeout, concurrent request bounds, private output
guards, and owned listener shutdown. It does not launch external executables.

## Later real kubectl gate (not run or enabled here)

The first declared command gate should use the real, reviewed Linux kubectl
binary and one phase at a time, with a fresh private empty `HOME`, owned
kubeconfig, and no inherited user config or credentials. Start this server in a
private temporary output directory, read the actual port from `ready.json`,
and direct `kubectl get --raw` only to the three exact PRE-01 paths documented
in that phase's ready file. The invocation shape is:

```sh
python3 evals/recorded-api/replay.py --phase absent \
  --out-dir /tmp/pre01-api-replay-absent --max-requests 8 --wall-seconds 60

# In the later, separately reviewed gate, configure only this owned local API
# endpoint in a fresh kubeconfig with an anonymous user and an empty owned HOME.
mkdir -m 700 /tmp/pre01-api-replay-home
cat > /tmp/pre01-api-replay.kubeconfig <<'EOF'
apiVersion: v1
kind: Config
clusters:
- name: recorded
  cluster:
    server: http://127.0.0.1:PORT_FROM_READY_JSON
contexts:
- name: recorded
  context:
    cluster: recorded
    user: anonymous
current-context: recorded
users:
- name: anonymous
  user: {}
EOF
env -i PATH=/reviewed/linux/bin HOME=/tmp/pre01-api-replay-home \
  KUBECONFIG=/tmp/pre01-api-replay.kubeconfig \
  /reviewed/linux/bin/kubectl get --raw \
  /apis/apiextensions.k8s.io/v1/customresourcedefinitions/servicemonitors.monitoring.coreos.com
```

Repeat `--raw` only for the exact route paths in `ready.json`; the example's
paths and paths under other phases are not interchangeable. Stop only the
owned replay process after the commands, then compare every trace request
against the selected capture rows and verify the raw output bytes/status.
Reject any unrecorded request, missing response, timeout, or cleanup
uncertainty. A captured 404 can produce a nonzero kubectl exit; that is the
captured HTTP status, not a replay claim that an unrecorded object is absent.
This gate must use the separately acquired and hash/signature-reviewed Linux
kubectl asset. It is not a live-cluster request and does not establish ordinary
tool parity for other frozen questions.

Helm is not enabled: these fixtures contain no Helm release storage or
source-captured Helm API route corpus. A later Helm replay needs its own exact
source evidence. This work makes no full Experiment A, model-answer, billing,
savings, or paid-run claim. The benchmark manifest, cases, graders and spend
ledger remain unchanged.

## PRE-03 declared child reference

`argo_child_reference.py` projects three pinned historical files from the
[existing PRE-03 example](../pre03-argo-child-failure/). The parent status declares
an exact target group/version/kind/namespace/name, while the child capture's
namespace/name and tracking ID match that reference. Their UIDs remain distinct;
no UID foreign key is supplied. Parent/child sync and health reports are separate.
Reported resources are bounded to 32 rows, text to 512 UTF-8 bytes, and output to
64 KiB; omitted counts expose bounded rows. Missing/drifted/malformed/ambiguous
source or tracking identity refuses without a fallback.

Both captures omit top-level API version/kind. `observedGVK` remains `unknown`;
`declaredTarget` must never be installed as observed child type fields. Capture
time/current state, atomicity and a full Kubernetes snapshot remain unestablished.
No Pod/tree join, workload-health decision or grader answer is manufactured.

```bash
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/recorded-api -p test_argo_child_reference.py -v
```

Seven deterministic controls cover the genuine source, missing/drifted inputs,
wrong/ambiguous references, tracking/UID/type overlays, malformed/oversized fields,
and ordering/bounds. An actual isolated host invocation against a read-only copy
of the original modules/three captures has [source and stream proof](local-argo-reference-proof.json)
retained at `/private/tmp/scout-v213-pre03-reference-host-final-20261004`.
This helper has no executable or MCP entrypoint. It prepares typed captured-field
reasoning; PRE-03 product recorded-object binding and frozen tool/runtime
admission remain blocked. Existing policy stays eleven candidates/thirteen
blocked; frozen prompts, bytes, grants and budgets remain unchanged.
