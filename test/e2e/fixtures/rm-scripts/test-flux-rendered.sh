#!/usr/bin/env bash
set -euo pipefail

# Test script for the "rendered" approach (pre-rendered YAML manifests)
# This validates that rendered manifests deploy correctly via Flux Kustomizations

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

# Source shared test functions
source "${SCRIPT_DIR}/lib-test.sh"

# Configuration
FLUX_NAMESPACE="flux-system"
ENVIRONMENT="${1:-dev}"
TIMEOUT_KUSTOMIZATIONS="${TIMEOUT_KUSTOMIZATIONS:-600}"  # 10 minutes

# Git server configuration (for local testing with Gitea)
GIT_SERVER_URL="${GIT_SERVER_URL:-http://gitea-http.gitea.svc:3000}"
GIT_REPO_NAME="${GIT_REPO_NAME:-rendered}"
GIT_USER="${GIT_USER:-gitea_admin}"
GIT_PASSWORD="${GIT_PASSWORD:-admin123}"

usage() {
    echo "Usage: $0 [environment]"
    echo ""
    echo "Tests the 'rendered' manifest approach with Flux Kustomizations"
    echo ""
    echo "Arguments:"
    echo "  environment    Environment to test (dev, staging, production). Default: dev"
    echo ""
    echo "Environment variables:"
    echo "  TIMEOUT_KUSTOMIZATIONS  Timeout for Kustomizations to be ready (default: 600s)"
    echo "  SKIP_APPLY              Skip applying manifests, just validate (default: false)"
    echo "  SKIP_RENDER             Skip re-rendering manifests (default: false)"
    echo "  SKIP_GIT_PUSH           Skip pushing to git server (default: false)"
    echo "  GIT_SERVER_URL          Git server URL (default: http://gitea-http.gitea.svc:3000)"
    echo "  GIT_USER                Git username (default: gitea_admin)"
    echo "  GIT_PASSWORD            Git password (default: admin123)"
    echo ""
    echo "Examples:"
    echo "  $0                      # Test dev environment"
    echo "  $0 staging              # Test staging environment"
    echo "  SKIP_RENDER=true $0     # Skip rendering, just deploy existing manifests"
    exit 1
}

# Parse arguments
if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
fi

render_manifests() {
    log_header "Rendering Manifests"

    local render_script="${SCRIPT_DIR}/flux-render.sh"

    if [[ ! -x "$render_script" ]]; then
        log_error "Render script not found or not executable: $render_script"
        return 1
    fi

    log_info "Running: $render_script $ENVIRONMENT"

    if "$render_script" "$ENVIRONMENT"; then
        log_success "Manifests rendered successfully"
        return 0
    else
        log_error "Failed to render manifests"
        return 1
    fi
}

push_to_git() {
    log_header "Pushing to Git Server"

    cd "${REPO_ROOT}/rendered"

    # Check if git is initialized
    if [[ ! -d .git ]]; then
        log_info "Initializing git repository..."
        git init
        git config user.email "test@example.com"
        git config user.name "Test User"
    fi

    # Check if Gitea is accessible via NodePort
    if ! curl -s -o /dev/null -w "%{http_code}" "http://localhost:3030" 2>/dev/null | grep -q "200\|302"; then
        log_error "Gitea not accessible at localhost:3030"
        return 1
    fi

    # Configure remote
    git remote remove gitea 2>/dev/null || true
    git remote add gitea "http://${GIT_USER}:${GIT_PASSWORD}@localhost:3030/${GIT_USER}/${GIT_REPO_NAME}.git"

    # Commit and push
    git add -A
    git commit -m "Update rendered manifests for ${ENVIRONMENT}" 2>/dev/null || log_info "No changes to commit"

    log_info "Pushing to Gitea..."
    if git push -f gitea main 2>&1; then
        log_success "Pushed to git server"
        return 0
    else
        log_error "Failed to push to git server"
        return 1
    fi
}

reconcile_flux() {
    log_header "Reconciling Flux Resources"

    local timestamp=$(date +%s)

    # Trigger GitRepository reconciliation (async via annotation)
    log_info "Triggering GitRepository reconciliation..."
    if kubectl annotate --overwrite gitrepository/flux-rendered -n "$FLUX_NAMESPACE" \
        "reconcile.fluxcd.io/requestedAt=$timestamp" 2>/dev/null; then
        log_success "GitRepository reconciliation triggered"
    else
        log_warning "Failed to trigger GitRepository reconciliation (may not exist yet)"
    fi

    # Trigger reconciliation of all kustomizations (async via annotation)
    log_info "Triggering Kustomization reconciliation..."
    local count=0
    for ks in $(kubectl get kustomizations -n "$FLUX_NAMESPACE" -o name 2>/dev/null); do
        kubectl annotate --overwrite "$ks" -n "$FLUX_NAMESPACE" \
            "reconcile.fluxcd.io/requestedAt=$timestamp" 2>/dev/null
        count=$((count + 1))
    done
    log_success "Triggered reconciliation for $count Kustomizations"
}

main() {
    local start_time=$(date +%s)

    log_header "Testing Rendered Mode (Kustomization)"
    log_info "Environment: $ENVIRONMENT"
    log_info "Timeout: ${TIMEOUT_KUSTOMIZATIONS}s"

    # Check prerequisites
    log_header "Checking Prerequisites"

    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    log_success "Kubernetes cluster accessible"

    if ! flux --version >/dev/null 2>&1; then
        log_error "Flux CLI not installed"
        exit 1
    fi
    log_success "Flux CLI available"

    # Check if Flux is installed in cluster
    if ! kubectl get namespace flux-system >/dev/null 2>&1; then
        log_error "Flux not installed in cluster (flux-system namespace missing)"
        exit 1
    fi
    log_success "Flux installed in cluster"

    # Check for required tools
    for cmd in yq kustomize helm; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            log_error "$cmd is required but not installed"
            exit 1
        fi
    done
    log_success "Required tools available (yq, kustomize, helm)"

    # Render manifests unless SKIP_RENDER is set
    if [[ "${SKIP_RENDER:-false}" != "true" ]]; then
        render_manifests || exit 1
    else
        log_info "SKIP_RENDER=true, using existing rendered manifests"
    fi

    # Push to git unless SKIP_GIT_PUSH is set
    if [[ "${SKIP_GIT_PUSH:-false}" != "true" ]]; then
        push_to_git || exit 1
    else
        log_info "SKIP_GIT_PUSH=true, skipping git push"
    fi

    # Apply manifests unless SKIP_APPLY is set
    if [[ "${SKIP_APPLY:-false}" != "true" ]]; then
        log_header "Applying Rendered Mode Manifests"

        local flux_sync_file="${REPO_ROOT}/rendered/flux/clusters/${ENVIRONMENT}/flux-sync.yaml"

        if [[ ! -f "$flux_sync_file" ]]; then
            log_error "Flux sync file not found: $flux_sync_file"
            exit 1
        fi

        log_info "Applying: $flux_sync_file"
        kubectl apply -f "$flux_sync_file"
        log_success "Manifests applied"

        # Give Flux a moment to start reconciling
        log_info "Waiting for Flux to start reconciliation..."
        sleep 5

        # Trigger reconciliation
        reconcile_flux
    else
        log_info "SKIP_APPLY=true, skipping manifest application"
    fi

    # Validate Kustomizations
    local ks_result=0
    check_kustomizations_ready "$FLUX_NAMESPACE" "$TIMEOUT_KUSTOMIZATIONS" || ks_result=$?

    # Generate summary
    local end_time=$(date +%s)
    local final_status="PASSED"

    if [[ $ks_result -ne 0 ]]; then
        final_status="FAILED"
    fi

    generate_summary "Rendered Mode (Kustomization)" "$start_time" "$end_time" "$final_status"

    # Exit with appropriate code
    if [[ "$final_status" == "FAILED" ]]; then
        exit 1
    fi

    exit 0
}

main "$@"
