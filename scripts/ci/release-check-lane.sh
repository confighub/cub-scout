#!/usr/bin/env bash
# Run `release check` against a real Flux OCI delivery and check its answer.
#
# The lane pushes a literal bundle to the kind registry, lets a Flux
# OCIRepository and Kustomization deliver it, copies the bundle into an OCI
# layout (release check reads a plain-HTTP registry only through
# --oci-layout), and runs release check twice:
#   - with the bundle Flux delivered: verdict PASS, answer `matches`;
#   - with a second bundle Flux was never pointed at, holding the same
#     objects under another revision (so its manifest digest differs and the
#     live configuration still matches): the OCIRepository and Kustomization
#     both report another revision, so the controller stage is WATCH and the
#     answer `not-compared` / `controller-reports-other-revision` (the
#     maintainer's decision of 2026-11-19, as for Argo CD).
# The second case changes nothing in the cluster.
set -euo pipefail
scout=${1:?usage: release-check-lane.sh <cub-scout binary> <output dir>}
out=${2:?usage: release-check-lane.sh <cub-scout binary> <output dir>}
context=${KUBE_CONTEXT:-kind-gitops-cluster}
root=${RUNNER_TEMP:?Expected disposable runner directory}/scout-release-check
mkdir -p "$root" "$out"

# A literal bundle. Every namespaced object names its namespace, as release
# check requires.
bundle_dir() {
  local dir=$1
  mkdir -p "$dir"
  cat > "$dir/objects.yaml" <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: release-check
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rc-app
  namespace: release-check
spec:
  replicas: 1
  selector:
    matchLabels:
      app: rc-app
  template:
    metadata:
      labels:
        app: rc-app
    spec:
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10
EOF
}
bundle_dir "$root/bundle"

push() {
  local tag=$1 dir=$2
  flux push artifact "oci://localhost:5000/release-check/app:$tag" --path "$dir" \
    --source "https://github.com/confighub/cub-scout" --revision "$tag" --output json \
    | jq -er '.digest'
}
d1=$(push v1 "$root/bundle")
d2=$(push v2 "$root/bundle")
echo "v1 bundle: $d1"
echo "v2 bundle: $d2"
[ "$d1" != "$d2" ] || { echo "the two bundles have the same digest" >&2; exit 1; }

# Flux pulls the v1 bundle by digest from the registry on the kind network.
kubectl apply -f - <<EOF
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: rc-app
  namespace: flux-system
spec:
  interval: 1m
  url: oci://kind-registry:5000/release-check/app
  insecure: true
  ref:
    digest: $d1
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: rc-app
  namespace: flux-system
spec:
  interval: 1m
  sourceRef:
    kind: OCIRepository
    name: rc-app
  path: ./
  prune: true
  wait: true
  timeout: 3m
EOF
if ! kubectl wait --for=condition=Ready kustomization/rc-app -n flux-system --timeout=300s; then
  kubectl get ocirepository,kustomization -n flux-system rc-app -o yaml > "$out/flux-objects.yaml" || true
  exit 1
fi
kubectl get ocirepository,kustomization -n flux-system rc-app -o yaml > "$out/flux-objects.yaml"

# The layouts are read from the same registry, through its host port.
go run ./scripts/ci/oci-layout-copy "localhost:5000/release-check/app@$d1" "$root/layout"
go run ./scripts/ci/oci-layout-copy "localhost:5000/release-check/app@$d2" "$root/layout"

check() {
  local digest=$1 file=$2 code=0
  "$scout" release check --bundle "oci://kind-registry:5000/release-check/app@$digest" --oci-layout "$root/layout" \
    --controller Kustomization/rc-app --api-version kustomize.toolkit.fluxcd.io/v1 \
    --controller-namespace flux-system --kube-context "$context" --format json \
    > "$out/$file" 2> "$out/$file.err" || code=$?
  echo "$code" > "$out/$file.exit"
  jq -c '{verdict, answer: (.answer | {state, reason, summary, requests: .cost.requestsMade})}' "$out/$file"
}

check "$d1" delivered.json
jq -e '.verdict == "PASS" and .answer.state == "matches" and .answer.reason == "bundle-reported-and-matches"
       and .answer.cluster.context == "'"$context"'" and .answer.cost.requestsMade > 0
       and ([.answer.unknowns[] | select(.what == "application-success")] | length == 1)' "$out/delivered.json" \
  || { jq '{verdict, stages, answer}' "$out/delivered.json"; exit 1; }

check "$d2" other-bundle.json
jq -e '.verdict == "WATCH" and .answer.state == "not-compared" and .answer.reason == "controller-reports-other-revision"
       and ([.answer.basis[] | select(.claim == "the controller stage is WATCH")] | length == 1)
       and ([.answer.basis[] | select(.claim == "the configuration stage is PASS")] | length == 1)' "$out/other-bundle.json" \
  || { jq '{verdict, stages, answer}' "$out/other-bundle.json"; exit 1; }
