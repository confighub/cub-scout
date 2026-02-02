package patterns

import (
	"fmt"

	"github.com/confighub/cub-scout/internal/graph"
)

func init() {
	// Argo CD resource presence pattern
	Register(Pattern{
		ID:          "gitops.argocd.resources_present",
		Name:        "Argo CD Resources Present",
		Description: "Reports the presence and count of Argo CD resources (Applications, ApplicationSets). Provides visibility into Argo CD usage without inferring hierarchy.",
		Category:    "gitops",
		Prerequisites: []Prerequisite{
			{Type: "requires_any_of_kinds", Kinds: []string{"Application", "ApplicationSet"}},
		},
		Detect: detectArgoCDResourcesPresent,
	})

	// Flux resource presence pattern
	Register(Pattern{
		ID:          "gitops.flux.resources_present",
		Name:        "Flux Resources Present",
		Description: "Reports the presence and count of Flux resources (Kustomizations, HelmReleases, GitRepositories). Provides visibility into Flux usage without inferring hierarchy.",
		Category:    "gitops",
		Prerequisites: []Prerequisite{
			{Type: "requires_any_of_kinds", Kinds: []string{"Kustomization", "HelmRelease", "GitRepository"}},
		},
		Detect: detectFluxResourcesPresent,
	})
}

// detectArgoCDResourcesPresent reports Argo CD resource counts.
// Prereqs guarantee at least one Argo CD resource exists.
func detectArgoCDResourcesPresent(g *graph.Graph) ([]Finding, Status) {
	var findings []Finding

	// Count by kind
	appCount := 0
	appSetCount := 0

	for _, node := range g.Nodes {
		if node.APIVersion != "argoproj.io/v1alpha1" {
			continue
		}
		switch node.Kind {
		case "Application":
			appCount++
		case "ApplicationSet":
			appSetCount++
		}
	}

	// One finding per kind that's present
	if appCount > 0 {
		findings = append(findings, Finding{
			Pattern:  "gitops.argocd.resources_present",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Argo CD Applications detected: %d", appCount),
		})
	}

	if appSetCount > 0 {
		findings = append(findings, Finding{
			Pattern:  "gitops.argocd.resources_present",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Argo CD ApplicationSets detected: %d", appSetCount),
		})
	}

	return findings, StatusPass
}

// detectFluxResourcesPresent reports Flux resource counts.
// Prereqs guarantee at least one Flux resource exists.
func detectFluxResourcesPresent(g *graph.Graph) ([]Finding, Status) {
	var findings []Finding

	// Count by kind
	kustomizationCount := 0
	helmReleaseCount := 0
	gitRepoCount := 0

	for _, node := range g.Nodes {
		switch {
		case node.Kind == "Kustomization" && node.APIVersion == "kustomize.toolkit.fluxcd.io/v1":
			kustomizationCount++
		case node.Kind == "HelmRelease" && node.APIVersion == "helm.toolkit.fluxcd.io/v2":
			helmReleaseCount++
		case node.Kind == "GitRepository" && node.APIVersion == "source.toolkit.fluxcd.io/v1":
			gitRepoCount++
		}
	}

	// One finding per kind that's present
	if kustomizationCount > 0 {
		findings = append(findings, Finding{
			Pattern:  "gitops.flux.resources_present",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Flux Kustomizations detected: %d", kustomizationCount),
		})
	}

	if helmReleaseCount > 0 {
		findings = append(findings, Finding{
			Pattern:  "gitops.flux.resources_present",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Flux HelmReleases detected: %d", helmReleaseCount),
		})
	}

	if gitRepoCount > 0 {
		findings = append(findings, Finding{
			Pattern:  "gitops.flux.resources_present",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Flux GitRepositories detected: %d", gitRepoCount),
		})
	}

	return findings, StatusPass
}
