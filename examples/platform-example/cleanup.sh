#!/bin/bash
# Platform Example Cleanup Script
# Removes all resources deployed by setup.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${CYAN}"
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║           Platform Example Cleanup                            ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Check prerequisites
if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}Error: kubectl not found${NC}"
    exit 1
fi

if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}Error: Cannot connect to Kubernetes cluster${NC}"
    exit 1
fi

# Confirm cleanup
echo -e "${YELLOW}This will remove:${NC}"
echo "  • Flux controllers and CRDs"
echo "  • All resources from flux2-kustomize-helm-example"
echo "  • Orphan demo resources"
echo ""
read -p "Continue with cleanup? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Cleanup cancelled."
    exit 0
fi

echo ""

# Step 1: Remove orphan resources
echo -e "${CYAN}Step 1: Removing orphan resources...${NC}"
kubectl delete -f "$SCRIPT_DIR/orphans.yaml" --ignore-not-found=true 2>/dev/null || true
echo -e "${GREEN}✓ Orphan resources removed${NC}"
echo ""

# Step 2: Remove Flux managed resources
echo -e "${CYAN}Step 2: Removing Flux managed resources...${NC}"

# Delete Kustomizations first (they manage other resources)
kubectl delete kustomization --all -n flux-system --ignore-not-found=true 2>/dev/null || true

# Wait for reconciliation to stop
sleep 5

# Delete GitRepositories
kubectl delete gitrepository --all -n flux-system --ignore-not-found=true 2>/dev/null || true

# Delete HelmReleases
kubectl delete helmrelease --all -A --ignore-not-found=true 2>/dev/null || true

# Delete HelmRepositories
kubectl delete helmrepository --all -A --ignore-not-found=true 2>/dev/null || true

echo -e "${GREEN}✓ Flux managed resources removed${NC}"
echo ""

# Step 3: Uninstall Flux
echo -e "${CYAN}Step 3: Uninstalling Flux...${NC}"
if command -v flux &> /dev/null; then
    flux uninstall --silent 2>/dev/null || true
else
    # Manual cleanup if flux CLI not available
    kubectl delete namespace flux-system --ignore-not-found=true 2>/dev/null || true
fi
echo -e "${GREEN}✓ Flux uninstalled${NC}"
echo ""

# Step 4: Clean up namespaces created by the example
echo -e "${CYAN}Step 4: Cleaning up namespaces...${NC}"
for ns in podinfo monitoring; do
    if kubectl get namespace "$ns" &> /dev/null; then
        kubectl delete namespace "$ns" --ignore-not-found=true 2>/dev/null || true
        echo "  Deleted namespace: $ns"
    fi
done
echo -e "${GREEN}✓ Namespaces cleaned up${NC}"
echo ""

# Summary
echo -e "${CYAN}"
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║                    Cleanup Complete!                          ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

echo "Your cluster has been cleaned up."
echo ""
echo "To delete the kind cluster entirely:"
echo -e "  ${GREEN}kind delete cluster --name platform-demo${NC}"
echo ""
