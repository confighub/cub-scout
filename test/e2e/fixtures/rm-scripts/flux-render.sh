#!/usr/bin/env bash
set -euo pipefail

# Render Flux HelmRelease manifests to plain YAML
# Usage:
#   ./flux-render.sh                              # Render all environments
#   ./flux-render.sh dev                          # Render dev environment
#   ./flux-render.sh production observability/grafana  # Render specific release
#   ./flux-render.sh dev security                 # Render all releases in a group

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
FLUX_DIR="${REPO_ROOT}/flux-helm-kustomize"
RENDERED_DIR="${REPO_ROOT}/rendered/flux/manifests"
CLUSTERS_DIR="${REPO_ROOT}/rendered/flux/clusters"
POST_PROCESS="${REPO_ROOT}/scripts/post-process.sh"

# This layout uses Flux for GitOps
OPERATOR="flux"

# Check dependencies
for cmd in yq kustomize helm; do
    if ! command -v "$cmd" &> /dev/null; then
        echo "Error: $cmd is required but not installed."
        exit 1
    fi
done

# Map HelmRepository names to URLs (bash 3.x compatible)
get_repo_url() {
    local repo_name="$1"
    case "$repo_name" in
        jetstack) echo "https://charts.jetstack.io" ;;
        external-secrets) echo "https://charts.external-secrets.io" ;;
        bitnami) echo "https://charts.bitnami.com/bitnami" ;;
        traefik) echo "https://traefik.github.io/charts" ;;
        prometheus-community) echo "https://prometheus-community.github.io/helm-charts" ;;
        grafana) echo "https://grafana.github.io/helm-charts" ;;
        kyverno) echo "https://kyverno.github.io/kyverno" ;;
        aquasecurity) echo "https://aquasecurity.github.io/helm-charts" ;;
        kedacore) echo "https://kedacore.github.io/charts" ;;
        vmware-tanzu) echo "https://vmware-tanzu.github.io/helm-charts" ;;
        kubernetes-sigs) echo "https://kubernetes-sigs.github.io/metrics-server" ;;
        stakater) echo "https://stakater.github.io/stakater-charts" ;;
        external-dns) echo "https://kubernetes-sigs.github.io/external-dns" ;;
        *) echo "" ;;
    esac
}

ENVIRONMENTS=(dev staging production)
CHART_GROUPS=(core security observability operations)

HELM_REPOS=(
    jetstack
    external-secrets
    bitnami
    traefik
    prometheus-community
    grafana
    kyverno
    aquasecurity
    kedacore
    vmware-tanzu
    kubernetes-sigs
    stakater
    external-dns
)

# Map releases to groups
get_release_group() {
    local release="$1"
    case "$release" in
        cert-manager|external-secrets|external-dns|traefik|metrics-server|reloader|prometheus) echo "core" ;;
        kyverno|trivy-operator) echo "security" ;;
        grafana|loki|alloy|tempo) echo "observability" ;;
        keda) echo "operations" ;;
        *) echo "" ;;
    esac
}

# Track namespaces per environment (file-based for bash 3.x compatibility)
NAMESPACE_TRACKING_FILE=""

init_namespace_tracking() {
    NAMESPACE_TRACKING_FILE=$(mktemp)
}

track_namespace() {
    local env="$1"
    local namespace="$2"
    if [[ -n "$namespace" && "$namespace" != "null" ]]; then
        echo "${env}:${namespace}" >> "$NAMESPACE_TRACKING_FILE"
    fi
}

get_tracked_namespaces() {
    local env="$1"
    if [[ -f "$NAMESPACE_TRACKING_FILE" ]]; then
        grep "^${env}:" "$NAMESPACE_TRACKING_FILE" | cut -d: -f2 | sort -u
    fi
}

cleanup_namespace_tracking() {
    rm -f "$NAMESPACE_TRACKING_FILE"
}

render_helmrelease() {
    local env="$1"
    local hr_yaml="$2"
    local group="$3"
    local output_dir="${RENDERED_DIR}/${env}/${group}"

    # Extract HelmRelease details
    local name chart version repo_name namespace
    name=$(echo "$hr_yaml" | yq '.metadata.name')
    # Use targetNamespace from spec, fallback to metadata.namespace
    namespace=$(echo "$hr_yaml" | yq '.spec.targetNamespace // .metadata.namespace')
    chart=$(echo "$hr_yaml" | yq '.spec.chart.spec.chart')
    version=$(echo "$hr_yaml" | yq '.spec.chart.spec.version')
    repo_name=$(echo "$hr_yaml" | yq '.spec.chart.spec.sourceRef.name')

    # Track namespace for later namespace resource generation
    track_namespace "$env" "$namespace"

    echo "Rendering ${group}/${name} for ${env}..."

    # Get repo URL
    local repo_url
    repo_url=$(get_repo_url "$repo_name")
    if [[ -z "$repo_url" ]]; then
        echo "  Warning: Unknown repository '${repo_name}', skipping"
        return 1
    fi

    # Add helm repo if not already added
    if ! helm repo list 2>/dev/null | grep -q "^${repo_name}"; then
        helm repo add "$repo_name" "$repo_url" --force-update >/dev/null 2>&1 || true
    fi

    # Extract values to temp file
    local values_file
    values_file=$(mktemp)
    echo "$hr_yaml" | yq '.spec.values // {}' > "$values_file"

    # Ensure output directory exists
    mkdir -p "$output_dir"

    # Render with helm template
    local output_file="${output_dir}/${name}.yaml"
    local temp_file="${output_file}.tmp"

    if ! helm template "$name" "${repo_name}/${chart}" \
        --version "$version" \
        --namespace "$namespace" \
        --include-crds \
        -f "$values_file" \
        > "$temp_file" 2>/dev/null; then
        echo "  Warning: Failed to render ${name}, skipping"
        rm -f "$values_file" "$temp_file"
        return 1
    fi

    rm -f "$values_file"

    # Post-process if script exists
    if [[ -x "${POST_PROCESS}" ]]; then
        "${POST_PROCESS}" "${OPERATOR}" "${env}/${group}" "${name}" "${temp_file}" "${RENDERED_DIR}"
        rm -f "${temp_file}"
    else
        mv "${temp_file}" "${output_file}"
        echo "  Warning: post-process.sh not found, skipping CRD/hook processing"
    fi

    echo "  Written to ${output_file}"
}

render_environment() {
    local env="$1"
    local target_group="${2:-}"
    local target_release="${3:-}"
    local overlay_dir="${FLUX_DIR}/overlays/${env}"

    if [[ ! -d "$overlay_dir" ]]; then
        echo "Error: Overlay directory not found: ${overlay_dir}"
        return 1
    fi

    echo "=== Rendering for ${env} ==="

    # Iterate over each group overlay directory
    for group in "${CHART_GROUPS[@]}"; do
        local group_overlay_dir="${overlay_dir}/${group}"

        # Skip if we're targeting a specific group and this isn't it
        if [[ -n "$target_group" && "$group" != "$target_group" ]]; then
            continue
        fi

        if [[ ! -d "$group_overlay_dir" ]]; then
            echo "Warning: Group overlay not found: ${group_overlay_dir}, skipping"
            continue
        fi

        # Build kustomization for this group and extract HelmReleases
        local kustomize_output
        kustomize_output=$(kustomize build "$group_overlay_dir")

        # Get list of HelmRelease names
        local releases
        releases=$(echo "$kustomize_output" | yq -N 'select(.kind == "HelmRelease") | .metadata.name')

        while IFS= read -r release_name; do
            [[ -z "$release_name" ]] && continue

            # Skip if we're targeting a specific release and this isn't it
            if [[ -n "$target_release" && "$release_name" != "$target_release" ]]; then
                continue
            fi

            # Extract this HelmRelease's YAML
            local hr_yaml
            hr_yaml=$(echo "$kustomize_output" | yq -N "select(.kind == \"HelmRelease\" and .metadata.name == \"${release_name}\")")

            render_helmrelease "$env" "$hr_yaml" "$group" || true
        done <<< "$releases"
    done

    echo
}

generate_namespaces() {
    local env="$1"
    local ns_dir="${RENDERED_DIR}/${env}/namespaces"

    # System namespaces to exclude
    local system_ns="default kube-system kube-public kube-node-lease flux-system"

    # Collect namespaces for this environment
    local namespaces
    namespaces=$(get_tracked_namespaces "$env")

    # Filter out system namespaces
    local filtered_namespaces=""
    for ns in $namespaces; do
        if ! echo "$system_ns" | grep -qw "$ns"; then
            filtered_namespaces="$filtered_namespaces $ns"
        fi
    done
    filtered_namespaces=$(echo "$filtered_namespaces" | xargs)

    if [[ -z "$filtered_namespaces" ]]; then
        return
    fi

    echo "Generating namespace resources for ${env}..."
    mkdir -p "$ns_dir"

    # Generate namespaces.yaml
    cat > "${ns_dir}/namespaces.yaml" << 'EOF'
# Auto-generated namespace resources
EOF

    for ns in $filtered_namespaces; do
        cat >> "${ns_dir}/namespaces.yaml" << EOF
---
apiVersion: v1
kind: Namespace
metadata:
  name: ${ns}
EOF
    done

    echo "  Created ${ns_dir}/namespaces.yaml with namespaces: ${filtered_namespaces}"
}

fix_missing_namespaces() {
    local env="$1"
    local rendered_env_dir="${RENDERED_DIR}/${env}"

    # Ensure all rendered manifests have proper namespace set
    # Some charts don't include namespace in templates for namespaced resources
    echo "Fixing missing namespaces in rendered manifests for ${env}..."

    # Namespaced resource kinds that should have namespace set
    local namespaced_kinds=(
        ServiceAccount ConfigMap Secret Service Deployment
        StatefulSet DaemonSet Job CronJob PersistentVolumeClaim
        Role RoleBinding ServiceMonitor PodMonitor Ingress NetworkPolicy
    )

    for group_dir in "$rendered_env_dir"/*/; do
        [[ -d "$group_dir" ]] || continue
        local group_name
        group_name=$(basename "$group_dir")
        [[ "$group_name" == "namespaces" ]] && continue

        for yaml_file in "$group_dir"/*.yaml; do
            [[ -f "$yaml_file" ]] || continue

            # Get the namespace used in this file (from metadata.namespace)
            # Strip quotes if present. Use || true to handle files with no namespace
            local ns
            ns=$(grep "^  namespace:" "$yaml_file" 2>/dev/null | head -1 | awk '{print $2}' | tr -d '"' | tr -d "'" || true)
            if [[ -n "$ns" ]]; then
                # Add namespace to each type of resource that's missing it
                for kind in "${namespaced_kinds[@]}"; do
                    yq -i "select(.kind == \"${kind}\" and .metadata.namespace == null).metadata.namespace = \"${ns}\"" "$yaml_file" 2>/dev/null || true
                done
            fi
        done
    done

    echo "  Done"
}

update_helm_repos() {
    echo "Updating Helm repositories..."
    for repo_name in "${HELM_REPOS[@]}"; do
        local repo_url
        repo_url=$(get_repo_url "$repo_name")
        helm repo add "$repo_name" "$repo_url" --force-update >/dev/null 2>&1 || true
    done
    helm repo update >/dev/null 2>&1 || true
    echo "Repositories updated."
    echo
}

contains() {
    local needle="$1"
    shift
    for item in "$@"; do
        if [[ "$item" == "$needle" ]]; then
            return 0
        fi
    done
    return 1
}

generate_flux_sync() {
    local env="$1"
    local env_clusters_dir="${CLUSTERS_DIR}/${env}"
    local flux_sync_file="${env_clusters_dir}/flux-sync.yaml"
    local rendered_env_dir="${RENDERED_DIR}/${env}"

    # Git repository URL - can be overridden via environment variable
    local git_repo_url="${GIT_REPO_URL:-http://gitea-http.gitea.svc:3000/gitea_admin/rendered.git}"

    echo "Generating Flux sync for ${env}..."
    mkdir -p "$env_clusters_dir"

    # Start with GitRepository
    cat > "$flux_sync_file" << EOF
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: flux-rendered
  namespace: flux-system
spec:
  interval: 1m
  url: ${git_repo_url}
  ref:
    branch: main
EOF

    # Find all groups that were rendered (excluding namespaces)
    local groups=()
    for dir in "$rendered_env_dir"/*/; do
        [[ -d "$dir" ]] || continue
        local group_name
        group_name=$(basename "$dir")
        [[ "$group_name" == "namespaces" ]] && continue
        groups+=("$group_name")
    done

    # Generate namespaces Kustomization first (if namespaces directory exists)
    local ns_dir="${rendered_env_dir}/namespaces"
    if [[ -d "$ns_dir" ]] && [[ -n "$(ls -A "$ns_dir" 2>/dev/null)" ]]; then
        cat >> "$flux_sync_file" << EOF
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: ${env}-namespaces
  namespace: flux-system
spec:
  interval: 10m
  sourceRef:
    kind: GitRepository
    name: flux-rendered
  path: ./flux/manifests/${env}/namespaces
  prune: false
  wait: true
  timeout: 2m
EOF
    fi

    # Check if namespaces exist
    local has_namespaces="false"
    if [[ -d "$ns_dir" ]] && [[ -n "$(ls -A "$ns_dir" 2>/dev/null)" ]]; then
        has_namespaces="true"
    fi

    # Generate CRD Kustomizations (depend on namespaces)
    for group in "${groups[@]}"; do
        local crds_dir="${rendered_env_dir}/${group}/crds"
        if [[ -d "$crds_dir" ]] && [[ -n "$(ls -A "$crds_dir" 2>/dev/null)" ]]; then
            cat >> "$flux_sync_file" << EOF
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: ${env}-${group}-crds
  namespace: flux-system
spec:
EOF
            if [[ "$has_namespaces" == "true" ]]; then
                cat >> "$flux_sync_file" << EOF
  dependsOn:
    - name: ${env}-namespaces
EOF
            fi
            cat >> "$flux_sync_file" << EOF
  interval: 10m
  sourceRef:
    kind: GitRepository
    name: flux-rendered
  path: ./flux/manifests/${env}/${group}/crds
  prune: false
  wait: true
  timeout: 5m
EOF
        fi
    done

    # Generate resource Kustomizations with dependsOn (namespaces + CRDs)
    for group in "${groups[@]}"; do
        local crds_dir="${rendered_env_dir}/${group}/crds"
        local has_crds="false"
        if [[ -d "$crds_dir" ]] && [[ -n "$(ls -A "$crds_dir" 2>/dev/null)" ]]; then
            has_crds="true"
        fi

        # Check if this group contains ServiceMonitor or PodMonitor resources
        # These require Prometheus CRDs from the observability group
        local needs_observability_crds="false"
        if [[ "$group" == "operations" ]]; then
            for yaml_file in "${rendered_env_dir}/${group}"/*.yaml; do
                [[ -f "$yaml_file" ]] || continue
                if grep -q "^kind: ServiceMonitor\|^kind: PodMonitor" "$yaml_file" 2>/dev/null; then
                    # Check if observability-crds exists
                    local obs_crds_dir="${rendered_env_dir}/observability/crds"
                    if [[ -d "$obs_crds_dir" ]] && [[ -n "$(ls -A "$obs_crds_dir" 2>/dev/null)" ]]; then
                        needs_observability_crds="true"
                        break
                    fi
                fi
            done
        fi

        cat >> "$flux_sync_file" << EOF
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: ${env}-${group}
  namespace: flux-system
spec:
  dependsOn:
EOF
        # Depend on namespaces first
        if [[ "$has_namespaces" == "true" ]]; then
            cat >> "$flux_sync_file" << EOF
    - name: ${env}-namespaces
EOF
        fi
        # Then depend on CRDs if they exist
        if [[ "$has_crds" == "true" ]]; then
            cat >> "$flux_sync_file" << EOF
    - name: ${env}-${group}-crds
EOF
        fi
        # Operations group needs observability CRDs for ServiceMonitor/PodMonitor
        if [[ "$needs_observability_crds" == "true" ]]; then
            cat >> "$flux_sync_file" << EOF
    - name: ${env}-observability-crds
EOF
        fi

        cat >> "$flux_sync_file" << EOF
  interval: 10m
  sourceRef:
    kind: GitRepository
    name: flux-rendered
  path: ./flux/manifests/${env}/${group}
  prune: true
  wait: true
  timeout: 5m
EOF
    done

    echo "  Written to ${flux_sync_file}"
}

main() {
    local target_env="${1:-}"
    local target="${2:-}"  # Can be "group" or "group/release"

    # Parse target
    local target_group=""
    local target_release=""
    if [[ -n "$target" ]]; then
        if [[ "$target" == */* ]]; then
            target_group="${target%%/*}"
            target_release="${target#*/}"
        else
            # Could be a group name
            if contains "$target" "${CHART_GROUPS[@]}"; then
                target_group="$target"
            else
                echo "Error: Invalid target '$target'"
                echo "Use: group (e.g., 'core') or group/release (e.g., 'observability/grafana')"
                exit 1
            fi
        fi
    fi

    # Validate environment if specified
    if [[ -n "$target_env" ]] && ! contains "$target_env" "${ENVIRONMENTS[@]}"; then
        echo "Error: Invalid environment '${target_env}'"
        echo "Valid environments: ${ENVIRONMENTS[*]}"
        exit 1
    fi

    # Validate group if specified
    if [[ -n "$target_group" ]] && ! contains "$target_group" "${CHART_GROUPS[@]}"; then
        echo "Error: Invalid group '${target_group}'"
        echo "Valid groups: ${CHART_GROUPS[*]}"
        exit 1
    fi

    update_helm_repos

    # Initialize namespace tracking
    init_namespace_tracking
    trap cleanup_namespace_tracking EXIT

    # Determine what to render
    local envs_to_render=("${ENVIRONMENTS[@]}")
    if [[ -n "$target_env" ]]; then
        envs_to_render=("$target_env")
    fi

    for env in "${envs_to_render[@]}"; do
        render_environment "$env" "$target_group" "$target_release"
        generate_namespaces "$env"
        fix_missing_namespaces "$env"
        generate_flux_sync "$env"
    done

    echo "Done! Rendered manifests are in ${RENDERED_DIR}/"
}

main "$@"
