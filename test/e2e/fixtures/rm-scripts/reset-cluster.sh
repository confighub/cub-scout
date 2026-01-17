#!/usr/bin/env bash
set -euo pipefail

# Reset cluster state between test runs
# Force-deletes resources for speed

echo "=== Resetting Cluster State ==="

# System namespaces to preserve
SYSTEM_NS="default kube-system kube-public kube-node-lease flux-system argocd gitea local-path-storage"

# First, delete Flux/Argo CRs quickly (without waiting for reconciliation)
if kubectl get crd kustomizations.kustomize.toolkit.fluxcd.io &>/dev/null; then
    echo "Deleting Flux CRs..."
    kubectl delete kustomizations -n flux-system --all --wait=false 2>/dev/null || true
    kubectl delete helmreleases -A --all --wait=false 2>/dev/null || true
    kubectl delete gitrepositories -n flux-system --all --wait=false 2>/dev/null || true
    kubectl delete helmrepositories -n flux-system --all --wait=false 2>/dev/null || true
fi

if kubectl get crd applications.argoproj.io &>/dev/null; then
    echo "Deleting Argo CD CRs..."
    kubectl delete applicationsets -n argocd --all --wait=false 2>/dev/null || true
    kubectl delete applications -n argocd --all --wait=false 2>/dev/null || true
fi

# Force-delete application namespaces
echo "Force-deleting application namespaces..."
for ns in $(kubectl get namespaces -o jsonpath='{.items[*].metadata.name}'); do
    if ! echo "$SYSTEM_NS" | grep -qw "$ns"; then
        echo "  Deleting: $ns"
        # Remove finalizers and delete
        kubectl patch namespace "$ns" -p '{"metadata":{"finalizers":[]}}' --type=merge 2>/dev/null || true
        kubectl delete namespace "$ns" --grace-period=0 --force 2>/dev/null || true
    fi
done

# Clean up any stuck HelmReleases by removing finalizers
echo "Cleaning up stuck HelmReleases..."
for hr in $(kubectl get helmreleases -A -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{"\n"}{end}' 2>/dev/null); do
    [[ -z "$hr" ]] && continue
    ns="${hr%%/*}"
    name="${hr##*/}"
    kubectl patch helmrelease "$name" -n "$ns" -p '{"metadata":{"finalizers":[]}}' --type=merge 2>/dev/null || true
done

# Wait briefly for cleanup
sleep 2

echo ""
echo "=== Reset Complete ==="
kubectl get ns | grep -v "STATUS"
