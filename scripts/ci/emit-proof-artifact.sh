#!/usr/bin/env bash
set -euo pipefail

out_dir="${1:-./proof-artifact}"
mkdir -p "${out_dir}"

timestamp="${PROOF_TIMESTAMP:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
run_id="${GITHUB_RUN_ID:-local}"
workflow="${GITHUB_WORKFLOW:-CI}"
event_name="${GITHUB_EVENT_NAME:-unknown}"
ref_name="${GITHUB_REF_NAME:-unknown}"
sha="${GITHUB_SHA:-unknown}"

unit_result="${PROOF_UNIT:-skipped}"
integration_result="${PROOF_INTEGRATION:-skipped}"
gitops_result="${PROOF_GITOPS:-skipped}"
demos_result="${PROOF_DEMOS:-skipped}"
connected_result="${PROOF_CONNECTED:-skipped}"
full_result="${PROOF_FULL:-skipped}"
coverage_total="${PROOF_COVERAGE_TOTAL:-unknown}"
coverage_min="${PROOF_COVERAGE_MIN:-unknown}"

is_numeric_percent() {
  [[ "$1" =~ ^[0-9]+([.][0-9]+)?$ ]]
}

if [[ "${unit_result}" == "success" ]]; then
  if [[ "${coverage_total}" == "unknown" || "${coverage_min}" == "unknown" ]]; then
    echo "coverage fields are required when unit tier succeeds" >&2
    exit 1
  fi

  if ! is_numeric_percent "${coverage_total}" || ! is_numeric_percent "${coverage_min}"; then
    echo "coverage fields must be numeric percentages when unit tier succeeds" >&2
    exit 1
  fi

  if ! awk -v total="${coverage_total}" -v min="${coverage_min}" 'BEGIN {exit !(total+0 >= 0 && total+0 <= 100 && min+0 >= 0 && min+0 <= 100)}'; then
    echo "coverage fields must be between 0 and 100 when unit tier succeeds" >&2
    exit 1
  fi

  if ! awk -v total="${coverage_total}" -v min="${coverage_min}" 'BEGIN {exit !(total+0 >= min+0)}'; then
    echo "coverage total must be greater than or equal to coverage minimum" >&2
    exit 1
  fi
fi

json_path="${out_dir}/proof-matrix.json"
md_path="${out_dir}/proof-summary.md"

cat >"${json_path}" <<EOF
{
  "generated_at": "${timestamp}",
  "run_id": "${run_id}",
  "workflow": "${workflow}",
  "event": "${event_name}",
  "ref": "${ref_name}",
  "sha": "${sha}",
  "coverage_total": "${coverage_total}",
  "coverage_min": "${coverage_min}",
  "tiers": {
    "unit": "${unit_result}",
    "integration": "${integration_result}",
    "gitops": "${gitops_result}",
    "demos": "${demos_result}",
    "connected": "${connected_result}",
    "full": "${full_result}"
  }
}
EOF

cat >"${md_path}" <<EOF
# CI Proof Matrix

- Generated: ${timestamp}
- Workflow: ${workflow}
- Run ID: ${run_id}
- Event: ${event_name}
- Ref: ${ref_name}
- SHA: ${sha}
- Coverage total: ${coverage_total}% (required >= ${coverage_min}%)

| Tier | Result |
|---|---|
| unit | ${unit_result} |
| integration | ${integration_result} |
| gitops | ${gitops_result} |
| demos | ${demos_result} |
| connected | ${connected_result} |
| full | ${full_result} |
EOF

echo "Wrote ${json_path}"
echo "Wrote ${md_path}"
