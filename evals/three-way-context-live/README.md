# Three-way comparison context proof preparation (#755)

One serial owned-kind capture is admitted after product/helper review, offline
checks and local cached-tool preflight. The immutable product pin is
`478b1095359c6325d38e89c02e837861fc0f4410`. Execution still requires `--execute`,
an exact committed helper source and complete tool SHA pins. Pending or invalid
source pins refuse before tools or configuration reads. No live acceptance yet.

The helper shares the reviewed doctor command/evidence primitives, Trace
process-group runner, ownership cleanup and private observer environment, and
Trace's bounded read-only API proxy. Sibling helpers are unchanged. The proxy
subclass adds exact query evidence and narrows routes; its upstream is the same
invocation-owned HTTPS loopback kind API for all three contexts. Distinct proxy
bindings prove selected-context routing, **not physically different clusters**.

The minimal fixture is namespace `scout-three-way-context-proof`, a zero-replica
Deployment `scout-context-marker`, and an inert Argo Application
`scout-context-app`. The Deployment carries its actual Argo identity and
ConfigHub `scout-proof-unit` / `scout-proof-space` labels. The Application records
`https://example.invalid/owned-proof.git`, revision `proof-revision`, path
`proof/manifests`, and synthetic Healthy/Synced status. No workload image is
pulled and no real Argo reconciliation is claimed. Only the Application CRD is
created; storage initialization uses bounded setup-only GET retries.

Generated private ServiceAccount credentials implement actual Kubernetes RBAC:
source-denied cannot LIST/GET Applications; pods-denied cannot LIST Pods. Allowed
uses the owned-kind admin identity. Observer kubeconfigs contain all bindings,
no credentials, immutable private files and a different ambient context from the
explicit selection. Shared kubeconfig bytes are read for integrity only.

## Frozen resource observation contract

Each positive CLI/MCP collection performs these exact selected-endpoint GETs:

1. Namespaced Deployment GET.
2. Cluster-wide Application LIST with only
   `fieldSelector=metadata.name=scout-context-app`.
3. Namespaced Application GET, omitted after source-denied LIST returns 403.
4. Namespaced Deployment GET for current-change evidence.
5. Namespaced Pods LIST with only `labelSelector=app=scout-context-marker`,
   returning actual 403 for pods-denied.

There are no discovery reads, arbitrary queries or mutation routes. Request
order, count, selected endpoint, query, disposition and exact statuses must
match. The raw API phase log must equal the receipt's traffic. The old product
pin `94e6edf339e0cd410994563fb7a7c57c52e18c09` receives an extra MCP `context`
argument and must actually read the other ambient proxy, with zero selected
requests. Its unchanged gateway ignores that argument; a flag-refusal imitation
would fail this behavioral control.

The private `cub` shell shim permits only `auth status` (synthetic exit 0) and
`unit get scout-proof-unit -o json --quiet --space scout-proof-space`
(deliberate failure 73). Every other argv is rejected and redacted. Exact
sequences are CLI: auth, unit; MCP/old: auth, auth, auth, unit; TUI: auth, unit
three times. MCP's three auth calls are gateway creation, tools/call session
refresh, and its separate CLI subprocess. Each visible TUI request refreshes
its connection cache once. These are bounded synthetic calls, with no real
ConfigHub auth/server/governance evidence.

Reports must retain the exact Deployment LIVE identity, labels, zero replicas,
current-change identity, usable Git-source anchor on allowed and pods-denied,
structured forbidden omissions on the denied phase, and unavailable DRY/WET
sides. Summary agreement must remain `partial`; missing/denied evidence cannot
certify convergence. MCP text and structured JSON must agree.

The Go probe starts the actual session collector, accepts its actual result into
the model, resizes, scrolls down/back, performs a real second observation through
`r`, and submits the prefilled identical scope through the actual edit route.
Esc must produce `tea.Quit`. Snapshot immutability is asserted during viewport
actions, not across real refresh. Versioned artifacts retain the initial,
refreshed and edited reports, full rendered content, view and action checks.
This exercises model navigation; it does not claim terminal UX.

## Offline controls and verification

Run the Python controls without cluster/tool discovery:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/three-way-context-live/test_capture_live.py
```

Compile the probe using an overlay, so no product source file is edited. Choose
one fresh `/tmp` directory; the example below also runs **only offline** viewport
controls and feeds their actual Go-rendered artifacts into the Python validator.
`TestThreeWayOwnedTUI` is the live probe and is not run by these commands.

```sh
SCOUT755_OFFLINE=$(mktemp -d /tmp/scout755-offline.XXXXXX)
export SCOUT755_OFFLINE
python3 - <<'PY'
import json, os
from pathlib import Path
repo, out = Path.cwd(), Path(os.environ['SCOUT755_OFFLINE'])
(out / 'overlay.json').write_text(json.dumps({'Replace': {
    str(repo / 'cmd/cub-scout/three_way_context_live_test.go'):
    str(repo / 'evals/three-way-context-live/three_way_tui_live_test.go.txt')}}))
(out / 'viewport-artifacts').mkdir()
PY
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -c \
  -overlay "$SCOUT755_OFFLINE/overlay.json" \
  -o "$SCOUT755_OFFLINE/three-way-tui.test" ./cmd/cub-scout
SCOUT_THREE_WAY_VIEWPORT_RESULTS="$SCOUT755_OFFLINE/viewport-artifacts" \
  "$SCOUT755_OFFLINE/three-way-tui.test" \
  -test.run '^TestThreeWayOwnedViewportControls$' -test.count=1 -test.v
SCOUT_THREE_WAY_VIEWPORT_RESULTS="$SCOUT755_OFFLINE/viewport-artifacts" \
  PYTHONDONTWRITEBYTECODE=1 python3 evals/three-way-context-live/test_capture_live.py
```

Controls cover refusal before config/tool reads, strict pins, exact argv/query
contracts, private HOME/XDG/PATH, real rejected proxy requests, wrong endpoints,
unexpected statuses/dispositions, wrong labels, hidden denial, fabricated sides,
false convergence, old ambient behavior, actual model actions and retained
failure/cleanup/integrity handling. Lifecycle controls reuse reviewed ownership
rules, including uncertain creation retention rather than unreviewed deletion.

## Admission requirements and retained evidence

Root must select a full product commit, review this helper/probe and commit a
clean helper checkout. `--helper-source` must equal its exact HEAD; product and
old pins must be ancestors as checked. Existing git/go/kind/kubectl/docker
binaries require one reviewed `--tool-pin NAME=SHA256` each; SHA checks precede
all tool execution. kind must be v0.31.0 and the shared immutable node-image
pin must already be cached. `DOCKER_HOST` is refused; the selected Docker
endpoint must be local unix. Builds use cached Go tooling with
`GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`; no installs or downloads are admitted.

After a separately reviewed admission change to `main`, execution still
requires explicit `--execute`, an integrity-only regular shared kubeconfig and
a fresh private output directory under `/tmp` or `/var/tmp`. One global 600s
command deadline reserves 240s for cleanup. The worktree cleanup deadline
adapter preserves reviewed removal rules while bounding its real commands.
Proxies drain before the final snapshot/log hashes; source, upstream and
observer config integrity is sealed before intentional kind cleanup. Only this
invocation's cluster/nodes, temporary worktrees and private credentials are
removed. Uncertain nodes require ownership review. Public receipts, raw logs,
TUI artifacts and hashes remain; failures and cleanup errors stay failed.

No live acceptance, genuine controller/server/governance, multi-cluster identity,
paid model evaluation, release, terminal UX or cost-savings claim is established
by this prepared packet.
