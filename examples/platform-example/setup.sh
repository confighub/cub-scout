#!/bin/bash
# Platform Example Setup Script
# Deploys flux2-kustomize-helm-example + orphan resources for cub-scout demo

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${CYAN}"
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║           Platform Example Setup for cub-scout                ║"
echo "║                                                               ║"
echo "║  This will deploy:                                            ║"
echo "║  • Flux GitOps controllers                                    ║"
echo "║  • flux2-kustomize-helm-example (~28 resources)              ║"
echo "║  • Orphan resources for demo (~7 resources)                   ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

if ! command -v kubectl &> /dev/null; then
    echo -e "${RED}Error: kubectl not found${NC}"
    exit 1
fi

if ! command -v flux &> /dev/null; then
    echo -e "${RED}Error: flux CLI not found${NC}"
    echo "Install from: https://fluxcd.io/flux/installation/"
    exit 1
fi

if ! kubectl cluster-info &> /dev/null; then
    echo -e "${RED}Error: Cannot connect to Kubernetes cluster${NC}"
    echo "Make sure your kubeconfig is set up correctly"
    exit 1
fi

echo -e "${GREEN}✓ Prerequisites OK${NC}"
echo ""

# Check if Flux is already installed
if kubectl get namespace flux-system &> /dev/null; then
    echo -e "${YELLOW}Flux appears to be already installed.${NC}"
    read -p "Continue anyway? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 0
    fi
fi

# Step 1: Bootstrap Flux with the example repo
echo -e "${CYAN}Step 1: Bootstrapping Flux...${NC}"
echo ""

# Check if GITHUB_TOKEN is set for private repo access
if [ -z "$GITHUB_TOKEN" ]; then
    echo -e "${YELLOW}Note: GITHUB_TOKEN not set. Using public repo (read-only).${NC}"
    echo "For full demo with sync, export GITHUB_TOKEN first."
    echo ""
fi

# Install Flux components
echo "Installing Flux controllers..."
flux install

# Wait for Flux to be ready
echo "Waiting for Flux controllers to be ready..."
kubectl wait --for=condition=available --timeout=120s deployment/source-controller -n flux-system
kubectl wait --for=condition=available --timeout=120s deployment/kustomize-controller -n flux-system
kubectl wait --for=condition=available --timeout=120s deployment/helm-controller -n flux-system

echo -e "${GREEN}✓ Flux installed${NC}"
echo ""

# Step 2: Add the flux2-kustomize-helm-example source
echo -e "${CYAN}Step 2: Adding flux2-kustomize-helm-example...${NC}"

# Create GitRepository pointing to the example repo
cat <<EOF | kubectl apply -f -
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: flux-system
  namespace: flux-system
spec:
  interval: 1m
  url: https://github.com/fluxcd/flux2-kustomize-helm-example
  ref:
    branch: main
EOF

# Create Kustomization for the staging cluster config
cat <<EOF | kubectl apply -f -
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: flux-system
  namespace: flux-system
spec:
  interval: 10m
  sourceRef:
    kind: GitRepository
    name: flux-system
  path: ./clusters/staging
  prune: true
  wait: true
  timeout: 5m
EOF

echo "Waiting for Kustomization to reconcile (this may take a few minutes)..."
sleep 10

# Wait for the main kustomization
kubectl wait --for=condition=ready --timeout=300s kustomization/flux-system -n flux-system || true

echo -e "${GREEN}✓ flux2-kustomize-helm-example deployed${NC}"
echo ""

# Step 3: Deploy orphan resources
echo -e "${CYAN}Step 3: Deploying orphan resources for demo...${NC}"

kubectl apply -f "$SCRIPT_DIR/orphans.yaml"

echo -e "${GREEN}✓ Orphan resources deployed${NC}"
echo ""

# Step 4: Summary
echo -e "${CYAN}"
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║                    Setup Complete!                            ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

echo -e "Try these cub-scout commands:"
echo ""
echo -e "  ${GREEN}cub-scout map${NC}              # Interactive TUI"
echo -e "  ${GREEN}cub-scout map workloads${NC}    # See ownership"
echo -e "  ${GREEN}cub-scout map orphans${NC}      # Find shadow IT"
echo -e "  ${GREEN}cub-scout map status${NC}       # Quick health check"
echo -e "  ${GREEN}cub-scout trace deploy/podinfo -n podinfo${NC}  # Trace to Git"
echo ""

# Show current state
echo -e "${YELLOW}Current cluster state:${NC}"
echo ""
flux get all -A 2>/dev/null || echo "(flux resources still reconciling...)"
echo ""
echo -e "${YELLOW}Orphan resources:${NC}"
kubectl get deploy,svc,cm -l cub-scout-demo=orphan -A 2>/dev/null || echo "(orphans deployed)"
echo ""
