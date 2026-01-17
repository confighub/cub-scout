#!/bin/bash
# setup-multi-tool-cluster.sh - Create a kind cluster with Flux, Argo CD, and test workloads
#
# Usage:
#   ./setup-multi-tool-cluster.sh [cluster-name]
#
# This script creates a comprehensive test environment for TUI E2E testing:
# - Kind cluster with Flux CD installed
# - Argo CD installed
# - Test workloads from multiple sources (Flux, Argo, Helm, Native, ConfigHub)
#
# Requirements:
#   - kind, kubectl, flux, helm installed
#   - Docker running

set -euo pipefail

CLUSTER_NAME="${1:-tui-e2e}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() { echo -e "${BLUE}==>${NC} $1"; }
success() { echo -e "${GREEN}✓${NC} $1"; }
warn() { echo -e "${YELLOW}⚠${NC} $1"; }
error() { echo -e "${RED}✗${NC} $1"; }

# Check prerequisites
check_prereqs() {
    log "Checking prerequisites..."
    local missing=()

    command -v kind >/dev/null 2>&1 || missing+=("kind")
    command -v kubectl >/dev/null 2>&1 || missing+=("kubectl")
    command -v flux >/dev/null 2>&1 || missing+=("flux")
    command -v helm >/dev/null 2>&1 || missing+=("helm")
    command -v docker >/dev/null 2>&1 || missing+=("docker")

    if [[ ${#missing[@]} -gt 0 ]]; then
        error "Missing required tools: ${missing[*]}"
        echo "Install with:"
        echo "  brew install kind kubectl fluxcd/tap/flux helm"
        exit 1
    fi

    if ! docker info >/dev/null 2>&1; then
        error "Docker is not running"
        exit 1
    fi

    success "All prerequisites met"
}

# Create kind cluster
create_cluster() {
    log "Creating kind cluster '$CLUSTER_NAME'..."

    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        warn "Cluster '$CLUSTER_NAME' already exists"
        kubectl config use-context "kind-${CLUSTER_NAME}"
        return 0
    fi

    cat <<EOF | kind create cluster --name "$CLUSTER_NAME" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
    extraPortMappings:
      - containerPort: 80
        hostPort: 80
        protocol: TCP
      - containerPort: 443
        hostPort: 443
        protocol: TCP
EOF

    kubectl config use-context "kind-${CLUSTER_NAME}"
    success "Cluster created"
}

# Install Flux CD
install_flux() {
    log "Installing Flux CD..."

    if kubectl get namespace flux-system >/dev/null 2>&1; then
        warn "Flux already installed"
        return 0
    fi

    flux install --components-extra=image-reflector-controller,image-automation-controller

    # Wait for Flux to be ready
    kubectl wait --for=condition=Ready pods -l app=source-controller -n flux-system --timeout=120s
    kubectl wait --for=condition=Ready pods -l app=kustomize-controller -n flux-system --timeout=120s

    success "Flux installed"
}

# Install Argo CD
install_argocd() {
    log "Installing Argo CD..."

    if kubectl get namespace argocd >/dev/null 2>&1; then
        warn "Argo CD already installed"
        return 0
    fi

    kubectl create namespace argocd
    kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

    # Wait for Argo CD to be ready
    kubectl wait --for=condition=Ready pods -l app.kubernetes.io/name=argocd-server -n argocd --timeout=180s

    success "Argo CD installed"
}

# Deploy Flux-managed workloads
deploy_flux_workloads() {
    log "Deploying Flux-managed workloads..."

    # Create namespace
    kubectl create namespace flux-demo --dry-run=client -o yaml | kubectl apply -f -

    # GitRepository source
    cat <<EOF | kubectl apply -f -
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: podinfo
  namespace: flux-demo
spec:
  interval: 1m
  url: https://github.com/stefanprodan/podinfo
  ref:
    tag: 6.5.0
EOF

    # Kustomization
    cat <<EOF | kubectl apply -f -
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: podinfo
  namespace: flux-demo
spec:
  interval: 5m
  path: ./kustomize
  prune: true
  sourceRef:
    kind: GitRepository
    name: podinfo
  targetNamespace: flux-demo
EOF

    # Wait for reconciliation
    kubectl wait --for=condition=Ready kustomization/podinfo -n flux-demo --timeout=120s 2>/dev/null || true

    success "Flux workloads deployed"
}

# Deploy Argo CD-managed workloads
deploy_argocd_workloads() {
    log "Deploying Argo CD-managed workloads..."

    # Create namespace
    kubectl create namespace argo-demo --dry-run=client -o yaml | kubectl apply -f -

    # Argo Application
    cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: guestbook
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/argoproj/argocd-example-apps.git
    targetRevision: HEAD
    path: guestbook
  destination:
    server: https://kubernetes.default.svc
    namespace: argo-demo
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
EOF

    success "Argo CD workloads deployed"
}

# Deploy Helm-managed workloads
deploy_helm_workloads() {
    log "Deploying Helm-managed workloads..."

    # Create namespace
    kubectl create namespace helm-demo --dry-run=client -o yaml | kubectl apply -f -

    # Add bitnami repo
    helm repo add bitnami https://charts.bitnami.com/bitnami 2>/dev/null || true
    helm repo update

    # Install nginx via Helm
    if ! helm list -n helm-demo | grep -q nginx; then
        helm install nginx bitnami/nginx \
            --namespace helm-demo \
            --set service.type=ClusterIP \
            --wait --timeout 120s
    else
        warn "Helm nginx already installed"
    fi

    success "Helm workloads deployed"
}

# Deploy Native (kubectl) workloads
deploy_native_workloads() {
    log "Deploying Native workloads..."

    # Create namespace
    kubectl create namespace native-demo --dry-run=client -o yaml | kubectl apply -f -

    # Deployment without GitOps labels
    cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mystery-app
  namespace: native-demo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mystery-app
  template:
    metadata:
      labels:
        app: mystery-app
    spec:
      containers:
        - name: nginx
          image: nginx:alpine
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: mystery-app
  namespace: native-demo
spec:
  selector:
    app: mystery-app
  ports:
    - port: 80
      targetPort: 80
EOF

    success "Native workloads deployed"
}

# Deploy ConfigHub-labeled workloads
deploy_confighub_workloads() {
    log "Deploying ConfigHub-labeled workloads..."

    # Create namespace
    kubectl create namespace confighub-demo --dry-run=client -o yaml | kubectl apply -f -

    # Deployment with ConfigHub labels
    cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payment-api
  namespace: confighub-demo
  labels:
    confighub.com/UnitSlug: payment-api
  annotations:
    confighub.com/SpaceName: payments-prod
    confighub.com/RevisionNum: "42"
spec:
  replicas: 2
  selector:
    matchLabels:
      app: payment-api
  template:
    metadata:
      labels:
        app: payment-api
        confighub.com/UnitSlug: payment-api
    spec:
      containers:
        - name: api
          image: nginx:alpine
          ports:
            - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: payment-api
  namespace: confighub-demo
  labels:
    confighub.com/UnitSlug: payment-api
spec:
  selector:
    app: payment-api
  ports:
    - port: 8080
      targetPort: 8080
EOF

    success "ConfigHub workloads deployed"
}

# Print cluster status
print_status() {
    log "Cluster Status"
    echo ""
    echo "Context: kind-${CLUSTER_NAME}"
    echo ""

    echo "Namespaces:"
    kubectl get namespaces | grep -E "flux-|argo|helm-|native-|confighub-" || true
    echo ""

    echo "Flux Controllers:"
    kubectl get pods -n flux-system --no-headers 2>/dev/null | awk '{print "  " $1 " " $3}' || echo "  (not installed)"
    echo ""

    echo "Argo CD Controllers:"
    kubectl get pods -n argocd --no-headers 2>/dev/null | awk '{print "  " $1 " " $3}' || echo "  (not installed)"
    echo ""

    echo "Workloads by Owner:"
    echo "  Flux:      $(kubectl get deploy -A -l kustomize.toolkit.fluxcd.io/name --no-headers 2>/dev/null | wc -l | tr -d ' ') deployments"
    echo "  ArgoCD:    $(kubectl get deploy -A -l argocd.argoproj.io/instance --no-headers 2>/dev/null | wc -l | tr -d ' ') deployments"
    echo "  Helm:      $(kubectl get deploy -A -l app.kubernetes.io/managed-by=Helm --no-headers 2>/dev/null | wc -l | tr -d ' ') deployments"
    echo "  ConfigHub: $(kubectl get deploy -A -l confighub.com/UnitSlug --no-headers 2>/dev/null | wc -l | tr -d ' ') deployments"
    echo ""

    success "Cluster ready for TUI E2E tests"
    echo ""
    echo "Run: cub-agent map"
}

# Main
main() {
    echo ""
    echo "=========================================="
    echo "  TUI E2E Multi-Tool Cluster Setup"
    echo "=========================================="
    echo ""

    check_prereqs
    create_cluster
    install_flux
    install_argocd
    deploy_flux_workloads
    deploy_argocd_workloads
    deploy_helm_workloads
    deploy_native_workloads
    deploy_confighub_workloads
    print_status
}

main "$@"
