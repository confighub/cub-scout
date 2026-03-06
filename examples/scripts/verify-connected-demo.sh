#!/usr/bin/env bash
# verify-connected-demo.sh - Fail-fast readiness check for connected demo environments.
#
# Checks:
#   1) At least one ready/connected worker in the given space
#   2) At least one Kubernetes target in the given space
#   3) At least one renderer target (argocdrenderer/fluxrenderer, or generic renderer)
#   4) Connected workload count from `cub-scout import --dry-run --json`
#      (labels + existing unit link-back in the target App Space)
#
# Usage:
#   examples/scripts/verify-connected-demo.sh --space argo-import-demo --renderer argocdrenderer
#   examples/scripts/verify-connected-demo.sh --space flux-import-demo --renderer fluxrenderer

set -euo pipefail

SPACE=""
RENDERER_TOKEN=""
MIN_CONNECTED=1

usage() {
    cat <<USAGE
Usage: $(basename "$0") --space <slug> [--renderer <token>] [--min-connected <n>]

Options:
  --space <slug>         ConfigHub space slug to validate (required)
  --renderer <token>     Renderer token to require (e.g., argocdrenderer, fluxrenderer)
                         If omitted, any target containing "renderer" satisfies the check.
  --min-connected <n>    Minimum connected workloads required from cub-scout dry-run (default: 1)
  -h, --help             Show this help
USAGE
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --space)
            [[ $# -ge 2 ]] || { echo "missing value for --space" >&2; usage; exit 2; }
            SPACE="$2"
            shift 2
            ;;
        --space=*)
            SPACE="${1#*=}"
            shift
            ;;
        --renderer)
            [[ $# -ge 2 ]] || { echo "missing value for --renderer" >&2; usage; exit 2; }
            RENDERER_TOKEN="$2"
            shift 2
            ;;
        --renderer=*)
            RENDERER_TOKEN="${1#*=}"
            shift
            ;;
        --min-connected)
            [[ $# -ge 2 ]] || { echo "missing value for --min-connected" >&2; usage; exit 2; }
            MIN_CONNECTED="$2"
            shift 2
            ;;
        --min-connected=*)
            MIN_CONNECTED="${1#*=}"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "unknown argument: $1" >&2
            usage
            exit 2
            ;;
    esac
done

if [[ -z "$SPACE" ]]; then
    echo "--space is required" >&2
    usage
    exit 2
fi

if ! [[ "$MIN_CONNECTED" =~ ^[0-9]+$ ]]; then
    echo "--min-connected must be a non-negative integer" >&2
    exit 2
fi

if ! command -v cub >/dev/null 2>&1; then
    echo "FAIL connected demo readiness"
    echo "- cub CLI not found in PATH"
    exit 1
fi

if [[ -n "${CUB_SCOUT_BIN:-}" ]]; then
    CUB_SCOUT="$CUB_SCOUT_BIN"
elif [[ -x "./cub-scout" ]]; then
    CUB_SCOUT="./cub-scout"
elif command -v cub-scout >/dev/null 2>&1; then
    CUB_SCOUT="$(command -v cub-scout)"
else
    echo "FAIL connected demo readiness"
    echo "- cub-scout binary not found (set CUB_SCOUT_BIN or build ./cub-scout)"
    exit 1
fi

workers_json=""
if ! workers_json=$(cub worker list --space "$SPACE" --json 2>/dev/null); then
    echo "FAIL connected demo readiness"
    echo "- failed to list workers for space '$SPACE'"
    exit 1
fi

targets_json=""
if ! targets_json=$(cub target list --space "$SPACE" --json 2>/dev/null); then
    echo "FAIL connected demo readiness"
    echo "- failed to list targets for space '$SPACE'"
    exit 1
fi

import_json=""
if ! import_json=$("$CUB_SCOUT" import --dry-run --json 2>/dev/null); then
    echo "FAIL connected demo readiness"
    echo "- failed to run '$CUB_SCOUT import --dry-run --json'"
    exit 1
fi

ready_workers=$(python3 - "$workers_json" <<'PY'
import json, sys
raw = (sys.argv[1] if len(sys.argv) > 1 else "").strip()
if not raw:
    print(-1)
    raise SystemExit(0)
try:
    data = json.loads(raw)
except Exception:
    print(-1)
    raise SystemExit(0)
if isinstance(data, dict):
    data = [data]
if not isinstance(data, list):
    print(-1)
    raise SystemExit(0)
count = 0
for item in data:
    if not isinstance(item, dict):
        continue
    nested = item.get("BridgeWorker")
    condition = ""
    if isinstance(nested, dict):
        condition = nested.get("Condition") or nested.get("condition") or ""
    if not condition:
        condition = item.get("Condition") or item.get("condition") or item.get("Status") or item.get("status") or ""
    if str(condition).strip().lower() in {"ready", "connected"}:
        count += 1
print(count)
PY
)

if [[ "$ready_workers" -lt 0 ]]; then
    echo "FAIL connected demo readiness"
    echo "- could not parse worker list JSON"
    exit 1
fi

read -r kubernetes_targets renderer_targets < <(python3 - "$targets_json" "$RENDERER_TOKEN" <<'PY'
import json, sys
raw = (sys.argv[1] if len(sys.argv) > 1 else "").strip()
renderer = (sys.argv[2] if len(sys.argv) > 2 else "").strip().lower()
if not raw:
    print("-1 -1")
    raise SystemExit(0)
try:
    data = json.loads(raw)
except Exception:
    print("-1 -1")
    raise SystemExit(0)
if isinstance(data, dict):
    data = [data]
if not isinstance(data, list):
    print("-1 -1")
    raise SystemExit(0)
k8s = 0
renderer_count = 0
for item in data:
    if not isinstance(item, dict):
        continue
    target = item.get("Target") if isinstance(item.get("Target"), dict) else {}
    slug = str(target.get("Slug") or item.get("Slug") or item.get("slug") or "")
    provider = str(target.get("ProviderType") or item.get("ProviderType") or item.get("providerType") or "")
    toolchain = str(target.get("ToolchainType") or item.get("ToolchainType") or item.get("toolchainType") or "")
    hay = f"{slug} {provider} {toolchain}".lower()
    if "kubernetes" in hay or "k8s" in hay:
        k8s += 1
    if renderer:
        if renderer in hay:
            renderer_count += 1
    else:
        if "renderer" in hay:
            renderer_count += 1
print(f"{k8s} {renderer_count}")
PY
)

if [[ "$kubernetes_targets" -lt 0 || "$renderer_targets" -lt 0 ]]; then
    echo "FAIL connected demo readiness"
    echo "- could not parse target list JSON"
    exit 1
fi

connected_workloads=$(python3 - "$import_json" <<'PY'
import json, sys
raw = (sys.argv[1] if len(sys.argv) > 1 else "").strip()
if not raw:
    print(-1)
    raise SystemExit(0)
try:
    data = json.loads(raw)
except Exception:
    print(-1)
    raise SystemExit(0)
if not isinstance(data, dict):
    print(-1)
    raise SystemExit(0)
workloads = data.get("workloads")
if not isinstance(workloads, list):
    print(-1)
    raise SystemExit(0)
count = 0
for w in workloads:
    if not isinstance(w, dict):
        continue
    value = w.get("connected")
    if value is True:
        count += 1
        continue
    if isinstance(value, str) and value.strip().lower() in {"true", "1", "yes"}:
        count += 1
print(count)
PY
)

if [[ "$connected_workloads" -lt 0 ]]; then
    echo "FAIL connected demo readiness"
    echo "- could not parse cub-scout import dry-run JSON"
    exit 1
fi

failures=()
if [[ "$ready_workers" -lt 1 ]]; then
    failures+=("no ready workers found in space '$SPACE'")
fi
if [[ "$kubernetes_targets" -lt 1 ]]; then
    failures+=("no Kubernetes target found in space '$SPACE'")
fi
if [[ "$renderer_targets" -lt 1 ]]; then
    if [[ -n "$RENDERER_TOKEN" ]]; then
        failures+=("no renderer target matching '$RENDERER_TOKEN' found in space '$SPACE'")
    else
        failures+=("no renderer target found in space '$SPACE'")
    fi
fi
if [[ "$connected_workloads" -lt "$MIN_CONNECTED" ]]; then
    failures+=("connected workloads below threshold ($connected_workloads < $MIN_CONNECTED)")
fi

if [[ ${#failures[@]} -gt 0 ]]; then
    echo "FAIL connected demo readiness"
    for msg in "${failures[@]}"; do
        echo "- $msg"
    done
    echo "workers_ready=$ready_workers targets_kubernetes=$kubernetes_targets targets_renderer=$renderer_targets connected_workloads=$connected_workloads"
    exit 1
fi

echo "PASS connected demo readiness"
echo "workers_ready=$ready_workers targets_kubernetes=$kubernetes_targets targets_renderer=$renderer_targets connected_workloads=$connected_workloads"
