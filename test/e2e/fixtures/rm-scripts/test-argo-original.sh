#!/usr/bin/env bash
set -euo pipefail

# Test script for Argo CD original mode (ApplicationSets with Helm source type)
# Usage: ./test-argo-original.sh <environment>
#
# Environment variables:
#   SKIP_GIT_PUSH=1   - Skip pushing to Gitea
#   SKIP_APPLY=1      - Skip applying manifests, only validate
#   TIMEOUT_APPS=900  - Timeout for Applications (default: 900s)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

# Source shared functions
source "${SCRIPT_DIR}/lib-test.sh"

# Configuration
ENVIRONMENT="${1:-dev}"
ARGO_NAMESPACE="argocd"
ARGO_DIR="${REPO_ROOT}/argo-umbrella-charts"

# Gitea configuration
GITEA_NAMESPACE="gitea"
GITEA_USER="gitea_admin"
GITEA_PASSWORD="admin123"
GITEA_REPO="argo-umbrella-charts"

# Timeouts
TIMEOUT_APPS="${TIMEOUT_APPS:-900}"

usage() {
    echo "Usage: $0 [environment]"
    echo ""
    echo "Tests the 'original' Argo CD ApplicationSet approach (Helm source type)"
    echo ""
    echo "Arguments:"
    echo "  environment    Environment to test (dev, staging, production). Default: dev"
    echo ""
    echo "Environment variables:"
    echo "  TIMEOUT_APPS       Timeout for Applications to be ready (default: 900s)"
    echo "  SKIP_APPLY         Skip applying manifests, just validate (default: false)"
    echo "  SKIP_GIT_PUSH      Skip pushing to git server (default: false)"
    echo "  GIT_SERVER_URL     Git server URL (default: http://gitea-http.gitea.svc:3000)"
    echo "  GIT_USER           Git username (default: gitea_admin)"
    echo "  GIT_PASSWORD       Git password (default: admin123)"
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

push_to_gitea() {
    if [[ "${SKIP_GIT_PUSH:-false}" == "true" ]]; then
        log_info "Skipping git push (SKIP_GIT_PUSH=true)"
        return 0
    fi

    log_header "Pushing to Git Server"

    # Check if Gitea is available
    if ! kubectl get svc gitea-http -n "$GITEA_NAMESPACE" >/dev/null 2>&1; then
        log_warning "Gitea not found in cluster, skipping git push"
        return 0
    fi

    cd "$ARGO_DIR"

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
    git remote add gitea "http://${GITEA_USER}:${GITEA_PASSWORD}@localhost:3030/${GITEA_USER}/${GITEA_REPO}.git"

    # Commit and push
    git add -A
    git commit -m "Update argo-umbrella-charts for ${ENVIRONMENT}" 2>/dev/null || log_info "No changes to commit"

    log_info "Pushing to Gitea..."
    if git push -f gitea main 2>&1; then
        log_success "Pushed to git server"
        return 0
    else
        log_error "Failed to push to git server"
        return 1
    fi
}

apply_applicationsets() {
    if [[ "${SKIP_APPLY:-false}" == "true" ]]; then
        log_info "Skipping apply (SKIP_APPLY=true)"
        return 0
    fi

    log_header "Applying Argo CD ApplicationSets"

    # Multi-environment ApplicationSet (uses matrix generator)
    local apps_file="${ARGO_DIR}/clusters/applicationset.yaml"

    if [[ ! -f "$apps_file" ]]; then
        log_error "ApplicationSet file not found: $apps_file"
        return 1
    fi

    log_info "Applying: $apps_file"
    kubectl apply -f "$apps_file"

    log_success "ApplicationSets applied"
    log_info "Waiting for Argo CD to start reconciliation..."
    sleep 5
}

refresh_applications() {
    log_header "Refreshing Argo CD Applications"

    local timestamp=$(date +%s)

    # Trigger refresh on all applications in argocd namespace
    log_info "Triggering Application refresh..."
    local count=0
    for app in $(kubectl get applications -n "$ARGO_NAMESPACE" -o name 2>/dev/null); do
        # Annotate to trigger refresh
        kubectl annotate --overwrite "$app" -n "$ARGO_NAMESPACE" \
            "argocd.argoproj.io/refresh=normal" 2>/dev/null || true
        count=$((count + 1))
    done
    log_success "Triggered refresh for $count Applications"
}

main() {
    local start_time=$(date +%s)

    log_header "Testing Argo CD Original Mode (ApplicationSet with Helm)"
    log_info "Environment: $ENVIRONMENT"
    log_info "Timeout (Applications): ${TIMEOUT_APPS}s"

    # Check prerequisites
    log_header "Checking Prerequisites"

    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "Cannot connect to Kubernetes cluster"
        exit 1
    fi
    log_success "Kubernetes cluster accessible"

    if ! kubectl get namespace "$ARGO_NAMESPACE" >/dev/null 2>&1; then
        log_error "Argo CD namespace not found: $ARGO_NAMESPACE"
        exit 1
    fi
    log_success "Argo CD installed in cluster"

    if ! command -v argocd >/dev/null 2>&1; then
        log_warning "argocd CLI not installed (optional)"
    else
        log_success "argocd CLI available"
    fi

    for cmd in helm; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            log_error "Required tool not found: $cmd"
            exit 1
        fi
    done
    log_success "Required tools available (helm)"

    # Push to Gitea
    push_to_gitea || exit 1

    # Apply ApplicationSets
    apply_applicationsets

    # Refresh applications
    refresh_applications

    # Wait for Applications to be ready (Argo CD's Healthy status includes pod health)
    if ! check_applications_ready "$ARGO_NAMESPACE" "$TIMEOUT_APPS"; then
        local end_time=$(date +%s)
        generate_summary "Argo CD Original Mode (ApplicationSet)" "$start_time" "$end_time" "FAILED"
        exit 1
    fi

    # Check for issues
    local issues=0
    check_common_issues || issues=$?

    local end_time=$(date +%s)

    if [[ $issues -gt 0 ]]; then
        generate_summary "Argo CD Original Mode (ApplicationSet)" "$start_time" "$end_time" "PASSED WITH WARNINGS"
    else
        generate_summary "Argo CD Original Mode (ApplicationSet)" "$start_time" "$end_time" "PASSED"
    fi
}

main "$@"
