#!/usr/bin/env bash
set -euo pipefail

# Setup a kind cluster with Argo CD and Gitea for testing
# Usage: ./setup-argo-cluster.sh [cluster-name]

CLUSTER_NAME="${1:-argo-test}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$SCRIPT_DIR")"

# Gitea configuration
GITEA_NAMESPACE="gitea"
GITEA_USER="gitea_admin"
GITEA_PASSWORD="admin123"
GITEA_REPO_RENDERED="rendered"
GITEA_REPO_ORIGINAL="argo-umbrella-charts"

echo "=== Setting up kind cluster '${CLUSTER_NAME}' with Argo CD and Gitea ==="

# Check dependencies
for cmd in kind kubectl helm argocd; do
    if ! command -v "$cmd" &> /dev/null; then
        echo "Error: $cmd is required but not installed."
        case "$cmd" in
            argocd) echo "Install with: brew install argocd" ;;
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
    kind create cluster --name "$CLUSTER_NAME" --config "${SCRIPT_DIR}/kind-config-argo.yaml" --wait 60s
fi

# Set kubectl context
kubectl cluster-info --context "kind-${CLUSTER_NAME}"

# Install Argo CD
echo "Installing Argo CD..."
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

echo "Waiting for Argo CD to be ready..."
kubectl wait --for=condition=available --timeout=300s deployment/argocd-server -n argocd
kubectl wait --for=condition=available --timeout=300s deployment/argocd-repo-server -n argocd
kubectl wait --for=condition=available --timeout=300s deployment/argocd-applicationset-controller -n argocd

# Patch Argo CD server to use NodePort for local access (mapped via Kind config)
echo "Patching Argo CD server to NodePort..."
kubectl patch svc argocd-server -n argocd -p '{"spec": {"type": "NodePort", "ports": [{"name": "http", "port": 80, "nodePort": 30080, "protocol": "TCP", "targetPort": 8080}, {"name": "https", "port": 443, "nodePort": 30443, "protocol": "TCP", "targetPort": 8080}]}}'

# Enable insecure mode for HTTP access (easier for local testing)
echo "Configuring Argo CD for HTTP access..."
kubectl patch configmap argocd-cmd-params-cm -n argocd --type merge -p '{"data":{"server.insecure":"true"}}' 2>/dev/null || \
  kubectl create configmap argocd-cmd-params-cm -n argocd --from-literal=server.insecure=true
kubectl rollout restart deployment/argocd-server -n argocd
kubectl rollout status deployment/argocd-server -n argocd --timeout=120s
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=60s

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
echo "Creating repository '${GITEA_REPO_RENDERED}' in Gitea (for rendered mode)..."
curl -s -X POST "http://localhost:3030/api/v1/user/repos" \
  -u "${GITEA_USER}:${GITEA_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"${GITEA_REPO_RENDERED}\", \"private\": false}" >/dev/null 2>&1 || true

echo "Creating repository '${GITEA_REPO_ORIGINAL}' in Gitea (for original mode)..."
curl -s -X POST "http://localhost:3030/api/v1/user/repos" \
  -u "${GITEA_USER}:${GITEA_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"name\": \"${GITEA_REPO_ORIGINAL}\", \"private\": false}" >/dev/null 2>&1 || true

# Get Argo CD admin password
ARGOCD_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" 2>/dev/null | base64 -d 2>/dev/null || echo "")

# Configure Argo CD to access Gitea
echo "Configuring Argo CD to access Gitea..."

if [[ -z "$ARGOCD_PASSWORD" ]]; then
    echo "Warning: Could not retrieve Argo CD admin password, skipping Argo CD configuration"
else
    # Wait for Argo CD to be accessible via NodePort
    echo "Waiting for Argo CD to be accessible..."
    max_attempts=30
    attempt=0
    while [[ $attempt -lt $max_attempts ]]; do
        if curl -s -o /dev/null -w "%{http_code}" http://localhost:9080 2>/dev/null | grep -q "200\|401\|403"; then
            echo "Argo CD is accessible"
            break
        fi
        attempt=$((attempt + 1))
        sleep 1
    done

    if [[ $attempt -eq $max_attempts ]]; then
        echo "Warning: Failed to connect to Argo CD at localhost:9080, skipping configuration"
    else
        # Login to Argo CD (use --plaintext for HTTP connections)
        if argocd login localhost:9080 --plaintext --username admin --password "$ARGOCD_PASSWORD" >/dev/null 2>&1; then
            echo "Logged in to Argo CD"

            # Add Gitea repos to Argo CD (using in-cluster URL)
            for repo in "$GITEA_REPO_RENDERED" "$GITEA_REPO_ORIGINAL"; do
                if argocd repo add "http://gitea-http.gitea.svc:3000/${GITEA_USER}/${repo}.git" \
                    --username "$GITEA_USER" \
                    --password "$GITEA_PASSWORD" \
                    --insecure-skip-server-verification >/dev/null 2>&1; then
                    echo "Added ${repo} repository to Argo CD"
                else
                    echo "Warning: Failed to add ${repo} repository to Argo CD (may already exist)"
                fi
            done
        else
            echo "Warning: Failed to login to Argo CD"
        fi
    fi
fi

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Cluster: ${CLUSTER_NAME}"
echo ""
echo "--- Argo CD ---"
if [[ -n "$ARGOCD_PASSWORD" ]]; then
    echo "Admin password: ${ARGOCD_PASSWORD}"
else
    echo "Admin password: (run 'kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath=\"{.data.password}\" | base64 -d' to retrieve)"
fi
echo ""
echo "Argo CD UI: http://localhost:9080"
echo ""
echo "--- Gitea ---"
echo "Credentials: ${GITEA_USER} / ${GITEA_PASSWORD}"
echo "In-cluster URL: http://gitea-http.gitea.svc:3000"
echo "Gitea UI: http://localhost:3030"
echo ""
echo "Gitea repos:"
echo "  - ${GITEA_REPO_RENDERED} (for rendered mode)"
echo "  - ${GITEA_REPO_ORIGINAL} (for original mode)"
echo ""
echo "--- Testing ---"
echo "  # Rendered mode (pre-rendered YAML):"
echo "  ./scripts/test-argo-rendered.sh dev"
echo ""
echo "  # Original mode (Helm source type):"
echo "  ./scripts/test-argo-original.sh dev"
echo ""
