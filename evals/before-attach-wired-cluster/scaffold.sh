#!/usr/bin/env bash
# FIXTURE-OWNED: the cluster export recorded by the attach lane (run 44350497933, argo, after attach)
# and the long help of cub v0.11.4 `cub cluster attach`. Regenerate from fixtures/cluster; do not edit.
set -euo pipefail
mkdir -p cluster
cat > cluster/applications.json <<'CUB_SCOUT_EVAL_EOF'
[
  {
    "apiVersion": "argoproj.io/v1alpha1",
    "kind": "Application",
    "metadata": {
      "annotations": {
        "argocd.argoproj.io/tracking-id": "scout-attach-argo-apps:argoproj.io/Application:argocd/argobot-scout-attach",
        "confighub.com/origin": "{\"spaceId\":\"a7fcd15e-9dd8-46f5-89ef-33860940ea83\",\"spaceSlug\":\"scout-attach-argo-apps\",\"unitId\":\"51c0b547-2e56-4014-bb0d-676f77ecf626\",\"unitSlug\":\"argobot-scout-attach\",\"revisionNum\":2}"
      },
      "creationTimestamp": "2026-11-18T17:00:57Z",
      "finalizers": [
        "resources-finalizer.argocd.argoproj.io"
      ],
      "generation": 15,
      "name": "argobot-scout-attach",
      "namespace": "argocd",
      "resourceVersion": "1737",
      "uid": "45744463-afc8-43ea-8433-0aef5594ef5c"
    },
    "spec": {
      "destination": {
        "server": "https://kubernetes.default.svc"
      },
      "project": "default",
      "source": {
        "path": ".",
        "repoURL": "oci://172.18.0.1:32181/space/argobot-scout-attach",
        "targetRevision": "latest"
      },
      "syncPolicy": {
        "automated": {
          "allowEmpty": true,
          "prune": true,
          "selfHeal": true
        }
      }
    },
    "status": {
      "controllerNamespace": "argocd",
      "health": {
        "lastTransitionTime": "2026-11-18T17:01:10Z",
        "status": "Healthy"
      },
      "history": [
        {
          "deployStartedAt": "2026-11-18T17:00:57Z",
          "deployedAt": "2026-11-18T17:00:58Z",
          "id": 0,
          "initiatedBy": {
            "automated": true
          },
          "revision": "sha256:feebe073e773bf333d1d3f8e2e81c82f3b415ab9d19ffe9de04bec777c60e16c",
          "source": {
            "path": ".",
            "repoURL": "oci://172.18.0.1:32181/space/argobot-scout-attach",
            "targetRevision": "latest"
          }
        }
      ],
      "operationState": {
        "finishedAt": "2026-11-18T17:00:58Z",
        "message": "successfully synced (all tasks run)",
        "operation": {
          "initiatedBy": {
            "automated": true
          },
          "retry": {
            "limit": 5
          },
          "sync": {
            "autoHealAttemptsCount": 1,
            "prune": true,
            "resources": [
              {
                "group": "rbac.authorization.k8s.io",
                "kind": "RoleBinding",
                "name": "argobot-read"
              },
              {
                "group": "apps",
                "kind": "Deployment",
                "name": "argobot"
              },
              {
                "group": "rbac.authorization.k8s.io",
                "kind": "RoleBinding",
                "name": "argobot-argocd"
              }
            ],
            "revision": "sha256:feebe073e773bf333d1d3f8e2e81c82f3b415ab9d19ffe9de04bec777c60e16c",
            "source": {
              "path": ".",
              "repoURL": "oci://172.18.0.1:32181/space/argobot-scout-attach",
              "targetRevision": "latest"
            }
          }
        },
        "phase": "Succeeded",
        "startedAt": "2026-11-18T17:00:58Z",
        "syncResult": {
          "resources": [
            {
              "group": "rbac.authorization.k8s.io",
              "hookPhase": "Running",
              "kind": "RoleBinding",
              "message": "rolebinding.rbac.authorization.k8s.io/argobot-argocd reconciledrolebinding.rbac.authorization.k8s.io/argobot-argocd unchanged",
              "name": "argobot-argocd",
              "namespace": "argocd",
              "status": "Synced",
              "syncPhase": "Sync",
              "version": "v1"
            },
            {
              "group": "rbac.authorization.k8s.io",
              "hookPhase": "Running",
              "kind": "RoleBinding",
              "message": "rolebinding.rbac.authorization.k8s.io/argobot-read reconciledrolebinding.rbac.authorization.k8s.io/argobot-read unchanged",
              "name": "argobot-read",
              "namespace": "kube-system",
              "status": "Synced",
              "syncPhase": "Sync",
              "version": "v1"
            },
            {
              "group": "apps",
              "hookPhase": "Running",
              "images": [
                "ghcr.io/confighub/argobot:v0.2.12"
              ],
              "kind": "Deployment",
              "message": "deployment.apps/argobot unchanged",
              "name": "argobot",
              "namespace": "argobot",
              "status": "Synced",
              "syncPhase": "Sync",
              "version": "v1"
            }
          ],
          "revision": "sha256:feebe073e773bf333d1d3f8e2e81c82f3b415ab9d19ffe9de04bec777c60e16c",
          "source": {
            "path": ".",
            "repoURL": "oci://172.18.0.1:32181/space/argobot-scout-attach",
            "targetRevision": "latest"
          }
        }
      },
      "reconciledAt": "2026-11-18T17:01:09Z",
      "resourceHealthSource": "appTree",
      "resources": [
        {
          "kind": "Namespace",
          "name": "argobot",
          "status": "Synced",
          "version": "v1"
        },
        {
          "kind": "ServiceAccount",
          "name": "argobot",
          "namespace": "argobot",
          "status": "Synced",
          "version": "v1"
        },
        {
          "group": "apps",
          "kind": "Deployment",
          "name": "argobot",
          "namespace": "argobot",
          "status": "Synced",
          "version": "v1"
        },
        {
          "group": "rbac.authorization.k8s.io",
          "kind": "ClusterRole",
          "name": "argobot-read",
          "status": "Synced",
          "version": "v1"
        },
        {
          "group": "rbac.authorization.k8s.io",
          "kind": "ClusterRoleBinding",
          "name": "argobot-read",
          "status": "Synced",
          "version": "v1"
        },
        {
          "group": "rbac.authorization.k8s.io",
          "kind": "Role",
          "name": "argobot-argocd",
          "namespace": "argocd",
          "status": "Synced",
          "version": "v1"
        },
        {
          "group": "rbac.authorization.k8s.io",
          "kind": "Role",
          "name": "argobot-read",
          "namespace": "kube-system",
          "status": "Synced",
          "version": "v1"
        },
        {
          "group": "rbac.authorization.k8s.io",
          "kind": "RoleBinding",
          "name": "argobot-argocd",
          "namespace": "argocd",
          "status": "Synced",
          "version": "v1"
        },
        {
          "group": "rbac.authorization.k8s.io",
          "kind": "RoleBinding",
          "name": "argobot-read",
          "namespace": "kube-system",
          "status": "Synced",
          "version": "v1"
        }
      ],
      "sourceHydrator": {},
      "sourceType": "Directory",
      "summary": {
        "images": [
          "ghcr.io/confighub/argobot:v0.2.12"
        ]
      },
      "sync": {
        "comparedTo": {
          "destination": {
            "server": "https://kubernetes.default.svc"
          },
          "source": {
            "path": ".",
            "repoURL": "oci://172.18.0.1:32181/space/argobot-scout-attach",
            "targetRevision": "latest"
          }
        },
        "revision": "sha256:feebe073e773bf333d1d3f8e2e81c82f3b415ab9d19ffe9de04bec777c60e16c",
        "status": "Synced"
      }
    }
  },
  {
    "apiVersion": "argoproj.io/v1alpha1",
    "kind": "Application",
    "metadata": {
      "annotations": {
        "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"argoproj.io/v1alpha1\",\"kind\":\"Application\",\"metadata\":{\"annotations\":{},\"name\":\"guestbook\",\"namespace\":\"argocd\"},\"spec\":{\"destination\":{\"namespace\":\"guestbook\",\"server\":\"https://kubernetes.default.svc\"},\"project\":\"default\",\"source\":{\"path\":\"guestbook\",\"repoURL\":\"https://github.com/argoproj/argocd-example-apps\",\"targetRevision\":\"8088f4c0d970abb09e250248cc97e35623447cb5\"},\"syncPolicy\":{\"automated\":{\"prune\":true,\"selfHeal\":true},\"syncOptions\":[\"CreateNamespace=true\"]}}}\n"
      },
      "creationTimestamp": "2026-11-18T16:59:19Z",
      "generation": 10,
      "name": "guestbook",
      "namespace": "argocd",
      "resourceVersion": "1397",
      "uid": "36ac30f9-b374-4949-bd17-e36e57816fad"
    },
    "spec": {
      "destination": {
        "namespace": "guestbook",
        "server": "https://kubernetes.default.svc"
      },
      "project": "default",
      "source": {
        "path": "guestbook",
        "repoURL": "https://github.com/argoproj/argocd-example-apps",
        "targetRevision": "8088f4c0d970abb09e250248cc97e35623447cb5"
      },
      "syncPolicy": {
        "automated": {
          "prune": true,
          "selfHeal": true
        },
        "syncOptions": [
          "CreateNamespace=true"
        ]
      }
    },
    "status": {
      "controllerNamespace": "argocd",
      "health": {
        "lastTransitionTime": "2026-11-18T16:59:34Z",
        "status": "Healthy"
      },
      "history": [
        {
          "deployStartedAt": "2026-11-18T16:59:21Z",
          "deployedAt": "2026-11-18T16:59:21Z",
          "id": 0,
          "initiatedBy": {
            "automated": true
          },
          "revision": "8088f4c0d970abb09e250248cc97e35623447cb5",
          "source": {
            "path": "guestbook",
            "repoURL": "https://github.com/argoproj/argocd-example-apps",
            "targetRevision": "8088f4c0d970abb09e250248cc97e35623447cb5"
          }
        }
      ],
      "operationState": {
        "finishedAt": "2026-11-18T16:59:21Z",
        "message": "successfully synced (all tasks run)",
        "operation": {
          "initiatedBy": {
            "automated": true
          },
          "retry": {
            "limit": 5
          },
          "sync": {
            "prune": true,
            "revision": "8088f4c0d970abb09e250248cc97e35623447cb5",
            "source": {
              "path": "guestbook",
              "repoURL": "https://github.com/argoproj/argocd-example-apps",
              "targetRevision": "8088f4c0d970abb09e250248cc97e35623447cb5"
            },
            "syncOptions": [
              "CreateNamespace=true"
            ]
          }
        },
        "phase": "Succeeded",
        "startedAt": "2026-11-18T16:59:21Z",
        "syncResult": {
          "resources": [
            {
              "group": "",
              "hookPhase": "Running",
              "kind": "Namespace",
              "message": "namespace/guestbook created",
              "name": "guestbook",
              "namespace": "",
              "status": "Synced",
              "syncPhase": "PreSync",
              "version": "v1"
            },
            {
              "group": "",
              "hookPhase": "Running",
              "kind": "Service",
              "message": "service/guestbook-ui created",
              "name": "guestbook-ui",
              "namespace": "guestbook",
              "status": "Synced",
              "syncPhase": "Sync",
              "version": "v1"
            },
            {
              "group": "apps",
              "hookPhase": "Running",
              "images": [
                "gcr.io/google-samples/gb-frontend:v5"
              ],
              "kind": "Deployment",
              "message": "deployment.apps/guestbook-ui created",
              "name": "guestbook-ui",
              "namespace": "guestbook",
              "status": "Synced",
              "syncPhase": "Sync",
              "version": "v1"
            }
          ],
          "revision": "8088f4c0d970abb09e250248cc97e35623447cb5",
          "source": {
            "path": "guestbook",
            "repoURL": "https://github.com/argoproj/argocd-example-apps",
            "targetRevision": "8088f4c0d970abb09e250248cc97e35623447cb5"
          }
        }
      },
      "reconciledAt": "2026-11-18T16:59:21Z",
      "resourceHealthSource": "appTree",
      "resources": [
        {
          "kind": "Service",
          "name": "guestbook-ui",
          "namespace": "guestbook",
          "status": "Synced",
          "version": "v1"
        },
        {
          "group": "apps",
          "kind": "Deployment",
          "name": "guestbook-ui",
          "namespace": "guestbook",
          "status": "Synced",
          "version": "v1"
        }
      ],
      "sourceHydrator": {},
      "sourceType": "Directory",
      "summary": {
        "images": [
          "gcr.io/google-samples/gb-frontend:v5"
        ]
      },
      "sync": {
        "comparedTo": {
          "destination": {
            "namespace": "guestbook",
            "server": "https://kubernetes.default.svc"
          },
          "source": {
            "path": "guestbook",
            "repoURL": "https://github.com/argoproj/argocd-example-apps",
            "targetRevision": "8088f4c0d970abb09e250248cc97e35623447cb5"
          }
        },
        "revision": "8088f4c0d970abb09e250248cc97e35623447cb5",
        "status": "Synced"
      }
    }
  },
  {
    "apiVersion": "argoproj.io/v1alpha1",
    "kind": "Application",
    "metadata": {
      "annotations": {
        "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"argoproj.io/v1alpha1\",\"kind\":\"Application\",\"metadata\":{\"annotations\":{},\"finalizers\":[\"resources-finalizer.argocd.argoproj.io\"],\"name\":\"scout-attach-argo-apps\",\"namespace\":\"argocd\"},\"spec\":{\"destination\":{\"namespace\":\"argocd\",\"server\":\"https://kubernetes.default.svc\"},\"project\":\"default\",\"source\":{\"path\":\".\",\"repoURL\":\"oci://172.18.0.1:32181/space/scout-attach-argo-apps\",\"targetRevision\":\"latest\"},\"syncPolicy\":{\"automated\":{\"prune\":true,\"selfHeal\":true},\"syncOptions\":[\"ServerSideApply=true\",\"ServerSideApply.ForceConflicts=true\",\"RespectIgnoreDifferences=true\",\"CreateNamespace=false\"]}}}\n"
      },
      "creationTimestamp": "2026-11-18T17:00:56Z",
      "finalizers": [
        "resources-finalizer.argocd.argoproj.io"
      ],
      "generation": 11,
      "name": "scout-attach-argo-apps",
      "namespace": "argocd",
      "resourceVersion": "1655",
      "uid": "2be1a7d6-a767-428c-8d85-52d34e3d7303"
    },
    "spec": {
      "destination": {
        "namespace": "argocd",
        "server": "https://kubernetes.default.svc"
      },
      "project": "default",
      "source": {
        "path": ".",
        "repoURL": "oci://172.18.0.1:32181/space/scout-attach-argo-apps",
        "targetRevision": "latest"
      },
      "syncPolicy": {
        "automated": {
          "prune": true,
          "selfHeal": true
        },
        "syncOptions": [
          "ServerSideApply=true",
          "ServerSideApply.ForceConflicts=true",
          "RespectIgnoreDifferences=true",
          "CreateNamespace=false"
        ]
      }
    },
    "status": {
      "controllerNamespace": "argocd",
      "health": {
        "lastTransitionTime": "2026-11-18T17:00:57Z",
        "status": "Healthy"
      },
      "history": [
        {
          "deployStartedAt": "2026-11-18T17:00:57Z",
          "deployedAt": "2026-11-18T17:00:57Z",
          "id": 0,
          "initiatedBy": {
            "automated": true
          },
          "revision": "sha256:72725b9232fef7d19da76be4446c9e56dea4e4a745dd16f0ae29c9e8de9bd8d6",
          "source": {
            "path": ".",
            "repoURL": "oci://172.18.0.1:32181/space/scout-attach-argo-apps",
            "targetRevision": "latest"
          }
        }
      ],
      "operationState": {
        "finishedAt": "2026-11-18T17:00:57Z",
        "message": "successfully synced (all tasks run)",
        "operation": {
          "initiatedBy": {
            "automated": true
          },
          "retry": {
            "limit": 5
          },
          "sync": {
            "autoHealAttemptsCount": 1,
            "prune": true,
            "resources": [
              {
                "group": "argoproj.io",
                "kind": "Application",
                "name": "argobot-scout-attach"
              }
            ],
            "revision": "sha256:72725b9232fef7d19da76be4446c9e56dea4e4a745dd16f0ae29c9e8de9bd8d6",
            "source": {
              "path": ".",
              "repoURL": "oci://172.18.0.1:32181/space/scout-attach-argo-apps",
              "targetRevision": "latest"
            },
            "syncOptions": [
              "ServerSideApply=true",
              "ServerSideApply.ForceConflicts=true",
              "RespectIgnoreDifferences=true",
              "CreateNamespace=false"
            ]
          }
        },
        "phase": "Succeeded",
        "startedAt": "2026-11-18T17:00:57Z",
        "syncResult": {
          "resources": [
            {
              "group": "argoproj.io",
              "hookPhase": "Running",
              "kind": "Application",
              "message": "application.argoproj.io/argobot-scout-attach serverside-applied",
              "name": "argobot-scout-attach",
              "namespace": "argocd",
              "status": "Synced",
              "syncPhase": "Sync",
              "version": "v1alpha1"
            }
          ],
          "revision": "sha256:72725b9232fef7d19da76be4446c9e56dea4e4a745dd16f0ae29c9e8de9bd8d6",
          "source": {
            "path": ".",
            "repoURL": "oci://172.18.0.1:32181/space/scout-attach-argo-apps",
            "targetRevision": "latest"
          }
        }
      },
      "reconciledAt": "2026-11-18T17:00:57Z",
      "resourceHealthSource": "appTree",
      "resources": [
        {
          "group": "argoproj.io",
          "kind": "Application",
          "name": "argobot-scout-attach",
          "namespace": "argocd",
          "status": "Synced",
          "version": "v1alpha1"
        },
        {
          "group": "argoproj.io",
          "kind": "Application",
          "name": "scout-attach-argo-apps",
          "namespace": "argocd",
          "status": "Synced",
          "version": "v1alpha1"
        }
      ],
      "sourceHydrator": {},
      "sourceType": "Directory",
      "summary": {
        "isAppOfApps": true
      },
      "sync": {
        "comparedTo": {
          "destination": {
            "namespace": "argocd",
            "server": "https://kubernetes.default.svc"
          },
          "source": {
            "path": ".",
            "repoURL": "oci://172.18.0.1:32181/space/scout-attach-argo-apps",
            "targetRevision": "latest"
          }
        },
        "revision": "sha256:72725b9232fef7d19da76be4446c9e56dea4e4a745dd16f0ae29c9e8de9bd8d6",
        "status": "Synced"
      }
    }
  }
]
CUB_SCOUT_EVAL_EOF
cat > cluster/argocd-cm.json <<'CUB_SCOUT_EVAL_EOF'
{
  "keys": [
    "resource.customizations.ignoreDifferences.argoproj.io_Application",
    "resource.customizations.ignoreResourceUpdates.ConfigMap",
    "resource.customizations.ignoreResourceUpdates.Endpoints",
    "resource.customizations.ignoreResourceUpdates.all",
    "resource.customizations.ignoreResourceUpdates.apps_ReplicaSet",
    "resource.customizations.ignoreResourceUpdates.argoproj.io_Application",
    "resource.customizations.ignoreResourceUpdates.argoproj.io_Rollout",
    "resource.customizations.ignoreResourceUpdates.autoscaling_HorizontalPodAutoscaler",
    "resource.customizations.ignoreResourceUpdates.discovery.k8s.io_EndpointSlice",
    "resource.exclusions"
  ],
  "resource.customizations.ignoreDifferences.argoproj.io_Application": "jqPathExpressions:\n- '.metadata.annotations.\"kubectl.kubernetes.io/last-applied-configuration\"'\n- '.metadata.annotations.\"confighub.com/origin\"'\n- '.metadata.annotations.\"confighub.com/RevisionNum\"'\n- '.metadata.annotations.\"confighub.com/SpaceID\"'\n- '.metadata.annotations.\"confighub.com/UnitSlug\"'\n- '.metadata.annotations.\"confighub.com/ResourceMergeID\"'\n- '.metadata.annotations.\"argocd.argoproj.io/tracking-id\"'\n- '.metadata.finalizers'\n"
}
CUB_SCOUT_EVAL_EOF
cat > cluster/cub-cluster-attach-help.txt <<'CUB_SCOUT_EVAL_EOF'
Attaches a cluster you already have — any cluster your kubeconfig can reach
with cluster-admin — into ConfigHub. It is everything "cub cluster up" does
after creating its kind cluster, and "cub cluster up" is built on it.

Attach never creates or destroys a cluster. The inverse, "cub cluster detach",
removes the wiring and leaves the cluster running.

Its first part — the controller in the cluster, the cluster Space and its OCI
target — is "cub cluster controller-install", and its second — the cluster's
service account, with its key and pull token in the cluster — is "cub cluster
svc-account-bootstrap". Either can be run on its own first, in that order.
Its third — the apps Space and the root — is "cub cluster app-bootstrap",
which finishes the attachment without argobot, and its last, argobot, is "cub
cluster k8sbot-install", which installs it in a cluster attached with
--no-k8sbot.
Attach then picks the cluster up where they left it: it keeps the
credentials, controller, Space, target and service account, takes the
controller and Space from them, and ignores the flags of those parts
(--provider and the credential flags, --controller-version, --no-install and
--no-placeholder-gate).

Where the cluster's credentials come from is --provider:

  kubeconfig  (default) your kubeconfig — $KUBECONFIG or ~/.kube/config, or
              --kubeconfig — at its current context, or the one --kube-context names.
  eks         the AWS CLI: "aws eks update-kubeconfig" for the cluster named by
              --cluster (defaults to the cluster name), honoring --aws-region and
              --aws-profile or the usual AWS environment. The kubeconfig
              authenticates through the CLI, so it stays good as long as your
              AWS credentials are.
  kind        a kind cluster already on this machine, named by --cluster
              (defaults to the cluster name), through "kind get kubeconfig". "cub cluster
              up" is the command that creates one and attaches it.

A self-contained copy of the credentials is written to
<config-dir>/clusters/<name>.kubeconfig, beside a record of which ConfigHub
server and organization the cluster is wired to, so "cub cluster list", "open"
and "down" find it whichever cub context is current. A kubeconfig with a
static token that expires, expires in the copy too.

An EKS cluster created by something other than you — an ACK controller, a
pipeline role — grants its bootstrap admin access to that identity, and
rejects you with a bare 401. Attach diagnoses that: if your AWS identity has
no access entry on the cluster it prints the commands that add one, or runs
them for you with --grant-access (an access entry with the
AmazonEKSClusterAdminPolicy).

What it sets up, in the cluster:

  Argo CD             installed from the upstream manifest if the argocd
                      namespace has no argocd-server; an existing Argo CD is
                      left as found (--no-install fails instead of installing).
                      Either way argocd-cm gets the ignoreDifferences block the
                      self-referencing root Application needs, unless it
                      already carries one.
  repo-creds Secret   a long-lived token of the cluster's service account, so
                      Argo CD can pull from ConfigHub's OCI registry.
  root Application    bootstrapped once with kubectl; from then on Argo pulls
                      everything else from ConfigHub. It never deletes an
                      Application unless --prune is given (below).
  argobot             unless --no-k8sbot: a bot that listens on a Channel and
                      force-syncs the matching Argo Application the moment a
                      deploy happens, instead of waiting for Argo's poll, and
                      reports each Release's live status back.

And in ConfigHub:

  <name>              the cluster Space: an OCI target that represents the
                      cluster, and argobot's Channel. A pure namespace — it
                      holds no config bundle.
  <name>-argo-apps    the apps Space: the root "app of apps" Application Unit
                      and every child Argo Application Unit. Its release target
                      is the OCI target in the cluster Space, so the apps Space
                      is the config bundle Argo pulls.
  argo-<name>         a service account with no organization role, the
                      cluster's identity: the target grants its user View,
                      Edit, ViewChildren and EditChildren; Argo CD pulls with a token
                      of it and argobot runs as it with a key.

The OCI target is gated: a "no-placeholders" Trigger in the cluster Space
blocks publishing a Release that still contains an unfilled
"confighubplaceholder" value, for every Space that releases to this cluster.
Pass --no-placeholder-gate to skip the Trigger; the target is wired either way.

By default nothing cub wires into Argo CD deletes anything: a deployment whose
Application Unit is removed from the apps Space keeps running, so a bad or
empty Release can never take workloads down. --prune opts the cluster into
removal: the root Application prunes an Application whose Unit is gone, and
"cub variant create --target" gives each deployment's Application prune and
Argo's resources finalizer, so removing the Unit removes what the deployment
deployed. --prune is recorded on the OCI target, so it covers deployments
created later; it does not change Applications that already exist.

A cluster is wired to one ConfigHub at a time: attach refuses a cluster whose
Argo CD already holds ConfigHub registry credentials, and a remote cluster
against a ConfigHub on localhost, which Argo inside the cluster could never
reach.

Argo CD is not exposed outside the cluster by attach. Reach its UI with
"kubectl port-forward svc/argocd-server -n argocd 8080:80" against the
kubeconfig attach wrote; the initial admin password is in
argocd-initial-admin-secret. The exception is a kind cluster that "cub cluster
create" made: its node carries the host port window create mapped, and when
attach installs Argo CD or Sveltos it puts the UI on the window's first port,
records the window on the cluster Space, and "cub cluster open" opens the UI
there, as for "cub cluster up".

With --controller sveltos, Sveltos takes Argo CD's place: it is installed
from the upstream manifest, with its dashboard, if the projectsveltos
namespace has no addon-controller, and an existing Sveltos is left as found
(--no-install fails instead of installing). The apps Space is
<name>-sveltos-apps, its root Unit a ClusterProfile that pulls the apps
Space's own Release, and argobot comes from "argobot-sveltos-base". The
dashboard is not exposed outside the cluster; reach it with "kubectl
port-forward svc/dashboard -n projectsveltos 8080:80", and log in with a
token from the platform-admin ServiceAccount. See "cub cluster up --help" for
how the profiles, polling and --prune work.

With --controller flux, Flux takes Argo CD's place: it is installed from the
upstream manifest if the flux-system namespace has no kustomize-controller,
and an existing Flux is left as found (--no-install fails instead of
installing). The apps Space is <name>-flux-apps, its root Unit an
OCIRepository and Kustomization, both named confighub-root, that pull and
apply the apps Space's own Release, and argobot comes from
"argobot-flux-base". Flux has no UI; "kubectl -n flux-system get
ocirepository,kustomization" shows its state. See "cub cluster up --help" for
how the Kustomizations, polling and --prune work.

A Sveltos or Flux cluster is refused, like an Argo CD one, when it already
holds the registry credentials of an attachment: the confighub-gateway Secret
in projectsveltos, or the confighub-oci Secret in flux-system.

Two Secrets hold ConfigHub credentials: the controller's registry credentials
(above) and argobot's key, argobot-secrets in the argobot namespace, whose
CONFIGHUB_AUTH_PRIVATE_KEY argobot authenticates with. Attach applies both. A
cluster that takes credentials only from a secret store, through the External
Secrets Operator, gets them with --secrets-dir <dir> instead: each Secret's
Kubernetes manifest, exactly what attach would apply, is written to
<dir>/<name>/<namespace>_<secret>.yaml and not applied. The values are in plain
text, in the manifest's stringData, so the file is readable only by you;
argobot's also holds the argobot Namespace, which attach applies itself. Load
the values into the store, and have the store keep a Secret of that name in the
cluster with the manifest's type, labels, annotations and keys. Until it is
there the controller cannot pull and argobot cannot start; attach does not wait
for it. The credentials belong to the service account attach creates, so they
cannot be put in the store beforehand, and a failed attach removes the files
along with the service account.

The Secrets Store CSI driver's secretObjects does not fit either flag on its
own: it keeps a Secret only while a pod mounts the driver's volume, and neither
argobot nor the controller mounts one. With it, run a pod that mounts the
SecretProviderClass in each Secret's namespace.

With --existing-secrets, attach uses credentials already in the store instead.
Before attaching, create the cluster's service account and its credentials:

  cub serviceaccount create argo-<name>
  cub serviceaccount token create argo-<name>      # the controller's pull token
  cub serviceaccount key add argo-<name> --generate <path>
                                                   # argobot's key; unless --no-k8sbot

load them into the store, and have it keep these Secrets in the cluster:

  argocd/confighub-oci-creds        (Argo CD) label argocd.argoproj.io/secret-type:
                                    repo-creds; keys type: oci, url: the
                                    registry attach prints, username: token,
                                    password: the token, and with a plain-HTTP
                                    registry insecureOCIForceHttp: "true"
  projectsveltos/confighub-gateway  (Sveltos) type
                                    addons.projectsveltos.io/cluster-profile;
                                    keys username: token, password: the token
  flux-system/confighub-oci         (Flux) type kubernetes.io/dockerconfigjson;
                                    an auth for the registry's host:port with
                                    username token and the token as password
  argobot/argobot-secrets           key CONFIGHUB_AUTH_PRIVATE_KEY: the private
                                    JWK the key add wrote

The service account takes no organization role; attach grants it the target.
Attach skips the check for an earlier attachment's credentials, applies the
argobot namespace (unless --no-k8sbot), waits for each Secret up to
--existing-secrets-timeout (default 5m), since a store can only create one once
its namespace is there and attach may be what installs the controller, and then
checks it: the controller's by its shape and by authenticating with its token,
argobot's by authenticating with its key, each as argo-<name>. Neither the service account nor the Secrets are removed when
attach fails, and "cub cluster detach" leaves the Secrets, and the argobot
namespace, in place.

Examples:
  # Attach whatever your current kube context points at
  cub cluster attach staging

  # Attach a named context from a specific kubeconfig
  cub cluster attach prod-eu --kubeconfig ~/.kube/eu.yaml --kube-context prod-eu

  # Attach a cluster that already runs Argo CD, without argobot
  cub cluster attach shared --no-install --no-k8sbot

  # Attach a cluster through Flux, using the Flux it already runs
  cub cluster attach staging --controller flux --no-install

  # Attach an EKS cluster of the same name, granting yourself cluster-admin first
  cub cluster attach inference-demo --provider eks \
    --aws-region us-west-2 --aws-profile cubdev --grant-access

  # Attach a kind cluster you created by hand
  cub cluster attach lab --provider kind
CUB_SCOUT_EVAL_EOF
cat > cluster/kustomizations.json <<'CUB_SCOUT_EVAL_EOF'
[
  {
    "apiVersion": "kustomize.toolkit.fluxcd.io/v1",
    "kind": "Kustomization",
    "metadata": {
      "annotations": {
        "kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"kustomize.toolkit.fluxcd.io/v1\",\"kind\":\"Kustomization\",\"metadata\":{\"annotations\":{},\"name\":\"podinfo\",\"namespace\":\"podinfo\"},\"spec\":{\"interval\":\"10m\",\"path\":\"./kustomize\",\"prune\":true,\"sourceRef\":{\"kind\":\"GitRepository\",\"name\":\"podinfo\"},\"targetNamespace\":\"podinfo\"}}\n"
      },
      "creationTimestamp": "2026-11-18T16:59:19Z",
      "finalizers": [
        "finalizers.fluxcd.io"
      ],
      "generation": 1,
      "name": "podinfo",
      "namespace": "podinfo",
      "resourceVersion": "1359",
      "uid": "9ac1c2d9-eed1-4280-9a97-473f07a8befe"
    },
    "spec": {
      "force": false,
      "interval": "10m",
      "path": "./kustomize",
      "prune": true,
      "sourceRef": {
        "kind": "GitRepository",
        "name": "podinfo"
      },
      "targetNamespace": "podinfo"
    },
    "status": {
      "conditions": [
        {
          "lastTransitionTime": "2026-11-18T16:59:22Z",
          "message": "Applied revision: sha1:fd3a834ed8d14b80ea334cf8a2fddd4597924da6",
          "observedGeneration": 1,
          "reason": "ReconciliationSucceeded",
          "status": "True",
          "type": "Ready"
        }
      ],
      "history": [
        {
          "digest": "sha256:5ba4c6a6b4c3ab8ca7182f199a986b1094049f716e4f4b9733545128ffba477d",
          "firstReconciled": "2026-11-18T16:59:22Z",
          "lastReconciled": "2026-11-18T16:59:22Z",
          "lastReconciledDuration": "205.839325ms",
          "lastReconciledStatus": "ReconciliationSucceeded",
          "metadata": {
            "revision": "sha1:fd3a834ed8d14b80ea334cf8a2fddd4597924da6"
          },
          "totalReconciliations": 1
        }
      ],
      "inventory": {
        "entries": [
          {
            "id": "podinfo_podinfo__Service",
            "v": "v1"
          },
          {
            "id": "podinfo_podinfo_apps_Deployment",
            "v": "v1"
          },
          {
            "id": "podinfo_podinfo_autoscaling_HorizontalPodAutoscaler",
            "v": "v2"
          }
        ]
      },
      "lastAppliedRevision": "sha1:fd3a834ed8d14b80ea334cf8a2fddd4597924da6",
      "lastAttemptedRevision": "sha1:fd3a834ed8d14b80ea334cf8a2fddd4597924da6",
      "observedGeneration": 1
    }
  }
]
CUB_SCOUT_EVAL_EOF
cat > cluster/namespaces.txt <<'CUB_SCOUT_EVAL_EOF'
namespace/argobot
namespace/argocd
namespace/default
namespace/flux-system
namespace/guestbook
namespace/kube-node-lease
namespace/kube-public
namespace/kube-system
namespace/legacy
namespace/local-path-storage
namespace/podinfo
CUB_SCOUT_EVAL_EOF
cat > cluster/ocirepositories.json <<'CUB_SCOUT_EVAL_EOF'
[]
CUB_SCOUT_EVAL_EOF
cat > cluster/secrets-by-name.txt <<'CUB_SCOUT_EVAL_EOF'
NAMESPACE     NAME                          TYPE                            LABELS
argobot       argobot-secrets               Opaque                          <none>
argocd        argocd-initial-admin-secret   Opaque                          <none>
argocd        argocd-notifications-secret   Opaque                          map[app.kubernetes.io/component:notifications-controller app.kubernetes.io/name:argocd-notifications-controller app.kubernetes.io/part-of:argocd]
argocd        argocd-redis                  Opaque                          <none>
argocd        argocd-secret                 Opaque                          map[app.kubernetes.io/name:argocd-secret app.kubernetes.io/part-of:argocd]
argocd        confighub-oci-creds           Opaque                          map[argocd.argoproj.io/secret-type:repo-creds]
kube-system   bootstrap-token-abcdef        bootstrap.kubernetes.io/token   <none>
CUB_SCOUT_EVAL_EOF
cat > cluster/workloads.txt <<'CUB_SCOUT_EVAL_EOF'
KIND          NAMESPACE            NAME                               IMAGES
Deployment    argobot              argobot                            ghcr.io/confighub/argobot:v0.2.12
Deployment    argocd               argocd-applicationset-controller   quay.io/argoproj/argocd:v3.6.1
Deployment    argocd               argocd-dex-server                  ghcr.io/dexidp/dex:v2.46.0
Deployment    argocd               argocd-notifications-controller    quay.io/argoproj/argocd:v3.6.1
Deployment    argocd               argocd-redis                       public.ecr.aws/docker/library/redis:8.10.1-alpine
Deployment    argocd               argocd-repo-server                 quay.io/argoproj/argocd:v3.6.1
Deployment    argocd               argocd-server                      quay.io/argoproj/argocd:v3.6.1
Deployment    flux-system          helm-controller                    ghcr.io/fluxcd/helm-controller:v1.7.1
Deployment    flux-system          kustomize-controller               ghcr.io/fluxcd/kustomize-controller:v1.10.1
Deployment    flux-system          notification-controller            ghcr.io/fluxcd/notification-controller:v1.10.0
Deployment    flux-system          source-controller                  ghcr.io/fluxcd/source-controller:v1.10.1
Deployment    guestbook            guestbook-ui                       gcr.io/google-samples/gb-frontend:v5
Deployment    kube-system          coredns                            registry.k8s.io/coredns/coredns:v1.13.1
Deployment    legacy               legacy-web                         registry.k8s.io/pause:3.10
Deployment    local-path-storage   local-path-provisioner             docker.io/kindest/local-path-provisioner:v20251212-v0.29.0-alpha-105-g20ccfc88
Deployment    podinfo              podinfo                            ghcr.io/stefanprodan/podinfo:6.17.0
StatefulSet   argocd               argocd-application-controller      quay.io/argoproj/argocd:v3.6.1
DaemonSet     kube-system          kindnet                            docker.io/kindest/kindnetd:v20251212-v0.29.0-alpha-105-g20ccfc88
DaemonSet     kube-system          kube-proxy                         registry.k8s.io/kube-proxy:v1.35.0
CUB_SCOUT_EVAL_EOF
