#!/usr/bin/env bash
# Strictly read-only CI promotion gate powered by cub-scout.
#
# Runs optional pre-write check (`cub-scout before-write`), post-deploy landing
# verification (`cub-scout landed`), and optional offline Ed25519 receipt
# signing/verification (`cub-scout receipt sign` / `cub-scout receipt verify`).
#
# Fails closed unless `answer.state == "landed"` (and signature verification
# succeeds when configured). Never writes to Kubernetes clusters.
#
# Refs #596, #645

set -euo pipefail

CUB_SCOUT_BIN="${CUB_SCOUT_BIN:-cub-scout}"
TARGET="${TARGET:-}"
NAMESPACE="${NAMESPACE:-default}"
REVISION="${REVISION:-}"
DIGEST="${DIGEST:-}"
UNIT_REVISION="${UNIT_REVISION:-}"
VERIFY_POD_IMAGES="${VERIFY_POD_IMAGES:-false}"
IMAGE="${IMAGE:-}"
WAIT_DURATION="${WAIT_DURATION:-60s}"
POLL_INTERVAL="${POLL_INTERVAL:-}"
KUBE_CONTEXT="${KUBE_CONTEXT:-}"
RECORDING="${RECORDING:-}"
RUN_BEFORE_WRITE="${RUN_BEFORE_WRITE:-false}"
FIELD_PATH="${FIELD_PATH:-}"
SIGN_KEY_PATH="${SIGN_KEY_PATH:-}"
VERIFY_PUBKEY_PATH="${VERIFY_PUBKEY_PATH:-}"
OUTPUT_DIR="${OUTPUT_DIR:-.cub-scout-gate}"

if [[ -z "$TARGET" ]]; then
  echo "ERROR: TARGET is required (for example deploy/api)" >&2
  exit 2
fi

if [[ -z "$REVISION" && -z "$DIGEST" && -z "$UNIT_REVISION" ]]; then
  echo "ERROR: one of REVISION, DIGEST, or UNIT_REVISION is required" >&2
  exit 2
fi

mkdir -p "$OUTPUT_DIR"

RECEIPT_PATH="${OUTPUT_DIR}/landed.json"
BEFORE_WRITE_PATH="${OUTPUT_DIR}/before-write.json"
SIGNED_RECEIPT_PATH=""

write_output() {
  local key="$1"
  local value="$2"
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    printf '%s=%s\n' "$key" "$value" >> "$GITHUB_OUTPUT"
  fi
}

extract_json_field() {
  local file="$1"
  local field="$2"
  if command -v jq >/dev/null 2>&1; then
    jq -r "${field} // empty" "$file" 2>/dev/null || true
    return
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$file" "$field" <<'PY'
import json, sys
path, expr = sys.argv[1], sys.argv[2].lstrip(".")
with open(path, "r", encoding="utf-8") as fh:
    data = json.load(fh)
cur = data
for part in expr.split("."):
    if isinstance(cur, dict) and part in cur:
        cur = cur[part]
    else:
        cur = None
        break
if cur is not None:
    print(cur)
PY
    return
  fi
  echo "ERROR: jq or python3 is required to parse cub-scout JSON output" >&2
  return 1
}

# 1. Optional strictly read-only pre-write safety check.
if [[ "$RUN_BEFORE_WRITE" == "true" ]]; then
  bw_args=(before-write "$TARGET" -n "$NAMESPACE" --format json)
  if [[ -n "$FIELD_PATH" ]]; then
    bw_args+=(--field-path "$FIELD_PATH")
  fi
  if [[ -n "$KUBE_CONTEXT" ]]; then
    bw_args+=(--kube-context "$KUBE_CONTEXT")
  fi
  if [[ -n "$RECORDING" ]]; then
    bw_args+=(--recording "$RECORDING")
  fi
  "$CUB_SCOUT_BIN" "${bw_args[@]}" > "$BEFORE_WRITE_PATH"
  bw_state="$(extract_json_field "$BEFORE_WRITE_PATH" '.answer.state')"
  echo "before-write: target=${TARGET} namespace=${NAMESPACE} state=${bw_state:-unknown} path=${BEFORE_WRITE_PATH}"
fi

# 2. Post-deploy read-only landing verification.
landed_args=(landed "$TARGET" -n "$NAMESPACE" --format json)
if [[ -n "$REVISION" ]]; then
  landed_args+=(--revision "$REVISION")
fi
if [[ -n "$DIGEST" ]]; then
  landed_args+=(--digest "$DIGEST")
fi
if [[ -n "$UNIT_REVISION" ]]; then
  landed_args+=(--unit-revision "$UNIT_REVISION")
fi
if [[ "$VERIFY_POD_IMAGES" == "true" ]]; then
  landed_args+=(--verify-pod-images)
  if [[ -n "$IMAGE" ]]; then
    landed_args+=(--image "$IMAGE")
  fi
fi
if [[ -n "$WAIT_DURATION" && -z "$RECORDING" ]]; then
  landed_args+=(--wait "$WAIT_DURATION")
fi
if [[ -n "$POLL_INTERVAL" && -z "$RECORDING" ]]; then
  landed_args+=(--poll-interval "$POLL_INTERVAL")
fi
if [[ -n "$KUBE_CONTEXT" ]]; then
  landed_args+=(--kube-context "$KUBE_CONTEXT")
fi
if [[ -n "$RECORDING" ]]; then
  landed_args+=(--recording "$RECORDING")
fi

landed_rc=0
"$CUB_SCOUT_BIN" "${landed_args[@]}" > "$RECEIPT_PATH" || landed_rc=$?

if [[ ! -s "$RECEIPT_PATH" ]]; then
  write_output "state" "not-observable"
  write_output "receipt-path" "$RECEIPT_PATH"
  write_output "signed-receipt-path" ""
  echo "FAIL-CLOSED: cub-scout landed produced no JSON output (exit=${landed_rc})" >&2
  exit 1
fi

STATE="$(extract_json_field "$RECEIPT_PATH" '.answer.state')"
REASON="$(extract_json_field "$RECEIPT_PATH" '.answer.reason')"
SUMMARY="$(extract_json_field "$RECEIPT_PATH" '.answer.summary')"

if [[ -z "$STATE" ]]; then
  STATE="not-observable"
fi

# 3. Optional offline Ed25519 receipt signing.
if [[ -n "$SIGN_KEY_PATH" ]]; then
  SIGNED_RECEIPT_PATH="${OUTPUT_DIR}/landed.signed.json"
  if ! "$CUB_SCOUT_BIN" receipt sign --input "$RECEIPT_PATH" --key "$SIGN_KEY_PATH" -o "$SIGNED_RECEIPT_PATH"; then
    write_output "state" "$STATE"
    write_output "receipt-path" "$RECEIPT_PATH"
    write_output "signed-receipt-path" ""
    echo "FAIL-CLOSED: cub-scout receipt sign failed" >&2
    exit 1
  fi
fi

# 4. Optional offline Ed25519 receipt verification.
if [[ -n "$VERIFY_PUBKEY_PATH" ]]; then
  if [[ -z "$SIGNED_RECEIPT_PATH" || ! -f "$SIGNED_RECEIPT_PATH" ]]; then
    write_output "state" "$STATE"
    write_output "receipt-path" "$RECEIPT_PATH"
    write_output "signed-receipt-path" ""
    echo "FAIL-CLOSED: verify-pubkey-path was set without a signed receipt (set sign-key-path)" >&2
    exit 1
  fi
  if ! "$CUB_SCOUT_BIN" receipt verify --input "$SIGNED_RECEIPT_PATH" --pubkey "$VERIFY_PUBKEY_PATH" --format json > "${OUTPUT_DIR}/receipt-verify.json"; then
    write_output "state" "$STATE"
    write_output "receipt-path" "$RECEIPT_PATH"
    write_output "signed-receipt-path" "$SIGNED_RECEIPT_PATH"
    echo "FAIL-CLOSED: cub-scout receipt verify failed" >&2
    exit 1
  fi
fi

write_output "state" "$STATE"
write_output "receipt-path" "$RECEIPT_PATH"
write_output "signed-receipt-path" "$SIGNED_RECEIPT_PATH"

echo "landed: target=${TARGET} namespace=${NAMESPACE} state=${STATE} reason=${REASON:-unknown}"
if [[ -n "$SUMMARY" ]]; then
  echo "summary: ${SUMMARY}"
fi

# Fail closed unless the deterministic answer state is strictly `landed`.
if [[ "$STATE" != "landed" ]]; then
  echo "FAIL-CLOSED: promotion gate blocked because answer.state='${STATE}' (expected 'landed')" >&2
  exit 1
fi
