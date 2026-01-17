#!/usr/bin/env bash
# Shared test/validation functions for GitOps testing

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Expected namespaces for deployed services
# Default: per-component namespaces (Flux HelmRelease pattern)
# Set UMBRELLA_MODE=true for group namespaces (Argo CD umbrella chart pattern)
if [[ "${UMBRELLA_MODE:-false}" == "true" ]]; then
    EXPECTED_NAMESPACES=(
        core
        observability
        security
        operations
    )
else
    EXPECTED_NAMESPACES=(
        cert-manager
        external-dns
        external-secrets
        keda
        kyverno
        monitoring
        reloader
        traefik
        trivy-system
    )
fi

# Get expected minimum pod count for a namespace (bash 3.x compatible)
get_expected_pod_count() {
    local ns="$1"
    case "$ns" in
        # Umbrella chart group namespaces
        core) echo 10 ;;           # cert-manager(3) + external-secrets(3) + external-dns(1) + traefik(1) + metrics-server(1) + reloader(1)
        observability) echo 8 ;;   # prometheus-stack + grafana + loki + alloy + tempo
        security) echo 5 ;;        # kyverno(4) + trivy-operator(1)
        operations) echo 2 ;;      # keda(2)
        # Per-component namespaces
        cert-manager) echo 3 ;;
        external-dns) echo 1 ;;
        external-secrets) echo 3 ;;
        keda) echo 2 ;;
        kyverno) echo 4 ;;
        monitoring) echo 8 ;;
        reloader) echo 1 ;;
        traefik) echo 1 ;;
        trivy-system) echo 1 ;;
        *) echo 1 ;;
    esac
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
}

log_header() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  $1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

# Wait for a condition with timeout
wait_for() {
    local description="$1"
    local check_cmd="$2"
    local timeout="${3:-300}"
    local interval="${4:-5}"

    log_info "Waiting for: $description (timeout: ${timeout}s)"

    local elapsed=0
    while [[ $elapsed -lt $timeout ]]; do
        if eval "$check_cmd" >/dev/null 2>&1; then
            log_success "$description"
            return 0
        fi
        sleep "$interval"
        elapsed=$((elapsed + interval))
        echo -n "."
    done
    echo ""
    log_error "$description (timed out after ${timeout}s)"
    return 1
}

# Check if all Flux HelmReleases are ready
check_helmreleases_ready() {
    local namespace="${1:-flux-system}"
    local timeout="${2:-600}"
    local interval=10

    log_header "Checking HelmReleases"

    local elapsed=0
    while [[ $elapsed -lt $timeout ]]; do
        local total=$(kubectl get helmreleases -n "$namespace" --no-headers 2>/dev/null | wc -l | tr -d '[:space:]')
        local ready=$(kubectl get helmreleases -n "$namespace" --no-headers 2>/dev/null | grep -c "True" 2>/dev/null || true)
        ready="${ready:-0}"
        ready=$(echo "$ready" | tr -d '[:space:]')

        if [[ "$total" -gt 0 && "$ready" -eq "$total" ]]; then
            log_success "All HelmReleases ready: $ready/$total"
            kubectl get helmreleases -n "$namespace"
            return 0
        fi

        log_info "HelmReleases: $ready/$total ready (elapsed: ${elapsed}s)"
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done

    log_error "HelmReleases not ready after ${timeout}s"
    kubectl get helmreleases -n "$namespace"

    # Show failed releases
    log_info "Failed HelmReleases:"
    kubectl get helmreleases -n "$namespace" -o json | \
        jq -r '.items[] | select(.status.conditions[]?.status != "True") | .metadata.name' 2>/dev/null | \
        while read -r hr; do
            echo "  - $hr"
            kubectl get helmrelease "$hr" -n "$namespace" -o jsonpath='{.status.conditions[*].message}' 2>/dev/null
            echo ""
        done

    return 1
}

# Check if all Flux Kustomizations are ready
check_kustomizations_ready() {
    local namespace="${1:-flux-system}"
    local timeout="${2:-600}"
    local interval=10

    log_header "Checking Kustomizations"

    local elapsed=0
    while [[ $elapsed -lt $timeout ]]; do
        local total=$(kubectl get kustomizations -n "$namespace" --no-headers 2>/dev/null | wc -l | tr -d '[:space:]')
        local ready=$(kubectl get kustomizations -n "$namespace" --no-headers 2>/dev/null | awk '$3 == "True" {count++} END {print count+0}' | tr -d '[:space:]')

        if [[ "$total" -gt 0 && "$ready" -eq "$total" ]]; then
            log_success "All Kustomizations ready: $ready/$total"
            kubectl get kustomizations -n "$namespace"
            return 0
        fi

        log_info "Kustomizations: $ready/$total ready (elapsed: ${elapsed}s)"
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done

    log_error "Kustomizations not ready after ${timeout}s"
    kubectl get kustomizations -n "$namespace"

    # Show failed kustomizations
    log_info "Checking failed Kustomizations:"
    for ks in $(kubectl get kustomizations -n "$namespace" --no-headers | awk '$2 != "True" {print $1}'); do
        echo "  - $ks:"
        kubectl describe kustomization "$ks" -n "$namespace" 2>/dev/null | tail -10
    done

    return 1
}

# Check pods in expected namespaces
check_pods_running() {
    local timeout="${1:-300}"
    local interval=10

    log_header "Checking Pod Status"

    local all_ready=false
    local elapsed=0

    while [[ $elapsed -lt $timeout ]]; do
        all_ready=true

        for ns in "${EXPECTED_NAMESPACES[@]}"; do
            local expected_count
            expected_count=$(get_expected_pod_count "$ns")
            local running_count=$(kubectl get pods -n "$ns" --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d '[:space:]')

            if [[ "$running_count" -lt "$expected_count" ]]; then
                all_ready=false
            fi
        done

        if $all_ready; then
            break
        fi

        log_info "Waiting for pods... (elapsed: ${elapsed}s)"
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done

    # Final status check
    local failed=0
    for ns in "${EXPECTED_NAMESPACES[@]}"; do
        local expected_count
        expected_count=$(get_expected_pod_count "$ns")
        local running_count=$(kubectl get pods -n "$ns" --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d ' ')

        if [[ "$running_count" -ge "$expected_count" ]]; then
            log_success "$ns: $running_count pods running (expected >= $expected_count)"
        else
            log_error "$ns: $running_count pods running (expected >= $expected_count)"
            kubectl get pods -n "$ns" 2>/dev/null
            failed=$((failed + 1))
        fi
    done

    return $failed
}

# Check for common issues
check_common_issues() {
    log_header "Checking for Common Issues"

    local issues=0

    # Check for ImagePullBackOff pods
    local image_pull_issues
    image_pull_issues=$(kubectl get pods -A --no-headers 2>/dev/null | grep -c "ImagePullBackOff\|ErrImagePull" 2>/dev/null || true)
    image_pull_issues="${image_pull_issues:-0}"
    image_pull_issues=$(echo "$image_pull_issues" | tr -d '[:space:]')
    if [[ "$image_pull_issues" -gt 0 ]]; then
        log_warning "Found $image_pull_issues pods with image pull issues"
        kubectl get pods -A | grep -E "ImagePullBackOff|ErrImagePull"
        issues=$((issues + 1))
    else
        log_success "No image pull issues"
    fi

    # Check for CrashLoopBackOff pods
    local crash_loop
    crash_loop=$(kubectl get pods -A --no-headers 2>/dev/null | grep -c "CrashLoopBackOff" 2>/dev/null || true)
    crash_loop="${crash_loop:-0}"
    crash_loop=$(echo "$crash_loop" | tr -d '[:space:]')
    if [[ "$crash_loop" -gt 0 ]]; then
        log_warning "Found $crash_loop pods in CrashLoopBackOff"
        kubectl get pods -A | grep "CrashLoopBackOff"
        issues=$((issues + 1))
    else
        log_success "No CrashLoopBackOff pods"
    fi

    # Check for pending pods (excluding Jobs)
    local pending=$(kubectl get pods -A --field-selector=status.phase=Pending --no-headers 2>/dev/null | wc -l | tr -d '[:space:]')
    if [[ "$pending" -gt 0 ]]; then
        log_warning "Found $pending pending pods"
        kubectl get pods -A --field-selector=status.phase=Pending
    else
        log_success "No pending pods"
    fi

    return $issues
}

# Generate summary report
generate_summary() {
    local test_type="$1"
    local start_time="$2"
    local end_time="$3"
    local status="$4"

    local duration=$((end_time - start_time))

    log_header "Test Summary: $test_type"

    echo "Duration: ${duration}s"
    echo "Status: $status"
    echo ""

    echo "Namespaces created:"
    for ns in "${EXPECTED_NAMESPACES[@]}"; do
        if kubectl get namespace "$ns" >/dev/null 2>&1; then
            local pod_count=$(kubectl get pods -n "$ns" --field-selector=status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d ' ')
            echo "  - $ns: $pod_count running pods"
        fi
    done

    echo ""
    echo "Pod status summary:"
    kubectl get pods -A --no-headers 2>/dev/null | awk '{print $4}' | sort | uniq -c | sort -rn
}

# Check if all Argo CD Applications are synced and healthy
check_applications_ready() {
    local namespace="${1:-argocd}"
    local timeout="${2:-600}"
    local interval=10

    log_header "Checking Argo CD Applications"

    local elapsed=0
    while [[ $elapsed -lt $timeout ]]; do
        local total=$(kubectl get applications -n "$namespace" --no-headers 2>/dev/null | wc -l | tr -d ' ')
        # Check for apps that are Synced AND Healthy
        # Column 2 = SYNC STATUS, Column 3 = HEALTH STATUS
        local ready=$(kubectl get applications -n "$namespace" --no-headers 2>/dev/null | awk '$2 == "Synced" && $3 == "Healthy" {count++} END {print count+0}')

        if [[ "$total" -gt 0 && "$ready" -eq "$total" ]]; then
            log_success "All Applications ready: $ready/$total"
            kubectl get applications -n "$namespace"
            return 0
        fi

        log_info "Applications: $ready/$total ready (elapsed: ${elapsed}s)"
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done

    log_error "Applications not ready after ${timeout}s"
    kubectl get applications -n "$namespace"

    # Show failed applications
    log_info "Checking non-healthy Applications:"
    kubectl get applications -n "$namespace" --no-headers 2>/dev/null | \
        awk '$2 != "Synced" || $3 != "Healthy" {print "  - " $1 ": Sync=" $2 ", Health=" $3}'

    return 1
}

# Cleanup helper - suspends Flux resources
suspend_flux_resources() {
    log_info "Suspending Flux resources..."
    flux suspend kustomization --all -n flux-system 2>/dev/null || true
    flux suspend helmrelease --all -n flux-system 2>/dev/null || true
}

# Resume Flux resources
resume_flux_resources() {
    log_info "Resuming Flux resources..."
    flux resume kustomization --all -n flux-system 2>/dev/null || true
    flux resume helmrelease --all -n flux-system 2>/dev/null || true
}
