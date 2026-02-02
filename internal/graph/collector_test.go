package graph

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCollectOwnershipChain_Empty(t *testing.T) {
	client := fake.NewSimpleClientset()
	collector := NewCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectOwnershipChain(context.Background(), g, "")
	if err != nil {
		t.Fatalf("CollectOwnershipChain failed: %v", err)
	}

	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(g.Edges))
	}
}

func TestCollectOwnershipChain_DeploymentReplicaSetPod(t *testing.T) {
	// Create a Deployment → ReplicaSet → Pod chain
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
			UID:       "deploy-uid-123",
		},
	}

	replicaset := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-abc123",
			Namespace: "default",
			UID:       "rs-uid-456",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "nginx",
					UID:        "deploy-uid-123",
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-abc123-xyz",
			Namespace: "default",
			UID:       "pod-uid-789",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "nginx-abc123",
					UID:        "rs-uid-456",
				},
			},
		},
	}

	client := fake.NewSimpleClientset(deployment, replicaset, pod)
	collector := NewCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectOwnershipChain(context.Background(), g, "default")
	if err != nil {
		t.Fatalf("CollectOwnershipChain failed: %v", err)
	}

	// Verify nodes
	if len(g.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(g.Nodes))
	}

	// Verify edges
	if len(g.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(g.Edges))
	}

	// Sort for deterministic checking
	g.Sort()

	// Check node IDs
	expectedNodeIDs := []string{
		"test-cluster/default/Deployment/nginx",
		"test-cluster/default/Pod/nginx-abc123-xyz",
		"test-cluster/default/ReplicaSet/nginx-abc123",
	}
	for i, node := range g.Nodes {
		if node.ID != expectedNodeIDs[i] {
			t.Errorf("node %d: expected ID %q, got %q", i, expectedNodeIDs[i], node.ID)
		}
	}

	// Check edge: Deployment → ReplicaSet
	foundDeployToRS := false
	for _, edge := range g.Edges {
		if edge.From == "test-cluster/default/Deployment/nginx" &&
			edge.To == "test-cluster/default/ReplicaSet/nginx-abc123" {
			foundDeployToRS = true
			if edge.Type != EdgeTypeOwns {
				t.Errorf("expected edge type %q, got %q", EdgeTypeOwns, edge.Type)
			}
			if len(edge.Evidence) == 0 {
				t.Error("expected non-empty evidence array")
			} else {
				if edge.Evidence[0].Field != "metadata.ownerReferences" {
					t.Errorf("expected field %q, got %q", "metadata.ownerReferences", edge.Evidence[0].Field)
				}
				if edge.Evidence[0].Reason == "" {
					t.Error("expected non-empty evidence reason")
				}
			}
		}
	}
	if !foundDeployToRS {
		t.Error("missing Deployment → ReplicaSet edge")
	}

	// Check edge: ReplicaSet → Pod
	foundRSToPod := false
	for _, edge := range g.Edges {
		if edge.From == "test-cluster/default/ReplicaSet/nginx-abc123" &&
			edge.To == "test-cluster/default/Pod/nginx-abc123-xyz" {
			foundRSToPod = true
			if edge.Type != EdgeTypeOwns {
				t.Errorf("expected edge type %q, got %q", EdgeTypeOwns, edge.Type)
			}
			if len(edge.Evidence) == 0 {
				t.Error("expected non-empty evidence array")
			} else if edge.Evidence[0].Field != "metadata.ownerReferences" {
				t.Errorf("expected field %q, got %q", "metadata.ownerReferences", edge.Evidence[0].Field)
			}
		}
	}
	if !foundRSToPod {
		t.Error("missing ReplicaSet → Pod edge")
	}
}

func TestCollectOwnershipChain_MultipleDeployments(t *testing.T) {
	// Create two separate ownership chains
	deploy1 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "default"},
	}
	deploy2 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "app2", Namespace: "default"},
	}
	rs1 := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app1-rs",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "app1"},
			},
		},
	}
	rs2 := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app2-rs",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "app2"},
			},
		},
	}
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app1-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "app1-rs"},
			},
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app2-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "app2-rs"},
			},
		},
	}

	client := fake.NewSimpleClientset(deploy1, deploy2, rs1, rs2, pod1, pod2)
	collector := NewCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectOwnershipChain(context.Background(), g, "default")
	if err != nil {
		t.Fatalf("CollectOwnershipChain failed: %v", err)
	}

	// 2 deployments + 2 replicasets + 2 pods = 6 nodes
	if len(g.Nodes) != 6 {
		t.Errorf("expected 6 nodes, got %d", len(g.Nodes))
	}

	// 2 deploy→rs edges + 2 rs→pod edges = 4 edges
	if len(g.Edges) != 4 {
		t.Errorf("expected 4 edges, got %d", len(g.Edges))
	}
}

func TestCollectOwnershipChain_NamespaceFilter(t *testing.T) {
	// Resources in different namespaces
	deployDefault := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
	}
	deployProd := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "production"},
	}

	client := fake.NewSimpleClientset(deployDefault, deployProd)
	collector := NewCollector(client, "test-cluster")

	// Collect only from default namespace
	g := NewGraph("test-cluster")
	err := collector.CollectOwnershipChain(context.Background(), g, "default")
	if err != nil {
		t.Fatalf("CollectOwnershipChain failed: %v", err)
	}

	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node (default namespace only), got %d", len(g.Nodes))
	}

	// Collect from all namespaces
	g2 := NewGraph("test-cluster")
	err = collector.CollectOwnershipChain(context.Background(), g2, "")
	if err != nil {
		t.Fatalf("CollectOwnershipChain failed: %v", err)
	}

	if len(g2.Nodes) != 2 {
		t.Errorf("expected 2 nodes (all namespaces), got %d", len(g2.Nodes))
	}
}

func TestCollectOwnershipChain_OrphanPod(t *testing.T) {
	// Pod without ownerReferences (orphan)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "orphan-pod",
			Namespace: "default",
		},
	}

	client := fake.NewSimpleClientset(pod)
	collector := NewCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectOwnershipChain(context.Background(), g, "default")
	if err != nil {
		t.Fatalf("CollectOwnershipChain failed: %v", err)
	}

	// Pod should be a node but no edges
	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges for orphan pod, got %d", len(g.Edges))
	}
}

func TestCollectOwnershipChain_NodeLabels(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
			Labels: map[string]string{
				"app":     "nginx",
				"version": "v1",
			},
		},
	}

	client := fake.NewSimpleClientset(deployment)
	collector := NewCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectOwnershipChain(context.Background(), g, "default")
	if err != nil {
		t.Fatalf("CollectOwnershipChain failed: %v", err)
	}

	if len(g.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(g.Nodes))
	}

	node := g.Nodes[0]
	if node.Labels["app"] != "nginx" {
		t.Errorf("expected label app=nginx, got %v", node.Labels)
	}
	if node.Labels["version"] != "v1" {
		t.Errorf("expected label version=v1, got %v", node.Labels)
	}
}
