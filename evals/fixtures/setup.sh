#!/usr/bin/env bash
# Apply evals/fixtures/scenario.yaml the way each owner would write it, so
# managedFields carry the owner's real field manager (the names cub-scout
# matches in pkg/agent/manager_strings.go). Hand edits made afterwards
# (evals/fixtures/incident.sh) are then distinguishable from delivery.
#
# Usage: evals/fixtures/setup.sh <kube-context>
set -euo pipefail
ctx="${1:?usage: setup.sh <kube-context>}"
dir="$(cd "$(dirname "$0")" && pwd)"
apply() { kubectl --context "$ctx" apply -f "$dir/scenario.yaml" "$@" >/dev/null; }

# Namespaces and unmanaged workloads: plain kubectl, as a person would.
apply -l '!kustomize.toolkit.fluxcd.io/name,!argocd.argoproj.io/instance,!app.kubernetes.io/managed-by,!confighub.com/UnitSlug'
# Flux Kustomization.
apply -l kustomize.toolkit.fluxcd.io/name --server-side --field-manager=kustomize-controller
# Argo CD Application.
apply -l argocd.argoproj.io/instance --server-side --field-manager=argocd-controller
# Helm release.
apply -l app.kubernetes.io/managed-by=Helm --server-side --field-manager=helm
# ConfigHub unit, delivered by Argo CD.
apply -l confighub.com/UnitSlug --server-side --field-manager=argocd-controller
