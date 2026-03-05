#!/usr/bin/env bash
set -euo pipefail

coverage_file="${1:-coverage.out}"
min_percent="${COVERAGE_MIN_PERCENT:-15.0}"
summary_path="${COVERAGE_SUMMARY_PATH:-coverage-summary.txt}"

observed=""

if [[ -n "${COVERAGE_TOTAL_OVERRIDE:-}" ]]; then
  observed="${COVERAGE_TOTAL_OVERRIDE}"
else
  if [[ ! -f "${coverage_file}" ]]; then
    echo "coverage profile not found: ${coverage_file}" >&2
    exit 1
  fi

  summary="$(go tool cover -func="${coverage_file}")"
  printf "%s\n" "${summary}" > "${summary_path}"

  observed="$(printf "%s\n" "${summary}" | awk '/^total:/{gsub("%","",$3); print $3}')"
fi

if [[ -z "${observed}" ]]; then
  echo "could not determine total coverage from ${coverage_file}" >&2
  exit 1
fi

if awk "BEGIN {exit !(${observed}+0 >= ${min_percent}+0)}"; then
  echo "Coverage gate passed: observed=${observed}% required>=${min_percent}%"
  exit 0
fi

echo "Coverage gate failed: observed=${observed}% required>=${min_percent}%" >&2
exit 1
