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
