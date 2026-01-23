#!/bin/bash
# Script to capture a nice workloads view screenshot for cub-scout
# This creates a kind cluster with diverse ownership and captures the TUI

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLUSTER_NAME="cub-scout-demo"

echo "🎬 Setting up demo cluster for screenshot..."

# Check prerequisites
if ! command -v kind &> /dev/null; then
    echo "❌ kind not found. Install from: https://kind.sigs.k8s.io/"
    exit 1
fi

if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl not found. Install from: https://kubernetes.io/docs/tasks/tools/"
    exit 1
fi

# Create kind cluster
echo "📦 Creating kind cluster: $CLUSTER_NAME"
kind create cluster --name "$CLUSTER_NAME" --wait 60s

# Apply diverse workloads
echo "🚀 Deploying workloads with diverse ownership..."
kubectl apply -f "$SCRIPT_DIR/diverse-ownership-demo.yaml"

# Wait for deployments to be ready
echo "⏳ Waiting for pods to be ready..."
sleep 10

# Show cluster info
echo ""
echo "✅ Demo cluster ready!"
echo ""
echo "Workloads by owner:"
kubectl get deploy,sts --all-namespaces --show-labels | grep -E "flux|argo|helm|confighub" | wc -l | xargs echo "  GitOps-managed:"
kubectl get deploy,sts --all-namespaces --show-labels | grep -v -E "flux|argo|helm|confighub" | grep -v "NAMESPACE" | wc -l | xargs echo "  Native:"

echo ""
echo "📸 Now run cub-scout to capture screenshot:"
echo ""
echo "  cd $SCRIPT_DIR/../.."
echo "  go build ./cmd/cub-scout"
echo "  ./cub-scout map"
echo ""
echo "  Press 'w' for workloads view"
echo "  Take screenshot (Cmd+Shift+4 on Mac)"
echo ""
echo "🧹 To cleanup when done:"
echo "  kind delete cluster --name $CLUSTER_NAME"
