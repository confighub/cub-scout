#!/bin/bash
# Create drift scenario - simulates 2am kubectl editing

set -e

NAMESPACE="apptique-drift"
DEPLOYMENT="frontend"

echo "=== Drift Detection Demo ==="
echo ""
echo "This script simulates someone editing production directly with kubectl."
echo ""

# Check if namespace exists
if ! kubectl get namespace "$NAMESPACE" &>/dev/null; then
    echo "ERROR: Namespace $NAMESPACE doesn't exist."
    echo "Run: kubectl apply -f base-deployment.yaml"
    exit 1
fi

echo "Before drift:"
kubectl get deployment "$DEPLOYMENT" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}' | xargs -I{} echo "  replicas: {}"
echo ""

echo "Creating drift..."
echo ""

# Drift 1: Change replicas from 3 to 10
echo "1. Scaling replicas: 3 -> 10"
kubectl patch deployment "$DEPLOYMENT" -n "$NAMESPACE" \
  -p '{"spec":{"replicas":10}}'

# Drift 2: Add DEBUG environment variable
echo "2. Adding DEBUG=true env var"
kubectl patch deployment "$DEPLOYMENT" -n "$NAMESPACE" \
  --type='json' \
  -p='[{"op": "add", "path": "/spec/template/spec/containers/0/env/-", "value": {"name": "DEBUG", "value": "true"}}]'

echo ""
echo "After drift:"
kubectl get deployment "$DEPLOYMENT" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}' | xargs -I{} echo "  replicas: {}"
kubectl get deployment "$DEPLOYMENT" -n "$NAMESPACE" -o jsonpath='{.spec.template.spec.containers[0].env[*].name}' | xargs -I{} echo "  env vars: {}"
echo ""

echo "=== Drift Created ==="
echo ""
echo "Now detect it with:"
echo "  ./cub-agent trace deployment/frontend -n apptique-drift"
echo "  ./test/atk/map problems"
echo ""
echo "To remediate:"
echo "  kubectl apply -f base-deployment.yaml"
