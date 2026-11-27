#!/usr/bin/env bash
# Deterministic offline test for the verify-landed CI promotion gate.
#
# Proves (with zero cluster access):
#   1. Matching revision + Pod image digest + before-write + Ed25519 sign/verify -> PASS (exit 0, state=landed).
#   2. Mismatched revision -> FAIL-CLOSED (non-zero exit, state=not-landed).
#   3. Wrong Ed25519 public key -> FAIL-CLOSED (non-zero exit on receipt verify).
#
# Refs #596, #645

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
GATE_SCRIPT="${SCRIPT_DIR}/verify-landed.sh"

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

CUB_SCOUT_BIN="${CUB_SCOUT_BIN:-}"
if [[ -z "$CUB_SCOUT_BIN" ]]; then
  CUB_SCOUT_BIN="${TMP_DIR}/cub-scout"
  (cd "$REPO_ROOT" && go build -o "$CUB_SCOUT_BIN" ./cmd/cub-scout)
fi

if [[ ! -x "$CUB_SCOUT_BIN" ]]; then
  echo "ERROR: cub-scout binary not executable: $CUB_SCOUT_BIN" >&2
  exit 1
fi

# Generate deterministic test Ed25519 keypairs (primary + wrong key).
"$CUB_SCOUT_BIN" receipt keygen \
  --out-private "${TMP_DIR}/priv.pem" \
  --out-public "${TMP_DIR}/pub.pem" >/dev/null

"$CUB_SCOUT_BIN" receipt keygen \
  --out-private "${TMP_DIR}/other-priv.pem" \
  --out-public "${TMP_DIR}/other-pub.pem" >/dev/null

# Write a deterministic offline recording fixture (Argo CD Application + Deployment + Running Pod).
RECORDING_FILE="${TMP_DIR}/recording.yaml"
cat > "$RECORDING_FILE" <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: staging
  generation: 2
  labels:
    argocd.argoproj.io/instance: api-app
spec:
  replicas: 2
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: ghcr.io/example/api:v1.2.3
status:
  observedGeneration: 2
  replicas: 2
  updatedReplicas: 2
  readyReplicas: 2
  availableReplicas: 2
  conditions:
    - type: Available
      status: "True"
    - type: Progressing
      status: "True"
      reason: NewReplicaSetAvailable
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: api-app
  namespace: argocd
status:
  sync:
    status: Synced
    revision: a1b2c3d4e5f67890
  health:
    status: Healthy
---
apiVersion: v1
kind: Pod
metadata:
  name: api-pod-1
  namespace: staging
  labels:
    app: api
status:
  phase: Running
  containerStatuses:
    - name: api
      image: ghcr.io/example/api:v1.2.3
      imageID: docker-pullable://ghcr.io/example/api@sha256:1111222233334444
EOF

# ------------------------------------------------------------------------------
# Case 1: Matching revision + Pod image digest + before-write + Ed25519 sign/verify -> PASS
# ------------------------------------------------------------------------------
PASS_OUT="${TMP_DIR}/pass-out"
PASS_GH_OUT="${TMP_DIR}/pass.github_output"
: > "$PASS_GH_OUT"

CUB_SCOUT_BIN="$CUB_SCOUT_BIN" \
TARGET="deploy/api" \
NAMESPACE="staging" \
REVISION="a1b2c3d" \
VERIFY_POD_IMAGES="true" \
IMAGE="sha256:1111222233334444" \
RECORDING="$RECORDING_FILE" \
RUN_BEFORE_WRITE="true" \
FIELD_PATH=".spec.replicas" \
SIGN_KEY_PATH="${TMP_DIR}/priv.pem" \
VERIFY_PUBKEY_PATH="${TMP_DIR}/pub.pem" \
OUTPUT_DIR="$PASS_OUT" \
GITHUB_OUTPUT="$PASS_GH_OUT" \
  bash "$GATE_SCRIPT"

grep -qx "state=landed" "$PASS_GH_OUT"
grep -qx "receipt-path=${PASS_OUT}/landed.json" "$PASS_GH_OUT"
grep -qx "signed-receipt-path=${PASS_OUT}/landed.signed.json" "$PASS_GH_OUT"
test -s "${PASS_OUT}/before-write.json"
test -s "${PASS_OUT}/landed.json"
test -s "${PASS_OUT}/landed.signed.json"
test -s "${PASS_OUT}/receipt-verify.json"

# ------------------------------------------------------------------------------
# Case 2: Mismatched revision -> FAIL-CLOSED (state=not-landed, exit != 0)
# ------------------------------------------------------------------------------
FAIL_OUT="${TMP_DIR}/fail-out"
FAIL_GH_OUT="${TMP_DIR}/fail.github_output"
: > "$FAIL_GH_OUT"

fail_rc=0
CUB_SCOUT_BIN="$CUB_SCOUT_BIN" \
TARGET="deploy/api" \
NAMESPACE="staging" \
REVISION="deadbeef" \
RECORDING="$RECORDING_FILE" \
OUTPUT_DIR="$FAIL_OUT" \
GITHUB_OUTPUT="$FAIL_GH_OUT" \
  bash "$GATE_SCRIPT" >/dev/null 2>&1 || fail_rc=$?

if [[ "$fail_rc" -eq 0 ]]; then
  echo "ERROR: expected non-zero exit for mismatched revision (fail-closed)" >&2
  exit 1
fi
grep -qx "state=not-landed" "$FAIL_GH_OUT"
test -s "${FAIL_OUT}/landed.json"

# ------------------------------------------------------------------------------
# Case 3: Wrong Ed25519 public key -> FAIL-CLOSED (exit != 0)
# ------------------------------------------------------------------------------
BADKEY_OUT="${TMP_DIR}/badkey-out"
BADKEY_GH_OUT="${TMP_DIR}/badkey.github_output"
: > "$BADKEY_GH_OUT"

badkey_rc=0
CUB_SCOUT_BIN="$CUB_SCOUT_BIN" \
TARGET="deploy/api" \
NAMESPACE="staging" \
REVISION="a1b2c3d" \
RECORDING="$RECORDING_FILE" \
SIGN_KEY_PATH="${TMP_DIR}/priv.pem" \
VERIFY_PUBKEY_PATH="${TMP_DIR}/other-pub.pem" \
OUTPUT_DIR="$BADKEY_OUT" \
GITHUB_OUTPUT="$BADKEY_GH_OUT" \
  bash "$GATE_SCRIPT" >/dev/null 2>&1 || badkey_rc=$?

if [[ "$badkey_rc" -eq 0 ]]; then
  echo "ERROR: expected non-zero exit for wrong Ed25519 public key (fail-closed)" >&2
  exit 1
fi

echo "PASS: verify-landed offline CI gate tests passed (landed pass, not-landed fail-closed, bad-key fail-closed)"
