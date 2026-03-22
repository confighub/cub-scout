#!/usr/bin/env bash
# verify-connected-demo.sh - Fail-fast readiness check for connected demo environments.
#
# Checks:
#   1) At least one ready/connected worker in the given space
#   2) At least one ready Kubernetes-backed target in the given space
#   3) At least one ready renderer target (argocdrenderer/fluxrenderer, or generic renderer)
#   4) At least one imported dry unit and one imported wet unit in the given space
#   5) Connected workload count from `cub-scout import --dry-run --json` is
#      reported when meaningful for the proposal App Space
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
  --min-connected <n>    Minimum connected workloads required from cub-scout dry-run
                         when the scout proposal App Space matches --space
                         (default: 1)
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

units_json=""
if ! units_json=$(cub unit list --space "$SPACE" --json 2>/dev/null); then
    echo "FAIL connected demo readiness"
    echo "- failed to list units for space '$SPACE'"
    exit 1
fi

import_json=""
if ! import_json=$("$CUB_SCOUT" import --dry-run --json 2>/dev/null); then
    echo "FAIL connected demo readiness"
    echo "- failed to run '$CUB_SCOUT import --dry-run --json'"
    exit 1
fi

WORKERS_JSON_FILE="$(mktemp)"
TARGETS_JSON_FILE="$(mktemp)"
UNITS_JSON_FILE="$(mktemp)"
IMPORT_JSON_FILE="$(mktemp)"
cleanup_json_files() {
    rm -f "$WORKERS_JSON_FILE" "$TARGETS_JSON_FILE" "$UNITS_JSON_FILE" "$IMPORT_JSON_FILE"
}
trap cleanup_json_files EXIT

printf '%s' "$workers_json" >"$WORKERS_JSON_FILE"
printf '%s' "$targets_json" >"$TARGETS_JSON_FILE"
printf '%s' "$units_json" >"$UNITS_JSON_FILE"
printf '%s' "$import_json" >"$IMPORT_JSON_FILE"

worker_parse="$(python3 - "$WORKERS_JSON_FILE" <<'PY'
import json, sys
path = sys.argv[1]
with open(path, "r", encoding="utf-8") as handle:
    raw = handle.read().strip()
if not raw:
    print("-1")
    raise SystemExit(0)
try:
    data = json.loads(raw)
except Exception:
    print("-1")
    raise SystemExit(0)
if isinstance(data, dict):
    data = [data]
if not isinstance(data, list):
    print("-1")
    raise SystemExit(0)
count = 0
ready_slugs = []
for item in data:
    if not isinstance(item, dict):
        continue
    nested = item.get("BridgeWorker")
    condition = ""
    slug = ""
    if isinstance(nested, dict):
        condition = nested.get("Condition") or nested.get("condition") or ""
        slug = nested.get("Slug") or nested.get("slug") or ""
    if not condition:
        condition = item.get("Condition") or item.get("condition") or item.get("Status") or item.get("status") or ""
    if not slug:
        slug = item.get("Slug") or item.get("slug") or item.get("Name") or item.get("name") or ""
    if str(condition).strip().lower() in {"ready", "connected"}:
        count += 1
        if slug:
            ready_slugs.append(str(slug).strip())
print(f"{count}\t{','.join(ready_slugs)}")
PY
)"
IFS=$'\t' read -r ready_workers ready_worker_slugs <<<"$worker_parse"

if [[ "$ready_workers" -lt 0 ]]; then
    echo "FAIL connected demo readiness"
    echo "- could not parse worker list JSON"
    exit 1
fi

target_parse="$(python3 - "$TARGETS_JSON_FILE" "$RENDERER_TOKEN" "$ready_worker_slugs" <<'PY'
import json, sys
path = sys.argv[1]
renderer = (sys.argv[2] if len(sys.argv) > 2 else "").strip().lower()
ready_workers = {slug for slug in (sys.argv[3] if len(sys.argv) > 3 else "").split(",") if slug}
with open(path, "r", encoding="utf-8") as handle:
    raw = handle.read().strip()
if not raw:
    print("-1 -1 -1 -1")
    raise SystemExit(0)
try:
    data = json.loads(raw)
except Exception:
    print("-1 -1 -1 -1")
    raise SystemExit(0)
if isinstance(data, dict):
    data = [data]
if not isinstance(data, list):
    print("-1 -1 -1 -1")
    raise SystemExit(0)
k8s = 0
renderer_count = 0
k8s_ready = 0
renderer_ready = 0
for item in data:
    if not isinstance(item, dict):
        continue
    target = item.get("Target") if isinstance(item.get("Target"), dict) else {}
    worker = item.get("BridgeWorker") if isinstance(item.get("BridgeWorker"), dict) else {}
    slug = str(target.get("Slug") or item.get("Slug") or item.get("slug") or "")
    provider = str(target.get("ProviderType") or item.get("ProviderType") or item.get("providerType") or "")
    toolchain = str(target.get("ToolchainType") or item.get("ToolchainType") or item.get("toolchainType") or "")
    worker_slug = str(worker.get("Slug") or item.get("BridgeWorkerSlug") or item.get("bridgeWorkerSlug") or "")
    hay = f"{slug} {provider} {toolchain}".lower()
    is_k8s = provider.lower() == "kubernetes" or toolchain.lower() == "kubernetes/yaml"
    if not is_k8s and ("kubernetes" in hay or "k8s" in hay):
        is_k8s = True
    if is_k8s:
        k8s += 1
        if worker_slug:
            if worker_slug in ready_workers:
                k8s_ready += 1
        elif ready_workers:
            k8s_ready += 1
    is_renderer = False
    if renderer:
        if renderer in hay:
            is_renderer = True
    else:
        if "renderer" in hay:
            is_renderer = True
    if is_renderer:
        renderer_count += 1
        if worker_slug:
            if worker_slug in ready_workers:
                renderer_ready += 1
        elif ready_workers:
            renderer_ready += 1
print(f"{k8s} {renderer_count} {k8s_ready} {renderer_ready}")
PY
)"
read -r kubernetes_targets renderer_targets ready_kubernetes_targets ready_renderer_targets <<<"$target_parse"

if [[ "$kubernetes_targets" -lt 0 || "$renderer_targets" -lt 0 || "$ready_kubernetes_targets" -lt 0 || "$ready_renderer_targets" -lt 0 ]]; then
    echo "FAIL connected demo readiness"
    echo "- could not parse target list JSON"
    exit 1
fi

import_parse="$(python3 - "$IMPORT_JSON_FILE" <<'PY'
import json, sys
path = sys.argv[1]
with open(path, "r", encoding="utf-8") as handle:
    raw = handle.read().strip()
if not raw:
    print("-1")
    raise SystemExit(0)
try:
    data = json.loads(raw)
except Exception:
    print("-1")
    raise SystemExit(0)
if not isinstance(data, dict):
    print("-1")
    raise SystemExit(0)
workloads = data.get("workloads")
if not isinstance(workloads, list):
    print("-1")
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
proposal = data.get("proposal") if isinstance(data.get("proposal"), dict) else {}
app_space = str(proposal.get("AppSpace") or proposal.get("appSpace") or "").strip()
print(f"{count}\t{app_space}")
PY
)"
IFS=$'\t' read -r connected_workloads proposal_app_space <<<"$import_parse"

if [[ "$connected_workloads" -lt 0 ]]; then
    echo "FAIL connected demo readiness"
    echo "- could not parse cub-scout import dry-run JSON"
    exit 1
fi

unit_parse="$(python3 - "$UNITS_JSON_FILE" <<'PY'
import json, sys
path = sys.argv[1]
with open(path, "r", encoding="utf-8") as handle:
    raw = handle.read().strip()
if not raw:
    print("-1 -1 -1")
    raise SystemExit(0)
try:
    data = json.loads(raw)
except Exception:
    print("-1 -1 -1")
    raise SystemExit(0)
if isinstance(data, dict):
    data = [data]
if not isinstance(data, list):
    print("-1 -1 -1")
    raise SystemExit(0)
dry = 0
wet = 0
managed = 0
for item in data:
    if not isinstance(item, dict):
        continue
    unit = item.get("Unit") if isinstance(item.get("Unit"), dict) else {}
    slug = str(unit.get("Slug") or item.get("Slug") or item.get("slug") or "").strip()
    if not slug or slug.startswith("discover-"):
        continue
    managed += 1
    if slug.endswith("-dry"):
        dry += 1
    if slug.endswith("-wet"):
        wet += 1
print(f"{dry} {wet} {managed}")
PY
)"
read -r dry_units wet_units managed_units <<<"$unit_parse"

if [[ "$dry_units" -lt 0 || "$wet_units" -lt 0 || "$managed_units" -lt 0 ]]; then
    echo "FAIL connected demo readiness"
    echo "- could not parse cub unit list JSON"
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
if [[ "$ready_kubernetes_targets" -lt 1 ]]; then
    failures+=("no ready Kubernetes-backed target found in space '$SPACE'")
fi
if [[ "$ready_renderer_targets" -lt 1 ]]; then
    if [[ -n "$RENDERER_TOKEN" ]]; then
        failures+=("no ready renderer target matching '$RENDERER_TOKEN' found in space '$SPACE'")
    else
        failures+=("no ready renderer target found in space '$SPACE'")
    fi
fi
if [[ "$dry_units" -lt 1 ]]; then
    failures+=("no imported dry units found in space '$SPACE'")
fi
if [[ "$wet_units" -lt 1 ]]; then
    failures+=("no imported wet units found in space '$SPACE'")
fi

connected_gate="enforced"
if [[ "$MIN_CONNECTED" -gt 0 ]]; then
    if [[ -n "$proposal_app_space" && "$proposal_app_space" != "$SPACE" ]]; then
        connected_gate="skipped"
    elif [[ "$connected_workloads" -lt "$MIN_CONNECTED" ]]; then
        failures+=("connected workloads below threshold ($connected_workloads < $MIN_CONNECTED)")
    fi
else
    connected_gate="disabled"
fi

if [[ ${#failures[@]} -gt 0 ]]; then
    echo "FAIL connected demo readiness"
    for msg in "${failures[@]}"; do
        echo "- $msg"
    done
    echo "workers_ready=$ready_workers targets_kubernetes=$kubernetes_targets ready_targets_kubernetes=$ready_kubernetes_targets targets_renderer=$renderer_targets ready_targets_renderer=$ready_renderer_targets units_dry=$dry_units units_wet=$wet_units units_managed=$managed_units connected_workloads=$connected_workloads connected_gate=$connected_gate proposal_app_space=${proposal_app_space:-none}"
    exit 1
fi

echo "PASS connected demo readiness"
echo "workers_ready=$ready_workers targets_kubernetes=$kubernetes_targets ready_targets_kubernetes=$ready_kubernetes_targets targets_renderer=$renderer_targets ready_targets_renderer=$ready_renderer_targets units_dry=$dry_units units_wet=$wet_units units_managed=$managed_units connected_workloads=$connected_workloads connected_gate=$connected_gate proposal_app_space=${proposal_app_space:-none}"
