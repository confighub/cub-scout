// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// HelmTracer implements Tracer for standalone Helm releases
// (not managed by Flux HelmRelease)
type HelmTracer struct {
	client kubernetes.Interface
}

// NewHelmTracer creates a new Helm tracer
func NewHelmTracer(client kubernetes.Interface) *HelmTracer {
	return &HelmTracer{
		client: client,
	}
}

// ToolName returns "helm"
func (h *HelmTracer) ToolName() string {
	return "helm"
}

// Available checks if we can trace Helm releases (always true if we have a k8s client)
func (h *HelmTracer) Available() bool {
	return h.client != nil
}

// Trace finds the Helm release that manages a resource and builds the ownership chain
func (h *HelmTracer) Trace(ctx context.Context, kind, name, namespace string) (*TraceResult, error) {
	// Find the Helm release in the namespace
	releases, err := h.listReleases(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("list helm releases: %w", err)
	}

	// Find the release that manages this resource
	var matchedRelease *helmRelease
	for _, rel := range releases {
		if h.releaseManagesResource(rel, kind, name) {
			matchedRelease = rel
			break
		}
	}

	if matchedRelease == nil {
		return &TraceResult{
			Object: ResourceRef{
				Kind:      kind,
				Name:      name,
				Namespace: namespace,
			},
			FullyManaged: false,
			Tool:         "helm",
			TracedAt:     time.Now(),
			Error:        "no Helm release found managing this resource",
		}, nil
	}

	return h.buildTraceResult(matchedRelease, kind, name, namespace)
}

// TraceRelease traces a Helm release by name
func (h *HelmTracer) TraceRelease(ctx context.Context, releaseName, namespace string) (*TraceResult, error) {
	release, err := h.getRelease(ctx, releaseName, namespace)
	if err != nil {
		return nil, err
	}

	if release == nil {
		return &TraceResult{
			Object: ResourceRef{
				Kind:      "Release",
				Name:      releaseName,
				Namespace: namespace,
			},
			FullyManaged: false,
			Tool:         "helm",
			TracedAt:     time.Now(),
			Error:        fmt.Sprintf("Helm release '%s' not found in namespace '%s'", releaseName, namespace),
		}, nil
	}

	return h.buildTraceResult(release, "Release", releaseName, namespace)
}

// helmRelease represents a Helm release stored in a Kubernetes secret
type helmRelease struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Version   int               `json:"version"`
	Info      helmReleaseInfo   `json:"info"`
	Chart     helmChart         `json:"chart"`
	Config    map[string]any    `json:"config"`
	Manifest  string            `json:"manifest"`
	Labels    map[string]string `json:"labels"`
}

type helmReleaseInfo struct {
	FirstDeployed time.Time `json:"first_deployed"`
	LastDeployed  time.Time `json:"last_deployed"`
	Deleted       time.Time `json:"deleted"`
	Description   string    `json:"description"`
	Status        string    `json:"status"`
}

type helmChart struct {
	Metadata helmChartMetadata `json:"metadata"`
}

type helmChartMetadata struct {
	Name        string   `json:"name"`
	Home        string   `json:"home"`
	Version     string   `json:"version"`
	AppVersion  string   `json:"appVersion"`
	Description string   `json:"description"`
	Sources     []string `json:"sources"`
}

// listReleases finds all Helm releases in a namespace
func (h *HelmTracer) listReleases(ctx context.Context, namespace string) ([]*helmRelease, error) {
	// Helm stores releases in secrets with owner=helm label
	secrets, err := h.client.CoreV1().Secrets(namespace).List(ctx, v1.ListOptions{
		LabelSelector: "owner=helm",
	})
	if err != nil {
		return nil, err
	}

	var releases []*helmRelease
	releaseMap := make(map[string]*helmRelease)

	for _, secret := range secrets.Items {
		// Secret name format: sh.helm.release.v1.<release-name>.v<version>
		if !strings.HasPrefix(secret.Name, "sh.helm.release.v1.") {
			continue
		}

		release, err := h.decodeRelease(secret.Data["release"])
		if err != nil {
			continue // Skip undecodable releases
		}

		// Keep only the latest version of each release
		existing, ok := releaseMap[release.Name]
		if !ok || release.Version > existing.Version {
			releaseMap[release.Name] = release
		}
	}

	for _, rel := range releaseMap {
		releases = append(releases, rel)
	}

	// Sort by name for consistent output
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Name < releases[j].Name
	})

	return releases, nil
}

// getRelease gets a specific Helm release by name
func (h *HelmTracer) getRelease(ctx context.Context, name, namespace string) (*helmRelease, error) {
	secrets, err := h.client.CoreV1().Secrets(namespace).List(ctx, v1.ListOptions{
		LabelSelector: fmt.Sprintf("owner=helm,name=%s", name),
	})
	if err != nil {
		return nil, err
	}

	var latestRelease *helmRelease
	for _, secret := range secrets.Items {
		if !strings.HasPrefix(secret.Name, "sh.helm.release.v1.") {
			continue
		}

		release, err := h.decodeRelease(secret.Data["release"])
		if err != nil {
			continue
		}

		if latestRelease == nil || release.Version > latestRelease.Version {
			latestRelease = release
		}
	}

	return latestRelease, nil
}

// decodeRelease decodes a Helm release from the secret data
// Helm stores releases as base64(gzip(json))
func (h *HelmTracer) decodeRelease(data []byte) (*helmRelease, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty release data")
	}

	// Base64 decode
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	// Gzip decompress
	reader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("gzip read: %w", err)
	}

	// JSON unmarshal
	var release helmRelease
	if err := json.Unmarshal(decompressed, &release); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return &release, nil
}

// releaseManagesResource checks if a Helm release manages a specific resource
func (h *HelmTracer) releaseManagesResource(release *helmRelease, kind, name string) bool {
	// Check the manifest for the resource
	// The manifest is a multi-document YAML string
	manifest := release.Manifest

	// Simple check: look for the resource in the manifest
	// Format: kind: <Kind> and name: <name>
	kindLower := strings.ToLower(kind)
	nameLower := strings.ToLower(name)

	// Split manifest into documents
	docs := strings.Split(manifest, "---")
	for _, doc := range docs {
		docLower := strings.ToLower(doc)
		if strings.Contains(docLower, "kind: "+kindLower) &&
			strings.Contains(docLower, "name: "+nameLower) {
			return true
		}
	}

	return false
}

// buildTraceResult builds a TraceResult from a Helm release
func (h *HelmTracer) buildTraceResult(release *helmRelease, kind, name, namespace string) (*TraceResult, error) {
	result := &TraceResult{
		Object: ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		Chain:        []ChainLink{},
		FullyManaged: true,
		Tool:         "helm",
		TracedAt:     time.Now(),
	}

	// Determine chart source URL if available
	chartURL := ""
	if len(release.Chart.Metadata.Sources) > 0 {
		chartURL = release.Chart.Metadata.Sources[0]
	}

	// Add chart as source link
	chartLink := ChainLink{
		Kind:     "HelmChart",
		Name:     release.Chart.Metadata.Name,
		Ready:    true,
		Status:   fmt.Sprintf("v%s", release.Chart.Metadata.Version),
		Revision: release.Chart.Metadata.Version,
		URL:      chartURL,
	}
	if release.Chart.Metadata.AppVersion != "" {
		chartLink.Status = fmt.Sprintf("v%s (app: %s)", release.Chart.Metadata.Version, release.Chart.Metadata.AppVersion)
	}
	result.Chain = append(result.Chain, chartLink)

	// Add release link
	releaseReady := release.Info.Status == "deployed"
	releaseLink := ChainLink{
		Kind:      "Release",
		Name:      release.Name,
		Namespace: release.Namespace,
		Ready:     releaseReady,
		Status:    release.Info.Status,
		Revision:  fmt.Sprintf("v%d", release.Version),
		Message:   release.Info.Description,
	}
	if !release.Info.LastDeployed.IsZero() {
		t := release.Info.LastDeployed
		releaseLink.LastTransitionTime = &t
	}
	result.Chain = append(result.Chain, releaseLink)

	if !releaseReady {
		result.FullyManaged = false
	}

	// Add the target resource link (only if not tracing the release itself)
	if kind == "Release" {
		// When tracing a release directly, don't add a redundant resource link
		return result, nil
	}

	resourceLink := ChainLink{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
		Ready:     releaseReady, // Inherit from release status
		Status:    "Managed by Helm",
	}
	result.Chain = append(result.Chain, resourceLink)

	return result, nil
}

// TraceByOwnership traces a resource by its Helm ownership labels
func (h *HelmTracer) TraceByOwnership(ctx context.Context, ownership Ownership) (*TraceResult, error) {
	if ownership.Type != OwnerHelm {
		return nil, fmt.Errorf("resource not owned by Helm")
	}

	// The ownership.Name is the release name
	return h.TraceRelease(ctx, ownership.Name, ownership.Namespace)
}

// GetReleaseHistory returns the deployment history for a Helm release
// History is returned sorted by version descending (most recent first)
func (h *HelmTracer) GetReleaseHistory(ctx context.Context, releaseName, namespace string) ([]HistoryEntry, error) {
	secrets, err := h.client.CoreV1().Secrets(namespace).List(ctx, v1.ListOptions{
		LabelSelector: fmt.Sprintf("owner=helm,name=%s", releaseName),
	})
	if err != nil {
		return nil, err
	}

	var releases []*helmRelease
	for _, secret := range secrets.Items {
		if !strings.HasPrefix(secret.Name, "sh.helm.release.v1.") {
			continue
		}

		release, err := h.decodeRelease(secret.Data["release"])
		if err != nil {
			continue
		}

		releases = append(releases, release)
	}

	if len(releases) == 0 {
		return nil, nil
	}

	// Sort by version descending (most recent first)
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Version > releases[j].Version
	})

	// Convert to HistoryEntry
	history := make([]HistoryEntry, 0, len(releases))
	for _, rel := range releases {
		entry := HistoryEntry{
			Timestamp: rel.Info.LastDeployed,
			Revision:  fmt.Sprintf("v%d", rel.Version),
			Status:    rel.Info.Status,
			Message:   rel.Info.Description,
			Source:    fmt.Sprintf("chart %s-%s", rel.Chart.Metadata.Name, rel.Chart.Metadata.Version),
		}
		history = append(history, entry)
	}

	return history, nil
}
