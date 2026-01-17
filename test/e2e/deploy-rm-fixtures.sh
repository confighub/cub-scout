#!/bin/bash
# deploy-rm-fixtures.sh - Deploy RM pattern fixtures for TUI testing
#
# Usage:
#   ./deploy-rm-fixtures.sh [flux|argo|both] [env]
#
# Examples:
#   ./deploy-rm-fixtures.sh flux dev      # Deploy Flux fixtures for dev
#   ./deploy-rm-fixtures.sh argo dev      # Deploy Argo fixtures for dev
#   ./deploy-rm-fixtures.sh both dev      # Deploy both

set -euo pipefail

PATTERN="${1:-both}"
ENV="${2:-dev}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIXTURES_DIR="$SCRIPT_DIR/fixtures"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}==>${NC} $1"; }
success() { echo -e "${GREEN}✓${NC} $1"; }

deploy_flux() {
    log "Deploying Flux RM fixtures (env: $ENV)..."

    # Check if Flux is installed
    if ! kubectl get namespace flux-system >/dev/null 2>&1; then
        echo "Error: Flux not installed. Run setup-multi-tool-cluster.sh first"
        exit 1
    fi

    # Create namespace for RM fixtures
    kubectl create namespace rm-flux-$ENV --dry-run=client -o yaml | kubectl apply -f -

    # Apply base sources
    if [[ -d "$FIXTURES_DIR/flux-helm-kustomize/base/sources" ]]; then
        log "Applying Flux sources..."
        kubectl apply -k "$FIXTURES_DIR/flux-helm-kustomize/base/sources" -n rm-flux-$ENV 2>/dev/null || true
    fi

    # Apply overlay for environment
    if [[ -d "$FIXTURES_DIR/flux-helm-kustomize/overlays/$ENV" ]]; then
        log "Applying Flux overlay for $ENV..."
        # Apply each group
        for group in core security observability operations; do
            if [[ -d "$FIXTURES_DIR/flux-helm-kustomize/overlays/$ENV/$group" ]]; then
                kubectl apply -k "$FIXTURES_DIR/flux-helm-kustomize/overlays/$ENV/$group" -n rm-flux-$ENV 2>/dev/null || true
            fi
        done
    fi

    success "Flux RM fixtures deployed"
}

deploy_argo() {
    log "Deploying Argo RM fixtures (env: $ENV)..."

    # Check if Argo is installed
    if ! kubectl get namespace argocd >/dev/null 2>&1; then
        echo "Error: Argo CD not installed. Run setup-multi-tool-cluster.sh first"
        exit 1
    fi

    # Create namespace for RM fixtures
    kubectl create namespace rm-argo-$ENV --dry-run=client -o yaml | kubectl apply -f -

    # Apply ApplicationSet
    if [[ -f "$FIXTURES_DIR/argo-umbrella-charts/clusters/applicationset.yaml" ]]; then
        log "Applying Argo ApplicationSet..."
        kubectl apply -f "$FIXTURES_DIR/argo-umbrella-charts/clusters/applicationset.yaml" -n argocd 2>/dev/null || true
    fi

    success "Argo RM fixtures deployed"
}

# Main
case "$PATTERN" in
    flux)
        deploy_flux
        ;;
    argo)
        deploy_argo
        ;;
    both)
        deploy_flux
        deploy_argo
        ;;
    *)
        echo "Usage: $0 [flux|argo|both] [env]"
        exit 1
        ;;
esac

log "RM fixtures deployed. Run 'cub-agent map' to see them."
