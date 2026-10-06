#!/usr/bin/env bash
set -euo pipefail
mkdir -p evidence
cat > evidence/watch-identity.json <<'JSON'
{"provenance":"authored product-contract fixture; not a live capture","observedAt":"2026-10-06T08:00:00Z","events":[{"type":"cluster.observed","cluster":{"id":"cluster-a","identity":"verified","cost":{"requestsMade":1}},"clusterCostScope":"identity-reader"},{"type":"resource.deleted","resourceIdentity":{"observed":{"clusterId":"cluster-a","group":"apps","kind":"Deployment","namespace":"team-a","name":"api","uid":"old"}},"cluster":{"cost":{"requestsMade":1}},"clusterCostScope":"identity-reader"},{"type":"resource.discovered","resourceIdentity":{"observed":{"clusterId":"cluster-a","group":"apps","kind":"Deployment","namespace":"team-a","name":"api","uid":"new"}},"cluster":{"cost":{"requestsMade":1}},"clusterCostScope":"identity-reader"}],"separateDeniedCycle":{"cluster":{"identity":"unverified","omission":"forbidden"},"resourceIdentity":{"status":"unverified","omission":"cluster_identity_unverified"}}}
JSON
