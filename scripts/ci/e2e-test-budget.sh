#!/usr/bin/env bash
# Run the cmd/cub-scout e2e test package under a hard cap and a time budget
# (#800). The cap is go test's -timeout; the budget is lower, so a suite that
# grows too slow fails here with a message naming the budget, long before a
# test-timeout panic inside whichever test happened to be running.
#
# Usage: scripts/ci/e2e-test-budget.sh
#        scripts/ci/e2e-test-budget.sh --check-log FILE   # judge a saved go test log
set -euo pipefail

cap="${E2E_TEST_TIMEOUT_SECONDS:-600}"
budget="${E2E_TEST_BUDGET_SECONDS:-300}"
package="github.com/confighub/cub-scout/v2/cmd/cub-scout"

check_log() {
  local log="$1" line seconds
  if grep -q '^panic: test timed out after' "${log}"; then
    echo "E2E time budget failed: ${package} hit the ${cap}s hard cap (budget ${budget}s). Speed the suite up or split it; see #800." >&2
    return 1
  fi
  line="$(grep -E "^(ok|FAIL)[[:space:]]+${package}[[:space:]]" "${log}" | tail -n 1 || true)"
  seconds="$(printf '%s\n' "${line}" | awk '{t=$NF; if (t ~ /^[0-9]+(\.[0-9]+)?s$/) {sub(/s$/, "", t); print t}}')"
  if [[ -z "${seconds}" ]]; then
    echo "E2E time budget failed: no timed result line for ${package}; elapsed time is unknown, not within budget." >&2
    return 1
  fi
  if awk -v s="${seconds}" -v b="${budget}" 'BEGIN { exit !(s > b) }'; then
    echo "E2E time budget failed: ${package} took ${seconds}s, over the ${budget}s budget (hard cap ${cap}s). Speed the suite up or split it; see #800." >&2
    return 1
  fi
  echo "E2E time budget passed: ${package} took ${seconds}s of a ${budget}s budget (hard cap ${cap}s)."
}

if [[ "${1:-}" == "--check-log" ]]; then
  check_log "${2:?usage: e2e-test-budget.sh --check-log FILE}"
  exit
fi

log="$(mktemp)"
trap 'rm -f "${log}"' EXIT
status=0
# -count=1: a cached result carries no elapsed time to judge.
go test -tags=e2e ./cmd/cub-scout/... -v -count=1 -timeout "${cap}s" 2>&1 | tee "${log}" || status=$?
if ! check_log "${log}"; then
  exit 1
fi
exit "${status}"
