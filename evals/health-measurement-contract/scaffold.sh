#!/usr/bin/env bash
# FIXTURE-OWNED: regenerated from case fixtures only; record.py preserves this script.
# Deterministic, case-scoped export and genuine sequential proof records.
set -euo pipefail
mkdir -p cluster evidence/health-measurement-contract
cat > cluster/deployments.yaml <<'CUB_SCOUT_EVAL_EOF'
{
  "apiVersion": "apps/v1",
  "kind": "DeploymentList",
  "items": [
    {
      "apiVersion": "apps/v1",
      "kind": "Deployment",
      "metadata": {
        "annotations": {
          "deployment.kubernetes.io/revision": "1",
          "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"annotations\":{},\"labels\":{\"app\":\"auth\"},\"name\":\"auth\",\"namespace\":\"team-02\"},\"spec\":{\"replicas\":0,\"selector\":{\"matchLabels\":{\"app\":\"auth\"}},\"template\":{\"metadata\":{\"labels\":{\"app\":\"auth\"}},\"spec\":{\"containers\":[{\"image\":\"registry.k8s.io/pause:3.9\",\"name\":\"auth\"}]}}}}\n"
        },
        "creationTimestamp": "2026-09-27T12:38:31Z",
        "generation": 1,
        "labels": {
          "app": "auth"
        },
        "managedFields": [
          {
            "apiVersion": "apps/v1",
            "fieldsType": "FieldsV1",
            "fieldsV1": {
              "f:metadata": {
                "f:annotations": {
                  "f:deployment.kubernetes.io/revision": {}
                }
              },
              "f:status": {
                "f:conditions": {
                  ".": {},
                  "k:{\"type\":\"Available\"}": {
                    ".": {},
                    "f:lastTransitionTime": {},
                    "f:lastUpdateTime": {},
                    "f:message": {},
                    "f:reason": {},
                    "f:status": {},
                    "f:type": {}
                  },
                  "k:{\"type\":\"Progressing\"}": {
                    ".": {},
                    "f:lastTransitionTime": {},
                    "f:lastUpdateTime": {},
                    "f:message": {},
                    "f:reason": {},
                    "f:status": {},
                    "f:type": {}
                  }
                },
                "f:observedGeneration": {},
                "f:terminatingReplicas": {}
              }
            },
            "manager": "kube-controller-manager",
            "operation": "Update",
            "subresource": "status",
            "time": "2026-09-27T12:38:31Z"
          },
          {
            "apiVersion": "apps/v1",
            "fieldsType": "FieldsV1",
            "fieldsV1": {
              "f:metadata": {
                "f:annotations": {
                  ".": {},
                  "f:kubectl.kubernetes.io/last-applied-configuration": {}
                },
                "f:labels": {
                  ".": {},
                  "f:app": {}
                }
              },
              "f:spec": {
                "f:progressDeadlineSeconds": {},
                "f:replicas": {},
                "f:revisionHistoryLimit": {},
                "f:selector": {},
                "f:strategy": {
                  "f:rollingUpdate": {
                    ".": {},
                    "f:maxSurge": {},
                    "f:maxUnavailable": {}
                  },
                  "f:type": {}
                },
                "f:template": {
                  "f:metadata": {
                    "f:labels": {
                      ".": {},
                      "f:app": {}
                    }
                  },
                  "f:spec": {
                    "f:containers": {
                      "k:{\"name\":\"auth\"}": {
                        ".": {},
                        "f:image": {},
                        "f:imagePullPolicy": {},
                        "f:name": {},
                        "f:resources": {},
                        "f:terminationMessagePath": {},
                        "f:terminationMessagePolicy": {}
                      }
                    },
                    "f:dnsPolicy": {},
                    "f:restartPolicy": {},
                    "f:schedulerName": {},
                    "f:securityContext": {},
                    "f:terminationGracePeriodSeconds": {}
                  }
                }
              }
            },
            "manager": "kubectl-client-side-apply",
            "operation": "Update",
            "time": "2026-09-27T12:38:31Z"
          }
        ],
        "name": "auth",
        "namespace": "team-02",
        "resourceVersion": "530",
        "uid": "dfc78f8e-d712-444e-af6b-54caeb4774bd"
      },
      "spec": {
        "progressDeadlineSeconds": 600,
        "replicas": 0,
        "revisionHistoryLimit": 10,
        "selector": {
          "matchLabels": {
            "app": "auth"
          }
        },
        "strategy": {
          "rollingUpdate": {
            "maxSurge": "25%",
            "maxUnavailable": "25%"
          },
          "type": "RollingUpdate"
        },
        "template": {
          "metadata": {
            "labels": {
              "app": "auth"
            }
          },
          "spec": {
            "containers": [
              {
                "image": "registry.k8s.io/pause:3.9",
                "imagePullPolicy": "IfNotPresent",
                "name": "auth",
                "resources": {},
                "terminationMessagePath": "/dev/termination-log",
                "terminationMessagePolicy": "File"
              }
            ],
            "dnsPolicy": "ClusterFirst",
            "restartPolicy": "Always",
            "schedulerName": "default-scheduler",
            "securityContext": {},
            "terminationGracePeriodSeconds": 30
          }
        }
      },
      "status": {
        "conditions": [
          {
            "lastTransitionTime": "2026-09-27T12:38:31Z",
            "lastUpdateTime": "2026-09-27T12:38:31Z",
            "message": "Deployment has minimum availability.",
            "reason": "MinimumReplicasAvailable",
            "status": "True",
            "type": "Available"
          },
          {
            "lastTransitionTime": "2026-09-27T12:38:31Z",
            "lastUpdateTime": "2026-09-27T12:38:31Z",
            "message": "ReplicaSet \"auth-65cfd67f89\" has successfully progressed.",
            "reason": "NewReplicaSetAvailable",
            "status": "True",
            "type": "Progressing"
          }
        ],
        "observedGeneration": 1,
        "terminatingReplicas": 0
      }
    }
  ]
}
CUB_SCOUT_EVAL_EOF
cat > evidence/health-measurement-contract/after-explain.json <<'CUB_SCOUT_PROOF_EOF'
{
  "resource": "Deployment/auth",
  "namespace": "team-02",
  "owner": "Native",
  "source": "unknown",
  "deployedVia": "partial trace only",
  "health": "Unavailable",
  "healthMeasurement": {
    "status": "unmeasured",
    "scope": "controller-chain",
    "reason": "No controller-chain health result was observed."
  },
  "risks": "Not assessed",
  "drift": "Unknown",
  "notes": [
    "partial trace: no GitOps owner chain was discovered"
  ],
  "currentChange": {
    "resource": {
      "apiVersion": "apps/v1",
      "kind": "Deployment",
      "namespace": "team-02",
      "name": "auth"
    },
    "progress": {
      "phase": "complete",
      "clockSource": "status.conditions[Progressing].lastUpdateTime",
      "progressAgeSeconds": 278737
    },
    "verdict": "PASS",
    "reason": "workload_converged",
    "message": "Deployment is available. Replicas: 0",
    "evidence": {
      "kstatusStatus": "Current",
      "kstatusMessage": "Deployment is available. Replicas: 0",
      "generation": 1,
      "observedGeneration": 1,
      "observedAt": "2026-09-30T18:04:08Z"
    }
  },
  "nextSteps": [
    {
      "actionType": "read-only",
      "reason": "Health status unknown - trace the chain to find the root cause",
      "nextCommand": "cub-scout trace deployment/auth -n team-02 --explain"
    },
    {
      "actionType": "read-only",
      "reason": "See all Native-managed resources in this scope",
      "nextCommand": "cub-scout map list -n team-02 -q \"owner=Native\""
    },
    {
      "actionType": "read-only",
      "reason": "Get a broad health summary to contextualize this resource",
      "nextCommand": "cub-scout doctor -n team-02"
    }
  ],
  "mutationCause": "manual-edit",
  "mutationManager": "kubectl-client-side-apply"
}
CUB_SCOUT_PROOF_EOF
cat > evidence/health-measurement-contract/after-mcp.json <<'CUB_SCOUT_PROOF_EOF'
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "text": "{\n  \"resource\": \"Deployment/auth\",\n  \"namespace\": \"team-02\",\n  \"owner\": \"Native\",\n  \"source\": \"unknown\",\n  \"deployedVia\": \"partial trace only\",\n  \"health\": \"Unavailable\",\n  \"healthMeasurement\": {\n    \"status\": \"unmeasured\",\n    \"scope\": \"controller-chain\",\n    \"reason\": \"No controller-chain health result was observed.\"\n  },\n  \"risks\": \"Not assessed\",\n  \"drift\": \"Unknown\",\n  \"notes\": [\n    \"partial trace: no GitOps owner chain was discovered\"\n  ],\n  \"currentChange\": {\n    \"resource\": {\n      \"apiVersion\": \"apps/v1\",\n      \"kind\": \"Deployment\",\n      \"namespace\": \"team-02\",\n      \"name\": \"auth\"\n    },\n    \"progress\": {\n      \"phase\": \"complete\",\n      \"clockSource\": \"status.conditions[Progressing].lastUpdateTime\",\n      \"progressAgeSeconds\": 278786\n    },\n    \"verdict\": \"PASS\",\n    \"reason\": \"workload_converged\",\n    \"message\": \"Deployment is available. Replicas: 0\",\n    \"evidence\": {\n      \"kstatusStatus\": \"Current\",\n      \"kstatusMessage\": \"Deployment is available. Replicas: 0\",\n      \"generation\": 1,\n      \"observedGeneration\": 1,\n      \"observedAt\": \"2026-09-30T18:04:57Z\"\n    }\n  },\n  \"nextSteps\": [\n    {\n      \"actionType\": \"read-only\",\n      \"reason\": \"Health status unknown - trace the chain to find the root cause\",\n      \"nextCommand\": \"cub-scout trace deployment/auth -n team-02 --explain\"\n    },\n    {\n      \"actionType\": \"read-only\",\n      \"reason\": \"See all Native-managed resources in this scope\",\n      \"nextCommand\": \"cub-scout map list -n team-02 -q \\\"owner=Native\\\"\"\n    },\n    {\n      \"actionType\": \"read-only\",\n      \"reason\": \"Get a broad health summary to contextualize this resource\",\n      \"nextCommand\": \"cub-scout doctor -n team-02\"\n    }\n  ],\n  \"mutationCause\": \"manual-edit\",\n  \"mutationManager\": \"kubectl-client-side-apply\"\n}",
        "type": "text"
      }
    ],
    "isError": false
  }
}
CUB_SCOUT_PROOF_EOF
cat > evidence/health-measurement-contract/after-object.json <<'CUB_SCOUT_PROOF_EOF'
{
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    "metadata": {
        "annotations": {
            "deployment.kubernetes.io/revision": "1",
            "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"annotations\":{},\"labels\":{\"app\":\"auth\"},\"name\":\"auth\",\"namespace\":\"team-02\"},\"spec\":{\"replicas\":0,\"selector\":{\"matchLabels\":{\"app\":\"auth\"}},\"template\":{\"metadata\":{\"labels\":{\"app\":\"auth\"}},\"spec\":{\"containers\":[{\"image\":\"registry.k8s.io/pause:3.9\",\"name\":\"auth\"}]}}}}\n"
        },
        "creationTimestamp": "2026-09-27T12:38:31Z",
        "generation": 1,
        "labels": {
            "app": "auth"
        },
        "managedFields": [
            {
                "apiVersion": "apps/v1",
                "fieldsType": "FieldsV1",
                "fieldsV1": {
                    "f:metadata": {
                        "f:annotations": {
                            "f:deployment.kubernetes.io/revision": {}
                        }
                    },
                    "f:status": {
                        "f:conditions": {
                            ".": {},
                            "k:{\"type\":\"Available\"}": {
                                ".": {},
                                "f:lastTransitionTime": {},
                                "f:lastUpdateTime": {},
                                "f:message": {},
                                "f:reason": {},
                                "f:status": {},
                                "f:type": {}
                            },
                            "k:{\"type\":\"Progressing\"}": {
                                ".": {},
                                "f:lastTransitionTime": {},
                                "f:lastUpdateTime": {},
                                "f:message": {},
                                "f:reason": {},
                                "f:status": {},
                                "f:type": {}
                            }
                        },
                        "f:observedGeneration": {},
                        "f:terminatingReplicas": {}
                    }
                },
                "manager": "kube-controller-manager",
                "operation": "Update",
                "subresource": "status",
                "time": "2026-09-27T12:38:31Z"
            },
            {
                "apiVersion": "apps/v1",
                "fieldsType": "FieldsV1",
                "fieldsV1": {
                    "f:metadata": {
                        "f:annotations": {
                            ".": {},
                            "f:kubectl.kubernetes.io/last-applied-configuration": {}
                        },
                        "f:labels": {
                            ".": {},
                            "f:app": {}
                        }
                    },
                    "f:spec": {
                        "f:progressDeadlineSeconds": {},
                        "f:replicas": {},
                        "f:revisionHistoryLimit": {},
                        "f:selector": {},
                        "f:strategy": {
                            "f:rollingUpdate": {
                                ".": {},
                                "f:maxSurge": {},
                                "f:maxUnavailable": {}
                            },
                            "f:type": {}
                        },
                        "f:template": {
                            "f:metadata": {
                                "f:labels": {
                                    ".": {},
                                    "f:app": {}
                                }
                            },
                            "f:spec": {
                                "f:containers": {
                                    "k:{\"name\":\"auth\"}": {
                                        ".": {},
                                        "f:image": {},
                                        "f:imagePullPolicy": {},
                                        "f:name": {},
                                        "f:resources": {},
                                        "f:terminationMessagePath": {},
                                        "f:terminationMessagePolicy": {}
                                    }
                                },
                                "f:dnsPolicy": {},
                                "f:restartPolicy": {},
                                "f:schedulerName": {},
                                "f:securityContext": {},
                                "f:terminationGracePeriodSeconds": {}
                            }
                        }
                    }
                },
                "manager": "kubectl-client-side-apply",
                "operation": "Update",
                "time": "2026-09-27T12:38:31Z"
            }
        ],
        "name": "auth",
        "namespace": "team-02",
        "resourceVersion": "530",
        "uid": "dfc78f8e-d712-444e-af6b-54caeb4774bd"
    },
    "spec": {
        "progressDeadlineSeconds": 600,
        "replicas": 0,
        "revisionHistoryLimit": 10,
        "selector": {
            "matchLabels": {
                "app": "auth"
            }
        },
        "strategy": {
            "rollingUpdate": {
                "maxSurge": "25%",
                "maxUnavailable": "25%"
            },
            "type": "RollingUpdate"
        },
        "template": {
            "metadata": {
                "labels": {
                    "app": "auth"
                }
            },
            "spec": {
                "containers": [
                    {
                        "image": "registry.k8s.io/pause:3.9",
                        "imagePullPolicy": "IfNotPresent",
                        "name": "auth",
                        "resources": {},
                        "terminationMessagePath": "/dev/termination-log",
                        "terminationMessagePolicy": "File"
                    }
                ],
                "dnsPolicy": "ClusterFirst",
                "restartPolicy": "Always",
                "schedulerName": "default-scheduler",
                "securityContext": {},
                "terminationGracePeriodSeconds": 30
            }
        }
    },
    "status": {
        "conditions": [
            {
                "lastTransitionTime": "2026-09-27T12:38:31Z",
                "lastUpdateTime": "2026-09-27T12:38:31Z",
                "message": "Deployment has minimum availability.",
                "reason": "MinimumReplicasAvailable",
                "status": "True",
                "type": "Available"
            },
            {
                "lastTransitionTime": "2026-09-27T12:38:31Z",
                "lastUpdateTime": "2026-09-27T12:38:31Z",
                "message": "ReplicaSet \"auth-65cfd67f89\" has successfully progressed.",
                "reason": "NewReplicaSetAvailable",
                "status": "True",
                "type": "Progressing"
            }
        ],
        "observedGeneration": 1,
        "terminatingReplicas": 0
    }
}
CUB_SCOUT_PROOF_EOF
cat > evidence/health-measurement-contract/before-explain.json <<'CUB_SCOUT_PROOF_EOF'
{
  "resource": "Deployment/auth",
  "namespace": "team-02",
  "owner": "Native",
  "source": "unknown",
  "deployedVia": "partial trace only",
  "health": "Unavailable",
  "risks": "Not assessed",
  "drift": "Unknown",
  "notes": [
    "partial trace: no GitOps owner chain was discovered"
  ],
  "currentChange": {
    "resource": {
      "apiVersion": "apps/v1",
      "kind": "Deployment",
      "namespace": "team-02",
      "name": "auth"
    },
    "progress": {
      "phase": "complete",
      "clockSource": "status.conditions[Progressing].lastUpdateTime",
      "progressAgeSeconds": 278735
    },
    "verdict": "PASS",
    "reason": "workload_converged",
    "message": "Deployment is available. Replicas: 0",
    "evidence": {
      "kstatusStatus": "Current",
      "kstatusMessage": "Deployment is available. Replicas: 0",
      "generation": 1,
      "observedGeneration": 1,
      "observedAt": "2026-09-30T18:04:06Z"
    }
  },
  "nextSteps": [
    {
      "actionType": "read-only",
      "reason": "Health status unknown - trace the chain to find the root cause",
      "nextCommand": "cub-scout trace deployment/auth -n team-02 --explain"
    },
    {
      "actionType": "read-only",
      "reason": "See all Native-managed resources in this scope",
      "nextCommand": "cub-scout map list -n team-02 -q \"owner=Native\""
    },
    {
      "actionType": "read-only",
      "reason": "Get a broad health summary to contextualize this resource",
      "nextCommand": "cub-scout doctor -n team-02"
    }
  ],
  "mutationCause": "manual-edit",
  "mutationManager": "kubectl-client-side-apply"
}
CUB_SCOUT_PROOF_EOF
cat > evidence/health-measurement-contract/before-mcp.json <<'CUB_SCOUT_PROOF_EOF'
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "text": "{\n  \"resource\": \"Deployment/auth\",\n  \"namespace\": \"team-02\",\n  \"owner\": \"Native\",\n  \"source\": \"unknown\",\n  \"deployedVia\": \"partial trace only\",\n  \"health\": \"Unavailable\",\n  \"risks\": \"Not assessed\",\n  \"drift\": \"Unknown\",\n  \"notes\": [\n    \"partial trace: no GitOps owner chain was discovered\"\n  ],\n  \"currentChange\": {\n    \"resource\": {\n      \"apiVersion\": \"apps/v1\",\n      \"kind\": \"Deployment\",\n      \"namespace\": \"team-02\",\n      \"name\": \"auth\"\n    },\n    \"progress\": {\n      \"phase\": \"complete\",\n      \"clockSource\": \"status.conditions[Progressing].lastUpdateTime\",\n      \"progressAgeSeconds\": 278786\n    },\n    \"verdict\": \"PASS\",\n    \"reason\": \"workload_converged\",\n    \"message\": \"Deployment is available. Replicas: 0\",\n    \"evidence\": {\n      \"kstatusStatus\": \"Current\",\n      \"kstatusMessage\": \"Deployment is available. Replicas: 0\",\n      \"generation\": 1,\n      \"observedGeneration\": 1,\n      \"observedAt\": \"2026-09-30T18:04:57Z\"\n    }\n  },\n  \"nextSteps\": [\n    {\n      \"actionType\": \"read-only\",\n      \"reason\": \"Health status unknown - trace the chain to find the root cause\",\n      \"nextCommand\": \"cub-scout trace deployment/auth -n team-02 --explain\"\n    },\n    {\n      \"actionType\": \"read-only\",\n      \"reason\": \"See all Native-managed resources in this scope\",\n      \"nextCommand\": \"cub-scout map list -n team-02 -q \\\"owner=Native\\\"\"\n    },\n    {\n      \"actionType\": \"read-only\",\n      \"reason\": \"Get a broad health summary to contextualize this resource\",\n      \"nextCommand\": \"cub-scout doctor -n team-02\"\n    }\n  ],\n  \"mutationCause\": \"manual-edit\",\n  \"mutationManager\": \"kubectl-client-side-apply\"\n}",
        "type": "text"
      }
    ],
    "isError": false
  }
}
CUB_SCOUT_PROOF_EOF
cat > evidence/health-measurement-contract/before-object.json <<'CUB_SCOUT_PROOF_EOF'
{
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    "metadata": {
        "annotations": {
            "deployment.kubernetes.io/revision": "1",
            "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"annotations\":{},\"labels\":{\"app\":\"auth\"},\"name\":\"auth\",\"namespace\":\"team-02\"},\"spec\":{\"replicas\":0,\"selector\":{\"matchLabels\":{\"app\":\"auth\"}},\"template\":{\"metadata\":{\"labels\":{\"app\":\"auth\"}},\"spec\":{\"containers\":[{\"image\":\"registry.k8s.io/pause:3.9\",\"name\":\"auth\"}]}}}}\n"
        },
        "creationTimestamp": "2026-09-27T12:38:31Z",
        "generation": 1,
        "labels": {
            "app": "auth"
        },
        "managedFields": [
            {
                "apiVersion": "apps/v1",
                "fieldsType": "FieldsV1",
                "fieldsV1": {
                    "f:metadata": {
                        "f:annotations": {
                            "f:deployment.kubernetes.io/revision": {}
                        }
                    },
                    "f:status": {
                        "f:conditions": {
                            ".": {},
                            "k:{\"type\":\"Available\"}": {
                                ".": {},
                                "f:lastTransitionTime": {},
                                "f:lastUpdateTime": {},
                                "f:message": {},
                                "f:reason": {},
                                "f:status": {},
                                "f:type": {}
                            },
                            "k:{\"type\":\"Progressing\"}": {
                                ".": {},
                                "f:lastTransitionTime": {},
                                "f:lastUpdateTime": {},
                                "f:message": {},
                                "f:reason": {},
                                "f:status": {},
                                "f:type": {}
                            }
                        },
                        "f:observedGeneration": {},
                        "f:terminatingReplicas": {}
                    }
                },
                "manager": "kube-controller-manager",
                "operation": "Update",
                "subresource": "status",
                "time": "2026-09-27T12:38:31Z"
            },
            {
                "apiVersion": "apps/v1",
                "fieldsType": "FieldsV1",
                "fieldsV1": {
                    "f:metadata": {
                        "f:annotations": {
                            ".": {},
                            "f:kubectl.kubernetes.io/last-applied-configuration": {}
                        },
                        "f:labels": {
                            ".": {},
                            "f:app": {}
                        }
                    },
                    "f:spec": {
                        "f:progressDeadlineSeconds": {},
                        "f:replicas": {},
                        "f:revisionHistoryLimit": {},
                        "f:selector": {},
                        "f:strategy": {
                            "f:rollingUpdate": {
                                ".": {},
                                "f:maxSurge": {},
                                "f:maxUnavailable": {}
                            },
                            "f:type": {}
                        },
                        "f:template": {
                            "f:metadata": {
                                "f:labels": {
                                    ".": {},
                                    "f:app": {}
                                }
                            },
                            "f:spec": {
                                "f:containers": {
                                    "k:{\"name\":\"auth\"}": {
                                        ".": {},
                                        "f:image": {},
                                        "f:imagePullPolicy": {},
                                        "f:name": {},
                                        "f:resources": {},
                                        "f:terminationMessagePath": {},
                                        "f:terminationMessagePolicy": {}
                                    }
                                },
                                "f:dnsPolicy": {},
                                "f:restartPolicy": {},
                                "f:schedulerName": {},
                                "f:securityContext": {},
                                "f:terminationGracePeriodSeconds": {}
                            }
                        }
                    }
                },
                "manager": "kubectl-client-side-apply",
                "operation": "Update",
                "time": "2026-09-27T12:38:31Z"
            }
        ],
        "name": "auth",
        "namespace": "team-02",
        "resourceVersion": "530",
        "uid": "dfc78f8e-d712-444e-af6b-54caeb4774bd"
    },
    "spec": {
        "progressDeadlineSeconds": 600,
        "replicas": 0,
        "revisionHistoryLimit": 10,
        "selector": {
            "matchLabels": {
                "app": "auth"
            }
        },
        "strategy": {
            "rollingUpdate": {
                "maxSurge": "25%",
                "maxUnavailable": "25%"
            },
            "type": "RollingUpdate"
        },
        "template": {
            "metadata": {
                "labels": {
                    "app": "auth"
                }
            },
            "spec": {
                "containers": [
                    {
                        "image": "registry.k8s.io/pause:3.9",
                        "imagePullPolicy": "IfNotPresent",
                        "name": "auth",
                        "resources": {},
                        "terminationMessagePath": "/dev/termination-log",
                        "terminationMessagePolicy": "File"
                    }
                ],
                "dnsPolicy": "ClusterFirst",
                "restartPolicy": "Always",
                "schedulerName": "default-scheduler",
                "securityContext": {},
                "terminationGracePeriodSeconds": 30
            }
        }
    },
    "status": {
        "conditions": [
            {
                "lastTransitionTime": "2026-09-27T12:38:31Z",
                "lastUpdateTime": "2026-09-27T12:38:31Z",
                "message": "Deployment has minimum availability.",
                "reason": "MinimumReplicasAvailable",
                "status": "True",
                "type": "Available"
            },
            {
                "lastTransitionTime": "2026-09-27T12:38:31Z",
                "lastUpdateTime": "2026-09-27T12:38:31Z",
                "message": "ReplicaSet \"auth-65cfd67f89\" has successfully progressed.",
                "reason": "NewReplicaSetAvailable",
                "status": "True",
                "type": "Progressing"
            }
        ],
        "observedGeneration": 1,
        "terminatingReplicas": 0
    }
}
CUB_SCOUT_PROOF_EOF
cat > evidence/health-measurement-contract/proof.json <<'CUB_SCOUT_PROOF_EOF'
{
  "date": "2026-09-30",
  "context": "kind-scout-evals-scale",
  "resource": "apps/v1 Deployment team-02/auth",
  "beforeSource": "v2.12.4 at 11c3e38",
  "afterSource": "8637c81",
  "privateKubeconfig": true,
  "cubHiddenFromPath": true,
  "sameUidAndResourceVersionBeforeAfter": true,
  "mutations": "none; all calls read-only",
  "files": {
    "after-explain.json": "5fba5c61bef92d22a9cef065ce1c99cec79351ceaed33a3c2f6a5a38e940436b",
    "after-mcp.json": "293290ed75d8c3e430e31b73c27f8bfff7028e24d46fd2a853d0a736bbb1a8d0",
    "after-object.json": "5604893ddfc9c6cfab3789932400e02f330769bffe2cb241929d310a623f9f3e",
    "before-explain.json": "f1b799472037e978758a4cdd6033724df753ef1134c84758461ec1b2c9332353",
    "before-mcp.json": "6215940853d3448eaf92e63b766cf25c13822dde04f03fae9045c8cceb9445a7",
    "before-object.json": "5604893ddfc9c6cfab3789932400e02f330769bffe2cb241929d310a623f9f3e"
  }
}
CUB_SCOUT_PROOF_EOF
