package graph

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// GitOpsCRD defines a GitOps CRD to collect.
type GitOpsCRD struct {
	GVR        schema.GroupVersionResource
	Kind       string
	APIVersion string
}

// ArgoCDCRDs lists the Argo CD CRDs to collect.
var ArgoCDCRDs = []GitOpsCRD{
	{
		GVR:        schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
		Kind:       "Application",
		APIVersion: "argoproj.io/v1alpha1",
	},
	{
		GVR:        schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applicationsets"},
		Kind:       "ApplicationSet",
		APIVersion: "argoproj.io/v1alpha1",
	},
}

// FluxCRDs lists the Flux CRDs to collect.
var FluxCRDs = []GitOpsCRD{
	{
		GVR:        schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
		Kind:       "Kustomization",
		APIVersion: "kustomize.toolkit.fluxcd.io/v1",
	},
	{
		GVR:        schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
		Kind:       "HelmRelease",
		APIVersion: "helm.toolkit.fluxcd.io/v2",
	},
	{
		GVR:        schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
		Kind:       "GitRepository",
		APIVersion: "source.toolkit.fluxcd.io/v1",
	},
}

// GitOpsCollector collects GitOps CRDs into the graph.
type GitOpsCollector struct {
	client  dynamic.Interface
	cluster string
}

// NewGitOpsCollector creates a new GitOps collector.
func NewGitOpsCollector(client dynamic.Interface, cluster string) *GitOpsCollector {
	return &GitOpsCollector{
		client:  client,
		cluster: cluster,
	}
}

// CollectGitOpsCRDs collects Argo CD and Flux CRDs as nodes.
// This is best-effort: missing CRDs are silently skipped.
func (c *GitOpsCollector) CollectGitOpsCRDs(ctx context.Context, g *Graph, namespace string) error {
	// Collect Argo CD CRDs
	for _, crd := range ArgoCDCRDs {
		if err := c.collectCRD(ctx, g, crd, namespace); err != nil {
			// Best-effort: skip on any error (missing CRD, RBAC forbidden, etc.)
			continue
		}
	}

	// Collect Flux CRDs
	for _, crd := range FluxCRDs {
		if err := c.collectCRD(ctx, g, crd, namespace); err != nil {
			// Best-effort: skip on any error (missing CRD, RBAC forbidden, etc.)
			continue
		}
	}

	return nil
}

// collectCRD collects a single CRD type into the graph.
func (c *GitOpsCollector) collectCRD(ctx context.Context, g *Graph, crd GitOpsCRD, namespace string) error {
	var resourceInterface dynamic.ResourceInterface
	if namespace == "" {
		resourceInterface = c.client.Resource(crd.GVR)
	} else {
		resourceInterface = c.client.Resource(crd.GVR).Namespace(namespace)
	}

	list, err := resourceInterface.List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, item := range list.Items {
		node := c.unstructuredToNode(item, crd)
		g.AddNode(node)
	}

	return nil
}

// unstructuredToNode converts an unstructured resource to a graph node.
func (c *GitOpsCollector) unstructuredToNode(u unstructured.Unstructured, crd GitOpsCRD) Node {
	labels := u.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	// Only include labels if non-empty
	var nodeLabels map[string]string
	if len(labels) > 0 {
		nodeLabels = labels
	}

	return Node{
		ID:         NodeID(c.cluster, u.GetNamespace(), crd.Kind, u.GetName()),
		Cluster:    c.cluster,
		Namespace:  u.GetNamespace(),
		Kind:       crd.Kind,
		Name:       u.GetName(),
		APIVersion: crd.APIVersion,
		Labels:     nodeLabels,
	}
}
