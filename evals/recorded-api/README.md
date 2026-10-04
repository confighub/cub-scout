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
