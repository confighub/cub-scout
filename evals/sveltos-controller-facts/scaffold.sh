#!/usr/bin/env bash
set -euo pipefail
mkdir -p cluster
cat > cluster/observations.yaml <<'CUB_SCOUT_EVAL_EOF'
# Authored contract fixture, not a live capture.
# Field shapes inspected in sveltos-confighub source 8187910f9fe226e109e55c4d9c7c0e21297ff424.
apiVersion: config.projectsveltos.io/v1beta1
kind: ClusterSummary
metadata:
  name: delivery-prod
  namespace: management
  uid: summary-instance
spec:
  clusterNamespace: projectsveltos
  clusterName: prod
  clusterType: Sveltos
status:
  featureSummaries:
    - featureID: Resources
      status: Provisioned
      lastAppliedTime: "2026-09-28T10:00:00Z"
---
apiVersion: lib.projectsveltos.io/v1beta1
kind: ClusterHealthCheck
metadata:
  name: workload-watch
  namespace: ""
  uid: watch-instance
status:
  clusterCondition:
    - clusterInfo:
        cluster:
          apiVersion: lib.projectsveltos.io/v1beta1
          kind: SveltosCluster
          namespace: projectsveltos
          name: prod
          uid: target-instance
      conditions:
        - type: HealthCheck:workloads
          status: "False"
          reason: ReportedUnhealthy
          message: "Deployment api: zero available replicas"
          lastTransitionTime: "2026-09-28T10:05:00Z"
CUB_SCOUT_EVAL_EOF
