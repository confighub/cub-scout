#!/usr/bin/env bash
set -euo pipefail

# Test script for Argo CD rendered mode (ApplicationSets with pre-rendered YAML)
# Usage: ./test-argo-rendered.sh <environment>
#
# Environment variables:
#   SKIP_RENDER=1     - Skip re-rendering manifests
#   SKIP_GIT_PUSH=1   - Skip pushing to Gitea
#   SKIP_APPLY=1      - Skip applying manifests, only validate
#   TIMEOUT_APPS=600  - Timeout for Applications (default: 600s)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

# Source shared functions
source "${SCRIPT_DIR}/lib-test.sh"

# Configuration
ENVIRONMENT="${1:-dev}"
ARGO_NAMESPACE="argocd"
RENDERED_DIR="${REPO_ROOT}/rendered"
RENDER_SCRIPT="${SCRIPT_DIR}/argo-render.sh"

# Gitea configuration
GITEA_NAMESPACE="gitea"
GITEA_USER="gitea_admin"
GITEA_PASSWORD="admin123"
GITEA_REPO="rendered"

# Timeouts
TIMEOUT_APPS="${TIMEOUT_APPS:-600}"

render_manifests() {
    if [[ "${SKIP_RENDER:-}" == "1" ]]; then
        log_info "Skipping render (SKIP_RENDER=1)"
        return 0
    fi

    log_header "Rendering Manifests"

    if [[ ! -x "$RENDER_SCRIPT" ]]; then
        log_error "Render script not found or not executable: $RENDER_SCRIPT"
        return 1
    fi

    log_info "Running: $RENDER_SCRIPT $ENVIRONMENT"
    "$RENDER_SCRIPT" "$ENVIRONMENT"

    log_success "Manifests rendered successfully"
}

push_to_gitea() {
    if [[ "${SKIP_GIT_PUSH:-}" == "1" ]]; then
        log_info "Skipping git push (SKIP_GIT_PUSH=1)"
        return 0
    fi

    log_header "Pushing to Git Server"

    # Check if Gitea is accessible via NodePort
    if ! curl -s -o /dev/null -w "%{http_code}" "http://localhost:3030" 2>/dev/null | grep -q "200\|302"; then
        log_error "Gitea not accessible at localhost:3030"
        return 1
    fi

    cd "$RENDERED_DIR"

    # Initialize git if needed
    if [[ ! -d .git ]]; then
        log_info "Initializing git repository..."
        git init
        git config user.email "test@example.com"
        git config user.name "Test User"
    fi

    # Configure remote
    git remote remove gitea 2>/dev/null || true
    git remote add gitea "http://${GITEA_USER}:${GITEA_PASSWORD}@localhost:3030/${GITEA_USER}/${GITEA_REPO}.git"

    # Commit and push
    git add -A
    git commit -m "Update rendered manifests for $ENVIRONMENT" 2>/dev/null || log_info "No changes to commit"

    log_info "Pushing to Gitea..."
    if git push -f gitea main 2>&1; then
        log_success "Pushed to git server"
    else
        log_error "Failed to push to git server"
        cd "$SCRIPT_DIR"
        return 1
    fi

    cd "$SCRIPT_DIR"
}

apply_applicationsets() {
    if [[ "${SKIP_APPLY:-}" == "1" ]]; then
        log_info "Skipping apply (SKIP_APPLY=1)"
        return 0
    fi

    log_header "Applying Argo CD ApplicationSets"

    local apps_file="${RENDERED_DIR}/argo/clusters/applicationset.yaml"

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

    log_header "Testing Argo CD Rendered Mode (ApplicationSet)"
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

    for cmd in yq helm; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            log_error "Required tool not found: $cmd"
            exit 1
        fi
    done
    log_success "Required tools available (yq, helm)"

    # Render manifests
    render_manifests

    # Push to Gitea
    push_to_gitea

    # Apply ApplicationSets
    apply_applicationsets

    # Refresh applications
    refresh_applications

    # Wait for Applications to be ready
    if ! check_applications_ready "$ARGO_NAMESPACE" "$TIMEOUT_APPS"; then
        local end_time=$(date +%s)
        generate_summary "Argo CD Rendered Mode (ApplicationSet)" "$start_time" "$end_time" "FAILED"
        exit 1
    fi

    # Check for issues
    local issues=0
    check_common_issues || issues=$?

    local end_time=$(date +%s)

    if [[ $issues -gt 0 ]]; then
        generate_summary "Argo CD Rendered Mode (ApplicationSet)" "$start_time" "$end_time" "PASSED WITH WARNINGS"
    else
        generate_summary "Argo CD Rendered Mode (ApplicationSet)" "$start_time" "$end_time" "PASSED"
    fi
}

main "$@"
