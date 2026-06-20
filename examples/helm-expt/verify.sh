#!/usr/bin/env bash
#
# verify.sh — run the three cub-scout install-receipt predicates end-to-end
# against the fixture cluster state and show how object-set-matches ALONE is a
# false green, while workloads-converged (#476) and prerequisites-met (#477)
# catch helm-expt finding F3 from both ends:
#
#   object-set-matches   PASS   every desired object is present + fields match
#   prerequisites-met    BLOCK  the required Secret is absent (pre-flight)
#   workloads-converged  BLOCK  the pod is wedged in CreateContainerConfigError (runtime)
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
PREREQS="$SCRIPT_DIR/fixtures/prerequisites.yaml"
RUN_DIR="$SCRIPT_DIR/out"

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

# Resolve a cub-scout binary that supports the install-receipt predicates. The
# prebuilt repo-root binary may predate them, so fall back to building fresh.
supports_predicates() { "$1" receipt verify --help 2>/dev/null | grep -q workloads-converged; }
resolve_cub_scout() {
    if [[ -n "${CUB_SCOUT_BIN:-}" ]] && supports_predicates "$CUB_SCOUT_BIN"; then printf '%s\n' "$CUB_SCOUT_BIN"; return; fi
    if [[ -x "$REPO_ROOT/cub-scout" ]] && supports_predicates "$REPO_ROOT/cub-scout"; then printf '%s\n' "$REPO_ROOT/cub-scout"; return; fi
    if command -v cub-scout >/dev/null 2>&1 && supports_predicates "$(command -v cub-scout)"; then command -v cub-scout; return; fi
    echo "==> building a fresh cub-scout (the available binary predates the install-receipt predicates)" >&2
    ( cd "$REPO_ROOT" && go build -o "$RUN_DIR/cub-scout" ./cmd/cub-scout ) >&2 || return 1
    printf '%s\n' "$RUN_DIR/cub-scout"
}
mkdir -p "$RUN_DIR"
CUB_SCOUT="$(resolve_cub_scout)"
[[ -n "$CUB_SCOUT" ]] || { echo "cub-scout binary not found and could not be built" >&2; exit 1; }

export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"
kubectl --context "$CONTEXT" get ns "$NS" >/dev/null 2>&1 || {
    echo "namespace '$NS' not found in context '$CONTEXT' — run ./setup.sh first" >&2; exit 1; }
( kubectl config use-context "$CONTEXT" >/dev/null 2>&1 || true )

rule() { printf '\n========== %s ==========\n' "$1"; }

# run_predicate <label> <verify args...> — runs a receipt, stores it in out/,
# and sets LAST_VERDICT / LAST_RC.
LAST_VERDICT=""; LAST_RC=0
run_predicate() {
    local label="$1"; shift
    set +e
    "$CUB_SCOUT" receipt verify "$@" --scope "namespace/$NS" --format json --ttl 1h \
        --out "$RUN_DIR/$label.receipt.json" --fail-on any-non-pass \
        >"$RUN_DIR/$label.stdout.json" 2>"$RUN_DIR/$label.stderr.txt"
    LAST_RC=$?
    set -e
    local f="$RUN_DIR/$label.receipt.json"; [[ -s "$f" ]] || f="$RUN_DIR/$label.stdout.json"
    LAST_VERDICT="$(jq -r '.predicate.verdict // "UNKNOWN"' "$f" 2>/dev/null || echo UNKNOWN)"
}

# ---------------------------------------------------------------------------
rule "1. object-set-matches — every desired object present + fields match"
run_predicate object-set --file "$MANIFESTS" --predicate object-set-matches
OS_VERDICT="$LAST_VERDICT"; OS_RC="$LAST_RC"
echo "verdict: $OS_VERDICT  (exit $OS_RC)"
jq -c '.predicate.evidence.objectSet.summary' "$RUN_DIR/object-set.receipt.json" 2>/dev/null | sed 's/^/  summary: /'

# ---------------------------------------------------------------------------
rule "2. prerequisites-met (#477) — are the declared target facts present?"
run_predicate prerequisites --prerequisites "$PREREQS" --predicate prerequisites-met
PQ_VERDICT="$LAST_VERDICT"; PQ_RC="$LAST_RC"
echo "verdict: $PQ_VERDICT  (exit $PQ_RC)"
jq -r '.predicate.evidence.prerequisites.facts[] | "  "+.kind+"/"+.name+" -> "+.status' "$RUN_DIR/prerequisites.receipt.json" 2>/dev/null || true

# ---------------------------------------------------------------------------
rule "3. workloads-converged (#476) — did the workloads actually become usable?"
run_predicate workloads --file "$MANIFESTS" --predicate workloads-converged
WL_VERDICT="$LAST_VERDICT"; WL_RC="$LAST_RC"
echo "verdict: $WL_VERDICT  (exit $WL_RC)"
jq -r '.predicate.evidence.workloads.workloads[] | select(.status!="converged") | "  "+.id.kind+"/"+.id.name+" -> "+.status+" (kstatus "+.kstatusStatus+(if (.podReasons|length)>0 then ", pod "+.podReasons[0].reason else "" end)+")"' "$RUN_DIR/workloads.receipt.json" 2>/dev/null || true

# ---------------------------------------------------------------------------
# object-set-diff (#496, Issue A): the SET-LEVEL delta receipt. One receipt
# shape serves both day-1 (dry-run: "what would a proposed change touch?") and
# day-2 (drift: "what differs now?"). Tool-agnostic: reads the cluster + the
# --dry-from source, never the reconciler. This reproduces the helm-expt#992
# image.digest worked example offline against THIS example's live fixture.
rule "4. object-set-diff (#496) — set-level delta receipt (dry-run + drift)"

# Drift reading: diff the EXACT applied render against live. Authored fields
# match, so the set-level diff is PASS (no changed/added/removed). This is the
# day-2 "what differs across the set now?" question.
set +e
"$CUB_SCOUT" compare object-set --dry-from "$MANIFESTS" --scope "namespace/$NS" \
    --format json --ttl 1h --out "$RUN_DIR/object-set-diff.drift.receipt.json" \
    >"$RUN_DIR/object-set-diff.drift.stdout.json" 2>"$RUN_DIR/object-set-diff.drift.stderr.txt"
set -e
DIFF_DRIFT_VERDICT="$(jq -r '.predicate.verdict // "UNKNOWN"' "$RUN_DIR/object-set-diff.drift.receipt.json" 2>/dev/null || echo UNKNOWN)"
echo "drift  (--dry-from = current render):  verdict $DIFF_DRIFT_VERDICT"

# Dry-run reading: build a CHANGED render (bump the container image, the
# image.digest case from helm-expt#992) and diff THAT against live. The image
# field delta makes the set-level diff BLOCK with one changedObject — "this
# proposed change would touch the web Deployment's image". The changed render is
# derived from the committed fixture at runtime so it never drifts from it.
CHANGED_MANIFEST="$RUN_DIR/release-objects.changed-image.yaml"
sed 's#image: busybox:1.36#image: busybox:1.37#' "$MANIFESTS" > "$CHANGED_MANIFEST"
set +e
"$CUB_SCOUT" compare object-set --dry-from "$CHANGED_MANIFEST" --scope "namespace/$NS" \
    --format json --ttl 1h --out "$RUN_DIR/object-set-diff.dryrun.receipt.json" \
    >"$RUN_DIR/object-set-diff.dryrun.stdout.json" 2>"$RUN_DIR/object-set-diff.dryrun.stderr.txt"
set -e
DIFF_DRYRUN_VERDICT="$(jq -r '.predicate.verdict // "UNKNOWN"' "$RUN_DIR/object-set-diff.dryrun.receipt.json" 2>/dev/null || echo UNKNOWN)"
echo "dry-run (--dry-from = changed image):  verdict $DIFF_DRYRUN_VERDICT"
jq -r '.predicate.evidence.objectSetDiff.changedObjects[]? | "  changed "+.id.kind+"/"+.id.name' "$RUN_DIR/object-set-diff.dryrun.receipt.json" 2>/dev/null || true
jq -r '.predicate.evidence.objectSetDiff.changedObjects[]?.differences[]? | "    "+.field+": "+.desired+" -> "+.live' "$RUN_DIR/object-set-diff.dryrun.receipt.json" 2>/dev/null || true

# ---------------------------------------------------------------------------
rule "5. what is ACTUALLY happening in the cluster"
kubectl --context "$CONTEXT" -n "$NS" get deploy,pods -o wide || true

# ---------------------------------------------------------------------------
rule "the install-receipt scorecard"
HAS_FRESHNESS="$(grep -iEc 'ttl|fresh|expire' "$RUN_DIR/object-set.receipt.json" 2>/dev/null || true)"
printf '%-38s | %s\n' "predicate / capability" "verdict"
printf -- '---------------------------------------+--------------------------------------\n'
printf '%-38s | %s\n' "object-set-matches (present+match)"  "$OS_VERDICT  <- green alone = false green"
printf '%-38s | %s\n' "prerequisites-met (#477, pre-flight)" "$PQ_VERDICT  <- target fact: Secret absent"
printf '%-38s | %s\n' "workloads-converged (#476, runtime)"  "$WL_VERDICT  <- pod CreateContainerConfigError"
printf '%-38s | %s\n' "object-set-diff drift (#496, day-2)"  "$DIFF_DRIFT_VERDICT  <- current render == live, set-level"
printf '%-38s | %s\n' "object-set-diff dry-run (#496, day-1)" "$DIFF_DRYRUN_VERDICT  <- changed image would touch web Deployment"
printf '%-38s | %s\n' "receipt freshness/TTL (#478)"         "$([[ "$HAS_FRESHNESS" -gt 0 ]] && echo "present (ttl 1h)" || echo ABSENT)"

# ---------------------------------------------------------------------------
rule "verdict for this example"
if [[ "$OS_VERDICT" == "PASS" && "$WL_VERDICT" == "BLOCK" && "$PQ_VERDICT" == "BLOCK" ]]; then
cat <<EOF
object-set-matches is PASS (exit $OS_RC) — desired objects are present and match.
On its own that is the helm-expt F3 "silent false green": the workload cannot start.

The two install-receipt predicates now catch it from both ends, on the SAME install:
  * prerequisites-met  (#477) BLOCKs pre-flight  — the required Secret is absent.
  * workloads-converged (#476) BLOCKs at runtime — the pod is in CreateContainerConfigError.

The three install-receipt predicates plus receipt freshness (#478, via --ttl:
observedAt + expiresAt) now ship — so a workerless consumer can tell a fresh
green from a stale one.

object-set-diff (#496) adds the SET-LEVEL delta receipt on top, with one shape
for both day-1 and day-2:
  * drift   (--dry-from = current render): $DIFF_DRIFT_VERDICT — the applied set still matches live.
  * dry-run (--dry-from = changed image):  $DIFF_DRYRUN_VERDICT — the proposed image bump would touch the web Deployment (image.digest case, helm-expt#992).

Receipts saved under: ${RUN_DIR/#$REPO_ROOT\//}
EOF
else
cat <<EOF
Unexpected verdicts: object-set=$OS_VERDICT prerequisites=$PQ_VERDICT workloads=$WL_VERDICT.
Did ./setup.sh run, and was app-db-secret accidentally created?
EOF
fi
