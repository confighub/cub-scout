package graph

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Collector builds a graph from Kubernetes resources.
type Collector struct {
	client  kubernetes.Interface
	cluster string
}

// NewCollector creates a new graph collector.
func NewCollector(client kubernetes.Interface, cluster string) *Collector {
	return &Collector{
		client:  client,
		cluster: cluster,
	}
}

// CollectOwnershipChain collects Deployment → ReplicaSet → Pod ownership chains.
// This is the initial v0.6 ingestion scope.
func (c *Collector) CollectOwnershipChain(ctx context.Context, g *Graph, namespace string) error {
	// Collect Deployments
	deployments, err := c.client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list deployments: %w", err)
	}

	for _, d := range deployments.Items {
		nodeID := NodeID(c.cluster, d.Namespace, "Deployment", d.Name)
		g.AddNode(Node{
			ID:         nodeID,
			Cluster:    c.cluster,
			Namespace:  d.Namespace,
			Kind:       "Deployment",
			Name:       d.Name,
			APIVersion: "apps/v1",
			Labels:     d.Labels,
		})
	}

	// Collect ReplicaSets
	replicasets, err := c.client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list replicasets: %w", err)
	}

	for _, rs := range replicasets.Items {
		nodeID := NodeID(c.cluster, rs.Namespace, "ReplicaSet", rs.Name)
		g.AddNode(Node{
			ID:         nodeID,
			Cluster:    c.cluster,
			Namespace:  rs.Namespace,
			Kind:       "ReplicaSet",
			Name:       rs.Name,
			APIVersion: "apps/v1",
			Labels:     rs.Labels,
		})

		// Add ownership edges from ownerReferences
		for _, owner := range rs.OwnerReferences {
			if owner.Kind == "Deployment" {
				ownerID := NodeID(c.cluster, rs.Namespace, owner.Kind, owner.Name)
				g.AddEdge(Edge{
					From: ownerID,
					To:   nodeID,
					Type: EdgeTypeOwns,
					Evidence: Evidence{
						Field:  fmt.Sprintf("metadata.ownerReferences[%s]", owner.Name),
						Reason: fmt.Sprintf("ReplicaSet %s has ownerReference to Deployment %s", rs.Name, owner.Name),
					},
				})
			}
		}
	}

	// Collect Pods
	pods, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	for _, pod := range pods.Items {
		nodeID := NodeID(c.cluster, pod.Namespace, "Pod", pod.Name)
		g.AddNode(Node{
			ID:         nodeID,
			Cluster:    c.cluster,
			Namespace:  pod.Namespace,
			Kind:       "Pod",
			Name:       pod.Name,
			APIVersion: "v1",
			Labels:     pod.Labels,
		})

		// Add ownership edges from ownerReferences
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "ReplicaSet" {
				ownerID := NodeID(c.cluster, pod.Namespace, owner.Kind, owner.Name)
				g.AddEdge(Edge{
					From: ownerID,
					To:   nodeID,
					Type: EdgeTypeOwns,
					Evidence: Evidence{
						Field:  fmt.Sprintf("metadata.ownerReferences[%s]", owner.Name),
						Reason: fmt.Sprintf("Pod %s has ownerReference to ReplicaSet %s", pod.Name, owner.Name),
					},
				})
			}
		}
	}

	return nil
}
