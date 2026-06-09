#!/usr/bin/env bash
#
# verify.sh — run the real cub-scout object-set-matches path end-to-end against
# the fixture cluster state, then contrast the receipt verdict with the actual
# workload reality. The point of this script is to make the current gaps
# OBSERVABLE, not to hide them.
#
# Read-only against the cluster. Writes only into ./out/ in this directory.
# Self-contained: never reads a helm-expt checkout.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
NS="helm-expt-demo"
CLUSTER="cub-scout-helm-expt"
CONTEXT="kind-${CLUSTER}"
MANIFESTS="$SCRIPT_DIR/fixtures/release-objects.yaml"
RUN_DIR="$SCRIPT_DIR/out"
RECEIPT="$RUN_DIR/object-set.receipt.json"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --context) CONTEXT="$2"; shift 2 ;;
        --context=*) CONTEXT="${1#*=}"; shift ;;
        --no-cluster) CONTEXT="$(kubectl config current-context)"; shift ;;
        -h|--help) echo "Usage: ./verify.sh [--context <ctx> | --no-cluster]"; exit 0 ;;
        *) echo "unknown argument: $1" >&2; exit 2 ;;
    esac
done

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

resolve_cub_scout() {
    if [[ -n "${CUB_SCOUT_BIN:-}" ]]; then printf '%s\n' "$CUB_SCOUT_BIN"; return; fi
    if [[ -x "$REPO_ROOT/cub-scout" ]]; then printf '%s\n' "$REPO_ROOT/cub-scout"; return; fi
    command -v cub-scout 2>/dev/null || echo ""
}
CUB_SCOUT="$(resolve_cub_scout)"
[[ -n "$CUB_SCOUT" ]] || { echo "cub-scout binary not found (build ./cmd/cub-scout or set CUB_SCOUT_BIN)" >&2; exit 1; }

export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"
kubectl --context "$CONTEXT" get ns "$NS" >/dev/null 2>&1 || {
    echo "namespace '$NS' not found in context '$CONTEXT' — run ./setup.sh first" >&2; exit 1; }

mkdir -p "$RUN_DIR"
rule() { printf '\n========== %s ==========\n' "$1"; }

# ---------------------------------------------------------------------------
rule "1. object-set-matches receipt (the keystone integration the READMEs promise)"
# cub-scout reads the live cluster via the current context, so point it there.
( kubectl config use-context "$CONTEXT" >/dev/null 2>&1 || true )
set +e
"$CUB_SCOUT" receipt verify \
    --file "$MANIFESTS" \
    --scope "namespace/$NS" \
    --format json \
    --out "$RECEIPT" \
    --fail-on any-non-pass >"$RUN_DIR/object-set.stdout.json" 2>"$RUN_DIR/object-set.stderr.txt"
RECEIPT_RC=$?
set -e
[[ -s "$RECEIPT" ]] || RECEIPT="$RUN_DIR/object-set.stdout.json"

VERDICT="$(jq -r '.predicate.verdict // .predicate.result // .verdict // "UNKNOWN"' "$RECEIPT" 2>/dev/null || echo UNKNOWN)"
MATCHED="$(jq -r '[.. | objects | .matched? // empty] | add // "?"' "$RECEIPT" 2>/dev/null || echo '?')"
echo "receipt verdict         : $VERDICT"
echo "receipt exit code       : $RECEIPT_RC   (--fail-on any-non-pass; 0 means cub-scout considers this a clean install)"
echo "receipt written to      : ${RECEIPT/#$SCRIPT_DIR\//}"

# ---------------------------------------------------------------------------
rule "2. what is ACTUALLY happening in the cluster"
kubectl --context "$CONTEXT" -n "$NS" get deploy,pods -o wide || true
POD_JSON="$(kubectl --context "$CONTEXT" -n "$NS" get pods -o json 2>/dev/null || echo '{}')"
WAIT_REASONS="$(echo "$POD_JSON" | jq -r '[.items[]?.status.containerStatuses[]?.state.waiting.reason] | map(select(. != null)) | unique | join(",")')"
READY="$(echo "$POD_JSON" | jq -r '[.items[]?.status.conditions[]? | select(.type=="Ready") | .status] | join(",")')"
SECRET_PRESENT="$(kubectl --context "$CONTEXT" -n "$NS" get secret app-db-secret >/dev/null 2>&1 && echo yes || echo no)"
echo
echo "pod waiting reason      : ${WAIT_REASONS:-<none>}"
echo "pod Ready condition     : ${READY:-<none>}"
echo "required Secret present : $SECRET_PRESENT  (app-db-secret — the unmet prerequisite)"

# ---------------------------------------------------------------------------
rule "3. receipt vs reality — where object-set-matches falls short"
FALSE_GREEN="no"
if [[ "$VERDICT" == "PASS" && ( "$WAIT_REASONS" == *CreateContainerConfigError* || "$READY" != *True* ) ]]; then
    FALSE_GREEN="yes"
fi

# Probe the receipt envelope for the fields helm-expt expects but cub-scout
# does not emit yet.
HAS_FRESHNESS="$(grep -iEc 'ttl|fresh|expire' "$RECEIPT" 2>/dev/null || true)"
HAS_READINESS="$(grep -iEc 'prerequisite|createcontainer|"ready"|isReady|readyReplicas' "$RECEIPT" 2>/dev/null || true)"
OMISSIONS="$(jq -r '[.. | objects | .omissions? // empty] | add // [] | map(.id // .reason // tostring) | join(", ")' "$RECEIPT" 2>/dev/null || echo '')"

printf '%-42s | %s\n' "claim / capability" "observed"
printf -- '-------------------------------------------+----------------------------------\n'
printf '%-42s | %s\n' "object-set-matches verdict"            "$VERDICT"
printf '%-42s | %s\n' "workload actually usable?"             "$([[ "$READY" == *True* ]] && echo yes || echo NO)"
printf '%-42s | %s\n' "=> false green (PASS but unusable)?"   "$FALSE_GREEN"
printf '%-42s | %s\n' "readiness / Job / PVC status in claim" "$([[ "$HAS_READINESS" -gt 0 ]] && echo present || echo ABSENT)"
printf '%-42s | %s\n' "prerequisite (required Secret) checked" "ABSENT"
printf '%-42s | %s\n' "freshness / TTL field on receipt"      "$([[ "$HAS_FRESHNESS" -gt 0 ]] && echo present || echo ABSENT)"
printf '%-42s | %s\n' "self-declared coverage omission(s)"    "${OMISSIONS:-<none>}"

# ---------------------------------------------------------------------------
rule "4. the rest of the runtime menu (informational)"
for desc_cmd in \
    "doctor|doctor --namespace $NS --format json" \
    "map status|map status --namespace $NS --json" \
    "compare drift|compare drift --file $MANIFESTS -n $NS --format json"; do
    desc="${desc_cmd%%|*}"; args="${desc_cmd#*|}"
    echo "--- cub-scout $desc ---"
    # shellcheck disable=SC2086
    "$CUB_SCOUT" $args >"$RUN_DIR/${desc// /-}.json" 2>/dev/null \
        && echo "  saved out/${desc// /-}.json" \
        || echo "  (command returned non-zero or unsupported in this build; see out/)"
done

# ---------------------------------------------------------------------------
rule "verdict for this example"
if [[ "$FALSE_GREEN" == "yes" ]]; then
cat <<EOF
object-set-matches returned ${VERDICT} and exit ${RECEIPT_RC}, yet the workload is
not usable (${WAIT_REASONS:-not ready}) because the prerequisite Secret is absent.

This is the helm-expt F3 "silent false green", reproduced against a real cluster.
It is not a bug in object-set-matches — that predicate proves presence + authored
field match by design — it is the SCOPE gap this example exists to document:

  * no readiness/Job/PVC predicate     -> see issue: workloads-converged
  * no prerequisite predicate          -> see issue: prerequisites-met
  * no freshness/TTL on the receipt    -> see issue: receipt observation freshness
  * extra live objects not covered     -> see issue: closed-world object-set

See docs/proposals/helm-expt-driven-gaps.md for the write-ups.
EOF
else
cat <<EOF
Expected a false-green reproduction but did not detect one. Inspect:
  receipt verdict = $VERDICT, pod waiting = ${WAIT_REASONS:-none}, ready = ${READY:-none}
Did ./setup.sh run, and was the Secret accidentally created?
EOF
fi
echo
echo "Receipt and command output saved under: ${RUN_DIR/#$REPO_ROOT\//}"
