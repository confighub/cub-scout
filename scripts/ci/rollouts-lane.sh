#!/usr/bin/env bash
# Install Argo Rollouts, create a real argoproj.io/v1alpha1 Rollout linked to
# the live Argo CD Application/guestbook, and prove `cub-scout landed` with
# `--verify-pod-images` against real controller status and running Pod imageIDs:
#   1. Healthy Rollout + matching running Pod image -> landed
#   2. Mismatched --image -> not-landed (workload-failed / workload)
#   3. Paused canary Rollout -> converging (workload-rolling-out)
set -euo pipefail

scout="${1:?usage: rollouts-lane.sh <cub-scout-binary> <output-dir>}"
out="${2:?usage: rollouts-lane.sh <cub-scout-binary> <output-dir>}"
root="${RUNNER_TEMP:-/tmp}/scout-rollouts"
mkdir -p "$root" "$out"

ctx="$(kubectl config current-context)"
echo "rollouts-lane: context=$ctx"

# 1. Install pinned Argo Rollouts v1.8.3 (SHA-256 verified).
curl --fail --silent --show-error --location --retry 3 --proto '=https' --proto-redir '=https' \
  https://github.com/argoproj/argo-rollouts/releases/download/v1.8.3/install.yaml -o "$root/argo-rollouts-install.yaml"
printf '%s  %s\n' 6f4f581dab41450ca623a03d246156fa7fa79901f7f01693c5df66f7557fc145 "$root/argo-rollouts-install.yaml" | sha256sum --check --status

kubectl create namespace argo-rollouts --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -n argo-rollouts -f "$root/argo-rollouts-install.yaml"
kubectl rollout status deployment/argo-rollouts -n argo-rollouts --timeout=300s
kubectl wait --for=condition=Established crd/rollouts.argoproj.io --timeout=120s

# 2. Read the live Argo CD Application/guestbook synced revision as our anchor.
argo_rev="$(kubectl get applications.argoproj.io guestbook -n argocd -o jsonpath='{.status.sync.revision}')"
if [[ -z "$argo_rev" ]]; then
  echo "ERROR: Argo CD Application/guestbook in argocd has no status.sync.revision" >&2
  exit 1
fi
echo "rollouts-lane: Argo CD revision=$argo_rev"

# 3. Create a real argoproj.io/v1alpha1 Rollout with canary pause step.
kubectl create namespace rollouts-lane --dry-run=client -o yaml | kubectl apply -f -
cat <<'EOF' | kubectl apply -n rollouts-lane -f -
apiVersion: argoproj.io/v1alpha1
kind: Rollout
metadata:
  name: lane-web
  labels:
    argocd.argoproj.io/instance: guestbook
spec:
  replicas: 1
  revisionHistoryLimit: 2
  selector:
    matchLabels:
      app: lane-web
  template:
    metadata:
      labels:
        app: lane-web
      annotations:
        rollout-lane/rev: "1"
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.9
  strategy:
    canary:
      steps:
        - setWeight: 50
        - pause: {}
EOF

# Wait for initial rollout to become Healthy with observedGeneration == generation.
healthy=""
for _ in $(seq 1 90); do
  phase="$(kubectl get rollouts.argoproj.io lane-web -n rollouts-lane -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  gen="$(kubectl get rollouts.argoproj.io lane-web -n rollouts-lane -o jsonpath='{.metadata.generation}' 2>/dev/null || true)"
  obs_gen="$(kubectl get rollouts.argoproj.io lane-web -n rollouts-lane -o jsonpath='{.status.observedGeneration}' 2>/dev/null || true)"
  if [[ "$phase" == "Healthy" && -n "$gen" && "$obs_gen" == "$gen" ]]; then
    healthy="yes"
    break
  fi
  sleep 2
done

kubectl get rollouts.argoproj.io lane-web -n rollouts-lane -o json > "$out/rollout-healthy.json" 2>&1 || true
if [[ -z "$healthy" ]]; then
  echo "ERROR: Rollout/lane-web did not reach Healthy with matching observedGeneration" >&2
  cat "$out/rollout-healthy.json" >&2
  kubectl logs deployment/argo-rollouts -n argo-rollouts --tail=60 > "$out/argo-rollouts.log" 2>&1 || true
  tail -n 40 "$out/argo-rollouts.log" >&2 || true
  exit 1
fi

# Wait for a Running pod with populated containerStatuses[0].imageID.
pod_image_id=""
for _ in $(seq 1 30); do
  pod_image_id="$(kubectl get pods -n rollouts-lane -l app=lane-web --field-selector=status.phase=Running -o jsonpath='{.items[0].status.containerStatuses[0].imageID}' 2>/dev/null || true)"
  [[ -n "$pod_image_id" ]] && break
  sleep 2
done
if [[ -z "$pod_image_id" ]]; then
  echo "ERROR: no running pod imageID found for Rollout/lane-web in rollouts-lane" >&2
  kubectl get pods -n rollouts-lane -o wide >&2 || true
  exit 1
fi
pod_digest="${pod_image_id##*@}"
echo "rollouts-lane: pod image digest=$pod_digest"

# Assertion 1: Healthy Rollout + matching running Pod image -> landed.
"$scout" landed ro/lane-web -n rollouts-lane --kube-context "$ctx" --revision "$argo_rev" --verify-pod-images --image "$pod_digest" --format json > "$out/rollout-healthy-landed.json"
jq -e '.answer.state == "landed" and .answer.cost.requestsMade > 0 and (.answer.basis | map(select(.claim | startswith("pod-images"))) | length > 0)' "$out/rollout-healthy-landed.json" >/dev/null

# Assertion 2: Mismatched --image -> not-landed (workload-failed / workload).
"$scout" landed ro/lane-web -n rollouts-lane --kube-context "$ctx" --revision "$argo_rev" --verify-pod-images --image sha256:0000000000000000000000000000000000000000000000000000000000000000 --format json > "$out/rollout-image-mismatch.json"
jq -e '.answer.state == "not-landed" and .answer.reason == "workload-failed" and .brokenLink.stage == "workload"' "$out/rollout-image-mismatch.json" >/dev/null

# Assertion 3: Trigger canary pause step -> Paused Rollout -> converging.
kubectl patch rollouts.argoproj.io lane-web -n rollouts-lane --type=merge \
  -p '{"spec":{"template":{"metadata":{"annotations":{"rollout-lane/rev":"2"}}}}}'

paused=""
for _ in $(seq 1 60); do
  phase="$(kubectl get rollouts.argoproj.io lane-web -n rollouts-lane -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  if [[ "$phase" == "Paused" ]]; then
    paused="yes"
    break
  fi
  sleep 2
done

kubectl get rollouts.argoproj.io lane-web -n rollouts-lane -o json > "$out/rollout-paused.json" 2>&1 || true
if [[ -z "$paused" ]]; then
  echo "ERROR: Rollout/lane-web did not reach Paused phase after canary patch" >&2
  cat "$out/rollout-paused.json" >&2
  kubectl logs deployment/argo-rollouts -n argo-rollouts --tail=60 > "$out/argo-rollouts-paused.log" 2>&1 || true
  tail -n 40 "$out/argo-rollouts-paused.log" >&2 || true
  exit 1
fi

"$scout" landed ro/lane-web -n rollouts-lane --kube-context "$ctx" --revision "$argo_rev" --verify-pod-images --image "$pod_digest" --format json > "$out/rollout-paused-converging.json"
jq -e '.answer.state == "converging" and .answer.reason == "workload-rolling-out"' "$out/rollout-paused-converging.json" >/dev/null

echo "rollouts-lane: PASS (Argo Rollouts Healthy->landed, mismatched --image->not-landed, Paused->converging verified)"
