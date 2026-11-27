#!/usr/bin/env bash
# Offline multi-cluster fleet promotion proof verification script.
#
# Proves (with zero cluster access, zero network calls, zero LLM):
#   1. Bundled 4-cluster signed receipts (dev, staging, prod-east, prod-west)
#      verify cleanly against test-pub.pem -> exit 0, answer.state == "landed".
#   2. Tampering with a copied receipt in a temp directory fails closed ->
#      non-zero exit, answer.state == "not-landed".
#
# Refs #889, #645

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

CUB_SCOUT_BIN="${CUB_SCOUT_BIN:-./cub-scout}"
if [[ ! -x "$CUB_SCOUT_BIN" ]]; then
  if [[ -x "${REPO_ROOT}/cub-scout" ]]; then
    CUB_SCOUT_BIN="${REPO_ROOT}/cub-scout"
  else
    echo "ERROR: cub-scout binary not found or not executable at: ${CUB_SCOUT_BIN}" >&2
    echo "Build it first with: go build -o ./cub-scout ./cmd/cub-scout" >&2
    exit 1
  fi
fi

EXPECTED_REVISION="7c9e2a4f"
REQUIRED_CLUSTERS="dev,staging,prod-east,prod-west"
PUBKEY="${SCRIPT_DIR}/test-pub.pem"
RECEIPTS_DIR="${SCRIPT_DIR}/receipts"

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

extract_answer_state() {
  local json_file="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -r '.answer.state // empty' "$json_file"
  elif command -v python3 >/dev/null 2>&1; then
    python3 - "$json_file" <<'PY'
import json, sys
with open(sys.argv[1], "r", encoding="utf-8") as f:
    doc = json.load(f)
print((doc.get("answer") or {}).get("state", ""))
PY
  else
    awk '
      /"answer"[[:space:]]*:/ { in_answer = 1 }
      in_answer && /"state"[[:space:]]*:/ {
        gsub(/.*"state"[[:space:]]*:[[:space:]]*"/, "")
        gsub(/".*/, "")
        print
        exit
      }
    ' "$json_file"
  fi
}

echo "==> 1. Verifying 4-cluster fleet receipts (${REQUIRED_CLUSTERS}) at revision ${EXPECTED_REVISION}..."
PASS_JSON="${TMP_DIR}/pass.json"
"$CUB_SCOUT_BIN" receipt verify-fleet \
  --inputs "$RECEIPTS_DIR" \
  --pubkey "$PUBKEY" \
  --revision "$EXPECTED_REVISION" \
  --require-clusters "$REQUIRED_CLUSTERS" \
  --format json > "$PASS_JSON"

PASS_STATE="$(extract_answer_state "$PASS_JSON")"
if [[ "$PASS_STATE" != "landed" ]]; then
  echo "ERROR: expected answer.state == landed, got: ${PASS_STATE:-<empty>}" >&2
  cat "$PASS_JSON" >&2
  exit 1
fi

"$CUB_SCOUT_BIN" receipt verify-fleet \
  --inputs "$RECEIPTS_DIR" \
  --pubkey "$PUBKEY" \
  --revision "$EXPECTED_REVISION" \
  --require-clusters "$REQUIRED_CLUSTERS"

echo "==> 2. Verifying fail-closed behavior on a tampered receipt copy..."
TAMPER_DIR="${TMP_DIR}/tampered-receipts"
cp -R "$RECEIPTS_DIR" "$TAMPER_DIR"

# Modify the signed payload of prod-west.json in the temporary copy without re-signing.
sed 's/Deployment\/api converged on the requested git-sha\./Deployment\/api forged summary./g' \
  "${TAMPER_DIR}/prod-west.json" > "${TAMPER_DIR}/prod-west.json.tmp"
mv "${TAMPER_DIR}/prod-west.json.tmp" "${TAMPER_DIR}/prod-west.json"
if cmp -s "${RECEIPTS_DIR}/prod-west.json" "${TAMPER_DIR}/prod-west.json"; then
  echo "ERROR: failed to mutate temporary prod-west.json copy for tamper check" >&2
  exit 1
fi

TAMPER_JSON="${TMP_DIR}/tamper.json"
tamper_rc=0
"$CUB_SCOUT_BIN" receipt verify-fleet \
  --inputs "$TAMPER_DIR" \
  --pubkey "$PUBKEY" \
  --revision "$EXPECTED_REVISION" \
  --require-clusters "$REQUIRED_CLUSTERS" \
  --format json > "$TAMPER_JSON" 2>/dev/null || tamper_rc=$?

if [[ "$tamper_rc" -eq 0 ]]; then
  echo "ERROR: expected non-zero exit code on tampered receipt (fail-closed)" >&2
  cat "$TAMPER_JSON" >&2
  exit 1
fi

TAMPER_STATE="$(extract_answer_state "$TAMPER_JSON")"
if [[ "$TAMPER_STATE" != "not-landed" ]]; then
  echo "ERROR: expected answer.state == not-landed on tampered receipt, got: ${TAMPER_STATE:-<empty>}" >&2
  cat "$TAMPER_JSON" >&2
  exit 1
fi

echo "PASS: fleet promotion proof verified (4/4 landed pass; tampered copy failed closed with not-landed, exit ${tamper_rc})"
