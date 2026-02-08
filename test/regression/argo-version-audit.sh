#!/usr/bin/env bash
# Compare Argo tree behavior between v0.4.0 and v0.19.6 using deterministic local fixtures.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORK_DIR="${TMPDIR:-/tmp}/cub-scout-regression"
OUTPUT_DIR=""
CLUSTER_NAME="cub-scout-regression-argo"
KEEP_CLUSTER=0
SKIP_BUILD=0
WRITE_DOC=0

V040_REF="v0.4.0"
V0196_REF="v0.19.6"
BIN_V040="${WORK_DIR}/bin/cub-scout-${V040_REF}"
BIN_V0196="${WORK_DIR}/bin/cub-scout-${V0196_REF}"

EXPECTED_ARGO_DEPLOYMENTS=4
EXPECTED_ARGO_APPLICATIONS=5

usage() {
  cat <<USAGE
Usage: test/regression/argo-version-audit.sh [options]

Options:
  --cluster-name NAME    Kind cluster name (default: ${CLUSTER_NAME})
  --output-dir DIR       Output directory (default: test/regression/output/<timestamp>)
  --keep-cluster         Do not delete a cluster created by this script
  --skip-build           Reuse existing binaries in ${WORK_DIR}/bin
  --bin-v040 PATH        Path to v0.4.0 binary
  --bin-v0196 PATH       Path to v0.19.6 binary
  --write-doc            Copy generated report into docs/releases/argo-regression-audit-v0.4.0-v0.19.6.md
  -h, --help             Show this help
USAGE
}

log() {
  printf '[argo-audit] %s\n' "$*"
}

fail() {
  printf '[argo-audit] ERROR: %s\n' "$*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster-name)
      CLUSTER_NAME="$2"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --keep-cluster)
      KEEP_CLUSTER=1
      shift
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    --bin-v040)
      BIN_V040="$2"
      shift 2
      ;;
    --bin-v0196)
      BIN_V0196="$2"
      shift 2
      ;;
    --write-doc)
      WRITE_DOC=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

if [[ -z "${OUTPUT_DIR}" ]]; then
  stamp="$(date +%Y%m%d-%H%M%S)"
  OUTPUT_DIR="${ROOT_DIR}/test/regression/output/${stamp}"
fi

require_cmd git
require_cmd go
require_cmd jq
require_cmd kubectl
require_cmd kind
require_cmd docker

if ! docker info >/dev/null 2>&1; then
  fail "docker is not running (required for kind)"
fi

mkdir -p "${WORK_DIR}/bin" "${WORK_DIR}/worktrees" "${OUTPUT_DIR}"

created_cluster=0
cleanup() {
  if [[ "${created_cluster}" -eq 1 && "${KEEP_CLUSTER}" -eq 0 ]]; then
    log "deleting temporary kind cluster ${CLUSTER_NAME}"
    kind delete cluster --name "${CLUSTER_NAME}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

ensure_cluster() {
  if ! kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"; then
    log "creating kind cluster ${CLUSTER_NAME}"
    kind create cluster --name "${CLUSTER_NAME}" >/dev/null
    created_cluster=1
  else
    log "reusing existing kind cluster ${CLUSTER_NAME}"
  fi
  kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null
}

build_binary() {
  local ref="$1"
  local out="$2"
  local safe_ref="${ref//[^A-Za-z0-9._-]/_}"
  local wt="${WORK_DIR}/worktrees/${safe_ref}"

  if [[ ! -d "${wt}/.git" && ! -f "${wt}/go.mod" ]]; then
    log "creating worktree for ${ref}"
    git -C "${ROOT_DIR}" worktree add --detach "${wt}" "${ref}" >/dev/null
  else
    log "refreshing worktree for ${ref}"
    git -C "${wt}" checkout --detach "${ref}" >/dev/null
  fi

  log "building ${ref}"
  (
    cd "${wt}"
    GOCACHE="${WORK_DIR}/gocache-${safe_ref}" go build -o "${out}" ./cmd/cub-scout
  )
}

prepare_fixtures() {
  log "applying Argo CRD fixtures"
  kubectl apply -f "${ROOT_DIR}/test/fixtures/regression/argo-minimal-crds.yaml" >/dev/null
  kubectl wait --for=condition=Established crd/applications.argoproj.io --timeout=60s >/dev/null
  kubectl wait --for=condition=Established crd/applicationsets.argoproj.io --timeout=60s >/dev/null

  log "applying App-of-Apps fixture"
  kubectl apply -f "${ROOT_DIR}/test/fixtures/regression/argo-app-of-apps.yaml" >/dev/null

  log "applying ApplicationSet fixture"
  kubectl apply -f "${ROOT_DIR}/test/fixtures/regression/argo-applicationset.yaml" >/dev/null
}

run_capture() {
  local bin="$1"
  local tag="$2"

  log "running ${tag}: tree ownership"
  "${bin}" tree ownership --all --json > "${OUTPUT_DIR}/${tag}.tree.ownership.json"

  log "running ${tag}: tree git"
  "${bin}" tree git --json > "${OUTPUT_DIR}/${tag}.tree.git.json"
}

read_argo_owner_count() {
  local path="$1"
  jq -r '
    if type == "object" and (.summary.byOwner? != null) then
      (.summary.byOwner.ArgoCD // 0)
    elif type == "object" and ((.groups? // []) | type == "array") then
      ([.groups[]? | select((.owner // "") == "ArgoCD") | ((.items // []) | length)] | add // 0)
    elif type == "array" then
      ([.[]? | select((.owner // "") == "ArgoCD")] | length)
    else
      0
    end
  ' "${path}" 2>/dev/null || echo 0
}

read_argo_app_count() {
  local path="$1"
  jq -r '
    if type == "object" then
      ((.argoApplications // .applications // []) | length)
    elif type == "array" then
      ([.[]? | select((.kind // "") == "Application")] | length)
    else
      0
    end
  ' "${path}" 2>/dev/null || echo 0
}

read_app_of_apps_visibility() {
  local path="$1"
  jq -r '
    def apps:
      if type == "object" then
        (.argoApplications // .applications // [])
      elif type == "array" then
        .
      else
        []
      end;

    apps | any(
      (.parent != null) or
      (.parentApp != null) or
      (.parentApplication != null) or
      (.parentRef != null) or
      ((.ownerRef? | type == "object") and ((.ownerRef.kind // "") | ascii_downcase) == "application")
    )
  ' "${path}" 2>/dev/null || echo false
}

read_appset_visibility() {
  local path="$1"
  jq -r '
    def apps:
      if type == "object" then
        (.argoApplications // .applications // [])
      elif type == "array" then
        .
      else
        []
      end;

    (((.applicationSets // .argoApplicationSets // []) | length) > 0) or
    (apps | any(
      (.applicationSet != null) or
      (.applicationSetName != null) or
      (.generatedBy != null) or
      (.generatedByApplicationSet != null)
    ))
  ' "${path}" 2>/dev/null || echo false
}

classify_count() {
  local old="$1"
  local new="$2"
  local expected="$3"

  if (( new < old )); then
    echo regression
  elif (( new > old )); then
    echo intentional
  elif (( new < expected )); then
    echo intentional-gap
  else
    echo match
  fi
}

classify_bool() {
  local old="$1"
  local new="$2"

  if [[ "${old}" == "true" && "${new}" == "false" ]]; then
    echo regression
  elif [[ "${old}" == "false" && "${new}" == "true" ]]; then
    echo intentional
  elif [[ "${old}" == "false" && "${new}" == "false" ]]; then
    echo intentional-gap
  else
    echo match
  fi
}

classification_label() {
  case "$1" in
    regression)
      echo regression
      ;;
    intentional)
      echo "intentional improvement"
      ;;
    intentional-gap)
      echo "known gap (no regression)"
      ;;
    *)
      echo match
      ;;
  esac
}

ensure_cluster
prepare_fixtures

if [[ "${SKIP_BUILD}" -eq 0 ]]; then
  build_binary "${V040_REF}" "${BIN_V040}"
  build_binary "${V0196_REF}" "${BIN_V0196}"
fi

[[ -x "${BIN_V040}" ]] || fail "v0.4.0 binary not found/executable: ${BIN_V040}"
[[ -x "${BIN_V0196}" ]] || fail "v0.19.6 binary not found/executable: ${BIN_V0196}"

run_capture "${BIN_V040}" "v0.4.0"
run_capture "${BIN_V0196}" "v0.19.6"

ownership_old="$(read_argo_owner_count "${OUTPUT_DIR}/v0.4.0.tree.ownership.json")"
ownership_new="$(read_argo_owner_count "${OUTPUT_DIR}/v0.19.6.tree.ownership.json")"
apps_old="$(read_argo_app_count "${OUTPUT_DIR}/v0.4.0.tree.git.json")"
apps_new="$(read_argo_app_count "${OUTPUT_DIR}/v0.19.6.tree.git.json")"
app_of_apps_old="$(read_app_of_apps_visibility "${OUTPUT_DIR}/v0.4.0.tree.git.json")"
app_of_apps_new="$(read_app_of_apps_visibility "${OUTPUT_DIR}/v0.19.6.tree.git.json")"
appset_old="$(read_appset_visibility "${OUTPUT_DIR}/v0.4.0.tree.git.json")"
appset_new="$(read_appset_visibility "${OUTPUT_DIR}/v0.19.6.tree.git.json")"

class_ownership="$(classify_count "${ownership_old}" "${ownership_new}" "${EXPECTED_ARGO_DEPLOYMENTS}")"
class_apps="$(classify_count "${apps_old}" "${apps_new}" "${EXPECTED_ARGO_APPLICATIONS}")"
class_app_of_apps="$(classify_bool "${app_of_apps_old}" "${app_of_apps_new}")"
class_appset="$(classify_bool "${appset_old}" "${appset_new}")"

report_path="${OUTPUT_DIR}/report.md"
cat > "${report_path}" <<REPORT
# Argo Regression Audit: v0.4.0 vs v0.19.6

Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
Cluster: kind-${CLUSTER_NAME}
Fixtures:
- test/fixtures/regression/argo-minimal-crds.yaml
- test/fixtures/regression/argo-app-of-apps.yaml
- test/fixtures/regression/argo-applicationset.yaml

Expected fixture counts:
- Argo-owned Deployments: ${EXPECTED_ARGO_DEPLOYMENTS}
- Argo Applications: ${EXPECTED_ARGO_APPLICATIONS}

| Check | v0.4.0 | v0.19.6 | Status | Notes |
|---|---:|---:|---|---|
| tree ownership: Argo-owned deployment visibility | ${ownership_old} | ${ownership_new} | $(classification_label "${class_ownership}") | If below expected on both: intentional gap; if lower in v0.19.6: regression |
| tree git: Argo Application visibility | ${apps_old} | ${apps_new} | $(classification_label "${class_apps}") | Compares observed applications in git view |
| App-of-Apps parent/child visibility in tree git | ${app_of_apps_old} | ${app_of_apps_new} | $(classification_label "${class_app_of_apps}") | Known gap tracked by #128 when false |
| ApplicationSet -> generated visibility in tree git | ${appset_old} | ${appset_new} | $(classification_label "${class_appset}") | Known gap tracked by #132 when false |

## Raw outputs
- v0.4.0 tree ownership JSON: \
  ${OUTPUT_DIR}/v0.4.0.tree.ownership.json
- v0.19.6 tree ownership JSON: \
  ${OUTPUT_DIR}/v0.19.6.tree.ownership.json
- v0.4.0 tree git JSON: \
  ${OUTPUT_DIR}/v0.4.0.tree.git.json
- v0.19.6 tree git JSON: \
  ${OUTPUT_DIR}/v0.19.6.tree.git.json
REPORT

# JSON summary for CI and automation.
cat > "${OUTPUT_DIR}/summary.json" <<JSON
{
  "ownership": {
    "v0_4_0": ${ownership_old},
    "v0_19_6": ${ownership_new},
    "classification": "${class_ownership}",
    "status": "$(classification_label "${class_ownership}")"
  },
  "gitApplications": {
    "v0_4_0": ${apps_old},
    "v0_19_6": ${apps_new},
    "classification": "${class_apps}",
    "status": "$(classification_label "${class_apps}")"
  },
  "appOfAppsVisibility": {
    "v0_4_0": ${app_of_apps_old},
    "v0_19_6": ${app_of_apps_new},
    "classification": "${class_app_of_apps}",
    "status": "$(classification_label "${class_app_of_apps}")"
  },
  "applicationSetVisibility": {
    "v0_4_0": ${appset_old},
    "v0_19_6": ${appset_new},
    "classification": "${class_appset}",
    "status": "$(classification_label "${class_appset}")"
  }
}
JSON

if [[ "${WRITE_DOC}" -eq 1 ]]; then
  cp "${report_path}" "${ROOT_DIR}/docs/releases/argo-regression-audit-v0.4.0-v0.19.6.md"
  log "updated docs/releases/argo-regression-audit-v0.4.0-v0.19.6.md"
fi

log "report: ${report_path}"
log "summary: ${OUTPUT_DIR}/summary.json"

regressions=0
for cls in "${class_ownership}" "${class_apps}" "${class_app_of_apps}" "${class_appset}"; do
  if [[ "${cls}" == "regression" ]]; then
    regressions=$((regressions + 1))
  fi
done

if (( regressions > 0 )); then
  fail "regression(s) detected: ${regressions}"
fi

log "no regressions detected (intentional gaps may remain for #128/#132)"
