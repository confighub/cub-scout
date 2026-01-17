#!/usr/bin/env bash
set -euo pipefail

# Render Helm umbrella charts to plain YAML manifests
# Usage:
#   ./argo-render.sh                    # Render all groups for all environments
#   ./argo-render.sh dev                # Render all groups for dev environment
#   ./argo-render.sh dev core           # Render specific group for specific environment

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"
ARGO_DIR="${REPO_ROOT}/argo-umbrella-charts"
CHARTS_DIR="${ARGO_DIR}/charts"
RENDERED_DIR="${REPO_ROOT}/rendered/argo/manifests"
CLUSTERS_DIR="${REPO_ROOT}/rendered/argo/clusters"

# Groups (umbrella charts)
CHART_GROUPS=(core observability security operations)

# All environments
ENVIRONMENTS=(dev staging production)

# Component to namespace mapping (for rendered mode which doesn't use group namespaces)
get_component_namespace() {
    local component="$1"
    case "$component" in
        cert-manager) echo "cert-manager" ;;
        external-secrets) echo "external-secrets" ;;
        external-dns) echo "external-dns" ;;
        traefik) echo "traefik" ;;
        metrics-server) echo "kube-system" ;;
        reloader) echo "reloader" ;;
        kube-prometheus-stack) echo "monitoring" ;;
        grafana) echo "monitoring" ;;
        loki) echo "monitoring" ;;
        alloy) echo "monitoring" ;;
        tempo) echo "monitoring" ;;
        kyverno) echo "kyverno" ;;
        trivy-operator) echo "trivy-system" ;;
        keda) echo "keda" ;;
        *) echo "default" ;;
    esac
}

render_group() {
    local env="$1"
    local group="$2"
    local chart_dir="${CHARTS_DIR}/${group}"
    local output_dir="${RENDERED_DIR}/${env}/${group}"
    local crds_dir="${RENDERED_DIR}/${env}/crds"

    echo "Rendering ${group} for ${env}..."

    # Check if chart exists
    if [[ ! -f "${chart_dir}/Chart.yaml" ]]; then
        echo "  Warning: ${chart_dir}/Chart.yaml not found, skipping"
        return
    fi

    # Ensure output directories exist
    mkdir -p "${output_dir}"
    mkdir -p "${crds_dir}"

    # Update dependencies if needed
    if [[ ! -d "${chart_dir}/charts" ]] || [[ -z "$(ls -A "${chart_dir}/charts" 2>/dev/null)" ]]; then
        echo "  Updating dependencies..."
        helm dependency update "${chart_dir}" --skip-refresh 2>/dev/null || true
    fi

    # Build values file list
    local values_args=""
    if [[ -f "${chart_dir}/values/base.yaml" ]]; then
        values_args="${values_args} -f ${chart_dir}/values/base.yaml"
    fi
    if [[ -f "${chart_dir}/values/${env}.yaml" ]]; then
        values_args="${values_args} -f ${chart_dir}/values/${env}.yaml"
    fi

    # Render the umbrella chart
    local temp_file
    temp_file=$(mktemp)

    helm template "${group}" "${chart_dir}" \
        --namespace "${group}" \
        --include-crds \
        ${values_args} \
        > "${temp_file}" 2>/dev/null || {
            echo "  Error rendering ${group}"
            rm -f "${temp_file}"
            return 1
        }

    # Split output by component
    split_by_component "${temp_file}" "${output_dir}" "${crds_dir}" "${group}"

    rm -f "${temp_file}"
    echo "  Done with ${group}"
}

split_by_component() {
    local input_file="$1"
    local output_dir="$2"
    local crds_dir="$3"
    local group="$4"

    # Use awk to split the file by component
    # Source comments look like: # Source: core/charts/cert-manager/templates/...
    # or for CRDs: # Source: core/charts/cert-manager/templates/crds/...

    awk -v output_dir="$output_dir" -v crds_dir="$crds_dir" -v group="$group" '
    BEGIN {
        current_component = ""
        current_file = ""
        is_crd = 0
        buffer = ""
    }

    /^---$/ {
        # Flush buffer to current file if we have one
        if (current_file != "" && buffer != "") {
            print buffer >> current_file
        }
        buffer = $0 "\n"
        next
    }

    /^# Source:/ {
        # Extract component name from source path
        # Pattern: # Source: <group>/charts/<component>/...
        # BSD awk compatible: use gsub to extract
        line = $0
        if (match(line, /charts\/[^\/]+\//)) {
            # Extract the matched portion and get component name
            matched = substr(line, RSTART, RLENGTH)
            gsub(/charts\//, "", matched)
            gsub(/\//, "", matched)
            new_component = matched

            # Check if this is a CRD
            is_crd = (line ~ /\/crds\//)

            if (new_component != current_component || (is_crd && current_file !~ /crds/)) {
                # Flush buffer to current file
                if (current_file != "" && buffer != "") {
                    print buffer >> current_file
                }
                buffer = ""

                current_component = new_component
                if (is_crd) {
                    current_file = crds_dir "/" current_component "-crds.yaml"
                } else {
                    current_file = output_dir "/" current_component ".yaml"
                }
            }
        }
        buffer = buffer $0 "\n"
        next
    }

    {
        buffer = buffer $0 "\n"
    }

    END {
        # Flush remaining buffer
        if (current_file != "" && buffer != "") {
            print buffer >> current_file
        }
    }
    ' "$input_file"

    # Report what was created
    local count
    count=$(find "${output_dir}" -maxdepth 1 -name "*.yaml" -type f 2>/dev/null | wc -l | tr -d ' ')
    echo "    Split into ${count} component files"

    # Count CRDs
    local crd_count
    crd_count=$(find "${crds_dir}" -maxdepth 1 -name "*-crds.yaml" -type f 2>/dev/null | wc -l | tr -d ' ')
    if [[ $crd_count -gt 0 ]]; then
        echo "    Created ${crd_count} CRD files"
    fi
}

generate_argo_applicationset() {
    local argo_apps_file="${CLUSTERS_DIR}/applicationset.yaml"

    # Git repository URL - can be overridden via environment variable
    local git_repo_url="${GIT_REPO_URL:-http://gitea-http.gitea.svc:3000/gitea_admin/rendered.git}"

    echo ""
    echo "Generating Argo CD ApplicationSet (rendered mode)..."
    mkdir -p "$CLUSTERS_DIR"

    # Collect environments that were actually rendered
    local rendered_envs=()
    for env in "${ENVIRONMENTS[@]}"; do
        if [[ -d "${RENDERED_DIR}/${env}" ]]; then
            rendered_envs+=("$env")
        fi
    done

    cat > "$argo_apps_file" << 'EOF'
# Multi-environment ApplicationSet for pre-rendered infrastructure manifests
# Uses matrix generator to create apps for all environment × group combinations
#
# This mirrors the original mode's ApplicationSet structure but for pre-rendered
# plain YAML manifests instead of Helm charts.
#
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: infrastructure-rendered
  namespace: argocd
spec:
  generators:
    - matrix:
        generators:
          # Environment generator - define clusters and their properties
          # Add staging/production when those clusters are registered in ArgoCD:
          #   - env: staging
          #     cluster: https://staging.k8s.example.com
          #   - env: production
          #     cluster: https://production.k8s.example.com
          - list:
              elements:
                - env: dev
                  cluster: https://kubernetes.default.svc  # In-cluster for dev
          # Chart group generator
          - list:
              elements:
                - group: core
                  syncWave: "0"
                - group: security
                  syncWave: "1"
                - group: observability
                  syncWave: "2"
                - group: operations
                  syncWave: "3"
  template:
    metadata:
      name: '{{env}}-{{group}}-rendered'
      annotations:
        # Sync wave ensures proper ordering: core -> security -> observability -> operations
        argocd.argoproj.io/sync-wave: '{{syncWave}}'
      labels:
        app.kubernetes.io/part-of: infrastructure-rendered
        app.kubernetes.io/component: '{{group}}'
        environment: '{{env}}'
    spec:
      project: default
      source:
EOF

    cat >> "$argo_apps_file" << EOF
        repoURL: ${git_repo_url}
EOF

    cat >> "$argo_apps_file" << 'EOF'
        targetRevision: main
        path: 'argo/manifests/{{env}}/{{group}}'
      destination:
        server: '{{cluster}}'
        namespace: '{{group}}'
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
          - CreateNamespace=true
          - ServerSideApply=true
        retry:
          limit: 5
          backoff:
            duration: 5s
            factor: 2
            maxDuration: 3m
---
# Separate ApplicationSet for CRDs (applied first via sync wave -1)
# CRDs must be installed before the resources that use them
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: infrastructure-crds-rendered
  namespace: argocd
spec:
  generators:
    # Environment generator for CRDs
    # Add staging/production when those clusters are registered:
    #   - env: staging
    #     cluster: https://staging.k8s.example.com
    #   - env: production
    #     cluster: https://production.k8s.example.com
    - list:
        elements:
          - env: dev
            cluster: https://kubernetes.default.svc
  template:
    metadata:
      name: '{{env}}-crds-rendered'
      annotations:
        argocd.argoproj.io/sync-wave: "-1"
      labels:
        app.kubernetes.io/part-of: infrastructure-rendered
        app.kubernetes.io/component: crds
        environment: '{{env}}'
    spec:
      project: default
      source:
EOF

    cat >> "$argo_apps_file" << EOF
        repoURL: ${git_repo_url}
EOF

    cat >> "$argo_apps_file" << 'EOF'
        targetRevision: main
        path: 'argo/manifests/{{env}}/crds'
      destination:
        server: '{{cluster}}'
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
          - ServerSideApply=true
          - Replace=true
EOF

    echo "  Written to ${argo_apps_file}"
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

clean_rendered_dir() {
    local env="$1"
    local rendered_env_dir="${RENDERED_DIR}/${env}"

    if [[ -d "$rendered_env_dir" ]]; then
        echo "Cleaning ${rendered_env_dir}..."
        rm -rf "$rendered_env_dir"
    fi
}

main() {
    local target_env="${1:-}"
    local target_group="${2:-}"

    # Validate environment if specified
    if [[ -n "${target_env}" ]] && ! contains "${target_env}" "${ENVIRONMENTS[@]}"; then
        echo "Error: Invalid environment '${target_env}'"
        echo "Valid environments: ${ENVIRONMENTS[*]}"
        exit 1
    fi

    # Validate group if specified
    if [[ -n "${target_group}" ]] && ! contains "${target_group}" "${CHART_GROUPS[@]}"; then
        echo "Error: Invalid group '${target_group}'"
        echo "Valid groups: ${CHART_GROUPS[*]}"
        exit 1
    fi

    # Determine what to render
    local envs_to_render=("${ENVIRONMENTS[@]}")
    if [[ -n "${target_env}" ]]; then
        envs_to_render=("${target_env}")
    fi

    local groups_to_render=("${CHART_GROUPS[@]}")
    if [[ -n "${target_group}" ]]; then
        groups_to_render=("${target_group}")
    fi

    # Render
    for env in "${envs_to_render[@]}"; do
        echo ""
        echo "=== Rendering for ${env} ==="

        # Clean existing rendered output for this env
        clean_rendered_dir "$env"

        for group in "${groups_to_render[@]}"; do
            render_group "${env}" "${group}"
        done
    done

    # Generate single multi-environment Argo CD ApplicationSet
    generate_argo_applicationset

    echo ""
    echo "Done! Rendered manifests are in ${RENDERED_DIR}/"
}

main "$@"
