#!/usr/bin/env bash
set -euo pipefail

# Test script for the "original" approach (Flux HelmRelease CRs)
# This validates that HelmReleases deploy correctly via Flux

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

# Source shared test functions
source "${SCRIPT_DIR}/lib-test.sh"

# Configuration
FLUX_NAMESPACE="flux-system"
ENVIRONMENT="${1:-dev}"
TIMEOUT_HELMRELEASES="${TIMEOUT_HELMRELEASES:-900}"  # 15 minutes

# Git server configuration (for local testing with Gitea)
GIT_SERVER_URL="${GIT_SERVER_URL:-http://gitea-http.gitea.svc:3000}"
GIT_REPO_NAME="${GIT_REPO_NAME:-flux-helm-kustomize}"
GIT_USER="${GIT_USER:-gitea_admin}"
GIT_PASSWORD="${GIT_PASSWORD:-admin123}"

usage() {
    echo "Usage: $0 [environment]"
    echo ""
    echo "Tests the 'original' Flux HelmRelease approach"
    echo ""
    echo "Arguments:"
    echo "  environment    Environment to test (dev, staging, production). Default: dev"
    echo ""
    echo "Environment variables:"
    echo "  TIMEOUT_HELMRELEASES  Timeout for HelmReleases to be ready (default: 900s)"
    echo "  SKIP_APPLY            Skip applying manifests, just validate (default: false)"
    echo "  SKIP_GIT_PUSH         Skip pushing to git server (default: false)"
    echo "  GIT_SERVER_URL        Git server URL (default: http://gitea-http.gitea.svc:3000)"
    echo "  GIT_USER              Git username (default: gitea_admin)"
    echo "  GIT_PASSWORD          Git password (default: admin123)"
    echo ""
    echo "Examples:"
    echo "  $0                    # Test dev environment"
    echo "  $0 staging            # Test staging environment"
    echo "  SKIP_APPLY=true $0    # Only validate, don't apply"
    exit 1
}

# Parse arguments
if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
fi

push_to_git() {
    log_header "Pushing to Git Server"

    cd "${REPO_ROOT}/flux-helm-kustomize"

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
    git commit -m "Update flux-helm-kustomize for ${ENVIRONMENT}" 2>/dev/null || log_info "No changes to commit"

    log_info "Pushing to Gitea..."
    if git push -f gitea main 2>&1; then
        log_success "Pushed to git server"
        return 0
    else
        log_error "Failed to push to git server"
        return 1
    fi
}

main() {
    local start_time=$(date +%s)

    log_header "Testing Original Mode (HelmRelease)"
    log_info "Environment: $ENVIRONMENT"
    log_info "Timeout: ${TIMEOUT_HELMRELEASES}s"

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

    # Push to git unless SKIP_GIT_PUSH is set
    if [[ "${SKIP_GIT_PUSH:-false}" != "true" ]]; then
        push_to_git || exit 1
    else
        log_info "SKIP_GIT_PUSH=true, skipping git push"
    fi

    # Apply manifests unless SKIP_APPLY is set
    if [[ "${SKIP_APPLY:-false}" != "true" ]]; then
        log_header "Applying Original Mode Manifests"

        local flux_sync_file="${REPO_ROOT}/flux-helm-kustomize/clusters/${ENVIRONMENT}/flux-sync.yaml"

        if [[ ! -f "$flux_sync_file" ]]; then
            log_error "Flux sync file not found: $flux_sync_file"
            exit 1
        fi

        log_info "Applying: $flux_sync_file"
        kubectl apply -f "$flux_sync_file"
        log_success "Manifests applied"

        # Give Flux a moment to start reconciling
        log_info "Waiting for Flux to start reconciliation..."
        sleep 10
    else
        log_info "SKIP_APPLY=true, skipping manifest application"
    fi

    # Validate HelmReleases
    local hr_result=0
    check_helmreleases_ready "$FLUX_NAMESPACE" "$TIMEOUT_HELMRELEASES" || hr_result=$?

    # Generate summary
    local end_time=$(date +%s)
    local final_status="PASSED"

    if [[ $hr_result -ne 0 ]]; then
        final_status="FAILED"
    fi

    generate_summary "Original Mode (HelmRelease)" "$start_time" "$end_time" "$final_status"

    # Exit with appropriate code
    if [[ "$final_status" == "FAILED" ]]; then
        exit 1
    fi

    exit 0
}

main "$@"
