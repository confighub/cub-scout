#!/usr/bin/env bash
set -euo pipefail

# Post-process rendered Helm output:
# For Argo: Keep CRDs with resources, use sync-waves for ordering, prepend Namespace
# For Flux: Split CRDs into separate files for Kustomization ordering
# Both: Strip Helm hooks (test hooks, lifecycle hooks, pre/post-install hooks)
#
# Usage: post-process.sh <operator> <env/group> <chart> <input-file> <output-dir> [namespace]
# Example: post-process.sh argo dev/core cert-manager rendered/dev/cert-manager.yaml.tmp rendered cert-manager
# Example: post-process.sh flux dev/core cert-manager /tmp/input.yaml /custom/rendered

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

# Check for yq
if ! command -v yq &> /dev/null; then
    echo "Error: yq is required but not installed."
    echo "Install with: brew install yq"
    exit 1
fi

process_file() {
    local operator="$1"
    local env_group="$2"  # e.g., "dev/core"
    local chart="$3"
    local input_file="$4"
    local output_base="${5:-${REPO_ROOT}/rendered}"
    local namespace="${6:-}"  # Optional namespace for Argo

    local rendered_dir="${output_base}/${env_group}"
    local output_file="${rendered_dir}/${chart}.yaml"

    # Create temp files
    local tmp_crds=$(mktemp)
    local tmp_resources=$(mktemp)
    trap "rm -f $tmp_crds $tmp_resources" EXIT

    echo "  Processing ${chart} for ${env_group} (${operator})..."

    # Extract CRDs
    yq eval 'select(.kind == "CustomResourceDefinition")' "$input_file" > "$tmp_crds" 2>/dev/null || true

    # Extract non-CRDs (resources), excluding:
    # - Test hooks (helm.sh/hook containing "test")
    # - Lifecycle hook Jobs (app.kubernetes.io/component: hooks) - used by kyverno and others
    # These hooks are meant for Helm lifecycle events, not GitOps
    yq eval '
        select(.kind != "CustomResourceDefinition") |
        select((.metadata.annotations."helm.sh/hook" | test("test") // false) | not) |
        select((.metadata.labels."app.kubernetes.io/component" == "hooks" and .kind == "Job") | not)
    ' "$input_file" > "$tmp_resources" 2>/dev/null || true

    # Process based on operator
    if [[ "$operator" == "argo" ]]; then
        process_argo "$tmp_crds" "$tmp_resources" "$output_file" "$namespace"
    elif [[ "$operator" == "flux" ]]; then
        local crds_dir="${rendered_dir}/crds"
        local crds_file="${crds_dir}/${chart}.yaml"
        mkdir -p "$crds_dir"
        process_flux "$tmp_crds" "$tmp_resources" "$crds_file" "$output_file"
        # Remove CRDs file if empty
        if [[ ! -s "$crds_file" ]] || [[ $(yq eval 'length' "$crds_file" 2>/dev/null || echo "0") == "0" ]]; then
            rm -f "$crds_file"
        fi
    else
        echo "Error: Unknown operator '$operator'. Use 'argo' or 'flux'."
        exit 1
    fi
}

process_argo() {
    local crds_in="$1"
    local resources_in="$2"
    local output_file="$3"
    local namespace="${4:-}"

    local tmp_crds_processed=$(mktemp)
    local tmp_resources_processed=$(mktemp)

    # System namespaces that shouldn't have Namespace resources created
    local system_ns="default kube-system kube-public kube-node-lease argocd"

    # Start output file - prepend Namespace resource with sync-wave: -2 if needed
    : > "$output_file"
    if [[ -n "$namespace" ]] && ! echo "$system_ns" | grep -qw "$namespace"; then
        cat >> "$output_file" << EOF
---
apiVersion: v1
kind: Namespace
metadata:
  name: ${namespace}
  annotations:
    argocd.argoproj.io/sync-wave: "-2"
EOF
    fi

    # Add sync-wave: -1 to CRDs (apply after namespace, before resources)
    if [[ -s "$crds_in" ]]; then
        yq eval '
            (select(.kind == "CustomResourceDefinition") | .metadata.annotations."argocd.argoproj.io/sync-wave") = "-1"
        ' "$crds_in" > "$tmp_crds_processed"
    fi

    # Strip helm hook annotations from resources
    yq eval '
        del(.metadata.annotations."helm.sh/hook") |
        del(.metadata.annotations."helm.sh/hook-weight") |
        del(.metadata.annotations."helm.sh/hook-delete-policy")
    ' "$resources_in" > "$tmp_resources_processed"

    # Append CRDs (sync-wave: -1)
    if [[ -s "$tmp_crds_processed" ]]; then
        echo "---" >> "$output_file"
        cat "$tmp_crds_processed" >> "$output_file"
    fi

    # Append resources (sync-wave: 0, default)
    echo "---" >> "$output_file"
    cat "$tmp_resources_processed" >> "$output_file"

    rm -f "$tmp_crds_processed" "$tmp_resources_processed"
}

process_flux() {
    local crds_in="$1"
    local resources_in="$2"
    local crds_out="$3"
    local resources_out="$4"

    # CRDs: just copy (no special annotations needed, ordering via Kustomization)
    if [[ -s "$crds_in" ]]; then
        cp "$crds_in" "$crds_out"
    fi

    # Resources: remove helm hook annotations (Flux doesn't use them)
    # Hooks will be handled by Kustomization ordering
    yq eval '
        del(.metadata.annotations."helm.sh/hook") |
        del(.metadata.annotations."helm.sh/hook-weight") |
        del(.metadata.annotations."helm.sh/hook-delete-policy")
    ' "$resources_in" > "$resources_out"
}

# Main
if [[ $# -lt 4 ]]; then
    echo "Usage: $0 <operator> <env/group> <chart> <input-file> <output-dir> [namespace]"
    echo "  operator: argo or flux"
    echo "  env/group: e.g., dev/core"
    echo "  chart: chart name (e.g., cert-manager)"
    echo "  input-file: path to rendered YAML file"
    echo "  output-dir: base output directory"
    echo "  namespace: (argo only) namespace for the chart"
    exit 1
fi

process_file "$1" "$2" "$3" "$4" "${5:-}" "${6:-}"
