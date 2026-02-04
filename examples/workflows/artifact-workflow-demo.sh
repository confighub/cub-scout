#!/usr/bin/env bash
# Artifact Workflow Demo — v0.18 (#103, #104)
#
# This script demonstrates the end-to-end artifact workflow:
#   1. CI produces a debug bundle (with git context)
#   2. Developer inspects the bundle (sees repo/commit/branch)
#   3. Developer replays sections
#   4. Developer exports human-readable reports
#
# All operations are file-based and reproducible.
# No live cluster access required for replay.
# Git context is pure metadata — replay semantics are unchanged.
#
# Usage:
#   ./examples/workflows/artifact-workflow-demo.sh
#
# Requirements:
#   - cub-scout binary built (go build ./cmd/cub-scout)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

# Ensure binary exists
if [[ ! -x "./cub-scout" ]]; then
  echo "Building cub-scout..."
  go build ./cmd/cub-scout
fi

# Create a temporary workspace
WORKSPACE="$(mktemp -d)"
trap 'rm -rf "$WORKSPACE"' EXIT

BUNDLE_DIR="$WORKSPACE/debug-bundle-demo"
REPORTS_DIR="$WORKSPACE/reports"

echo "=============================================="
echo "  Artifact Workflow Demo"
echo "=============================================="
echo ""

# -----------------------------------------------------------------------------
# Step 1: Simulate CI producing a bundle
# -----------------------------------------------------------------------------
echo "Step 1: CI produces a debug bundle"
echo "-----------------------------------"
echo ""

# Create a minimal test bundle (simulating what CI would produce)
mkdir -p "$BUNDLE_DIR"

# metadata.json (with git context - v0.18+)
cat > "$BUNDLE_DIR/metadata.json" << 'METADATA'
{
  "formatVersion": "v1",
  "cubScoutVersion": "0.18.0",
  "createdAt": "2026-02-04T10:00:00Z",
  "label": "ci-run-12345",
  "target": {
    "kind": "Deployment",
    "name": "web-frontend",
    "namespace": "production"
  },
  "contents": {
    "hasSession": true,
    "hasDrift": true,
    "hasEvents": false,
    "hasLogs": false,
    "hasAttribution": true
  },
  "gitContext": {
    "repoRoot": "/home/runner/work/myapp/myapp",
    "commitSHA": "abc123def456789",
    "branch": "main",
    "remoteURL": "https://github.com/myorg/myapp.git"
  }
}
METADATA

# session.json
cat > "$BUNDLE_DIR/session.json" << 'SESSION'
{
  "target": {
    "kind": "Deployment",
    "name": "web-frontend",
    "namespace": "production"
  },
  "startedAt": "2026-02-04T10:00:00Z",
  "completedAt": "2026-02-04T10:00:05Z"
}
SESSION

# drift.json
cat > "$BUNDLE_DIR/drift.json" << 'DRIFT'
[
  {
    "schema_version": "drift.v1",
    "object": {
      "kind": "Deployment",
      "name": "web-frontend",
      "namespace": "production"
    },
    "severity": "warning",
    "category": "image",
    "field": "spec.template.spec.containers[0].image",
    "expected": "myapp:v1.2.3",
    "actual": "myapp:v1.2.4-hotfix",
    "message": "Container image differs from git source"
  }
]
DRIFT

# attribution.json
cat > "$BUNDLE_DIR/attribution.json" << 'ATTRIBUTION'
{
  "schema_version": "attribution-graph.v1",
  "generated_from": {
    "bundle_id": "ci-run-12345",
    "created_at": "2026-02-04T10:00:00Z",
    "tool_version": "0.16.0"
  },
  "nodes": [
    {
      "id": "k8s:Deployment:production:web-frontend",
      "type": "k8s_object",
      "ref": {
        "kind": "Deployment",
        "name": "web-frontend",
        "namespace": "production"
      },
      "present": true
    },
    {
      "id": "kustomize:overlays/production",
      "type": "kustomize_overlay",
      "ref": {
        "kind": "Kustomization",
        "name": "overlays/production"
      },
      "present": true
    }
  ],
  "edges": [
    {
      "id": "edge:kustomize-owns-deployment",
      "type": "owns",
      "from": "kustomize:overlays/production",
      "to": "k8s:Deployment:production:web-frontend",
      "evidence": "kustomize_overlay"
    }
  ],
  "summary": {
    "nodes_by_type": {
      "k8s_object": 1,
      "kustomize_overlay": 1
    },
    "edges_by_type": {
      "owns": 1
    },
    "unattributed_count": 0,
    "ambiguous_count": 0
  }
}
ATTRIBUTION

# README.md
cat > "$BUNDLE_DIR/README.md" << 'README'
# Debug Bundle: ci-run-12345

**Target:** Deployment/web-frontend (production)
**Created:** 2026-02-04T10:00:00Z
**Tool Version:** cub-scout 0.18.0

## Git Context

- **Repo:** /home/runner/work/myapp/myapp
- **Commit:** abc123def456789
- **Branch:** main
- **Remote:** https://github.com/myorg/myapp.git

## Contents

- session.json: Debug session metadata
- drift.json: 1 drift finding (warning)
- attribution.json: Ownership graph

## Quick Replay

```bash
cub-scout bundle inspect ./
cub-scout bundle replay ./ --section drift
cub-scout bundle replay ./ --section attribution
```
README

echo "Bundle created at: $BUNDLE_DIR"
echo "Contents:"
ls -la "$BUNDLE_DIR"
echo ""

# -----------------------------------------------------------------------------
# Step 2: Developer inspects the bundle
# -----------------------------------------------------------------------------
echo "Step 2: Developer inspects the bundle"
echo "--------------------------------------"
echo ""

./cub-scout bundle inspect "$BUNDLE_DIR"
echo ""

# -----------------------------------------------------------------------------
# Step 3: Developer replays sections
# -----------------------------------------------------------------------------
echo "Step 3: Developer replays drift findings"
echo "-----------------------------------------"
echo ""

./cub-scout bundle replay "$BUNDLE_DIR" --section drift
echo ""

echo "Step 3b: Developer replays attribution graph"
echo "---------------------------------------------"
echo ""

./cub-scout bundle replay "$BUNDLE_DIR" --section attribution
echo ""

# -----------------------------------------------------------------------------
# Step 4: Developer exports reports
# -----------------------------------------------------------------------------
echo "Step 4: Developer exports reports"
echo "----------------------------------"
echo ""

mkdir -p "$REPORTS_DIR"

# Export drift as JSON
./cub-scout bundle replay "$BUNDLE_DIR" --section drift --format json > "$REPORTS_DIR/drift.json"
echo "Exported: $REPORTS_DIR/drift.json"

# Export attribution as JSON
./cub-scout bundle replay "$BUNDLE_DIR" --section attribution --format json > "$REPORTS_DIR/attribution.json"
echo "Exported: $REPORTS_DIR/attribution.json"

# Export human-readable versions
./cub-scout bundle replay "$BUNDLE_DIR" --section drift > "$REPORTS_DIR/drift.txt"
echo "Exported: $REPORTS_DIR/drift.txt"

./cub-scout bundle replay "$BUNDLE_DIR" --section attribution > "$REPORTS_DIR/attribution.txt"
echo "Exported: $REPORTS_DIR/attribution.txt"

echo ""
echo "Report files:"
ls -la "$REPORTS_DIR"
echo ""

# -----------------------------------------------------------------------------
# Summary
# -----------------------------------------------------------------------------
echo "=============================================="
echo "  Demo Complete"
echo "=============================================="
echo ""
echo "This workflow demonstrated:"
echo "  1. CI produces artifacts (bundle directory with git context)"
echo "  2. Developer downloads and inspects locally (sees repo/commit/branch)"
echo "  3. Developer replays any section offline"
echo "  4. Developer exports reports (JSON + human-readable)"
echo ""
echo "All operations are:"
echo "  - File-based (no cluster access for replay)"
echo "  - Deterministic (same input = same output)"
echo "  - Self-contained (bundle has everything needed)"
echo "  - Git-aware (repo, commit, branch captured as context)"
echo ""
echo "Git context is pure metadata — replay semantics are unchanged."
echo ""
echo "For real CI usage, replace step 1 with:"
echo "  cub-scout debug deploy/web-frontend -n production --non-interactive --save-bundle ./artifacts/bundle"
echo ""
echo "Git context is automatically captured when running in a git repository."
echo ""
