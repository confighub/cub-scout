// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// ApplyBackend represents the type of GitOps controller or apply mechanism
type ApplyBackend string

const (
	// BackendFlux indicates resources are applied via Flux CD
	BackendFlux ApplyBackend = "flux"

	// BackendArgoCD indicates resources are applied via Argo CD
	BackendArgoCD ApplyBackend = "argocd"

	// BackendWorker indicates resources are applied via ConfigHub Worker (direct apply)
	BackendWorker ApplyBackend = "worker"

	// BackendNone indicates no GitOps backend detected
	BackendNone ApplyBackend = "none"
)

// TransportMode represents the source transport mechanism
type TransportMode string

const (
	// TransportOCI indicates OCI registry transport
	TransportOCI TransportMode = "oci"

	// TransportGit indicates Git repository transport
	TransportGit TransportMode = "git"

	// TransportHelm indicates Helm repository transport
	TransportHelm TransportMode = "helm"

	// TransportUnknown indicates unknown transport
	TransportUnknown TransportMode = "unknown"
)

// ApplyBackendInfo contains information about the detected apply backend
type ApplyBackendInfo struct {
	// Backend is the type of apply mechanism: flux, argocd, worker, none
	Backend ApplyBackend `json:"backend"`

	// Transport is the source transport: oci, git, helm, unknown
	Transport TransportMode `json:"transport"`

	// Deployers are the detected deployer resources (Kustomization, HelmRelease, Application)
	Deployers []DeployerRef `json:"deployers,omitempty"`

	// Sources are the detected source resources (OCIRepository, GitRepository, etc.)
	Sources []SourceRef `json:"sources,omitempty"`

	// ConfigHubTarget is the ConfigHub target metadata if detected
	ConfigHubTarget *ConfigHubTarget `json:"confighubTarget,omitempty"`
}

// ConfigHubTarget contains ConfigHub-specific metadata
type ConfigHubTarget struct {
	// Space is the ConfigHub space name
	Space string `json:"space,omitempty"`

	// Target is the ConfigHub target name
	Target string `json:"target,omitempty"`

	// Revision is the applied revision
	Revision string `json:"revision,omitempty"`
}

// ApplyBackendDetector detects the apply backend for a cluster or namespace
type ApplyBackendDetector struct {
	client dynamic.Interface
}

// NewApplyBackendDetector creates a new apply backend detector
func NewApplyBackendDetector(client dynamic.Interface) *ApplyBackendDetector {
	return &ApplyBackendDetector{client: client}
}

// DetectBackend detects the apply backend for a cluster or specific namespace
// If namespace is empty, it scans cluster-wide
func (d *ApplyBackendDetector) DetectBackend(ctx context.Context, namespace string) (*ApplyBackendInfo, error) {
	info := &ApplyBackendInfo{
		Backend:   BackendNone,
		Transport: TransportUnknown,
		Deployers: []DeployerRef{},
		Sources:   []SourceRef{},
	}

	// 1. Check for Flux CRDs
	fluxDeployers, fluxSources, err := d.detectFlux(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("detect flux: %w", err)
	}

	if len(fluxDeployers) > 0 {
		info.Backend = BackendFlux
		info.Deployers = append(info.Deployers, fluxDeployers...)
		info.Sources = append(info.Sources, fluxSources...)

		// Determine transport from sources
		info.Transport = d.inferTransport(fluxSources)
	}

	// 2. Check for Argo CD CRDs
	argoDeployers, err := d.detectArgoCD(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("detect argocd: %w", err)
	}

	if len(argoDeployers) > 0 {
		// If we also found Flux, this is a mixed environment
		if info.Backend == BackendNone {
			info.Backend = BackendArgoCD
		}
		info.Deployers = append(info.Deployers, argoDeployers...)

		// Infer transport from Argo sources
		for _, deployer := range argoDeployers {
			if deployer.SourceRef != nil {
				info.Sources = append(info.Sources, *deployer.SourceRef)
			}
		}
		if info.Transport == TransportUnknown {
			info.Transport = d.inferTransport(info.Sources)
		}
	}

	// 3. Check for ConfigHub Worker signals (labels/annotations on resources)
	chTarget := d.detectConfigHubWorker(ctx, namespace)
	if chTarget != nil {
		info.ConfigHubTarget = chTarget
		if info.Backend == BackendNone {
			info.Backend = BackendWorker
		}
	}

	return info, nil
}

// detectFlux looks for Flux Kustomizations and HelmReleases
func (d *ApplyBackendDetector) detectFlux(ctx context.Context, namespace string) ([]DeployerRef, []SourceRef, error) {
	var deployers []DeployerRef
	var sources []SourceRef
	seenSources := make(map[string]bool)

	// Check Kustomizations
	kustomizations, err := d.listResources(ctx, schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}, namespace)
	if err != nil && !apierrors.IsNotFound(err) {
		// CRD not installed is OK, other errors propagate
		if !isCRDNotFound(err) {
			return nil, nil, err
		}
	}

	for _, ks := range kustomizations {
		if ref, ok := ParseDeployerRef(&ks); ok {
			deployers = append(deployers, ref)
			if ref.SourceRef != nil && !seenSources[ref.SourceRef.Key()] {
				sources = append(sources, *ref.SourceRef)
				seenSources[ref.SourceRef.Key()] = true
			}
		}
	}

	// Check HelmReleases
	helmReleases, err := d.listResources(ctx, schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
	}, namespace)
	if err != nil && !apierrors.IsNotFound(err) {
		if !isCRDNotFound(err) {
			return nil, nil, err
		}
	}

	for _, hr := range helmReleases {
		if ref, ok := ParseDeployerRef(&hr); ok {
			deployers = append(deployers, ref)
			if ref.SourceRef != nil && !seenSources[ref.SourceRef.Key()] {
				sources = append(sources, *ref.SourceRef)
				seenSources[ref.SourceRef.Key()] = true
			}
		}
	}

	return deployers, sources, nil
}

// detectArgoCD looks for Argo CD Applications
func (d *ApplyBackendDetector) detectArgoCD(ctx context.Context, namespace string) ([]DeployerRef, error) {
	var deployers []DeployerRef

	// Check Applications
	apps, err := d.listResources(ctx, schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}, namespace)
	if err != nil && !apierrors.IsNotFound(err) {
		if !isCRDNotFound(err) {
			return nil, err
		}
	}

	for _, app := range apps {
		ref := DeployerRef{
			Kind:      "Application",
			Name:      app.GetName(),
			Namespace: app.GetNamespace(),
		}

		// Extract source information from Argo Application
		sourceRef := d.parseArgoSource(&app)
		if sourceRef != nil {
			ref.SourceRef = sourceRef
		}

		deployers = append(deployers, ref)
	}

	return deployers, nil
}

// parseArgoSource extracts source information from an Argo CD Application
func (d *ApplyBackendDetector) parseArgoSource(app *unstructured.Unstructured) *SourceRef {
	// Check spec.source.repoURL
	repoURL, _, _ := unstructured.NestedString(app.Object, "spec", "source", "repoURL")
	if repoURL == "" {
		return nil
	}

	ref := &SourceRef{
		Name:      app.GetName(),
		Namespace: app.GetNamespace(),
	}

	// Determine source type from URL
	if isOCIURL(repoURL) {
		ref.Kind = "OCIRepository"
	} else if isHelmURL(repoURL) {
		ref.Kind = "HelmRepository"
	} else {
		ref.Kind = "GitRepository"
	}

	return ref
}

// detectConfigHubWorker looks for ConfigHub worker signals
func (d *ApplyBackendDetector) detectConfigHubWorker(ctx context.Context, namespace string) *ConfigHubTarget {
	// Look for resources with ConfigHub labels
	// confighub.com/space, confighub.com/target, confighub.com/revision

	// Check for labeled Deployments as a sample
	deployments, err := d.listResources(ctx, schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}, namespace)
	if err != nil {
		return nil
	}

	for _, deploy := range deployments {
		labels := deploy.GetLabels()
		if space, ok := labels["confighub.com/space"]; ok {
			target := &ConfigHubTarget{
				Space: space,
			}
			if t, ok := labels["confighub.com/target"]; ok {
				target.Target = t
			}
			if rev, ok := labels["confighub.com/revision"]; ok {
				target.Revision = rev
			}
			return target
		}
	}

	return nil
}

// listResources lists resources of a given GVR, optionally scoped to a namespace
func (d *ApplyBackendDetector) listResources(ctx context.Context, gvr schema.GroupVersionResource, namespace string) ([]unstructured.Unstructured, error) {
	var list *unstructured.UnstructuredList
	var err error

	if namespace == "" {
		list, err = d.client.Resource(gvr).List(ctx, metav1.ListOptions{})
	} else {
		list, err = d.client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, err
	}

	return list.Items, nil
}

// inferTransport determines the transport mode from source references
func (d *ApplyBackendDetector) inferTransport(sources []SourceRef) TransportMode {
	for _, source := range sources {
		switch source.Kind {
		case "OCIRepository":
			return TransportOCI
		case "GitRepository":
			return TransportGit
		case "HelmRepository":
			return TransportHelm
		}
	}
	return TransportUnknown
}

// isCRDNotFound checks if the error is because the CRD is not installed
func isCRDNotFound(err error) bool {
	if apierrors.IsNotFound(err) {
		return true
	}
	// Also check for "no matches for kind" errors
	if err != nil && (strings.Contains(err.Error(), "no matches for kind") ||
		strings.Contains(err.Error(), "the server could not find the requested resource")) {
		return true
	}
	return false
}

// isOCIURL checks if a URL is an OCI registry URL
func isOCIURL(url string) bool {
	return len(url) > 6 && url[:6] == "oci://"
}

// isHelmURL checks if a URL looks like a Helm repository
func isHelmURL(url string) bool {
	// OCI URLs are not Helm repository URLs (they may contain "/charts" but are OCI transport)
	if isOCIURL(url) {
		return false
	}
	return strings.Contains(url, "/charts") || strings.Contains(url, "chartmuseum") || strings.Contains(url, "helm")
}
