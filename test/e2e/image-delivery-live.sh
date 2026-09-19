#!/usr/bin/env bash
# Creates only disposable test infrastructure; cub-scout invocations are read-only.
# Acceptance: real OCI pulls and Deployment execution PASS for both adapters;
# a one-pod cap is INCONCLUSIVE and an authored-field mutation is BLOCK.
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root"
for tool in docker kind kubectl flux oras jq go; do
  command -v "$tool" >/dev/null || { printf 'Missing tool: %s\n' "$tool" >&2; exit 1; }
done
out=$(mktemp -d "${TMPDIR:-/tmp}/scout-image-live.XXXXXX")
name="scout-image-$(date +%s)-$$"
registry="${name}-registry"
export KUBECONFIG="$out/kubeconfig"
created_cluster=false
created_registry=false
cleanup() {
  if $created_cluster; then kind delete cluster --name "$name"; fi
  if $created_registry; then docker rm -f "$registry" >/dev/null; fi
  printf 'Evidence retained: %s\n' "$out"
}
trap cleanup EXIT
printf 'Evidence directory: %s\n' "$out"
git rev-parse HEAD > "$out/source-commit.txt"
git diff > "$out/source.patch"
while IFS= read -r -d '' file; do
  result=0
  git diff --no-index /dev/null "$file" >> "$out/source.patch" || result=$?
  [[ $result -le 1 ]]
done < <(git ls-files --others --exclude-standard -z)
go build -o "$out/cub-scout" ./cmd/cub-scout
"$out/cub-scout" version > "$out/version.txt"
shasum -a 256 "$out/cub-scout" > "$out/binary.sha256"
flux version --client > "$out/flux-version.txt"
created_cluster=true
kind create cluster --name "$name" --image kindest/node:v1.35.0 --wait 120s
docker run -d --name "$registry" --network kind -p 127.0.0.1::5000 registry:2 > "$out/registry-id.txt"
created_registry=true
port=$(docker inspect "$registry" --format '{{(index (index .NetworkSettings.Ports "5000/tcp") 0).HostPort}}')
address=$(docker inspect "$registry" --format '{{(index .NetworkSettings.Networks "kind").IPAddress}}')
repo="$address:5000/config"
kubectl create namespace argocd
kubectl apply --server-side -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/v3.4.4/manifests/core-install.yaml
kubectl wait --for=condition=Established crd/appprojects.argoproj.io crd/applications.argoproj.io --timeout=60s
kubectl apply -f test/e2e/fixtures/image-proof-project.yaml
flux install --version=v2.8.6 --components=source-controller,kustomize-controller
kubectl -n argocd rollout status deployment/argocd-repo-server --timeout=240s
kubectl -n argocd rollout status statefulset/argocd-application-controller --timeout=240s
kubectl -n flux-system rollout status deployment/source-controller --timeout=240s
kubectl -n flux-system rollout status deployment/kustomize-controller --timeout=240s
kubectl get deployments,statefulsets -A -o json > "$out/controller-versions.json"
arch=$(kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.architecture}')
# Resolve a pinned index from the registry, not from pod status.
oras manifest fetch registry.k8s.io/e2e-test-images/agnhost@sha256:cc249acbd34692826b2b335335615e060fdb3c0bca4954507aa3a1d1194de253 > "$out/image-index.json"
image_digest=$(jq -er --arg arch "$arch" '[.manifests[] | select(.platform.os == "linux" and .platform.architecture == $arch)] | select(length == 1) | .[0].digest' "$out/image-index.json")

for adapter in argo flux; do
  ns="proof-$adapter"
  kubectl create namespace "$ns"
  cat > "$out/$adapter.yaml" <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: proof
  namespace: $ns
spec:
  replicas: 2
  selector:
    matchLabels:
      app: proof
  template:
    metadata:
      labels:
        app: proof
    spec:
      containers:
        - name: app
          image: registry.k8s.io/e2e-test-images/agnhost@$image_digest
          args: [netexec, --http-port=8080]
          ports:
            - containerPort: 8080
EOF
  reference=$(go run ./examples/oci-release-check/create-layout "$out/$adapter.yaml" "$out/$adapter-layout")
  digest=${reference##*@}
  oras cp --from-oci-layout --to-plain-http "$out/$adapter-layout@$digest" "127.0.0.1:$port/config/$adapter:proof"
  bundle="oci://$repo/$adapter@$digest"
  printf '%s\n' "$bundle" > "$out/$adapter-bundle.txt"
  if [[ $adapter == argo ]]; then
    kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: proof-registry
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
stringData:
  type: oci
  url: oci://$repo/argo
  insecureOCIForceHttp: "true"
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: proof
  namespace: argocd
spec:
  project: default
  source:
    repoURL: oci://$repo/argo
    targetRevision: $digest
    path: .
  destination:
    server: https://kubernetes.default.svc
    namespace: $ns
  syncPolicy:
    automated:
      selfHeal: false
EOF
    controller=Application/proof
    api=argoproj.io/v1alpha1
    control_ns=argocd
    kubectl -n argocd wait application/proof --for=jsonpath='{.status.sync.status}'=Synced --timeout=240s
  else
    kubectl apply -f - <<EOF
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: proof
  namespace: flux-system
spec:
  interval: 1h
  url: oci://$repo/flux
  insecure: true
  ref:
    digest: $digest
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: proof
  namespace: flux-system
spec:
  interval: 1h
  path: ./
  prune: false
  wait: true
  sourceRef:
    kind: OCIRepository
    name: proof
EOF
    controller=Kustomization/proof
    api=kustomize.toolkit.fluxcd.io/v1
    control_ns=flux-system
    kubectl -n flux-system wait kustomization/proof --for=condition=Ready --timeout=240s
  fi
  kubectl -n "$ns" rollout status deployment/proof --timeout=240s
  if [[ $adapter == argo ]]; then
    kubectl -n argocd wait application/proof --for=jsonpath='{.status.health.status}'=Healthy --timeout=240s
  fi
  # Let fixture status settle before testing a consistent observation. Do not
  # retry or overwrite an observer result to manufacture a passing report.
  previous=""
  stable=0
  for ((attempt = 0; attempt < 30; attempt++)); do
    revision=$(kubectl -n "$control_ns" get "$controller" -o jsonpath='{.metadata.resourceVersion}')
    if [[ $revision == "$previous" ]]; then stable=$((stable + 1)); else stable=0; fi
    if [[ $stable -ge 3 ]]; then break; fi
    previous=$revision
    sleep 1
  done
  [[ $stable -ge 3 ]] || { printf 'Controller fixture did not settle\n' >&2; exit 1; }
  args=(release check --bundle "$bundle" --oci-layout "$out/$adapter-layout"
    --controller "$controller" --api-version "$api" --controller-namespace "$control_ns"
    --kube-context "kind-$name" --check-running-image --format json --fail-on any-non-pass)
  check() {
    local scenario=$1 expected=$2 expected_exit=$3
    shift 3
    local result=0
    "$out/cub-scout" "${args[@]}" "$@" > "$out/$adapter-$scenario.json" 2> "$out/$adapter-$scenario.stderr" || result=$?
    printf '%s\n' "$result" > "$out/$adapter-$scenario.exit"
    jq '{verdict, requestCounts, stages: [.stages[] | {name, verdict}]}' "$out/$adapter-$scenario.json"
    [[ $result == "$expected_exit" ]]
    jq -e --arg expected "$expected" '.verdict == $expected' "$out/$adapter-$scenario.json" >/dev/null
  }
  check pass PASS 0
  bash examples/oci-release-check/validate-live.sh \
    --binary "$out/cub-scout" --bundle "$bundle" --oci-layout "$out/$adapter-layout" \
    --controller "$controller" --api-version "$api" --controller-namespace "$control_ns" \
    --kube-context "kind-$name" --expect-verdict PASS --out-dir "$out/operator-$adapter-pass"
  check capped INCONCLUSIVE 2 --max-pods 1
  bash examples/oci-release-check/validate-live.sh \
    --binary "$out/cub-scout" --bundle "$bundle" --oci-layout "$out/$adapter-layout" \
    --controller "$controller" --api-version "$api" --controller-namespace "$control_ns" \
    --kube-context "kind-$name" --max-pods 1 --expect-verdict INCONCLUSIVE \
    --out-dir "$out/operator-$adapter-capped"
  # Only this script's disposable workload is changed; the observer never writes.
  kubectl -n "$ns" patch deployment/proof --type=merge -p '{"spec":{"replicas":3}}'
  kubectl -n "$ns" rollout status deployment/proof --timeout=120s
  check drift BLOCK 2
  kubectl -n "$control_ns" get "$controller" -o json > "$out/$adapter-controller.json"
  kubectl -n "$ns" get deployment,replicaset,pod -o json > "$out/$adapter-workloads.json"
done
jq -s 'map({context, controller, bundle, startedAt, finishedAt, verdict,
  requestCounts, stages, runningImage: {verdict: .runningImage.verdict,
  reason: .runningImage.reason, podReads: .runningImage.podReads,
  coverage: [.runningImage.workloads[] | {id, verdict, reason, deployment}]}})' \
  "$out/argo-pass.json" "$out/argo-capped.json" "$out/argo-drift.json" \
  "$out/flux-pass.json" "$out/flux-capped.json" "$out/flux-drift.json" > "$out/summary.json"
printf 'Both real-controller acceptance lanes passed. No ConfigHub publication or TLS/auth registry proof is implied.\n'
