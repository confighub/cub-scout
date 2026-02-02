package graph

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// allGitOpsGVRs returns all GitOps GVRs for fake client registration.
func allGitOpsGVRs() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}:             "ApplicationList",
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applicationsets"}:          "ApplicationSetList",
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}: "KustomizationList",
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}:        "HelmReleaseList",
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}:   "GitRepositoryList",
	}
}

func TestCollectGitOpsCRDs_Empty(t *testing.T) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allGitOpsGVRs())

	collector := NewGitOpsCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectGitOpsCRDs(context.Background(), g, "")
	if err != nil {
		t.Fatalf("CollectGitOpsCRDs failed: %v", err)
	}

	// No objects, so no nodes
	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(g.Nodes))
	}
}

func TestCollectGitOpsCRDs_ArgoApplication(t *testing.T) {
	app := &unstructured.Unstructured{
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
			"spec": map[string]interface{}{
				"project": "default",
			},
		},
	}

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allGitOpsGVRs(), app)

	collector := NewGitOpsCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectGitOpsCRDs(context.Background(), g, "")
	if err != nil {
		t.Fatalf("CollectGitOpsCRDs failed: %v", err)
	}

	// Should have 1 node for the Application
	if len(g.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(g.Nodes))
	}

	node := g.Nodes[0]
	if node.Kind != "Application" {
		t.Errorf("expected kind Application, got %s", node.Kind)
	}
	if node.Name != "my-app" {
		t.Errorf("expected name my-app, got %s", node.Name)
	}
	if node.Namespace != "argocd" {
		t.Errorf("expected namespace argocd, got %s", node.Namespace)
	}
	if node.APIVersion != "argoproj.io/v1alpha1" {
		t.Errorf("expected api_version argoproj.io/v1alpha1, got %s", node.APIVersion)
	}
	if node.ID != "test-cluster/argocd/Application/my-app" {
		t.Errorf("expected ID test-cluster/argocd/Application/my-app, got %s", node.ID)
	}
	if node.Labels["app.kubernetes.io/name"] != "my-app" {
		t.Errorf("expected label app.kubernetes.io/name=my-app, got %v", node.Labels)
	}
}

func TestCollectGitOpsCRDs_FluxKustomization(t *testing.T) {
	ks := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "my-ks",
				"namespace": "flux-system",
			},
			"spec": map[string]interface{}{
				"interval": "10m",
			},
		},
	}

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allGitOpsGVRs(), ks)

	collector := NewGitOpsCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectGitOpsCRDs(context.Background(), g, "")
	if err != nil {
		t.Fatalf("CollectGitOpsCRDs failed: %v", err)
	}

	if len(g.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(g.Nodes))
	}

	node := g.Nodes[0]
	if node.Kind != "Kustomization" {
		t.Errorf("expected kind Kustomization, got %s", node.Kind)
	}
	if node.APIVersion != "kustomize.toolkit.fluxcd.io/v1" {
		t.Errorf("expected api_version kustomize.toolkit.fluxcd.io/v1, got %s", node.APIVersion)
	}
}

func TestCollectGitOpsCRDs_MultipleTypes(t *testing.T) {
	app := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "app1",
				"namespace": "argocd",
			},
		},
	}

	appSet := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      "appset1",
				"namespace": "argocd",
			},
		},
	}

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allGitOpsGVRs(), app, appSet)

	collector := NewGitOpsCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectGitOpsCRDs(context.Background(), g, "")
	if err != nil {
		t.Fatalf("CollectGitOpsCRDs failed: %v", err)
	}

	if len(g.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(g.Nodes))
	}

	// Verify we have both kinds
	kinds := make(map[string]bool)
	for _, node := range g.Nodes {
		kinds[node.Kind] = true
	}
	if !kinds["Application"] {
		t.Error("missing Application node")
	}
	if !kinds["ApplicationSet"] {
		t.Error("missing ApplicationSet node")
	}
}

func TestCollectGitOpsCRDs_NamespaceFilter(t *testing.T) {
	app1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "app1",
				"namespace": "argocd",
			},
		},
	}
	app2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "app2",
				"namespace": "other-ns",
			},
		},
	}

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allGitOpsGVRs(), app1, app2)

	collector := NewGitOpsCollector(client, "test-cluster")

	// Collect only from argocd namespace
	g := NewGraph("test-cluster")
	err := collector.CollectGitOpsCRDs(context.Background(), g, "argocd")
	if err != nil {
		t.Fatalf("CollectGitOpsCRDs failed: %v", err)
	}

	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 node (argocd namespace only), got %d", len(g.Nodes))
	}
	if g.Nodes[0].Name != "app1" {
		t.Errorf("expected app1, got %s", g.Nodes[0].Name)
	}
}

func TestCollectGitOpsCRDs_NoEdges(t *testing.T) {
	// Per design: #61 adds nodes only, no edges
	app := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "my-app",
				"namespace": "argocd",
			},
		},
	}

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allGitOpsGVRs(), app)

	collector := NewGitOpsCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectGitOpsCRDs(context.Background(), g, "")
	if err != nil {
		t.Fatalf("CollectGitOpsCRDs failed: %v", err)
	}

	// Should have no edges (nodes only for Phase 2)
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(g.Edges))
	}
}

func TestCollectGitOpsCRDs_AllFluxTypes(t *testing.T) {
	ks := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "my-ks",
				"namespace": "flux-system",
			},
		},
	}

	hr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "helm.toolkit.fluxcd.io/v2",
			"kind":       "HelmRelease",
			"metadata": map[string]interface{}{
				"name":      "my-hr",
				"namespace": "flux-system",
			},
		},
	}

	gr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "source.toolkit.fluxcd.io/v1",
			"kind":       "GitRepository",
			"metadata": map[string]interface{}{
				"name":      "my-repo",
				"namespace": "flux-system",
			},
		},
	}

	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, allGitOpsGVRs(), ks, hr, gr)

	collector := NewGitOpsCollector(client, "test-cluster")
	g := NewGraph("test-cluster")

	err := collector.CollectGitOpsCRDs(context.Background(), g, "")
	if err != nil {
		t.Fatalf("CollectGitOpsCRDs failed: %v", err)
	}

	if len(g.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(g.Nodes))
	}

	kinds := make(map[string]bool)
	for _, node := range g.Nodes {
		kinds[node.Kind] = true
	}
	if !kinds["Kustomization"] {
		t.Error("missing Kustomization node")
	}
	if !kinds["HelmRelease"] {
		t.Error("missing HelmRelease node")
	}
	if !kinds["GitRepository"] {
		t.Error("missing GitRepository node")
	}
}
