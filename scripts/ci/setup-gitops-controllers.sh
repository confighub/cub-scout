#!/usr/bin/env bash
# Provision real controllers for disposable GitOps and demo acceptance clusters.
set -euo pipefail
: "${GITHUB_PATH:?Expected GitHub Actions path file}"
root=${RUNNER_TEMP:?Expected disposable runner directory}/scout-controllers
mkdir -p "$root"
curl --fail --silent --show-error --location --retry 3 --proto '=https' --proto-redir '=https' \
  https://github.com/fluxcd/flux2/releases/download/v2.10.0/flux_2.10.0_linux_amd64.tar.gz -o "$root/flux.tar.gz"
printf '%s  %s\n' 208dd7012c04fedeed5b2d0b6e63df3b15b1c34a381527811685c098bc9f8659 "$root/flux.tar.gz" | sha256sum --check --status
tar -xzf "$root/flux.tar.gz" -C "$root" flux
printf '%s\n' "$root" >> "$GITHUB_PATH"
"$root/flux" install
kubectl rollout status deployment/source-controller -n flux-system --timeout=300s
kubectl rollout status deployment/kustomize-controller -n flux-system --timeout=300s
kubectl rollout status deployment/helm-controller -n flux-system --timeout=300s
curl --fail --silent --show-error --location --retry 3 --proto '=https' --proto-redir '=https' \
  https://raw.githubusercontent.com/argoproj/argo-cd/v3.6.1/manifests/install.yaml -o "$root/argocd.yaml"
printf '%s  %s\n' 7be23ca7d9ca3d3074dee8332f1ca92a933b990cee7c9cf22cf3843ee8c2913e "$root/argocd.yaml" | sha256sum --check --status
kubectl create namespace argocd
kubectl apply --server-side --force-conflicts -n argocd -f "$root/argocd.yaml"
kubectl wait --for=condition=Established crd/applications.argoproj.io --timeout=120s
kubectl rollout status deployment/argocd-server -n argocd --timeout=300s
kubectl rollout status deployment/argocd-repo-server -n argocd --timeout=300s
kubectl rollout status statefulset/argocd-application-controller -n argocd --timeout=300s
"$root/flux" version > "$root/flux-version.txt"
