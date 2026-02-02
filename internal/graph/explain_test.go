package graph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildTestGraph creates a graph with Deployment → ReplicaSet → Pod chain
// plus an Argo Application for testing.
func buildTestGraph() *Graph {
	g := NewGraph("test-cluster")

	// Deployment
	g.AddNode(Node{
		ID:         "test-cluster/default/Deployment/nginx",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "Deployment",
		Name:       "nginx",
		APIVersion: "apps/v1",
		Labels:     map[string]string{"app": "nginx"},
	})

	// ReplicaSet
	g.AddNode(Node{
		ID:         "test-cluster/default/ReplicaSet/nginx-abc123",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "ReplicaSet",
		Name:       "nginx-abc123",
		APIVersion: "apps/v1",
		Labels:     map[string]string{"app": "nginx", "pod-template-hash": "abc123"},
	})

	// Pod
	g.AddNode(Node{
		ID:         "test-cluster/default/Pod/nginx-abc123-xyz789",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "Pod",
		Name:       "nginx-abc123-xyz789",
		APIVersion: "v1",
		Labels:     map[string]string{"app": "nginx"},
	})

	// Argo Application (no edges)
	g.AddNode(Node{
		ID:         "test-cluster/argocd/Application/my-app",
		Cluster:    "test-cluster",
		Namespace:  "argocd",
		Kind:       "Application",
		Name:       "my-app",
		APIVersion: "argoproj.io/v1alpha1",
		Labels:     map[string]string{"app.kubernetes.io/name": "my-app"},
	})

	// Deployment → ReplicaSet edge
	g.AddEdge(Edge{
		From: "test-cluster/default/Deployment/nginx",
		To:   "test-cluster/default/ReplicaSet/nginx-abc123",
		Type: EdgeTypeOwns,
		Evidence: []Evidence{{
			Field:  "metadata.ownerReferences",
			Reason: "ReplicaSet nginx-abc123 has ownerReference to Deployment nginx",
		}},
	})

	// ReplicaSet → Pod edge
	g.AddEdge(Edge{
		From: "test-cluster/default/ReplicaSet/nginx-abc123",
		To:   "test-cluster/default/Pod/nginx-abc123-xyz789",
		Type: EdgeTypeOwns,
		Evidence: []Evidence{{
			Field:  "metadata.ownerReferences",
			Reason: "Pod nginx-abc123-xyz789 has ownerReference to ReplicaSet nginx-abc123",
		}},
	})

	return g
}

func TestExplain_Deployment_Golden(t *testing.T) {
	g := buildTestGraph()

	output, exitCode := Explain(g, "test-cluster", "default", "Deployment", "nginx")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	goldenPath := filepath.Join("..", "..", "test", "golden", "graph-explain", "testdata", "deployment.golden.txt")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if output != string(golden) {
		t.Errorf("output mismatch\ngot:\n%s\n\nwant:\n%s", output, string(golden))
	}
}

func TestExplain_Pod_Golden(t *testing.T) {
	g := buildTestGraph()

	output, exitCode := Explain(g, "test-cluster", "default", "Pod", "nginx-abc123-xyz789")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	goldenPath := filepath.Join("..", "..", "test", "golden", "graph-explain", "testdata", "pod.golden.txt")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if output != string(golden) {
		t.Errorf("output mismatch\ngot:\n%s\n\nwant:\n%s", output, string(golden))
	}
}

func TestExplain_CRDNoEdges_Golden(t *testing.T) {
	g := buildTestGraph()

	output, exitCode := Explain(g, "test-cluster", "argocd", "Application", "my-app")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	goldenPath := filepath.Join("..", "..", "test", "golden", "graph-explain", "testdata", "crd-no-edges.golden.txt")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if output != string(golden) {
		t.Errorf("output mismatch\ngot:\n%s\n\nwant:\n%s", output, string(golden))
	}
}

func TestExplain_NotFound_Golden(t *testing.T) {
	g := buildTestGraph()

	output, exitCode := Explain(g, "test-cluster", "default", "Deployment", "nonexistent")

	if exitCode != 3 {
		t.Fatalf("expected exit code 3, got %d", exitCode)
	}

	goldenPath := filepath.Join("..", "..", "test", "golden", "graph-explain", "testdata", "not-found.golden.txt")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if output != string(golden) {
		t.Errorf("output mismatch\ngot:\n%s\n\nwant:\n%s", output, string(golden))
	}
}

func TestExplain_Deterministic(t *testing.T) {
	g := buildTestGraph()

	output1, _ := Explain(g, "test-cluster", "default", "Deployment", "nginx")
	output2, _ := Explain(g, "test-cluster", "default", "Deployment", "nginx")

	if output1 != output2 {
		t.Error("Explain is not deterministic")
	}
}

func TestExplain_NoLabels(t *testing.T) {
	g := NewGraph("test-cluster")
	g.AddNode(Node{
		ID:         "test-cluster/default/Pod/orphan",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "Pod",
		Name:       "orphan",
		APIVersion: "v1",
		Labels:     nil, // No labels
	})

	output, exitCode := Explain(g, "test-cluster", "default", "Pod", "orphan")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// Should contain "(none)" for labels
	if !strings.Contains(output, "labels:\n    (none)") {
		t.Errorf("expected '(none)' for empty labels, got:\n%s", output)
	}
}

func TestExplain_MultipleOutgoingEdges(t *testing.T) {
	g := NewGraph("test-cluster")

	// Deployment that owns two ReplicaSets
	g.AddNode(Node{
		ID:         "test-cluster/default/Deployment/app",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "Deployment",
		Name:       "app",
		APIVersion: "apps/v1",
		Labels:     map[string]string{"app": "test"},
	})
	g.AddNode(Node{
		ID:         "test-cluster/default/ReplicaSet/app-v1",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "ReplicaSet",
		Name:       "app-v1",
		APIVersion: "apps/v1",
	})
	g.AddNode(Node{
		ID:         "test-cluster/default/ReplicaSet/app-v2",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "ReplicaSet",
		Name:       "app-v2",
		APIVersion: "apps/v1",
	})

	// Add edges in reverse order to test sorting
	g.AddEdge(Edge{
		From:     "test-cluster/default/Deployment/app",
		To:       "test-cluster/default/ReplicaSet/app-v2",
		Type:     EdgeTypeOwns,
		Evidence: []Evidence{{Field: "metadata.ownerReferences", Reason: "RS v2"}},
	})
	g.AddEdge(Edge{
		From:     "test-cluster/default/Deployment/app",
		To:       "test-cluster/default/ReplicaSet/app-v1",
		Type:     EdgeTypeOwns,
		Evidence: []Evidence{{Field: "metadata.ownerReferences", Reason: "RS v1"}},
	})

	output, exitCode := Explain(g, "test-cluster", "default", "Deployment", "app")

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// Should show outgoing (2)
	if !strings.Contains(output, "outgoing (2):") {
		t.Errorf("expected 'outgoing (2):', got:\n%s", output)
	}

	// v1 should come before v2 (sorted by target)
	v1Idx := strings.Index(output, "app-v1")
	v2Idx := strings.Index(output, "app-v2")
	if v1Idx > v2Idx {
		t.Errorf("expected app-v1 before app-v2 (sorted), got:\n%s", output)
	}
}
