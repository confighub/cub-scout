#!/usr/bin/env bash
# Provision real controllers for disposable GitOps and demo acceptance clusters.
set -euo pipefail
: "${GITHUB_PATH:?Expected GitHub Actions path file}"
root=${RUNNER_TEMP:?Expected disposable runner directory}/scout-controllers
mkdir -p "$root"
curl --fail --silent --show-error --location --retry 3 --proto '=https' --proto-redir '=https' \
  https://github.com/fluxcd/flux2/releases/download/v2.9.6/flux_2.9.6_linux_amd64.tar.gz -o "$root/flux.tar.gz"
printf '%s  %s\n' b4d22673e9246cbd628881f1a9ef3b090085dced291e42d804555cee8e8d42c5 "$root/flux.tar.gz" | sha256sum --check --status
tar -xzf "$root/flux.tar.gz" -C "$root" flux
printf '%s\n' "$root" >> "$GITHUB_PATH"
"$root/flux" install
kubectl rollout status deployment/source-controller -n flux-system --timeout=300s
kubectl rollout status deployment/kustomize-controller -n flux-system --timeout=300s
kubectl rollout status deployment/helm-controller -n flux-system --timeout=300s
curl --fail --silent --show-error --location --retry 3 --proto '=https' --proto-redir '=https' \
  https://raw.githubusercontent.com/argoproj/argo-cd/v3.5.3/manifests/install.yaml -o "$root/argocd.yaml"
printf '%s  %s\n' 7efe2d6bbc03f63623640f1e4198f16c84009d510fb810ef71e56df1b7614ba9 "$root/argocd.yaml" | sha256sum --check --status
kubectl create namespace argocd
kubectl apply --server-side --force-conflicts -n argocd -f "$root/argocd.yaml"
kubectl wait --for=condition=Established crd/applications.argoproj.io --timeout=120s
kubectl rollout status deployment/argocd-server -n argocd --timeout=300s
kubectl rollout status deployment/argocd-repo-server -n argocd --timeout=300s
kubectl rollout status statefulset/argocd-application-controller -n argocd --timeout=300s
"$root/flux" version > "$root/flux-version.txt"
