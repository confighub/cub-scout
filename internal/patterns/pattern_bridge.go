// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package patterns

import (
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/internal/graph"
)

func init() {
	Register(Pattern{
		ID:          "delivery.bridge.git_flux",
		Name:        "Git → Flux Delivery Bridge",
		Description: "Detects resources delivered through a Git → Flux pipeline (GitRepository → Kustomization/HelmRelease → workloads).",
		Category:    "delivery",
		Prerequisites: []Prerequisite{
			{Type: "requires_any_of_kinds", Kinds: []string{"GitRepository"}},
			{Type: "requires_any_of_kinds", Kinds: []string{"Kustomization", "HelmRelease"}},
		},
		Detect: detectBridgeGitFlux,
	})

	Register(Pattern{
		ID:          "delivery.bridge.git_argocd",
		Name:        "Git → ArgoCD Delivery Bridge",
		Description: "Detects resources delivered through a Git → ArgoCD pipeline (Application → workloads with argocd.argoproj.io/instance labels).",
		Category:    "delivery",
		Prerequisites: []Prerequisite{
			{Type: "requires_any_of_kinds", Kinds: []string{"Application"}},
		},
		Detect: detectBridgeGitArgoCD,
	})

	Register(Pattern{
		ID:          "delivery.bridge.confighub_oci",
		Name:        "ConfigHub → OCI Delivery Bridge",
		Description: "Detects resources delivered through a ConfigHub → OCI pipeline (OCIRepository with ConfigHub origin → deployer → workloads).",
		Category:    "delivery",
		Prerequisites: []Prerequisite{
			{Type: "requires_any_of_kinds", Kinds: []string{"OCIRepository"}},
		},
		Detect: detectBridgeConfigHubOCI,
	})

	Register(Pattern{
		ID:          "delivery.bridge.live_import",
		Name:        "Live Import (ConfigHub without GitOps)",
		Description: "Detects resources with ConfigHub ownership labels but no GitOps deployer, indicating live-imported resources.",
		Category:    "delivery",
		Prerequisites: []Prerequisite{
			{Type: "requires_any_of_kinds", Kinds: []string{"Deployment", "StatefulSet", "DaemonSet"}},
		},
		Detect: detectBridgeLiveImport,
	})
}

// detectBridgeGitFlux detects the Git → Flux delivery bridge.
func detectBridgeGitFlux(g *graph.Graph) ([]Finding, Status) {
	var gitRepos, deployers, managedWorkloads int

	for _, node := range g.Nodes {
		switch node.Kind {
		case "GitRepository":
			gitRepos++
		case "Kustomization", "HelmRelease":
			deployers++
		default:
			// Count workloads managed by Flux (have Flux labels)
			if isFluxManaged(node) {
				managedWorkloads++
			}
		}
	}

	if gitRepos == 0 || deployers == 0 {
		return nil, StatusPass
	}

	confidence := 0.95
	findings := []Finding{
		{
			Pattern:    "delivery.bridge.git_flux",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("Git → Flux delivery bridge: %d sources, %d deployers, %d managed workloads", gitRepos, deployers, managedWorkloads),
			Confidence: &confidence,
			Evidence: []string{
				fmt.Sprintf("GitRepository count: %d", gitRepos),
				fmt.Sprintf("Kustomization/HelmRelease count: %d", deployers),
				fmt.Sprintf("Workloads with Flux labels: %d", managedWorkloads),
			},
		},
	}

	return findings, StatusPass
}

// detectBridgeGitArgoCD detects the Git → ArgoCD delivery bridge.
func detectBridgeGitArgoCD(g *graph.Graph) ([]Finding, Status) {
	var apps, managedWorkloads int

	for _, node := range g.Nodes {
		switch node.Kind {
		case "Application":
			apps++
		default:
			if isArgoManaged(node) {
				managedWorkloads++
			}
		}
	}

	if apps == 0 {
		return nil, StatusPass
	}

	confidence := 0.95
	findings := []Finding{
		{
			Pattern:    "delivery.bridge.git_argocd",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("Git → ArgoCD delivery bridge: %d Applications, %d managed workloads", apps, managedWorkloads),
			Confidence: &confidence,
			Evidence: []string{
				fmt.Sprintf("Application count: %d", apps),
				fmt.Sprintf("Workloads with ArgoCD labels: %d", managedWorkloads),
			},
		},
	}

	return findings, StatusPass
}

// detectBridgeConfigHubOCI detects the ConfigHub → OCI delivery bridge.
func detectBridgeConfigHubOCI(g *graph.Graph) ([]Finding, Status) {
	var configHubOCISources int

	for _, node := range g.Nodes {
		if node.Kind != "OCIRepository" {
			continue
		}
		if isConfigHubOCISource(node) {
			configHubOCISources++
		}
	}

	if configHubOCISources == 0 {
		return nil, StatusPass
	}

	// Count workloads with both deployer + ConfigHub labels
	var dualManaged int
	for _, node := range g.Nodes {
		if !isWorkloadKind(node.Kind) {
			continue
		}
		if isConfigHubManaged(node) && (isFluxManaged(node) || isArgoManaged(node)) {
			dualManaged++
		}
	}

	confidence := 0.9
	findings := []Finding{
		{
			Pattern:    "delivery.bridge.confighub_oci",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("ConfigHub → OCI delivery bridge: %d OCI sources with ConfigHub origin, %d dual-managed workloads", configHubOCISources, dualManaged),
			Confidence: &confidence,
			Evidence: []string{
				fmt.Sprintf("OCIRepository with ConfigHub origin: %d", configHubOCISources),
				fmt.Sprintf("Workloads with both deployer + ConfigHub labels: %d", dualManaged),
			},
		},
	}

	return findings, StatusPass
}

// detectBridgeLiveImport detects live-imported resources (ConfigHub without GitOps deployer).
func detectBridgeLiveImport(g *graph.Graph) ([]Finding, Status) {
	var liveImported int

	for _, node := range g.Nodes {
		if !isWorkloadKind(node.Kind) {
			continue
		}
		if isConfigHubManaged(node) && !isFluxManaged(node) && !isArgoManaged(node) && !isHelmManaged(node) {
			liveImported++
		}
	}

	if liveImported == 0 {
		return nil, StatusPass
	}

	confidence := 0.85
	findings := []Finding{
		{
			Pattern:    "delivery.bridge.live_import",
			Severity:   SeverityInfo,
			Message:    fmt.Sprintf("Live import: %d resources with ConfigHub ownership but no GitOps deployer", liveImported),
			Confidence: &confidence,
			Evidence: []string{
				fmt.Sprintf("Workloads with confighub.com/UnitSlug but no GitOps labels: %d", liveImported),
			},
		},
	}

	return findings, StatusPass
}

// --- Label detection helpers ---

func isFluxManaged(node graph.Node) bool {
	for k := range node.Labels {
		if strings.HasPrefix(k, "kustomize.toolkit.fluxcd.io/") ||
			strings.HasPrefix(k, "helm.toolkit.fluxcd.io/") {
			return true
		}
	}
	return false
}

func isArgoManaged(node graph.Node) bool {
	_, ok := node.Labels["argocd.argoproj.io/instance"]
	return ok
}

func isHelmManaged(node graph.Node) bool {
	return node.Labels["app.kubernetes.io/managed-by"] == "Helm"
}

func isConfigHubManaged(node graph.Node) bool {
	_, ok := node.Labels["confighub.com/UnitSlug"]
	return ok
}

func isConfigHubOCISource(node graph.Node) bool {
	// Detect ConfigHub origin from labels or name convention
	if _, ok := node.Labels["confighub.com/UnitSlug"]; ok {
		return true
	}
	if _, ok := node.Labels["confighub.com/SpaceName"]; ok {
		return true
	}
	return false
}

func isWorkloadKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "Service", "ConfigMap", "Secret", "Ingress",
		"Job", "CronJob":
		return true
	default:
		return false
	}
}
