#!/usr/bin/env bash
set -euo pipefail

# Setup a kind cluster with Flux and Gitea for testing
# Usage: ./setup-flux-cluster.sh [cluster-name]

CLUSTER_NAME="${1:-flux-test}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

# Gitea configuration
GITEA_NAMESPACE="gitea"
GITEA_USER="gitea_admin"
GITEA_PASSWORD="admin123"
GITEA_REPO_ORIGINAL="flux-helm-kustomize"
GITEA_REPO_RENDERED="rendered"

echo "=== Setting up kind cluster '${CLUSTER_NAME}' with Flux and Gitea ==="

# Check dependencies
for cmd in kind kubectl flux helm; do
    if ! command -v "$cmd" &> /dev/null; then
        echo "Error: $cmd is required but not installed."
        case "$cmd" in
            flux) echo "Install with: brew install fluxcd/tap/flux" ;;
            helm) echo "Install with: brew install helm" ;;
        esac
        exit 1
    fi
done

# Create kind cluster if it doesn't exist
if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster '${CLUSTER_NAME}' already exists"
else
    echo "Creating kind cluster '${CLUSTER_NAME}'..."
    kind create cluster --name "$CLUSTER_NAME" --config "${SCRIPT_DIR}/kind-config-flux.yaml" --wait 60s
fi

# Set kubectl context
kubectl cluster-info --context "kind-${CLUSTER_NAME}"

# Install Flux
echo "Installing Flux..."
flux install

echo "Waiting for Flux to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/source-controller -n flux-system
kubectl wait --for=condition=available --timeout=300s deployment/kustomize-controller -n flux-system

echo ""
echo "=== Installing Gitea for local git server ==="

# Add Gitea helm repo
helm repo add gitea https://dl.gitea.com/charts/ 2>/dev/null || true
helm repo update gitea >/dev/null

# Create namespace
kubectl create namespace "$GITEA_NAMESPACE" 2>/dev/null || true

# Install Gitea with minimal resources (SQLite, no Redis, no HA)
echo "Installing Gitea (this may take a few minutes)..."
helm upgrade --install gitea gitea/gitea \
  --namespace "$GITEA_NAMESPACE" \
  --set replicaCount=1 \
  --set persistence.enabled=false \
  --set gitea.admin.username="$GITEA_USER" \
  --set gitea.admin.password="$GITEA_PASSWORD" \
  --set gitea.config.server.ROOT_URL=http://gitea-http.gitea.svc:3000 \
  --set gitea.config.database.DB_TYPE=sqlite3 \
  --set gitea.config.session.PROVIDER=memory \
  --set gitea.config.cache.ADAPTER=memory \
  --set gitea.config.queue.TYPE=level \
  --set postgresql-ha.enabled=false \
  --set postgresql.enabled=false \
  --set valkey-cluster.enabled=false \
  --set test.enabled=false \
  --set service.http.type=NodePort \
  --set service.http.nodePort=30300 \
  --set-string service.http.clusterIP="" \
  --wait --timeout 3m

echo "Waiting for Gitea to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/gitea -n "$GITEA_NAMESPACE"
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=gitea -n "$GITEA_NAMESPACE" --timeout=60s

# Wait for Gitea to be accessible via NodePort
echo "Waiting for Gitea to be accessible..."
max_attempts=30
attempt=0
while [[ $attempt -lt $max_attempts ]]; do
    if curl -s -o /dev/null -w "%{http_code}" "http://localhost:3030" 2>/dev/null | grep -q "200\|302"; then
        echo "Gitea is accessible"
        break
    fi
    attempt=$((attempt + 1))
    sleep 1
done

if [[ $attempt -eq $max_attempts ]]; then
    echo "Error: Failed to connect to Gitea at localhost:3030"
    exit 1
fi

# Create repositories in Gitea
echo "Creating repository '${GITEA_REPO_ORIGINAL}' in Gitea (for original/HelmRelease mode)..."
curl -s -X POST "http://localhost:3030/api/v1/user/repos" \
  -u "${GITEA_USER}:${GITEA_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"${GITEA_REPO_ORIGINAL}\", \"private\": false}" >/dev/null 2>&1 || true

echo "Creating repository '${GITEA_REPO_RENDERED}' in Gitea (for rendered/plain YAML mode)..."
curl -s -X POST "http://localhost:3030/api/v1/user/repos" \
  -u "${GITEA_USER}:${GITEA_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"${GITEA_REPO_RENDERED}\", \"private\": false}" >/dev/null 2>&1 || true

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Cluster: ${CLUSTER_NAME}"
echo ""
echo "--- Gitea ---"
echo "Credentials: ${GITEA_USER} / ${GITEA_PASSWORD}"
echo "In-cluster URL: http://gitea-http.gitea.svc:3000"
echo ""
echo "Gitea UI: http://localhost:3030"
echo ""
echo "Gitea repos:"
echo "  - ${GITEA_REPO_ORIGINAL} (for original/HelmRelease mode)"
echo "  - ${GITEA_REPO_RENDERED} (for rendered/plain YAML mode)"
echo ""
echo "--- Testing ---"
echo ""
echo "  # Test 'original' mode (HelmRelease CRs):"
echo "  ./scripts/test-flux-original.sh dev"
echo ""
echo "  # Test 'rendered' mode (plain YAML):"
echo "  ./scripts/test-flux-rendered.sh dev"
echo ""
echo "To check Flux status:"
echo "  flux get all"
echo ""
