package graph

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

// TestExport_OwnershipChain_Golden is a golden test that verifies the JSON export
// format for a Deployment → ReplicaSet → Pod ownership chain.
func TestExport_OwnershipChain_Golden(t *testing.T) {
	// Fixed timestamp for deterministic output
	fixedTime, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")

	// Create fixtures
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "default",
			Labels: map[string]string{
				"app": "nginx",
			},
		},
	}

	replicaset := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-abc123",
			Namespace: "default",
			Labels: map[string]string{
				"app":               "nginx",
				"pod-template-hash": "abc123",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "nginx",
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-abc123-xyz",
			Namespace: "default",
			Labels: map[string]string{
				"app":               "nginx",
				"pod-template-hash": "abc123",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "nginx-abc123",
				},
			},
		},
	}

	// Build graph
	client := fake.NewSimpleClientset(deployment, replicaset, pod)
	collector := NewCollector(client, "test-cluster")
	g := NewGraph("test-cluster")
	g.GeneratedAt = fixedTime

	err := collector.CollectOwnershipChain(context.Background(), g, "default")
	if err != nil {
		t.Fatalf("CollectOwnershipChain failed: %v", err)
	}

	// Export
	output, err := g.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Golden file path
	goldenPath := filepath.Join("testdata", "ownership-chain.golden.json")

	// Update golden file if -update flag is set
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatalf("failed to create testdata dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, output, 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Log("Updated golden file")
		return
	}

	// Load and compare
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v\nRun with UPDATE_GOLDEN=1 to create it", goldenPath, err)
	}

	if string(output) != string(golden) {
		t.Errorf("output mismatch\ngot:\n%s\n\nwant:\n%s", output, golden)
	}
}

func TestExport_Deterministic(t *testing.T) {
	// Verify that Export produces identical output for identical input
	fixedTime, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")

	makeGraph := func() *Graph {
		g := NewGraph("test-cluster")
		g.GeneratedAt = fixedTime
		g.AddNode(Node{ID: "b", Kind: "Pod", Name: "b"})
		g.AddNode(Node{ID: "a", Kind: "Pod", Name: "a"})
		g.AddEdge(Edge{From: "b", To: "c", Type: EdgeTypeOwns})
		g.AddEdge(Edge{From: "a", To: "b", Type: EdgeTypeOwns})
		return g
	}

	g1 := makeGraph()
	g2 := makeGraph()

	out1, _ := g1.Export()
	out2, _ := g2.Export()

	if string(out1) != string(out2) {
		t.Error("Export is not deterministic")
	}
}

func TestExport_ValidJSON(t *testing.T) {
	g := NewGraph("test-cluster")
	g.AddNode(Node{
		ID:         "test-cluster/default/Pod/test",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "Pod",
		Name:       "test",
		APIVersion: "v1",
	})

	output, err := g.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed Graph
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Errorf("Export output is not valid JSON: %v", err)
	}

	// Verify schema version
	if parsed.SchemaVersion != SchemaVersion {
		t.Errorf("expected schema version %q, got %q", SchemaVersion, parsed.SchemaVersion)
	}
}

// TestExport_GitOpsCRDs_Golden is a golden test that verifies the JSON export
// format for GitOps CRDs (Argo CD Application, Flux Kustomization).
func TestExport_GitOpsCRDs_Golden(t *testing.T) {
	fixedTime, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")

	// Create GitOps fixtures
	argoApp := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "my-app",
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"app.kubernetes.io/name": "my-app",
				},
			},
		},
	}

	fluxKs := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "infra",
				"namespace": "flux-system",
			},
		},
	}

	// Create fake dynamic client with all GitOps GVRs
	scheme := runtime.NewScheme()
	gvrs := map[schema.GroupVersionResource]string{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}:             "ApplicationList",
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applicationsets"}:          "ApplicationSetList",
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}: "KustomizationList",
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}:        "HelmReleaseList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}:   "GitRepositoryList",
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrs, argoApp, fluxKs)

	// Build graph
	collector := NewGitOpsCollector(client, "test-cluster")
	g := NewGraph("test-cluster")
	g.GeneratedAt = fixedTime

	err := collector.CollectGitOpsCRDs(context.Background(), g, "")
	if err != nil {
		t.Fatalf("CollectGitOpsCRDs failed: %v", err)
	}

	// Export
	output, err := g.Export()
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Golden file path
	goldenPath := filepath.Join("testdata", "gitops-crds.golden.json")

	// Update golden file if -update flag is set
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatalf("failed to create testdata dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, output, 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Log("Updated golden file")
		return
	}

	// Load and compare
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v\nRun with UPDATE_GOLDEN=1 to create it", goldenPath, err)
	}

	if string(output) != string(golden) {
		t.Errorf("output mismatch\ngot:\n%s\n\nwant:\n%s", output, golden)
	}
}
