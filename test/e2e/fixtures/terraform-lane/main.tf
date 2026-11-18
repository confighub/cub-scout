# Two Deployments at zero replicas, one through each way Terraform's
# Kubernetes provider writes objects: a typed resource and kubernetes_manifest.
terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "= 3.5.0"
    }
  }
}

variable "kubeconfig" {
  type = string
}

provider "kubernetes" {
  config_path = var.kubeconfig
}

resource "kubernetes_namespace_v1" "lane" {
  metadata {
    name = "tf-lane"
  }
}

resource "kubernetes_deployment_v1" "typed" {
  metadata {
    name      = "tf-typed"
    namespace = kubernetes_namespace_v1.lane.metadata[0].name
  }
  spec {
    replicas = 0
    selector {
      match_labels = { app = "tf-typed" }
    }
    template {
      metadata {
        labels = { app = "tf-typed" }
      }
      spec {
        container {
          name  = "pause"
          image = "registry.k8s.io/pause:3.10"
        }
      }
    }
  }
  wait_for_rollout = false
}

resource "kubernetes_manifest" "manifest" {
  depends_on = [kubernetes_namespace_v1.lane]
  manifest = {
    apiVersion = "apps/v1"
    kind       = "Deployment"
    metadata   = { name = "tf-manifest", namespace = "tf-lane" }
    spec = {
      replicas = 0
      selector = { matchLabels = { app = "tf-manifest" } }
      template = {
        metadata = { labels = { app = "tf-manifest" } }
        spec     = { containers = [{ name = "pause", image = "registry.k8s.io/pause:3.10" }] }
      }
    }
  }
}
